package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AlphaVantageClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewAlphaVantageClient(apiKey string, timeout time.Duration) *AlphaVantageClient {
	return &AlphaVantageClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: "https://www.alphavantage.co",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *AlphaVantageClient) Configured() bool {
	return c != nil && c.apiKey != ""
}

func (c *AlphaVantageClient) FetchCompanyProfile(ctx context.Context, symbol string) (CompanyProfile, error) {
	if !c.Configured() {
		return CompanyProfile{}, errors.New("ALPHA_VANTAGE_API_KEY 未配置")
	}

	query := url.Values{}
	query.Set("function", "OVERVIEW")
	query.Set("symbol", normalizeSymbol(symbol))
	query.Set("apikey", c.apiKey)

	var row struct {
		Symbol      string `json:"Symbol"`
		Name        string `json:"Name"`
		Description string `json:"Description"`
		Sector      string `json:"Sector"`
		Industry    string `json:"Industry"`
		Website     string `json:"OfficialSite"`
		Note        string `json:"Note"`
		Information string `json:"Information"`
		Error       string `json:"Error Message"`
	}
	if err := c.getJSON(ctx, "/query", query, &row); err != nil {
		return CompanyProfile{}, err
	}
	if row.Error != "" {
		return CompanyProfile{}, errors.New(row.Error)
	}
	if row.Note != "" {
		return CompanyProfile{}, errors.New(row.Note)
	}
	if row.Information != "" {
		return CompanyProfile{}, errors.New(row.Information)
	}

	profile := CompanyProfile{
		Symbol:      normalizeSymbol(firstNonEmpty(row.Symbol, symbol)),
		Name:        strings.TrimSpace(row.Name),
		Description: cleanDescription(row.Description),
		Sector:      strings.TrimSpace(row.Sector),
		Industry:    strings.TrimSpace(row.Industry),
		Website:     strings.TrimSpace(row.Website),
		Source:      "Alpha Vantage",
		UpdatedAt:   time.Now().UTC().Format("2006-01-02"),
	}
	if profile.Description == "" {
		return CompanyProfile{}, errors.New("Alpha Vantage 公司简介为空")
	}
	return profile, nil
}

func (c *AlphaVantageClient) getJSON(ctx context.Context, path string, query url.Values, target interface{}) error {
	u := c.baseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Alpha Vantage 请求失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(target)
}
