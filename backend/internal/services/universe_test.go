package services

import (
	"strings"
	"testing"
)

func TestParseIWMHoldingsSpreadsheetIgnoresInvalidDisclaimerXML(t *testing.T) {
	input := `<?xml version="1.0"?>
<ss:Workbook xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">
<ss:Worksheet ss:Name="Disclaimers">
<ss:Table>
<ss:Row>
<ss:Cell ss:HRef="https://example.com?a=1&b=2"><ss:Data ss:Type="String">bad xml before holdings</ss:Data></ss:Cell>
</ss:Row>
</ss:Table>
</ss:Worksheet>
<ss:Worksheet ss:Name="Holdings">
<ss:Table>
<ss:Row><ss:Cell><ss:Data ss:Type="String">Fund Holdings as of</ss:Data></ss:Cell></ss:Row>
<ss:Row>
<ss:Cell><ss:Data ss:Type="String">Ticker</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Name</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Sector</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Asset Class</ss:Data></ss:Cell>
</ss:Row>
<ss:Row>
<ss:Cell><ss:Data ss:Type="String">MOGA</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">MOOG INC CLASS A</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Industrials</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Equity</ss:Data></ss:Cell>
</ss:Row>
<ss:Row>
<ss:Cell><ss:Data ss:Type="String">USD</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">USD CASH</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Cash and/or Derivatives</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Cash</ss:Data></ss:Cell>
</ss:Row>
<ss:Row>
<ss:Cell><ss:Data ss:Type="String">COKE</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">COCA COLA CONSOLIDATED INC</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Consumer Staples</ss:Data></ss:Cell>
<ss:Cell><ss:Data ss:Type="String">Equity</ss:Data></ss:Cell>
</ss:Row>
</ss:Table>
</ss:Worksheet>
</ss:Workbook>`

	candidates, err := parseIWMHoldingsSpreadsheet(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseIWMHoldingsSpreadsheet returned error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 equity candidates, got %d: %+v", len(candidates), candidates)
	}
	if candidates[0].Symbol != "MOGA" || candidates[1].Symbol != "COKE" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestEnrichCandidatesWithListingsCanRemapCollapsedClassTickers(t *testing.T) {
	candidates := []StockCandidate{
		{Symbol: "MOGA", Name: "MOOG INC CLASS A"},
		{Symbol: "COKE", Name: "COCA COLA CONSOLIDATED INC"},
	}
	listed := map[string]string{
		"MOG-A": "Moog Inc. Class A Common Stock",
		"COKE":  "Coca-Cola Consolidated, Inc. Common Stock",
	}

	enriched := enrichCandidatesWithListings(candidates, listed)
	if enriched[0].Symbol != "MOG-A" {
		t.Fatalf("expected MOGA to remap to MOG-A, got %+v", enriched[0])
	}
	if enriched[1].Symbol != "COKE" {
		t.Fatalf("expected direct match to remain COKE, got %+v", enriched[1])
	}
}

func TestNormalizeUniverseSymbolRejectsPlaceholderTickers(t *testing.T) {
	if symbol := normalizeUniverseSymbol("--"); symbol != "" {
		t.Fatalf("expected placeholder ticker to be rejected, got %q", symbol)
	}
	if symbol := normalizeUniverseSymbol("MOG.A"); symbol != "MOG-A" {
		t.Fatalf("expected class ticker to normalize, got %q", symbol)
	}
}
