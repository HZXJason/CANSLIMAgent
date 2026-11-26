package models

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTradeGroupsMultipleImmutableExecutionFills(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&Trade{}, &TradeJournalEntry{}); err != nil {
		t.Fatal(err)
	}
	trade := Trade{Symbol: "NVDA", Market: TradeMarketUS, Direction: TradeDirectionLong, Status: TradeStatusOpen, OpenedAt: time.Now().UTC()}
	if err := database.Create(&trade).Error; err != nil {
		t.Fatal(err)
	}
	for _, quantity := range []float64{5, 7} {
		entry := TradeJournalEntry{TradeID: &trade.ID, ExecutedAt: time.Now().UTC(), Symbol: "NVDA", Side: TradeSideBuy, Quantity: quantity, Price: 100, Amount: quantity * 100, Market: TradeMarketUS, Source: "manual"}
		if err := database.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	var fills []TradeJournalEntry
	if err := database.Where("trade_id = ?", trade.ID).Order("created_at asc").Find(&fills).Error; err != nil {
		t.Fatal(err)
	}
	if trade.UserID != DefaultUserID || len(fills) != 2 || fills[0].UserID != DefaultUserID || fills[1].UserID != DefaultUserID {
		t.Fatalf("unexpected trade aggregate: trade=%+v fills=%+v", trade, fills)
	}
}
