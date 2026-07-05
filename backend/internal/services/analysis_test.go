package services

import (
	"fmt"
	"testing"
)

func TestRSScoreStaysContinuousAtExtremes(t *testing.T) {
	strong := rsScore(90, 20)
	stronger := rsScore(110, 20)
	if strong >= 99 || stronger >= 99 {
		t.Fatalf("expected strong scores to stay below the upper bound, got %.2f and %.2f", strong, stronger)
	}
	if stronger <= strong {
		t.Fatalf("expected stronger relative return to produce higher score, got %.2f then %.2f", strong, stronger)
	}

	weak := rsScore(-70, 20)
	weaker := rsScore(-90, 20)
	if weak <= 1 || weaker <= 1 {
		t.Fatalf("expected weak scores to stay above the lower bound, got %.2f and %.2f", weak, weaker)
	}
	if weaker >= weak {
		t.Fatalf("expected weaker relative return to produce lower score, got %.2f then %.2f", weak, weaker)
	}

	extremelyStrong := rsScore(600, 0)
	extremelyWeak := rsScore(-600, 0)
	if extremelyStrong >= 99 || extremelyWeak <= 1 {
		t.Fatalf("expected extreme scores to avoid flat chart bounds, got %.2f and %.2f", extremelyStrong, extremelyWeak)
	}
}

func TestBuildRSLineDoesNotFlattenExtremeRuns(t *testing.T) {
	stock := make([]Candle, 0, 60)
	benchmark := make([]Candle, 0, 60)
	for i := 0; i < 60; i++ {
		date := fmt.Sprintf("2025-W%02d", i+1)
		stockClose := 100.0
		if i >= 52 {
			stockClose = 180 + float64(i-52)*5
		}
		stock = append(stock, Candle{Date: date, Close: stockClose})
		benchmark = append(benchmark, Candle{Date: date, Close: 100})
	}

	line := buildRSLine(stock, benchmark, PeriodWeekly)
	if len(line) != 8 {
		t.Fatalf("expected 8 weekly RS points, got %d", len(line))
	}
	if line[0].Value >= line[len(line)-1].Value {
		t.Fatalf("expected rising relative strength to remain visible, got %.2f then %.2f", line[0].Value, line[len(line)-1].Value)
	}
	for _, point := range line {
		if point.Value <= 1 || point.Value >= 99 {
			t.Fatalf("expected smoothed score inside bounds, got %+v", point)
		}
	}
}
