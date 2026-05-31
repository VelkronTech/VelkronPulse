package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/velkron/pulse/internal/metrics"
	"github.com/velkron/pulse/internal/services"
	"github.com/velkron/pulse/internal/store"
)

func testServer(t *testing.T, token string) *Server {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	dataStore, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { dataStore.Close() })

	hub := NewHub()
	go hub.Run()

	staticFS, err := fs.Sub(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><head></head><body>ok</body></html>")},
	}, ".")
	if err != nil {
		t.Fatalf("fs sub: %v", err)
	}

	return NewServer("127.0.0.1", 2024, token, hub, metrics.New(time.Second), services.New(time.Minute, nil, nil), dataStore, staticFS)
}

func TestAuthRequired(t *testing.T) {
	s := testServer(t, "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthBearerAccepted(t *testing.T) {
	s := testServer(t, "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHealthPublic(t *testing.T) {
	s := testServer(t, "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSettingsAllowlist(t *testing.T) {
	s := testServer(t, "secret-token")
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"key":"evil","value":"1"}`))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestIndexInjectsRuntimeConfig(t *testing.T) {
	s := testServer(t, "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "window.__PULSE_TOKEN__") || !strings.Contains(body, "window.__PULSE_VERSION__") {
		t.Fatalf("expected injected runtime config, got: %s", body)
	}
}
