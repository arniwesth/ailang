package effects

import (
	"fmt"
	"io"
	"strings"

	"github.com/sunholo/ailang/internal/eval"
)

// init registers IO effect operations
func init() {
	RegisterOp("IO", "print", ioPrint)
	RegisterOp("IO", "println", ioPrintln)
	RegisterOp("IO", "readLine", ioReadLine)
	RegisterOp("IO", "pollStdin", ioPollStdin)
	RegisterOp("IO", "writeBytes", ioWriteBytes)
	RegisterOp("IO", "exit", ioExit)
}

// ioPrint implements IO.print(s: String) -> ()
//
// Prints a string to stdout without a trailing newline.
//
// Parameters:
//   - ctx: Effect context (capability check already done by Call())
//   - args: [StringValue] - the string to print
//
// Returns:
//   - UnitValue on success
//   - Error if wrong number/type of arguments
//
// Example AILANG code:
//
//	print("Hello")  -- prints "Hello" without newline
func ioPrint(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("print: expected 1 argument, got %d", len(args))
	}

	str, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("print: expected String, got %T", args[0])
	}

	fmt.Fprint(ctx.GetIOWriter(), str.Value)
	return &eval.UnitValue{}, nil
}

// ioPrintln implements IO.println(s: String) -> ()
//
// Prints a string to stdout with a trailing newline.
//
// Parameters:
//   - ctx: Effect context
//   - args: [StringValue] - the string to print
//
// Returns:
//   - UnitValue on success
//   - Error if wrong number/type of arguments
//
// Example AILANG code:
//
//	println("Hello")  -- prints "Hello\n"
func ioPrintln(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("println: expected 1 argument, got %d", len(args))
	}

	str, ok := args[0].(*eval.StringValue)
	if !ok {
		return nil, fmt.Errorf("println: expected String, got %T", args[0])
	}

	fmt.Fprintln(ctx.GetIOWriter(), str.Value)
	return &eval.UnitValue{}, nil
}

// ioReadLine implements IO.readLine() -> String
//
// Reads a line from stdin, blocking until a newline is encountered.
// The trailing newline (and carriage return on Windows) are removed.
//
// Parameters:
//   - ctx: Effect context
//   - args: [] - no arguments
//
// Returns:
//   - StringValue with the line read (without newline)
//   - Empty string on EOF
//   - Error if wrong number of arguments or read fails
//
// Example AILANG code:
//
//	let name = readLine()  -- blocks until user presses Enter
func ioReadLine(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("readLine: expected 0 arguments, got %d", len(args))
	}

	reader := ctx.GetIOReader()
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			// Return whatever was read before EOF (may be partial last line)
			line = strings.TrimSuffix(line, "\r")
			return &eval.StringValue{Value: line}, nil
		}
		return nil, fmt.Errorf("readLine: %w", err)
	}

	// Trim trailing newline
	line = strings.TrimSuffix(line, "\n")
	// Also trim \r on Windows
	line = strings.TrimSuffix(line, "\r")

	return &eval.StringValue{Value: line}, nil
}

// ioPollStdin implements IO.pollStdin() -> String
//
// Non-blocking stdin peek. If a complete line (terminated by \n) is buffered,
// returns it without the trailing newline. Otherwise returns "" immediately.
//
// This enables the SWE agent brain to check for abort/model_change commands
// from the TypeScript parent without blocking the recursive rpc_loop.
//
// Parameters:
//   - ctx: Effect context (capability check already done by Call())
//   - args: [] - no arguments
//
// Returns:
//   - StringValue with the pending line (without \n), or "" if nothing buffered
//   - Error if wrong number of arguments
//
// Implementation notes:
//   We peek at the bufio.Reader buffer without blocking. Only data already
//   buffered by Go's reader (i.e. previously read from the fd) is visible.
//   This is sufficient because the TypeScript parent writes complete JSONL
//   commands terminated by \n, and the OS pipe buffer delivers them promptly.
func ioPollStdin(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("pollStdin: expected 0 arguments, got %d", len(args))
	}

	reader := ctx.GetIOReader()
	if reader == nil {
		return &eval.StringValue{Value: ""}, nil
	}

	// bufio.Reader.Buffered() returns the number of bytes in the read buffer.
	if reader.Buffered() == 0 {
		return &eval.StringValue{Value: ""}, nil
	}

	// Peek without consuming. Check for a complete line (\n) in the buffer.
	peek, _ := reader.Peek(reader.Buffered())
	idx := -1
	for i, b := range peek {
		if b == '\n' {
			idx = i
			break
		}
	}
	if idx < 0 {
		// No complete line buffered yet
		return &eval.StringValue{Value: ""}, nil
	}

	// A complete line is available — read it (ReadString consumes from buffer).
	line, err := reader.ReadString('\n')
	if err != nil {
		// Unexpected error during read of data we already peeked; return "".
		return &eval.StringValue{Value: ""}, nil
	}

	// Trim trailing newline (and \r for safety)
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")

	return &eval.StringValue{Value: line}, nil
}

// ioExit implements IO.exit(code: Int) -> ()
//
// Terminates the AILANG process with the given exit code.
// Uses a sentinel panic (EvalExitCode) that propagates up to the runtime,
// which catches it, flushes telemetry, and calls os.Exit(code).
//
// Parameters:
//   - ctx: Effect context (capability check already done by Call())
//   - args: [IntValue] - the exit code (0-255 typical, OS takes code & 0xFF)
//
// Returns:
//   - Never returns — panics with EvalExitCode sentinel
func ioExit(_ *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("exit: expected 1 argument, got %d", len(args))
	}

	intVal, ok := args[0].(*eval.IntValue)
	if !ok {
		return nil, fmt.Errorf("exit: expected Int, got %T", args[0])
	}

	panic(&eval.EvalExitCode{Code: intVal.Value})
}

// ioWriteBytes implements IO.writeBytes(data: Bytes) -> ()
//
// Writes raw bytes to stdout. Unlike print/println which take strings,
// writeBytes writes binary data directly — enabling piping to external
// tools (e.g., `ailang run ... | afplay -f LEI16 -r 24000 -c 1 -`).
//
// Parameters:
//   - ctx: Effect context
//   - args: [BytesValue] - the bytes to write
//
// Returns:
//   - UnitValue on success
//   - Error if wrong number/type of arguments or write fails
//
// Example AILANG code:
//
//	writeBytes(pcmData)  -- writes raw PCM audio to stdout
func ioWriteBytes(ctx *EffContext, args []eval.Value) (eval.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("writeBytes: expected 1 argument, got %d", len(args))
	}

	bytesVal, ok := args[0].(*eval.BytesValue)
	if !ok {
		return nil, fmt.Errorf("writeBytes: expected Bytes, got %T", args[0])
	}

	if _, err := ctx.GetIOWriter().Write(bytesVal.Value); err != nil {
		return nil, fmt.Errorf("writeBytes: %w", err)
	}

	return &eval.UnitValue{}, nil
}
