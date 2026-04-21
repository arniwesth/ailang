package builtins

import (
	"fmt"

	"github.com/sunholo/ailang/internal/effects"
	"github.com/sunholo/ailang/internal/eval"
	"github.com/sunholo/ailang/internal/types"
)

func registerIOMotokoBuiltins() {
	impl := func(ctx *effects.EffContext, args []eval.Value) (eval.Value, error) {
		if len(args) != 1 {
			panic("internal invariant violation: _io_poll_stdin expects exactly 1 argument (unit)")
		}
		if _, ok := args[0].(*eval.UnitValue); !ok {
			panic("internal invariant violation: _io_poll_stdin expected unit argument")
		}
		return effects.Call(ctx, "IO", "pollStdin", nil)
	}

	typeFn := func() types.Type {
		T := types.NewBuilder()
		return T.Func(T.Unit()).Returns(T.String()).Effects("IO")
	}

	err := RegisterEffectBuiltin(BuiltinSpec{
		Module:  "std/io_motoko",
		Name:    "_io_poll_stdin",
		NumArgs: 1,
		IsPure:  false,
		Effect:  "IO",
		Type:    typeFn,
		Impl:    impl,
		Metadata: &BuiltinMetadata{
			Description: "Non-blocking stdin poll; returns one buffered line or empty string",
			LongDesc: "Checks currently buffered stdin data without blocking. Returns one line " +
				"when a newline is already buffered; otherwise returns empty string.",
			Returns:   "Buffered line without newline, or empty string",
			Since:     "v0.13.0-motoko",
			Stability: StabilityExperimental,
			Tags:      []string{"io", "stdin", "non-blocking", "motoko"},
			Category:  "io",
		},
	})
	if err != nil {
		panic(fmt.Sprintf("failed to register _io_poll_stdin: %v", err))
	}
}
