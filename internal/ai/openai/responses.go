package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sunholo/ailang/internal/ai"
)

// generateResponses uses the Responses API (/v1/responses).
// This is used for codex models that support autonomous operation and reasoning.
func (c *Client) generateResponses(ctx context.Context, req *ai.Request) (*ai.Response, error) {
	input := makeResponsesInput(req.SystemPrompt, req.UserPrompt)

	// Build request
	apiReq := responsesRequest{
		Model: req.Model,
		Input: input,
		Tools: makeMotokoTools(),
	}
	apiReq.ToolChoice = openAIToolChoice()

	// Set max tokens if specified
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16384 // Codex models support larger outputs
	}
	apiReq.MaxTokens = maxTokens

	// Set reasoning effort from options (default: medium)
	effort := "medium"
	if req.Options != nil {
		if e, ok := req.Options["reasoning_effort"].(string); ok {
			effort = e
		}
	}
	apiReq.Reasoning = &responsesReasoning{Effort: effort}

	// Add structured output configuration
	if req.ResponseFormat == "json" {
		if req.ResponseSchema != "" {
			schema := ensureStrictSchemaCompliance(json.RawMessage(req.ResponseSchema))
			apiReq.Text = &responsesText{
				Format: responsesTextFormat{
					Type:   "json_schema",
					Name:   "response",
					Schema: schema,
					Strict: true,
				},
			}
		} else {
			apiReq.Text = &responsesText{
				Format: responsesTextFormat{
					Type: "json_object",
				},
			}
		}
	}

	return c.generateResponsesWithRequest(ctx, &apiReq)
}

// generateResponsesStream uses Responses API SSE stream.
func (c *Client) generateResponsesStream(ctx context.Context, req *ai.Request, onEvent ai.StreamHandler) (*ai.Response, error) {
	ctx, cancel := withOpenAIStreamTimeout(ctx)
	defer cancel()

	input := makeResponsesInput(req.SystemPrompt, req.UserPrompt)

	apiReq := responsesRequest{
		Model:  req.Model,
		Input:  input,
		Stream: true,
		Tools:  makeMotokoTools(),
	}
	apiReq.ToolChoice = openAIToolChoice()
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16384
	}
	apiReq.MaxTokens = maxTokens

	effort := "medium"
	if req.Options != nil {
		if e, ok := req.Options["reasoning_effort"].(string); ok {
			effort = e
		}
	}
	apiReq.Reasoning = &responsesReasoning{Effort: effort}

	if req.ResponseFormat == "json" {
		if req.ResponseSchema != "" {
			schema := ensureStrictSchemaCompliance(json.RawMessage(req.ResponseSchema))
			apiReq.Text = &responsesText{
				Format: responsesTextFormat{
					Type:   "json_schema",
					Name:   "response",
					Schema: schema,
					Strict: true,
				},
			}
		} else {
			apiReq.Text = &responsesText{
				Format: responsesTextFormat{Type: "json_object"},
			}
		}
	}

	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/responses", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(c.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, ai.NewProviderError("openai", resp.StatusCode, errResp.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", resp.StatusCode, string(body), nil)
	}

	var textBuilder strings.Builder
	seq := 0
	modelName := req.Model
	usage := responsesUsage{}
	completedPayloadText := ""
	continuationID := ""
	toolCallsByID := map[string]ai.NativeToolCall{}
	toolOrder := make([]string, 0, 8)

	err = readSSEData(resp.Body, func(payload string) error {
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			// Some OpenAI-compatible backends emit non-standard/non-JSON frames.
			// Ignore these frames instead of aborting the whole stream.
			return nil
		}
		if event.Error != nil && event.Error.Message != "" {
			return ai.NewProviderError("openai", 0, event.Error.Message, nil)
		}

		if event.Type == "response.output_text.delta" {
			delta := rawJSONToString(event.Delta)
			if delta != "" {
				textBuilder.WriteString(delta)
				if onEvent != nil {
					if err := onEvent(ai.StreamEvent{
						Type:      ai.StreamEventDelta,
						Seq:       seq,
						TextDelta: delta,
					}); err != nil {
						return err
					}
				}
				seq++
			}
			return nil
		}

		if event.Type == "response.output_item.done" && event.Item != nil && isToolCallType(event.Item.Type) {
			call := ai.NativeToolCall{
				ProviderCallID: firstNonEmpty(event.Item.CallID, event.Item.ID),
				Name:           event.Item.Name,
				ArgumentsJSON:  event.Item.Arguments,
			}
			if call.ProviderCallID != "" {
				if _, ok := toolCallsByID[call.ProviderCallID]; !ok {
					toolOrder = append(toolOrder, call.ProviderCallID)
				}
				toolCallsByID[call.ProviderCallID] = call
			}
			if onEvent != nil {
				if err := onEvent(ai.StreamEvent{
					Type:     ai.StreamEventToolCall,
					Seq:      seq,
					ToolCall: &call,
				}); err != nil {
					return err
				}
			}
			return nil
		}

		if event.Type == "response.completed" && event.Response != nil {
			if continuationID == "" {
				continuationID = event.ResponseID
			}
			if event.Response.Model != "" {
				modelName = event.Response.Model
			}
			usage = event.Response.Usage
			completedPayloadText = extractResponsesTextFromOutput(event.Response.Output)
			for _, call := range extractNativeToolCalls(event.Response.Output) {
				if call.ProviderCallID == "" {
					continue
				}
				if _, ok := toolCallsByID[call.ProviderCallID]; !ok {
					toolOrder = append(toolOrder, call.ProviderCallID)
				}
				toolCallsByID[call.ProviderCallID] = call
			}
		}
		return nil
	})
	if err != nil {
		if pe, ok := err.(*ai.ProviderError); ok {
			return nil, pe
		}
		fallbackReq := apiReq
		fallbackReq.Stream = false
		fallbackResp, fallbackErr := c.generateResponsesWithRequest(ctx, &fallbackReq)
		if fallbackErr == nil {
			if onEvent != nil && fallbackResp.Text != "" {
				_ = onEvent(ai.StreamEvent{Type: ai.StreamEventDelta, Seq: seq, TextDelta: fallbackResp.Text})
			}
			return fallbackResp, nil
		}
		return nil, ai.NewProviderError("openai", 0, "stream parse failed: "+err.Error(), err)
	}

	if textBuilder.Len() == 0 && completedPayloadText != "" {
		textBuilder.WriteString(completedPayloadText)
		if onEvent != nil {
			if err := onEvent(ai.StreamEvent{
				Type:      ai.StreamEventDelta,
				Seq:       seq,
				TextDelta: completedPayloadText,
			}); err != nil {
				return nil, err
			}
		}
	}

	outputTokens := usage.OutputTokens
	reasoningTokens := usage.OutputDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}

	toolCalls := make([]ai.NativeToolCall, 0, len(toolOrder))
	for _, id := range toolOrder {
		call, ok := toolCallsByID[id]
		if !ok {
			continue
		}
		toolCalls = append(toolCalls, call)
	}
	return &ai.Response{
		Text:            textBuilder.String(),
		InputTokens:     usage.InputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     usage.TotalTokens,
		ReasonTokens:    reasoningTokens,
		Model:           modelName,
		NativeToolCalls: toolCalls,
		ContinuationID:  continuationID,
	}, nil
}

func extractResponsesTextFromOutput(output []responsesOutputItem) string {
	var textBuilder strings.Builder
	for _, item := range output {
		if item.Type == "message" && item.Role == "assistant" {
			for _, content := range item.Content {
				if content.Type != "output_text" {
					continue
				}
				if textBuilder.Len() > 0 {
					textBuilder.WriteString("\n")
				}
				textBuilder.WriteString(content.Text)
			}
		}
		if item.Type == "reasoning" {
			for _, s := range item.Summary {
				if strings.TrimSpace(s.Text) == "" {
					continue
				}
				if textBuilder.Len() > 0 {
					textBuilder.WriteString("\n")
				}
				textBuilder.WriteString(s.Text)
			}
		}
	}
	return textBuilder.String()
}

func extractNativeToolCalls(output []responsesOutputItem) []ai.NativeToolCall {
	calls := make([]ai.NativeToolCall, 0, 4)
	for _, item := range output {
		if !isToolCallType(item.Type) {
			continue
		}
		if item.Name == "" {
			continue
		}
		calls = append(calls, ai.NativeToolCall{
			ProviderCallID: firstNonEmpty(item.CallID, item.ID),
			Name:           item.Name,
			ArgumentsJSON:  item.Arguments,
		})
	}
	return calls
}

func isToolCallType(t string) bool {
	return t == "function_call" || t == "tool_call" || t == "function"
}

func firstNonEmpty(a string, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func makeResponsesInput(systemPrompt string, userPrompt string) []responsesInputItem {
	input := make([]responsesInputItem, 0, 2)
	if systemPrompt != "" {
		input = append(input, responsesInputItem{
			Role:    "developer",
			Content: systemPrompt,
		})
	}
	input = append(input, responsesInputItem{
		Role:    "user",
		Content: userPrompt,
	})
	return input
}

func makeContinuationInput(results []ai.NativeToolResult) []responsesInputItem {
	input := make([]responsesInputItem, 0, len(results))
	for _, r := range results {
		if strings.TrimSpace(r.ProviderCallID) == "" {
			continue
		}
		input = append(input, responsesInputItem{
			Type:   "function_call_output",
			CallID: r.ProviderCallID,
			Output: r.OutputJSON,
		})
	}
	return input
}

func (c *Client) continueResponsesStream(ctx context.Context, model string, continuationID string, results []ai.NativeToolResult, onEvent ai.StreamHandler) (*ai.Response, error) {
	apiReq := responsesRequest{
		Model:              model,
		Input:              makeContinuationInput(results),
		Stream:             true,
		PreviousResponseID: continuationID,
		Tools:              makeMotokoTools(),
		ToolChoice:         openAIToolChoice(),
	}
	return c.generateResponsesStreamWithRequest(ctx, &apiReq, onEvent)
}

func (c *Client) generateResponsesStreamWithRequest(ctx context.Context, apiReq *responsesRequest, onEvent ai.StreamHandler) (*ai.Response, error) {
	ctx, cancel := withOpenAIStreamTimeout(ctx)
	defer cancel()

	// Preserve original logic by routing through the same request path used by generateResponsesStream.
	// generateResponsesStream builds apiReq itself; this helper allows continuation requests.
	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/responses", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(c.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, ai.NewProviderError("openai", resp.StatusCode, errResp.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", resp.StatusCode, string(body), nil)
	}

	// Reuse standard stream parser by decoding directly here.
	var textBuilder strings.Builder
	seq := 0
	modelName := apiReq.Model
	usage := responsesUsage{}
	completedPayloadText := ""
	continuationResponseID := ""
	toolCallsByID := map[string]ai.NativeToolCall{}
	toolOrder := make([]string, 0, 8)

	err = readSSEData(resp.Body, func(payload string) error {
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil
		}
		if event.Error != nil && event.Error.Message != "" {
			return ai.NewProviderError("openai", 0, event.Error.Message, nil)
		}
		if event.Type == "response.output_text.delta" {
			delta := rawJSONToString(event.Delta)
			if delta != "" {
				textBuilder.WriteString(delta)
				if onEvent != nil {
					if err := onEvent(ai.StreamEvent{Type: ai.StreamEventDelta, Seq: seq, TextDelta: delta}); err != nil {
						return err
					}
				}
				seq++
			}
			return nil
		}
		if event.Type == "response.output_item.done" && event.Item != nil && isToolCallType(event.Item.Type) {
			call := ai.NativeToolCall{
				ProviderCallID: firstNonEmpty(event.Item.CallID, event.Item.ID),
				Name:           event.Item.Name,
				ArgumentsJSON:  event.Item.Arguments,
			}
			if call.ProviderCallID != "" {
				if _, ok := toolCallsByID[call.ProviderCallID]; !ok {
					toolOrder = append(toolOrder, call.ProviderCallID)
				}
				toolCallsByID[call.ProviderCallID] = call
			}
			if onEvent != nil {
				if err := onEvent(ai.StreamEvent{Type: ai.StreamEventToolCall, Seq: seq, ToolCall: &call}); err != nil {
					return err
				}
			}
			return nil
		}
		if event.Type == "response.completed" && event.Response != nil {
			if continuationResponseID == "" {
				continuationResponseID = event.ResponseID
			}
			if event.Response.Model != "" {
				modelName = event.Response.Model
			}
			usage = event.Response.Usage
			completedPayloadText = extractResponsesTextFromOutput(event.Response.Output)
			for _, call := range extractNativeToolCalls(event.Response.Output) {
				if call.ProviderCallID == "" {
					continue
				}
				if _, ok := toolCallsByID[call.ProviderCallID]; !ok {
					toolOrder = append(toolOrder, call.ProviderCallID)
				}
				toolCallsByID[call.ProviderCallID] = call
			}
		}
		return nil
	})
	if err != nil {
		if pe, ok := err.(*ai.ProviderError); ok {
			return nil, pe
		}
		fallbackReq := *apiReq
		fallbackReq.Stream = false
		fallbackResp, fallbackErr := c.generateResponsesWithRequest(ctx, &fallbackReq)
		if fallbackErr == nil {
			if onEvent != nil && fallbackResp.Text != "" {
				_ = onEvent(ai.StreamEvent{Type: ai.StreamEventDelta, Seq: seq, TextDelta: fallbackResp.Text})
			}
			return fallbackResp, nil
		}
		return nil, ai.NewProviderError("openai", 0, "stream parse failed: "+err.Error(), err)
	}

	if textBuilder.Len() == 0 && completedPayloadText != "" {
		textBuilder.WriteString(completedPayloadText)
		if onEvent != nil {
			if err := onEvent(ai.StreamEvent{
				Type:      ai.StreamEventDelta,
				Seq:       seq,
				TextDelta: completedPayloadText,
			}); err != nil {
				return nil, err
			}
		}
	}

	outputTokens := usage.OutputTokens
	reasoningTokens := usage.OutputDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}
	toolCalls := make([]ai.NativeToolCall, 0, len(toolOrder))
	for _, id := range toolOrder {
		call, ok := toolCallsByID[id]
		if !ok {
			continue
		}
		toolCalls = append(toolCalls, call)
	}
	return &ai.Response{
		Text:            textBuilder.String(),
		InputTokens:     usage.InputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     usage.TotalTokens,
		ReasonTokens:    reasoningTokens,
		Model:           modelName,
		NativeToolCalls: toolCalls,
		ContinuationID:  continuationResponseID,
	}, nil
}

func makeMotokoTools() []responsesTool {
	return []responsesTool{
		{
			Type:        "function",
			Name:        "ReadFile",
			Description: "Read a text file in the workspace by line range.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"start":{"type":"integer","minimum":1},"end":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Type:        "function",
			Name:        "Search",
			Description: "Search files in a directory using a regex pattern.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"dir":{"type":"string"},"context":{"type":"integer","minimum":0}},"required":["pattern"],"additionalProperties":false}`),
		},
		{
			Type:        "function",
			Name:        "WriteFile",
			Description: "Write full file content to a workspace path.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
		},
		{
			Type:        "function",
			Name:        "EditFile",
			Description: "Apply structured edits to a file path.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old":{"type":"string"},"new":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old","new"],"additionalProperties":false}},"dry_run":{"type":"boolean"},"expected_sha256":{"type":"string"}},"required":["path","edits"],"additionalProperties":false}`),
		},
		{
			Type:        "function",
			Name:        "BashExec",
			Description: "Run a bash command with optional args and execution options.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"cwd":{"type":"string"},"streaming":{"type":"boolean"},"needs_stderr_live":{"type":"boolean"},"needs_hard_cancel":{"type":"boolean"}},"required":["cmd"],"additionalProperties":false}`),
		},
		{
			Type:        "function",
			Name:        "RunTests",
			Description: "Run tests as a process command with optional args and execution options.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"cwd":{"type":"string"},"streaming":{"type":"boolean"},"needs_stderr_live":{"type":"boolean"},"needs_hard_cancel":{"type":"boolean"}},"required":["cmd"],"additionalProperties":false}`),
		},
	}
}

func openAIToolChoice() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OPENAI_TOOL_CHOICE")))
	if v == "required" {
		return "required"
	}
	return "auto"
}

func (c *Client) generateResponsesWithRequest(ctx context.Context, apiReq *responsesRequest) (*ai.Response, error) {
	reqCopy := *apiReq
	reqCopy.Stream = false

	jsonBody, err := json.Marshal(reqCopy)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/responses", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "request failed", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ai.NewProviderError("openai", resp.StatusCode, "failed to read response", err)
	}
	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, ai.NewProviderError("openai", resp.StatusCode, errResp.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", resp.StatusCode, string(body), nil)
	}
	var result responsesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to parse response", err)
	}
	text := extractResponsesTextFromOutput(result.Output)
	nativeCalls := extractNativeToolCalls(result.Output)
	outputTokens := result.Usage.OutputTokens
	reasoningTokens := result.Usage.OutputDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}
	return &ai.Response{
		Text:            text,
		InputTokens:     result.Usage.InputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     result.Usage.TotalTokens,
		ReasonTokens:    reasoningTokens,
		Model:           result.Model,
		NativeToolCalls: nativeCalls,
		ContinuationID:  result.ID,
	}, nil
}

func rawJSONToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj["text"].(string); ok {
			return v
		}
		if v, ok := obj["value"].(string); ok {
			return v
		}
	}
	return ""
}

func withOpenAIStreamTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	const defaultMs = 45000
	raw := strings.TrimSpace(os.Getenv("OPENAI_STREAM_TIMEOUT_MS"))
	ms := defaultMs
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			ms = n
		}
	}
	return context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
}
