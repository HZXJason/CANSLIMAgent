package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

func setupAIAnalysisRouter(sidecarURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAIAnalysisHandlerWithSidecarURL(sidecarURL)
	router.GET("/api/v1/analysis/ai/config", handler.Config)
	router.POST("/api/v1/analysis/:symbol/ai", handler.Stream)
	return router
}

func TestAIAnalysisStreamProxiesSSE(t *testing.T) {
	var received services.AIAnalysisRequest
	var receivedBody []byte
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			t.Fatalf("unexpected sidecar path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(receivedBody, &received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Analysis-Run-ID", "run-nvda-test-001")
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
		strings.NewReader(`{"llm_provider":"openai_compatible","deep_think_llm":"deep-custom","quick_think_llm":"quick-custom","backend_url":"https://ai-gateway.vercel.sh/v1","max_debate_rounds":2,"max_risk_rounds":3,"online_tools":true,"analysts":["fundamental","canslim_second_layer"],"api_keys":{"openai_compatible":"sk-test"},"analysis_context":{"stock_summary":{"symbol":"NVDA"}},"run_id":"run-nvda-test-001","resume":true,"last_event_id":12}`),
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
	if recorder.Header().Get("X-Analysis-Run-ID") != "run-nvda-test-001" {
		t.Fatalf("expected run id response header, got %q", recorder.Header().Get("X-Analysis-Run-ID"))
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
	if received.DeepThinkLLM != "deep-custom" || received.QuickThinkLLM != "quick-custom" {
		t.Fatalf("expected frontend models to be forwarded: %+v", received)
	}
	if received.MaxDebateRounds != 2 || received.MaxRiskRounds != 3 || !received.OnlineTools {
		t.Fatalf("expected frontend analysis controls to be forwarded: %+v", received)
	}
	if received.RunID != "run-nvda-test-001" || !received.Resume || received.LastEventID != 12 {
		t.Fatalf("expected resume cursor to be forwarded: %+v", received)
	}
	if received.AnalysisContext["stock_summary"] == nil {
		t.Fatalf("expected analysis context to be forwarded")
	}
	if bytes := string(receivedBody); strings.Contains(bytes, "api_keys") || strings.Contains(bytes, "sk-test") {
		t.Fatalf("browser API key leaked to sidecar: %s", bytes)
	}
}

func TestAIAnalysisStreamAddsStructuredTradeJournal(t *testing.T) {
	var received services.AIAnalysisRequest
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	defer sidecar.Close()

	handler := NewAIAnalysisHandlerWithSidecarURL(sidecar.URL)
	handler.loadTradeJournal = func(_ context.Context, symbol string) ([]pythonTradeRecord, error) {
		if symbol != "NVDA" {
			t.Fatalf("unexpected journal symbol: %s", symbol)
		}
		return []pythonTradeRecord{{
			Datetime: "2026-07-09 10:30:00", Symbol: "NVDA", Name: "NVIDIA",
			Side: "buy", Quantity: 5, Price: 160, Amount: 800, Fee: 1, Market: "us",
		}}, nil
	}
	router := gin.New()
	router.POST("/api/v1/analysis/:symbol/ai", handler.Stream)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/NVDA/ai", strings.NewReader(`{"analysis_context":{"stock_summary":{"symbol":"NVDA"}}}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	journal, ok := received.AnalysisContext["trade_journal"].(map[string]interface{})
	if !ok || journal["schema_version"] != "vibe.trade_record.v1" {
		t.Fatalf("missing structured trade journal: %+v", received.AnalysisContext)
	}
	records, ok := journal["records"].([]interface{})
	if !ok || len(records) != 1 {
		t.Fatalf("unexpected journal records: %#v", journal["records"])
	}
}

func TestAIAnalysisConfigExposesStableContract(t *testing.T) {
	router := setupAIAnalysisRouter("http://127.0.0.1:1")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/ai/config", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), services.AIAnalysisContractVersion) {
		t.Fatalf("expected AI analysis contract version, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "/api/v1/analysis/{symbol}/ai") {
		t.Fatalf("expected stream endpoint contract, got %s", recorder.Body.String())
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
	if !strings.Contains(recorder.Body.String(), "上游服务") {
		t.Fatalf("expected upstream error message, got %s", recorder.Body.String())
	}
}
