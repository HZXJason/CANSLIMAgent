package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type BrokerExecution struct {
	ExternalID string  `json:"external_id"`
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	Side       string  `json:"side"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price"`
	Fee        float64 `json:"fee"`
	Currency   string  `json:"currency"`
	Market     string  `json:"market"`
	ExecutedAt string  `json:"executed_at"`
}

type BrokerPosition struct {
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Market   string  `json:"market"`
	Currency string  `json:"currency"`
}

type BrokerSyncRequest struct {
	Broker      string            `json:"broker"`
	Credentials map[string]string `json:"credentials"`
	StartAt     time.Time         `json:"start_at"`
	EndAt       time.Time         `json:"end_at"`
}

type BrokerSyncResult struct {
	Positions  []BrokerPosition  `json:"positions"`
	Executions []BrokerExecution `json:"executions"`
	Warnings   []string          `json:"warnings"`
}

type BrokerSyncUpstream interface {
	Sync(context.Context, BrokerSyncRequest) (BrokerSyncResult, error)
}

// HTTPBrokerSyncUpstream calls the local sidecar only. Credentials are sent
// over loopback for the duration of one sync and are never logged or returned.
type HTTPBrokerSyncUpstream struct {
	url    string
	client *http.Client
}

func NewHTTPBrokerSyncUpstream(sidecarURL string, timeout time.Duration) *HTTPBrokerSyncUpstream {
	return &HTTPBrokerSyncUpstream{
		url:    strings.TrimRight(strings.TrimSpace(sidecarURL), "/") + "/broker/sync",
		client: &http.Client{Timeout: timeout},
	}
}

func (u *HTTPBrokerSyncUpstream) Sync(ctx context.Context, payload BrokerSyncRequest) (BrokerSyncResult, error) {
	if err := validateLoopbackBrokerUpstream(u.url); err != nil {
		return BrokerSyncResult{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BrokerSyncResult{}, fmt.Errorf("encode broker sync request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url, bytes.NewReader(body))
	if err != nil {
		return BrokerSyncResult{}, fmt.Errorf("create broker sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.client.Do(req)
	if err != nil {
		return BrokerSyncResult{}, fmt.Errorf("broker sync upstream unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var detail struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&detail)
		if detail.Detail == "" {
			detail.Detail = resp.Status
		}
		return BrokerSyncResult{}, fmt.Errorf("broker sync failed: %s", detail.Detail)
	}
	var result BrokerSyncResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return BrokerSyncResult{}, fmt.Errorf("decode broker sync response: %w", err)
	}
	return result, nil
}

func validateLoopbackBrokerUpstream(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" {
		return fmt.Errorf("broker sync upstream must be a local HTTP sidecar")
	}
	if !isLoopbackBrokerHost(parsed.Hostname()) {
		return fmt.Errorf("broker credentials may only be sent to a loopback sidecar")
	}
	return nil
}
