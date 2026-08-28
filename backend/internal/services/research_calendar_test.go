package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResearchCalendarClientUsesSidecarResearchSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/week" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Sidecar-Token") != "sidecar-secret" {
			t.Fatal("missing sidecar authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"date":"2026-08-05","kind":"earnings","title":"NVDA 财报","subtitle":"预估 EPS 1.23","symbol":"NVDA","importance":"high","source":"Yahoo Finance"},{"date":"2026-08-07","kind":"economic","title":"Employment Situation","subtitle":"美国 · 高影响宏观发布","importance":"high","source":"FRED"}],"sources":[{"name":"Yahoo Finance","url":"https://finance.yahoo.com"},{"name":"FRED","url":"https://fred.stlouisfed.org/releases/calendar"}],"errors":[]}`))
	}))
	defer server.Close()

	client := NewHTTPResearchCalendarClient(server.URL, time.Second, "sidecar-secret")
	result, err := client.FetchWeekEvents(context.Background(), "US", time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), []string{"NVDA"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || result.Events[0].Source != "Yahoo Finance" || result.Events[1].Source != "FRED" {
		t.Fatalf("unexpected events: %+v", result.Events)
	}
	if len(result.Sources) != 2 || result.Sources[0].Name != "Yahoo Finance" || result.Sources[1].Name != "FRED" {
		t.Fatalf("unexpected sources: %+v", result.Sources)
	}
}

func TestResearchCalendarClientFetchesMonthAndPreservesOutcomeFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/month" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"date":"2026-08-12","time":"08:30","kind":"economic","title":"美国消费者价格指数（CPI）","subtitle":"美国 · Jul","importance":"high","source":"Yahoo Economic Calendar","result":"实际 3.4% · 前值 3.5%","market_impact":"bullish","impact_reason":"通胀降温通常缓解利率压力","live_url":"https://www.federalreserve.gov/live-broadcast.htm"}],"sources":[],"errors":[]}`))
	}))
	defer server.Close()

	client := NewHTTPResearchCalendarClient(server.URL, time.Second, "")
	result, err := client.FetchMonthEvents(context.Background(), "US", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	event := result.Events[0]
	if event.Result == "" || event.MarketImpact != "bullish" || event.ImpactReason == "" || event.LiveURL == "" {
		t.Fatalf("outcome fields were not preserved: %+v", event)
	}
}
