package services

import "testing"

func TestResolveJournalReflectionURL(t *testing.T) {
	for input, expected := range map[string]string{
		"http://127.0.0.1:8765":         "http://127.0.0.1:8765/journal/reflect",
		"http://127.0.0.1:8765/":        "http://127.0.0.1:8765/journal/reflect",
		"http://127.0.0.1:8765/analyze": "http://127.0.0.1:8765/journal/reflect",
	} {
		if actual := resolveJournalReflectionURL(input); actual != expected {
			t.Fatalf("resolve %q: got %q, want %q", input, actual, expected)
		}
	}
}
