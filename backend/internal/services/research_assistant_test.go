package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResearchAssistantGatewayForwardsSelectedModelAndSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assistant/ask" || r.Header.Get("X-Sidecar-Token") != "sidecar-secret" {
			t.Fatalf("unexpected request: %s secret=%q", r.URL.Path, r.Header.Get("X-Sidecar-Token"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-test" || body["api_key"] != "secret-key" {
			t.Fatalf("missing model or credential: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"CPI 通过利率预期影响估值。","model":"gpt-test"}`))
	}))
	defer server.Close()

	client := NewHTTPResearchAssistant(server.URL, time.Second, "sidecar-secret")
	result, err := client.Ask(context.Background(), ResearchAssistantInput{Question: "CPI 有什么影响？", Market: "US", Provider: "openai", Model: "gpt-test", APIKey: "secret-key"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "gpt-test" || result.Answer == "" {
		t.Fatalf("unexpected response: %+v", result)
	}
}
