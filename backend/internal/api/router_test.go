package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterAllowsDevCORSPreflight(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodOptions, "/api/scenarios", nil)
	request.Header.Set("Origin", "http://localhost:5175")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusNoContent, response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5175" {
		t.Fatalf("expected localhost origin to be allowed, got %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
	if response.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected allowed methods header")
	}
	if response.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("expected allowed headers header")
	}
}
