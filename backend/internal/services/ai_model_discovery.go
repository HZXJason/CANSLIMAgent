package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxModelDiscoveryResponseBytes = 2 << 20

type AIModelDiscoveryRequest struct {
	Provider   string `json:"provider"`
	BackendURL string `json:"backend_url"`
	APIKey     string `json:"api_key"`
}

type AIModelDiscoveryResponse struct {
	Models []string `json:"models"`
	Source string   `json:"source"`
}

type AIModelDiscoveryService struct {
	client *http.Client
}

type AIModelDiscoverer interface {
	Discover(context.Context, AIModelDiscoveryRequest) (AIModelDiscoveryResponse, error)
}

func NewAIModelDiscoveryService(timeout time.Duration) *AIModelDiscoveryService {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &AIModelDiscoveryService{client: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}}
}

func (s *AIModelDiscoveryService) Discover(ctx context.Context, input AIModelDiscoveryRequest) (AIModelDiscoveryResponse, error) {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	modelsURL, err := resolveAIModelsURL(provider, input.BackendURL)
	if err != nil {
		return AIModelDiscoveryResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL.String(), nil)
	if err != nil {
		return AIModelDiscoveryResponse{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CANSLIMAgent/1.0 model-discovery")
	apiKey := strings.TrimSpace(input.APIKey)
	switch provider {
	case "anthropic":
		if apiKey != "" {
			request.Header.Set("x-api-key", apiKey)
		}
		request.Header.Set("anthropic-version", "2023-06-01")
	case "google":
		if apiKey != "" {
			request.Header.Set("x-goog-api-key", apiKey)
		}
	default:
		if apiKey != "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	response, err := s.client.Do(request)
	if err != nil {
		return AIModelDiscoveryResponse{}, fmt.Errorf("model endpoint request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		message := strings.TrimSpace(string(detail))
		if len(message) > 500 {
			message = message[:500]
		}
		return AIModelDiscoveryResponse{}, fmt.Errorf("model endpoint returned HTTP %d: %s", response.StatusCode, message)
	}

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxModelDiscoveryResponseBytes))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return AIModelDiscoveryResponse{}, fmt.Errorf("model endpoint returned invalid JSON: %w", err)
	}
	models := extractAIModelIDs(payload, provider)
	if len(models) == 0 {
		return AIModelDiscoveryResponse{}, errors.New("model endpoint returned no model IDs")
	}
	return AIModelDiscoveryResponse{Models: models, Source: modelsURL.Host}, nil
}

func resolveAIModelsURL(provider, backendURL string) (*url.URL, error) {
	raw := strings.TrimSpace(backendURL)
	if raw == "" {
		switch provider {
		case "openai":
			raw = "https://api.openai.com/v1"
		case "anthropic":
			raw = "https://api.anthropic.com/v1"
		case "google":
			raw = "https://generativelanguage.googleapis.com/v1beta"
		default:
			return nil, errors.New("backend_url is required for an OpenAI-compatible provider")
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("backend_url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("backend_url must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("backend_url must include a host and must not contain credentials")
	}

	cleanPath := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{"/chat/completions", "/responses", "/models"} {
		if strings.HasSuffix(cleanPath, suffix) {
			cleanPath = strings.TrimSuffix(cleanPath, suffix)
			break
		}
	}
	if cleanPath == "" {
		cleanPath = "/v1"
	}
	parsed.Path = cleanPath + "/models"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func extractAIModelIDs(payload any, provider string) []string {
	var candidates []any
	switch value := payload.(type) {
	case []any:
		candidates = value
	case map[string]any:
		for _, key := range []string{"data", "models", "result"} {
			if rows, ok := value[key].([]any); ok {
				candidates = rows
				break
			}
		}
	}
	seen := make(map[string]struct{})
	models := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		modelID := ""
		switch value := candidate.(type) {
		case string:
			modelID = value
		case map[string]any:
			for _, key := range []string{"id", "name", "model"} {
				if raw, ok := value[key].(string); ok && strings.TrimSpace(raw) != "" {
					modelID = raw
					break
				}
			}
		}
		modelID = strings.TrimSpace(modelID)
		if provider == "google" {
			modelID = strings.TrimPrefix(modelID, "models/")
		}
		if modelID == "" {
			continue
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		seen[modelID] = struct{}{}
		models = append(models, modelID)
	}
	sort.Strings(models)
	if len(models) > 500 {
		models = models[:500]
	}
	return models
}
