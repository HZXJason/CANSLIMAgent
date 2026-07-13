package handlers

import (
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type LLMCredentialHandler struct {
	store *services.LLMCredentialStore
}

type saveLLMCredentialRequest struct {
	BackendURL string `json:"backend_url"`
	APIKey     string `json:"api_key"`
}

func NewLLMCredentialHandler(store *services.LLMCredentialStore) *LLMCredentialHandler {
	return &LLMCredentialHandler{store: store}
}

func (h *LLMCredentialHandler) Status(c *gin.Context) {
	if h.store == nil {
		Fail(c, http.StatusServiceUnavailable, "LLM credential storage is not configured")
		return
	}
	status, err := h.store.Status(c.Request.Context(), c.Param("provider"), c.Query("backend_url"))
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, status)
}

func (h *LLMCredentialHandler) Save(c *gin.Context) {
	if h.store == nil || !h.store.StorageReady() {
		Fail(c, http.StatusServiceUnavailable, "安全的 LLM 密钥存储尚未配置")
		return
	}
	var payload saveLLMCredentialRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		Fail(c, http.StatusBadRequest, "LLM 密钥请求格式错误")
		return
	}
	status, err := h.store.Save(c.Request.Context(), c.Param("provider"), payload.BackendURL, payload.APIKey)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, status)
}

func (h *LLMCredentialHandler) Delete(c *gin.Context) {
	if h.store == nil || !h.store.StorageReady() {
		Fail(c, http.StatusServiceUnavailable, "安全的 LLM 密钥存储尚未配置")
		return
	}
	if err := h.store.Delete(c.Request.Context(), c.Param("provider"), strings.TrimSpace(c.Query("backend_url"))); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}
