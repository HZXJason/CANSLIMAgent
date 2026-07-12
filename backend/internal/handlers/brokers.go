package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BrokerHandler struct {
	db     *gorm.DB
	vault  *services.BrokerCredentialVault
	syncer services.BrokerSyncUpstream
	now    func() time.Time
}

type brokerConnectionResponse struct {
	ID                 string     `json:"id"`
	Broker             string     `json:"broker"`
	Label              string     `json:"label"`
	Connected          bool       `json:"connected"`
	SecureStorageReady bool       `json:"secure_storage_ready"`
	LastSyncedAt       *time.Time `json:"last_synced_at"`
}

type connectBrokerRequest struct {
	Label       string            `json:"label"`
	Credentials map[string]string `json:"credentials"`
}

type brokerSyncRequest struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type importBrokerExecutionsRequest struct {
	Executions []services.BrokerExecution `json:"executions"`
}

func NewBrokerHandler(database *gorm.DB, vault *services.BrokerCredentialVault, syncer services.BrokerSyncUpstream) *BrokerHandler {
	return &BrokerHandler{db: database, vault: vault, syncer: syncer, now: time.Now}
}

func (h *BrokerHandler) List(c *gin.Context) {
	var connections []models.BrokerConnection
	if err := h.db.WithContext(c.Request.Context()).Order("broker asc").Find(&connections).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "broker connections could not be loaded")
		return
	}
	byBroker := make(map[string]models.BrokerConnection, len(connections))
	for _, connection := range connections {
		byBroker[connection.Broker] = connection
	}
	response := make([]brokerConnectionResponse, 0, 4)
	for _, broker := range []string{models.BrokerLongPort, models.BrokerTiger, models.BrokerFutu, models.BrokerIBKR} {
		connection, connected := byBroker[broker]
		item := brokerConnectionResponse{Broker: broker, Connected: connected, SecureStorageReady: h.vault != nil}
		if connected {
			item.ID = connection.ID.String()
			item.Label = connection.Label
			item.LastSyncedAt = connection.LastSyncedAt
		}
		response = append(response, item)
	}
	OK(c, response)
}

func (h *BrokerHandler) Connect(c *gin.Context) {
	if h.vault == nil {
		Fail(c, http.StatusServiceUnavailable, "secure broker credential storage is not configured")
		return
	}
	broker := strings.ToLower(strings.TrimSpace(c.Param("broker")))
	if !models.ValidBroker(broker) {
		Fail(c, http.StatusNotFound, "unsupported broker")
		return
	}
	var request connectBrokerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	credentials := cleanBrokerCredentials(request.Credentials)
	if err := validateBrokerCredentials(broker, credentials); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ciphertext, nonce, err := h.vault.Encrypt(credentials)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker credentials could not be encrypted")
		return
	}
	label := strings.TrimSpace(request.Label)
	if label == "" {
		label = brokerDisplayName(broker)
	}
	var connection models.BrokerConnection
	err = h.db.WithContext(c.Request.Context()).Where("broker = ?", broker).First(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		connection = models.BrokerConnection{Broker: broker, Label: label, CredentialCiphertext: ciphertext, CredentialNonce: nonce}
		err = h.db.WithContext(c.Request.Context()).Create(&connection).Error
	} else if err == nil {
		err = h.db.WithContext(c.Request.Context()).Model(&connection).Updates(map[string]any{"label": label, "credential_ciphertext": ciphertext, "credential_nonce": nonce, "updated_at": h.now().UTC()}).Error
		if err == nil {
			connection.Label = label
			connection.CredentialCiphertext = ciphertext
			connection.CredentialNonce = nonce
		}
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be saved")
		return
	}
	OK(c, newBrokerConnectionResponse(connection, true, true))
}

func (h *BrokerHandler) Delete(c *gin.Context) {
	connection, ok := h.findConnection(c)
	if !ok {
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Delete(&connection).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be deleted")
		return
	}
	OK(c, gin.H{"deleted": true})
}

func (h *BrokerHandler) Sync(c *gin.Context) {
	connection, credentials, ok := h.readConnectionCredentials(c)
	if !ok {
		return
	}
	var request brokerSyncRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	endAt := h.now().UTC()
	startAt := endAt.AddDate(0, -3, 0)
	var err error
	if strings.TrimSpace(request.StartAt) != "" {
		startAt, err = parseBrokerTime(request.StartAt)
		if err != nil {
			Fail(c, http.StatusBadRequest, "start_at must be RFC3339")
			return
		}
	}
	if strings.TrimSpace(request.EndAt) != "" {
		endAt, err = parseBrokerTime(request.EndAt)
		if err != nil {
			Fail(c, http.StatusBadRequest, "end_at must be RFC3339")
			return
		}
	}
	if startAt.After(endAt) || endAt.Sub(startAt) > 366*24*time.Hour {
		Fail(c, http.StatusBadRequest, "sync range must be between zero and 366 days")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Second)
	defer cancel()
	result, err := h.syncer.Sync(ctx, services.BrokerSyncRequest{Broker: connection.Broker, Credentials: credentials, StartAt: startAt, EndAt: endAt})
	if err != nil {
		Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	normalized, warnings := normalizeBrokerExecutions(connection.Broker, result.Executions)
	result.Executions = normalized
	result.Warnings = append(result.Warnings, warnings...)
	OK(c, result)
}

func (h *BrokerHandler) Import(c *gin.Context) {
	connection, _, ok := h.readConnectionCredentials(c)
	if !ok {
		return
	}
	var request importBrokerExecutionsRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Executions) == 0 {
		Fail(c, http.StatusBadRequest, "at least one broker execution is required")
		return
	}
	executions, warnings := normalizeBrokerExecutions(connection.Broker, request.Executions)
	if len(executions) == 0 {
		Fail(c, http.StatusBadRequest, "no valid broker executions to import")
		return
	}
	created, skipped := 0, 0
	source := "broker:" + connection.Broker
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		for _, execution := range executions {
			var existing models.TradeJournalEntry
			err := tx.Where("source = ? AND order_id = ?", source, execution.ExternalID).First(&existing).Error
			if err == nil {
				skipped++
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			executedAt, _ := parseBrokerTime(execution.ExecutedAt)
			externalID := execution.ExternalID
			entry := models.TradeJournalEntry{ExecutedAt: executedAt, Symbol: execution.Symbol, Name: execution.Name, Side: execution.Side, Quantity: execution.Quantity, Price: execution.Price, Amount: execution.Quantity * execution.Price, Fee: execution.Fee, Market: execution.Market, Currency: execution.Currency, OrderID: &externalID, Source: source}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			created++
		}
		return nil
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker executions could not be imported")
		return
	}
	now := h.now().UTC()
	_ = h.db.WithContext(c.Request.Context()).Model(&connection).Updates(map[string]any{"last_synced_at": now, "updated_at": now}).Error
	OK(c, gin.H{"created": created, "skipped": skipped, "warnings": warnings})
}

func (h *BrokerHandler) findConnection(c *gin.Context) (models.BrokerConnection, bool) {
	var connection models.BrokerConnection
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "broker connection not found")
		return connection, false
	}
	err = h.db.WithContext(c.Request.Context()).First(&connection, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "broker connection not found")
		return connection, false
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be loaded")
		return connection, false
	}
	return connection, true
}

func (h *BrokerHandler) readConnectionCredentials(c *gin.Context) (models.BrokerConnection, map[string]string, bool) {
	if h.vault == nil {
		Fail(c, http.StatusServiceUnavailable, "secure broker credential storage is not configured")
		return models.BrokerConnection{}, nil, false
	}
	connection, ok := h.findConnection(c)
	if !ok {
		return models.BrokerConnection{}, nil, false
	}
	credentials, err := h.vault.Decrypt(connection.CredentialCiphertext, connection.CredentialNonce)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker credentials could not be read")
		return models.BrokerConnection{}, nil, false
	}
	return connection, credentials, true
}

func newBrokerConnectionResponse(connection models.BrokerConnection, connected, secure bool) brokerConnectionResponse {
	return brokerConnectionResponse{ID: connection.ID.String(), Broker: connection.Broker, Label: connection.Label, Connected: connected, SecureStorageReady: secure, LastSyncedAt: connection.LastSyncedAt}
}

func cleanBrokerCredentials(values map[string]string) map[string]string {
	cleaned := map[string]string{}
	for key, value := range values {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			cleaned[key] = value
		}
	}
	return cleaned
}

func validateBrokerCredentials(broker string, values map[string]string) error {
	required := map[string][]string{
		models.BrokerLongPort: {"access_token"},
		models.BrokerTiger:    {"tiger_id", "account", "private_key"},
		models.BrokerFutu:     {"host", "port"},
		models.BrokerIBKR:     {"host", "port"},
	}
	for _, key := range required[broker] {
		if values[key] == "" {
			return fmt.Errorf("missing required broker setting: %s", key)
		}
	}
	return nil
}

func brokerDisplayName(broker string) string {
	switch broker {
	case models.BrokerLongPort:
		return "长桥 LongPort"
	case models.BrokerTiger:
		return "老虎证券 Tiger"
	case models.BrokerFutu:
		return "富途证券 Futu"
	case models.BrokerIBKR:
		return "盈透证券 IBKR"
	default:
		return broker
	}
}
