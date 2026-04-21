package effects

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/sunholo/ailang/internal/eval"
)

func registerIOMotokoOps() {
	RegisterOp("IO", "pollStdin", ioPollStdinMotoko)
}

// ioPollStdinMotoko implements IO.pollStdin() -> String.
//
// Behavior:
// - Non-blocking: only examines currently buffered bytes.
// - Returns "" when no full newline-terminated line is buffered.
// - Consumes exactly one line when newline is present.
func ioPollStdinMotoko(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("pollStdin: expected 0 arguments, got %d", len(args))
	}

	reader := ctx.GetIOReader()
	buffered := reader.Buffered()
	if buffered == 0 {
		return &eval.StringValue{Value: ""}, nil
	}

	peeked, err := reader.Peek(buffered)
	if err != nil {
		if err == io.EOF {
			return &eval.StringValue{Value: ""}, nil
		}
		return nil, fmt.Errorf("pollStdin: %w", err)
	}

	newlineIdx := bytes.IndexByte(peeked, '\n')
	if newlineIdx < 0 {
		return &eval.StringValue{Value: ""}, nil
	}

	lineBuf := make([]byte, newlineIdx+1)
	if _, err := io.ReadFull(reader, lineBuf); err != nil {
		return nil, fmt.Errorf("pollStdin: failed to consume buffered line: %w", err)
	}

	line := strings.TrimSuffix(string(lineBuf), "\n")
	line = strings.TrimSuffix(line, "\r")
	return &eval.StringValue{Value: line}, nil
}
