package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type policyRequest struct {
	Kind          *string        `json:"kind"`
	Category      *string        `json:"category"`
	Title         *string        `json:"title"`
	Statement     *string        `json:"statement"`
	Condition     map[string]any `json:"condition"`
	Action        map[string]any `json:"action"`
	Scope         map[string]any `json:"scope"`
	Strength      *string        `json:"strength"`
	Origin        *string        `json:"origin"`
	Status        *string        `json:"status"`
	PromptEnabled *bool          `json:"prompt_enabled"`
}

func (h *ProfileHandler) ListPolicies(c *gin.Context) {
	var policies []models.TradingPolicy
	query := h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID)
	if kind := strings.ToLower(strings.TrimSpace(c.Query("kind"))); kind != "" {
		if !models.ValidPolicyKind(kind) {
			Fail(c, http.StatusBadRequest, "交易策略类型不正确")
			return
		}
		query = query.Where("kind = ?", kind)
	}
	if err := query.Order("status asc, created_at asc").Find(&policies).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "交易策略读取失败")
		return
	}
	OK(c, policies)
}

func (h *ProfileHandler) CreatePolicy(c *gin.Context) {
	var request policyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	policy, message := newTradingPolicy(request)
	if message != "" {
		Fail(c, http.StatusBadRequest, message)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&policy).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "交易策略保存失败")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: policy})
}

func (h *ProfileHandler) PatchPolicy(c *gin.Context) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "交易策略 ID 不正确")
		return
	}
	var policy models.TradingPolicy
	err = h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "交易策略不存在")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "交易策略读取失败")
		return
	}
	var request policyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	if message := applyPolicyPatch(&policy, request); message != "" {
		Fail(c, http.StatusBadRequest, message)
		return
	}
	policy.Version++
	if err := h.db.WithContext(c.Request.Context()).Save(&policy).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "交易策略更新失败")
		return
	}
	OK(c, policy)
}

func newTradingPolicy(request policyRequest) (models.TradingPolicy, string) {
	if request.Kind == nil || request.Category == nil || request.Title == nil || request.Statement == nil {
		return models.TradingPolicy{}, "类型、分类、标题和内容不能为空"
	}
	policy := models.TradingPolicy{
		UserID: models.DefaultUserID, Kind: normalized(request.Kind), Category: normalized(request.Category),
		Title: trimmed(request.Title), Statement: trimmed(request.Statement), Condition: request.Condition,
		Action: request.Action, Scope: request.Scope, Strength: models.PolicyStrengthReminder,
		Origin: models.PolicyOriginManual, Status: models.PolicyStatusDraft, PromptEnabled: false,
	}
	if request.Strength != nil {
		policy.Strength = normalized(request.Strength)
	}
	if request.Origin != nil {
		policy.Origin = normalized(request.Origin)
	}
	if request.Status != nil {
		policy.Status = normalized(request.Status)
	}
	if request.PromptEnabled != nil {
		policy.PromptEnabled = *request.PromptEnabled
	}
	if !models.ValidPolicyKind(policy.Kind) || policy.Category == "" || policy.Title == "" || policy.Statement == "" {
		return models.TradingPolicy{}, "交易策略基础信息不正确"
	}
	if !models.ValidPolicyStrength(policy.Strength) || !models.ValidPolicyOrigin(policy.Origin) || !models.ValidPolicyStatus(policy.Status) {
		return models.TradingPolicy{}, "交易策略状态、来源或强度不正确"
	}
	return policy, ""
}

func applyPolicyPatch(policy *models.TradingPolicy, request policyRequest) string {
	if request.Kind != nil {
		value := normalized(request.Kind)
		if !models.ValidPolicyKind(value) {
			return "交易策略类型不正确"
		}
		policy.Kind = value
	}
	if request.Category != nil {
		policy.Category = normalized(request.Category)
	}
	if request.Title != nil {
		policy.Title = trimmed(request.Title)
	}
	if request.Statement != nil {
		policy.Statement = trimmed(request.Statement)
	}
	if policy.Category == "" || policy.Title == "" || policy.Statement == "" {
		return "交易策略分类、标题和内容不能为空"
	}
	if request.Strength != nil {
		value := normalized(request.Strength)
		if !models.ValidPolicyStrength(value) {
			return "交易策略强度不正确"
		}
		policy.Strength = value
	}
	if request.Status != nil {
		value := normalized(request.Status)
		if !models.ValidPolicyStatus(value) {
			return "交易策略状态不正确"
		}
		policy.Status = value
	}
	if request.PromptEnabled != nil {
		policy.PromptEnabled = *request.PromptEnabled
	}
	if request.Condition != nil {
		policy.Condition = request.Condition
	}
	if request.Action != nil {
		policy.Action = request.Action
	}
	if request.Scope != nil {
		policy.Scope = request.Scope
	}
	return ""
}

func normalized(value *string) string { return strings.ToLower(strings.TrimSpace(*value)) }
func trimmed(value *string) string    { return strings.TrimSpace(*value) }
