package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDecisionSpineCandidateSnapshotRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.CandidateSnapshot{}, &models.ResearchCase{}, &models.UserThesis{}); err != nil {
		t.Fatal(err)
	}

	handler := NewDecisionSpineHandler(database)
	router := gin.New()
	router.POST("/api/v1/decision-spine/candidates", handler.CreateCandidateSnapshot)
	router.GET("/api/v1/decision-spine/candidates/:id", handler.GetCandidateSnapshot)

	create := httptest.NewRecorder()
	requestBody := `{
		"symbol":" nvda ",
		"market":"NASDAQ100",
		"screen_name":"CANSLIM",
		"inclusion_reasons":["RS 达标"],
		"exclusion_risks":["财报临近"],
		"context_snapshot":{"data_as_of":"2026-08-04"},
		"data_as_of":"2026-08-04T00:00:00Z"
	}`
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/decision-spine/candidates", strings.NewReader(requestBody)))
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}

	var created struct {
		Success bool `json:"success"`
		Data    struct {
			ID               string         `json:"id"`
			Symbol           string         `json:"symbol"`
			InclusionReasons []string       `json:"inclusion_reasons"`
			ContextSnapshot  map[string]any `json:"context_snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.Success || created.Data.ID == "" || created.Data.Symbol != "NVDA" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	if len(created.Data.InclusionReasons) != 1 || created.Data.InclusionReasons[0] != "RS 达标" {
		t.Fatalf("expected preserved inclusion reasons: %+v", created.Data.InclusionReasons)
	}

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/decision-spine/candidates/"+created.Data.ID, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var fetched struct {
		Data struct {
			ID              string         `json:"id"`
			Symbol          string         `json:"symbol"`
			ContextSnapshot map[string]any `json:"context_snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.Data.ID != created.Data.ID || fetched.Data.Symbol != "NVDA" {
		t.Fatalf("unexpected fetched response: %+v", fetched.Data)
	}
	if fetched.Data.ContextSnapshot["data_as_of"] != "2026-08-04" {
		t.Fatalf("expected immutable context snapshot, got %+v", fetched.Data.ContextSnapshot)
	}
}

func TestDecisionSpineResearchCaseRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.CandidateSnapshot{}, &models.ResearchCase{}, &models.UserThesis{}); err != nil {
		t.Fatal(err)
	}
	candidate := models.CandidateSnapshot{
		Symbol:          "NVDA",
		Market:          models.MarketNASDAQ100,
		ScreenName:      "CANSLIM",
		ContextSnapshot: map[string]any{"data_as_of": "2026-08-04"},
	}
	if err := database.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewDecisionSpineHandler(database)
	router := gin.New()
	router.POST("/api/v1/decision-spine/research-cases", handler.CreateResearchCase)
	router.GET("/api/v1/decision-spine/research-cases/:id", handler.GetResearchCase)

	create := httptest.NewRecorder()
	requestBody := `{
		"candidate_snapshot_id":"` + candidate.ID.String() + `",
		"symbol":"NVDA",
		"market":"NASDAQ100",
		"analysis_run_id":"run-research-001",
		"evidence_snapshot":{"status":"partial","unknown_fields":["institutional"]}
	}`
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/decision-spine/research-cases", strings.NewReader(requestBody)))
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			ID                  string         `json:"id"`
			CandidateSnapshotID string         `json:"candidate_snapshot_id"`
			Symbol              string         `json:"symbol"`
			EvidenceSnapshot    map[string]any `json:"evidence_snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.CandidateSnapshotID != candidate.ID.String() || created.Data.Symbol != "NVDA" {
		t.Fatalf("unexpected research response: %+v", created.Data)
	}

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/decision-spine/research-cases/"+created.Data.ID, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"unknown_fields":["institutional"]`) {
		t.Fatalf("expected preserved evidence snapshot, got %d: %s", get.Code, get.Body.String())
	}
}

func TestDecisionSpineRejectsResearchMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.CandidateSnapshot{}, &models.ResearchCase{}, &models.UserThesis{}); err != nil {
		t.Fatal(err)
	}
	candidate := models.CandidateSnapshot{
		Symbol:          "NVDA",
		Market:          models.MarketNASDAQ100,
		ScreenName:      "CANSLIM",
		ContextSnapshot: map[string]any{},
	}
	if err := database.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewDecisionSpineHandler(database)
	router := gin.New()
	router.POST("/api/v1/decision-spine/research-cases", handler.CreateResearchCase)

	response := httptest.NewRecorder()
	requestBody := `{"candidate_snapshot_id":"` + candidate.ID.String() + `","symbol":"AAPL","market":"NASDAQ100","evidence_snapshot":{}}`
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/decision-spine/research-cases", strings.NewReader(requestBody)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "does not match candidate snapshot") {
		t.Fatalf("expected mismatch explanation, got %s", response.Body.String())
	}
}

func TestDecisionSpineRevisesThesisWithoutOverwrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.CandidateSnapshot{}, &models.ResearchCase{}, &models.UserThesis{}); err != nil {
		t.Fatal(err)
	}
	candidate := models.CandidateSnapshot{Symbol: "NVDA", Market: models.MarketNASDAQ100, ScreenName: "CANSLIM", ContextSnapshot: map[string]any{}}
	if err := database.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}
	research := models.ResearchCase{CandidateSnapshotID: candidate.ID, Symbol: candidate.Symbol, Market: candidate.Market, EvidenceSnapshot: map[string]any{}}
	if err := database.Create(&research).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewDecisionSpineHandler(database)
	router := gin.New()
	router.POST("/api/v1/decision-spine/research-cases/:id/theses", handler.CreateUserThesis)
	router.POST("/api/v1/decision-spine/theses/:id/revise", handler.ReviseUserThesis)
	router.GET("/api/v1/decision-spine/research-cases/:id/theses", handler.ListUserTheses)

	create := httptest.NewRecorder()
	createBody := `{
		"stance":"watch",
		"summary":"等待价格确认",
		"accepted_arguments":["收入增长"],
		"rejected_arguments":[],
		"unresolved_questions":["财报催化是否定价"],
		"falsifiers":["跌破关键结构"]
	}`
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/decision-spine/research-cases/"+research.ID.String()+"/theses", strings.NewReader(createBody)))
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Summary string `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.Version != 1 {
		t.Fatalf("unexpected initial thesis: %+v", created.Data)
	}

	revise := httptest.NewRecorder()
	reviseBody := `{"stance":"long","summary":"催化确认后考虑计划","accepted_arguments":["收入增长","催化确认"],"rejected_arguments":[],"unresolved_questions":[],"falsifiers":["跌破关键结构"]}`
	router.ServeHTTP(revise, httptest.NewRequest(http.MethodPost, "/api/v1/decision-spine/theses/"+created.Data.ID+"/revise", strings.NewReader(reviseBody)))
	if revise.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", revise.Code, revise.Body.String())
	}
	var revised struct {
		Data struct {
			ID       string `json:"id"`
			ParentID string `json:"parent_id"`
			Version  int    `json:"version"`
			Summary  string `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(revise.Body.Bytes(), &revised); err != nil {
		t.Fatal(err)
	}
	if revised.Data.ID == created.Data.ID || revised.Data.ParentID != created.Data.ID || revised.Data.Version != 2 {
		t.Fatalf("expected append-only revision, got %+v", revised.Data)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/decision-spine/research-cases/"+research.ID.String()+"/theses", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Summary string `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 2 || listed.Data[0].Summary != "等待价格确认" || listed.Data[1].Summary != "催化确认后考虑计划" {
		t.Fatalf("expected both thesis versions, got %+v", listed.Data)
	}
}

func TestRegisterDecisionSpineRoutesExposesImmutableAndVersionedOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterDecisionSpineRoutes(router.Group("/api/v1"), NewDecisionSpineHandler(nil))

	want := map[string]bool{
		"POST /api/v1/decision-spine/candidates":                false,
		"GET /api/v1/decision-spine/candidates/:id":             false,
		"POST /api/v1/decision-spine/research-cases":            false,
		"GET /api/v1/decision-spine/research-cases/:id":         false,
		"POST /api/v1/decision-spine/research-cases/:id/theses": false,
		"GET /api/v1/decision-spine/research-cases/:id/theses":  false,
		"POST /api/v1/decision-spine/theses/:id/revise":         false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("expected registered route %s", route)
		}
	}
}
