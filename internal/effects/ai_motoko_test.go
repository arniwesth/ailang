package effects

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sunholo/ailang/internal/ai"
	"github.com/sunholo/ailang/internal/eval"
	"github.com/sunholo/ailang/internal/trace"
	"github.com/stretchr/testify/require"
)

type mockAIHandlerMotoko struct {
	callResp      string
	callErr       error
	callJSONResp  string
	callJSONErr   error
	streamChunks  []string
	streamErrAt   int
	streamErr     error
	streamCallCnt int
}

func (m *mockAIHandlerMotoko) Call(input string) (string, error) {
	return m.callResp, m.callErr
}

func (m *mockAIHandlerMotoko) CallJson(input string, schema string) (string, error) {
	return m.callJSONResp, m.callJSONErr
}

func (m *mockAIHandlerMotoko) CallImage(prompt string, outputPath string, options string) (string, error) {
	return "", errors.New("not used")
}

func (m *mockAIHandlerMotoko) CallImageBase64(prompt string, options string) (string, error) {
	return "", errors.New("not used")
}

func (m *mockAIHandlerMotoko) CallStream(input string, onEvent ai.StreamHandler) (string, error) {
	m.streamCallCnt++
	var out strings.Builder
	for i, chunk := range m.streamChunks {
		if m.streamErr != nil && i == m.streamErrAt {
			return out.String(), m.streamErr
		}
		if onEvent != nil {
			if err := onEvent(ai.StreamEvent{Type: ai.StreamEventDelta, Seq: i, TextDelta: chunk}); err != nil {
				return out.String(), err
			}
		}
		out.WriteString(chunk)
	}
	return out.String(), nil
}

func TestAICallStreamResultMotoko_OrderedChunks(t *testing.T) {
	ctx := NewEffContext(nil)
	ctx.Grant(NewCapability("AI"))
	ctx.AI = NewAIContext(&mockAIHandlerMotoko{
		streamChunks: []string{"hel", "lo", "!"},
	})

	args := []eval.Value{
		&eval.StringValue{Value: "input"},
		&eval.IntValue{Value: 1},
		&eval.StringValue{Value: "s1"},
		&eval.StringValue{Value: "openai/gpt-4o-mini"},
	}
	got, err := aiCallStreamResultMotoko(ctx, args)
	require.NoError(t, err)

	rec := got.(*eval.RecordValue)
	require.True(t, rec.Fields["ok"].(*eval.BoolValue).Value)
	require.Equal(t, "hello!", rec.Fields["output"].(*eval.StringValue).Value)
	chunks := rec.Fields["chunks"].(*eval.ListValue).Elements
	require.Len(t, chunks, 3)
	for i, v := range chunks {
		c := v.(*eval.RecordValue)
		require.Equal(t, i, c.Fields["seq"].(*eval.IntValue).Value)
	}
}

func TestAICallStreamResultMotoko_ErrorMidStream(t *testing.T) {
	ctx := NewEffContext(nil)
	ctx.Grant(NewCapability("AI"))
	ctx.AI = NewAIContext(&mockAIHandlerMotoko{
		streamChunks: []string{"a", "b", "c", "d"},
		streamErrAt:  3,
		streamErr:    ai.NewProviderError("openai", 429, "rate limit", nil),
	})

	args := []eval.Value{
		&eval.StringValue{Value: "input"},
		&eval.IntValue{Value: 2},
		&eval.StringValue{Value: "s2"},
		&eval.StringValue{Value: "openai/gpt-4o-mini"},
	}
	got, err := aiCallStreamResultMotoko(ctx, args)
	require.NoError(t, err)

	rec := got.(*eval.RecordValue)
	require.False(t, rec.Fields["ok"].(*eval.BoolValue).Value)
	require.Equal(t, "openai", rec.Fields["provider"].(*eval.StringValue).Value)
	require.Equal(t, 429, rec.Fields["status_code"].(*eval.IntValue).Value)
	require.True(t, rec.Fields["retryable"].(*eval.BoolValue).Value)
	require.Equal(t, "HTTP_429", rec.Fields["error_code"].(*eval.StringValue).Value)
	require.Equal(t, "abc", rec.Fields["output"].(*eval.StringValue).Value)
}

func TestAICallStreamResultMotoko_Abort(t *testing.T) {
	ctx := NewEffContext(nil)
	ctx.Grant(NewCapability("AI"))
	ctx.AI = NewAIContext(&mockAIHandlerMotoko{
		streamChunks: []string{"x", "y"},
	})
	ctx.IOReader = strings.NewReader("{\"type\":\"abort\"}\n")
	_, _ = ctx.GetIOReader().Peek(20)

	args := []eval.Value{
		&eval.StringValue{Value: "input"},
		&eval.IntValue{Value: 3},
		&eval.StringValue{Value: "s3"},
		&eval.StringValue{Value: "openai/gpt-4o-mini"},
	}
	got, err := aiCallStreamResultMotoko(ctx, args)
	require.NoError(t, err)

	rec := got.(*eval.RecordValue)
	require.False(t, rec.Fields["ok"].(*eval.BoolValue).Value)
	require.Equal(t, 0, len(rec.Fields["chunks"].(*eval.ListValue).Elements))
}

func TestAICallStreamResultMotoko_TraceVisibility(t *testing.T) {
	ctx := NewEffContext(nil)
	ctx.Grant(NewCapability("AI"))
	ctx.AI = NewAIContext(&mockAIHandlerMotoko{
		streamChunks: []string{"a", "b"},
	})
	ctx.Trace = trace.NewCollector()
	ctx.IOReader = strings.NewReader("")
	ctx.IOWriter = io.Discard

	args := []eval.Value{
		&eval.StringValue{Value: "input"},
		&eval.IntValue{Value: 4},
		&eval.StringValue{Value: "s4"},
		&eval.StringValue{Value: "openai/gpt-4o-mini"},
	}
	_, err := aiCallStreamResultMotoko(ctx, args)
	require.NoError(t, err)

	ops := map[string]int{}
	for _, ev := range ctx.Trace.Events() {
		if ev.Effect != nil && ev.Effect.EffectName == "AI" {
			ops[ev.Effect.OpName]++
		}
	}
	require.GreaterOrEqual(t, ops["stream_start"], 1)
	require.GreaterOrEqual(t, ops["stream_delta"], 2)
	require.GreaterOrEqual(t, ops["stream_end"], 1)
}

func TestAICallResultMotoko_TypedProviderError(t *testing.T) {
	ctx := NewEffContext(nil)
	ctx.Grant(NewCapability("AI"))
	ctx.AI = NewAIContext(&mockAIHandlerMotoko{
		callErr: ai.NewProviderError("openai", 500, "server exploded", nil),
	})
	got, err := aiCallResultMotoko(ctx, []eval.Value{&eval.StringValue{Value: "ping"}})
	require.NoError(t, err)
	rec := got.(*eval.RecordValue)
	require.False(t, rec.Fields["ok"].(*eval.BoolValue).Value)
	require.Equal(t, "openai", rec.Fields["provider"].(*eval.StringValue).Value)
	require.Equal(t, 500, rec.Fields["status_code"].(*eval.IntValue).Value)
	require.Equal(t, "HTTP_500", rec.Fields["error_code"].(*eval.StringValue).Value)
}
