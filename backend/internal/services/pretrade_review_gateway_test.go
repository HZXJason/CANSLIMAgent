package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPreTradeAIReviewerUsesFixedEndpointAndSecret(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/trade-plans/review" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("X-Sidecar-Token") != "sidecar-secret" {
			t.Fatalf("missing sidecar secret")
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"risk_warnings":[{"code":"event_risk","severity":"high","category":"event","message":"事件风险","field":"valid_until","evidence":{"source_ids":["news-1"]}}],
			"behavior_reminders":[],"unresolved_questions":[],"changes_since_research":[],
			"freshness":{"news":{"status":"fresh"}},
			"sources":[{"id":"news-1","label":"公告","url":"https://example.com/filing","status":"available"}]
		}`))
	}))
	defer server.Close()

	reviewer := NewHTTPPreTradeAIReviewer(server.URL, time.Second, "sidecar-secret")
	result, err := reviewer.Review(context.Background(), PreTradeAIReviewInput{
		Ticker: "NVDA", Provider: "openai", Model: "gpt-test", APIKey: "server-only-key",
		PlanSnapshot: map[string]any{"entry_price": 120}, CurrentEvidence: map[string]any{"sources": []any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if received["api_key"] != "server-only-key" || received["llm_provider"] != "openai" || received["model"] != "gpt-test" {
		t.Fatalf("unexpected wire payload: %+v", received)
	}
	if len(result.RiskWarnings) != 1 || result.RiskWarnings[0].Code != "event_risk" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestHTTPPreTradeAIReviewerReturnsBoundedUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"detail":"provider unavailable"}`))
	}))
	defer server.Close()

	reviewer := NewHTTPPreTradeAIReviewer(server.URL, time.Second, "")
	_, err := reviewer.Review(context.Background(), PreTradeAIReviewInput{Ticker: "NVDA", Provider: "openai", Model: "gpt-test", APIKey: "must-not-appear"})
	if err == nil || err.Error() != "provider unavailable" {
		t.Fatalf("expected bounded upstream detail without credentials, got %v", err)
	}
}
