package effects

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
	"github.com/sunholo/ailang/internal/eval"
)

// ErrNoAIHandler is returned when AI.call is invoked without a configured handler
var ErrNoAIHandler = errors.New("no AI model configured — add --ai <model> flag (e.g. --ai gemini-2-5-flash), or --ai-stub for testing")

// AIHandler interface for pluggable AI implementation
//
// The AI effect is AILANG's general-purpose AI oracle - an opaque, host-provided
// effect for calling external AI/ML systems. Use cases include:
//   - Game NPC decision-making (via typed wrappers)
//   - CLI tools with AI assistance
//   - Agents written in AILANG calling LLMs
//   - Data analysis pipelines with AI steps
//
// The interface is intentionally simple: string → string
// By convention, JSON is used for input/output, but this is not enforced.
type AIHandler interface {
	Call(input string) (string, error)
	// CallJson sends a request configured for structured JSON output.
	// If schema is non-empty, providers enforce the schema.
	// Returns raw JSON string (caller parses to Json ADT).
	CallJson(input string, schema string) (string, error)
	// CallImage generates an image and writes it to outputPath.
	// Options is a JSON string with optional fields: aspect_ratio, mime_type.
	// Returns the output path on success.
	CallImage(prompt string, outputPath string, options string) (string, error)
	// CallImageBase64 generates an image and returns it as a JSON string
	// containing base64-encoded data: {"base64": "...", "mime_type": "image/png"}.
	CallImageBase64(prompt string, options string) (string, error)
}

// AIContext holds the handler for the current execution
//
// Thread-safety: AIContext is designed for single-threaded use
// within one evaluation. Create a new context for each step/tick.
type AIContext struct {
	handler AIHandler
}

// NewAIContext creates a context with the given handler
//
// IMPORTANT: Pass nil only in tests to verify error handling.
// Production code should always have a real handler or explicit stub.
func NewAIContext(handler AIHandler) *AIContext {
	return &AIContext{handler: handler}
}

// Call invokes the AI handler with the given input
//
// Returns ErrNoAIHandler if no handler is configured.
// This is intentional - no silent fallbacks for AI calls.
func (c *AIContext) Call(input string) (string, error) {
	if c.handler == nil {
		return "", ErrNoAIHandler
	}
	return c.handler.Call(input)
}

// CallJson invokes the AI handler requesting structured JSON output.
// If schema is non-empty, providers enforce the schema on the response.
func (c *AIContext) CallJson(input string, schema string) (string, error) {
	if c.handler == nil {
		return "", ErrNoAIHandler
	}
	return c.handler.CallJson(input, schema)
}

// CallImage generates an image and writes it to outputPath.
func (c *AIContext) CallImage(prompt, outputPath, options string) (string, error) {
	if c.handler == nil {
		return "", ErrNoAIHandler
	}
	return c.handler.CallImage(prompt, outputPath, options)
}

// CallImageBase64 generates an image and returns it as base64 JSON.
func (c *AIContext) CallImageBase64(prompt, options string) (string, error) {
	if c.handler == nil {
		return "", ErrNoAIHandler
	}
	return c.handler.CallImageBase64(prompt, options)
}

// StubAIHandler returns deterministic placeholder responses
//
// Use for testing and development. Supports:
//   - Default response for all inputs
//   - Per-input canned responses
type StubAIHandler struct {
	defaultResponse string
	responses       map[string]string // exact match input → response
}

// NewStubAIHandler creates a stub handler with a sensible default
func NewStubAIHandler() *StubAIHandler {
	return &StubAIHandler{
		defaultResponse: `{"kind":"Wait"}`,
		responses:       make(map[string]string),
	}
}

// Call returns the configured response for the input
func (h *StubAIHandler) Call(input string) (string, error) {
	if resp, ok := h.responses[input]; ok {
		return resp, nil
	}
	return h.defaultResponse, nil
}

// CallJson returns valid JSON for structured output requests.
// The stub returns the default response (which is valid JSON).
func (h *StubAIHandler) CallJson(input string, schema string) (string, error) {
	if resp, ok := h.responses[input]; ok {
		return resp, nil
	}
	return h.defaultResponse, nil
}

// SetResponse sets a canned response for an exact input match
func (h *StubAIHandler) SetResponse(input, response string) {
	h.responses[input] = response
}

// SetDefaultResponse sets the fallback for unmatched inputs
func (h *StubAIHandler) SetDefaultResponse(response string) {
	h.defaultResponse = response
}

// CallImage returns a stub image path (writes a minimal 1x1 PNG to disk).
func (h *StubAIHandler) CallImage(prompt, outputPath, options string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", fmt.Errorf("stub: failed to create directory: %w", err)
	}
	if err := os.WriteFile(outputPath, stubPNG, 0o644); err != nil {
		return "", fmt.Errorf("stub: failed to write image: %w", err)
	}
	return outputPath, nil
}

// CallImageBase64 returns a stub base64 JSON response.
func (h *StubAIHandler) CallImageBase64(prompt, options string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(stubPNG)
	return fmt.Sprintf(`{"base64":"%s","mime_type":"image/png"}`, b64), nil
}

// stubPNG is a minimal valid 1x1 transparent PNG (67 bytes).
var stubPNG = func() []byte {
	// Minimal 1x1 RGBA PNG
	b, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	)
	return b
}()

// init registers AI effect operations
func init() {
	RegisterOp("AI", "call", aiCall)
	RegisterOp("AI", "callJson", aiCallJson)
	RegisterOp("AI", "callJsonSimple", aiCallJsonSimple)
	RegisterOp("AI", "callImage", aiCallImage)
	RegisterOp("AI", "callImageBase64", aiCallImageBase64)
	RegisterOp("AI", "callResult", aiCallResult)
	RegisterOp("AI", "callJsonResult", aiCallJsonResult)
	RegisterOp("AI", "callJsonSimpleResult", aiCallJsonSimpleResult)
}

// retryableStatuses is the set of HTTP status codes that warrant a retry.
var retryableStatuses = map[int]bool{
	408: true,
	409: true,
	425: true,
	429: true,
	500: true,
	502: true,
	503: true,
	504: true,
}

// classifyAIError maps a Go error from an AI call into the structured result record fields.
func classifyAIError(err error) (provider, errorCode, message string, statusCode int, retryable bool) {
	var provErr *ai.ProviderError
	if errors.As(err, &provErr) {
		provider = provErr.Provider
		statusCode = provErr.StatusCode
		message = provErr.Error()
		retryable = retryableStatuses[statusCode]
		if statusCode > 0 {
			errorCode = fmt.Sprintf("HTTP_%d", statusCode)
		} else {
			errorCode = "E_PROVIDER"
		}
		return
	}
	// Fallback: inspect error message for transport-level transients.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection refused") {
		retryable = true
		errorCode = "E_TRANSPORT"
	} else {
		retryable = false
		errorCode = "E_UNKNOWN"
	}
	message = err.Error()
	return
}

// makeAIResultRecord builds the structured result record returned by callResult variants.
// ok=true: output populated, error fields are zero-valued.
// ok=false: error fields populated, output="".
func makeAIResultRecord(ok bool, output string, err error) *eval.RecordValue {
	if ok {
		return &eval.RecordValue{Fields: map[string]eval.Value{
			"ok":            &eval.BoolValue{Value: true},
			"output":        &eval.StringValue{Value: output},
			"error_message": &eval.StringValue{Value: ""},
			"provider":      &eval.StringValue{Value: ""},
			"status_code":   &eval.IntValue{Value: 0},
			"retryable":     &eval.BoolValue{Value: false},
			"error_code":    &eval.StringValue{Value: ""},
		}}
	}
	provider, errorCode, message, statusCode, retryable := classifyAIError(err)
	return &eval.RecordValue{Fields: map[string]eval.Value{
		"ok":            &eval.BoolValue{Value: false},
		"output":        &eval.StringValue{Value: ""},
		"error_message": &eval.StringValue{Value: message},
		"provider":      &eval.StringValue{Value: provider},
		"status_code":   &eval.IntValue{Value: statusCode},
		"retryable":     &eval.BoolValue{Value: retryable},
		"error_code":    &eval.StringValue{Value: errorCode},
	}}
}

// aiCallResult implements AI.callResult(input: string) -> {ok, output, error_message, ...}
// Never returns a Go error for provider failures; only returns Go errors for programmer misuse.
func aiCallResult(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callResult: expected 1 argument, got %d", len(args))
	}
	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callResult: expected string input, got %T", args[0])
	}
	if ctx.AI == nil {
		return makeAIResultRecord(false, "", ErrNoAIHandler), nil
	}
	output, err := ctx.AI.Call(input.Value)
	if err != nil {
		return makeAIResultRecord(false, "", err), nil
	}
	return makeAIResultRecord(true, output, nil), nil
}

// aiCallJsonResult implements AI.callJsonResult(input: string, schema: string) -> {ok, output, ...}
func aiCallJsonResult(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonResult: expected 2 arguments, got %d", len(args))
	}
	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonResult: expected string input, got %T", args[0])
	}
	schema, ok := args[1].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonResult: expected string schema, got %T", args[1])
	}
	if ctx.AI == nil {
		return makeAIResultRecord(false, "", ErrNoAIHandler), nil
	}
	output, err := ctx.AI.CallJson(input.Value, schema.Value)
	if err != nil {
		return makeAIResultRecord(false, "", err), nil
	}
	return makeAIResultRecord(true, output, nil), nil
}

// aiCallJsonSimpleResult implements AI.callJsonSimpleResult(input: string) -> {ok, output, ...}
func aiCallJsonSimpleResult(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonSimpleResult: expected 1 argument, got %d", len(args))
	}
	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonSimpleResult: expected string input, got %T", args[0])
	}
	if ctx.AI == nil {
		return makeAIResultRecord(false, "", ErrNoAIHandler), nil
	}
	output, err := ctx.AI.CallJson(input.Value, "")
	if err != nil {
		return makeAIResultRecord(false, "", err), nil
	}
	return makeAIResultRecord(true, output, nil), nil
}

// aiCall implements AI.call(input: string) -> string
//
// Invokes the configured AI handler with the input string.
// Returns the handler's response or an error if no handler is configured.
//
// Parameters:
//   - ctx: Effect context
//   - args: [StringValue (input)]
//
// Returns:
//   - StringValue with handler response
func aiCall(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: call: expected 1 argument, got %d", len(args))
	}

	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: call: expected string input, got %T", args[0])
	}

	// Get AI context - must be configured
	if ctx.AI == nil {
		return nil, ErrNoAIHandler
	}

	output, err := ctx.AI.Call(input.Value)
	if err != nil {
		return nil, fmt.Errorf("E_AI_CALL_ERROR: %w", err)
	}

	return &eval.StringValue{Value: output}, nil
}

// aiCallJson implements AI.callJson(input: string, schema: string) -> string
func aiCallJson(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJson: expected 2 arguments, got %d", len(args))
	}

	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJson: expected string input, got %T", args[0])
	}

	schema, ok := args[1].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJson: expected string schema, got %T", args[1])
	}

	if ctx.AI == nil {
		return nil, ErrNoAIHandler
	}

	output, err := ctx.AI.CallJson(input.Value, schema.Value)
	if err != nil {
		return nil, fmt.Errorf("E_AI_CALL_ERROR: %w", err)
	}

	return &eval.StringValue{Value: output}, nil
}

// aiCallJsonSimple implements AI.callJsonSimple(input: string) -> string
func aiCallJsonSimple(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonSimple: expected 1 argument, got %d", len(args))
	}

	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callJsonSimple: expected string input, got %T", args[0])
	}

	if ctx.AI == nil {
		return nil, ErrNoAIHandler
	}

	output, err := ctx.AI.CallJson(input.Value, "")
	if err != nil {
		return nil, fmt.Errorf("E_AI_CALL_ERROR: %w", err)
	}

	return &eval.StringValue{Value: output}, nil
}

// aiCallImage implements AI.callImage(prompt: string, output_path: string, options: string) -> string
func aiCallImage(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImage: expected 3 arguments, got %d", len(args))
	}

	prompt, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImage: expected string prompt, got %T", args[0])
	}

	outputPath, ok := args[1].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImage: expected string output_path, got %T", args[1])
	}

	options, ok := args[2].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImage: expected string options, got %T", args[2])
	}

	if ctx.AI == nil {
		return nil, ErrNoAIHandler
	}

	result, err := ctx.AI.CallImage(prompt.Value, outputPath.Value, options.Value)
	if err != nil {
		return nil, fmt.Errorf("E_AI_CALL_ERROR: %w", err)
	}

	return &eval.StringValue{Value: result}, nil
}

// aiCallImageBase64 implements AI.callImageBase64(prompt: string, options: string) -> string
func aiCallImageBase64(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImageBase64: expected 2 arguments, got %d", len(args))
	}

	prompt, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImageBase64: expected string prompt, got %T", args[0])
	}

	options, ok := args[1].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callImageBase64: expected string options, got %T", args[1])
	}

	if ctx.AI == nil {
		return nil, ErrNoAIHandler
	}

	result, err := ctx.AI.CallImageBase64(prompt.Value, options.Value)
	if err != nil {
		return nil, fmt.Errorf("E_AI_CALL_ERROR: %w", err)
	}

	return &eval.StringValue{Value: result}, nil
}
