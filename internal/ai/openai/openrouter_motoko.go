package openai

import (
	"context"
	"strings"

	"github.com/sunholo/ailang/internal/ai"
)

const openRouterPrefixMotoko = "openrouter/"

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

	reqCopy := *req
	reqCopy.Model = model

	return &clientCopy, &reqCopy
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

	*c = routedClient
	*req = routedReq
	return nil
}
