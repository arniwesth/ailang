package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
)

// generateChat uses the Chat Completions API (/v1/chat/completions).
func (c *Client) generateChat(ctx context.Context, req *ai.Request) (*ai.Response, error) {
	// Build messages
	var messages []chatMessage

	if req.SystemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: req.UserPrompt,
	})

	// Build request
	apiReq := chatRequest{
		Model:    req.Model,
		Messages: messages,
	}

	// Set max tokens based on model type
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	if usesMaxCompletionTokens(req.Model) {
		apiReq.MaxCompletionTokens = maxTokens
	} else {
		apiReq.MaxTokens = maxTokens
	}

	if req.Temperature > 0 {
		apiReq.Temperature = req.Temperature
	}

	// Check for seed in options
	if req.Options != nil {
		if seed, ok := req.Options["seed"].(int64); ok {
			apiReq.Seed = &seed
		}
	}

	// Add structured output configuration
	if req.ResponseFormat == "json" {
		if req.ResponseSchema != "" {
			schema := ensureStrictSchemaCompliance(json.RawMessage(req.ResponseSchema))
			apiReq.ResponseFormat = &chatResponseFormat{
				Type: "json_schema",
				JSONSchema: &chatJSONSchema{
					Name:   "response",
					Schema: schema,
					Strict: true,
				},
			}
		} else {
			apiReq.ResponseFormat = &chatResponseFormat{
				Type: "json_object",
			}
		}
	}

	// Marshal request
	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to create request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "request failed", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ai.NewProviderError("openai", resp.StatusCode, "failed to read response", err)
	}

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, ai.NewProviderError("openai", resp.StatusCode, errResp.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", resp.StatusCode, string(body), nil)
	}

	// Parse successful response
	var result chatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to parse response", err)
	}

	if len(result.Choices) == 0 {
		// OpenRouter (and some proxies) return HTTP 200 with an error object
		// and an empty choices array when the model is unavailable or rate-limited.
		if result.Error != nil && result.Error.Message != "" {
			return nil, ai.NewProviderError("openai", 0, result.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", 0, "no choices in response", nil)
	}

	text := result.Choices[0].Message.Content
	if reasoning := result.Choices[0].Message.Reasoning; reasoning != "" {
		text = "<think>\n" + reasoning + "\n</think>\n\n" + text
	}

	// Calculate output tokens
	// For GPT-5+ reasoning models, completion_tokens includes reasoning_tokens
	outputTokens := result.Usage.CompletionTokens
	reasoningTokens := result.Usage.CompletionTokensDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}

	return &ai.Response{
		Text:         text,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: outputTokens,
		TotalTokens:  result.Usage.TotalTokens,
		ReasonTokens: reasoningTokens,
		Model:        result.Model,
	}, nil
}

// generateChatStream uses Chat Completions SSE streaming.
func (c *Client) generateChatStream(ctx context.Context, req *ai.Request, onEvent ai.StreamHandler) (*ai.Response, error) {
	var messages []chatMessage
	if req.SystemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: req.UserPrompt,
	})

	apiReq := chatRequest{
		Model:         req.Model,
		Messages:      messages,
		Stream:        true,
		StreamOptions: &chatStreamOptions{IncludeUsage: true},
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if usesMaxCompletionTokens(req.Model) {
		apiReq.MaxCompletionTokens = maxTokens
	} else {
		apiReq.MaxTokens = maxTokens
	}
	if req.Temperature > 0 {
		apiReq.Temperature = req.Temperature
	}
	if req.Options != nil {
		if seed, ok := req.Options["seed"].(int64); ok {
			apiReq.Seed = &seed
		}
	}
	if req.ResponseFormat == "json" {
		if req.ResponseSchema != "" {
			schema := ensureStrictSchemaCompliance(json.RawMessage(req.ResponseSchema))
			apiReq.ResponseFormat = &chatResponseFormat{
				Type: "json_schema",
				JSONSchema: &chatJSONSchema{
					Name:   "response",
					Schema: schema,
					Strict: true,
				},
			}
		} else {
			apiReq.ResponseFormat = &chatResponseFormat{Type: "json_object"}
		}
	}

	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
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
	var usage chatUsage

	err = readSSEData(resp.Body, func(payload string) error {
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("invalid chat stream chunk: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return ai.NewProviderError("openai", 0, chunk.Error.Message, nil)
		}
		if chunk.Model != "" {
			modelName = chunk.Model
		}
		if chunk.Usage.TotalTokens > 0 {
			usage = chunk.Usage
		}

		for _, choice := range chunk.Choices {
			delta := extractChatStreamText(choice.Delta.Content)
			if delta == "" {
				continue
			}
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
	})
	if err != nil {
		if pe, ok := err.(*ai.ProviderError); ok {
			return nil, pe
		}
		return nil, ai.NewProviderError("openai", 0, "stream parse failed", err)
	}

	outputTokens := usage.CompletionTokens
	reasoningTokens := usage.CompletionTokensDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}

	return &ai.Response{
		Text:         textBuilder.String(),
		InputTokens:  usage.PromptTokens,
		OutputTokens: outputTokens,
		TotalTokens:  usage.TotalTokens,
		ReasonTokens: reasoningTokens,
		Model:        modelName,
	}, nil
}

func extractChatStreamText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var out strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := m["type"].(string)
			if kind != "text" && kind != "output_text" {
				continue
			}
			text, _ := m["text"].(string)
			out.WriteString(text)
		}
		return out.String()
	default:
		return ""
	}
}
