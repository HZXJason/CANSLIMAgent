package handlers

import (
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestCreateJournalEntryBuildsVibeTradeRecord(t *testing.T) {
	quantity := 12.5
	price := 185.25
	fee := 1.2
	entry, validationErr := createJournalEntryRequest{
		Datetime: "2026-07-10T09:35",
		Symbol:   " aapl ",
		Name:     "Apple Inc.",
		Side:     "BUY",
		Quantity: &quantity,
		Price:    &price,
		Fee:      &fee,
	}.toModel()

	if validationErr != "" {
		t.Fatalf("unexpected validation error: %s", validationErr)
	}
	if entry.Symbol != "AAPL" || entry.Side != models.TradeSideBuy {
		t.Fatalf("unexpected identity: %+v", entry)
	}
	if entry.Market != models.TradeMarketUS || entry.Currency != "USD" {
		t.Fatalf("expected inferred US market/currency, got %+v", entry)
	}
	if entry.Amount != quantity*price || entry.Fee != fee {
		t.Fatalf("unexpected execution amounts: %+v", entry)
	}
	if entry.ExecutedAt.Format("2006-01-02 15:04") != "2026-07-10 09:35" {
		t.Fatalf("unexpected execution time: %s", entry.ExecutedAt)
	}
}

func TestCreateJournalEntryRejectsMissingQuantity(t *testing.T) {
	price := 10.0
	_, validationErr := createJournalEntryRequest{
		Datetime: "2026-07-10",
		Symbol:   "NVDA",
		Side:     "buy",
		Price:    &price,
	}.toModel()
	if validationErr != "quantity must be positive" {
		t.Fatalf("unexpected validation error: %q", validationErr)
	}
}

func TestPythonTradeRecordUsesExpectedFieldValues(t *testing.T) {
	entry := models.TradeJournalEntry{
		ExecutedAt: time.Date(2026, 7, 10, 14, 5, 0, 0, time.UTC),
		Symbol:     "NVDA",
		Name:       "NVIDIA",
		Side:       "sell",
		Quantity:   3,
		Price:      170,
		Amount:     510,
		Fee:        0.75,
		Market:     "us",
	}
	record := newPythonTradeRecord(entry)
	if record.Datetime != "2026-07-10 14:05:00" || record.Symbol != "NVDA" || record.Quantity != 3 || record.Amount != 510 {
		t.Fatalf("unexpected python record: %+v", record)
	}
}
