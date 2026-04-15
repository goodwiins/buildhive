package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildhive/buildhive/internal/api"
)

func TestHealthCheck(t *testing.T) {
	srv := api.New(api.Config{AdminTokenHash: "testhash"}, nil)
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestAdminAuthRejects(t *testing.T) {
	srv := api.New(api.Config{AdminTokenHash: "fakehash"}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}
