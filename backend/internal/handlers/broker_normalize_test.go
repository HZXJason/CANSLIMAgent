package handlers

import (
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/services"
)

func TestNormalizeLongPortExecutionsForJournal(t *testing.T) {
	executions, warnings := normalizeBrokerExecutions("longport", []services.BrokerExecution{
		{ExternalID: "fill-1", Symbol: "AAPL.US", Side: "BUY", Quantity: 2, Price: 200, Fee: 1.25, ExecutedAt: "2026-07-12T09:30:00Z"},
		{ExternalID: "fill-1", Symbol: "AAPL.US", Side: "BUY", Quantity: 2, Price: 200, ExecutedAt: "2026-07-12T09:30:00Z"},
	})
	if len(executions) != 1 || executions[0].Symbol != "AAPL" || executions[0].Side != "buy" {
		t.Fatalf("unexpected normalized executions: %+v", executions)
	}
	if executions[0].Market != "us" || executions[0].Currency != "USD" {
		t.Fatalf("expected US journal fields, got %+v", executions[0])
	}
	if len(warnings) != 1 {
		t.Fatalf("expected duplicate warning, got %v", warnings)
	}
}

func TestNormalizeBrokerExecutionsSkipsInvalidRecords(t *testing.T) {
	executions, warnings := normalizeBrokerExecutions("futu", []services.BrokerExecution{
		{ExternalID: "", Symbol: "US.NVDA", Side: "BUY", Quantity: 1, Price: 150, ExecutedAt: "2026-07-12T09:30:00Z"},
	})
	if len(executions) != 0 || len(warnings) != 1 {
		t.Fatalf("expected invalid fill to be skipped, got %+v / %v", executions, warnings)
	}
}
