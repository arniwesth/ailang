package effects

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
	"github.com/sunholo/ailang/internal/eval"
)

func registerAIMotokoOps() {
	RegisterOp("AI", "callResult", aiCallResultMotoko)
	RegisterOp("AI", "callJsonResult", aiCallJsonResultMotoko)
	RegisterOp("AI", "callStreamResult", aiCallStreamResultMotoko)
}

type aiStreamCallerMotoko interface {
	CallStream(input string, onEvent ai.StreamHandler) (string, error)
}

type streamChunkMotoko struct {
	seq       int
	textDelta string
}

func aiCallResultMotoko(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callResult: expected 1 argument, got %d", len(args))
	}
	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callResult: expected string input, got %T", args[0])
	}
	if ctx.AI == nil {
		return makeAIResultRecordMotoko(false, "", ErrNoAIHandler), nil
	}
	output, err := ctx.AI.Call(input.Value)
	if err != nil {
		return makeAIResultRecordMotoko(false, "", err), nil
	}
	return makeAIResultRecordMotoko(true, output, nil), nil
}

func aiCallJsonResultMotoko(ctx *EffContext, args []eval.Value) (eval.Value, error) {
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
		return makeAIResultRecordMotoko(false, "", ErrNoAIHandler), nil
	}
	output, err := ctx.AI.CallJson(input.Value, schema.Value)
	if err != nil {
		return makeAIResultRecordMotoko(false, "", err), nil
	}
	return makeAIResultRecordMotoko(true, output, nil), nil
}

// aiCallStreamResultMotoko implements:
// AI.callStreamResult(input: string, step: int, stream_id: string, model: string)
func aiCallStreamResultMotoko(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callStreamResult: expected 4 arguments, got %d", len(args))
	}
	input, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callStreamResult: expected string input, got %T", args[0])
	}
	step, ok := args[1].(*eval.IntValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callStreamResult: expected int step, got %T", args[1])
	}
	streamID, ok := args[2].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callStreamResult: expected string stream_id, got %T", args[2])
	}
	model, ok := args[3].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("E_AI_TYPE_ERROR: callStreamResult: expected string model, got %T", args[3])
	}
	if ctx.AI == nil {
		return makeAIStreamResultRecordMotoko(false, "", ErrNoAIHandler, nil, false, false), nil
	}
	if ctx.AI.handler == nil {
		return makeAIStreamResultRecordMotoko(false, "", ErrNoAIHandler, nil, false, false), nil
	}

	streamer, ok := ctx.AI.handler.(aiStreamCallerMotoko)
	if !ok {
		output, err := ctx.AI.Call(input.Value)
		if err != nil {
			return makeAIStreamResultRecordMotoko(false, "", err, nil, false, false), nil
		}
		if output == "" {
			return makeAIStreamResultRecordMotoko(true, "", nil, nil, false, false), nil
		}
		chunks := []streamChunkMotoko{{seq: 0, textDelta: output}}
		return makeAIStreamResultRecordMotoko(true, output, nil, chunks, false, false), nil
	}

	maxChunks := parseEnvIntMotoko("AI_STREAM_MAX_CHUNKS", 4000)
	maxBytes := parseEnvIntMotoko("AI_STREAM_MAX_BYTES", 2*1024*1024)
	if maxChunks <= 0 {
		maxChunks = 4000
	}
	if maxBytes <= 0 {
		maxBytes = 2 * 1024 * 1024
	}

	chunks := make([]streamChunkMotoko, 0, 128)
	var outputBuilder strings.Builder
	seq := 0
	emittedBytes := 0
	truncated := false
	aborted := false

	emitMotokoStreamEvent(ctx, "thinking_stream_start", map[string]any{
		"step":      step.Value,
		"stream_id": streamID.Value,
		"model":     model.Value,
	})
	ctx.RecordEffect("AI", "stream_start", []string{fmt.Sprintf("step=%d", step.Value), "stream_id=" + streamID.Value, "model=" + model.Value}, "ok")

	output, err := streamer.CallStream(input.Value, func(ev ai.StreamEvent) error {
		if pollBufferedAbortMotoko(ctx) {
			aborted = true
			return errors.New("stream aborted by user")
		}
		if ev.Type != ai.StreamEventDelta || ev.TextDelta == "" {
			return nil
		}
		if len(chunks) >= maxChunks || emittedBytes+len(ev.TextDelta) > maxBytes {
			if !truncated {
				truncated = true
				ctx.RecordEffect("AI", "stream_truncated", []string{fmt.Sprintf("max_chunks=%d", maxChunks), fmt.Sprintf("max_bytes=%d", maxBytes)}, "true")
			}
			return nil
		}

		chunk := streamChunkMotoko{seq: seq, textDelta: ev.TextDelta}
		chunks = append(chunks, chunk)
		outputBuilder.WriteString(ev.TextDelta)
		emittedBytes += len(ev.TextDelta)

		emitMotokoStreamEvent(ctx, "thinking_delta", map[string]any{
			"step":       step.Value,
			"stream_id":  streamID.Value,
			"seq":        seq,
			"text_delta": ev.TextDelta,
		})
		ctx.RecordEffect("AI", "stream_delta", []string{fmt.Sprintf("seq=%d", seq), fmt.Sprintf("bytes=%d", len(ev.TextDelta))}, ev.TextDelta)
		seq++
		return nil
	})

	if err != nil {
		if aborted {
			emitMotokoStreamEvent(ctx, "thinking_stream_end", map[string]any{
				"step":      step.Value,
				"stream_id": streamID.Value,
				"status":    "aborted",
			})
			ctx.RecordEffect("AI", "stream_end", []string{"stream_id=" + streamID.Value, "status=aborted"}, "")
			return makeAIStreamResultRecordMotoko(false, outputBuilder.String(), err, chunks, true, truncated), nil
		}
		_, _, msg, _, retryable := classifyAIErrorMotoko(err)
		emitMotokoStreamEvent(ctx, "thinking_stream_error", map[string]any{
			"step":      step.Value,
			"stream_id": streamID.Value,
			"message":   msg,
			"retryable": retryable,
		})
		emitMotokoStreamEvent(ctx, "thinking_stream_end", map[string]any{
			"step":      step.Value,
			"stream_id": streamID.Value,
			"status":    "errored",
		})
		ctx.RecordEffect("AI", "stream_error", []string{"stream_id=" + streamID.Value}, msg)
		ctx.RecordEffect("AI", "stream_end", []string{"stream_id=" + streamID.Value, "status=errored"}, "")
		return makeAIStreamResultRecordMotoko(false, outputBuilder.String(), err, chunks, true, truncated), nil
	}

	finalOutput := output
	if finalOutput == "" {
		finalOutput = outputBuilder.String()
	}
	emitMotokoStreamEvent(ctx, "thinking_stream_end", map[string]any{
		"step":      step.Value,
		"stream_id": streamID.Value,
		"status":    "completed",
	})
	ctx.RecordEffect("AI", "stream_end", []string{"stream_id=" + streamID.Value, "status=completed"}, "")
	return makeAIStreamResultRecordMotoko(true, finalOutput, nil, chunks, true, truncated), nil
}

func parseEnvIntMotoko(name string, fallback int) int {
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

func shouldEmitMotokoStreamEvents() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MOTOKO_STREAM_EVENTS")))
	return v == "1" || v == "true" || v == "yes"
}

func emitMotokoStreamEvent(ctx *EffContext, typ string, fields map[string]any) {
	if !shouldEmitMotokoStreamEvents() {
		return
	}
	payload := map[string]any{"type": typ}
	for k, v := range fields {
		payload[k] = v
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(ctx.GetIOWriter(), string(b))
}

func pollBufferedAbortMotoko(ctx *EffContext) bool {
	if ctx == nil {
		return false
	}
	reader := ctx.GetIOReader()
	if reader == nil {
		return false
	}
	for {
		buffered := reader.Buffered()
		if buffered == 0 {
			return false
		}
		peeked, err := reader.Peek(buffered)
		if err != nil && err != io.EOF {
			return false
		}
		idx := bytes.IndexByte(peeked, '\n')
		if idx < 0 {
			return false
		}
		lineBuf := make([]byte, idx+1)
		if _, err := io.ReadFull(reader, lineBuf); err != nil {
			return false
		}
		line := strings.TrimSpace(strings.TrimSuffix(string(lineBuf), "\n"))
		line = strings.TrimSuffix(line, "\r")
		if isAbortLineMotoko(line) {
			return true
		}
	}
}

func isAbortLineMotoko(line string) bool {
	if strings.EqualFold(strings.TrimSpace(line), "abort") {
		return true
	}
	if line == "" {
		return false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return false
	}
	t, _ := raw["type"].(string)
	return strings.EqualFold(strings.TrimSpace(t), "abort")
}

func makeAIResultRecordMotoko(ok bool, output string, err error) *eval.RecordValue {
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
	provider, code, msg, status, retryable := classifyAIErrorMotoko(err)
	return &eval.RecordValue{Fields: map[string]eval.Value{
		"ok":            &eval.BoolValue{Value: false},
		"output":        &eval.StringValue{Value: ""},
		"error_message": &eval.StringValue{Value: msg},
		"provider":      &eval.StringValue{Value: provider},
		"status_code":   &eval.IntValue{Value: status},
		"retryable":     &eval.BoolValue{Value: retryable},
		"error_code":    &eval.StringValue{Value: code},
	}}
}

func makeAIStreamResultRecordMotoko(ok bool, output string, err error, chunks []streamChunkMotoko, streamed bool, truncated bool) *eval.RecordValue {
	if ok {
		return &eval.RecordValue{Fields: map[string]eval.Value{
			"ok":               &eval.BoolValue{Value: true},
			"output":           &eval.StringValue{Value: output},
			"error_message":    &eval.StringValue{Value: ""},
			"provider":         &eval.StringValue{Value: ""},
			"status_code":      &eval.IntValue{Value: 0},
			"retryable":        &eval.BoolValue{Value: false},
			"error_code":       &eval.StringValue{Value: ""},
			"chunks":           chunksToEvalMotoko(chunks),
			"streamed":         &eval.BoolValue{Value: streamed},
			"stream_truncated": &eval.BoolValue{Value: truncated},
		}}
	}
	provider, code, msg, status, retryable := classifyAIErrorMotoko(err)
	return &eval.RecordValue{Fields: map[string]eval.Value{
		"ok":               &eval.BoolValue{Value: false},
		"output":           &eval.StringValue{Value: output},
		"error_message":    &eval.StringValue{Value: msg},
		"provider":         &eval.StringValue{Value: provider},
		"status_code":      &eval.IntValue{Value: status},
		"retryable":        &eval.BoolValue{Value: retryable},
		"error_code":       &eval.StringValue{Value: code},
		"chunks":           chunksToEvalMotoko(chunks),
		"streamed":         &eval.BoolValue{Value: streamed},
		"stream_truncated": &eval.BoolValue{Value: truncated},
	}}
}

func chunksToEvalMotoko(chunks []streamChunkMotoko) *eval.ListValue {
	items := make([]eval.Value, 0, len(chunks))
	for _, chunk := range chunks {
		items = append(items, &eval.RecordValue{Fields: map[string]eval.Value{
			"seq":        &eval.IntValue{Value: chunk.seq},
			"text_delta": &eval.StringValue{Value: chunk.textDelta},
		}})
	}
	return &eval.ListValue{Elements: items}
}

func classifyAIErrorMotoko(err error) (provider, errorCode, message string, statusCode int, retryable bool) {
	var provErr *ai.ProviderError
	if errors.As(err, &provErr) {
		provider = provErr.Provider
		statusCode = provErr.StatusCode
		message = provErr.Error()
		retryable = isRetryableStatusMotoko(statusCode)
		if statusCode > 0 {
			errorCode = fmt.Sprintf("HTTP_%d", statusCode)
		} else {
			errorCode = "E_PROVIDER"
		}
		return
	}

	message = err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "eof") {
		retryable = true
		errorCode = "E_TRANSPORT"
	} else {
		retryable = false
		errorCode = "E_UNKNOWN"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		retryable = true
		errorCode = "E_TRANSPORT"
	}
	return
}

func isRetryableStatusMotoko(status int) bool {
	switch status {
	case 408, 409, 425, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
