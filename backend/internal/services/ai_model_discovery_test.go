package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveAIModelsURL(t *testing.T) {
	tests := map[string]string{
		"https://lingsuan.top":                     "https://lingsuan.top/v1/models",
		"https://lingsuan.top/":                    "https://lingsuan.top/v1/models",
		"https://lingsuan.top/v1":                  "https://lingsuan.top/v1/models",
		"https://lingsuan.top/v1/chat/completions": "https://lingsuan.top/v1/models",
	}
	for input, expected := range tests {
		actual, err := resolveAIModelsURL("openai_compatible", input)
		if err != nil {
			t.Fatalf("resolve %q: %v", input, err)
		}
		if actual.String() != expected {
			t.Fatalf("resolve %q: got %q, want %q", input, actual.String(), expected)
		}
	}
}

func TestDiscoverAIModelsUsesBearerAndParsesIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("missing bearer auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b"},{"id":"model-a"},{"id":"model-a"}]}`))
	}))
	defer server.Close()

	service := NewAIModelDiscoveryService(2 * time.Second)
	result, err := service.Discover(context.Background(), AIModelDiscoveryRequest{
		Provider:   "openai_compatible",
		BackendURL: server.URL,
		APIKey:     "sk-test",
	})
	if err != nil {
		t.Fatalf("discover models: %v", err)
	}
	if len(result.Models) != 2 || result.Models[0] != "model-a" || result.Models[1] != "model-b" {
		t.Fatalf("unexpected models: %#v", result.Models)
	}
}

func TestExtractGoogleModelNames(t *testing.T) {
	models := extractAIModelIDs(map[string]any{
		"models": []any{
			map[string]any{"name": "models/gemini-2.5-pro"},
		},
	}, "google")
	if len(models) != 1 || models[0] != "gemini-2.5-pro" {
		t.Fatalf("unexpected models: %#v", models)
	}
}
