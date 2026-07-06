package services

import (
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestToStockResultTreatsNilMetricValuesAsInsufficient(t *testing.T) {
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
