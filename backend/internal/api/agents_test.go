package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentHeartbeatRegistersAndUpdatesAgent(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	firstHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(firstHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.12",
		"version": "0.1.0",
		"labels": {
			"service": "checkout",
			"zone": "shanghai-a"
		},
		"capabilities": ["host_metrics", "process_metrics"]
	}`))

	if firstHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, firstHeartbeat.Code, firstHeartbeat.Body.String())
	}
	var registered struct {
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Capabilities []string          `json:"capabilities"`
		LastSeenAt   string            `json:"lastSeenAt"`
		TokenID      string            `json:"tokenId"`
	}
	if err := json.NewDecoder(firstHeartbeat.Body).Decode(&registered); err != nil {
		t.Fatalf("expected heartbeat JSON, got decode error: %v", err)
	}
	if registered.ID == "" {
		t.Fatal("expected generated agent id")
	}
	if registered.Name != "checkout-01" || registered.Status != "online" {
		t.Fatalf("expected online checkout agent, got %#v", registered)
	}
	if registered.Labels["service"] != "checkout" {
		t.Fatalf("expected labels to round-trip, got %#v", registered.Labels)
	}
	if registered.LastSeenAt == "" {
		t.Fatal("expected lastSeenAt to be set")
	}
	if registered.TokenID == "" {
		t.Fatal("expected heartbeat to bind the token id")
	}

	secondHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(secondHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
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
	if secondHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected update heartbeat status %d, got %d with body %s", http.StatusOK, secondHeartbeat.Code, secondHeartbeat.Body.String())
	}

	restartedRouter := NewRouter()
	listResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/agents", nil))

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var agents []struct {
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		IP           string            `json:"ip"`
		Version      string            `json:"version"`
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Capabilities []string          `json:"capabilities"`
		TokenID      string            `json:"tokenId"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&agents); err != nil {
		t.Fatalf("expected agent list JSON, got decode error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one upserted agent, got %#v", agents)
	}
	if agents[0].ID != registered.ID {
		t.Fatalf("expected stable agent id %q, got %q", registered.ID, agents[0].ID)
	}
	if agents[0].IP != "10.0.0.13" || agents[0].Version != "0.2.0" {
		t.Fatalf("expected heartbeat update to be listed, got %#v", agents[0])
	}
	if agents[0].Labels["zone"] != "shanghai-b" {
		t.Fatalf("expected updated labels, got %#v", agents[0].Labels)
	}
	if len(agents[0].Capabilities) != 3 {
		t.Fatalf("expected updated capabilities, got %#v", agents[0].Capabilities)
	}
	if agents[0].TokenID != registered.TokenID {
		t.Fatalf("expected listed token id %q, got %q", registered.TokenID, agents[0].TokenID)
	}
}

func TestAgentTokenProjectEnvironmentPropagatesToHeartbeatAndListFilter(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createToken := func(body string) struct {
		ID          string `json:"id"`
		Token       string `json:"token"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	} {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
		}
		var token struct {
			ID          string `json:"id"`
			Token       string `json:"token"`
			ProjectID   string `json:"projectId"`
			Environment string `json:"environment"`
		}
		if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
			t.Fatalf("expected token JSON, got decode error: %v", err)
		}
		return token
	}

	checkoutToken := createToken(`{
		"name": "checkout-install",
		"projectId": "project-checkout",
		"environment": "staging"
	}`)
	if checkoutToken.ID == "" || checkoutToken.Token == "" {
		t.Fatalf("expected checkout token metadata and secret, got %#v", checkoutToken)
	}
	if checkoutToken.ProjectID != "project-checkout" || checkoutToken.Environment != "staging" {
		t.Fatalf("expected checkout token project and environment, got %#v", checkoutToken)
	}

	checkoutHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(checkoutHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", checkoutToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.13",
		"version": "0.3.0",
		"labels": {
			"service": "checkout"
		},
		"capabilities": ["host_metrics"]
	}`))
	if checkoutHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected checkout heartbeat status %d, got %d with body %s", http.StatusOK, checkoutHeartbeat.Code, checkoutHeartbeat.Body.String())
	}
	var checkoutAgent struct {
		ID          string `json:"id"`
		TokenID     string `json:"tokenId"`
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(checkoutHeartbeat.Body).Decode(&checkoutAgent); err != nil {
		t.Fatalf("expected checkout heartbeat JSON, got decode error: %v", err)
	}
	if checkoutAgent.TokenID != checkoutToken.ID {
		t.Fatalf("expected heartbeat to bind token %q, got %#v", checkoutToken.ID, checkoutAgent)
	}
	if checkoutAgent.ProjectID != "project-checkout" || checkoutAgent.Environment != "staging" {
		t.Fatalf("expected heartbeat to inherit token project and environment, got %#v", checkoutAgent)
	}

	billingToken := createToken(`{
		"name": "billing-install",
		"projectId": "project-billing",
		"environment": "prod"
	}`)
	billingHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(billingHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", billingToken.Token, `{
		"name": "billing-01",
		"hostname": "billing-host-01",
		"ip": "10.0.0.21",
		"version": "0.3.0",
		"labels": {
			"service": "billing"
		},
		"capabilities": ["host_metrics"]
	}`))
	if billingHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected billing heartbeat status %d, got %d with body %s", http.StatusOK, billingHeartbeat.Code, billingHeartbeat.Body.String())
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/agents?projectId=project-checkout&environment=staging", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected filtered list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var agents []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&agents); err != nil {
		t.Fatalf("expected filtered agent list JSON, got decode error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected only checkout staging agent, got %#v", agents)
	}
	if agents[0].ID != checkoutAgent.ID || agents[0].Name != "checkout-01" {
		t.Fatalf("expected checkout agent in filtered list, got %#v", agents[0])
	}
	if agents[0].ProjectID != "project-checkout" || agents[0].Environment != "staging" {
		t.Fatalf("expected filtered agent project and environment, got %#v", agents[0])
	}
}

func TestAgentListMarksAgentsBehindLatestManifestVersion(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	binaryDir := t.TempDir()
	t.Setenv("AGENT_BINARY_DIR", binaryDir)
	binaryName := "all-in-one-agent-linux-amd64"
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), []byte("linux-amd64-agent"), 0o755); err != nil {
		t.Fatalf("expected test binary to be written, got %v", err)
	}
	manifest := `{
		"artifacts": [
			{
				"name": "all-in-one-agent-linux-amd64",
				"url": "/agent/binaries/all-in-one-agent-linux-amd64",
				"checksumUrl": "/agent/binaries/all-in-one-agent-linux-amd64.sha256",
				"sha256": "abc123",
				"sizeBytes": 17,
				"version": "v0.4.0",
				"commit": "release-commit",
				"date": "2026-06-23T00:00:00Z"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(binaryDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("expected release manifest to be written, got %v", err)
	}

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-version-check-01",
		"name": "checkout-version-check",
		"hostname": "checkout-host",
		"version": "v0.3.0",
		"capabilities": ["host_metrics"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected agent list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var agents []struct {
		ID               string `json:"id"`
		Version          string `json:"version"`
		LatestVersion    string `json:"latestVersion"`
		UpgradeAvailable bool   `json:"upgradeAvailable"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&agents); err != nil {
		t.Fatalf("expected agent list JSON, got %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %#v", agents)
	}
	if agents[0].ID != "agent-version-check-01" || agents[0].Version != "v0.3.0" {
		t.Fatalf("expected listed old-version agent, got %#v", agents[0])
	}
	if agents[0].LatestVersion != "v0.4.0" || !agents[0].UpgradeAvailable {
		t.Fatalf("expected listed agent to be marked upgradeable, got %#v", agents[0])
	}
}

func TestAgentMetricsAreAttachedToAgentList(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.13",
		"version": "0.2.0",
		"labels": {
			"service": "checkout"
		},
		"capabilities": ["host_metrics", "process_metrics"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}
	var registered struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(heartbeatResponse.Body).Decode(&registered); err != nil {
		t.Fatalf("expected heartbeat JSON, got decode error: %v", err)
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, `{
		"agentId": "`+registered.ID+`",
		"cpuUsagePercent": 72.5,
		"memoryUsagePercent": 61.25,
		"diskReadBytesPerSec": 1048576,
		"diskWriteBytesPerSec": 2097152,
		"networkRxBytesPerSec": 32768,
		"networkTxBytesPerSec": 65536,
		"processes": [
			{
				"pid": 1234,
				"name": "checkout",
				"cmdline": "checkout --config=/etc/checkout/prod.yaml --port=8080",
				"cpuUsagePercent": 34.5,
				"memoryRssBytes": 268435456,
				"fdCount": 88,
				"threadCount": 12
			}
		]
	}`))
	if metricsResponse.Code != http.StatusAccepted {
		t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, metricsResponse.Code, metricsResponse.Body.String())
	}

	restartedRouter := NewRouter()
	listResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var agents []struct {
		ID            string `json:"id"`
		LatestMetrics *struct {
			AgentID             string  `json:"agentId"`
			CPUUsagePercent     float64 `json:"cpuUsagePercent"`
			MemoryUsagePercent  float64 `json:"memoryUsagePercent"`
			DiskReadBytesPerSec float64 `json:"diskReadBytesPerSec"`
			Processes           []struct {
				PID             int     `json:"pid"`
				Name            string  `json:"name"`
				Cmdline         string  `json:"cmdline"`
				CPUUsagePercent float64 `json:"cpuUsagePercent"`
				MemoryRSSBytes  int64   `json:"memoryRssBytes"`
			} `json:"processes"`
		} `json:"latestMetrics"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&agents); err != nil {
		t.Fatalf("expected agent list JSON, got decode error: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != registered.ID {
		t.Fatalf("expected listed agent %q, got %#v", registered.ID, agents)
	}
	if agents[0].LatestMetrics == nil {
		t.Fatal("expected latest metrics to be attached to agent")
	}
	if agents[0].LatestMetrics.AgentID != registered.ID {
		t.Fatalf("expected latest metrics agent id %q, got %q", registered.ID, agents[0].LatestMetrics.AgentID)
	}
	if agents[0].LatestMetrics.CPUUsagePercent != 72.5 || agents[0].LatestMetrics.MemoryUsagePercent != 61.25 {
		t.Fatalf("expected host metrics to round-trip, got %#v", agents[0].LatestMetrics)
	}
	if len(agents[0].LatestMetrics.Processes) != 1 || agents[0].LatestMetrics.Processes[0].Name != "checkout" {
		t.Fatalf("expected process metrics to round-trip, got %#v", agents[0].LatestMetrics.Processes)
	}
	if agents[0].LatestMetrics.Processes[0].Cmdline != "checkout --config=/etc/checkout/prod.yaml --port=8080" {
		t.Fatalf("expected process cmdline to round-trip, got %#v", agents[0].LatestMetrics.Processes[0])
	}
}

func TestAgentTokenCannotSubmitMetricsForAnotherAgent(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	checkoutToken := createScopedAgentToken(t, router, "checkout-install", "project-checkout", "staging")
	checkoutAgentID := registerScopedAgent(t, router, checkoutToken.Token, "checkout-secure-01")
	billingToken := createScopedAgentToken(t, router, "billing-install", "project-billing", "prod")
	billingAgentID := registerScopedAgent(t, router, billingToken.Token, "billing-secure-01")

	rejectedMetrics := httptest.NewRecorder()
	router.ServeHTTP(rejectedMetrics, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", checkoutToken.Token, `{
		"agentId": "`+billingAgentID+`",
		"cpuUsagePercent": 12.5
	}`))
	if rejectedMetrics.Code != http.StatusForbidden {
		t.Fatalf("expected cross-agent metrics status %d, got %d with body %s", http.StatusForbidden, rejectedMetrics.Code, rejectedMetrics.Body.String())
	}

	allowedMetrics := httptest.NewRecorder()
	router.ServeHTTP(allowedMetrics, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", checkoutToken.Token, `{
		"agentId": "`+checkoutAgentID+`",
		"cpuUsagePercent": 11.5
	}`))
	if allowedMetrics.Code != http.StatusAccepted {
		t.Fatalf("expected own-agent metrics status %d, got %d with body %s", http.StatusAccepted, allowedMetrics.Code, allowedMetrics.Body.String())
	}
}

func TestAgentReportsTargetHealthCheckResult(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createScopedAgentToken(t, router, "checkout-install", "project-checkout", "staging")
	agentID := registerScopedAgentWithID(t, router, token.Token, "agent-checkout-health-01", "checkout-health-01")

	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-service",
		"projectId": "project-checkout",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "staging",
		"agentIds": ["`+agentID+`"],
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 200,
			"timeoutMs": 1000
		}
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target targetRecord
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/target-health-checks", token.Token, `{
		"agentId": "`+agentID+`",
		"targetId": "`+target.ID+`",
		"status": "unhealthy",
		"url": "http://checkout.internal:8080/ready",
		"expectedStatus": 200,
		"observedStatus": 503,
		"latencyMs": 12.5,
		"error": "expected HTTP 200, got HTTP 503",
		"checkedAt": "2026-06-24T08:30:00Z"
	}`))
	if reportResponse.Code != http.StatusAccepted {
		t.Fatalf("expected agent target health report status %d, got %d with body %s", http.StatusAccepted, reportResponse.Code, reportResponse.Body.String())
	}
	var reported targetHealthCheckResult
	if err := json.NewDecoder(reportResponse.Body).Decode(&reported); err != nil {
		t.Fatalf("expected reported target health JSON, got decode error: %v", err)
	}
	if reported.TargetID != target.ID || reported.TargetName != "checkout-service" || reported.Status != "unhealthy" || reported.ObservedStatus != http.StatusServiceUnavailable {
		t.Fatalf("expected agent reported target health metadata, got %#v", reported)
	}
	if reported.CheckedAt != "2026-06-24T08:30:00Z" || reported.Error == "" || reported.LatencyMs != 12.5 || reported.Source != "agent_daemon" {
		t.Fatalf("expected agent reported target health details, got %#v", reported)
	}

	restartedRouter := NewRouter()
	listResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/targets?projectId=project-checkout&environment=staging", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []struct {
		ID              string                   `json:"id"`
		LastHealthCheck *targetHealthCheckResult `json:"lastHealthCheck"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != target.ID || targets[0].LastHealthCheck == nil {
		t.Fatalf("expected listed target with latest agent health result, got %#v", targets)
	}
	if targets[0].LastHealthCheck.Status != "unhealthy" ||
		targets[0].LastHealthCheck.ObservedStatus != http.StatusServiceUnavailable ||
		targets[0].LastHealthCheck.Source != "agent_daemon" {
		t.Fatalf("expected latest health result from agent report, got %#v", targets[0].LastHealthCheck)
	}

	historyResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+target.ID+"/health-checks?limit=1", nil))
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("expected target health history status %d, got %d with body %s", http.StatusOK, historyResponse.Code, historyResponse.Body.String())
	}
	var history []targetHealthCheckResult
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatalf("expected target health history JSON, got decode error: %v", err)
	}
	if len(history) != 1 ||
		history[0].TargetID != target.ID ||
		history[0].Status != "unhealthy" ||
		history[0].CheckedAt != "2026-06-24T08:30:00Z" ||
		history[0].Source != "agent_daemon" {
		t.Fatalf("expected latest agent health report in history, got %#v", history)
	}
}

func TestAgentListsOnlyBoundTargets(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	checkoutToken := createScopedAgentToken(t, router, "checkout-install", "project-checkout", "staging")
	checkoutAgentID := registerScopedAgentWithID(t, router, checkoutToken.Token, "agent-checkout-targets-01", "checkout-targets-01")
	billingToken := createScopedAgentToken(t, router, "billing-install", "project-billing", "prod")
	billingAgentID := registerScopedAgentWithID(t, router, billingToken.Token, "agent-billing-targets-01", "billing-targets-01")

	createTarget := func(body string) targetRecord {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
		}
		var target targetRecord
		if err := json.NewDecoder(response.Body).Decode(&target); err != nil {
			t.Fatalf("expected target JSON, got decode error: %v", err)
		}
		return target
	}

	checkoutTarget := createTarget(`{
		"name": "checkout-service",
		"projectId": "project-checkout",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "staging",
		"agentIds": ["` + checkoutAgentID + `"],
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1500
		}
	}`)
	createTarget(`{
		"name": "checkout-unbound",
		"projectId": "project-checkout",
		"baseUrl": "http://checkout-unbound.internal:8080",
		"environment": "staging",
		"agentIds": []
	}`)
	createTarget(`{
		"name": "billing-service",
		"projectId": "project-billing",
		"baseUrl": "http://billing.internal:8080",
		"environment": "prod",
		"agentIds": ["` + billingAgentID + `"]
	}`)

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/targets?agentId="+checkoutAgentID, checkoutToken.Token, ""))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected bound target list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var targets []targetRecord
	if err := json.NewDecoder(listResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected bound target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one checkout bound target, got %#v", targets)
	}
	if targets[0].ID != checkoutTarget.ID || targets[0].ProjectID != "project-checkout" || targets[0].Environment != "staging" {
		t.Fatalf("expected checkout target in agent list, got %#v", targets[0])
	}
	if !targets[0].HealthCheck.Enabled || targets[0].HealthCheck.Path != "/ready" || targets[0].HealthCheck.ExpectedStatus != http.StatusNoContent || targets[0].HealthCheck.TimeoutMs != 1500 {
		t.Fatalf("expected target health check config for agent, got %#v", targets[0].HealthCheck)
	}

	crossAgentResponse := httptest.NewRecorder()
	router.ServeHTTP(crossAgentResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/targets?agentId="+billingAgentID, checkoutToken.Token, ""))
	if crossAgentResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-agent target list status %d, got %d with body %s", http.StatusForbidden, crossAgentResponse.Code, crossAgentResponse.Body.String())
	}
}

func TestAgentMetricsTimelineCanBeQueriedByAgentAndTimeRange(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-checkout-01",
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"capabilities": ["host_metrics", "process_metrics"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}

	for _, payload := range []string{
		`{
			"agentId": "agent-checkout-01",
			"collectedAt": "2026-06-14T10:00:00Z",
			"cpuUsagePercent": 30,
			"memoryUsagePercent": 45,
			"processes": []
		}`,
		`{
			"agentId": "agent-checkout-01",
			"collectedAt": "2026-06-14T10:01:00Z",
			"cpuUsagePercent": 42.5,
			"memoryUsagePercent": 61.25,
			"diskReadBytesPerSec": 1024,
			"networkRxBytesPerSec": 2048,
			"processes": [
				{
					"pid": 1234,
					"name": "checkout",
					"cmdline": "checkout --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 34.5,
					"memoryRssBytes": 268435456,
					"fdCount": 88,
					"threadCount": 12
				}
			]
		}`,
		`{
			"agentId": "agent-checkout-01",
			"collectedAt": "2026-06-14T10:02:00Z",
			"cpuUsagePercent": 51,
			"memoryUsagePercent": 63,
			"diskReadBytesPerSec": 4096,
			"networkRxBytesPerSec": 8192,
			"processes": []
		}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, payload))
		if response.Code != http.StatusAccepted {
			t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, response.Code, response.Body.String())
		}
	}

	query := url.Values{}
	query.Set("agentId", "agent-checkout-01")
	query.Set("from", "2026-06-14T10:01:00Z")
	query.Set("to", "2026-06-14T10:02:00Z")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/agent-metrics?"+query.Encode(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected metrics timeline status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}

	var timeline []struct {
		AgentID              string  `json:"agentId"`
		CollectedAt          string  `json:"collectedAt"`
		CPUUsagePercent      float64 `json:"cpuUsagePercent"`
		MemoryUsagePercent   float64 `json:"memoryUsagePercent"`
		DiskReadBytesPerSec  float64 `json:"diskReadBytesPerSec"`
		NetworkRxBytesPerSec float64 `json:"networkRxBytesPerSec"`
		Processes            []struct {
			PID             int     `json:"pid"`
			Name            string  `json:"name"`
			Cmdline         string  `json:"cmdline"`
			CPUUsagePercent float64 `json:"cpuUsagePercent"`
		} `json:"processes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&timeline); err != nil {
		t.Fatalf("expected metrics timeline JSON, got decode error: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected two timeline samples, got %#v", timeline)
	}
	if timeline[0].CollectedAt != "2026-06-14T10:01:00Z" || timeline[1].CollectedAt != "2026-06-14T10:02:00Z" {
		t.Fatalf("expected ascending timeline samples, got %#v", timeline)
	}
	if timeline[0].CPUUsagePercent != 42.5 || timeline[0].MemoryUsagePercent != 61.25 {
		t.Fatalf("expected first ranged sample values, got %#v", timeline[0])
	}
	if timeline[0].DiskReadBytesPerSec != 1024 || timeline[0].NetworkRxBytesPerSec != 2048 {
		t.Fatalf("expected IO metrics to round-trip, got %#v", timeline[0])
	}
	if len(timeline[0].Processes) != 1 || timeline[0].Processes[0].Name != "checkout" {
		t.Fatalf("expected process metric to round-trip, got %#v", timeline[0].Processes)
	}
	if timeline[0].Processes[0].Cmdline != "checkout --config=/etc/checkout/prod.yaml --port=8080" {
		t.Fatalf("expected process cmdline to round-trip in timeline, got %#v", timeline[0].Processes[0])
	}
}

func TestAgentTokenAuthenticatesHeartbeatAndMetrics(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	missingHeartbeatAuth := httptest.NewRecorder()
	router.ServeHTTP(missingHeartbeatAuth, httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeat", bytes.NewBufferString(`{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`)))
	if missingHeartbeatAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing heartbeat token status %d, got %d", http.StatusUnauthorized, missingHeartbeatAuth.Code)
	}

	badHeartbeatAuth := httptest.NewRecorder()
	router.ServeHTTP(badHeartbeatAuth, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", "wrong-token", `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if badHeartbeatAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid heartbeat token status %d, got %d", http.StatusUnauthorized, badHeartbeatAuth.Code)
	}

	validHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(validHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if validHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected valid heartbeat status %d, got %d with body %s", http.StatusOK, validHeartbeat.Code, validHeartbeat.Body.String())
	}
	var registered struct {
		ID      string `json:"id"`
		TokenID string `json:"tokenId"`
	}
	if err := json.NewDecoder(validHeartbeat.Body).Decode(&registered); err != nil {
		t.Fatalf("expected heartbeat JSON, got decode error: %v", err)
	}
	if registered.ID == "" || registered.TokenID == "" {
		t.Fatalf("expected registered agent id and token id, got %#v", registered)
	}

	missingMetricsAuth := httptest.NewRecorder()
	router.ServeHTTP(missingMetricsAuth, httptest.NewRequest(http.MethodPost, "/agent/v1/metrics", bytes.NewBufferString(`{
		"agentId": "`+registered.ID+`",
		"cpuUsagePercent": 7.5
	}`)))
	if missingMetricsAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing metrics token status %d, got %d", http.StatusUnauthorized, missingMetricsAuth.Code)
	}

	validMetrics := httptest.NewRecorder()
	router.ServeHTTP(validMetrics, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, `{
		"agentId": "`+registered.ID+`",
		"cpuUsagePercent": 7.5
	}`))
	if validMetrics.Code != http.StatusAccepted {
		t.Fatalf("expected valid metrics status %d, got %d with body %s", http.StatusAccepted, validMetrics.Code, validMetrics.Body.String())
	}
}

func TestRevokedAgentTokenCannotAuthenticate(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "checkout-install"
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var createdToken struct {
		ID     string `json:"id"`
		Token  string `json:"token"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&createdToken); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	if createdToken.ID == "" || createdToken.Token == "" || createdToken.Status != "active" {
		t.Fatalf("expected active token with secret, got %#v", createdToken)
	}

	validHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(validHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", createdToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if validHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected valid heartbeat status %d, got %d with body %s", http.StatusOK, validHeartbeat.Code, validHeartbeat.Body.String())
	}

	revokeResponse := httptest.NewRecorder()
	router.ServeHTTP(revokeResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens/"+createdToken.ID+"/revoke", nil))
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("expected token revoke status %d, got %d with body %s", http.StatusOK, revokeResponse.Code, revokeResponse.Body.String())
	}
	var revokedToken struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.NewDecoder(revokeResponse.Body).Decode(&revokedToken); err != nil {
		t.Fatalf("expected revoked token JSON, got decode error: %v", err)
	}
	if revokedToken.ID != createdToken.ID || revokedToken.Status != "revoked" || revokedToken.Token != "" || revokedToken.UpdatedAt == "" {
		t.Fatalf("expected revoked token metadata without secret, got %#v", revokedToken)
	}

	rejectedHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(rejectedHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", createdToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if rejectedHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked token heartbeat status %d, got %d with body %s", http.StatusUnauthorized, rejectedHeartbeat.Code, rejectedHeartbeat.Body.String())
	}
}

func TestExpiredAgentTokenCannotAuthenticate(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	expiredAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "checkout-install-expired",
		"expiresAt": "`+expiredAt+`"
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var createdToken struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		Status    string `json:"status"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&createdToken); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	if createdToken.ID == "" || createdToken.Token == "" || createdToken.Status != "active" || createdToken.ExpiresAt != expiredAt {
		t.Fatalf("expected active expired token metadata with secret, got %#v", createdToken)
	}

	rejectedHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(rejectedHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", createdToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if rejectedHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired token heartbeat status %d, got %d with body %s", http.StatusUnauthorized, rejectedHeartbeat.Code, rejectedHeartbeat.Body.String())
	}
}

func TestRotatedAgentTokenInvalidatesOldSecret(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "checkout-install-rotate",
		"expiresInSeconds": 3600
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var createdToken struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		Status    string `json:"status"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&createdToken); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	if createdToken.ID == "" || createdToken.Token == "" || createdToken.Status != "active" || createdToken.ExpiresAt == "" {
		t.Fatalf("expected active token with secret and expiry, got %#v", createdToken)
	}

	validOldHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(validOldHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", createdToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if validOldHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected old token heartbeat status %d before rotation, got %d with body %s", http.StatusOK, validOldHeartbeat.Code, validOldHeartbeat.Body.String())
	}

	rotateResponse := httptest.NewRecorder()
	router.ServeHTTP(rotateResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens/"+createdToken.ID+"/rotate", bytes.NewBufferString(`{
		"expiresInSeconds": 7200
	}`)))
	if rotateResponse.Code != http.StatusOK {
		t.Fatalf("expected token rotate status %d, got %d with body %s", http.StatusOK, rotateResponse.Code, rotateResponse.Body.String())
	}
	var rotatedToken struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		Status    string `json:"status"`
		ExpiresAt string `json:"expiresAt"`
		RotatedAt string `json:"rotatedAt"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.NewDecoder(rotateResponse.Body).Decode(&rotatedToken); err != nil {
		t.Fatalf("expected rotated token JSON, got decode error: %v", err)
	}
	if rotatedToken.ID != createdToken.ID || rotatedToken.Status != "active" || rotatedToken.Token == "" || rotatedToken.Token == createdToken.Token {
		t.Fatalf("expected rotated token to keep id and return a new secret, got %#v", rotatedToken)
	}
	if rotatedToken.ExpiresAt == "" || rotatedToken.ExpiresAt == createdToken.ExpiresAt || rotatedToken.RotatedAt == "" || rotatedToken.UpdatedAt == "" {
		t.Fatalf("expected rotated token metadata to include new expiry and rotation timestamps, got %#v", rotatedToken)
	}

	rejectedOldHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(rejectedOldHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", createdToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if rejectedOldHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("expected old token heartbeat status %d after rotation, got %d with body %s", http.StatusUnauthorized, rejectedOldHeartbeat.Code, rejectedOldHeartbeat.Body.String())
	}

	validNewHeartbeat := httptest.NewRecorder()
	router.ServeHTTP(validNewHeartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", rotatedToken.Token, `{
		"name": "checkout-01",
		"hostname": "checkout-host-01"
	}`))
	if validNewHeartbeat.Code != http.StatusOK {
		t.Fatalf("expected new token heartbeat status %d, got %d with body %s", http.StatusOK, validNewHeartbeat.Code, validNewHeartbeat.Body.String())
	}
}

func TestAgentUploadsProfileArtifact(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	payload := []byte("agent-uploaded-cpu-profile")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"runId":        "run-agent-upload-1",
		"scenarioId":   "scenario-agent-upload-1",
		"scenarioName": "agent-upload-checkout",
		"targetId":     "target-agent-upload-1",
		"targetName":   "checkout-target",
		"profileType":  "cpu",
		"sourceUrl":    "agent://checkout-01/debug/pprof/profile",
		"startedAt":    "2026-06-13T08:30:00Z",
		"finishedAt":   "2026-06-13T08:30:01Z",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("expected multipart field %s to be written, got %v", key, err)
		}
	}
	file, err := writer.CreateFormFile("file", "agent-cpu.pprof")
	if err != nil {
		t.Fatalf("expected multipart file to be created, got %v", err)
	}
	if _, err := file.Write(payload); err != nil {
		t.Fatalf("expected multipart file payload to be written, got %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected multipart writer to close, got %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/agent/v1/profile-artifacts", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected upload status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}
	var artifact struct {
		ID           string `json:"id"`
		RunID        string `json:"runId"`
		ScenarioName string `json:"scenarioName"`
		TargetName   string `json:"targetName"`
		ProfileType  string `json:"profileType"`
		Status       string `json:"status"`
		SourceURL    string `json:"sourceUrl"`
		FileName     string `json:"fileName"`
		ContentType  string `json:"contentType"`
		SizeBytes    int64  `json:"sizeBytes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&artifact); err != nil {
		t.Fatalf("expected uploaded artifact JSON, got decode error: %v", err)
	}
	if artifact.ID == "" {
		t.Fatal("expected uploaded artifact id")
	}
	if artifact.RunID != "run-agent-upload-1" || artifact.ScenarioName != "agent-upload-checkout" {
		t.Fatalf("expected run and scenario metadata to round-trip, got %#v", artifact)
	}
	if artifact.TargetName != "checkout-target" || artifact.ProfileType != "cpu" || artifact.Status != "collected" {
		t.Fatalf("expected collected target cpu artifact, got %#v", artifact)
	}
	if artifact.SourceURL != "agent://checkout-01/debug/pprof/profile" {
		t.Fatalf("expected source URL to round-trip, got %#v", artifact)
	}
	if artifact.FileName != "agent-cpu.pprof" {
		t.Fatalf("expected uploaded filename, got %#v", artifact)
	}
	if artifact.SizeBytes != int64(len(payload)) {
		t.Fatalf("expected artifact size %d, got %d", len(payload), artifact.SizeBytes)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/profile-artifacts", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected artifact list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var artifacts []struct {
		ID     string `json:"id"`
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&artifacts); err != nil {
		t.Fatalf("expected artifact list JSON, got decode error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID || artifacts[0].RunID != artifact.RunID || artifacts[0].Status != "collected" {
		t.Fatalf("expected uploaded artifact to be listed, got %#v", artifacts)
	}

	downloadResponse := httptest.NewRecorder()
	router.ServeHTTP(downloadResponse, httptest.NewRequest(http.MethodGet, "/api/profile-artifacts/"+artifact.ID+"/download", nil))
	if downloadResponse.Code != http.StatusOK {
		t.Fatalf("expected download status %d, got %d with body %s", http.StatusOK, downloadResponse.Code, downloadResponse.Body.String())
	}
	if !bytes.Equal(downloadResponse.Body.Bytes(), payload) {
		t.Fatalf("expected downloaded payload %q, got %q", payload, downloadResponse.Body.Bytes())
	}
}

func TestAgentTokenCannotUploadProfileArtifactForAnotherWorkspaceRun(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	checkoutToken := createScopedAgentToken(t, router, "checkout-install", "project-checkout", "staging")
	registerScopedAgent(t, router, checkoutToken.Token, "checkout-artifact-01")

	runStore := newRunHistoryStoreFromEnv()
	if err := runStore.save(createRunResponse{
		ID:            "run-billing-artifact-secure",
		ScenarioName:  "billing-secure",
		TargetName:    "billing-target",
		ProjectID:     "project-billing",
		Environment:   "prod",
		Name:          "billing artifact secure",
		Status:        "completed",
		Protocol:      "http",
		Method:        "GET",
		URL:           "http://billing.internal/health",
		TotalRequests: 1,
		CreatedAt:     "2026-06-13T08:30:00Z",
	}); err != nil {
		t.Fatalf("expected billing run history to be saved, got %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"runId":       "run-billing-artifact-secure",
		"profileType": "cpu",
		"sourceUrl":   "agent://checkout-artifact-01/debug/pprof/profile",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("expected multipart field %s to be written, got %v", key, err)
		}
	}
	file, err := writer.CreateFormFile("file", "cross-run.pprof")
	if err != nil {
		t.Fatalf("expected multipart file to be created, got %v", err)
	}
	if _, err := file.Write([]byte("profile")); err != nil {
		t.Fatalf("expected multipart payload to be written, got %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected multipart writer to close, got %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/agent/v1/profile-artifacts", body)
	request.Header.Set("Authorization", "Bearer "+checkoutToken.Token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected cross-run artifact upload status %d, got %d with body %s", http.StatusForbidden, response.Code, response.Body.String())
	}
}

func TestAgentProfileArtifactUploadRequiresToken(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	file, err := writer.CreateFormFile("file", "agent-cpu.pprof")
	if err != nil {
		t.Fatalf("expected multipart file to be created, got %v", err)
	}
	if _, err := file.Write([]byte("profile")); err != nil {
		t.Fatalf("expected multipart payload to be written, got %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected multipart writer to close, got %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/agent/v1/profile-artifacts", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing token status %d, got %d with body %s", http.StatusUnauthorized, response.Code, response.Body.String())
	}
}

func TestAgentInstallScriptCanBeDownloaded(t *testing.T) {
	router := NewRouter()
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/install.sh", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected install script status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/x-shellscript; charset=utf-8" {
		t.Fatalf("expected shell script content type, got %q", contentType)
	}
	body := response.Body.String()
	for _, expected := range []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"--server",
		"--token",
		"--binary-url",
		"agent/binaries/all-in-one-agent-linux-amd64",
		"BINARY_URL=\"${SERVER_URL%/}/agent/binaries/all-in-one-agent-linux-amd64\"",
		"CHECKSUM_URL=\"${BINARY_URL}.sha256\"",
		"DOWNLOAD_PATH=\"$(mktemp \"${BINARY_PATH}.download.XXXXXX\")\"",
		"curl -fsSL \"${BINARY_URL}\" -o \"${DOWNLOAD_PATH}\"",
		"curl -fsSL \"${CHECKSUM_URL}\" -o \"${DOWNLOAD_PATH}.sha256\"",
		"sha256sum -c \"${DOWNLOAD_PATH}.sha256\"",
		"install -m 0755 \"${DOWNLOAD_PATH}\" \"${BINARY_PATH}\"",
		"/etc/all-in-one-agent/agent.json",
		"/etc/systemd/system/all-in-one-agent.service",
		`"controlPlaneUrl": "${SERVER_URL}"`,
		`"token": "${AGENT_TOKEN}"`,
		`"agentId": "${AGENT_ID}"`,
		`"heartbeatInterval": "${HEARTBEAT_INTERVAL}"`,
		"ExecStart=${BINARY_PATH} --daemon --config ${CONFIG_FILE}",
		"systemctl enable --now all-in-one-agent.service",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected install script to contain %q, got body:\n%s", expected, body)
		}
	}
}

func TestAgentInstallScriptSupportsForcedBinaryDownloadForUpgrades(t *testing.T) {
	router := NewRouter()
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/install.sh", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected install script status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"[--force-download]",
		"--force-download|--upgrade)",
		"FORCE_DOWNLOAD=\"${FORCE_DOWNLOAD:-0}\"",
		"FORCE_DOWNLOAD=\"1\"",
		"if [[ \"${FORCE_DOWNLOAD}\" == \"1\" || ! -x \"${BINARY_PATH}\" ]]; then",
		"systemctl restart all-in-one-agent.service",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected install script upgrade support to contain %q, got body:\n%s", expected, body)
		}
	}
}

func TestAgentBinaryDownloadServesConfiguredArtifact(t *testing.T) {
	binaryDir := t.TempDir()
	t.Setenv("AGENT_BINARY_DIR", binaryDir)
	expectedBinary := []byte("linux-amd64-agent")
	if err := os.WriteFile(filepath.Join(binaryDir, "all-in-one-agent-linux-amd64"), expectedBinary, 0o755); err != nil {
		t.Fatalf("expected test agent binary to be written, got %v", err)
	}
	router := NewRouter()
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/binaries/all-in-one-agent-linux-amd64", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected binary download status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/octet-stream" {
		t.Fatalf("expected binary content type, got %q", contentType)
	}
	if disposition := response.Header().Get("Content-Disposition"); !strings.Contains(disposition, `filename="all-in-one-agent-linux-amd64"`) {
		t.Fatalf("expected binary attachment disposition, got %q", disposition)
	}
	if !bytes.Equal(response.Body.Bytes(), expectedBinary) {
		t.Fatalf("expected binary body %q, got %q", expectedBinary, response.Body.Bytes())
	}
}

func TestAgentBinaryChecksumDownloadServesConfiguredArtifactChecksum(t *testing.T) {
	binaryDir := t.TempDir()
	t.Setenv("AGENT_BINARY_DIR", binaryDir)
	binaryName := "all-in-one-agent-linux-amd64"
	expectedBinary := []byte("linux-amd64-agent")
	sum := sha256.Sum256(expectedBinary)
	expectedChecksum := fmt.Sprintf("%x  %s\n", sum, binaryName)
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName+".sha256"), []byte(expectedChecksum), 0o644); err != nil {
		t.Fatalf("expected test checksum to be written, got %v", err)
	}

	router := NewRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/binaries/all-in-one-agent-linux-amd64.sha256", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected checksum download status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
		t.Fatalf("expected checksum content type, got %q", contentType)
	}
	if response.Body.String() != expectedChecksum {
		t.Fatalf("expected checksum body %q, got %q", expectedChecksum, response.Body.String())
	}
}

func TestAgentBinaryChecksumDownloadCanGenerateChecksumFromBinary(t *testing.T) {
	binaryDir := t.TempDir()
	t.Setenv("AGENT_BINARY_DIR", binaryDir)
	binaryName := "all-in-one-agent-linux-amd64"
	expectedBinary := []byte("linux-amd64-agent")
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), expectedBinary, 0o755); err != nil {
		t.Fatalf("expected test binary to be written, got %v", err)
	}
	sum := sha256.Sum256(expectedBinary)
	expectedChecksum := fmt.Sprintf("%x  %s\n", sum, binaryName)

	router := NewRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/binaries/all-in-one-agent-linux-amd64.sha256", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected generated checksum status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if response.Body.String() != expectedChecksum {
		t.Fatalf("expected generated checksum body %q, got %q", expectedChecksum, response.Body.String())
	}
}

func TestAgentBinaryManifestServesAvailableArtifacts(t *testing.T) {
	binaryDir := t.TempDir()
	t.Setenv("AGENT_BINARY_DIR", binaryDir)
	binaryName := "all-in-one-agent-linux-amd64"
	expectedBinary := []byte("linux-amd64-agent")
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), expectedBinary, 0o755); err != nil {
		t.Fatalf("expected test binary to be written, got %v", err)
	}
	sum := sha256.Sum256(expectedBinary)
	expectedSHA256 := fmt.Sprintf("%x", sum)

	router := NewRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/binaries/manifest.json", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected manifest status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var manifest struct {
		Artifacts []struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			ChecksumURL string `json:"checksumUrl"`
			SHA256      string `json:"sha256"`
			SizeBytes   int64  `json:"sizeBytes"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatalf("expected manifest JSON, got %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected one manifest artifact, got %#v", manifest)
	}
	artifact := manifest.Artifacts[0]
	if artifact.Name != binaryName || artifact.URL != "/agent/binaries/"+binaryName || artifact.ChecksumURL != "/agent/binaries/"+binaryName+".sha256" {
		t.Fatalf("expected manifest artifact links, got %#v", artifact)
	}
	if artifact.SHA256 != expectedSHA256 || artifact.SizeBytes != int64(len(expectedBinary)) {
		t.Fatalf("expected manifest checksum and size, got %#v", artifact)
	}
}

func createAgentToken(t *testing.T, router http.Handler) string {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "checkout-install"
	}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}

	var token struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Token     string `json:"token"`
		Status    string `json:"status"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	if token.ID == "" || token.Token == "" || token.CreatedAt == "" {
		t.Fatalf("expected token metadata and one-time secret, got %#v", token)
	}
	if token.Name != "checkout-install" || token.Status != "active" {
		t.Fatalf("expected active checkout token, got %#v", token)
	}
	return token.Token
}

type testScopedAgentToken struct {
	ID          string `json:"id"`
	Token       string `json:"token"`
	ProjectID   string `json:"projectId"`
	Environment string `json:"environment"`
}

func createScopedAgentToken(t *testing.T, router http.Handler, name string, projectID string, environment string) testScopedAgentToken {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "`+name+`",
		"projectId": "`+projectID+`",
		"environment": "`+environment+`"
	}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected scoped token creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}

	var token testScopedAgentToken
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatalf("expected scoped token JSON, got decode error: %v", err)
	}
	if token.ID == "" || token.Token == "" {
		t.Fatalf("expected scoped token metadata and secret, got %#v", token)
	}
	if token.ProjectID != projectID || token.Environment != environment {
		t.Fatalf("expected scoped token %s/%s, got %#v", projectID, environment, token)
	}
	return token
}

func registerScopedAgent(t *testing.T, router http.Handler, token string, name string) string {
	return registerScopedAgentWithID(t, router, token, "", name)
}

func registerScopedAgentWithID(t *testing.T, router http.Handler, token string, id string, name string) string {
	t.Helper()

	idField := ""
	if id != "" {
		idField = `"id": "` + id + `",`
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		`+idField+`
		"name": "`+name+`",
		"hostname": "`+name+`-host",
		"ip": "10.0.0.10",
		"version": "0.3.0",
		"capabilities": ["host_metrics", "pprof"]
	}`))
	if response.Code != http.StatusOK {
		t.Fatalf("expected scoped agent heartbeat status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}

	var agent struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&agent); err != nil {
		t.Fatalf("expected scoped agent heartbeat JSON, got decode error: %v", err)
	}
	if agent.ID == "" {
		t.Fatalf("expected scoped agent id, got %#v", agent)
	}
	return agent.ID
}

func newAuthorizedAgentRequest(method string, path string, token string, body string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}
