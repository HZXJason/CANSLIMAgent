package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupAIAnalysisRouter(sidecarURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAIAnalysisHandlerWithSidecarURL(sidecarURL)
	router.POST("/api/v1/analysis/:symbol/ai", handler.Stream)
	return router
}

func TestAIAnalysisStreamProxiesSSE(t *testing.T) {
	var received AIAnalysisRequest
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			t.Fatalf("unexpected sidecar path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: progress\n"))
		_, _ = w.Write([]byte("data: {\"agent\":\"FundamentalsAnalyst\"}\n\n"))
		_, _ = w.Write([]byte("event: done\n"))
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	defer sidecar.Close()

	router := setupAIAnalysisRouter(sidecar.URL)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/analysis/nvda/ai",
		strings.NewReader(`{"llm_provider":"openai_compatible","backend_url":"https://ai-gateway.vercel.sh/v1","analysts":["fundamental","canslim_second_layer"],"api_keys":{"openai_compatible":"sk-test"},"analysis_context":{"stock_summary":{"symbol":"NVDA"}}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", recorder.Header().Get("Content-Type"))
	}
	if body := recorder.Body.String(); !strings.Contains(body, "event: progress") || !strings.Contains(body, "event: done") {
		t.Fatalf("unexpected proxied body: %s", body)
	}
	if received.Ticker != "NVDA" {
		t.Fatalf("expected path symbol to be forwarded as NVDA, got %q", received.Ticker)
	}
	if received.LLMProvider != "openai_compatible" {
		t.Fatalf("expected provider forwarded, got %q", received.LLMProvider)
	}
	if received.BackendURL != "https://ai-gateway.vercel.sh/v1" {
		t.Fatalf("expected backend URL forwarded, got %q", received.BackendURL)
	}
	if received.APIKeys["openai_compatible"] != "sk-test" {
		t.Fatalf("expected compatible api key to be forwarded")
	}
	if received.AnalysisContext["stock_summary"] == nil {
		t.Fatalf("expected analysis context to be forwarded")
	}
}

func TestAIAnalysisStreamReturns503WhenSidecarUnavailable(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	sidecarURL := sidecar.URL
	sidecar.Close()

	router := setupAIAnalysisRouter(sidecarURL)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/analysis/NVDA/ai",
		strings.NewReader(`{"llm_provider":"anthropic"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "sidecar") {
		t.Fatalf("expected sidecar error message, got %s", recorder.Body.String())
	}
}
