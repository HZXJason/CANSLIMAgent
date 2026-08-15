package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResearchCalendarResponseBytes = 2 << 20

type CalendarSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type MarketCalendarEvent struct {
	Date       time.Time
	Time       string
	Kind       string
	Title      string
	Subtitle   string
	Symbol     string
	Importance string
	Source     string
}

type CalendarFetchResult struct {
	Events  []MarketCalendarEvent
	Sources []CalendarSource
}

type HTTPResearchCalendarClient struct {
	endpoint string
	client   *http.Client
	secret   string
}

func NewHTTPResearchCalendarClient(upstreamURL string, timeout time.Duration, sharedSecret string) *HTTPResearchCalendarClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	base := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if base == "" {
		base = DefaultAIAnalysisUpstreamURL
	}
	if !strings.HasSuffix(base, "/calendar/week") {
		base += "/calendar/week"
	}
	return &HTTPResearchCalendarClient{endpoint: base, client: &http.Client{Timeout: timeout}, secret: strings.TrimSpace(sharedSecret)}
}

func (c *HTTPResearchCalendarClient) Configured() bool {
	return c != nil && c.endpoint != ""
}

func (c *HTTPResearchCalendarClient) FetchWeekEvents(ctx context.Context, market string, from, to time.Time, symbols []string) (CalendarFetchResult, error) {
	wire, err := json.Marshal(map[string]any{
		"market": market, "week_start": from.Format("2006-01-02"), "week_end": to.Format("2006-01-02"), "symbols": symbols,
	})
	if err != nil {
		return CalendarFetchResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(wire))
	if err != nil {
		return CalendarFetchResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if c.secret != "" {
		request.Header.Set("X-Sidecar-Token", c.secret)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return CalendarFetchResult{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResearchCalendarResponseBytes+1))
	if err != nil {
		return CalendarFetchResult{}, err
	}
	if len(body) > maxResearchCalendarResponseBytes {
		return CalendarFetchResult{}, errors.New("research calendar response exceeds size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CalendarFetchResult{}, fmt.Errorf("research calendar upstream returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Events []struct {
			Date       string `json:"date"`
			Time       string `json:"time"`
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Subtitle   string `json:"subtitle"`
			Symbol     string `json:"symbol"`
			Importance string `json:"importance"`
			Source     string `json:"source"`
		} `json:"events"`
		Sources []CalendarSource `json:"sources"`
		Errors  []string         `json:"errors"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return CalendarFetchResult{}, errors.New("research calendar response is invalid")
	}
	result := CalendarFetchResult{Events: make([]MarketCalendarEvent, 0, len(payload.Events)), Sources: payload.Sources}
	for _, event := range payload.Events {
		date, err := time.Parse("2006-01-02", event.Date)
		if err != nil || strings.TrimSpace(event.Kind) == "" || strings.TrimSpace(event.Title) == "" {
			continue
		}
		result.Events = append(result.Events, MarketCalendarEvent{Date: date, Time: event.Time, Kind: event.Kind, Title: event.Title, Subtitle: event.Subtitle, Symbol: event.Symbol, Importance: event.Importance, Source: event.Source})
	}
	if len(payload.Errors) > 0 {
		return result, errors.New(strings.Join(payload.Errors, "; "))
	}
	return result, nil
}
