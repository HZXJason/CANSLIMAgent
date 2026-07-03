package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStooqFetchCandlesParsesCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprintln(w, "Date,Open,High,Low,Close,Volume")
		fmt.Fprintln(w, "2026-06-29,10,12,9,11,1000")
		fmt.Fprintln(w, "2026-06-30,11,13,10,12,1500")
	}))
	defer server.Close()

	client := NewStooqClient(true, time.Second)
	client.baseURL = server.URL

	candles, err := client.FetchCandles(context.Background(), "AAPL", PeriodDaily)
	if err != nil {
		t.Fatalf("FetchCandles returned error: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}
	if candles[1].Date != "2026-06-30" || candles[1].Close != 12 || candles[1].Volume != 1500 {
		t.Fatalf("unexpected candle: %+v", candles[1])
	}
}

func TestStooqFetchCandlesRejectsHTMLChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, "<!doctype html><html></html>")
	}))
	defer server.Close()

	client := NewStooqClient(true, time.Second)
	client.baseURL = server.URL

	if _, err := client.FetchCandles(context.Background(), "AAPL", PeriodDaily); err == nil {
		t.Fatal("expected HTML response to be rejected")
	}
	if client.Configured() {
		t.Fatal("expected Stooq client to disable itself after non-CSV response")
	}
}
