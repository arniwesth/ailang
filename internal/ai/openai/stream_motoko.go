package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sunholo/ailang/internal/ai"
)

type chatStreamRequestMotoko struct {
	chatRequest
	Stream bool `json:"stream"`
}

type chatStreamChunkMotoko struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// GenerateStream implements ai.StreamingProvider for OpenAI models.
func (c *Client) GenerateStream(ctx context.Context, req *ai.Request, onEvent ai.StreamHandler) (*ai.Response, error) {
	if ai.RequestsImage(req) {
		return nil, ai.NewProviderError("openai", 0, "image generation not supported by provider \"openai\" (model: "+req.Model+") — use a Gemini image model", nil)
	}

	routedClient, routedReq := c, req
	if err := applyModelRoutingMotoko(&routedClient, &routedReq); err != nil {
		return nil, err
	}

	switch routedClient.detectAPIType(routedReq.Model) {
	case APIResponses:
		// Responses API streaming is not yet implemented in this port.
		resp, err := routedClient.generateResponses(ctx, routedReq)
		if err != nil {
			return nil, err
		}
		if onEvent != nil && resp != nil && resp.Text != "" {
			if err := onEvent(ai.StreamEvent{Type: ai.StreamEventDelta, Seq: 0, TextDelta: resp.Text}); err != nil {
				return nil, err
			}
		}
		return resp, nil
	default:
		return routedClient.generateChatStreamMotoko(ctx, routedReq, onEvent)
	}
}

func (c *Client) generateChatStreamMotoko(ctx context.Context, req *ai.Request, onEvent ai.StreamHandler) (*ai.Response, error) {
	var messages []chatMessage
	if req.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: req.UserPrompt})

	apiReq := chatStreamRequestMotoko{
		chatRequest: chatRequest{
			Model:    req.Model,
			Messages: messages,
		},
		Stream: true,
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

	jsonBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to marshal stream request", err)
	}

	timeoutSec := parseEnvIntOpenAIMotoko("OPENAI_STREAM_TIMEOUT_SEC", 120)
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "failed to create stream request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, ai.NewProviderError("openai", 0, "stream request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, ai.NewProviderError("openai", resp.StatusCode, errResp.Error.Message, nil)
		}
		return nil, ai.NewProviderError("openai", resp.StatusCode, strings.TrimSpace(string(body)), nil)
	}

	seq := 0
	var textBuilder strings.Builder
	streamModel := req.Model

	err = readSSEDataMotoko(resp.Body, func(payload string) error {
		var chunk chatStreamChunkMotoko
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return ai.NewProviderError("openai", 0, "failed to parse stream chunk", err)
		}
		if chunk.Model != "" {
			streamModel = chunk.Model
		}
		sawFinish := false
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				if choice.FinishReason != "" {
					sawFinish = true
				}
				continue
			}
			textBuilder.WriteString(delta)
			if onEvent != nil {
				evt := ai.StreamEvent{Type: ai.StreamEventDelta, Seq: seq, TextDelta: delta}
				if err := onEvent(evt); err != nil {
					return err
				}
			}
			seq++
			if choice.FinishReason != "" {
				sawFinish = true
			}
		}
		// Some OpenAI-compatible servers emit finish_reason but omit [DONE].
		// Treat finish_reason as terminal to avoid waiting on an open socket forever.
		if sawFinish {
			return io.EOF
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	text := textBuilder.String()
	if text == "" {
		return nil, ai.NewProviderError("openai", 0, "no streamed text in response", nil)
	}

	return &ai.Response{Text: text, Model: streamModel}, nil
}

// readSSEDataMotoko reads text/event-stream frames and calls onData for each
// aggregated data payload block.
func readSSEDataMotoko(body io.Reader, onData func(string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var dataLines []string
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			return io.EOF
		}
		return onData(payload)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed reading stream: %w", err)
	}
	if err := flush(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func parseEnvIntOpenAIMotoko(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
