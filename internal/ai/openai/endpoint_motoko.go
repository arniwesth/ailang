package openai

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
)

const (
	openAIEndpointPrefixMotoko = "openai://"
)

type endpointRouteMotoko struct {
	baseURL string
	model   string
}

// AIErrorMotoko is a typed OpenAI error surface used by Motoko-facing wrappers.
type AIErrorMotoko struct {
	Provider   string
	Kind       string
	Message    string
	StatusCode int
	Retryable  bool
}

func (e *AIErrorMotoko) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s/%s (%d): %s", e.Provider, e.Kind, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s/%s: %s", e.Provider, e.Kind, e.Message)
}

func parseEndpointModelMotoko(model string) (*endpointRouteMotoko, bool, error) {
	trimmed := strings.TrimSpace(model)
	if !strings.HasPrefix(strings.ToLower(trimmed), openAIEndpointPrefixMotoko) {
		return nil, false, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, true, ai.NewProviderError("openai", 0, "invalid openai:// model URI", err)
	}
	if u.Host == "" {
		return nil, true, ai.NewProviderError("openai", 0, "invalid openai:// model URI: missing host", nil)
	}

	path := strings.TrimSpace(strings.Trim(u.Path, "/"))
	if strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	if path == "" {
		return nil, true, ai.NewProviderError("openai", 0, "invalid openai:// model URI: missing model path", nil)
	}

	return &endpointRouteMotoko{
		baseURL: "http://" + u.Host + "/v1",
		model:   path,
	}, true, nil
}

func routeEndpointModelMotoko(c *Client, req *ai.Request) (*Client, *ai.Request, error) {
	if c == nil || req == nil {
		return c, req, nil
	}

	target, ok, err := parseEndpointModelMotoko(req.Model)
	if err != nil {
		return c, req, err
	}
	if !ok {
		return c, req, nil
	}

	clientCopy := *c
	clientCopy.baseURL = target.baseURL

	reqCopy := *req
	reqCopy.Model = target.model

	return &clientCopy, &reqCopy, nil
}

func classifyOpenAIErrorMotoko(err error) *AIErrorMotoko {
	if err == nil {
		return nil
	}

	if pe, ok := err.(*ai.ProviderError); ok {
		return &AIErrorMotoko{
			Provider:   pe.Provider,
			Kind:       classifyProviderKindMotoko(pe),
			Message:    pe.Message,
			StatusCode: pe.StatusCode,
			Retryable:  isRetryableProviderErrorMotoko(pe),
		}
	}

	return &AIErrorMotoko{
		Provider:  "openai",
		Kind:      "unknown",
		Message:   err.Error(),
		Retryable: false,
	}
}

func classifyProviderKindMotoko(pe *ai.ProviderError) string {
	if pe == nil {
		return "unknown"
	}
	switch {
	case pe.StatusCode == 429:
		return "rate_limited"
	case pe.StatusCode >= 500:
		return "server_error"
	case pe.StatusCode >= 400:
		return "client_error"
	case pe.Err != nil:
		if ne, ok := pe.Err.(net.Error); ok && ne.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	default:
		return "provider_error"
	}
}

func isRetryableProviderErrorMotoko(pe *ai.ProviderError) bool {
	if pe == nil {
		return false
	}
	switch pe.StatusCode {
	case 408, 409, 425, 429, 500, 502, 503, 504:
		return true
	}

	if pe.Err != nil {
		if ne, ok := pe.Err.(net.Error); ok && (ne.Timeout() || ne.Temporary()) {
			return true
		}
	}
	return false
}
