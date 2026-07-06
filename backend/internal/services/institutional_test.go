package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchInstitutionalSignalFromNasdaqSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/AAPL/institutional-holdings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"activePositions": {
					"asOf": "06/30/2026",
					"rows": [
						{"positions": "Increased Positions", "holders": "2,668", "shares": "1,751,838,825"},
						{"positions": "Decreased Positions", "holders": "3,285", "shares": "204,536,861"},
						{"positions": "Held Positions", "holders": "414", "shares": "9,047,996,296"},
						{"positions": "Total Institutional Shares", "holders": "6,367", "shares": "11,004,371,982"}
					]
				},
				"newSoldOutPositions": {
					"rows": [
						{"positions": "New Positions", "holders": "201", "shares": "1,470,777,663"},
						{"positions": "Sold Out Positions", "holders": "100", "shares": "6,170,988"}
					]
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewInstitutionalClient(true, time.Second)
	client.baseURL = server.URL

	signal, err := client.FetchInstitutionalSignal(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("FetchInstitutionalSignal returned error: %v", err)
	}
	if !signal.Sufficient || signal.Delta == nil {
		t.Fatalf("expected sufficient delta, got %#v", signal)
	}
	if *signal.Delta != -516 {
		t.Fatalf("unexpected delta: got %d want -516", *signal.Delta)
	}
	if signal.DataAsOf == nil || signal.DataAsOf.Format("2006-01-02") != "2026-06-30" {
		t.Fatalf("unexpected data as of: %#v", signal.DataAsOf)
	}
}

func TestNormalizeNasdaqInstitutionalSymbol(t *testing.T) {
	got := normalizeNasdaqInstitutionalSymbol(" brk-b ")
	if got != "BRK.B" {
		t.Fatalf("unexpected symbol: got %s want BRK.B", got)
	}
}
