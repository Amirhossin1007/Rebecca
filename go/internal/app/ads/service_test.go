package ads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceFetchesNormalizesAndCachesAds(t *testing.T) {
	hits := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"header": [
				{"id": "h1", "title": "Header", "image_url": "", "link": "", "metadata": null}
			],
			"locales": {
				"fa": {
					"sidebar": [
						{"id": "s1", "type": "banner", "link": "https://example.com"}
					]
				}
			}
		}`))
	}))
	defer source.Close()

	service := NewService(source.URL, time.Hour, time.Second)
	first := service.Cached(context.Background())
	second := service.Cached(context.Background())

	if hits != 1 {
		t.Fatalf("source hits = %d, want 1", hits)
	}
	if len(first.Default.Header) != 1 || first.Default.Header[0].ID != "h1" {
		t.Fatalf("unexpected default header ads: %#v", first.Default.Header)
	}
	if first.Default.Header[0].ImageURL != nil || first.Default.Header[0].Link != nil {
		encoded, _ := json.Marshal(first.Default.Header[0])
		t.Fatalf("empty image/link should normalize to null/omit: %s", encoded)
	}
	if first.Default.Header[0].Type != "text" || first.Default.Header[0].Metadata == nil {
		t.Fatalf("default ad fields were not populated: %#v", first.Default.Header[0])
	}
	if len(first.Locales["fa"].Sidebar) != 1 || first.Locales["fa"].Sidebar[0].ID != "s1" {
		t.Fatalf("unexpected locale sidebar ads: %#v", first.Locales)
	}
	if len(second.Default.Header) != 1 || second.Default.Header[0].ID != "h1" {
		t.Fatalf("cached response changed unexpectedly: %#v", second)
	}
}

func TestServiceReturnsEmptyPayloadWhenSourceFails(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer source.Close()

	service := NewService(source.URL, time.Hour, time.Second)
	payload := service.Cached(context.Background())
	if len(payload.Default.Header) != 0 || len(payload.Default.Sidebar) != 0 || len(payload.Locales) != 0 {
		t.Fatalf("expected empty payload on source failure, got %#v", payload)
	}
}
