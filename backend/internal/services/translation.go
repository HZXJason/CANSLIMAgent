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
	enabled          bool
	endpoint         string
	libreEndpoint    string
	fallbackEndpoint string
	client           *http.Client
}

func NewTranslationClient(enabled bool, timeout time.Duration) *TranslationClient {
	return &TranslationClient{
		enabled:          enabled,
		endpoint:         "https://translate.googleapis.com/translate_a/single",
		libreEndpoint:    "https://translate.fedilab.app/translate",
		fallbackEndpoint: "https://api.mymemory.translated.net/get",
		client:           &http.Client{Timeout: timeout},
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

	if translated, err := c.translateLibre(ctx, text); err == nil {
		return translated, nil
	}
	if translated, err := c.translateMyMemory(ctx, text); err == nil {
		return translated, nil
	}
	if translated, err := c.translateGoogle(ctx, text); err == nil {
		return translated, nil
	}
	return text, errors.New("公司简介翻译服务暂不可用")
}

func (c *TranslationClient) translateLibre(ctx context.Context, text string) (string, error) {
	chunks := splitTranslationText(text, 600)
	translations := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		payload, err := json.Marshal(map[string]string{
			"q":      chunk,
			"source": "en",
			"target": "zh",
			"format": "text",
		})
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.libreEndpoint, strings.NewReader(string(payload)))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "AlphaLens/1.0")
		resp, err := c.client.Do(req)
		if err != nil {
			return "", err
		}
		var body struct {
			TranslatedText string `json:"translatedText"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("LibreTranslate 请求失败：HTTP %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return "", decodeErr
		}
		translated := cleanDescription(body.TranslatedText)
		if translated == "" || !containsCJK(translated) {
			return "", errors.New("LibreTranslate 返回空结果")
		}
		translations = append(translations, translated)
	}
	return cleanDescription(strings.Join(translations, "")), nil
}

func (c *TranslationClient) translateGoogle(ctx context.Context, text string) (string, error) {
	form := url.Values{}
	form.Set("client", "gtx")
	form.Set("sl", "auto")
	form.Set("tl", "zh-CN")
	form.Set("dt", "t")
	form.Set("q", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AlphaLens/1.0")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google Translate 请求失败：HTTP %d", resp.StatusCode)
	}
	var payload interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	translated := cleanDescription(parseGoogleTranslateText(payload))
	if translated == "" {
		return "", errors.New("Google Translate 返回空结果")
	}
	return translated, nil
}

func (c *TranslationClient) translateMyMemory(ctx context.Context, text string) (string, error) {
	chunks := splitTranslationText(text, 400)
	translations := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		translated, err := c.translateMyMemoryChunk(ctx, chunk)
		if err != nil {
			return "", err
		}
		translations = append(translations, translated)
	}
	return cleanDescription(strings.Join(translations, "")), nil
}

func (c *TranslationClient) translateMyMemoryChunk(ctx context.Context, text string) (string, error) {
	query := url.Values{}
	query.Set("q", text)
	query.Set("langpair", "en|zh-CN")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fallbackEndpoint+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AlphaLens/1.0")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("MyMemory 翻译请求失败：HTTP %d", resp.StatusCode)
	}
	var payload struct {
		ResponseStatus int `json:"responseStatus"`
		ResponseData   struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	translated := cleanDescription(payload.ResponseData.TranslatedText)
	if payload.ResponseStatus != 200 || translated == "" || !containsCJK(translated) {
		return "", errors.New("MyMemory 翻译返回空结果")
	}
	return translated, nil
}

func splitTranslationText(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 || len([]rune(text)) <= maxRunes {
		return []string{text}
	}

	chunks := make([]string, 0, len(text)/maxRunes+1)
	words := strings.Fields(text)
	var builder strings.Builder
	current := 0
	for _, word := range words {
		wordRunes := []rune(word)
		if current > 0 && current+1+len(wordRunes) > maxRunes {
			chunks = append(chunks, builder.String())
			builder.Reset()
			current = 0
		}
		if current > 0 {
			builder.WriteByte(' ')
			current++
		}
		for len(wordRunes) > maxRunes {
			if current > 0 {
				chunks = append(chunks, builder.String())
				builder.Reset()
				current = 0
			}
			chunks = append(chunks, string(wordRunes[:maxRunes]))
			wordRunes = wordRunes[maxRunes:]
		}
		builder.WriteString(string(wordRunes))
		current += len(wordRunes)
	}
	if builder.Len() > 0 {
		chunks = append(chunks, builder.String())
	}
	return chunks
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
