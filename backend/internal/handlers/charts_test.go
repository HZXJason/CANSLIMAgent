package handlers

import "testing"

func TestNormalizeSymbolParamCanonicalizesAshareAtHTTPBoundary(t *testing.T) {
	tests := map[string]string{
		"600519":    "600519.SH",
		"600519.ss": "600519.SH",
		"sh.600519": "600519.SH",
		"bj-830799": "830799.BJ",
		"300750.sz": "300750.SZ",
		"920000.bj": "920000.BJ",
		"AAPL":      "AAPL",
		"brk.b":     "BRK.B",
		"../AAPL":   "",
		"123456":    "",
		"123456.sz": "",
		"900901.sh": "",
		"200002.sz": "",
		"sz600519":  "",
		"sz.600519": "",
		"sh-000001": "",
	}
	for input, want := range tests {
		if got := normalizeSymbolParam(input); got != want {
			t.Errorf("normalizeSymbolParam(%q) = %q, want %q", input, got, want)
		}
	}
}
