package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_Get(t *testing.T) {
	r := New()
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test handler"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Body.String() != "test handler" {
		t.Fatalf("expected body %q, got %q", "test handler", rec.Body.String())
	}
}

func TestRouter_Parameter(t *testing.T) {
	r := New()
	r.Get("/user/:id", func(w http.ResponseWriter, r *http.Request) {
		id := GetParameter(r.Context(), "id")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user id: " + id))
	})

	req := httptest.NewRequest(http.MethodGet, "/user/123", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	expectedBody := "user id: 123"
	if rec.Body.String() != expectedBody {
		t.Fatalf("expected body %q, got %q", expectedBody, rec.Body.String())
	}
}

func TestRouter_NotFound(t *testing.T) {
	r := New()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}
