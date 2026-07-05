package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type TranslationClient struct {
	enabled  bool
	endpoint string
	client   *http.Client
}

func NewTranslationClient(enabled bool, timeout time.Duration) *TranslationClient {
	return &TranslationClient{
		enabled:  enabled,
		endpoint: "https://translate.googleapis.com/translate_a/single",
		client:   &http.Client{Timeout: timeout},
	}
}

func (c *TranslationClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *TranslationClient) TranslateToChinese(ctx context.Context, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if containsCJK(text) {
		return text, nil
	}
	if !c.Configured() {
		return text, errors.New("公司简介翻译未启用")
	}

	form := url.Values{}
	form.Set("client", "gtx")
	form.Set("sl", "auto")
	form.Set("tl", "zh-CN")
	form.Set("dt", "t")
	form.Set("q", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return text, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AlphaLens/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return text, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return text, fmt.Errorf("翻译请求失败：HTTP %d", resp.StatusCode)
	}

	var payload interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return text, err
	}
	translated := cleanDescription(parseGoogleTranslateText(payload))
	if translated == "" {
		return text, errors.New("翻译结果为空")
	}
	return translated, nil
}

func parseGoogleTranslateText(payload interface{}) string {
	root, ok := payload.([]interface{})
	if !ok || len(root) == 0 {
		return ""
	}
	segments, ok := root[0].([]interface{})
	if !ok {
		return ""
	}

	var builder strings.Builder
	for _, segment := range segments {
		values, ok := segment.([]interface{})
		if !ok || len(values) == 0 {
			continue
		}
		text, ok := values[0].(string)
		if !ok {
			continue
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func containsCJK(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
