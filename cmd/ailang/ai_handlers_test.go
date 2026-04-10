package main

import "testing"

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "host only appends v1", input: "http://localhost:8000", want: "http://localhost:8000/v1"},
		{name: "trailing slash appends v1", input: "http://localhost:8000/", want: "http://localhost:8000/v1"},
		{name: "keeps explicit v1", input: "http://localhost:8000/v1", want: "http://localhost:8000/v1"},
		{name: "nested path appends v1", input: "http://localhost:8000/openai", want: "http://localhost:8000/openai/v1"},
		{name: "rejects empty", input: "", wantErr: true},
		{name: "rejects unsupported scheme", input: "ftp://localhost:8000", wantErr: true},
		{name: "rejects query", input: "http://localhost:8000/v1?x=1", wantErr: true},
		{name: "rejects fragment", input: "http://localhost:8000/v1#frag", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOpenAIBaseURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeOpenAIBaseURL(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOpenAIBaseURL(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeOpenAIBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveOpenAIBaseURLFromEnv(t *testing.T) {
	t.Run("unset env returns default non-custom", func(t *testing.T) {
		t.Setenv("OPENAI_BASE_URL", "")
		got, custom, err := resolveOpenAIBaseURLFromEnv()
		if err != nil {
			t.Fatalf("resolveOpenAIBaseURLFromEnv() error = %v", err)
		}
		if got != defaultOpenAIBaseURL {
			t.Fatalf("baseURL = %q, want %q", got, defaultOpenAIBaseURL)
		}
		if custom {
			t.Fatalf("custom = true, want false")
		}
	})

	t.Run("explicit default is not custom", func(t *testing.T) {
		t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1/")
		got, custom, err := resolveOpenAIBaseURLFromEnv()
		if err != nil {
			t.Fatalf("resolveOpenAIBaseURLFromEnv() error = %v", err)
		}
		if got != defaultOpenAIBaseURL {
			t.Fatalf("baseURL = %q, want %q", got, defaultOpenAIBaseURL)
		}
		if custom {
			t.Fatalf("custom = true, want false")
		}
	})

	t.Run("custom endpoint is marked custom", func(t *testing.T) {
		t.Setenv("OPENAI_BASE_URL", "http://localhost:8000")
		got, custom, err := resolveOpenAIBaseURLFromEnv()
		if err != nil {
			t.Fatalf("resolveOpenAIBaseURLFromEnv() error = %v", err)
		}
		if got != "http://localhost:8000/v1" {
			t.Fatalf("baseURL = %q, want %q", got, "http://localhost:8000/v1")
		}
		if !custom {
			t.Fatalf("custom = false, want true")
		}
	})

	t.Run("invalid env returns error", func(t *testing.T) {
		t.Setenv("OPENAI_BASE_URL", "not-a-url")
		_, _, err := resolveOpenAIBaseURLFromEnv()
		if err == nil {
			t.Fatal("resolveOpenAIBaseURLFromEnv() expected error, got nil")
		}
	})
}
