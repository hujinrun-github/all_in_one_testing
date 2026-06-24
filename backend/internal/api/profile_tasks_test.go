package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileTaskLifecycleCreatesLeasesAndCompletesTask(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)

	heartbeat := httptest.NewRecorder()
	router.ServeHTTP(heartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-checkout-01",
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.13",
		"version": "0.3.0",
		"capabilities": ["pprof"]
	}`))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-profile-task-1",
		"scenarioName": "checkout-smoke",
		"targetName": "checkout-01",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "heap",
		"profileSeconds": 1
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID             string `json:"id"`
		AgentID        string `json:"agentId"`
		RunID          string `json:"runId"`
		PprofBaseURL   string `json:"pprofBaseUrl"`
		ProfileType    string `json:"profileType"`
		ProfileSeconds int    `json:"profileSeconds"`
		Source         string `json:"source"`
		Status         string `json:"status"`
		CreatedAt      string `json:"createdAt"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected task create JSON, got decode error: %v", err)
	}
	if created.ID == "" || created.CreatedAt == "" {
		t.Fatalf("expected task id and createdAt, got %#v", created)
	}
	if created.AgentID != "agent-checkout-01" || created.RunID != "run-profile-task-1" || created.Status != "pending" {
		t.Fatalf("expected pending task metadata, got %#v", created)
	}
	if created.PprofBaseURL != "http://127.0.0.1:6060/debug/pprof" || created.ProfileType != "heap" || created.ProfileSeconds != 1 {
		t.Fatalf("expected pprof task fields, got %#v", created)
	}
	if created.Source != "api" {
		t.Fatalf("expected default task source api, got %#v", created)
	}

	pollResponse := httptest.NewRecorder()
	router.ServeHTTP(pollResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if pollResponse.Code != http.StatusOK {
		t.Fatalf("expected task poll status %d, got %d with body %s", http.StatusOK, pollResponse.Code, pollResponse.Body.String())
	}
	var tasks []struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		PprofBaseURL string `json:"pprofBaseUrl"`
		ProfileType  string `json:"profileType"`
		Source       string `json:"source"`
		LeasedAt     string `json:"leasedAt"`
	}
	if err := json.NewDecoder(pollResponse.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected task poll JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one leased task, got %#v", tasks)
	}
	if tasks[0].ID != created.ID || tasks[0].Status != "leased" || tasks[0].LeasedAt == "" {
		t.Fatalf("expected leased task to match created task, got %#v", tasks[0])
	}
	if tasks[0].PprofBaseURL != created.PprofBaseURL || tasks[0].ProfileType != "heap" {
		t.Fatalf("expected leased task pprof fields, got %#v", tasks[0])
	}
	if tasks[0].Source != "api" {
		t.Fatalf("expected leased task source api, got %#v", tasks[0])
	}

	completeResponse := httptest.NewRecorder()
	router.ServeHTTP(completeResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/"+created.ID+"/complete", token, `{
		"artifactId": "profile-heap-artifact-1"
	}`))
	if completeResponse.Code != http.StatusOK {
		t.Fatalf("expected complete status %d, got %d with body %s", http.StatusOK, completeResponse.Code, completeResponse.Body.String())
	}
	var completed struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ArtifactID  string `json:"artifactId"`
		CompletedAt string `json:"completedAt"`
	}
	if err := json.NewDecoder(completeResponse.Body).Decode(&completed); err != nil {
		t.Fatalf("expected task complete JSON, got decode error: %v", err)
	}
	if completed.ID != created.ID || completed.Status != "completed" || completed.ArtifactID != "profile-heap-artifact-1" || completed.CompletedAt == "" {
		t.Fatalf("expected completed task with artifact id, got %#v", completed)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected task list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var listed []struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		ArtifactID string `json:"artifactId"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("expected task list JSON, got decode error: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID || listed[0].Status != "completed" || listed[0].ArtifactID != "profile-heap-artifact-1" {
		t.Fatalf("expected completed task in list, got %#v", listed)
	}
}

func TestAgentTokenCannotPollOrCompleteProfileTasksForAnotherAgent(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	checkoutToken := createScopedAgentToken(t, router, "checkout-install", "project-checkout", "staging")
	registerScopedAgent(t, router, checkoutToken.Token, "checkout-profile-secure-01")
	billingToken := createScopedAgentToken(t, router, "billing-install", "project-billing", "prod")
	billingAgentID := registerScopedAgent(t, router, billingToken.Token, "billing-profile-secure-01")

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "`+billingAgentID+`",
		"runId": "run-billing-profile-secure",
		"scenarioName": "billing-secure",
		"targetName": "billing-01",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "cpu",
		"profileSeconds": 1
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected task create JSON, got decode error: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created task id")
	}

	pollResponse := httptest.NewRecorder()
	router.ServeHTTP(pollResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId="+billingAgentID, checkoutToken.Token, ""))
	if pollResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-agent task poll status %d, got %d with body %s", http.StatusForbidden, pollResponse.Code, pollResponse.Body.String())
	}

	completeResponse := httptest.NewRecorder()
	router.ServeHTTP(completeResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/"+created.ID+"/complete", checkoutToken.Token, `{
		"artifactId": "cross-agent-artifact"
	}`))
	if completeResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-agent task complete status %d, got %d with body %s", http.StatusForbidden, completeResponse.Code, completeResponse.Body.String())
	}

	allowedPoll := httptest.NewRecorder()
	router.ServeHTTP(allowedPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId="+billingAgentID, billingToken.Token, ""))
	if allowedPoll.Code != http.StatusOK {
		t.Fatalf("expected own-agent task poll status %d, got %d with body %s", http.StatusOK, allowedPoll.Code, allowedPoll.Body.String())
	}
	var tasks []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(allowedPoll.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected own-agent task poll JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != created.ID {
		t.Fatalf("expected billing token to lease its own task, got %#v", tasks)
	}
}

func TestCreateProfileTaskRejectsInvalidProfileSeconds(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	tests := []struct {
		name           string
		profileSeconds int
	}{
		{name: "zero", profileSeconds: 0},
		{name: "too high", profileSeconds: 301},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createResponse := httptest.NewRecorder()
			body := strings.NewReader(fmt.Sprintf(`{
				"agentId": "agent-checkout-01",
				"runId": "run-profile-task-invalid-%s",
				"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
				"profileType": "cpu",
				"profileSeconds": %d
			}`, strings.ReplaceAll(tt.name, " ", "-"), tt.profileSeconds))
			router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", body))

			if createResponse.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid profile seconds status %d, got %d with body %s", http.StatusBadRequest, createResponse.Code, createResponse.Body.String())
			}
			if !strings.Contains(createResponse.Body.String(), "profileSeconds must be between 1 and 300") {
				t.Fatalf("expected profile seconds validation error, got body %s", createResponse.Body.String())
			}
		})
	}
}

func TestCreateProfileTaskRejectsInvalidSource(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-profile-task-invalid-source",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "cpu",
		"profileSeconds": 1,
		"source": "cron"
	}`)))

	if createResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid source status %d, got %d with body %s", http.StatusBadRequest, createResponse.Code, createResponse.Body.String())
	}
	if !strings.Contains(createResponse.Body.String(), "source must be api, run_auto, target_manual, or threshold_auto") {
		t.Fatalf("expected source validation error, got body %s", createResponse.Body.String())
	}
}

func TestProfileTaskLifecycleCreatesAndLeasesCommandProfilerTask(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)
	registerScopedAgentWithID(t, router, token, "agent-checkout-01", "checkout-01")

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-command-profile-task-1",
		"targetName": "checkout-01",
		"profileType": "perf",
		"profileSeconds": 30,
		"profileCommand": "perf",
		"profileCommandArgs": ["record", "-F", "99", "-p", "1234", "-g", "-o", "{{output}}", "--", "sleep", "{{seconds}}"],
		"profileCommandOutput": "/tmp/checkout-perf.data",
		"profileCommandTimeoutMs": 45000
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected command task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID                      string   `json:"id"`
		Status                  string   `json:"status"`
		ProfileType             string   `json:"profileType"`
		ProfileSeconds          int      `json:"profileSeconds"`
		ProfileCommand          string   `json:"profileCommand"`
		ProfileCommandArgs      []string `json:"profileCommandArgs"`
		ProfileCommandOutput    string   `json:"profileCommandOutput"`
		ProfileCommandTimeoutMs int      `json:"profileCommandTimeoutMs"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected command task create JSON, got decode error: %v", err)
	}
	if created.ID == "" || created.Status != "pending" || created.ProfileType != "perf" || created.ProfileSeconds != 30 {
		t.Fatalf("expected pending command task metadata, got %#v", created)
	}
	if created.ProfileCommand != "perf" || created.ProfileCommandOutput != "/tmp/checkout-perf.data" || created.ProfileCommandTimeoutMs != 45000 {
		t.Fatalf("expected command profiler fields to be persisted, got %#v", created)
	}
	if len(created.ProfileCommandArgs) != 11 || created.ProfileCommandArgs[7] != "{{output}}" || created.ProfileCommandArgs[10] != "{{seconds}}" {
		t.Fatalf("expected command profiler args to be persisted, got %#v", created.ProfileCommandArgs)
	}

	pollResponse := httptest.NewRecorder()
	router.ServeHTTP(pollResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if pollResponse.Code != http.StatusOK {
		t.Fatalf("expected task poll status %d, got %d with body %s", http.StatusOK, pollResponse.Code, pollResponse.Body.String())
	}
	var tasks []struct {
		ID                      string   `json:"id"`
		Status                  string   `json:"status"`
		ProfileCommand          string   `json:"profileCommand"`
		ProfileCommandArgs      []string `json:"profileCommandArgs"`
		ProfileCommandOutput    string   `json:"profileCommandOutput"`
		ProfileCommandTimeoutMs int      `json:"profileCommandTimeoutMs"`
		LeasedAt                string   `json:"leasedAt"`
	}
	if err := json.NewDecoder(pollResponse.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected task poll JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != created.ID || tasks[0].Status != "leased" || tasks[0].LeasedAt == "" {
		t.Fatalf("expected command task to be leased, got %#v", tasks)
	}
	if tasks[0].ProfileCommand != created.ProfileCommand || tasks[0].ProfileCommandOutput != created.ProfileCommandOutput || tasks[0].ProfileCommandTimeoutMs != created.ProfileCommandTimeoutMs {
		t.Fatalf("expected leased command profiler fields, got %#v", tasks[0])
	}
	if len(tasks[0].ProfileCommandArgs) != len(created.ProfileCommandArgs) || tasks[0].ProfileCommandArgs[7] != "{{output}}" {
		t.Fatalf("expected leased command profiler args, got %#v", tasks[0].ProfileCommandArgs)
	}
}

func TestProfileTaskLeaseExpiresAndCanBeReclaimed(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	t.Setenv("PROFILE_TASK_LEASE_TIMEOUT_MS", "1")
	router := NewRouter()
	token := createAgentToken(t, router)
	registerScopedAgentWithID(t, router, token, "agent-checkout-01", "checkout-01")

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-profile-task-timeout",
		"scenarioName": "checkout-smoke",
		"targetName": "checkout-01",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "goroutine",
		"profileSeconds": 1
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected task create JSON, got decode error: %v", err)
	}

	firstPoll := httptest.NewRecorder()
	router.ServeHTTP(firstPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if firstPoll.Code != http.StatusOK {
		t.Fatalf("expected first poll status %d, got %d with body %s", http.StatusOK, firstPoll.Code, firstPoll.Body.String())
	}
	var firstTasks []struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		LeasedAt       string `json:"leasedAt"`
		LeaseExpiresAt string `json:"leaseExpiresAt"`
	}
	if err := json.NewDecoder(firstPoll.Body).Decode(&firstTasks); err != nil {
		t.Fatalf("expected first poll JSON, got decode error: %v", err)
	}
	if len(firstTasks) != 1 || firstTasks[0].ID != created.ID || firstTasks[0].Status != "leased" || firstTasks[0].LeasedAt == "" || firstTasks[0].LeaseExpiresAt == "" {
		t.Fatalf("expected first poll to lease task, got %#v", firstTasks)
	}
	firstLeasedAt, err := time.Parse(time.RFC3339Nano, firstTasks[0].LeasedAt)
	if err != nil {
		t.Fatalf("expected leasedAt to be RFC3339Nano, got %q: %v", firstTasks[0].LeasedAt, err)
	}
	firstLeaseExpiresAt, err := time.Parse(time.RFC3339Nano, firstTasks[0].LeaseExpiresAt)
	if err != nil {
		t.Fatalf("expected leaseExpiresAt to be RFC3339Nano, got %q: %v", firstTasks[0].LeaseExpiresAt, err)
	}
	if !firstLeaseExpiresAt.Equal(firstLeasedAt.Add(time.Millisecond)) {
		t.Fatalf("expected leaseExpiresAt to equal leasedAt plus timeout, leasedAt=%q leaseExpiresAt=%q", firstTasks[0].LeasedAt, firstTasks[0].LeaseExpiresAt)
	}

	time.Sleep(5 * time.Millisecond)

	secondPoll := httptest.NewRecorder()
	router.ServeHTTP(secondPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if secondPoll.Code != http.StatusOK {
		t.Fatalf("expected second poll status %d, got %d with body %s", http.StatusOK, secondPoll.Code, secondPoll.Body.String())
	}
	var secondTasks []struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		LeasedAt       string `json:"leasedAt"`
		LeaseExpiresAt string `json:"leaseExpiresAt"`
	}
	if err := json.NewDecoder(secondPoll.Body).Decode(&secondTasks); err != nil {
		t.Fatalf("expected second poll JSON, got decode error: %v", err)
	}
	if len(secondTasks) != 1 || secondTasks[0].ID != created.ID || secondTasks[0].Status != "leased" || secondTasks[0].LeaseExpiresAt == "" {
		t.Fatalf("expected expired lease to be reclaimed, got %#v", secondTasks)
	}
	if secondTasks[0].LeasedAt == "" || secondTasks[0].LeasedAt == firstTasks[0].LeasedAt {
		t.Fatalf("expected reclaimed task to receive a new leasedAt, first=%q second=%q", firstTasks[0].LeasedAt, secondTasks[0].LeasedAt)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?status=leased&limit=1", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected leased task list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var listedTasks []struct {
		ID             string `json:"id"`
		LeaseExpiresAt string `json:"leaseExpiresAt"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listedTasks); err != nil {
		t.Fatalf("expected leased task list JSON, got decode error: %v", err)
	}
	if len(listedTasks) != 1 || listedTasks[0].ID != created.ID || listedTasks[0].LeaseExpiresAt != secondTasks[0].LeaseExpiresAt {
		t.Fatalf("expected list response to expose the leased task expiry, got %#v and poll expiry %q", listedTasks, secondTasks[0].LeaseExpiresAt)
	}
}

func TestListProfileTasksFiltersByRunAgentStatusAndLimit(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	for _, body := range []string{
		`{
			"agentId": "agent-checkout-01",
			"runId": "run-profile-filter-1",
			"targetName": "checkout-01",
			"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
			"profileType": "cpu",
			"profileSeconds": 1,
			"source": "target_manual"
		}`,
		`{
			"agentId": "agent-checkout-02",
			"runId": "run-profile-filter-1",
			"targetName": "checkout-02",
			"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
			"profileType": "heap",
			"profileSeconds": 1
		}`,
		`{
			"agentId": "agent-checkout-01",
			"runId": "run-profile-filter-2",
			"targetName": "checkout-01",
			"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
			"profileType": "goroutine",
			"profileSeconds": 1
		}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("expected profile task create status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
		}
	}

	filteredResponse := httptest.NewRecorder()
	router.ServeHTTP(filteredResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?runId=run-profile-filter-1&agentId=agent-checkout-01&status=pending&limit=1", nil))
	if filteredResponse.Code != http.StatusOK {
		t.Fatalf("expected filtered task list status %d, got %d with body %s", http.StatusOK, filteredResponse.Code, filteredResponse.Body.String())
	}
	var filtered []profileTaskRecord
	if err := json.NewDecoder(filteredResponse.Body).Decode(&filtered); err != nil {
		t.Fatalf("expected filtered task list JSON, got decode error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected one filtered task, got %#v", filtered)
	}
	if filtered[0].RunID != "run-profile-filter-1" || filtered[0].AgentID != "agent-checkout-01" || filtered[0].Status != "pending" || filtered[0].ProfileType != "cpu" {
		t.Fatalf("expected filtered task to match run, agent, and status, got %#v", filtered[0])
	}

	sourceFilteredResponse := httptest.NewRecorder()
	router.ServeHTTP(sourceFilteredResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?runId=run-profile-filter-1&source=target_manual&limit=5", nil))
	if sourceFilteredResponse.Code != http.StatusOK {
		t.Fatalf("expected source-filtered task list status %d, got %d with body %s", http.StatusOK, sourceFilteredResponse.Code, sourceFilteredResponse.Body.String())
	}
	var sourceFiltered []profileTaskRecord
	if err := json.NewDecoder(sourceFilteredResponse.Body).Decode(&sourceFiltered); err != nil {
		t.Fatalf("expected source-filtered task list JSON, got decode error: %v", err)
	}
	if len(sourceFiltered) != 1 || sourceFiltered[0].Source != targetManualProfileTaskSource || sourceFiltered[0].AgentID != "agent-checkout-01" {
		t.Fatalf("expected one target_manual task, got %#v", sourceFiltered)
	}

	invalidSourceResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidSourceResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?source=cron", nil))
	if invalidSourceResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid source filter status %d, got %d with body %s", http.StatusBadRequest, invalidSourceResponse.Code, invalidSourceResponse.Body.String())
	}

	invalidLimitResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidLimitResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?limit=0", nil))
	if invalidLimitResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid limit status %d, got %d with body %s", http.StatusBadRequest, invalidLimitResponse.Code, invalidLimitResponse.Body.String())
	}
}

func TestProfileTaskFailureRetriesUntilMaxAttempts(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)
	registerScopedAgentWithID(t, router, token, "agent-checkout-01", "checkout-01")

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-profile-task-retry",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "heap",
		"profileSeconds": 1,
		"maxAttempts": 2
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID          string `json:"id"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"maxAttempts"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected task create JSON, got decode error: %v", err)
	}
	if created.Attempts != 0 || created.MaxAttempts != 2 {
		t.Fatalf("expected created task attempts 0/2, got %#v", created)
	}

	firstPoll := httptest.NewRecorder()
	router.ServeHTTP(firstPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if firstPoll.Code != http.StatusOK {
		t.Fatalf("expected first poll status %d, got %d with body %s", http.StatusOK, firstPoll.Code, firstPoll.Body.String())
	}

	firstFailure := httptest.NewRecorder()
	router.ServeHTTP(firstFailure, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/"+created.ID+"/complete", token, `{
		"error": "pprof endpoint timed out"
	}`))
	if firstFailure.Code != http.StatusOK {
		t.Fatalf("expected first failure status %d, got %d with body %s", http.StatusOK, firstFailure.Code, firstFailure.Body.String())
	}
	var retryable struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"maxAttempts"`
		Error       string `json:"error"`
		LeasedAt    string `json:"leasedAt"`
		CompletedAt string `json:"completedAt"`
	}
	if err := json.NewDecoder(firstFailure.Body).Decode(&retryable); err != nil {
		t.Fatalf("expected first failure JSON, got decode error: %v", err)
	}
	if retryable.ID != created.ID || retryable.Status != "pending" || retryable.Attempts != 1 || retryable.MaxAttempts != 2 {
		t.Fatalf("expected retryable failure to return pending with attempts 1/2, got %#v", retryable)
	}
	if retryable.Error != "pprof endpoint timed out" || retryable.LeasedAt != "" || retryable.CompletedAt != "" {
		t.Fatalf("expected retryable failure to keep last error and clear lease/completion fields, got %#v", retryable)
	}

	secondPoll := httptest.NewRecorder()
	router.ServeHTTP(secondPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if secondPoll.Code != http.StatusOK {
		t.Fatalf("expected second poll status %d, got %d with body %s", http.StatusOK, secondPoll.Code, secondPoll.Body.String())
	}
	var secondTasks []struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"maxAttempts"`
	}
	if err := json.NewDecoder(secondPoll.Body).Decode(&secondTasks); err != nil {
		t.Fatalf("expected second poll JSON, got decode error: %v", err)
	}
	if len(secondTasks) != 1 || secondTasks[0].ID != created.ID || secondTasks[0].Status != "leased" || secondTasks[0].Attempts != 1 || secondTasks[0].MaxAttempts != 2 {
		t.Fatalf("expected pending retry to be leased with attempts 1/2, got %#v", secondTasks)
	}

	finalFailure := httptest.NewRecorder()
	router.ServeHTTP(finalFailure, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/"+created.ID+"/complete", token, `{
		"error": "pprof endpoint still unavailable"
	}`))
	if finalFailure.Code != http.StatusOK {
		t.Fatalf("expected final failure status %d, got %d with body %s", http.StatusOK, finalFailure.Code, finalFailure.Body.String())
	}
	var failed struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"maxAttempts"`
		Error       string `json:"error"`
		CompletedAt string `json:"completedAt"`
	}
	if err := json.NewDecoder(finalFailure.Body).Decode(&failed); err != nil {
		t.Fatalf("expected final failure JSON, got decode error: %v", err)
	}
	if failed.ID != created.ID || failed.Status != "failed" || failed.Attempts != 2 || failed.MaxAttempts != 2 {
		t.Fatalf("expected final failure to be failed with attempts 2/2, got %#v", failed)
	}
	if failed.Error != "pprof endpoint still unavailable" || failed.CompletedAt == "" {
		t.Fatalf("expected final failure to persist last error and completedAt, got %#v", failed)
	}

	thirdPoll := httptest.NewRecorder()
	router.ServeHTTP(thirdPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if thirdPoll.Code != http.StatusOK {
		t.Fatalf("expected third poll status %d, got %d with body %s", http.StatusOK, thirdPoll.Code, thirdPoll.Body.String())
	}
	var thirdTasks []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(thirdPoll.Body).Decode(&thirdTasks); err != nil {
		t.Fatalf("expected third poll JSON, got decode error: %v", err)
	}
	if len(thirdTasks) != 0 {
		t.Fatalf("expected failed task to stop polling, got %#v", thirdTasks)
	}
}

func TestRetryFailedProfileTaskRequeuesTask(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createAgentToken(t, router)
	registerScopedAgentWithID(t, router, token, "agent-checkout-01", "checkout-01")

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "agent-checkout-01",
		"runId": "run-profile-task-requeue",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "heap",
		"profileSeconds": 1,
		"maxAttempts": 1
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected task create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected task create JSON, got decode error: %v", err)
	}

	pollResponse := httptest.NewRecorder()
	router.ServeHTTP(pollResponse, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if pollResponse.Code != http.StatusOK {
		t.Fatalf("expected task poll status %d, got %d with body %s", http.StatusOK, pollResponse.Code, pollResponse.Body.String())
	}

	failResponse := httptest.NewRecorder()
	router.ServeHTTP(failResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/"+created.ID+"/complete", token, `{
		"error": "profile endpoint returned HTTP 500"
	}`))
	if failResponse.Code != http.StatusOK {
		t.Fatalf("expected task failure status %d, got %d with body %s", http.StatusOK, failResponse.Code, failResponse.Body.String())
	}
	var failed profileTaskRecord
	if err := json.NewDecoder(failResponse.Body).Decode(&failed); err != nil {
		t.Fatalf("expected failed task JSON, got decode error: %v", err)
	}
	if failed.Status != "failed" || failed.Attempts != 1 || failed.CompletedAt == "" || failed.Error == "" {
		t.Fatalf("expected task to fail before retry, got %#v", failed)
	}

	retryResponse := httptest.NewRecorder()
	router.ServeHTTP(retryResponse, httptest.NewRequest(http.MethodPost, "/api/profile-tasks/"+created.ID+"/retry", nil))
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("expected retry status %d, got %d with body %s", http.StatusOK, retryResponse.Code, retryResponse.Body.String())
	}
	var retried profileTaskRecord
	if err := json.NewDecoder(retryResponse.Body).Decode(&retried); err != nil {
		t.Fatalf("expected retried task JSON, got decode error: %v", err)
	}
	if retried.ID != created.ID || retried.Status != "pending" || retried.Attempts != 0 || retried.MaxAttempts != 1 {
		t.Fatalf("expected retry to requeue task with attempts reset, got %#v", retried)
	}
	if retried.Error != "" || retried.ArtifactID != "" || retried.LeasedAt != "" || retried.CompletedAt != "" {
		t.Fatalf("expected retry to clear error, artifact, lease, and completion fields, got %#v", retried)
	}

	secondPoll := httptest.NewRecorder()
	router.ServeHTTP(secondPoll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId=agent-checkout-01", token, ""))
	if secondPoll.Code != http.StatusOK {
		t.Fatalf("expected second poll status %d, got %d with body %s", http.StatusOK, secondPoll.Code, secondPoll.Body.String())
	}
	var tasks []profileTaskRecord
	if err := json.NewDecoder(secondPoll.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected second poll JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != created.ID || tasks[0].Status != "leased" || tasks[0].Attempts != 0 {
		t.Fatalf("expected retried task to be leased again, got %#v", tasks)
	}
}
