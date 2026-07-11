package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type JournalTradeRecord struct {
	Datetime string  `json:"datetime"`
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Side     string  `json:"side"`
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price"`
	Amount   float64 `json:"amount"`
	Fee      float64 `json:"fee"`
	Market   string  `json:"market"`
}

type JournalReflectionRequest struct {
	SchemaVersion string               `json:"schema_version"`
	Records       []JournalTradeRecord `json:"records"`
}

type JournalReflectionResult map[string]any

type JournalReflectionUpstream interface {
	Reflect(context.Context, JournalReflectionRequest) (JournalReflectionResult, error)
}

type HTTPJournalReflectionUpstream struct {
	url    string
	client *http.Client
}

func NewHTTPJournalReflectionUpstream(upstreamURL string, timeout time.Duration) *HTTPJournalReflectionUpstream {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &HTTPJournalReflectionUpstream{
		url:    resolveJournalReflectionURL(upstreamURL),
		client: &http.Client{Timeout: timeout},
	}
}

func (u *HTTPJournalReflectionUpstream) Reflect(ctx context.Context, payload JournalReflectionRequest) (JournalReflectionResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("journal reflection upstream returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	var result JournalReflectionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func resolveJournalReflectionURL(upstreamURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if trimmed == "" {
		trimmed = DefaultAIAnalysisUpstreamURL
	}
	trimmed = strings.TrimSuffix(trimmed, "/analyze")
	return trimmed + "/journal/reflect"
}
