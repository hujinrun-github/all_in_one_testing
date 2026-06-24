package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestWorkspaceScopeHeadersFilterListEndpoints(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createWorkspaceScenario(t, router, "checkout-scenario", "project-checkout", "staging")
	createWorkspaceScenario(t, router, "billing-scenario", "project-billing", "prod")
	createWorkspaceTarget(t, router, "checkout-target", "project-checkout", "staging")
	createWorkspaceTarget(t, router, "billing-target", "project-billing", "prod")
	createWorkspaceAgent(t, router, "checkout-agent", "project-checkout", "staging")
	createWorkspaceAgent(t, router, "billing-agent", "project-billing", "prod")

	scenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(scenarioResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/scenarios", "", "project-checkout", "staging"))
	if scenarioResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped scenario list status %d, got %d with body %s", http.StatusOK, scenarioResponse.Code, scenarioResponse.Body.String())
	}
	var scenarios []struct {
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(scenarioResponse.Body).Decode(&scenarios); err != nil {
		t.Fatalf("expected scenario list JSON, got decode error: %v", err)
	}
	if len(scenarios) != 1 || scenarios[0].Name != "checkout-scenario" {
		t.Fatalf("expected only checkout scenario in scoped list, got %#v", scenarios)
	}
	if scenarios[0].ProjectID != "project-checkout" || scenarios[0].Environment != "staging" {
		t.Fatalf("expected scoped scenario metadata, got %#v", scenarios[0])
	}

	targetResponse := httptest.NewRecorder()
	router.ServeHTTP(targetResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/targets", "", "project-checkout", "staging"))
	if targetResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped target list status %d, got %d with body %s", http.StatusOK, targetResponse.Code, targetResponse.Body.String())
	}
	var targets []struct {
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(targetResponse.Body).Decode(&targets); err != nil {
		t.Fatalf("expected target list JSON, got decode error: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "checkout-target" {
		t.Fatalf("expected only checkout target in scoped list, got %#v", targets)
	}
	if targets[0].ProjectID != "project-checkout" || targets[0].Environment != "staging" {
		t.Fatalf("expected scoped target metadata, got %#v", targets[0])
	}

	agentResponse := httptest.NewRecorder()
	router.ServeHTTP(agentResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/agents", "", "project-checkout", "staging"))
	if agentResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped agent list status %d, got %d with body %s", http.StatusOK, agentResponse.Code, agentResponse.Body.String())
	}
	var agents []struct {
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(agentResponse.Body).Decode(&agents); err != nil {
		t.Fatalf("expected agent list JSON, got decode error: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "checkout-agent" {
		t.Fatalf("expected only checkout agent in scoped list, got %#v", agents)
	}
	if agents[0].ProjectID != "project-checkout" || agents[0].Environment != "staging" {
		t.Fatalf("expected scoped agent metadata, got %#v", agents[0])
	}
}

func TestWorkspaceScopeHeadersRejectCrossScopeWrites(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	for _, testCase := range []struct {
		name string
		path string
		body string
	}{
		{
			name: "scenario",
			path: "/api/scenarios",
			body: `{
				"name": "billing-scenario",
				"projectId": "project-billing",
				"environment": "prod",
				"protocol": "HTTP",
				"method": "GET",
				"baseUrl": "http://billing.internal:8080",
				"path": "/ready"
			}`,
		},
		{
			name: "target",
			path: "/api/targets",
			body: `{
				"name": "billing-target",
				"projectId": "project-billing",
				"environment": "prod",
				"baseUrl": "http://billing.internal:8080"
			}`,
		},
		{
			name: "agent token",
			path: "/api/agent-tokens",
			body: `{
				"name": "billing-agent-token",
				"projectId": "project-billing",
				"environment": "prod"
			}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodPost, testCase.path, testCase.body, "project-checkout", "staging"))
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected cross-scope write status %d, got %d with body %s", http.StatusForbidden, response.Code, response.Body.String())
			}
		})
	}
}

func TestWorkspaceScopeHeadersPopulateMissingCreateScope(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodPost, "/api/scenarios", `{
		"name": "checkout-scenario",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://checkout.internal:8080",
		"path": "/ready"
	}`, "project-checkout", "staging"))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected scoped scenario creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}
	var scenario struct {
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(response.Body).Decode(&scenario); err != nil {
		t.Fatalf("expected scenario JSON, got decode error: %v", err)
	}
	if scenario.ProjectID != "project-checkout" || scenario.Environment != "staging" {
		t.Fatalf("expected missing scope to be populated from headers, got %#v", scenario)
	}
}

func TestWorkspaceScopeHeadersRejectCrossScopeSingleResourceOperations(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	billingScenarioID := createWorkspaceScenario(t, router, "billing-scenario", "project-billing", "prod")
	billingTargetID := createWorkspaceTarget(t, router, "billing-target", "project-billing", "prod")

	for _, testCase := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "get scenario",
			method: http.MethodGet,
			path:   "/api/scenarios/" + billingScenarioID,
		},
		{
			name:   "update scenario",
			method: http.MethodPut,
			path:   "/api/scenarios/" + billingScenarioID,
			body: `{
				"name": "billing-scenario-updated",
				"projectId": "project-billing",
				"environment": "prod",
				"protocol": "HTTP",
				"method": "GET",
				"baseUrl": "http://billing.internal:8080",
				"path": "/ready"
			}`,
		},
		{
			name:   "delete scenario",
			method: http.MethodDelete,
			path:   "/api/scenarios/" + billingScenarioID,
		},
		{
			name:   "get target",
			method: http.MethodGet,
			path:   "/api/targets/" + billingTargetID,
		},
		{
			name:   "update target",
			method: http.MethodPut,
			path:   "/api/targets/" + billingTargetID,
			body: `{
				"name": "billing-target-updated",
				"projectId": "project-billing",
				"environment": "prod",
				"baseUrl": "http://billing.internal:8080"
			}`,
		},
		{
			name:   "delete target",
			method: http.MethodDelete,
			path:   "/api/targets/" + billingTargetID,
		},
		{
			name:   "check target health",
			method: http.MethodPost,
			path:   "/api/targets/" + billingTargetID + "/health-check",
		},
		{
			name:   "list target health history",
			method: http.MethodGet,
			path:   "/api/targets/" + billingTargetID + "/health-checks?limit=5",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newWorkspaceScopedAPIRequest(testCase.method, testCase.path, testCase.body, "project-checkout", "staging"))
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected cross-scope %s status %d, got %d with body %s", testCase.name, http.StatusForbidden, response.Code, response.Body.String())
			}
		})
	}
}

func TestWorkspaceScopeHeadersFilterRunEndpoints(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	store := newRunHistoryStoreFromEnv()

	checkoutRun := createRunResponse{
		ID:            "run-checkout",
		Name:          "checkout run",
		ProjectID:     "project-checkout",
		Environment:   "staging",
		Status:        "running",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://checkout.internal/ready",
		TotalRequests: 10,
		CreatedAt:     "2026-06-12T09:00:00Z",
	}
	billingRun := createRunResponse{
		ID:            "run-billing",
		Name:          "billing run",
		ProjectID:     "project-billing",
		Environment:   "prod",
		Status:        "running",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://billing.internal/ready",
		TotalRequests: 10,
		CreatedAt:     "2026-06-12T09:01:00Z",
	}
	if err := store.save(checkoutRun); err != nil {
		t.Fatalf("expected checkout run save to succeed, got %v", err)
	}
	if err := store.save(billingRun); err != nil {
		t.Fatalf("expected billing run save to succeed, got %v", err)
	}
	for _, event := range []runEventRecord{
		{
			ID:            "event-checkout-started",
			RunID:         checkoutRun.ID,
			Type:          "run_started",
			Status:        "running",
			Message:       "Run started",
			TotalRequests: checkoutRun.TotalRequests,
			CreatedAt:     "2026-06-12T09:00:00Z",
		},
		{
			ID:            "event-billing-finished",
			RunID:         billingRun.ID,
			Type:          "run_finished",
			Status:        "finished",
			Message:       "Run finished",
			TotalRequests: billingRun.TotalRequests,
			CreatedAt:     "2026-06-12T09:01:00Z",
		},
	} {
		if err := store.appendEvent(event); err != nil {
			t.Fatalf("expected run event save to succeed, got %v", err)
		}
	}

	router := NewRouter()

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/runs", "", "project-checkout", "staging"))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped run list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var runs []struct {
		ID          string `json:"id"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&runs); err != nil {
		t.Fatalf("expected run list JSON, got decode error: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != checkoutRun.ID {
		t.Fatalf("expected only checkout run in scoped list, got %#v", runs)
	}
	if runs[0].ProjectID != checkoutRun.ProjectID || runs[0].Environment != checkoutRun.Environment {
		t.Fatalf("expected scoped run metadata, got %#v", runs[0])
	}

	for _, testCase := range []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "get run",
			method: http.MethodGet,
			path:   "/api/runs/" + billingRun.ID,
		},
		{
			name:   "list run events",
			method: http.MethodGet,
			path:   "/api/runs/" + billingRun.ID + "/events",
		},
		{
			name:   "stream run events",
			method: http.MethodGet,
			path:   "/api/runs/" + billingRun.ID + "/events/stream",
		},
		{
			name:   "stop run",
			method: http.MethodPost,
			path:   "/api/runs/" + billingRun.ID + "/stop",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newWorkspaceScopedAPIRequest(testCase.method, testCase.path, "", "project-checkout", "staging"))
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected cross-scope %s status %d, got %d with body %s", testCase.name, http.StatusForbidden, response.Code, response.Body.String())
			}
		})
	}

	queryScopedStreamResponse := httptest.NewRecorder()
	router.ServeHTTP(queryScopedStreamResponse, httptest.NewRequest(http.MethodGet, "/api/runs/"+billingRun.ID+"/events/stream?projectId=project-checkout&environment=staging", nil))
	if queryScopedStreamResponse.Code != http.StatusForbidden {
		t.Fatalf("expected query-scoped cross-scope run event stream status %d, got %d with body %s", http.StatusForbidden, queryScopedStreamResponse.Code, queryScopedStreamResponse.Body.String())
	}
}

func TestWorkspaceScopeHeadersConstrainRunCreation(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	router := NewRouter()

	checkoutScenarioID := createWorkspaceScenarioWithBaseURL(t, router, "checkout-scenario", "project-checkout", "staging", upstream.URL)
	billingScenarioID := createWorkspaceScenarioWithBaseURL(t, router, "billing-scenario", "project-billing", "prod", upstream.URL)
	billingTargetID := createWorkspaceTargetWithBaseURL(t, router, "billing-target", "project-billing", "prod", upstream.URL)

	for _, testCase := range []struct {
		name string
		body string
	}{
		{
			name: "billing scenario",
			body: `{
				"scenarioId": "` + billingScenarioID + `",
				"totalRequests": 1,
				"concurrency": 1,
				"timeoutMs": 50
			}`,
		},
		{
			name: "billing target",
			body: `{
				"scenarioId": "` + checkoutScenarioID + `",
				"targetId": "` + billingTargetID + `",
				"totalRequests": 1,
				"concurrency": 1,
				"timeoutMs": 50
			}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodPost, "/api/runs", testCase.body, "project-checkout", "staging"))
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected cross-scope run creation status %d, got %d with body %s", http.StatusForbidden, response.Code, response.Body.String())
			}
		})
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodPost, "/api/runs", `{
		"name": "ad-hoc checkout run",
		"method": "GET",
		"url": "`+upstream.URL+`/ready",
		"totalRequests": 1,
		"concurrency": 1,
		"timeoutMs": 50
	}`, "project-checkout", "staging"))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected scoped ad-hoc run creation status %d, got %d with body %s", http.StatusAccepted, response.Code, response.Body.String())
	}
	var run struct {
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatalf("expected run JSON, got decode error: %v", err)
	}
	if run.ProjectID != "project-checkout" || run.Environment != "staging" {
		t.Fatalf("expected ad-hoc run scope from headers, got %#v", run)
	}
}

func TestWorkspaceScopeHeadersFilterReportEndpoints(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	store := newRunHistoryStoreFromEnv()

	checkoutRun := createRunResponse{
		ID:              "report-run-checkout",
		Name:            "checkout report run",
		ScenarioName:    "checkout scenario",
		ProjectID:       "project-checkout",
		Environment:     "staging",
		Status:          "finished",
		Protocol:        "HTTP",
		Method:          "GET",
		URL:             "http://checkout.internal/ready",
		TotalRequests:   10,
		SuccessRequests: 10,
		DurationMs:      100,
		QPS:             100,
		P95LatencyMs:    10,
		CreatedAt:       "2026-06-12T09:00:00Z",
	}
	billingRun := createRunResponse{
		ID:              "report-run-billing",
		Name:            "billing report run",
		ScenarioName:    "billing scenario",
		ProjectID:       "project-billing",
		Environment:     "prod",
		Status:          "finished",
		Protocol:        "HTTP",
		Method:          "GET",
		URL:             "http://billing.internal/ready",
		TotalRequests:   10,
		SuccessRequests: 10,
		DurationMs:      50,
		QPS:             200,
		P95LatencyMs:    5,
		CreatedAt:       "2026-06-12T09:01:00Z",
	}
	if err := store.save(checkoutRun); err != nil {
		t.Fatalf("expected checkout report run save to succeed, got %v", err)
	}
	if err := store.save(billingRun); err != nil {
		t.Fatalf("expected billing report run save to succeed, got %v", err)
	}

	router := NewRouter()

	detailResponse := httptest.NewRecorder()
	router.ServeHTTP(detailResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/reports/runs/"+billingRun.ID, "", "project-checkout", "staging"))
	if detailResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-scope report detail status %d, got %d with body %s", http.StatusForbidden, detailResponse.Code, detailResponse.Body.String())
	}

	latestResponse := httptest.NewRecorder()
	router.ServeHTTP(latestResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/reports/latest", "", "project-checkout", "staging"))
	if latestResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped latest report status %d, got %d with body %s", http.StatusOK, latestResponse.Code, latestResponse.Body.String())
	}
	var latestReport struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.NewDecoder(latestResponse.Body).Decode(&latestReport); err != nil {
		t.Fatalf("expected latest report JSON, got decode error: %v", err)
	}
	if latestReport.Run.ID != checkoutRun.ID {
		t.Fatalf("expected latest scoped report to use checkout run, got %#v", latestReport.Run)
	}

	compareResponse := httptest.NewRecorder()
	router.ServeHTTP(compareResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/reports/compare?limit=2", "", "project-checkout", "staging"))
	if compareResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped comparison status %d, got %d with body %s", http.StatusOK, compareResponse.Code, compareResponse.Body.String())
	}
	var comparison struct {
		Runs []struct {
			ID string `json:"id"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(compareResponse.Body).Decode(&comparison); err != nil {
		t.Fatalf("expected comparison report JSON, got decode error: %v", err)
	}
	if len(comparison.Runs) != 1 || comparison.Runs[0].ID != checkoutRun.ID {
		t.Fatalf("expected scoped comparison to include only checkout run, got %#v", comparison.Runs)
	}

	for _, path := range []string{
		"/api/reports/compare?runIds=" + checkoutRun.ID + "," + billingRun.ID,
		"/api/reports/profile-artifacts/compare?runIds=" + checkoutRun.ID + "," + billingRun.ID,
		"/api/reports/process-trends/compare?runIds=" + checkoutRun.ID + "," + billingRun.ID,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodGet, path, "", "project-checkout", "staging"))
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected cross-scope comparison %s status %d, got %d with body %s", path, http.StatusForbidden, response.Code, response.Body.String())
		}
	}
}

func TestWorkspaceScopeHeadersFilterProfileTaskEndpoints(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	runStore := newRunHistoryStoreFromEnv()
	taskStore := newProfileTaskStoreFromEnv()

	checkoutRun := createRunResponse{
		ID:            "profile-task-run-checkout",
		Name:          "checkout profile task run",
		ProjectID:     "project-checkout",
		Environment:   "staging",
		Status:        "finished",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://checkout.internal/ready",
		TotalRequests: 1,
		CreatedAt:     "2026-06-12T09:00:00Z",
	}
	billingRun := createRunResponse{
		ID:            "profile-task-run-billing",
		Name:          "billing profile task run",
		ProjectID:     "project-billing",
		Environment:   "prod",
		Status:        "finished",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://billing.internal/ready",
		TotalRequests: 1,
		CreatedAt:     "2026-06-12T09:01:00Z",
	}
	if err := runStore.save(checkoutRun); err != nil {
		t.Fatalf("expected checkout run save to succeed, got %v", err)
	}
	if err := runStore.save(billingRun); err != nil {
		t.Fatalf("expected billing run save to succeed, got %v", err)
	}

	checkoutTask := createWorkspaceProfileTask(t, taskStore, checkoutRun.ID, "agent-checkout")
	billingTask := createWorkspaceProfileTask(t, taskStore, billingRun.ID, "agent-billing")
	failedBillingTask, found, err := taskStore.complete(billingTask.ID, completeProfileTaskRequest{Error: "profile failed"})
	if err != nil || !found {
		t.Fatalf("expected billing profile task failure to persist, found=%v err=%v", found, err)
	}
	if failedBillingTask.Status != "failed" {
		t.Fatalf("expected billing profile task to be failed, got %#v", failedBillingTask)
	}

	router := NewRouter()

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/profile-tasks", "", "project-checkout", "staging"))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped profile task list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var tasks []struct {
		ID    string `json:"id"`
		RunID string `json:"runId"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected profile task list JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != checkoutTask.ID || tasks[0].RunID != checkoutRun.ID {
		t.Fatalf("expected only checkout profile task in scoped list, got %#v", tasks)
	}

	crossRunListResponse := httptest.NewRecorder()
	router.ServeHTTP(crossRunListResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/profile-tasks?runId="+billingRun.ID, "", "project-checkout", "staging"))
	if crossRunListResponse.Code != http.StatusOK {
		t.Fatalf("expected cross-scope filtered profile task list status %d, got %d with body %s", http.StatusOK, crossRunListResponse.Code, crossRunListResponse.Body.String())
	}
	var crossRunTasks []profileTaskRecord
	if err := json.NewDecoder(crossRunListResponse.Body).Decode(&crossRunTasks); err != nil {
		t.Fatalf("expected cross-scope profile task list JSON, got decode error: %v", err)
	}
	if len(crossRunTasks) != 0 {
		t.Fatalf("expected cross-scope run filter to return no profile tasks, got %#v", crossRunTasks)
	}

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, newWorkspaceScopedAPIRequest(http.MethodPost, "/api/profile-tasks", `{
		"agentId": "agent-billing",
		"runId": "`+billingRun.ID+`",
		"pprofBaseUrl": "http://billing.internal:6060/debug/pprof",
		"profileType": "cpu",
		"profileSeconds": 1
	}`, "project-checkout", "staging"))
	if createResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-scope profile task creation status %d, got %d with body %s", http.StatusForbidden, createResponse.Code, createResponse.Body.String())
	}

	retryResponse := httptest.NewRecorder()
	router.ServeHTTP(retryResponse, newWorkspaceScopedAPIRequest(http.MethodPost, "/api/profile-tasks/"+failedBillingTask.ID+"/retry", "", "project-checkout", "staging"))
	if retryResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-scope profile task retry status %d, got %d with body %s", http.StatusForbidden, retryResponse.Code, retryResponse.Body.String())
	}
}

func TestWorkspaceScopeHeadersFilterProfileArtifactEndpoints(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	runStore := newRunHistoryStoreFromEnv()
	artifactStore := newProfileArtifactStoreFromEnv()

	checkoutRun := createRunResponse{
		ID:            "profile-artifact-run-checkout",
		Name:          "checkout profile artifact run",
		ProjectID:     "project-checkout",
		Environment:   "staging",
		Status:        "finished",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://checkout.internal/ready",
		TotalRequests: 1,
		CreatedAt:     "2026-06-12T09:00:00Z",
	}
	billingRun := createRunResponse{
		ID:            "profile-artifact-run-billing",
		Name:          "billing profile artifact run",
		ProjectID:     "project-billing",
		Environment:   "prod",
		Status:        "finished",
		Protocol:      "HTTP",
		Method:        "GET",
		URL:           "http://billing.internal/ready",
		TotalRequests: 1,
		CreatedAt:     "2026-06-12T09:01:00Z",
	}
	if err := runStore.save(checkoutRun); err != nil {
		t.Fatalf("expected checkout run save to succeed, got %v", err)
	}
	if err := runStore.save(billingRun); err != nil {
		t.Fatalf("expected billing run save to succeed, got %v", err)
	}
	checkoutArtifact := saveWorkspaceProfileArtifact(t, artifactStore, "profile-artifact-checkout", checkoutRun.ID, "checkout.pprof")
	billingArtifact := saveWorkspaceProfileArtifact(t, artifactStore, "profile-artifact-billing", billingRun.ID, "billing.pprof")

	router := NewRouter()

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/profile-artifacts", "", "project-checkout", "staging"))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped profile artifact list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var artifacts []struct {
		ID    string `json:"id"`
		RunID string `json:"runId"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&artifacts); err != nil {
		t.Fatalf("expected profile artifact list JSON, got decode error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != checkoutArtifact.ID || artifacts[0].RunID != checkoutRun.ID {
		t.Fatalf("expected only checkout profile artifact in scoped list, got %#v", artifacts)
	}

	downloadResponse := httptest.NewRecorder()
	router.ServeHTTP(downloadResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/profile-artifacts/"+billingArtifact.ID+"/download", "", "project-checkout", "staging"))
	if downloadResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-scope profile artifact download status %d, got %d with body %s", http.StatusForbidden, downloadResponse.Code, downloadResponse.Body.String())
	}
}

func TestWorkspaceScopeHeadersRestrictAgentMetricsEndpoint(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	store := newAgentStoreFromEnv()
	checkoutAgent := upsertWorkspaceAgentRecord(t, store, "agent-checkout-metrics", "project-checkout", "staging")
	billingAgent := upsertWorkspaceAgentRecord(t, store, "agent-billing-metrics", "project-billing", "prod")
	saveWorkspaceAgentMetrics(t, store, checkoutAgent.ID, "2026-06-12T09:00:00Z")
	saveWorkspaceAgentMetrics(t, store, billingAgent.ID, "2026-06-12T09:01:00Z")

	router := NewRouter()

	crossResponse := httptest.NewRecorder()
	router.ServeHTTP(crossResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/agent-metrics?agentId="+billingAgent.ID, "", "project-checkout", "staging"))
	if crossResponse.Code != http.StatusForbidden {
		t.Fatalf("expected cross-scope agent metrics status %d, got %d with body %s", http.StatusForbidden, crossResponse.Code, crossResponse.Body.String())
	}

	ownResponse := httptest.NewRecorder()
	router.ServeHTTP(ownResponse, newWorkspaceScopedAPIRequest(http.MethodGet, "/api/agent-metrics?agentId="+checkoutAgent.ID, "", "project-checkout", "staging"))
	if ownResponse.Code != http.StatusOK {
		t.Fatalf("expected scoped agent metrics status %d, got %d with body %s", http.StatusOK, ownResponse.Code, ownResponse.Body.String())
	}
	var metrics []agentMetricsRecord
	if err := json.NewDecoder(ownResponse.Body).Decode(&metrics); err != nil {
		t.Fatalf("expected agent metrics JSON, got decode error: %v", err)
	}
	if len(metrics) != 1 || metrics[0].AgentID != checkoutAgent.ID {
		t.Fatalf("expected checkout agent metrics only, got %#v", metrics)
	}
}

func TestWorkspaceScopeHeadersRejectCrossScopeAgentTokenOperations(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	tokenResponse := httptest.NewRecorder()
	router.ServeHTTP(tokenResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "billing-token",
		"projectId": "project-billing",
		"environment": "prod"
	}`)))
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, tokenResponse.Code, tokenResponse.Body.String())
	}
	var token struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(tokenResponse.Body).Decode(&token); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	if token.ID == "" {
		t.Fatal("expected created token to include id")
	}

	for _, path := range []string{
		"/api/agent-tokens/" + token.ID + "/revoke",
		"/api/agent-tokens/" + token.ID + "/rotate",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, newWorkspaceScopedAPIRequest(http.MethodPost, path, `{}`, "project-checkout", "staging"))
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected cross-scope token operation %s status %d, got %d with body %s", path, http.StatusForbidden, response.Code, response.Body.String())
		}
	}
}

func createWorkspaceScenario(t *testing.T, router http.Handler, name string, projectID string, environment string) string {
	t.Helper()
	return createWorkspaceScenarioWithBaseURL(t, router, name, projectID, environment, "http://"+name+".internal:8080")
}

func createWorkspaceScenarioWithBaseURL(t *testing.T, router http.Handler, name string, projectID string, environment string, baseURL string) string {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "`+name+`",
		"projectId": "`+projectID+`",
		"environment": "`+environment+`",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+baseURL+`",
		"path": "/ready"
	}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected scenario creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}
	var scenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&scenario); err != nil {
		t.Fatalf("expected scenario creation JSON, got decode error: %v", err)
	}
	if scenario.ID == "" {
		t.Fatal("expected scenario creation response to include id")
	}
	return scenario.ID
}

func createWorkspaceTarget(t *testing.T, router http.Handler, name string, projectID string, environment string) string {
	t.Helper()
	return createWorkspaceTargetWithBaseURL(t, router, name, projectID, environment, "http://"+name+".internal:8080")
}

func createWorkspaceTargetWithBaseURL(t *testing.T, router http.Handler, name string, projectID string, environment string, baseURL string) string {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "`+name+`",
		"projectId": "`+projectID+`",
		"environment": "`+environment+`",
		"baseUrl": "`+baseURL+`"
	}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&target); err != nil {
		t.Fatalf("expected target creation JSON, got decode error: %v", err)
	}
	if target.ID == "" {
		t.Fatal("expected target creation response to include id")
	}
	return target.ID
}

func createWorkspaceAgent(t *testing.T, router http.Handler, name string, projectID string, environment string) {
	t.Helper()
	tokenResponse := httptest.NewRecorder()
	router.ServeHTTP(tokenResponse, httptest.NewRequest(http.MethodPost, "/api/agent-tokens", bytes.NewBufferString(`{
		"name": "`+name+`-token",
		"projectId": "`+projectID+`",
		"environment": "`+environment+`"
	}`)))
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("expected token creation status %d, got %d with body %s", http.StatusCreated, tokenResponse.Code, tokenResponse.Body.String())
	}
	var token struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResponse.Body).Decode(&token); err != nil {
		t.Fatalf("expected token JSON, got decode error: %v", err)
	}
	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token.Token, `{
		"name": "`+name+`",
		"hostname": "`+name+`-host",
		"ip": "10.0.0.10",
		"version": "0.3.0",
		"capabilities": ["host_metrics"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}
}

func createWorkspaceProfileTask(t *testing.T, store *profileTaskStore, runID string, agentID string) profileTaskRecord {
	t.Helper()
	task, err := store.create(profileTaskRecord{
		AgentID:        agentID,
		RunID:          runID,
		PprofBaseURL:   "http://" + agentID + ".internal:6060/debug/pprof",
		ProfileType:    "cpu",
		ProfileSeconds: 1,
		MaxAttempts:    1,
	})
	if err != nil {
		t.Fatalf("expected profile task creation to succeed, got %v", err)
	}
	return task
}

func saveWorkspaceProfileArtifact(t *testing.T, store *profileArtifactStore, id string, runID string, fileName string) profileArtifactRecord {
	t.Helper()
	artifact, err := store.save(profileArtifactRecord{
		ID:          id,
		RunID:       runID,
		ProfileType: "cpu",
		Status:      "collected",
		SourceURL:   "http://" + id + ".internal/debug/pprof/profile",
		FileName:    fileName,
		ContentType: "application/octet-stream",
		SizeBytes:   7,
		StartedAt:   "2026-06-12T09:00:00Z",
		FinishedAt:  "2026-06-12T09:00:01Z",
	}, []byte("profile"))
	if err != nil {
		t.Fatalf("expected profile artifact save to succeed, got %v", err)
	}
	return artifact
}

func upsertWorkspaceAgentRecord(t *testing.T, store *agentStore, id string, projectID string, environment string) agentRecord {
	t.Helper()
	agent, err := store.upsertHeartbeat(agentRecord{
		ID:           id,
		TokenID:      "token-" + id,
		ProjectID:    projectID,
		Environment:  environment,
		Name:         id,
		Hostname:     id + "-host",
		IP:           "10.0.0.10",
		Version:      "0.3.0",
		Capabilities: []string{"host_metrics", "process_metrics"},
	})
	if err != nil {
		t.Fatalf("expected agent heartbeat to succeed, got %v", err)
	}
	return agent
}

func saveWorkspaceAgentMetrics(t *testing.T, store *agentStore, agentID string, collectedAt string) {
	t.Helper()
	_, err := store.saveMetrics(agentMetricsRecord{
		AgentID:            agentID,
		CollectedAt:        collectedAt,
		CPUUsagePercent:    15,
		MemoryUsagePercent: 30,
	})
	if err != nil {
		t.Fatalf("expected agent metrics save to succeed, got %v", err)
	}
}

func newWorkspaceScopedAPIRequest(method string, path string, body string, projectID string, environment string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("X-AIT-Project-ID", projectID)
	request.Header.Set("X-AIT-Environment", environment)
	return request
}
