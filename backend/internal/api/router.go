package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status string `json:"status"`
}

func NewRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(handleDevCORS)
	scenarioStore := newScenarioStoreFromEnv()
	router.Get("/api/health", handleHealth)
	router.Get("/api/scenarios", handleListScenarios(scenarioStore))
	router.Post("/api/scenarios", handleCreateScenario(scenarioStore))
	router.Get("/api/scenarios/{id}", handleGetScenario(scenarioStore))
	router.Put("/api/scenarios/{id}", handleUpdateScenario(scenarioStore))
	router.Delete("/api/scenarios/{id}", handleDeleteScenario(scenarioStore))
	router.Post("/api/runs", handleCreateRun(scenarioStore))
	return router
}

func handleDevCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedDevOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedDevOrigin(origin string) bool {
	switch origin {
	case "http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:5175",
		"http://127.0.0.1:5175":
		return true
	default:
		return false
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
