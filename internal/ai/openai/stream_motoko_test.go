package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/sunholo/ailang/internal/ai"
)

func TestReadSSEDataMotoko(t *testing.T) {
	input := strings.NewReader("data: {\"a\":1}\n\ndata: {\"b\":2}\n\ndata: [DONE]\n\n")
	var got []string

	err := readSSEDataMotoko(input, func(payload string) error {
		got = append(got, payload)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"{\"a\":1}", "{\"b\":2}"}, got)
}

func TestReadSSEDataMotoko_OnDataError(t *testing.T) {
	input := strings.NewReader("data: {\"a\":1}\n\n")
	wantErr := errors.New("boom")
	err := readSSEDataMotoko(input, func(string) error { return wantErr })
	require.ErrorIs(t, err, wantErr)
}

func TestGenerateStreamMotoko_ChatSSE(t *testing.T) {
	var gotTemplateKwargs map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		var reqBody chatStreamRequestMotoko
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		gotTemplateKwargs = reqBody.ChatTemplateKwargs
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"model\":\"gpt-4o-mini\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	req := &ai.Request{
		Model:      "gpt-4o-mini",
		UserPrompt: "Say hello",
		Options: map[string]any{
			"chat_template_kwargs": map[string]any{"enable_thinking": false},
		},
	}

	var deltas []ai.StreamEvent
	resp, err := client.GenerateStream(context.Background(), req, func(ev ai.StreamEvent) error {
		deltas = append(deltas, ev)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Text)
	require.Equal(t, "gpt-4o-mini", resp.Model)
	require.Len(t, deltas, 2)
	require.Equal(t, 0, deltas[0].Seq)
	require.Equal(t, "hel", deltas[0].TextDelta)
	require.Equal(t, 1, deltas[1].Seq)
	require.Equal(t, "lo", deltas[1].TextDelta)
	require.Equal(t, false, gotTemplateKwargs["enable_thinking"])
}

func TestGenerateStreamMotoko_AbortFromHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"one\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"two\"}}]}\n\n")
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	errAbort := errors.New("abort")
	_, err := client.GenerateStream(context.Background(), &ai.Request{Model: "gpt-4o-mini", UserPrompt: "x"}, func(ev ai.StreamEvent) error {
		if ev.Seq == 0 {
			return errAbort
		}
		return nil
	})
	require.ErrorIs(t, err, errAbort)
}

func TestGenerateStreamMotoko_FinishReasonWithoutDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.WriteString(w, "data: {\"model\":\"gpt-4o-mini\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
			f.Flush()
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n")
			f.Flush()
			time.Sleep(2 * time.Second)
			return
		}
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	start := time.Now()
	resp, err := client.GenerateStream(context.Background(), &ai.Request{Model: "gpt-4o-mini", UserPrompt: "x"}, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, "hi", resp.Text)
	require.Less(t, elapsed, 1500*time.Millisecond)
}

func TestGenerateStreamMotoko_ReasoningContentDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"model\":\"gpt-4o-mini\",\"choices\":[{\"delta\":{\"reasoning_content\":\"<thinking>\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"step-by-step\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"</thinking>final\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	resp, err := client.GenerateStream(context.Background(), &ai.Request{Model: "gpt-4o-mini", UserPrompt: "x"}, nil)
	require.NoError(t, err)
	require.Equal(t, "<thinking>step-by-step</thinking>final", resp.Text)
}
