package eval_harness

import "testing"

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"http://localhost:8000", "http://localhost:8000/v1", false},
		{"http://localhost:8000/v1", "http://localhost:8000/v1", false},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1", false},
		{"localhost:8000", "", true},
	}

	for _, tt := range tests {
		got, err := normalizeOpenAIBaseURL(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeOpenAIBaseURL(%q): expected error, got nil", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeOpenAIBaseURL(%q): unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeOpenAIBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
