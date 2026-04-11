package openai

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// readSSEData reads text/event-stream frames and calls onData for each
// aggregated "data:" payload block.
func readSSEData(body io.Reader, onData func(string) error) error {
	scanner := bufio.NewScanner(body)
	// Default scanner token is 64K; streaming chunks can exceed this.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var dataLines []string
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			return io.EOF
		}
		return onData(payload)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		// Some OpenAI-compatible servers terminate chunked streams abruptly.
		// Treat unexpected EOF as end-of-stream and flush buffered data.
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(strings.ToLower(err.Error()), "unexpected eof") {
			if ferr := flush(); ferr != nil && ferr != io.EOF {
				return ferr
			}
			return nil
		}
		return fmt.Errorf("failed reading stream: %w", err)
	}
	if err := flush(); err != nil && err != io.EOF {
		return err
	}
	return nil
}
