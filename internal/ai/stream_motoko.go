package ai

import "context"

// StreamEventType identifies the kind of event emitted by a streaming provider.
type StreamEventType string

const (
	StreamEventDelta StreamEventType = "delta"
)

// StreamEvent is one incremental event from a streaming provider.
type StreamEvent struct {
	Type      StreamEventType
	Seq       int
	TextDelta string
}

// StreamHandler handles a streaming event.
type StreamHandler func(StreamEvent) error

// StreamingProvider is an optional provider capability for chunked output.
type StreamingProvider interface {
	GenerateStream(ctx context.Context, req *Request, onEvent StreamHandler) (*Response, error)
}

// CallStream streams model output when the underlying provider supports it.
// Providers without streaming support fall back to a single-chunk Call.
func (h *Handler) CallStream(input string, onEvent StreamHandler) (string, error) {
	if h == nil {
		return "", nil
	}

	if sp, ok := h.provider.(StreamingProvider); ok {
		resp, err := sp.GenerateStream(context.Background(), &Request{
			Model:        h.model,
			SystemPrompt: h.systemPrompt,
			UserPrompt:   input,
			MaxTokens:    h.maxTokens,
			Options:      h.requestOpts,
		}, onEvent)
		if err != nil {
			return "", err
		}
		if resp == nil {
			return "", nil
		}
		return resp.Text, nil
	}

	out, err := h.Call(input)
	if err != nil {
		return "", err
	}
	if onEvent != nil && out != "" {
		if err := onEvent(StreamEvent{Type: StreamEventDelta, Seq: 0, TextDelta: out}); err != nil {
			return "", err
		}
	}
	return out, nil
}
