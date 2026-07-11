package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseGoogleTranslateText(t *testing.T) {
	payload := []interface{}{
		[]interface{}{
			[]interface{}{"苹果公司设计、制造和销售智能手机", "Apple Inc. designs, manufactures, and markets smartphones"},
			[]interface{}{"和个人电脑。", "and personal computers."},
		},
	}

	got := parseGoogleTranslateText(payload)
	want := "苹果公司设计、制造和销售智能手机和个人电脑。"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestContainsCJK(t *testing.T) {
	if !containsCJK("苹果公司 designs phones") {
		t.Fatal("expected mixed Chinese text to be detected as CJK")
	}
	if containsCJK("Apple Inc. designs phones") {
		t.Fatal("expected English text not to be detected as CJK")
	}
}

func TestTranslateToChineseFallsBackWhenGoogleIsBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/google":
			http.Error(w, "blocked", http.StatusTooManyRequests)
		case "/libre":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"translatedText":"美光科技"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTranslationClient(true, 0)
	client.endpoint = server.URL + "/google"
	client.libreEndpoint = server.URL + "/libre"
	translated, err := client.TranslateToChinese(context.Background(), "Micron Technology")
	if err != nil {
		t.Fatalf("TranslateToChinese returned error: %v", err)
	}
	if translated != "美光科技" {
		t.Fatalf("unexpected fallback translation: %q", translated)
	}
}

func TestSplitTranslationTextKeepsChunksWithinLimit(t *testing.T) {
	text := "one two three four five six seven"
	chunks := splitTranslationText(text, 12)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 12 {
			t.Fatalf("chunk exceeds limit: %q", chunk)
		}
	}
}
