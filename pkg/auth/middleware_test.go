package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := GetToken(r.Context(), "")
		if token != "test-token" {
			t.Errorf("expected token 'test-token', got '%s'", token)
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := TokenMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestTokenMiddleware_NoToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := GetToken(r.Context(), "default-token")
		if token != "default-token" {
			t.Errorf("expected token 'default-token', got '%s'", token)
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := TokenMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}
