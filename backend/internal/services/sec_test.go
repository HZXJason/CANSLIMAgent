package services

import "testing"

func TestBuildSECQuarterlyAndAnnualMetrics(t *testing.T) {
	facts := secCompanyFacts{
		Facts: map[string]map[string]secFactTag{
			"us-gaap": {
				"EarningsPerShareDiluted": {
					Units: map[string][]secFactValue{
						"USD/shares": {
							{Start: "2025-01-01", End: "2025-03-31", Val: 2.50, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2025-05-01", Frame: "CY2025Q1"},
							{Start: "2024-01-01", End: "2024-03-31", Val: 2.00, FY: 2024, FP: "Q1", Form: "10-Q", Filed: "2024-05-01", Frame: "CY2024Q1"},
							{Start: "2025-01-01", End: "2025-12-31", Val: 8.00, FY: 2025, FP: "FY", Form: "10-K", Filed: "2026-02-01", Frame: "CY2025"},
							{Start: "2024-01-01", End: "2024-12-31", Val: 6.40, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01", Frame: "CY2024"},
							{Start: "2023-01-01", End: "2023-12-31", Val: 5.12, FY: 2023, FP: "FY", Form: "10-K", Filed: "2024-02-01", Frame: "CY2023"},
							{Start: "2022-01-01", End: "2022-12-31", Val: 4.00, FY: 2022, FP: "FY", Form: "10-K", Filed: "2023-02-01", Frame: "CY2022"},
						},
					},
				},
				"RevenueFromContractWithCustomerExcludingAssessedTax": {
					Units: map[string][]secFactValue{
						"USD": {
							{Start: "2025-01-01", End: "2025-03-31", Val: 100, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2025-05-01", Frame: "CY2025Q1"},
							{Start: "2024-01-01", End: "2024-03-31", Val: 80, FY: 2024, FP: "Q1", Form: "10-Q", Filed: "2024-05-01", Frame: "CY2024Q1"},
							{Start: "2025-01-01", End: "2025-12-31", Val: 420, FY: 2025, FP: "FY", Form: "10-K", Filed: "2026-02-01", Frame: "CY2025"},
							{Start: "2024-01-01", End: "2024-12-31", Val: 350, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01", Frame: "CY2024"},
						},
					},
				},
				"GrossProfit": {
					Units: map[string][]secFactValue{
						"USD": {
							{Start: "2025-01-01", End: "2025-03-31", Val: 45, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2025-05-01", Frame: "CY2025Q1"},
						},
					},
				},
			},
		},
	}

	eps := buildSECEPSQuarterly("AAPL", facts)
	if len(eps.Quarters) == 0 {
		t.Fatal("expected quarterly EPS rows")
	}
	if eps.Quarters[0].Period != "2025Q1" || eps.Quarters[0].EPSYoYGrowth != 25 || !eps.Quarters[0].DataSufficient {
		t.Fatalf("unexpected EPS quarter: %+v", eps.Quarters[0])
	}

	revenue := buildSECRevenueAnnual("AAPL", facts)
	if len(revenue.Years) == 0 {
		t.Fatal("expected annual revenue rows")
	}
	if revenue.Years[0].Year != 2025 || revenue.Years[0].RevenueYoYGrowth != 20 || !revenue.Years[0].DataSufficient {
		t.Fatalf("unexpected revenue year: %+v", revenue.Years[0])
	}

	margin := buildSECGrossMarginQuarterly("AAPL", facts)
	if len(margin.Quarters) == 0 {
		t.Fatal("expected gross margin rows")
	}
	if margin.Quarters[0].Period != "2025Q1" || margin.Quarters[0].GrossMargin != 45 {
		t.Fatalf("unexpected gross margin quarter: %+v", margin.Quarters[0])
	}

	annualGrowth, sufficient := buildSECAnnualEPSGrowth(facts)
	if len(annualGrowth) != 3 || !sufficient {
		t.Fatalf("expected 3 sufficient annual EPS growth rows, got %v sufficient=%v", annualGrowth, sufficient)
	}
	if annualGrowth[0] != 25 {
		t.Fatalf("unexpected annual EPS growth: %v", annualGrowth)
	}
}
