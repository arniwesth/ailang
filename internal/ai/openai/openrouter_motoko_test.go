package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunholo/ailang/internal/ai"
)

func TestStripOpenRouterPrefixMotoko(t *testing.T) {
	model, ok := stripOpenRouterPrefixMotoko("openrouter/gpt-4o-mini")
	require.True(t, ok)
	require.Equal(t, "gpt-4o-mini", model)

	_, ok = stripOpenRouterPrefixMotoko("openrouter/")
	require.False(t, ok)

	_, ok = stripOpenRouterPrefixMotoko("gpt-4o-mini")
	require.False(t, ok)
}

func TestRouteOpenRouterMotoko(t *testing.T) {
	client := NewClient("test-key")
	req := &ai.Request{
		Model:      "openrouter/gpt-4o-mini",
		UserPrompt: "hello",
	}

	routedClient, routedReq := routeOpenRouterMotoko(client, req)
	require.NotNil(t, routedClient)
	require.NotNil(t, routedReq)
	require.Equal(t, openRouterBaseURLMotoko, routedClient.baseURL)
	require.Equal(t, "gpt-4o-mini", routedReq.Model)

	// Ensure originals are unchanged.
	require.Equal(t, defaultBaseURL, client.baseURL)
	require.Equal(t, "openrouter/gpt-4o-mini", req.Model)
}

func TestGenerate_OpenRouterRouteMotoko(t *testing.T) {
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

	prev := openRouterBaseURLMotoko
	openRouterBaseURLMotoko = server.URL
	defer func() { openRouterBaseURLMotoko = prev }()

	client := NewClient("test-key")
	resp, err := client.Generate(context.Background(), &ai.Request{
		Model:      "openrouter/gpt-4o-mini",
		UserPrompt: "hi",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
	require.Equal(t, "/chat/completions", gotPath)
	require.Equal(t, "gpt-4o-mini", gotModel)
}

func TestGenerate_OpenRouterPassThroughMotoko(t *testing.T) {
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

	client := NewClient("test-key", WithBaseURL(server.URL))
	resp, err := client.Generate(context.Background(), &ai.Request{
		Model:      "gpt-4o-mini",
		UserPrompt: "hi",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
	require.Equal(t, "/chat/completions", gotPath)
	require.Equal(t, "gpt-4o-mini", gotModel)
}
