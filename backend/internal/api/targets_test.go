package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTargetPersistsAgentBinding(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createScopedAgentToken(t, router, "checkout-prod-install", "default", "prod")

	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.13",
		"version": "0.2.0",
		"labels": {
			"service": "checkout",
			"zone": "shanghai-b"
		},
		"capabilities": ["host_metrics", "process_metrics", "pprof"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}
	var registeredAgent struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(heartbeatResponse.Body).Decode(&registeredAgent); err != nil {
		t.Fatalf("expected heartbeat JSON, got decode error: %v", err)
	}

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "prod",
		"agentIds": ["`+registeredAgent.ID+`"],
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof",
		"processMatch": {
			"name": "checkout",
			"cmdlineContains": "--config=/etc/checkout/config.yaml"
		},
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1500
		},
		"metricThresholds": {
			"cpuMaxPercent": 70,
			"memoryMaxPercent": 60,
			"diskReadMaxBytesPerSec": 4096
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		BaseURL         string   `json:"baseUrl"`
		Environment     string   `json:"environment"`
		AgentIDs        []string `json:"agentIds"`
		ProfileEndpoint string   `json:"profileEndpoint"`
		ProcessMatch    struct {
			Name            string `json:"name"`
			CmdlineContains string `json:"cmdlineContains"`
		} `json:"processMatch"`
		HealthCheck struct {
			Enabled        bool   `json:"enabled"`
			Path           string `json:"path"`
			ExpectedStatus int    `json:"expectedStatus"`
			TimeoutMs      int    `json:"timeoutMs"`
		} `json:"healthCheck"`
		MetricThresholds struct {
			CPUMaxPercent          float64 `json:"cpuMaxPercent"`
			MemoryMaxPercent       float64 `json:"memoryMaxPercent"`
			DiskReadMaxBytesPerSec float64 `json:"diskReadMaxBytesPerSec"`
		} `json:"metricThresholds"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}
	if created.ID == "" || created.CreatedAt == "" {
		t.Fatalf("expected generated target id and createdAt, got %#v", created)
	}
	if created.Name != "checkout-service" || created.BaseURL != "http://checkout.internal:8080" {
		t.Fatalf("expected created target fields to round-trip, got %#v", created)
	}
	if len(created.AgentIDs) != 1 || created.AgentIDs[0] != registeredAgent.ID {
		t.Fatalf("expected target to bind agent %q, got %#v", registeredAgent.ID, created.AgentIDs)
	}
	if created.ProcessMatch.Name != "checkout" || created.ProcessMatch.CmdlineContains == "" {
		t.Fatalf("expected process match to round-trip, got %#v", created.ProcessMatch)
	}
	if !created.HealthCheck.Enabled || created.HealthCheck.Path != "/ready" || created.HealthCheck.ExpectedStatus != 204 || created.HealthCheck.TimeoutMs != 1500 {
		t.Fatalf("expected health check to round-trip, got %#v", created.HealthCheck)
	}
	if created.MetricThresholds.CPUMaxPercent != 70 || created.MetricThresholds.MemoryMaxPercent != 60 || created.MetricThresholds.DiskReadMaxBytesPerSec != 4096 {
		t.Fatalf("expected metric thresholds to round-trip, got %#v", created.MetricThresholds)
	}

	restartedRouter := NewRouter()
	listResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		AgentIDs         []string `json:"agentIds"`
		ProfileEndpoint  string   `json:"profileEndpoint"`
		MetricThresholds struct {
			CPUMaxPercent float64 `json:"cpuMaxPercent"`
		} `json:"metricThresholds"`
		HealthCheck struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
		} `json:"healthCheck"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one persisted target, got %#v", targets)
	}
	if targets[0].ID != created.ID || targets[0].AgentIDs[0] != registeredAgent.ID {
		t.Fatalf("expected persisted target binding %#v, got %#v", created, targets[0])
	}
	if targets[0].MetricThresholds.CPUMaxPercent != 70 {
		t.Fatalf("expected persisted metric thresholds, got %#v", targets[0].MetricThresholds)
	}
	if !targets[0].HealthCheck.Enabled || targets[0].HealthCheck.Path != "/ready" {
		t.Fatalf("expected persisted health check, got %#v", targets[0].HealthCheck)
	}

	getResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+created.ID, nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected target get status %d, got %d with body %s", http.StatusOK, getResponse.Code, getResponse.Body.String())
	}
}

func TestTargetAPIStoresAndFiltersProjectEnvironment(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createTarget := func(body string) struct {
		ID          string `json:"id"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	} {
		t.Helper()

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
		}
		var created struct {
			ID          string `json:"id"`
			ProjectID   string `json:"projectId"`
			Environment string `json:"environment"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			t.Fatalf("expected target JSON, got decode error: %v", err)
		}
		return created
	}

	checkout := createTarget(`{
		"name": "checkout-staging",
		"projectId": "project-checkout",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "staging"
	}`)
	createTarget(`{
		"name": "billing-prod",
		"projectId": "project-billing",
		"baseUrl": "http://billing.internal:8080",
		"environment": "prod"
	}`)

	if checkout.ProjectID != "project-checkout" || checkout.Environment != "staging" {
		t.Fatalf("expected created target to preserve project/environment, got %#v", checkout)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/targets?projectId=project-checkout&environment=staging", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []struct {
		ID          string `json:"id"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one matching target, got %d: %#v", len(targets), targets)
	}
	if targets[0].ID != checkout.ID || targets[0].ProjectID != "project-checkout" || targets[0].Environment != "staging" {
		t.Fatalf("expected filtered checkout staging target, got %#v", targets[0])
	}
}

func TestTargetRejectsUnknownAgentBinding(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "http://checkout.internal:8080",
		"agentIds": ["agent-missing"]
	}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown agent binding status %d, got %d with body %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func TestTargetHealthCheckEndpointReportsHealthyStatus(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Fatalf("expected health check path /ready, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	router := NewRouter()
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "`+upstream.URL+`",
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1000
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	checkResponse := httptest.NewRecorder()
	router.ServeHTTP(checkResponse, httptest.NewRequest(http.MethodPost, "/api/targets/"+created.ID+"/health-check", nil))
	if checkResponse.Code != http.StatusOK {
		t.Fatalf("expected health check status %d, got %d with body %s", http.StatusOK, checkResponse.Code, checkResponse.Body.String())
	}
	var result struct {
		TargetID       string  `json:"targetId"`
		TargetName     string  `json:"targetName"`
		Status         string  `json:"status"`
		URL            string  `json:"url"`
		ExpectedStatus int     `json:"expectedStatus"`
		ObservedStatus int     `json:"observedStatus"`
		LatencyMs      float64 `json:"latencyMs"`
		Error          string  `json:"error"`
		CheckedAt      string  `json:"checkedAt"`
		Source         string  `json:"source"`
	}
	if err := json.NewDecoder(checkResponse.Body).Decode(&result); err != nil {
		t.Fatalf("expected health check JSON, got decode error: %v", err)
	}
	if result.TargetID != created.ID || result.TargetName != "checkout-service" {
		t.Fatalf("expected target identity in result, got %#v", result)
	}
	if result.Status != "healthy" || result.ObservedStatus != http.StatusNoContent || result.ExpectedStatus != http.StatusNoContent {
		t.Fatalf("expected healthy 204 result, got %#v", result)
	}
	if result.URL != upstream.URL+"/ready" || result.CheckedAt == "" || result.LatencyMs < 0 || result.Error != "" || result.Source != "control_plane" {
		t.Fatalf("expected health check metadata, got %#v", result)
	}
}

func TestTargetHealthCheckEndpointPersistsLatestResult(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	router := NewRouter()
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "`+upstream.URL+`",
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1000
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	checkResponse := httptest.NewRecorder()
	router.ServeHTTP(checkResponse, httptest.NewRequest(http.MethodPost, "/api/targets/"+created.ID+"/health-check", nil))
	if checkResponse.Code != http.StatusOK {
		t.Fatalf("expected health check status %d, got %d with body %s", http.StatusOK, checkResponse.Code, checkResponse.Body.String())
	}
	var checked targetHealthCheckResult
	if err := json.NewDecoder(checkResponse.Body).Decode(&checked); err != nil {
		t.Fatalf("expected health check JSON, got decode error: %v", err)
	}

	restartedRouter := NewRouter()
	listResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []struct {
		ID              string                  `json:"id"`
		LastHealthCheck targetHealthCheckResult `json:"lastHealthCheck"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %#v", targets)
	}
	if targets[0].ID != created.ID {
		t.Fatalf("expected persisted target %q, got %#v", created.ID, targets[0])
	}
	if targets[0].LastHealthCheck.Status != "healthy" ||
		targets[0].LastHealthCheck.ObservedStatus != http.StatusNoContent ||
		targets[0].LastHealthCheck.URL != upstream.URL+"/ready" ||
		targets[0].LastHealthCheck.CheckedAt != checked.CheckedAt ||
		targets[0].LastHealthCheck.Source != "control_plane" {
		t.Fatalf("expected latest health result to persist, checked=%#v listed=%#v", checked, targets[0].LastHealthCheck)
	}
}

func TestTargetHealthCheckHistoryEndpointReturnsRecentChecks(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	statuses := []int{http.StatusNoContent, http.StatusServiceUnavailable, http.StatusNoContent}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := statuses[0]
		if len(statuses) > 1 {
			statuses = statuses[1:]
		}
		w.WriteHeader(status)
	}))
	defer upstream.Close()

	router := NewRouter()
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "`+upstream.URL+`",
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1000
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	for i := 0; i < 3; i++ {
		checkResponse := httptest.NewRecorder()
		router.ServeHTTP(checkResponse, httptest.NewRequest(http.MethodPost, "/api/targets/"+created.ID+"/health-check", nil))
		if checkResponse.Code != http.StatusOK {
			t.Fatalf("expected health check status %d, got %d with body %s", http.StatusOK, checkResponse.Code, checkResponse.Body.String())
		}
	}

	restartedRouter := NewRouter()
	historyResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+created.ID+"/health-checks?limit=2", nil))
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("expected health check history status %d, got %d with body %s", http.StatusOK, historyResponse.Code, historyResponse.Body.String())
	}
	var history []targetHealthCheckResult
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatalf("expected health check history JSON, got decode error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected two recent history records, got %#v", history)
	}
	if history[0].TargetID != created.ID || history[1].TargetID != created.ID {
		t.Fatalf("expected target id in history records, got %#v", history)
	}
	if history[0].Status != "healthy" || history[0].ObservedStatus != http.StatusNoContent || history[0].Source != "control_plane" {
		t.Fatalf("expected latest health check first, got %#v", history)
	}
	if history[1].Status != "unhealthy" || history[1].ObservedStatus != http.StatusServiceUnavailable || history[1].Error == "" || history[1].Source != "control_plane" {
		t.Fatalf("expected previous unhealthy check second, got %#v", history)
	}
}

func TestTargetHealthCheckEndpointReportsUnhealthyStatus(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	router := NewRouter()
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "`+upstream.URL+`",
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 200,
			"timeoutMs": 1000
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	checkResponse := httptest.NewRecorder()
	router.ServeHTTP(checkResponse, httptest.NewRequest(http.MethodPost, "/api/targets/"+created.ID+"/health-check", nil))
	if checkResponse.Code != http.StatusOK {
		t.Fatalf("expected health check status %d, got %d with body %s", http.StatusOK, checkResponse.Code, checkResponse.Body.String())
	}
	var result struct {
		Status         string `json:"status"`
		ExpectedStatus int    `json:"expectedStatus"`
		ObservedStatus int    `json:"observedStatus"`
		Error          string `json:"error"`
	}
	if err := json.NewDecoder(checkResponse.Body).Decode(&result); err != nil {
		t.Fatalf("expected health check JSON, got decode error: %v", err)
	}
	if result.Status != "unhealthy" || result.ExpectedStatus != http.StatusOK || result.ObservedStatus != http.StatusServiceUnavailable || result.Error == "" {
		t.Fatalf("expected unhealthy status result, got %#v", result)
	}
}

func TestUpdateTargetPersistsChanges(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "staging",
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof",
		"processMatch": {
			"name": "checkout",
			"cmdlineContains": "--config=/etc/checkout/config.yaml"
		}
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created targetRecord
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/api/targets/"+created.ID, bytes.NewBufferString(`{
		"name": "checkout-service-prod",
		"baseUrl": "http://checkout.prod.internal:8080",
		"environment": "prod",
		"profileEndpoint": "http://127.0.0.1:7070/debug/pprof",
		"processMatch": {
			"name": "checkout-prod",
			"cmdlineContains": "--config=/etc/checkout/prod.yaml"
		},
		"metricThresholds": {
			"cpuMaxPercent": 80,
			"memoryMaxPercent": 75
		}
	}`)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected target update status %d, got %d with body %s", http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	}
	var updated struct {
		ID               string             `json:"id"`
		Name             string             `json:"name"`
		BaseURL          string             `json:"baseUrl"`
		Environment      string             `json:"environment"`
		ProfileEndpoint  string             `json:"profileEndpoint"`
		ProcessMatch     targetProcessMatch `json:"processMatch"`
		CreatedAt        string             `json:"createdAt"`
		MetricThresholds struct {
			CPUMaxPercent    float64 `json:"cpuMaxPercent"`
			MemoryMaxPercent float64 `json:"memoryMaxPercent"`
		} `json:"metricThresholds"`
	}
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatalf("expected updated target JSON, got decode error: %v", err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("expected update to preserve id and createdAt, created=%#v updated=%#v", created, updated)
	}
	if updated.Name != "checkout-service-prod" || updated.BaseURL != "http://checkout.prod.internal:8080" {
		t.Fatalf("expected updated target fields to round-trip, got %#v", updated)
	}
	if updated.Environment != "prod" || updated.ProfileEndpoint != "http://127.0.0.1:7070/debug/pprof" {
		t.Fatalf("expected updated target metadata, got %#v", updated)
	}
	if updated.ProcessMatch.Name != "checkout-prod" || updated.ProcessMatch.CmdlineContains != "--config=/etc/checkout/prod.yaml" {
		t.Fatalf("expected updated process match, got %#v", updated.ProcessMatch)
	}
	if updated.MetricThresholds.CPUMaxPercent != 80 || updated.MetricThresholds.MemoryMaxPercent != 75 {
		t.Fatalf("expected updated metric thresholds, got %#v", updated.MetricThresholds)
	}

	restartedRouter := NewRouter()
	getResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+created.ID, nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected target get status %d, got %d with body %s", http.StatusOK, getResponse.Code, getResponse.Body.String())
	}
	var persisted struct {
		Name             string             `json:"name"`
		BaseURL          string             `json:"baseUrl"`
		ProcessMatch     targetProcessMatch `json:"processMatch"`
		MetricThresholds struct {
			CPUMaxPercent    float64 `json:"cpuMaxPercent"`
			MemoryMaxPercent float64 `json:"memoryMaxPercent"`
		} `json:"metricThresholds"`
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&persisted); err != nil {
		t.Fatalf("expected persisted target JSON, got decode error: %v", err)
	}
	if persisted.Name != updated.Name || persisted.BaseURL != updated.BaseURL || persisted.ProcessMatch.Name != updated.ProcessMatch.Name {
		t.Fatalf("expected persisted update %#v, got %#v", updated, persisted)
	}
	if persisted.MetricThresholds.CPUMaxPercent != 80 || persisted.MetricThresholds.MemoryMaxPercent != 75 {
		t.Fatalf("expected persisted metric thresholds, got %#v", persisted.MetricThresholds)
	}
}

func TestDeleteTargetRemovesPersistedTarget(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "staging",
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof"
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/targets/"+created.ID, nil))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d with body %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body.String())
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+created.ID, nil))
	if getResponse.Code != http.StatusNotFound {
		t.Fatalf("expected deleted target get status %d, got %d with body %s", http.StatusNotFound, getResponse.Code, getResponse.Body.String())
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []targetRecord
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected deleted target to be absent from list, got %#v", targets)
	}
}
