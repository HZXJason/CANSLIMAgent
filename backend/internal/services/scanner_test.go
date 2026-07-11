package services

import (
	"context"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestToStockResultTreatsNilMetricValuesAsInsufficient(t *testing.T) {
	aiAnalysisAt := time.Now().UTC()
	stock := models.Stock{
		Symbol: "TEST",
		Name:   "Test Company",
		Market: models.MarketSP500,
		Metric: models.StockMetric{
			ID:                1,
			CDataSufficient:   true,
			SDataSufficient:   true,
			LDataSufficient:   true,
			IDataSufficient:   true,
			IInstitutionDelta: nil,
			AIAnalysisAt:      &aiAnalysisAt,
			DataAsOf:          time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC),
		},
	}

	result := toStockResult(stock)
	if result.Metrics.C.Sufficient {
		t.Fatal("expected nil C value to be insufficient")
	}
	if result.Metrics.S.Sufficient {
		t.Fatal("expected nil S value to be insufficient")
	}
	if result.Metrics.L.Sufficient {
		t.Fatal("expected nil L value to be insufficient")
	}
	if result.Metrics.I.Sufficient {
		t.Fatal("expected nil I value to be insufficient")
	}
	if !result.AIAnalyzed {
		t.Fatal("expected completed AI analysis marker to be exposed")
	}
}

func TestCompleteCachedMetricRejectsMissingBackfillValues(t *testing.T) {
	volume := 120.0
	metric := models.StockMetric{
		ID:                1,
		SVolumeRatio:      &volume,
		SDataSufficient:   true,
		LDataSufficient:   true,
		IDataSufficient:   false,
		IInstitutionDelta: nil,
	}
	if isCompleteCachedMetric(metric, true) {
		t.Fatal("expected missing L rating to make cache incomplete")
	}

	rs := 82
	metric.LRSRating = &rs
	metric.IDataSufficient = true
	if isCompleteCachedMetric(metric, true) {
		t.Fatal("expected missing institutional delta to make cache incomplete")
	}

	delta := 12
	metric.IInstitutionDelta = &delta
	if !isCompleteCachedMetric(metric, true) {
		t.Fatal("expected metric with required values to be complete")
	}
}

func TestCompleteCachedMetricAllowsMissingInstitutionalDataWhenDisabled(t *testing.T) {
	volume := 120.0
	rs := 82
	metric := models.StockMetric{
		ID:              1,
		SVolumeRatio:    &volume,
		SDataSufficient: true,
		LRSRating:       &rs,
		LDataSufficient: true,
	}
	if !isCompleteCachedMetric(metric, false) {
		t.Fatal("expected missing institutional delta to be complete when institutional data is disabled")
	}
}

func TestNeedsSingleStockHydration(t *testing.T) {
	volume := 125.0
	rating := 87
	delta := 42
	metric := models.StockMetric{
		SVolumeRatio:      &volume,
		SDataSufficient:   true,
		LRSRating:         &rating,
		LDataSufficient:   true,
		IInstitutionDelta: &delta,
		IDataSufficient:   true,
		DataAsOf:          time.Now().UTC(),
	}
	if needsSingleStockHydration(metric) {
		t.Fatal("expected complete current metric to be retained")
	}
	metric.DataAsOf = time.Now().UTC().AddDate(0, 0, -4)
	if !needsSingleStockHydration(metric) {
		t.Fatal("expected stale metric to be refreshed")
	}
	metric.DataAsOf = time.Now().UTC()
	metric.IInstitutionDelta = nil
	if !needsSingleStockHydration(metric) {
		t.Fatal("expected missing I metric to be refreshed")
	}
}

func TestRefreshJobContextSurvivesRequestCancellation(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := requestContext.Err(); err == nil {
		t.Fatal("expected request context to be cancelled")
	}
	if err := refreshJobContext(requestContext).Err(); err != nil {
		t.Fatalf("refresh job inherited cancelled request context: %v", err)
	}
}
