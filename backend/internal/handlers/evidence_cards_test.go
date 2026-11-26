package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEvidenceCardsCompilesDeterministicallyWithoutSidecar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEvidenceCardsHandler(nil, nil, "", "")
	router := gin.New()
	router.POST("/api/v1/stocks/:symbol/evidence-cards", handler.Compile)

	resp := performJSON(router, http.MethodPost, "/api/v1/stocks/NVDA/evidence-cards", map[string]any{"include_ai": false})
	if resp.Code != http.StatusOK {
		t.Fatalf("evidence-cards = %d %s", resp.Code, resp.Body.String())
	}
	for _, needle := range []string{`"symbol":"NVDA"`, `"growth_inflection"`, `"price_volume_rs"`, `"audit_and_unknowns"`, `"ai_status":"not_requested"`} {
		if !bytes.Contains(resp.Body.Bytes(), []byte(needle)) {
			t.Fatalf("missing %s in %s", needle, resp.Body.String())
		}
	}
}

func TestEvidenceCardsForwardsToSidecarWhenAIRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evidence-cards" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"NVDA","cards":[],"sources":[],"ai_status":"completed","counter_thesis":{"bull_anchors":[{"title":"RS强","detail":"高位","source_ids":["summary-metrics"]}],"bear_falsifiers":[],"blind_spots":[]}}`))
	}))
	defer sidecar.Close()

	handler := NewEvidenceCardsHandler(nil, nil, sidecar.URL, "")
	router := gin.New()
	router.POST("/api/v1/stocks/:symbol/evidence-cards", handler.Compile)

	resp := performJSON(router, http.MethodPost, "/api/v1/stocks/NVDA/evidence-cards", map[string]any{
		"include_ai": true, "llm_provider": "openai", "model": "gpt-4o-mini",
	})
	if resp.Code != http.StatusOK || !bytes.Contains(resp.Body.Bytes(), []byte(`"ai_status":"completed"`)) {
		t.Fatalf("ai evidence-cards = %d %s", resp.Code, resp.Body.String())
	}
}
