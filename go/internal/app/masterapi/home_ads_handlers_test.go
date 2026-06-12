//go:build cgo

package masterapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	adminapp "github.com/rebeccapanel/rebecca/go/internal/app/admin"
	adsapp "github.com/rebeccapanel/rebecca/go/internal/app/ads"
)

func TestHomeRouteServesHomeTemplateFromGo(t *testing.T) {
	server, db := testAdminServer(t)
	createSettingsTables(t, db)

	defaultTemplates := filepath.Join(t.TempDir(), "app-templates")
	if err := os.MkdirAll(filepath.Join(defaultTemplates, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultTemplates, "home", "index.html"), []byte("<html>Rebecca Home</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REBECCA_APP_TEMPLATE_BASE", defaultTemplates)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	if rec.Body.String() != "<html>Rebecca Home</html>" {
		t.Fatalf("unexpected home body: %s", rec.Body.String())
	}
}

func TestAdsRouteServesCachedAdsFromGo(t *testing.T) {
	server, db := testAdminServer(t)
	insertMasterAPIAdmin(t, db, 1, "pouria", "pass123", adminapp.RoleFullAccess, adminapp.StatusActive)
	token := adminBearerToken(t, server, "pouria", "pass123")

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"header":[{"id":"h1","image_url":"","link":""}]}`))
	}))
	defer source.Close()
	server.adsService = adsapp.NewService(source.URL, time.Hour, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/api/ads", nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Default struct {
			Header []struct {
				ID string `json:"id"`
			} `json:"header"`
			Sidebar []any `json:"sidebar"`
		} `json:"default"`
		Locales map[string]any `json:"locales"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Default.Header) != 1 || payload.Default.Header[0].ID != "h1" {
		t.Fatalf("unexpected ads payload: %#v", payload)
	}
}
