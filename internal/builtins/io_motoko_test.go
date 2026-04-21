package builtins

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunholo/ailang/internal/effects"
	"github.com/sunholo/ailang/internal/eval"
)

func TestIOPollStdinBufferedLine(t *testing.T) {
	spec, ok := GetSpec("_io_poll_stdin")
	require.True(t, ok, "expected _io_poll_stdin to be registered")

	reader := bufio.NewReader(strings.NewReader("hello\nworld\n"))
	_, err := reader.Peek(len("hello\n"))
	require.NoError(t, err)

	ctx := effects.NewEffContext([]string{"IO"})
	ctx.Grant(effects.NewCapability("IO"))
	ctx.IOReader = reader

	v, err := spec.Impl(ctx, []eval.Value{&eval.UnitValue{}})
	require.NoError(t, err)

	sv, ok := v.(*eval.StringValue)
	require.True(t, ok)
	require.Equal(t, "hello", sv.Value)
}

func TestIOPollStdinEmptyBuffer(t *testing.T) {
	spec, ok := GetSpec("_io_poll_stdin")
	require.True(t, ok, "expected _io_poll_stdin to be registered")

	ctx := effects.NewEffContext([]string{"IO"})
	ctx.Grant(effects.NewCapability("IO"))
	ctx.IOReader = bufio.NewReader(strings.NewReader(""))

	v, err := spec.Impl(ctx, []eval.Value{&eval.UnitValue{}})
	require.NoError(t, err)

	sv, ok := v.(*eval.StringValue)
	require.True(t, ok)
	require.Equal(t, "", sv.Value)
}

func TestIOPollStdinPartialLineReturnsEmpty(t *testing.T) {
	spec, ok := GetSpec("_io_poll_stdin")
	require.True(t, ok, "expected _io_poll_stdin to be registered")

	reader := bufio.NewReader(strings.NewReader("partial"))
	_, err := reader.Peek(len("partial"))
	require.NoError(t, err)
	require.Equal(t, len("partial"), reader.Buffered())

	ctx := effects.NewEffContext([]string{"IO"})
	ctx.Grant(effects.NewCapability("IO"))
	ctx.IOReader = reader

	v, err := spec.Impl(ctx, []eval.Value{&eval.UnitValue{}})
	require.NoError(t, err)

	sv, ok := v.(*eval.StringValue)
	require.True(t, ok)
	require.Equal(t, "", sv.Value)
	require.Equal(t, len("partial"), reader.Buffered(), "partial line should remain buffered")
}
