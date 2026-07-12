package handlers

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BrokerHandler struct {
	db         *gorm.DB
	vault      *services.BrokerCredentialVault
	syncer     services.BrokerSyncUpstream
	authorizer services.BrokerAuthorizer
	now        func() time.Time
}

type brokerConnectionResponse struct {
	ID                 string                        `json:"id"`
	Broker             string                        `json:"broker"`
	Label              string                        `json:"label"`
	AccountName        string                        `json:"account_name"`
	ConnectionMethod   string                        `json:"connection_method"`
	ConnectionStatus   models.BrokerConnectionStatus `json:"connection_status"`
	Message            string                        `json:"message"`
	Connected          bool                          `json:"connected"`
	SecureStorageReady bool                          `json:"secure_storage_ready"`
	LastSyncedAt       *time.Time                    `json:"last_synced_at"`
}

type brokerConnectAction struct {
	Action           string `json:"action"`
	AuthorizationURL string `json:"authorization_url,omitempty"`
}

type brokerSyncRequest struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type importBrokerExecutionsRequest struct {
	Executions []services.BrokerExecution `json:"executions"`
}

func NewBrokerHandler(database *gorm.DB, vault *services.BrokerCredentialVault, syncer services.BrokerSyncUpstream, authorizer services.BrokerAuthorizer) *BrokerHandler {
	return &BrokerHandler{db: database, vault: vault, syncer: syncer, authorizer: authorizer, now: time.Now}
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
		item := brokerConnectionResponse{Broker: broker, ConnectionStatus: models.BrokerConnectionStatusUnconnected, Message: brokerConnectionMessage(models.BrokerConnectionStatusUnconnected), SecureStorageReady: h.vault != nil}
		if connected {
			item = newBrokerConnectionResponse(connection, h.vault != nil)
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
	if h.authorizer == nil {
		Fail(c, http.StatusServiceUnavailable, "broker authorization is not configured")
		return
	}
	if isGatewayBroker(broker) {
		h.detectGateway(c, broker)
		return
	}

	start, err := h.authorizer.BeginOAuth(c.Request.Context(), broker)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not configured") {
			Fail(c, http.StatusConflict, "OAuth 开发者应用尚未配置")
			return
		}
		Fail(c, http.StatusBadGateway, "OAuth authorization could not be started")
		return
	}
	if strings.TrimSpace(start.AuthorizationURL) == "" || len(start.StateHash) == 0 || len(start.VerifierCiphertext) == 0 || len(start.VerifierNonce) == 0 || !start.ExpiresAt.After(h.now()) {
		Fail(c, http.StatusInternalServerError, "OAuth authorization could not be initialized")
		return
	}
	now := h.now()
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		session := models.BrokerAuthorizationSession{
			Broker:                 broker,
			StateHash:              start.StateHash,
			PKCEVerifierCiphertext: start.VerifierCiphertext,
			PKCEVerifierNonce:      start.VerifierNonce,
			ExpiresAt:              start.ExpiresAt,
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if err := h.setPendingOAuthConnection(tx, broker, session.ID, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker authorization could not be saved")
		return
	}
	OK(c, brokerConnectAction{Action: "redirect", AuthorizationURL: start.AuthorizationURL})
}

func (h *BrokerHandler) OAuthCallback(c *gin.Context) {
	broker := strings.ToLower(strings.TrimSpace(c.Param("broker")))
	if !isOAuthBroker(broker) {
		Fail(c, http.StatusNotFound, "unsupported OAuth broker")
		return
	}
	session, ok := h.consumeOAuthSession(c.Request.Context(), broker, c.Query("state"))
	if !ok {
		Fail(c, http.StatusBadRequest, "OAuth authorization state is invalid or expired")
		return
	}
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, providerError)
		return
	}
	if strings.TrimSpace(c.Query("code")) == "" || h.authorizer == nil || h.vault == nil {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, "authorization_failed")
		return
	}
	if len(session.PKCEVerifierNonce) != 12 {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, "authorization_failed")
		return
	}
	verifier, err := h.vault.Decrypt(session.PKCEVerifierCiphertext, session.PKCEVerifierNonce)
	if err != nil || strings.TrimSpace(verifier["pkce_verifier"]) == "" {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, "authorization_failed")
		return
	}
	identity, err := h.authorizer.CompleteOAuth(c.Request.Context(), broker, c.Query("code"), verifier["pkce_verifier"])
	if err != nil || !serverCredentialsPresent(identity.Credentials) {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, "authorization_failed")
		return
	}
	ciphertext, nonce, err := h.vault.Encrypt(identity.Credentials)
	if err != nil {
		h.clearPendingOAuthConnection(c.Request.Context(), broker, session.ID)
		h.redirectBrokerSettings(c, broker, "authorization_failed")
		return
	}
	if err := h.completeOAuthConnection(c.Request.Context(), broker, session.ID, identity.AccountName, identity.ExpiresAt, ciphertext, nonce); err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be saved")
		return
	}
	h.redirectBrokerSettings(c, broker, "")
}

func (h *BrokerHandler) Detect(c *gin.Context) {
	if h.vault == nil {
		Fail(c, http.StatusServiceUnavailable, "secure broker credential storage is not configured")
		return
	}
	broker := strings.ToLower(strings.TrimSpace(c.Param("broker")))
	if !isGatewayBroker(broker) {
		Fail(c, http.StatusNotFound, "unsupported gateway broker")
		return
	}
	if h.authorizer == nil {
		Fail(c, http.StatusServiceUnavailable, "broker authorization is not configured")
		return
	}
	h.detectGateway(c, broker)
}

func (h *BrokerHandler) detectGateway(c *gin.Context, broker string) {
	identity, err := h.authorizer.DetectGateway(c.Request.Context(), broker)
	if err != nil {
		var unavailable *services.BrokerGatewayUnavailableError
		if errors.As(err, &unavailable) {
			connection, saveErr := h.saveGatewayUnavailable(c.Request.Context(), broker)
			if saveErr != nil {
				Fail(c, http.StatusInternalServerError, "broker connection could not be saved")
				return
			}
			OK(c, newBrokerConnectionResponse(connection, true))
			return
		}
		Fail(c, http.StatusBadGateway, "broker gateway detection failed")
		return
	}
	if !serverCredentialsPresent(identity.Credentials) || strings.TrimSpace(identity.AccountName) == "" {
		Fail(c, http.StatusBadGateway, "broker gateway returned invalid account metadata")
		return
	}
	ciphertext, nonce, err := h.vault.Encrypt(identity.Credentials)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker credentials could not be encrypted")
		return
	}
	if err := h.upsertConnectedConnection(c.Request.Context(), broker, "gateway", identity.AccountName, nil, ciphertext, nonce); err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be saved")
		return
	}
	var connection models.BrokerConnection
	if err := h.db.WithContext(c.Request.Context()).Where("broker = ?", broker).First(&connection).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "broker connection could not be loaded")
		return
	}
	OK(c, newBrokerConnectionResponse(connection, true))
}

func (h *BrokerHandler) Delete(c *gin.Context) {
	connection, ok := h.findConnection(c)
	if !ok {
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("broker = ?", connection.Broker).Delete(&models.BrokerAuthorizationSession{}).Error; err != nil {
			return err
		}
		return tx.Delete(&connection).Error
	}); err != nil {
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
		if isBrokerAuthorizationFailure(err) {
			now := h.now().UTC()
			_ = h.db.WithContext(c.Request.Context()).Model(&connection).Updates(map[string]any{"connection_status": models.BrokerConnectionStatusReconnectRequired, "updated_at": now}).Error
			Fail(c, http.StatusUnauthorized, "broker authorization requires reconnection")
			return
		}
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
	now := h.now()
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
	if connection.ConnectionStatus != models.BrokerConnectionStatusConnected {
		Fail(c, http.StatusConflict, "broker connection is not authorized")
		return models.BrokerConnection{}, nil, false
	}
	credentials, err := h.vault.Decrypt(connection.CredentialCiphertext, connection.CredentialNonce)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "broker credentials could not be read")
		return models.BrokerConnection{}, nil, false
	}
	return connection, credentials, true
}

func newBrokerConnectionResponse(connection models.BrokerConnection, secure bool) brokerConnectionResponse {
	status := connection.ConnectionStatus
	if !models.ValidBrokerConnectionStatus(status) {
		status = models.BrokerConnectionStatusUnconnected
	}
	return brokerConnectionResponse{
		ID:                 connection.ID.String(),
		Broker:             connection.Broker,
		Label:              connection.Label,
		AccountName:        connection.AccountName,
		ConnectionMethod:   connection.ConnectionMethod,
		ConnectionStatus:   status,
		Message:            brokerConnectionMessage(status),
		Connected:          status == models.BrokerConnectionStatusConnected,
		SecureStorageReady: secure,
		LastSyncedAt:       connection.LastSyncedAt,
	}
}

func (h *BrokerHandler) setPendingOAuthConnection(tx *gorm.DB, broker string, sessionID uuid.UUID, now time.Time) error {
	var connection models.BrokerConnection
	err := tx.Where("broker = ?", broker).First(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&models.BrokerConnection{
			Broker:                        broker,
			Label:                         brokerDisplayName(broker),
			CredentialCiphertext:          []byte{},
			CredentialNonce:               []byte{},
			ConnectionMethod:              "oauth",
			ConnectionStatus:              models.BrokerConnectionStatusPending,
			PendingAuthorizationSessionID: &sessionID,
			UpdatedAt:                     now,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&connection).Updates(map[string]any{
		"account_name":                     "",
		"connection_method":                "oauth",
		"connection_status":                models.BrokerConnectionStatusPending,
		"pending_authorization_session_id": sessionID,
		"authorization_expires_at":         nil,
		"updated_at":                       now,
	}).Error
}

func (h *BrokerHandler) consumeOAuthSession(ctx context.Context, broker, rawState string) (models.BrokerAuthorizationSession, bool) {
	if strings.TrimSpace(rawState) == "" {
		return models.BrokerAuthorizationSession{}, false
	}
	digest := sha256.Sum256([]byte(rawState))
	now := h.now()
	var session models.BrokerAuthorizationSession
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("broker = ? AND state_hash = ?", broker, digest[:]).First(&session).Error; err != nil {
			return err
		}
		result := tx.Model(&models.BrokerAuthorizationSession{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", session.ID, now).
			Updates(map[string]any{"consumed_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return session, err == nil
}

func (h *BrokerHandler) clearPendingOAuthConnection(ctx context.Context, broker string, sessionID uuid.UUID) {
	now := h.now()
	_ = h.db.WithContext(ctx).Model(&models.BrokerConnection{}).
		Where("broker = ? AND connection_status = ? AND pending_authorization_session_id = ?", broker, models.BrokerConnectionStatusPending, sessionID).
		Updates(map[string]any{
			"account_name":                     "",
			"connection_method":                "",
			"connection_status":                models.BrokerConnectionStatusUnconnected,
			"pending_authorization_session_id": nil,
			"authorization_expires_at":         nil,
			"updated_at":                       now,
		}).Error
}

// completeOAuthConnection stores credentials only when this callback still
// owns the pending connection. A later Connect call binds a newer session ID,
// turning older callbacks into no-ops rather than allowing them to overwrite
// that newer authorization attempt.
func (h *BrokerHandler) completeOAuthConnection(ctx context.Context, broker string, sessionID uuid.UUID, accountName string, expiresAt *time.Time, ciphertext, nonce []byte) error {
	now := h.now()
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.BrokerConnection{}).
			Where("broker = ? AND connection_status = ? AND pending_authorization_session_id = ?", broker, models.BrokerConnectionStatusPending, sessionID).
			Updates(map[string]any{
				"credential_ciphertext":            ciphertext,
				"credential_nonce":                 nonce,
				"account_name":                     strings.TrimSpace(accountName),
				"connection_method":                "oauth",
				"connection_status":                models.BrokerConnectionStatusConnected,
				"pending_authorization_session_id": nil,
				"authorization_expires_at":         expiresAt,
				"updated_at":                       now,
			}).Error
	})
}

func (h *BrokerHandler) upsertConnectedConnection(ctx context.Context, broker, method, accountName string, expiresAt *time.Time, ciphertext, nonce []byte) error {
	now := h.now()
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var connection models.BrokerConnection
		err := tx.Where("broker = ?", broker).First(&connection).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&models.BrokerConnection{
				Broker:                 broker,
				Label:                  brokerDisplayName(broker),
				CredentialCiphertext:   ciphertext,
				CredentialNonce:        nonce,
				AccountName:            strings.TrimSpace(accountName),
				ConnectionMethod:       method,
				ConnectionStatus:       models.BrokerConnectionStatusConnected,
				AuthorizationExpiresAt: expiresAt,
				UpdatedAt:              now,
			}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&connection).Updates(map[string]any{
			"credential_ciphertext":    ciphertext,
			"credential_nonce":         nonce,
			"account_name":             strings.TrimSpace(accountName),
			"connection_method":        method,
			"connection_status":        models.BrokerConnectionStatusConnected,
			"authorization_expires_at": expiresAt,
			"updated_at":               now,
		}).Error
	})
}

func (h *BrokerHandler) saveGatewayUnavailable(ctx context.Context, broker string) (models.BrokerConnection, error) {
	now := h.now().UTC()
	var connection models.BrokerConnection
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("broker = ?", broker).First(&connection).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			connection = models.BrokerConnection{
				Broker:               broker,
				Label:                brokerDisplayName(broker),
				CredentialCiphertext: []byte{},
				CredentialNonce:      []byte{},
				ConnectionMethod:     "gateway",
				ConnectionStatus:     models.BrokerConnectionStatusGatewayUnavailable,
				UpdatedAt:            now,
			}
			return tx.Create(&connection).Error
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&connection).Updates(map[string]any{
			"account_name":      "",
			"connection_method": "gateway",
			"connection_status": models.BrokerConnectionStatusGatewayUnavailable,
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}
		connection.AccountName = ""
		connection.ConnectionMethod = "gateway"
		connection.ConnectionStatus = models.BrokerConnectionStatusGatewayUnavailable
		return nil
	})
	return connection, err
}

func (h *BrokerHandler) redirectBrokerSettings(c *gin.Context, broker, connectionError string) {
	query := url.Values{"broker": {broker}}
	if strings.TrimSpace(connectionError) != "" {
		query.Set("connection_error", connectionError)
	}
	c.Redirect(http.StatusFound, "/settings?"+query.Encode())
}

func serverCredentialsPresent(values map[string]string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func isOAuthBroker(broker string) bool {
	return broker == models.BrokerLongPort || broker == models.BrokerTiger
}

func isGatewayBroker(broker string) bool {
	return broker == models.BrokerFutu || broker == models.BrokerIBKR
}

func isBrokerAuthorizationFailure(err error) bool {
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"authorization", "unauthorized", "authentication", "invalid token", "token expired", "token revoked"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func brokerConnectionMessage(status models.BrokerConnectionStatus) string {
	switch status {
	case models.BrokerConnectionStatusPending:
		return "等待授权完成"
	case models.BrokerConnectionStatusConnected:
		return "已连接"
	case models.BrokerConnectionStatusReconnectRequired:
		return "授权已失效，请重新连接"
	case models.BrokerConnectionStatusGatewayUnavailable:
		return "未检测到本地券商网关"
	default:
		return "未连接"
	}
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
