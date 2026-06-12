package ads

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultSourceURL = "https://raw.githubusercontent.com/rebeccapanel/rebecca-ads/main/ads.json"

type Advertisement struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    *string        `json:"title,omitempty"`
	Text     *string        `json:"text,omitempty"`
	ImageURL *string        `json:"image_url,omitempty"`
	Link     *string        `json:"link,omitempty"`
	CTA      *string        `json:"cta,omitempty"`
	Metadata map[string]any `json:"metadata"`
}

type PlacementAds struct {
	Header  []Advertisement `json:"header"`
	Sidebar []Advertisement `json:"sidebar"`
}

type Response struct {
	Default PlacementAds            `json:"default"`
	Locales map[string]PlacementAds `json:"locales"`
}

type Service struct {
	sourceURL    string
	cacheTTL     time.Duration
	fetchTimeout time.Duration
	client       *http.Client

	mu          sync.Mutex
	payload     Response
	lastRefresh time.Time
	lastAttempt time.Time
	lastError   string
}

func NewService(sourceURL string, cacheTTL time.Duration, fetchTimeout time.Duration) *Service {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		sourceURL = defaultSourceURL
	}
	if cacheTTL <= 0 {
		cacheTTL = 24 * time.Hour
	}
	if fetchTimeout <= 0 {
		fetchTimeout = 15 * time.Second
	}
	return &Service{
		sourceURL:    sourceURL,
		cacheTTL:     cacheTTL,
		fetchTimeout: fetchTimeout,
		client:       &http.Client{Timeout: fetchTimeout},
		payload:      emptyResponse(),
	}
}

func (s *Service) Cached(ctx context.Context) Response {
	if s == nil {
		return emptyResponse()
	}
	return s.refresh(ctx, false)
}

func (s *Service) refresh(ctx context.Context, force bool) Response {
	s.mu.Lock()
	if !s.shouldRefreshLocked(force) {
		payload := s.payload
		s.mu.Unlock()
		return payload
	}
	s.lastAttempt = time.Now().UTC()
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.sourceURL, nil)
	if err != nil {
		return s.recordError(err)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return s.recordError(err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return s.recordError(&statusError{status: res.Status})
	}
	var raw map[string]any
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return s.recordError(err)
	}
	normalizeURLs(raw)
	payload := normalizeResponse(raw)

	s.mu.Lock()
	s.payload = payload
	s.lastRefresh = time.Now().UTC()
	s.lastError = ""
	s.mu.Unlock()
	return payload
}

func (s *Service) shouldRefreshLocked(force bool) bool {
	if force {
		return true
	}
	now := time.Now().UTC()
	if s.lastError != "" && !s.lastAttempt.IsZero() {
		minRetry := s.cacheTTL
		if minRetry > 5*time.Minute {
			minRetry = 5 * time.Minute
		}
		if minRetry < 30*time.Second {
			minRetry = 30 * time.Second
		}
		if now.Sub(s.lastAttempt) < minRetry {
			return false
		}
	}
	if s.lastRefresh.IsZero() {
		return true
	}
	return now.Sub(s.lastRefresh) >= s.cacheTTL
}

func (s *Service) recordError(err error) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAttempt = time.Now().UTC()
	s.lastError = err.Error()
	if s.payload.Locales == nil {
		s.payload = emptyResponse()
	}
	return s.payload
}

func normalizeResponse(raw map[string]any) Response {
	if raw == nil {
		return emptyResponse()
	}
	if _, ok := raw["default"]; !ok {
		defaultPayload := map[string]any{}
		if header, ok := raw["header"]; ok {
			defaultPayload["header"] = header
		}
		if sidebar, ok := raw["sidebar"]; ok {
			defaultPayload["sidebar"] = sidebar
		}
		if len(defaultPayload) > 0 {
			raw["default"] = defaultPayload
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return emptyResponse()
	}
	var response Response
	if err := json.Unmarshal(encoded, &response); err != nil {
		return emptyResponse()
	}
	if response.Locales == nil {
		response.Locales = map[string]PlacementAds{}
	}
	if response.Default.Header == nil {
		response.Default.Header = []Advertisement{}
	}
	if response.Default.Sidebar == nil {
		response.Default.Sidebar = []Advertisement{}
	}
	for i := range response.Default.Header {
		ensureAdDefaults(&response.Default.Header[i])
	}
	for i := range response.Default.Sidebar {
		ensureAdDefaults(&response.Default.Sidebar[i])
	}
	for locale, placement := range response.Locales {
		if placement.Header == nil {
			placement.Header = []Advertisement{}
		}
		if placement.Sidebar == nil {
			placement.Sidebar = []Advertisement{}
		}
		for i := range placement.Header {
			ensureAdDefaults(&placement.Header[i])
		}
		for i := range placement.Sidebar {
			ensureAdDefaults(&placement.Sidebar[i])
		}
		response.Locales[locale] = placement
	}
	return response
}

func ensureAdDefaults(ad *Advertisement) {
	if ad.Type == "" {
		ad.Type = "text"
	}
	if ad.Metadata == nil {
		ad.Metadata = map[string]any{}
	}
}

func normalizeURLs(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"image_url", "link"} {
			if raw, ok := typed[key].(string); ok && raw == "" {
				typed[key] = nil
			}
		}
		for _, child := range typed {
			normalizeURLs(child)
		}
	case []any:
		for _, child := range typed {
			normalizeURLs(child)
		}
	}
}

func emptyResponse() Response {
	return Response{
		Default: PlacementAds{
			Header:  []Advertisement{},
			Sidebar: []Advertisement{},
		},
		Locales: map[string]PlacementAds{},
	}
}

type statusError struct {
	status string
}

func (e *statusError) Error() string {
	return "ads source returned " + e.status
}
