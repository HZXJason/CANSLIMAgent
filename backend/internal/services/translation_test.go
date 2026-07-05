package services

import "testing"

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
