package models

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDecisionSpinePersistsImmutableIdentity(t *testing.T) {
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&CandidateSnapshot{}, &ResearchCase{}, &UserThesis{}); err != nil {
		t.Fatal(err)
	}

	candidate := CandidateSnapshot{
		Symbol:          "NVDA",
		Market:          MarketNASDAQ100,
		ScreenName:      "CANSLIM",
		ContextSnapshot: map[string]any{"data_as_of": "2026-08-04"},
	}
	if err := database.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}
	research := ResearchCase{
		CandidateSnapshotID: candidate.ID,
		Symbol:              "NVDA",
		Market:              MarketNASDAQ100,
		EvidenceSnapshot:    map[string]any{"status": "partial"},
	}
	if err := database.Create(&research).Error; err != nil {
		t.Fatal(err)
	}
	thesis := UserThesis{
		ResearchCaseID: research.ID,
		Symbol:         "NVDA",
		Version:        1,
		Stance:         "watch",
		Summary:        "等待确认",
		Falsifiers:     []string{"跌破结构"},
	}
	if err := database.Create(&thesis).Error; err != nil {
		t.Fatal(err)
	}

	if candidate.ID == uuid.Nil || research.ID == uuid.Nil || thesis.ID == uuid.Nil {
		t.Fatal("expected stable UUIDs")
	}
	if research.CandidateSnapshotID != candidate.ID || thesis.ResearchCaseID != research.ID {
		t.Fatal("expected traceable foreign identities")
	}
}
