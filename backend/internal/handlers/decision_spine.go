package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DecisionSpineHandler struct {
	db *gorm.DB
}

type createCandidateSnapshotRequest struct {
	Symbol             string         `json:"symbol"`
	Market             string         `json:"market"`
	ScreenName         string         `json:"screen_name"`
	ScreenDefinitionID *string        `json:"screen_definition_id"`
	InclusionReasons   []string       `json:"inclusion_reasons"`
	ExclusionRisks     []string       `json:"exclusion_risks"`
	ContextSnapshot    map[string]any `json:"context_snapshot"`
	DataAsOf           *string        `json:"data_as_of"`
}

type candidateSnapshotResponse struct {
	ID                 string         `json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	Symbol             string         `json:"symbol"`
	Market             string         `json:"market"`
	ScreenName         string         `json:"screen_name"`
	ScreenDefinitionID *string        `json:"screen_definition_id"`
	InclusionReasons   []string       `json:"inclusion_reasons"`
	ExclusionRisks     []string       `json:"exclusion_risks"`
	ContextSnapshot    map[string]any `json:"context_snapshot"`
	DataAsOf           *time.Time     `json:"data_as_of"`
}

type createResearchCaseRequest struct {
	CandidateSnapshotID string         `json:"candidate_snapshot_id"`
	Symbol              string         `json:"symbol"`
	Market              string         `json:"market"`
	AnalysisRunID       *string        `json:"analysis_run_id"`
	EvidenceSnapshot    map[string]any `json:"evidence_snapshot"`
}

type researchCaseResponse struct {
	ID                  string         `json:"id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	CandidateSnapshotID string         `json:"candidate_snapshot_id"`
	Symbol              string         `json:"symbol"`
	Market              string         `json:"market"`
	AnalysisRunID       *string        `json:"analysis_run_id"`
	EvidenceSnapshot    map[string]any `json:"evidence_snapshot"`
}

type writeUserThesisRequest struct {
	Stance              string   `json:"stance"`
	Summary             string   `json:"summary"`
	AcceptedArguments   []string `json:"accepted_arguments"`
	RejectedArguments   []string `json:"rejected_arguments"`
	UnresolvedQuestions []string `json:"unresolved_questions"`
	Falsifiers          []string `json:"falsifiers"`
}

type userThesisResponse struct {
	ID                  string    `json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	ResearchCaseID      string    `json:"research_case_id"`
	ParentID            *string   `json:"parent_id"`
	Symbol              string    `json:"symbol"`
	Version             int       `json:"version"`
	Stance              string    `json:"stance"`
	Summary             string    `json:"summary"`
	AcceptedArguments   []string  `json:"accepted_arguments"`
	RejectedArguments   []string  `json:"rejected_arguments"`
	UnresolvedQuestions []string  `json:"unresolved_questions"`
	Falsifiers          []string  `json:"falsifiers"`
}

func NewDecisionSpineHandler(database *gorm.DB) *DecisionSpineHandler {
	return &DecisionSpineHandler{db: database}
}

func RegisterDecisionSpineRoutes(router gin.IRoutes, handler *DecisionSpineHandler) {
	router.POST("/decision-spine/candidates", handler.CreateCandidateSnapshot)
	router.GET("/decision-spine/candidates/:id", handler.GetCandidateSnapshot)
	router.POST("/decision-spine/research-cases", handler.CreateResearchCase)
	router.GET("/decision-spine/research-cases/:id", handler.GetResearchCase)
	router.POST("/decision-spine/research-cases/:id/theses", handler.CreateUserThesis)
	router.GET("/decision-spine/research-cases/:id/theses", handler.ListUserTheses)
	router.POST("/decision-spine/theses/:id/revise", handler.ReviseUserThesis)
}

func (h *DecisionSpineHandler) CreateCandidateSnapshot(c *gin.Context) {
	var request createCandidateSnapshotRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	snapshot, validationErr := request.toModel()
	if validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&snapshot).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "candidate snapshot could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newCandidateSnapshotResponse(snapshot), Error: nil})
}

func (h *DecisionSpineHandler) GetCandidateSnapshot(c *gin.Context) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "candidate snapshot id is invalid")
		return
	}
	var snapshot models.CandidateSnapshot
	err = h.db.WithContext(c.Request.Context()).First(&snapshot, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "candidate snapshot not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "candidate snapshot could not be loaded")
		return
	}
	OK(c, newCandidateSnapshotResponse(snapshot))
}

func (h *DecisionSpineHandler) CreateResearchCase(c *gin.Context) {
	var request createResearchCaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	candidateID, err := uuid.Parse(strings.TrimSpace(request.CandidateSnapshotID))
	if err != nil {
		Fail(c, http.StatusBadRequest, "candidate_snapshot_id is invalid")
		return
	}
	var candidate models.CandidateSnapshot
	err = h.db.WithContext(c.Request.Context()).First(&candidate, "id = ?", candidateID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusBadRequest, "candidate snapshot not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "candidate snapshot could not be loaded")
		return
	}

	symbol := strings.ToUpper(strings.TrimSpace(request.Symbol))
	market := strings.ToUpper(strings.TrimSpace(request.Market))
	if symbol != candidate.Symbol || market != candidate.Market {
		Fail(c, http.StatusBadRequest, "research identity does not match candidate snapshot")
		return
	}
	if request.EvidenceSnapshot == nil {
		request.EvidenceSnapshot = map[string]any{}
	}
	if request.AnalysisRunID != nil {
		trimmed := strings.TrimSpace(*request.AnalysisRunID)
		if trimmed == "" {
			request.AnalysisRunID = nil
		} else {
			request.AnalysisRunID = &trimmed
		}
	}

	research := models.ResearchCase{
		CandidateSnapshotID: candidate.ID,
		Symbol:              candidate.Symbol,
		Market:              candidate.Market,
		AnalysisRunID:       request.AnalysisRunID,
		EvidenceSnapshot:    request.EvidenceSnapshot,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&research).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "research case could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newResearchCaseResponse(research), Error: nil})
}

func (h *DecisionSpineHandler) GetResearchCase(c *gin.Context) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "research case id is invalid")
		return
	}
	var research models.ResearchCase
	err = h.db.WithContext(c.Request.Context()).First(&research, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "research case not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "research case could not be loaded")
		return
	}
	OK(c, newResearchCaseResponse(research))
}

func (h *DecisionSpineHandler) CreateUserThesis(c *gin.Context) {
	researchID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "research case id is invalid")
		return
	}
	var request writeUserThesisRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	if validationErr := request.validate(); validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}

	var research models.ResearchCase
	err = h.db.WithContext(c.Request.Context()).First(&research, "id = ?", researchID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "research case not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "research case could not be loaded")
		return
	}

	thesis := request.newModel(research, 1, nil)
	if err := h.db.WithContext(c.Request.Context()).Create(&thesis).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "user thesis could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newUserThesisResponse(thesis), Error: nil})
}

func (h *DecisionSpineHandler) ReviseUserThesis(c *gin.Context) {
	parentID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "user thesis id is invalid")
		return
	}
	var request writeUserThesisRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	if validationErr := request.validate(); validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}

	var revised models.UserThesis
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var parent models.UserThesis
		if err := tx.First(&parent, "id = ?", parentID).Error; err != nil {
			return err
		}
		var latestVersion int
		if err := tx.Model(&models.UserThesis{}).
			Where("research_case_id = ?", parent.ResearchCaseID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&latestVersion).Error; err != nil {
			return err
		}
		research := models.ResearchCase{ID: parent.ResearchCaseID, Symbol: parent.Symbol}
		revised = request.newModel(research, latestVersion+1, &parent.ID)
		return tx.Create(&revised).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "user thesis not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "user thesis could not be revised")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newUserThesisResponse(revised), Error: nil})
}

func (h *DecisionSpineHandler) ListUserTheses(c *gin.Context) {
	researchID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "research case id is invalid")
		return
	}
	var theses []models.UserThesis
	if err := h.db.WithContext(c.Request.Context()).
		Where("research_case_id = ?", researchID).
		Order("version ASC").
		Find(&theses).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "user theses could not be loaded")
		return
	}
	responses := make([]userThesisResponse, 0, len(theses))
	for _, thesis := range theses {
		responses = append(responses, newUserThesisResponse(thesis))
	}
	OK(c, responses)
}

func (request createCandidateSnapshotRequest) toModel() (models.CandidateSnapshot, string) {
	symbol := strings.ToUpper(strings.TrimSpace(request.Symbol))
	if symbol == "" {
		return models.CandidateSnapshot{}, "symbol is required"
	}
	market := strings.ToUpper(strings.TrimSpace(request.Market))
	if !models.ValidMarket(market) {
		return models.CandidateSnapshot{}, "market is not supported"
	}
	screenName := strings.TrimSpace(request.ScreenName)
	if screenName == "" {
		return models.CandidateSnapshot{}, "screen_name is required"
	}

	var screenDefinitionID *uuid.UUID
	if request.ScreenDefinitionID != nil && strings.TrimSpace(*request.ScreenDefinitionID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*request.ScreenDefinitionID))
		if err != nil {
			return models.CandidateSnapshot{}, "screen_definition_id is invalid"
		}
		screenDefinitionID = &parsed
	}
	var dataAsOf *time.Time
	if request.DataAsOf != nil && strings.TrimSpace(*request.DataAsOf) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*request.DataAsOf))
		if err != nil {
			return models.CandidateSnapshot{}, "data_as_of must be an RFC3339 timestamp"
		}
		parsed = parsed.UTC()
		dataAsOf = &parsed
	}
	if request.InclusionReasons == nil {
		request.InclusionReasons = []string{}
	}
	if request.ExclusionRisks == nil {
		request.ExclusionRisks = []string{}
	}
	if request.ContextSnapshot == nil {
		request.ContextSnapshot = map[string]any{}
	}

	return models.CandidateSnapshot{
		Symbol:             symbol,
		Market:             market,
		ScreenName:         screenName,
		ScreenDefinitionID: screenDefinitionID,
		InclusionReasons:   request.InclusionReasons,
		ExclusionRisks:     request.ExclusionRisks,
		ContextSnapshot:    request.ContextSnapshot,
		DataAsOf:           dataAsOf,
	}, ""
}

func newCandidateSnapshotResponse(snapshot models.CandidateSnapshot) candidateSnapshotResponse {
	var screenDefinitionID *string
	if snapshot.ScreenDefinitionID != nil {
		value := snapshot.ScreenDefinitionID.String()
		screenDefinitionID = &value
	}
	return candidateSnapshotResponse{
		ID:                 snapshot.ID.String(),
		CreatedAt:          snapshot.CreatedAt,
		Symbol:             snapshot.Symbol,
		Market:             snapshot.Market,
		ScreenName:         snapshot.ScreenName,
		ScreenDefinitionID: screenDefinitionID,
		InclusionReasons:   snapshot.InclusionReasons,
		ExclusionRisks:     snapshot.ExclusionRisks,
		ContextSnapshot:    snapshot.ContextSnapshot,
		DataAsOf:           snapshot.DataAsOf,
	}
}

func newResearchCaseResponse(research models.ResearchCase) researchCaseResponse {
	return researchCaseResponse{
		ID:                  research.ID.String(),
		CreatedAt:           research.CreatedAt,
		UpdatedAt:           research.UpdatedAt,
		CandidateSnapshotID: research.CandidateSnapshotID.String(),
		Symbol:              research.Symbol,
		Market:              research.Market,
		AnalysisRunID:       research.AnalysisRunID,
		EvidenceSnapshot:    research.EvidenceSnapshot,
	}
}

func (request writeUserThesisRequest) validate() string {
	request.Stance = strings.ToLower(strings.TrimSpace(request.Stance))
	switch request.Stance {
	case "long", "short", "watch", "pass", "insufficient":
	default:
		return "stance must be long, short, watch, pass, or insufficient"
	}
	if strings.TrimSpace(request.Summary) == "" {
		return "summary is required"
	}
	return ""
}

func (request writeUserThesisRequest) newModel(research models.ResearchCase, version int, parentID *uuid.UUID) models.UserThesis {
	return models.UserThesis{
		ResearchCaseID:      research.ID,
		ParentID:            parentID,
		Symbol:              research.Symbol,
		Version:             version,
		Stance:              strings.ToLower(strings.TrimSpace(request.Stance)),
		Summary:             strings.TrimSpace(request.Summary),
		AcceptedArguments:   cleanStringList(request.AcceptedArguments),
		RejectedArguments:   cleanStringList(request.RejectedArguments),
		UnresolvedQuestions: cleanStringList(request.UnresolvedQuestions),
		Falsifiers:          cleanStringList(request.Falsifiers),
	}
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func newUserThesisResponse(thesis models.UserThesis) userThesisResponse {
	var parentID *string
	if thesis.ParentID != nil {
		value := thesis.ParentID.String()
		parentID = &value
	}
	return userThesisResponse{
		ID:                  thesis.ID.String(),
		CreatedAt:           thesis.CreatedAt,
		ResearchCaseID:      thesis.ResearchCaseID.String(),
		ParentID:            parentID,
		Symbol:              thesis.Symbol,
		Version:             thesis.Version,
		Stance:              thesis.Stance,
		Summary:             thesis.Summary,
		AcceptedArguments:   thesis.AcceptedArguments,
		RejectedArguments:   thesis.RejectedArguments,
		UnresolvedQuestions: thesis.UnresolvedQuestions,
		Falsifiers:          thesis.Falsifiers,
	}
}
