package builtins

import (
	"fmt"

	"github.com/sunholo/ailang/internal/effects"
	"github.com/sunholo/ailang/internal/eval"
	"github.com/sunholo/ailang/internal/types"
)

func registerAIMotokoBuiltins() {
	registerAICallResultMotoko()
	registerAICallJsonResultMotoko()
	registerAICallStreamResultMotoko()
}

func aiResultRecordTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Record(
		types.Field("ok", T.Bool()),
		types.Field("output", T.String()),
		types.Field("error_message", T.String()),
		types.Field("provider", T.String()),
		types.Field("status_code", T.Int()),
		types.Field("retryable", T.Bool()),
		types.Field("error_code", T.String()),
	)
}

func aiStreamChunkTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Record(
		types.Field("seq", T.Int()),
		types.Field("text_delta", T.String()),
	)
}

func aiStreamResultRecordTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Record(
		types.Field("ok", T.Bool()),
		types.Field("output", T.String()),
		types.Field("error_message", T.String()),
		types.Field("provider", T.String()),
		types.Field("status_code", T.Int()),
		types.Field("retryable", T.Bool()),
		types.Field("error_code", T.String()),
		types.Field("chunks", T.List(aiStreamChunkTypeMotoko())),
		types.Field("streamed", T.Bool()),
		types.Field("stream_truncated", T.Bool()),
	)
}

func registerAICallResultMotoko() {
	err := RegisterEffectBuiltin(BuiltinSpec{
		Module:  "std/ai_motoko",
		Name:    "_ai_call_result",
		NumArgs: 1,
		Effect:  "AI",
		Type:    makeAICallResultTypeMotoko,
		Impl:    aiCallResultImplMotoko,
		Metadata: &BuiltinMetadata{
			Description: "Call AI and return a structured result record (no host error throw on provider failures)",
			Params: []ParamDoc{
				{Name: "input", Description: "Input string"},
			},
			Returns:   "{ok, output, error_message, provider, status_code, retryable, error_code}",
			Since:     "v0.13.0-motoko",
			Stability: StabilityExperimental,
			Tags:      []string{"ai", "result", "error-handling"},
			Category:  "ai",
		},
	})
	if err != nil {
		panic("failed to register _ai_call_result builtin (motoko): " + err.Error())
	}
}

func makeAICallResultTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Func(T.String()).Returns(aiResultRecordTypeMotoko()).Effects("AI")
}

func aiCallResultImplMotoko(ctx *effects.EffContext, args []eval.Value) (eval.Value, error) {
	if err := ctx.RequireCapWithBudget("AI", ""); err != nil {
		return nil, err
	}
	return effects.Call(ctx, "AI", "callResult", args)
}

func registerAICallJsonResultMotoko() {
	err := RegisterEffectBuiltin(BuiltinSpec{
		Module:  "std/ai_motoko",
		Name:    "_ai_call_json_result",
		NumArgs: 2,
		Effect:  "AI",
		Type:    makeAICallJsonResultTypeMotoko,
		Impl:    aiCallJsonResultImplMotoko,
		Metadata: &BuiltinMetadata{
			Description: "Call AI JSON mode and return structured result record",
			Params: []ParamDoc{
				{Name: "input", Description: "Input string"},
				{Name: "schema", Description: "JSON Schema string"},
			},
			Returns:   "{ok, output, error_message, provider, status_code, retryable, error_code}",
			Since:     "v0.13.0-motoko",
			Stability: StabilityExperimental,
			Tags:      []string{"ai", "json", "result", "error-handling"},
			Category:  "ai",
		},
	})
	if err != nil {
		panic("failed to register _ai_call_json_result builtin (motoko): " + err.Error())
	}
}

func makeAICallJsonResultTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Func(T.String(), T.String()).Returns(aiResultRecordTypeMotoko()).Effects("AI")
}

func aiCallJsonResultImplMotoko(ctx *effects.EffContext, args []eval.Value) (eval.Value, error) {
	if err := ctx.RequireCapWithBudget("AI", ""); err != nil {
		return nil, err
	}
	return effects.Call(ctx, "AI", "callJsonResult", args)
}

func registerAICallStreamResultMotoko() {
	err := RegisterEffectBuiltin(BuiltinSpec{
		Module:  "std/ai_motoko",
		Name:    "_ai_call_stream_result",
		NumArgs: 4,
		Effect:  "AI",
		Type:    makeAICallStreamResultTypeMotoko,
		Impl:    aiCallStreamResultImplMotoko,
		Metadata: &BuiltinMetadata{
			Description: "Call AI via streaming path and return chunks plus structured terminal result",
			Params: []ParamDoc{
				{Name: "input", Description: "Input string"},
				{Name: "step", Description: "Runtime step index"},
				{Name: "stream_id", Description: "Caller stream id"},
				{Name: "model", Description: "Model label"},
			},
			Returns:   "{ok, output, error_*, chunks, streamed, stream_truncated}",
			Since:     "v0.13.0-motoko",
			Stability: StabilityExperimental,
			Tags:      []string{"ai", "streaming", "result", "error-handling"},
			Category:  "ai",
		},
	})
	if err != nil {
		panic("failed to register _ai_call_stream_result builtin (motoko): " + err.Error())
	}
}

func makeAICallStreamResultTypeMotoko() types.Type {
	T := types.NewBuilder()
	return T.Func(T.String(), T.Int(), T.String(), T.String()).Returns(aiStreamResultRecordTypeMotoko()).Effects("AI")
}

func aiCallStreamResultImplMotoko(ctx *effects.EffContext, args []eval.Value) (eval.Value, error) {
	// Single charging site: charge budget/cap once here, then invoke registered op directly.
	if err := ctx.RequireCapWithBudget("AI", ""); err != nil {
		return nil, err
	}
	ops, ok := effects.Registry["AI"]
	if !ok {
		return nil, fmt.Errorf("unknown effect: AI")
	}
	op, ok := ops["callStreamResult"]
	if !ok {
		return nil, fmt.Errorf("unknown operation callStreamResult in effect AI")
	}
	return op(ctx, args)
}
