package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
	allowedHeaders := response.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowedHeaders, workspaceProjectHeader) || !strings.Contains(allowedHeaders, workspaceEnvironmentHeader) {
		t.Fatalf("expected workspace scope headers to be allowed, got %q", allowedHeaders)
	}
}

func TestRouterAllowsDynamicLocalDevCORSOrigin(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodOptions, "/api/agents", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5177")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusNoContent, response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5177" {
		t.Fatalf("expected dynamic local origin to be allowed, got %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}
