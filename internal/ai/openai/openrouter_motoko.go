package openai

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
)

const openRouterPrefixMotoko = "openrouter/"
const openAIPrefixMotoko = "openai/"

// openRouterBaseURLMotoko is package-level for test override.
var openRouterBaseURLMotoko = "https://openrouter.ai/api/v1"

func isOpenRouterModelMotoko(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), openRouterPrefixMotoko)
}

func stripOpenRouterPrefixMotoko(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, openRouterPrefixMotoko) {
		return "", false
	}

	stripped := strings.TrimSpace(trimmed[len(openRouterPrefixMotoko):])
	if stripped == "" {
		return "", false
	}
	return stripped, true
}

func routeOpenRouterMotoko(c *Client, req *ai.Request) (*Client, *ai.Request) {
	if c == nil || req == nil {
		return c, req
	}

	model, ok := stripOpenRouterPrefixMotoko(req.Model)
	if !ok {
		return c, req
	}

	clientCopy := *c
	clientCopy.baseURL = openRouterBaseURLMotoko
	// Prefer OPENROUTER_API_KEY for OpenRouter-routed models.
	// Fallback to existing client key (typically OPENAI_API_KEY) when unset.
	if openRouterKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); openRouterKey != "" {
		clientCopy.apiKey = openRouterKey
	}

	reqCopy := *req
	reqCopy.Model = model

	return &clientCopy, &reqCopy
}

func stripOpenAIPrefixMotoko(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, openAIPrefixMotoko) {
		return "", false
	}
	stripped := strings.TrimSpace(trimmed[len(openAIPrefixMotoko):])
	if stripped == "" {
		return "", false
	}
	return stripped, true
}

func normalizeOpenAIBaseURLMotoko(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}
	if u.Host == "" {
		return "", fmt.Errorf("URL must include host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("URL must not contain query parameters or fragments")
	}

	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/v1"
	} else if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	u.Path = path
	return u.String(), nil
}

func routeOpenAIBaseURLFromEnvMotoko(c *Client) (*Client, error) {
	if c == nil {
		return c, nil
	}
	// Don't override explicit endpoint/openrouter routing.
	if c.baseURL != defaultBaseURL {
		return c, nil
	}

	raw := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if raw == "" {
		return c, nil
	}
	baseURL, err := normalizeOpenAIBaseURLMotoko(raw)
	if err != nil {
		return c, ai.NewProviderError("openai", 0, "invalid OPENAI_BASE_URL", err)
	}
	if baseURL == "" {
		return c, nil
	}
	clientCopy := *c
	clientCopy.baseURL = baseURL
	return &clientCopy, nil
}

func routeOpenAIPrefixMotoko(req *ai.Request) *ai.Request {
	if req == nil {
		return req
	}
	model, ok := stripOpenAIPrefixMotoko(req.Model)
	if !ok {
		return req
	}
	reqCopy := *req
	reqCopy.Model = model
	return &reqCopy
}

func generateOpenRouterMotoko(c *Client, ctx context.Context, req *ai.Request) (*ai.Response, error) {
	routedClient, routedReq := routeOpenRouterMotoko(c, req)
	apiType := routedClient.detectAPIType(routedReq.Model)

	switch apiType {
	case APIResponses:
		return routedClient.generateResponses(ctx, routedReq)
	default:
		return routedClient.generateChat(ctx, routedReq)
	}
}

func applyModelRoutingMotoko(c **Client, req **ai.Request) error {
	if c == nil || req == nil || *c == nil || *req == nil {
		return nil
	}

	// Endpoint routing first, then OpenRouter prefix routing.
	routedClient, routedReq, err := routeEndpointModelMotoko(*c, *req)
	if err != nil {
		return err
	}
	routedClient, routedReq = routeOpenRouterMotoko(routedClient, routedReq)
	if routedClient.baseURL != openRouterBaseURLMotoko {
		routedClient, err = routeOpenAIBaseURLFromEnvMotoko(routedClient)
		if err != nil {
			return err
		}
		routedReq = routeOpenAIPrefixMotoko(routedReq)
	}

	*c = routedClient
	*req = routedReq
	return nil
}
