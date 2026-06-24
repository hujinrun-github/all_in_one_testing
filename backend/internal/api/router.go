package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status string `json:"status"`
}

func NewRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(handleDevCORS)
	router.Use(handleJSONPCallback)
	scenarioStore := newScenarioStoreFromEnv()
	runHistoryStore := newRunHistoryStoreFromEnv()
	agentStore := newAgentStoreFromEnv()
	targetStore := newTargetStoreFromEnv()
	profileArtifactStore := newProfileArtifactStoreFromEnv()
	profileTaskStore := newProfileTaskStoreFromEnv()
	profileTaskTemplateStore := newProfileTaskTemplateStoreFromEnv()
	runManager := newRunManager()
	router.Get("/api/health", handleHealth)
	router.Get("/api/agents", handleListAgents(agentStore))
	router.Get("/api/agent-metrics", handleListAgentMetrics(agentStore))
	router.Post("/api/agent-tokens", handleCreateAgentToken(agentStore))
	router.Post("/api/agent-tokens/{id}/revoke", handleRevokeAgentToken(agentStore))
	router.Post("/api/agent-tokens/{id}/rotate", handleRotateAgentToken(agentStore))
	router.Get("/api/targets", handleListTargets(targetStore))
	router.Post("/api/targets", handleCreateTarget(targetStore))
	router.Get("/api/targets/{id}", handleGetTarget(targetStore))
	router.Put("/api/targets/{id}", handleUpdateTarget(targetStore))
	router.Delete("/api/targets/{id}", handleDeleteTarget(targetStore))
	router.Post("/api/targets/{id}/health-check", handleTargetHealthCheck(targetStore))
	router.Get("/api/targets/{id}/health-checks", handleListTargetHealthChecks(targetStore))
	router.Get("/api/scenarios", handleListScenarios(scenarioStore))
	router.Post("/api/scenarios", handleCreateScenario(scenarioStore))
	router.Get("/api/scenarios/{id}", handleGetScenario(scenarioStore))
	router.Put("/api/scenarios/{id}", handleUpdateScenario(scenarioStore))
	router.Delete("/api/scenarios/{id}", handleDeleteScenario(scenarioStore))
	router.Get("/api/runs", handleListRuns(runHistoryStore))
	router.Post("/api/runs", handleCreateRun(scenarioStore, targetStore, agentStore, runHistoryStore, profileArtifactStore, profileTaskStore, runManager))
	router.Get("/api/runs/{id}", handleGetRun(runHistoryStore))
	router.Get("/api/runs/{id}/events", handleListRunEvents(runHistoryStore))
	router.Get("/api/runs/{id}/events/stream", handleStreamRunEvents(runHistoryStore))
	router.Post("/api/runs/{id}/stop", handleStopRun(runHistoryStore, runManager))
	router.Get("/api/reports/compare", handleGetRunComparisonReport(runHistoryStore))
	router.Get("/api/reports/profile-artifacts/compare", handleGetProfileArtifactComparisonReport(runHistoryStore, profileArtifactStore))
	router.Get("/api/reports/process-trends/compare", handleGetProcessTrendComparisonReport(runHistoryStore, targetStore, agentStore))
	router.Get("/api/reports/latest", handleGetLatestRunReport(runHistoryStore, profileArtifactStore, targetStore, agentStore))
	router.Get("/api/reports/runs/{id}", handleGetRunReport(runHistoryStore, profileArtifactStore, targetStore, agentStore))
	router.Get("/api/profile-artifacts", handleListProfileArtifacts(profileArtifactStore))
	router.Get("/api/profile-artifacts/{id}/download", handleDownloadProfileArtifact(profileArtifactStore))
	router.Get("/api/profile-task-templates", handleListProfileTaskTemplates(profileTaskTemplateStore))
	router.Post("/api/profile-task-templates", handleCreateProfileTaskTemplate(profileTaskTemplateStore))
	router.Put("/api/profile-task-templates/{id}", handleUpdateProfileTaskTemplate(profileTaskTemplateStore))
	router.Delete("/api/profile-task-templates/{id}", handleDeleteProfileTaskTemplate(profileTaskTemplateStore))
	router.Get("/api/profile-tasks", handleListProfileTasks(profileTaskStore))
	router.Post("/api/profile-tasks", handleCreateProfileTask(profileTaskStore))
	router.Post("/api/profile-tasks/{id}/retry", handleRetryProfileTask(profileTaskStore))
	router.Get("/agent/install.sh", handleAgentInstallScript)
	router.Get("/agent/binaries/{name}", handleAgentBinaryDownload)
	router.Post("/agent/v1/heartbeat", handleAgentHeartbeat(agentStore))
	router.Post("/agent/v1/metrics", handleAgentMetrics(agentStore))
	router.Get("/agent/v1/targets", handleAgentTargets(agentStore, targetStore))
	router.Post("/agent/v1/target-health-checks", handleAgentTargetHealthChecks(agentStore, targetStore))
	router.Post("/agent/v1/profile-artifacts", handleAgentProfileArtifactUpload(agentStore, profileArtifactStore))
	router.Get("/agent/v1/profile-tasks", handleAgentPollProfileTasks(agentStore, profileTaskStore))
	router.Post("/agent/v1/profile-tasks/{id}/complete", handleCompleteProfileTask(agentStore, profileTaskStore))
	return router
}

type jsonpCallbackResponseWriter struct {
	http.ResponseWriter
	callback string
}

func (w *jsonpCallbackResponseWriter) jsonpCallback() string {
	return w.callback
}

func handleJSONPCallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callback := r.URL.Query().Get("callback")
		if callback == "" || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		if !isSafeJSONPCallback(callback) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "callback must be a safe JavaScript function path"})
			return
		}

		next.ServeHTTP(&jsonpCallbackResponseWriter{ResponseWriter: w, callback: callback}, r)
	})
}

func isSafeJSONPCallback(callback string) bool {
	if callback == "" || len(callback) > 128 {
		return false
	}
	for _, part := range strings.Split(callback, ".") {
		if part == "" {
			return false
		}
		for index, char := range part {
			isLetter := (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
			isDigit := char >= '0' && char <= '9'
			if !(isLetter || char == '_' || char == '$' || (index > 0 && isDigit)) {
				return false
			}
			if index == 0 && isDigit {
				return false
			}
		}
	}
	return true
}

func handleDevCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedDevOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-AIT-Project-ID, X-AIT-Environment")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedDevOrigin(origin string) bool {
	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https" {
		return false
	}

	switch parsedOrigin.Hostname() {
	case "localhost", "127.0.0.1", "::1":
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
