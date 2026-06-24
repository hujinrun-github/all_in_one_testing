package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setRunTestDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
}

func decodeRunResponse(t *testing.T, response *httptest.ResponseRecorder) createRunResponse {
	t.Helper()

	var result createRunResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("expected run JSON response, got decode error: %v", err)
	}
	return result
}

func waitForRunStatus(t *testing.T, router http.Handler, runID string, status string) createRunResponse {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var latest createRunResponse
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("expected run lookup status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
		}
		latest = decodeRunResponse(t, response)
		if latest.Status == status {
			return latest
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected run %s to reach status %q, latest run was %#v", runID, status, latest)
	return latest
}

func createAsyncRun(t *testing.T, router http.Handler, body string) createRunResponse {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected async run status %d, got %d with body %s", http.StatusAccepted, response.Code, response.Body.String())
	}
	result := decodeRunResponse(t, response)
	if result.ID == "" {
		t.Fatal("expected generated run id")
	}
	if result.Status != "running" {
		t.Fatalf("expected initial running status, got %q", result.Status)
	}
	return result
}

func TestRunProgressDurationIncludesTimeSinceRunCreation(t *testing.T) {
	createdAt := time.Now().UTC().Add(-75 * time.Millisecond)
	progress := runProgress{
		input: createRunRequest{
			RunID:         "run-created-before-worker",
			Name:          "created-before-worker",
			Method:        http.MethodGet,
			URL:           "http://127.0.0.1:8080/health",
			TotalRequests: 1,
			CreatedAt:     createdAt.Format(time.RFC3339Nano),
		},
		startedAt: time.Now().UTC(),
		latencies: []float64{1},
		success:   1,
	}

	snapshot := progress.snapshot("finished")

	if snapshot.DurationMs < 50 {
		t.Fatalf("expected duration to include time since run creation, got %#v", snapshot)
	}
}

type testRunEvent struct {
	ID              string  `json:"id"`
	RunID           string  `json:"runId"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Message         string  `json:"message"`
	SuccessRequests int     `json:"successRequests"`
	FailedRequests  int     `json:"failedRequests"`
	TotalRequests   int     `json:"totalRequests"`
	QPS             float64 `json:"qps"`
	P95LatencyMs    float64 `json:"p95LatencyMs"`
	CreatedAt       string  `json:"createdAt"`
}

func listRunEvents(t *testing.T, router http.Handler, runID string) []testRunEvent {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/events", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected run events status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var events []testRunEvent
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatalf("expected run events JSON, got decode error: %v", err)
	}
	return events
}

func runReportErrors(t *testing.T, router http.Handler, runID string) []string {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+runID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected run report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var report struct {
		ErrorSamples []struct {
			Error string `json:"error"`
		} `json:"errorSamples"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected run report JSON, got decode error: %v", err)
	}
	errors := make([]string, 0, len(report.ErrorSamples))
	for _, sample := range report.ErrorSamples {
		errors = append(errors, sample.Error)
	}
	return errors
}

func assertRunEventType(t *testing.T, events []testRunEvent, eventType string) testRunEvent {
	t.Helper()

	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("expected run event type %q in %#v", eventType, events)
	return testRunEvent{}
}

func waitForRunEventType(t *testing.T, router http.Handler, runID string, eventType string) []testRunEvent {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var events []testRunEvent
	for time.Now().Before(deadline) {
		events = listRunEvents(t, router, runID)
		for _, event := range events {
			if event.Type == eventType {
				return events
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected run %s to emit event %q, got %#v", runID, eventType, events)
	return events
}

func TestCreateRunExecutesHTTPLoadTest(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	body := bytes.NewBufferString(`{
		"name": "health-smoke",
		"method": "GET",
		"url": "` + target.URL + `",
		"totalRequests": 12,
		"concurrency": 3,
		"timeoutMs": 1000
	}`)
	created := createAsyncRun(t, router, body.String())
	result := waitForRunStatus(t, router, created.ID, "finished")
	if result.TotalRequests != 12 {
		t.Fatalf("expected 12 total requests, got %d", result.TotalRequests)
	}
	if result.SuccessRequests != 12 {
		t.Fatalf("expected 12 successful requests, got %d", result.SuccessRequests)
	}
	if result.FailedRequests != 0 {
		t.Fatalf("expected 0 failed requests, got %d", result.FailedRequests)
	}
	if result.QPS <= 0 {
		t.Fatalf("expected positive qps, got %f", result.QPS)
	}
	if result.P95LatencyMs < 0 {
		t.Fatalf("expected non-negative p95 latency, got %f", result.P95LatencyMs)
	}
	if hits.Load() != 12 {
		t.Fatalf("expected target to receive 12 requests, got %d", hits.Load())
	}
}

func TestCreateRunUsesWeightedBodyVariants(t *testing.T) {
	setRunTestDatabase(t)

	var preferredHits atomic.Int64
	var ignoredHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch string(body) {
		case `{"sku":"preferred"}`:
			preferredHits.Add(1)
		case `{"sku":"ignored"}`:
			ignoredHits.Add(1)
		default:
			t.Errorf("unexpected request body %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	body := bytes.NewBufferString(`{
		"name": "weighted-body-smoke",
		"method": "POST",
		"url": "` + target.URL + `",
		"totalRequests": 8,
		"concurrency": 2,
		"timeoutMs": 1000,
		"bodyVariants": [
			{ "name": "ignored", "weight": 0, "body": "{\"sku\":\"ignored\"}" },
			{ "name": "preferred", "weight": 10, "body": "{\"sku\":\"preferred\"}" }
		]
	}`)
	created := createAsyncRun(t, router, body.String())
	waitForRunStatus(t, router, created.ID, "finished")
	if preferredHits.Load() != 8 {
		t.Fatalf("expected preferred body to be used for all 8 requests, got %d", preferredHits.Load())
	}
	if ignoredHits.Load() != 0 {
		t.Fatalf("expected zero-weight body to be ignored, got %d hits", ignoredHits.Load())
	}
}

func TestCreateRunUsesWeightedQueryVariants(t *testing.T) {
	setRunTestDatabase(t)

	var preferredHits atomic.Int64
	var ignoredHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("sku") {
		case "preferred":
			preferredHits.Add(1)
		case "ignored":
			ignoredHits.Add(1)
		default:
			t.Errorf("unexpected request query %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("static") != "1" {
			t.Errorf("expected base query static=1, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	body := bytes.NewBufferString(`{
		"name": "weighted-query-smoke",
		"method": "GET",
		"url": "` + target.URL + `?static=1",
		"totalRequests": 8,
		"concurrency": 2,
		"timeoutMs": 1000,
		"queryVariants": [
			{ "name": "ignored", "weight": 0, "queryParams": "sku=ignored" },
			{ "name": "preferred", "weight": 10, "queryParams": "sku=preferred" }
		]
	}`)
	created := createAsyncRun(t, router, body.String())
	waitForRunStatus(t, router, created.ID, "finished")
	if preferredHits.Load() != 8 {
		t.Fatalf("expected preferred query to be used for all 8 requests, got %d", preferredHits.Load())
	}
	if ignoredHits.Load() != 0 {
		t.Fatalf("expected zero-weight query to be ignored, got %d hits", ignoredHits.Load())
	}
}

func TestCreateRunExecutesSavedScenario(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/checkout" {
			t.Errorf("expected /checkout path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("tenant") != "stored" {
			t.Errorf("expected stored tenant query, got %q", r.URL.RawQuery)
		}
		if r.Header.Get("X-Scenario") != "from-sqlite" {
			t.Errorf("expected scenario header, got %q", r.Header.Get("X-Scenario"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		if string(body) != `{"kind":"saved"}` {
			t.Errorf("expected saved body, got %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "saved-checkout",
		"protocol": "HTTP",
		"method": "POST",
		"baseUrl": "`+target.URL+`",
		"path": "/checkout",
		"queryVariants": [
			{ "name": "stored", "weight": "100", "queryParams": "tenant=stored" }
		],
		"headers": [
			{ "key": "X-Scenario", "value": "from-sqlite" }
		],
		"bodyVariants": [
			{ "name": "saved", "weight": "100", "body": "{\"kind\":\"saved\"}" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 5,
		"concurrency": 2
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if hits.Load() != 5 {
		t.Fatalf("expected target to receive 5 scenario requests, got %d", hits.Load())
	}
	if result.ScenarioID != createdScenario.ID {
		t.Fatalf("expected scenario id %q, got %q", createdScenario.ID, result.ScenarioID)
	}
	if result.ScenarioName != "saved-checkout" {
		t.Fatalf("expected scenario name saved-checkout, got %q", result.ScenarioName)
	}
	if result.Method != http.MethodPost {
		t.Fatalf("expected POST method, got %q", result.Method)
	}
	if result.URL != target.URL+"/checkout" {
		t.Fatalf("expected scenario URL %q, got %q", target.URL+"/checkout", result.URL)
	}
	if result.TotalRequests != 5 || result.SuccessRequests != 5 {
		t.Fatalf("expected 5 successful requests, got %#v", result)
	}
}

func TestCreateRunExecutesCustomRPCScenarioThroughAdapter(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	adapter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("expected adapter POST method, got %s", r.Method)
		}
		if r.URL.Path != "/invoke" {
			t.Errorf("expected adapter path /invoke, got %s", r.URL.Path)
		}
		var request struct {
			Protocol   string          `json:"protocol"`
			Method     string          `json:"method"`
			RunID      string          `json:"runId"`
			ScenarioID string          `json:"scenarioId"`
			Body       json.RawMessage `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("expected adapter request JSON, got %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Protocol != "CUSTOM_RPC" {
			t.Errorf("expected CUSTOM_RPC protocol, got %q", request.Protocol)
		}
		if request.Method != "checkout.OrderService/CreateOrder" {
			t.Errorf("expected RPC method to be preserved, got %q", request.Method)
		}
		if request.RunID == "" || request.ScenarioID == "" {
			t.Errorf("expected run and scenario ids, got %#v", request)
		}
		if string(request.Body) != `{"sku":"book"}` {
			t.Errorf("expected JSON body variant, got %s", string(request.Body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"statusCode":200}`))
	}))
	t.Cleanup(adapter.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "custom-rpc-order",
		"protocol": "CUSTOM_RPC",
		"method": "checkout.OrderService/CreateOrder",
		"baseUrl": "`+adapter.URL+`",
		"path": "/invoke",
		"bodyVariants": [
			{ "name": "book", "weight": "100", "body": "{\"sku\":\"book\"}" }
		],
		"timeoutMs": "1200",
		"retryCount": "0"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID     string `json:"id"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}
	if createdScenario.Method != "checkout.OrderService/CreateOrder" {
		t.Fatalf("expected scenario RPC method to keep case, got %q", createdScenario.Method)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 4,
		"concurrency": 2
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if hits.Load() != 4 {
		t.Fatalf("expected adapter to receive 4 requests, got %d", hits.Load())
	}
	if result.SuccessRequests != 4 || result.FailedRequests != 0 {
		t.Fatalf("expected 4 successful custom rpc requests, got %#v", result)
	}
	if result.Method != "checkout.OrderService/CreateOrder" {
		t.Fatalf("expected RPC method in run result, got %q", result.Method)
	}
	runLookup := httptest.NewRecorder()
	router.ServeHTTP(runLookup, httptest.NewRequest(http.MethodGet, "/api/runs/"+createdRun.ID, nil))
	if runLookup.Code != http.StatusOK {
		t.Fatalf("expected run lookup status %d, got %d with body %s", http.StatusOK, runLookup.Code, runLookup.Body.String())
	}
	var runJSON map[string]any
	if err := json.NewDecoder(runLookup.Body).Decode(&runJSON); err != nil {
		t.Fatalf("expected run lookup JSON, got decode error: %v", err)
	}
	if runJSON["protocol"] != "CUSTOM_RPC" {
		t.Fatalf("expected CUSTOM_RPC protocol in run JSON, got %#v", runJSON["protocol"])
	}
}

func TestCreateRunExecutesEnabledScenarioFlowStepsInOrder(t *testing.T) {
	setRunTestDatabase(t)

	var mu sync.Mutex
	received := []string{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-checkout",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "draft", "name": "draft request", "type": "request", "enabled": false, "method": "GET", "path": "/draft" },
			{ "id": "login", "name": "HTTP login", "type": "request", "enabled": true, "method": "GET", "path": "/login" },
			{ "id": "checkout", "name": "HTTP checkout", "type": "request", "enabled": true, "method": "POST", "path": "/checkout" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 2,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.TotalRequests != 2 || result.SuccessRequests != 2 || result.FailedRequests != 0 {
		t.Fatalf("expected two successful flow samples, got %#v", result)
	}

	mu.Lock()
	actual := append([]string(nil), received...)
	mu.Unlock()
	expected := []string{"GET /login", "POST /checkout", "GET /login", "POST /checkout"}
	if len(actual) != len(expected) {
		t.Fatalf("expected flow requests %v, got %v", expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected flow requests %v, got %v", expected, actual)
		}
	}
}

func TestCreateRunExecutesScenarioAssertionFlowStep(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accepted" {
			t.Errorf("expected /accepted path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-assertion",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "accepted", "name": "accepted request", "type": "request", "enabled": true, "method": "GET", "path": "/accepted" },
			{ "id": "assert-ok", "name": "assert ok", "type": "assertion", "enabled": true, "assertion": "status == 200" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 500"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 0 || result.FailedRequests != 1 {
		t.Fatalf("expected assertion flow step to fail the sample, got %#v", result)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+createdRun.ID, nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, reportResponse.Code, reportResponse.Body.String())
	}
	var report struct {
		ErrorSamples []struct {
			StatusCode int    `json:"statusCode"`
			Error      string `json:"error"`
		} `json:"errorSamples"`
	}
	if err := json.NewDecoder(reportResponse.Body).Decode(&report); err != nil {
		t.Fatalf("expected run report JSON, got decode error: %v", err)
	}
	if len(report.ErrorSamples) == 0 || report.ErrorSamples[0].StatusCode != http.StatusAccepted || !strings.Contains(report.ErrorSamples[0].Error, "status assertion failed") {
		t.Fatalf("expected assertion step failure sample, got %#v", report.ErrorSamples)
	}
}

func TestCreateRunRecordsFailureWhenScenarioJSONAssertionDoesNotMatch(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/state" {
			t.Errorf("expected /state path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"fail"}`))
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "json-assertion",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "state", "name": "state request", "type": "request", "enabled": true, "method": "GET", "path": "/state" },
			{ "id": "assert-state", "name": "assert state", "type": "assertion", "enabled": true, "assertions": [
				{ "name": "state ok", "source": "json", "path": "state", "operator": "equals", "expected": "ok" }
			] }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 500"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 0 || result.FailedRequests != 1 {
		t.Fatalf("expected JSON assertion mismatch to fail the sample, got %#v", result)
	}
	errors := runReportErrors(t, router, createdRun.ID)
	if len(errors) == 0 || !strings.Contains(errors[0], "response assertion failed") || !strings.Contains(errors[0], "state ok") {
		t.Fatalf("expected JSON assertion failure sample, got %#v", errors)
	}
}

func TestCreateRunExtractsJSONVariableForLaterFlowStep(t *testing.T) {
	setRunTestDatabase(t)

	var checkoutHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"token-abc"}`))
		case "/checkout/token-abc":
			checkoutHits.Add(1)
			if r.Header.Get("Authorization") != "Bearer token-abc" {
				http.Error(w, "missing extracted token", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-json-extract",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [{ "key": "Authorization", "value": "Bearer ${token}" }],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "login", "name": "login", "type": "request", "enabled": true, "method": "GET", "path": "/login" },
			{ "id": "extract-token", "name": "extract token", "type": "extract", "enabled": true, "extractors": [
				{ "name": "token", "source": "json", "path": "token" }
			] },
			{ "id": "checkout", "name": "checkout", "type": "request", "enabled": true, "method": "GET", "path": "/checkout/${token}" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || checkoutHits.Load() != 1 {
		t.Fatalf("expected extracted JSON token to be used by checkout step, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), checkoutHits.Load())
	}
}

func TestCreateRunExtractsHeaderVariableForLaterFlowStep(t *testing.T) {
	setRunTestDatabase(t)

	var checkoutHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Set("X-Session-Id", "session-42")
			w.WriteHeader(http.StatusNoContent)
		case "/checkout/session-42":
			checkoutHits.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-header-extract",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "start", "name": "start", "type": "request", "enabled": true, "method": "GET", "path": "/start" },
			{ "id": "extract-session", "name": "extract session", "type": "extract", "enabled": true, "extractors": [
				{ "name": "session", "source": "header", "path": "X-Session-Id" }
			] },
			{ "id": "checkout", "name": "checkout", "type": "request", "enabled": true, "method": "GET", "path": "/checkout/${session}" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || checkoutHits.Load() != 1 {
		t.Fatalf("expected extracted header value to be used by checkout step, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), checkoutHits.Load())
	}
}

func TestCreateRunExtractsRegexVariableForLaterFlowStep(t *testing.T) {
	setRunTestDatabase(t)

	var checkoutHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/create":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("created order=ORD-42 ready"))
		case "/checkout/ORD-42":
			checkoutHits.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-regex-extract",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "create", "name": "create", "type": "request", "enabled": true, "method": "GET", "path": "/create" },
			{ "id": "extract-order", "name": "extract order", "type": "extract", "enabled": true, "extractors": [
				{ "name": "order", "source": "regex", "path": "order=(ORD-[0-9]+)" }
			] },
			{ "id": "checkout", "name": "checkout", "type": "request", "enabled": true, "method": "GET", "path": "/checkout/${order}" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || checkoutHits.Load() != 1 {
		t.Fatalf("expected regex extracted order to be used by checkout step, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), checkoutHits.Load())
	}
}

func TestCreateRunCarriesCookiesBetweenScenarioFlowSteps(t *testing.T) {
	setRunTestDatabase(t)

	var checkoutHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "session-abc", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/checkout":
			checkoutHits.Add(1)
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "session-abc" {
				http.Error(w, "missing session cookie", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-cookie-session",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "login", "name": "login", "type": "request", "enabled": true, "method": "GET", "path": "/login" },
			{ "id": "checkout", "name": "checkout", "type": "request", "enabled": true, "method": "GET", "path": "/checkout" }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || checkoutHits.Load() != 1 {
		t.Fatalf("expected login cookie to be carried to checkout step, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), checkoutHits.Load())
	}
}

func TestHTTPClientWithCookieJarCreatesIndependentCookieJars(t *testing.T) {
	baseClient := &http.Client{Timeout: 1500 * time.Millisecond}

	first := httpClientWithCookieJar(baseClient)
	second := httpClientWithCookieJar(baseClient)

	if first == baseClient || second == baseClient {
		t.Fatal("expected sample clients to clone the base client")
	}
	if first.Jar == nil || second.Jar == nil {
		t.Fatal("expected sample clients to have cookie jars")
	}
	if first.Jar == second.Jar {
		t.Fatal("expected each sample client to get an independent cookie jar")
	}
	if first.Timeout != baseClient.Timeout || second.Timeout != baseClient.Timeout {
		t.Fatalf("expected cloned clients to preserve timeout, got %s and %s", first.Timeout, second.Timeout)
	}
}

func TestCreateRunPassesScenarioRegexResponseAssertion(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("expected /status path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("checkout state=OK order=ORD-42"))
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "regex-assertion",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "status", "name": "status request", "type": "request", "enabled": true, "method": "GET", "path": "/status" },
			{ "id": "assert-status", "name": "assert status body", "type": "assertion", "enabled": true, "assertions": [
				{ "name": "body has order", "source": "body", "operator": "matches", "expected": "state=OK\\s+order=ORD-[0-9]+" }
			] }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 {
		t.Fatalf("expected regex response assertion to pass, got run %#v and errors %#v", result, runReportErrors(t, router, createdRun.ID))
	}
}

func TestCreateRunSkipsFlowStepWhenJSONConditionDoesNotMatch(t *testing.T) {
	setRunTestDatabase(t)

	var checkoutHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"runCheckout":false}`))
		case "/checkout":
			checkoutHits.Add(1)
			http.Error(w, "checkout should have been skipped", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-json-when-skip",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "flags", "name": "flags", "type": "request", "enabled": true, "method": "GET", "path": "/flags" },
			{ "id": "checkout", "name": "checkout", "type": "request", "enabled": true, "method": "GET", "path": "/checkout",
			  "when": { "name": "checkout enabled", "source": "json", "path": "runCheckout", "operator": "equals", "expected": "true" } }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || checkoutHits.Load() != 0 {
		t.Fatalf("expected checkout step to be skipped by JSON when condition, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), checkoutHits.Load())
	}
}

func TestCreateRunSkipsFlowStepWhenVariableConditionDoesNotMatch(t *testing.T) {
	setRunTestDatabase(t)

	var optionalHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Set("X-Mode", "skip")
			w.WriteHeader(http.StatusNoContent)
		case "/optional":
			optionalHits.Add(1)
			http.Error(w, "optional should have been skipped", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "flow-variable-when-skip",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/ignored",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [
			{ "id": "start", "name": "start", "type": "request", "enabled": true, "method": "GET", "path": "/start" },
			{ "id": "extract-mode", "name": "extract mode", "type": "extract", "enabled": true, "extractors": [
				{ "name": "mode", "source": "header", "path": "X-Mode" }
			] },
			{ "id": "optional", "name": "optional", "type": "request", "enabled": true, "method": "GET", "path": "/optional",
			  "when": { "name": "mode is run", "source": "variable", "path": "mode", "operator": "equals", "expected": "run" } }
		],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 1 || result.FailedRequests != 0 || optionalHits.Load() != 0 {
		t.Fatalf("expected optional step to be skipped by variable when condition, got run %#v, errors %#v and hits %d", result, runReportErrors(t, router, createdRun.ID), optionalHits.Load())
	}
}

func TestCreateRunAppliesScenarioStatusAssertion(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "accepted-not-found",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/maybe",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 500"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 3,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 3 || result.FailedRequests != 0 {
		t.Fatalf("expected status assertion to treat 404 as successful business response, got %#v", result)
	}
}

func TestCreateRunRecordsFailureWhenScenarioStatusAssertionDoesNotMatch(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "expects-server-error",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "`+target.URL+`",
		"path": "/health",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"flowSteps": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status >= 500"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 2,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if result.SuccessRequests != 0 || result.FailedRequests != 2 {
		t.Fatalf("expected failed status assertion to count failed requests, got %#v", result)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+createdRun.ID, nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, reportResponse.Code, reportResponse.Body.String())
	}
	var report struct {
		ErrorSamples []struct {
			StatusCode int    `json:"statusCode"`
			Error      string `json:"error"`
		} `json:"errorSamples"`
	}
	if err := json.NewDecoder(reportResponse.Body).Decode(&report); err != nil {
		t.Fatalf("expected run report JSON, got decode error: %v", err)
	}
	if len(report.ErrorSamples) == 0 || report.ErrorSamples[0].StatusCode != http.StatusOK || !strings.Contains(report.ErrorSamples[0].Error, "status assertion failed") {
		t.Fatalf("expected status assertion failure sample, got %#v", report.ErrorSamples)
	}
}

func TestCreateRunUsesTargetBaseURLForSavedScenario(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/checkout" {
			t.Errorf("expected /checkout path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "saved-target-checkout",
		"projectId": "project-checkout",
		"environment": "staging",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://scenario.example.invalid",
		"path": "/checkout",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(createTargetResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-target",
		"projectId": "project-checkout",
		"baseUrl": "`+target.URL+`",
		"environment": "prod",
		"agentIds": [],
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof",
		"processMatch": { "name": "checkout", "cmdlineContains": "" }
	}`)))
	if createTargetResponse.Code != http.StatusCreated {
		t.Fatalf("expected target status %d, got %d with body %s", http.StatusCreated, createTargetResponse.Code, createTargetResponse.Body.String())
	}
	var createdTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTargetResponse.Body).Decode(&createdTarget); err != nil {
		t.Fatalf("expected created target JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"targetId": "`+createdTarget.ID+`",
		"totalRequests": 3,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if hits.Load() != 3 {
		t.Fatalf("expected target to receive 3 scenario requests, got %d", hits.Load())
	}
	if result.TargetID != createdTarget.ID {
		t.Fatalf("expected target id %q, got %q", createdTarget.ID, result.TargetID)
	}
	if result.TargetName != "checkout-target" {
		t.Fatalf("expected target name checkout-target, got %q", result.TargetName)
	}
	if result.URL != target.URL+"/checkout" {
		t.Fatalf("expected target URL %q, got %q", target.URL+"/checkout", result.URL)
	}
	if result.TotalRequests != 3 || result.SuccessRequests != 3 {
		t.Fatalf("expected 3 successful requests, got %#v", result)
	}

	lookupResponse := httptest.NewRecorder()
	router.ServeHTTP(lookupResponse, httptest.NewRequest(http.MethodGet, "/api/runs/"+createdRun.ID, nil))
	if lookupResponse.Code != http.StatusOK {
		t.Fatalf("expected run lookup status %d, got %d with body %s", http.StatusOK, lookupResponse.Code, lookupResponse.Body.String())
	}
	var lookup struct {
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(lookupResponse.Body).Decode(&lookup); err != nil {
		t.Fatalf("expected run lookup JSON, got decode error: %v", err)
	}
	if lookup.ProjectID != "project-checkout" || lookup.Environment != "prod" {
		t.Fatalf("expected run to inherit target project/environment, got %#v", lookup)
	}

	restartedRouter := NewRouter()
	historyResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("expected history status %d, got %d with body %s", http.StatusOK, historyResponse.Code, historyResponse.Body.String())
	}
	var history []struct {
		TargetID    string `json:"targetId"`
		TargetName  string `json:"targetName"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatalf("expected history JSON, got decode error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one persisted run, got %#v", history)
	}
	if history[0].TargetID != createdTarget.ID || history[0].TargetName != "checkout-target" {
		t.Fatalf("expected target metadata to persist, got %#v", history[0])
	}
	if history[0].ProjectID != "project-checkout" || history[0].Environment != "prod" {
		t.Fatalf("expected project/environment to persist, got %#v", history[0])
	}
}

func TestCreateRunRejectsUnhealthyTargetPreflight(t *testing.T) {
	setRunTestDatabase(t)

	var healthHits atomic.Int64
	var loadHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			healthHits.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/checkout":
			loadHits.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected target path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "saved-target-preflight",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://scenario.example.invalid",
		"path": "/checkout",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(createTargetResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-target",
		"baseUrl": "`+target.URL+`",
		"environment": "test",
		"agentIds": [],
		"profileEndpoint": "",
		"processMatch": { "name": "checkout", "cmdlineContains": "" },
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 200,
			"timeoutMs": 1000
		}
	}`)))
	if createTargetResponse.Code != http.StatusCreated {
		t.Fatalf("expected target status %d, got %d with body %s", http.StatusCreated, createTargetResponse.Code, createTargetResponse.Body.String())
	}
	var createdTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTargetResponse.Body).Decode(&createdTarget); err != nil {
		t.Fatalf("expected created target JSON, got decode error: %v", err)
	}

	createRunResponse := httptest.NewRecorder()
	router.ServeHTTP(createRunResponse, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(`{
		"scenarioId": "`+createdScenario.ID+`",
		"targetId": "`+createdTarget.ID+`",
		"totalRequests": 3,
		"concurrency": 1
	}`)))
	if createRunResponse.Code != http.StatusConflict {
		t.Fatalf("expected unhealthy target status %d, got %d with body %s", http.StatusConflict, createRunResponse.Code, createRunResponse.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(createRunResponse.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error, got decode error: %v", err)
	}
	if !strings.Contains(body.Error, "target health preflight failed") || !strings.Contains(body.Error, "expected HTTP 200, got HTTP 503") {
		t.Fatalf("expected preflight error message, got %q", body.Error)
	}
	if healthHits.Load() != 1 {
		t.Fatalf("expected one health check request, got %d", healthHits.Load())
	}
	if loadHits.Load() != 0 {
		t.Fatalf("expected no load requests after failed preflight, got %d", loadHits.Load())
	}

	getTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(getTargetResponse, httptest.NewRequest(http.MethodGet, "/api/targets/"+createdTarget.ID, nil))
	if getTargetResponse.Code != http.StatusOK {
		t.Fatalf("expected target get status %d, got %d with body %s", http.StatusOK, getTargetResponse.Code, getTargetResponse.Body.String())
	}
	var persistedTarget struct {
		LastHealthCheck *targetHealthCheckResult `json:"lastHealthCheck"`
	}
	if err := json.NewDecoder(getTargetResponse.Body).Decode(&persistedTarget); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}
	if persistedTarget.LastHealthCheck == nil ||
		persistedTarget.LastHealthCheck.Status != "unhealthy" ||
		persistedTarget.LastHealthCheck.ObservedStatus != http.StatusServiceUnavailable ||
		persistedTarget.LastHealthCheck.Error == "" {
		t.Fatalf("expected failed preflight to persist latest health result, got %#v", persistedTarget.LastHealthCheck)
	}
}

func TestCreateRunCapturesTargetProfileArtifact(t *testing.T) {
	setRunTestDatabase(t)

	var loadHits atomic.Int64
	var profileHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checkout":
			loadHits.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/debug/pprof/profile":
			profileHits.Add(1)
			if r.URL.Query().Get("seconds") != "1" {
				t.Errorf("expected one-second profile capture, got query %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("pprof-cpu-sample"))
		default:
			t.Errorf("unexpected target path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "profiled-checkout",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://scenario.example.invalid",
		"path": "/checkout",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var createdScenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&createdScenario); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	createTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(createTargetResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "profiled-target",
		"baseUrl": "`+target.URL+`",
		"environment": "test",
		"agentIds": [],
		"profileEndpoint": "`+target.URL+`/debug/pprof",
		"processMatch": { "name": "checkout", "cmdlineContains": "" }
	}`)))
	if createTargetResponse.Code != http.StatusCreated {
		t.Fatalf("expected target status %d, got %d with body %s", http.StatusCreated, createTargetResponse.Code, createTargetResponse.Body.String())
	}
	var createdTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTargetResponse.Body).Decode(&createdTarget); err != nil {
		t.Fatalf("expected created target JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+createdScenario.ID+`",
		"targetId": "`+createdTarget.ID+`",
		"totalRequests": 2,
		"concurrency": 1
	}`)
	result := waitForRunStatus(t, router, createdRun.ID, "finished")
	if loadHits.Load() != 2 {
		t.Fatalf("expected target to receive 2 load requests, got %d", loadHits.Load())
	}
	if profileHits.Load() != 1 {
		t.Fatalf("expected target pprof endpoint to be called once, got %d", profileHits.Load())
	}

	artifactResponse := httptest.NewRecorder()
	router.ServeHTTP(artifactResponse, httptest.NewRequest(http.MethodGet, "/api/profile-artifacts", nil))
	if artifactResponse.Code != http.StatusOK {
		t.Fatalf("expected artifact list status %d, got %d with body %s", http.StatusOK, artifactResponse.Code, artifactResponse.Body.String())
	}
	var artifacts []struct {
		ID     string `json:"id"`
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(artifactResponse.Body).Decode(&artifacts); err != nil {
		t.Fatalf("expected artifact list JSON, got decode error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one persisted artifact, got %#v", artifacts)
	}
	if artifacts[0].RunID != result.ID || artifacts[0].Status != "collected" {
		t.Fatalf("expected persisted artifact to match response, got %#v", artifacts[0])
	}
}

func TestCreateRunSchedulesAgentProfileTaskForBoundTarget(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout" {
			t.Errorf("expected /checkout path, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-checkout-01",
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"ip": "10.0.0.13",
		"version": "0.4.0",
		"capabilities": ["pprof"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}

	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "profile-task-checkout",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://placeholder.internal",
		"path": "/checkout",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var scenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&scenario); err != nil {
		t.Fatalf("expected scenario JSON, got decode error: %v", err)
	}

	createTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(createTargetResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-target",
		"baseUrl": "`+target.URL+`",
		"environment": "test",
		"agentIds": ["agent-checkout-01"],
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof",
		"processMatch": {
			"name": "checkout"
		}
	}`)))
	if createTargetResponse.Code != http.StatusCreated {
		t.Fatalf("expected target status %d, got %d with body %s", http.StatusCreated, createTargetResponse.Code, createTargetResponse.Body.String())
	}
	var createdTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTargetResponse.Body).Decode(&createdTarget); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+scenario.ID+`",
		"targetId": "`+createdTarget.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)

	taskResponse := httptest.NewRecorder()
	router.ServeHTTP(taskResponse, httptest.NewRequest(http.MethodGet, "/api/profile-tasks", nil))
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("expected profile task list status %d, got %d with body %s", http.StatusOK, taskResponse.Code, taskResponse.Body.String())
	}
	var tasks []profileTaskRecord
	if err := json.NewDecoder(taskResponse.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected profile task list JSON, got decode error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one scheduled profile task, got %#v", tasks)
	}
	task := tasks[0]
	if task.Status != "pending" || task.AgentID != "agent-checkout-01" || task.RunID != createdRun.ID {
		t.Fatalf("expected pending task for created run and bound agent, got %#v", task)
	}
	if task.TargetID != createdTarget.ID || task.TargetName != "checkout-target" {
		t.Fatalf("expected task target metadata, got %#v", task)
	}
	if task.ProfileType != "cpu" || task.ProfileSeconds != 1 || task.PprofBaseURL != "http://127.0.0.1:6060/debug/pprof" {
		t.Fatalf("expected task profile metadata, got %#v", task)
	}
	if task.Source != "run_auto" {
		t.Fatalf("expected run-scheduled task source run_auto, got %#v", task)
	}
	if task.Attempts != 0 || task.MaxAttempts != defaultProfileTaskMaxAttempts {
		t.Fatalf("expected task retry metadata 0/%d, got %#v", defaultProfileTaskMaxAttempts, task)
	}
}

func TestCreateRunSchedulesThresholdProfileTaskWhenTargetMetricsExceedThreshold(t *testing.T) {
	setRunTestDatabase(t)

	const thresholdProfileTaskSource = "threshold_auto"

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout" {
			t.Errorf("expected /checkout path, got %s", r.URL.Path)
		}
		startedOnce.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	defer releaseOnce.Do(func() { close(release) })

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-threshold-01",
		"name": "threshold-01",
		"hostname": "threshold-host-01",
		"ip": "10.0.0.14",
		"version": "0.4.0",
		"capabilities": ["host_metrics", "process_metrics", "pprof"]
	}`))
	if heartbeatResponse.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeatResponse.Code, heartbeatResponse.Body.String())
	}

	createScenarioResponse := httptest.NewRecorder()
	router.ServeHTTP(createScenarioResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "threshold-profile-checkout",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://placeholder.internal",
		"path": "/checkout",
		"queryVariants": [],
		"headers": [],
		"bodyVariants": [],
		"timeoutMs": "1200",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createScenarioResponse.Code != http.StatusCreated {
		t.Fatalf("expected scenario status %d, got %d with body %s", http.StatusCreated, createScenarioResponse.Code, createScenarioResponse.Body.String())
	}
	var scenario struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createScenarioResponse.Body).Decode(&scenario); err != nil {
		t.Fatalf("expected scenario JSON, got decode error: %v", err)
	}

	createTargetResponse := httptest.NewRecorder()
	router.ServeHTTP(createTargetResponse, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "threshold-target",
		"baseUrl": "`+target.URL+`",
		"environment": "test",
		"agentIds": ["agent-threshold-01"],
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof",
		"processMatch": {
			"name": "checkout"
		},
		"metricThresholds": {
			"cpuMaxPercent": 70
		}
	}`)))
	if createTargetResponse.Code != http.StatusCreated {
		t.Fatalf("expected target status %d, got %d with body %s", http.StatusCreated, createTargetResponse.Code, createTargetResponse.Body.String())
	}
	var createdTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTargetResponse.Body).Decode(&createdTarget); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	createdRun := createAsyncRun(t, router, `{
		"scenarioId": "`+scenario.ID+`",
		"targetId": "`+createdTarget.ID+`",
		"totalRequests": 1,
		"concurrency": 1
	}`)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected target request to start before posting metrics")
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, `{
		"agentId": "agent-threshold-01",
		"collectedAt": "`+time.Now().UTC().Format(time.RFC3339Nano)+`",
		"cpuUsagePercent": 88.5,
		"memoryUsagePercent": 50,
		"processes": [
			{
				"pid": 2345,
				"name": "checkout-worker",
				"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml",
				"cpuUsagePercent": 91.2,
				"memoryRssBytes": 268435456,
				"fdCount": 42,
				"threadCount": 16
			}
		]
	}`))
	if metricsResponse.Code != http.StatusAccepted {
		t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, metricsResponse.Code, metricsResponse.Body.String())
	}

	releaseOnce.Do(func() { close(release) })
	finished := waitForRunStatus(t, router, createdRun.ID, "finished")
	tasks := waitForProfileTasksBySource(t, router, finished.ID, thresholdProfileTaskSource)
	if len(tasks) != 1 {
		t.Fatalf("expected one threshold-triggered profile task, got %#v", tasks)
	}
	task := tasks[0]
	if task.AgentID != "agent-threshold-01" || task.RunID != finished.ID || task.TargetID != createdTarget.ID {
		t.Fatalf("expected threshold task to target the breached run and agent, got %#v", task)
	}
	if task.Source != thresholdProfileTaskSource || task.ProfileType != "cpu" || task.ProfileSeconds != 1 {
		t.Fatalf("expected threshold task source/profile metadata, got %#v", task)
	}
	if task.PprofBaseURL != "http://127.0.0.1:6060/debug/pprof" {
		t.Fatalf("expected threshold task to use target pprof endpoint, got %#v", task)
	}
}

func waitForProfileTasksBySource(t *testing.T, router http.Handler, runID string, source string) []profileTaskRecord {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var matched []profileTaskRecord
	var latest []profileTaskRecord
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/profile-tasks?runId="+runID+"&limit=20", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("expected profile task list status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
		}
		if err := json.NewDecoder(response.Body).Decode(&latest); err != nil {
			t.Fatalf("expected profile task list JSON, got decode error: %v", err)
		}
		matched = matched[:0]
		for _, task := range latest {
			if task.Source == source {
				matched = append(matched, task)
			}
		}
		if len(matched) > 0 {
			return matched
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected run %s to create profile task source %q, latest tasks were %#v", runID, source, latest)
	return matched
}

func TestCreateRunPersistsRunHistory(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "history-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 4,
		"concurrency": 2,
		"timeoutMs": 1000
	}`)
	finished := waitForRunStatus(t, router, created.ID, "finished")
	if created.CreatedAt == "" {
		t.Fatal("expected persisted run to have createdAt")
	}

	restartedRouter := NewRouter()
	historyResponse := httptest.NewRecorder()
	restartedRouter.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/runs", nil))

	if historyResponse.Code != http.StatusOK {
		t.Fatalf("expected history status %d, got %d with body %s", http.StatusOK, historyResponse.Code, historyResponse.Body.String())
	}
	var history []createRunResponse
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatalf("expected history JSON, got decode error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one persisted run, got %#v", history)
	}
	if history[0].ID != finished.ID ||
		history[0].Status != finished.Status ||
		history[0].Name != finished.Name ||
		history[0].URL != finished.URL ||
		history[0].TotalRequests != finished.TotalRequests ||
		history[0].SuccessRequests != finished.SuccessRequests ||
		history[0].CreatedAt != finished.CreatedAt {
		t.Fatalf("expected persisted run %#v, got %#v", finished, history[0])
	}
	if hits.Load() != 4 {
		t.Fatalf("expected target to receive 4 requests, got %d", hits.Load())
	}
}

func TestCreateRunReturnsRunningBeforeTargetCompletes(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	responseCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(`{
		"name": "async-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 1,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)))
		responseCh <- response
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected background run to call target")
	}

	var createResponse *httptest.ResponseRecorder
	select {
	case createResponse = <-responseCh:
	case <-time.After(100 * time.Millisecond):
		close(release)
		<-responseCh
		t.Fatal("expected create run to return before target completes")
	}
	if createResponse.Code != http.StatusAccepted {
		close(release)
		t.Fatalf("expected async run status %d, got %d with body %s", http.StatusAccepted, createResponse.Code, createResponse.Body.String())
	}
	created := decodeRunResponse(t, createResponse)
	if created.Status != "running" {
		close(release)
		t.Fatalf("expected initial running status, got %#v", created)
	}

	lookupResponse := httptest.NewRecorder()
	router.ServeHTTP(lookupResponse, httptest.NewRequest(http.MethodGet, "/api/runs/"+created.ID, nil))
	if lookupResponse.Code != http.StatusOK {
		t.Fatalf("expected lookup status %d, got %d with body %s", http.StatusOK, lookupResponse.Code, lookupResponse.Body.String())
	}
	lookup := decodeRunResponse(t, lookupResponse)
	if lookup.Status != "running" {
		t.Fatalf("expected run to stay running while target is blocked, got %#v", lookup)
	}

	close(release)
	finished := waitForRunStatus(t, router, created.ID, "finished")
	if finished.SuccessRequests != 1 || hits.Load() != 1 {
		t.Fatalf("expected one successful background request, got run %#v and hits %d", finished, hits.Load())
	}
}

func TestRunningRunReportsCompletedSamplesBeforeFinish(t *testing.T) {
	setRunTestDatabase(t)

	firstDone := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var firstDoneOnce sync.Once
	var secondStartedOnce sync.Once
	var releaseOnce sync.Once
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit := hits.Add(1)
		switch hit {
		case 1:
			firstDoneOnce.Do(func() { close(firstDone) })
			w.WriteHeader(http.StatusOK)
		default:
			secondStartedOnce.Do(func() { close(secondStarted) })
			<-release
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(target.Close)
	defer releaseOnce.Do(func() { close(release) })

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "progress-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 2,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("expected first request to finish")
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("expected second request to start and block")
	}

	lookupResponse := httptest.NewRecorder()
	router.ServeHTTP(lookupResponse, httptest.NewRequest(http.MethodGet, "/api/runs/"+created.ID, nil))
	if lookupResponse.Code != http.StatusOK {
		t.Fatalf("expected lookup status %d, got %d with body %s", http.StatusOK, lookupResponse.Code, lookupResponse.Body.String())
	}
	progress := decodeRunResponse(t, lookupResponse)
	if progress.Status != "running" {
		t.Fatalf("expected running progress status, got %#v", progress)
	}
	if progress.SuccessRequests != 1 || progress.FailedRequests != 0 || progress.TotalRequests != 2 {
		t.Fatalf("expected one completed successful sample while run is active, got %#v", progress)
	}
	if progress.QPS <= 0 {
		t.Fatalf("expected running progress to include positive QPS, got %#v", progress)
	}

	releaseOnce.Do(func() { close(release) })
	finished := waitForRunStatus(t, router, created.ID, "finished")
	if finished.SuccessRequests != 2 {
		t.Fatalf("expected finished run to include both samples, got %#v", finished)
	}
}

func TestRunEventsTrackStartProgressAndFinish(t *testing.T) {
	setRunTestDatabase(t)

	firstDone := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var firstDoneOnce sync.Once
	var secondStartedOnce sync.Once
	var releaseOnce sync.Once
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit := hits.Add(1)
		switch hit {
		case 1:
			firstDoneOnce.Do(func() { close(firstDone) })
			w.WriteHeader(http.StatusOK)
		default:
			secondStartedOnce.Do(func() { close(secondStarted) })
			<-release
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(target.Close)
	defer releaseOnce.Do(func() { close(release) })

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "event-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 2,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("expected first request to finish")
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("expected second request to start and block")
	}

	runningEvents := listRunEvents(t, router, created.ID)
	assertRunEventType(t, runningEvents, "run_started")
	progress := assertRunEventType(t, runningEvents, "run_progress")
	if progress.SuccessRequests != 1 || progress.FailedRequests != 0 || progress.TotalRequests != 2 {
		t.Fatalf("expected progress event to include completed sample counts, got %#v", progress)
	}

	releaseOnce.Do(func() { close(release) })
	finished := waitForRunStatus(t, router, created.ID, "finished")
	if finished.SuccessRequests != 2 {
		t.Fatalf("expected finished run to include two successes, got %#v", finished)
	}

	finishedEvents := listRunEvents(t, router, created.ID)
	finishedEvent := assertRunEventType(t, finishedEvents, "run_finished")
	if finishedEvent.SuccessRequests != 2 || finishedEvent.Status != "finished" {
		t.Fatalf("expected finished event to include final result, got %#v", finishedEvent)
	}
}

func TestRunEventsStreamReturnsServerSentEvents(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "event-stream-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 1,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)
	waitForRunStatus(t, router, created.ID, "finished")
	waitForRunEventType(t, router, created.ID, "run_finished")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+created.ID+"/events/stream", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected stream status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "event: run_finished") {
		t.Fatalf("expected run_finished SSE event, got %q", body)
	}
	if !strings.Contains(body, `"type":"run_finished"`) {
		t.Fatalf("expected event JSON data in stream, got %q", body)
	}
}

func TestCreateRunAbortsWhenP95GuardrailIsExceeded(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "p95-guardrail-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 20,
		"concurrency": 1,
		"timeoutMs": 1000,
		"maxP95LatencyMs": 1
	}`)

	final := waitForRunStatus(t, router, created.ID, "aborted")
	if final.SuccessRequests >= final.TotalRequests {
		t.Fatalf("expected guardrail to stop before all requests completed, got %#v", final)
	}
	if hits.Load() >= int64(final.TotalRequests) {
		t.Fatalf("expected target to receive fewer than %d requests, got %d", final.TotalRequests, hits.Load())
	}

	events := waitForRunEventType(t, router, created.ID, "run_aborted")
	abortedEvent := assertRunEventType(t, events, "run_aborted")
	if abortedEvent.Status != "aborted" {
		t.Fatalf("expected aborted event to carry aborted status, got %#v", abortedEvent)
	}
}

func TestCreateRunAbortsWhenErrorRateGuardrailIsExceeded(t *testing.T) {
	setRunTestDatabase(t)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "error-rate-guardrail-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 20,
		"concurrency": 1,
		"timeoutMs": 1000,
		"maxErrorRatePercent": 10
	}`)

	final := waitForRunStatus(t, router, created.ID, "aborted")
	if final.FailedRequests == 0 {
		t.Fatalf("expected guardrail run to record failed requests, got %#v", final)
	}
	if final.FailedRequests >= final.TotalRequests {
		t.Fatalf("expected guardrail to stop before all requests completed, got %#v", final)
	}
	if hits.Load() >= int64(final.TotalRequests) {
		t.Fatalf("expected target to receive fewer than %d requests, got %d", final.TotalRequests, hits.Load())
	}
}

func TestGuardrailProgressEventsDoNotReturnToRunningAfterAbort(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "guardrail-event-status-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 20,
		"concurrency": 4,
		"timeoutMs": 1000,
		"maxP95LatencyMs": 1
	}`)
	waitForRunStatus(t, router, created.ID, "aborted")

	events := waitForRunEventType(t, router, created.ID, "run_aborted")
	sawAbortedStatus := false
	for _, event := range events {
		if event.Status == "aborted" {
			sawAbortedStatus = true
			continue
		}
		if sawAbortedStatus && event.Status == "running" {
			t.Fatalf("expected event status not to return to running after abort, got events %#v", events)
		}
	}
	if !sawAbortedStatus {
		t.Fatalf("expected at least one aborted event, got %#v", events)
	}
}

func TestStopRunCancelsRunningRun(t *testing.T) {
	setRunTestDatabase(t)

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var startedOnce sync.Once
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	responseCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(`{
		"name": "cancel-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 20,
		"concurrency": 1,
		"timeoutMs": 10000
	}`)))
		responseCh <- response
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected background run to start before stopping it")
	}

	var createResponse *httptest.ResponseRecorder
	select {
	case createResponse = <-responseCh:
	case <-time.After(100 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		<-responseCh
		t.Fatal("expected create run to return before target completes")
	}
	if createResponse.Code != http.StatusAccepted {
		releaseOnce.Do(func() { close(release) })
		t.Fatalf("expected async run status %d, got %d with body %s", http.StatusAccepted, createResponse.Code, createResponse.Body.String())
	}
	created := decodeRunResponse(t, createResponse)
	if created.Status != "running" {
		releaseOnce.Do(func() { close(release) })
		t.Fatalf("expected initial running status, got %#v", created)
	}

	stopResponse := httptest.NewRecorder()
	router.ServeHTTP(stopResponse, httptest.NewRequest(http.MethodPost, "/api/runs/"+created.ID+"/stop", nil))
	if stopResponse.Code != http.StatusOK {
		t.Fatalf("expected stop status %d, got %d with body %s", http.StatusOK, stopResponse.Code, stopResponse.Body.String())
	}
	stopped := decodeRunResponse(t, stopResponse)
	if stopped.Status != "canceled" {
		t.Fatalf("expected stop response to mark run canceled, got %#v", stopped)
	}

	final := waitForRunStatus(t, router, created.ID, "canceled")
	if final.SuccessRequests != 0 {
		t.Fatalf("expected canceled run to have no successful requests, got %#v", final)
	}
	releaseOnce.Do(func() { close(release) })
}

func TestCreateRunRejectsInvalidRequest(t *testing.T) {
	setRunTestDatabase(t)

	router := NewRouter()
	request := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(`{
		"url": "",
		"totalRequests": 0,
		"concurrency": 0
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error, got decode error: %v", err)
	}
	if body.Error == "" {
		t.Fatal("expected readable validation error")
	}
}
