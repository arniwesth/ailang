package builtins

import (
	"errors"
	"strings"
	"testing"

	"github.com/sunholo/ailang/internal/ai"
	"github.com/sunholo/ailang/internal/effects"
	"github.com/sunholo/ailang/internal/eval"
	"github.com/stretchr/testify/require"
)

type mockAIHandlerForBuiltinsMotoko struct {
	streamChunks  []string
	streamCallCnt int
}

func (m *mockAIHandlerForBuiltinsMotoko) Call(input string) (string, error) {
	return "", nil
}

func (m *mockAIHandlerForBuiltinsMotoko) CallJson(input string, schema string) (string, error) {
	return "", nil
}

func (m *mockAIHandlerForBuiltinsMotoko) CallImage(prompt string, outputPath string, options string) (string, error) {
	return "", errors.New("not used")
}

func (m *mockAIHandlerForBuiltinsMotoko) CallImageBase64(prompt string, options string) (string, error) {
	return "", errors.New("not used")
}

func (m *mockAIHandlerForBuiltinsMotoko) CallStream(input string, onEvent ai.StreamHandler) (string, error) {
	m.streamCallCnt++
	var out strings.Builder
	for i, chunk := range m.streamChunks {
		if onEvent != nil {
			if err := onEvent(ai.StreamEvent{
				Type:      ai.StreamEventDelta,
				Seq:       i,
				TextDelta: chunk,
			}); err != nil {
				return out.String(), err
			}
		}
		out.WriteString(chunk)
	}
	return out.String(), nil
}

func intPtrMotoko(v int) *int { return &v }

func TestAICallStreamResultImplMotoko_SingleBudgetUnit(t *testing.T) {
	ctx := effects.NewEffContext(nil)
	ctx.Grant(effects.NewCapability("AI"))
	ctx.SetBudget(effects.NewBudgetContext(map[string]*int{
		"AI": intPtrMotoko(1),
	}))

	mock := &mockAIHandlerForBuiltinsMotoko{streamChunks: []string{"a", "b"}}
	ctx.AI = effects.NewAIContext(mock)

	args := []eval.Value{
		&eval.StringValue{Value: "in"},
		&eval.IntValue{Value: 1},
		&eval.StringValue{Value: "s1"},
		&eval.StringValue{Value: "openai/gpt-4o-mini"},
	}
	got, err := aiCallStreamResultImplMotoko(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 1, ctx.Budget.Used("AI"))
	require.Equal(t, 1, mock.streamCallCnt)
}
