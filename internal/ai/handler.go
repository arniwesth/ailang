package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Handler wraps a Provider for use with the effects.AIHandler interface.
// This bridges the unified AI package with AILANG's effect system.
type Handler struct {
	provider     Provider
	model        string
	systemPrompt string
	maxTokens    int
}

// HandlerOption configures a Handler.
type HandlerOption func(*Handler)

// WithSystemPrompt sets the system prompt for all requests.
func WithSystemPrompt(prompt string) HandlerOption {
	return func(h *Handler) {
		h.systemPrompt = prompt
	}
}

// WithMaxTokens sets the maximum response tokens.
func WithMaxTokens(tokens int) HandlerOption {
	return func(h *Handler) {
		h.maxTokens = tokens
	}
}

// NewHandler creates a new Handler that wraps a Provider.
//
// The Handler implements effects.AIHandler, allowing any Provider to be
// used with AILANG's AI effect system.
//
// Example:
//
//	client := anthropic.NewClient(apiKey)
//	handler := ai.NewHandler(client, "claude-sonnet-4-5",
//	    ai.WithSystemPrompt("You are a helpful assistant."),
//	    ai.WithMaxTokens(4096),
//	)
//	effCtx.AI = effects.NewAIContext(handler)
func NewHandler(provider Provider, model string, opts ...HandlerOption) *Handler {
	h := &Handler{
		provider:  provider,
		model:     model,
		maxTokens: 4096, // Default
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Call implements effects.AIHandler.
// It sends the input to the provider and returns the generated text.
func (h *Handler) Call(input string) (string, error) {
	resp, err := h.provider.Generate(context.Background(), &Request{
		Model:        h.model,
		SystemPrompt: h.systemPrompt,
		UserPrompt:   input,
		MaxTokens:    h.maxTokens,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// jsonMaxTokensMinimum is the minimum max_tokens for JSON structured output.
// JSON responses are often larger than freeform text (keys, braces, quotes),
// so we use a higher floor to avoid truncation.
const jsonMaxTokensMinimum = 8192

// CallJson sends a request configured for JSON structured output.
// If schema is non-empty, providers enforce the schema on the response.
// If schema is empty, providers return valid JSON without schema enforcement.
//
// Uses at least 8192 max tokens (JSON responses need more room than freeform text).
// Trims whitespace from the response (some providers pad output to token boundary).
func (h *Handler) CallJson(input string, schema string) (string, error) {
	// JSON structured output needs more tokens than freeform text
	maxTokens := h.maxTokens
	if maxTokens < jsonMaxTokensMinimum {
		maxTokens = jsonMaxTokensMinimum
	}

	resp, err := h.provider.Generate(context.Background(), &Request{
		Model:          h.model,
		SystemPrompt:   h.systemPrompt,
		UserPrompt:     input,
		MaxTokens:      maxTokens,
		ResponseFormat: "json",
		ResponseSchema: schema,
	})
	if err != nil {
		return "", err
	}
	// Trim whitespace: some providers (notably Gemini) pad structured output
	// with trailing spaces when approaching the token limit.
	return strings.TrimSpace(resp.Text), nil
}

// CallWithContext is like Call but accepts a context for cancellation/timeout.
func (h *Handler) CallWithContext(ctx context.Context, input string) (string, error) {
	resp, err := h.provider.Generate(ctx, &Request{
		Model:        h.model,
		SystemPrompt: h.systemPrompt,
		UserPrompt:   input,
		MaxTokens:    h.maxTokens,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// CallStream sends a request and emits typed stream deltas as they arrive.
// If the provider does not implement StreamingProvider, this falls back to
// a single-shot call and emits one synthetic delta with the full text.
func (h *Handler) CallStream(input string, onEvent StreamHandler) (*Response, error) {
	return h.CallStreamWithContext(context.Background(), input, onEvent)
}

// CallStreamWithContext is like CallStream but accepts a context.
func (h *Handler) CallStreamWithContext(ctx context.Context, input string, onEvent StreamHandler) (*Response, error) {
	req := &Request{
		Model:        h.model,
		SystemPrompt: h.systemPrompt,
		UserPrompt:   input,
		MaxTokens:    h.maxTokens,
	}

	if sp, ok := h.provider.(StreamingProvider); ok {
		resp, err := sp.GenerateStream(ctx, req, onEvent)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Compatibility fallback for non-streaming providers.
	resp, err := h.provider.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	if onEvent != nil && resp.Text != "" {
		if err := onEvent(StreamEvent{
			Type:      StreamEventDelta,
			Seq:       0,
			TextDelta: resp.Text,
		}); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// ContinueStream submits provider-native tool results and streams the
// continuation response. Returns the full provider response.
func (h *Handler) ContinueStream(continuationID string, results []NativeToolResult, onEvent StreamHandler) (*Response, error) {
	return h.ContinueStreamWithContext(context.Background(), continuationID, results, onEvent)
}

// ContinueStreamWithContext is like ContinueStream but accepts a context.
func (h *Handler) ContinueStreamWithContext(ctx context.Context, continuationID string, results []NativeToolResult, onEvent StreamHandler) (*Response, error) {
	if p, ok := h.provider.(NativeToolStreamingProvider); ok {
		return p.ContinueStream(ctx, h.model, continuationID, results, onEvent)
	}
	return nil, fmt.Errorf("provider %q does not support native tool continuation", h.provider.Name())
}

// GenerateWithDetails returns the full response including token counts.
// This is useful for eval harness and cost tracking.
func (h *Handler) GenerateWithDetails(ctx context.Context, input string) (*Response, error) {
	return h.provider.Generate(ctx, &Request{
		Model:        h.model,
		SystemPrompt: h.systemPrompt,
		UserPrompt:   input,
		MaxTokens:    h.maxTokens,
	})
}

// CallImage generates an image and writes it to outputPath.
// Options is a JSON string: {"aspect_ratio": "16:9", "mime_type": "image/png"}.
func (h *Handler) CallImage(prompt, outputPath, options string) (string, error) {
	opts := parseImageOptions(options)
	resp, err := h.provider.Generate(context.Background(), &Request{
		Model:              h.model,
		SystemPrompt:       h.systemPrompt,
		UserPrompt:         prompt,
		ResponseModalities: []string{"IMAGE"},
		ImageOptions:       opts,
	})
	if err != nil {
		return "", err
	}
	if resp.ImageData == nil {
		return "", fmt.Errorf("provider returned no image data for prompt: %s", prompt)
	}
	// Ensure parent directory exists
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(outputPath, resp.ImageData, 0o644); err != nil {
		return "", fmt.Errorf("failed to write image to %s: %w", outputPath, err)
	}
	return outputPath, nil
}

// CallImageBase64 generates an image and returns JSON with base64 data.
// Returns: {"base64": "...", "mime_type": "image/png"}
func (h *Handler) CallImageBase64(prompt, options string) (string, error) {
	opts := parseImageOptions(options)
	resp, err := h.provider.Generate(context.Background(), &Request{
		Model:              h.model,
		SystemPrompt:       h.systemPrompt,
		UserPrompt:         prompt,
		ResponseModalities: []string{"IMAGE"},
		ImageOptions:       opts,
	})
	if err != nil {
		return "", err
	}
	if resp.ImageData == nil {
		return "", fmt.Errorf("provider returned no image data for prompt: %s", prompt)
	}
	b64 := base64.StdEncoding.EncodeToString(resp.ImageData)
	mime := resp.ImageMIME
	if mime == "" {
		mime = "image/png"
	}
	return fmt.Sprintf(`{"base64":"%s","mime_type":"%s"}`, b64, mime), nil
}

// parseImageOptions parses a JSON options string into ImageOptions.
func parseImageOptions(optionsJSON string) *ImageOptions {
	if optionsJSON == "" || optionsJSON == "{}" {
		return nil
	}
	var raw struct {
		AspectRatio string `json:"aspect_ratio"`
		MIMEType    string `json:"mime_type"`
	}
	if err := json.Unmarshal([]byte(optionsJSON), &raw); err != nil {
		return nil
	}
	if raw.AspectRatio == "" && raw.MIMEType == "" {
		return nil
	}
	return &ImageOptions{
		AspectRatio: raw.AspectRatio,
		MIMEType:    raw.MIMEType,
	}
}

// Provider returns the underlying provider.
func (h *Handler) Provider() Provider {
	return h.provider
}

// Model returns the model name.
func (h *Handler) Model() string {
	return h.model
}
