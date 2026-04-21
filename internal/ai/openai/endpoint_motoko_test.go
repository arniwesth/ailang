package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunholo/ailang/internal/ai"
)

func TestParseEndpointModelMotoko(t *testing.T) {
	target, ok, err := parseEndpointModelMotoko("openai://localhost:8000/google/gemma-4")
	require.True(t, ok)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8000/v1", target.baseURL)
	require.Equal(t, "google/gemma-4", target.model)

	target, ok, err = parseEndpointModelMotoko("openai://127.0.0.1:1234/v1/gpt-4o-mini")
	require.True(t, ok)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:1234/v1", target.baseURL)
	require.Equal(t, "gpt-4o-mini", target.model)
}

func TestParseEndpointModelMotoko_Invalid(t *testing.T) {
	_, ok, err := parseEndpointModelMotoko("openai://localhost:8000/")
	require.True(t, ok)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing model path")

	_, ok, err = parseEndpointModelMotoko("openai:///gpt-4o-mini")
	require.True(t, ok)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing host")
}

func TestRouteEndpointModelMotoko(t *testing.T) {
	client := NewClient("test-key")
	req := &ai.Request{Model: "openai://localhost:7000/google/gemma-4"}

	routedClient, routedReq, err := routeEndpointModelMotoko(client, req)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:7000/v1", routedClient.baseURL)
	require.Equal(t, "google/gemma-4", routedReq.Model)
	require.Equal(t, defaultBaseURL, client.baseURL)
	require.Equal(t, "openai://localhost:7000/google/gemma-4", req.Model)
}

func TestGenerate_LocalEndpointRouteMotoko(t *testing.T) {
	var gotPath string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		var reqBody chatRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		gotModel = reqBody.Model

		resp := chatResponse{
			Model: "gpt-4o-mini",
			Choices: []chatChoice{
				{Message: chatMessage{Content: "ok"}},
			},
			Usage: chatUsage{
				PromptTokens:     1,
				CompletionTokens: 1,
				TotalTokens:      2,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	client := NewClient("test-key")
	resp, err := client.Generate(context.Background(), &ai.Request{
		Model:      "openai://" + server.Listener.Addr().String() + "/gpt-4o-mini",
		UserPrompt: "hi",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
	require.Equal(t, "/v1/chat/completions", gotPath)
	require.Equal(t, "gpt-4o-mini", gotModel)
}

type timeoutErrMotoko struct{}

func (timeoutErrMotoko) Error() string   { return "timeout" }
func (timeoutErrMotoko) Timeout() bool   { return true }
func (timeoutErrMotoko) Temporary() bool { return true }

func TestClassifyOpenAIErrorMotoko(t *testing.T) {
	rateErr := ai.NewProviderError("openai", 429, "rate limited", nil)
	classified := classifyOpenAIErrorMotoko(rateErr)
	require.Equal(t, "rate_limited", classified.Kind)
	require.True(t, classified.Retryable)

	serverErr := ai.NewProviderError("openai", 503, "upstream unavailable", nil)
	classified = classifyOpenAIErrorMotoko(serverErr)
	require.Equal(t, "server_error", classified.Kind)
	require.True(t, classified.Retryable)

	timeoutWrapped := ai.NewProviderError("openai", 0, "request failed", timeoutErrMotoko{})
	classified = classifyOpenAIErrorMotoko(timeoutWrapped)
	require.Equal(t, "network_timeout", classified.Kind)
	require.True(t, classified.Retryable)

	unknown := classifyOpenAIErrorMotoko(errors.New("plain error"))
	require.Equal(t, "unknown", unknown.Kind)
	require.False(t, unknown.Retryable)
}
