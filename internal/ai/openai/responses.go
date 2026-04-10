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

// generateResponses uses the Responses API (/v1/responses).
// This is used for codex models that support autonomous operation and reasoning.
func (c *Client) generateResponses(ctx context.Context, req *ai.Request) (*ai.Response, error) {
	// Build input array with developer/user roles
	var input []responsesInput

	// Map SystemPrompt to "developer" role (Responses API equivalent of "system")
	if req.SystemPrompt != "" {
		input = append(input, responsesInput{
			Role:    "developer",
			Content: req.SystemPrompt,
		})
	}

	input = append(input, responsesInput{
		Role:    "user",
		Content: req.UserPrompt,
	})

	// Build request
	apiReq := responsesRequest{
		Model: req.Model,
		Input: input,
	}

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

	// Marshal request
	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal request", err)
	}

	// Create HTTP request to /v1/responses endpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/responses", bytes.NewReader(jsonBody))
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
	var result responsesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to parse response", err)
	}

	// Extract text from polymorphic output items
	// Output can contain "message", "reasoning", "function_call" types
	var textBuilder strings.Builder
	for _, item := range result.Output {
		if item.Type == "message" && item.Role == "assistant" {
			for _, content := range item.Content {
				if content.Type == "output_text" {
					if textBuilder.Len() > 0 {
						textBuilder.WriteString("\n")
					}
					textBuilder.WriteString(content.Text)
				}
			}
		}
	}

	text := textBuilder.String()
	if text == "" {
		return nil, ai.NewProviderError("openai", 0, "no text output in response", nil)
	}

	// Calculate output tokens (subtract reasoning tokens from total output)
	outputTokens := result.Usage.OutputTokens
	reasoningTokens := result.Usage.OutputDetails.ReasoningTokens
	if reasoningTokens > 0 {
		outputTokens = outputTokens - reasoningTokens
	}

	return &ai.Response{
		Text:         text,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  result.Usage.TotalTokens,
		ReasonTokens: reasoningTokens,
		Model:        result.Model,
	}, nil
}

// generateResponsesStream uses Responses API SSE stream.
func (c *Client) generateResponsesStream(ctx context.Context, req *ai.Request, onEvent ai.StreamHandler) (*ai.Response, error) {
	var input []responsesInput
	if req.SystemPrompt != "" {
		input = append(input, responsesInput{
			Role:    "developer",
			Content: req.SystemPrompt,
		})
	}
	input = append(input, responsesInput{
		Role:    "user",
		Content: req.UserPrompt,
	})

	apiReq := responsesRequest{
		Model:  req.Model,
		Input:  input,
		Stream: true,
	}
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

	err = readSSEData(resp.Body, func(payload string) error {
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("invalid responses stream event: %w", err)
		}
		if event.Error != nil && event.Error.Message != "" {
			return ai.NewProviderError("openai", 0, event.Error.Message, nil)
		}

		if event.Type == "response.output_text.delta" {
			if event.Delta != "" {
				textBuilder.WriteString(event.Delta)
				if onEvent != nil {
					if err := onEvent(ai.StreamEvent{
						Type:      ai.StreamEventDelta,
						Seq:       seq,
						TextDelta: event.Delta,
					}); err != nil {
						return err
					}
				}
				seq++
			}
			return nil
		}

		if event.Type == "response.completed" && event.Response != nil {
			if event.Response.Model != "" {
				modelName = event.Response.Model
			}
			usage = event.Response.Usage
			completedPayloadText = extractResponsesTextFromOutput(event.Response.Output)
		}
		return nil
	})
	if err != nil {
		if pe, ok := err.(*ai.ProviderError); ok {
			return nil, pe
		}
		return nil, ai.NewProviderError("openai", 0, "stream parse failed", err)
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

	return &ai.Response{
		Text:         textBuilder.String(),
		InputTokens:  usage.InputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  usage.TotalTokens,
		ReasonTokens: reasoningTokens,
		Model:        modelName,
	}, nil
}

func extractResponsesTextFromOutput(output []responsesOutputItem) string {
	var textBuilder strings.Builder
	for _, item := range output {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
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
	return textBuilder.String()
}
