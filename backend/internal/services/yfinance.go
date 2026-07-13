package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type YFinanceClient struct {
	enabled    bool
	python     string
	scriptPath string
	timeout    time.Duration
	checkOnce  sync.Once
	checkErr   error
}

func NewYFinanceClient(enabled bool, python string, timeout time.Duration) *YFinanceClient {
	_, currentFile, _, _ := runtime.Caller(0)
	return &YFinanceClient{
		enabled:    enabled,
		python:     resolvePythonExecutable(python),
		scriptPath: filepath.Join(filepath.Dir(currentFile), "yfinance_helper.py"),
		timeout:    timeout,
	}
}

func resolvePythonExecutable(configured string) string {
	resolved, err := exec.LookPath(strings.TrimSpace(configured))
	if err != nil {
		return ""
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return ""
	}
	if evaluated, evalErr := filepath.EvalSymlinks(resolved); evalErr == nil {
		resolved = evaluated
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	base := strings.ToLower(filepath.Base(resolved))
	if !strings.HasPrefix(base, "python") {
		return ""
	}
	if relative, err := filepath.Rel(os.TempDir(), resolved); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return resolved
}

func (c *YFinanceClient) Configured() bool {
	return c != nil && c.enabled && c.python != ""
}

func (c *YFinanceClient) FetchUniverse(ctx context.Context, market string) ([]StockCandidate, error) {
	if err := c.ensureAvailable(); err != nil {
		return nil, err
	}

	var payload struct {
		Error string `json:"error"`
		Items []struct {
			Symbol string `json:"symbol"`
			Name   string `json:"name"`
		} `json:"items"`
	}
	if err := c.runJSON(ctx, &payload, "constituents", market); err != nil {
		return nil, err
	}
	if payload.Error != "" {
		return nil, errors.New(payload.Error)
	}

	items := make([]StockCandidate, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.Symbol == "" || item.Name == "" {
			continue
		}
		items = append(items, StockCandidate{Symbol: item.Symbol, Name: item.Name})
	}
	if len(items) == 0 {
		return nil, errors.New("yfinance 成分股返回为空")
	}
	return items, nil
}

func (c *YFinanceClient) FetchPriceSignal(ctx context.Context, symbol string, _ time.Time) (priceSignal, error) {
	if err := c.ensureAvailable(); err != nil {
		return priceSignal{}, err
	}

	var payload struct {
		Error             string   `json:"error"`
		VolumeRatio       *float64 `json:"volume_ratio"`
		TwelveMonthReturn *float64 `json:"twelve_month_return"`
		Sufficient        bool     `json:"sufficient"`
		DataAsOf          string   `json:"data_as_of"`
	}
	if err := c.runJSON(ctx, &payload, "price", symbol); err != nil {
		return priceSignal{}, err
	}
	if payload.Error != "" {
		return priceSignal{}, errors.New(payload.Error)
	}

	var dataAsOf *time.Time
	if payload.DataAsOf != "" {
		if parsed, err := time.Parse("2006-01-02", payload.DataAsOf); err == nil {
			dataAsOf = &parsed
		}
	}
	return priceSignal{
		VolumeRatio:       payload.VolumeRatio,
		TwelveMonthReturn: payload.TwelveMonthReturn,
		Sufficient:        payload.Sufficient,
		DataAsOf:          dataAsOf,
	}, nil
}

func (c *YFinanceClient) FetchEPSGrowth(ctx context.Context, symbol string) (quarterGrowth *float64, quarterSufficient bool, annualGrowth []float64, yearsAvailable int, annualSufficient bool, dataAsOf time.Time, err error) {
	if err := c.ensureAvailable(); err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}

	var payload struct {
		Error             string    `json:"error"`
		QuarterGrowth     *float64  `json:"quarter_growth"`
		QuarterSufficient bool      `json:"quarter_sufficient"`
		AnnualGrowth      []float64 `json:"annual_growth"`
		YearsAvailable    int       `json:"years_available"`
		AnnualSufficient  bool      `json:"annual_sufficient"`
		DataAsOf          string    `json:"data_as_of"`
	}
	if err := c.runJSON(ctx, &payload, "eps", symbol); err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}
	if payload.Error != "" {
		return nil, false, nil, 0, false, time.Time{}, errors.New(payload.Error)
	}
	if payload.DataAsOf != "" {
		dataAsOf, _ = time.Parse("2006-01-02", payload.DataAsOf)
	}
	return payload.QuarterGrowth, payload.QuarterSufficient, payload.AnnualGrowth, payload.YearsAvailable, payload.AnnualSufficient, dataAsOf, nil
}

func (c *YFinanceClient) FetchEPSQuarterly(ctx context.Context, symbol string) (EPSData, error) {
	if err := c.ensureAvailable(); err != nil {
		return EPSData{}, err
	}

	var payload struct {
		Error    string       `json:"error"`
		DataAsOf string       `json:"data_as_of"`
		Quarters []EPSQuarter `json:"quarters"`
	}
	if err := c.runJSON(ctx, &payload, "eps_quarterly", symbol); err != nil {
		return EPSData{}, err
	}
	if payload.Error != "" {
		return EPSData{}, errors.New(payload.Error)
	}
	if len(payload.Quarters) == 0 {
		return EPSData{}, errors.New("yfinance EPS data is empty")
	}
	return EPSData{
		Symbol:   symbol,
		Source:   "yfinance",
		DataAsOf: payload.DataAsOf,
		Quarters: payload.Quarters,
	}, nil
}

func (c *YFinanceClient) FetchRevenueAnnual(ctx context.Context, symbol string) (RevenueData, error) {
	if err := c.ensureAvailable(); err != nil {
		return RevenueData{}, err
	}

	var payload struct {
		Error    string        `json:"error"`
		DataAsOf string        `json:"data_as_of"`
		Years    []RevenueYear `json:"years"`
	}
	if err := c.runJSON(ctx, &payload, "annual_revenue", symbol); err != nil {
		return RevenueData{}, err
	}
	if payload.Error != "" {
		return RevenueData{}, errors.New(payload.Error)
	}
	if len(payload.Years) == 0 {
		return RevenueData{}, errors.New("yfinance annual revenue data is empty")
	}
	return RevenueData{
		Symbol:   symbol,
		Source:   "yfinance",
		DataAsOf: payload.DataAsOf,
		Years:    payload.Years,
	}, nil
}

func (c *YFinanceClient) FetchGrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	if err := c.ensureAvailable(); err != nil {
		return GrossMarginData{}, err
	}

	var payload struct {
		Error    string               `json:"error"`
		DataAsOf string               `json:"data_as_of"`
		Quarters []GrossMarginQuarter `json:"quarters"`
	}
	if err := c.runJSON(ctx, &payload, "gross_margin_quarterly", symbol); err != nil {
		return GrossMarginData{}, err
	}
	if payload.Error != "" {
		return GrossMarginData{}, errors.New(payload.Error)
	}
	if len(payload.Quarters) == 0 {
		return GrossMarginData{}, errors.New("yfinance gross margin data is empty")
	}
	return GrossMarginData{
		Symbol:   symbol,
		Source:   "yfinance",
		DataAsOf: payload.DataAsOf,
		Quarters: payload.Quarters,
	}, nil
}

func (c *YFinanceClient) FetchCompanyProfile(ctx context.Context, symbol string) (CompanyProfile, error) {
	if err := c.ensureAvailable(); err != nil {
		return CompanyProfile{}, err
	}

	var payload struct {
		Error       string `json:"error"`
		Symbol      string `json:"symbol"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Sector      string `json:"sector"`
		Industry    string `json:"industry"`
		Website     string `json:"website"`
	}
	if err := c.runJSON(ctx, &payload, "profile", symbol); err != nil {
		return CompanyProfile{}, err
	}
	if payload.Error != "" {
		return CompanyProfile{}, errors.New(payload.Error)
	}
	profile := CompanyProfile{
		Symbol:      normalizeSymbol(firstNonEmpty(payload.Symbol, symbol)),
		Name:        strings.TrimSpace(payload.Name),
		Description: cleanDescription(payload.Description),
		Sector:      strings.TrimSpace(payload.Sector),
		Industry:    strings.TrimSpace(payload.Industry),
		Website:     strings.TrimSpace(payload.Website),
		Source:      "yfinance",
		UpdatedAt:   time.Now().UTC().Format("2006-01-02"),
	}
	if profile.Description == "" {
		return CompanyProfile{}, errors.New("yfinance 公司简介为空")
	}
	return profile, nil
}

func (c *YFinanceClient) FetchSecondLayerData(ctx context.Context, symbol string) (SecondLayerData, error) {
	if err := c.ensureAvailable(); err != nil {
		return SecondLayerData{}, err
	}

	var payload map[string]interface{}
	if err := c.runJSON(ctx, &payload, "second_layer", symbol); err != nil {
		return SecondLayerData{}, err
	}
	if message, ok := payload["error"].(string); ok && strings.TrimSpace(message) != "" {
		return SecondLayerData{}, errors.New(message)
	}
	if len(payload) == 0 {
		return SecondLayerData{}, errors.New("yfinance 二层筛选数据为空")
	}
	return SecondLayerData(payload), nil
}

func (c *YFinanceClient) ensureAvailable() error {
	if !c.Configured() {
		return errors.New("yfinance 兜底未启用")
	}
	c.checkOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		c.checkErr = c.runJSON(ctx, &struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}{}, "health")
	})
	return c.checkErr
}

func (c *YFinanceClient) runJSON(ctx context.Context, target interface{}, args ...string) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	allArgs := append([]string{c.scriptPath}, args...)
	// #nosec G204 -- c.python is resolved to a regular Python executable at construction; the helper path is repo-owned.
	cmd := exec.CommandContext(ctx, c.python, allArgs...)
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				message = stderr
			}
		}
		if message != "" {
			return fmt.Errorf("yfinance 执行失败：%w：%s", err, message)
		}
		return fmt.Errorf("yfinance 执行失败：%w", err)
	}
	if err := json.Unmarshal(output, target); err != nil {
		return fmt.Errorf("yfinance 返回解析失败：%w", err)
	}
	return nil
}
