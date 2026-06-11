package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestCreateRunExecutesHTTPLoadTest(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/api/runs", body)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if hits.Load() != 12 {
		t.Fatalf("expected target to receive 12 requests, got %d", hits.Load())
	}

	var result struct {
		ID              string  `json:"id"`
		Status          string  `json:"status"`
		TotalRequests   int     `json:"totalRequests"`
		SuccessRequests int     `json:"successRequests"`
		FailedRequests  int     `json:"failedRequests"`
		QPS             float64 `json:"qps"`
		P95LatencyMs    float64 `json:"p95LatencyMs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("expected JSON response, got decode error: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected generated run id")
	}
	if result.Status != "finished" {
		t.Fatalf("expected finished status, got %q", result.Status)
	}
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
}

func TestCreateRunUsesWeightedBodyVariants(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/api/runs", body)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if preferredHits.Load() != 8 {
		t.Fatalf("expected preferred body to be used for all 8 requests, got %d", preferredHits.Load())
	}
	if ignoredHits.Load() != 0 {
		t.Fatalf("expected zero-weight body to be ignored, got %d hits", ignoredHits.Load())
	}
}

func TestCreateRunUsesWeightedQueryVariants(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/api/runs", body)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if preferredHits.Load() != 8 {
		t.Fatalf("expected preferred query to be used for all 8 requests, got %d", preferredHits.Load())
	}
	if ignoredHits.Load() != 0 {
		t.Fatalf("expected zero-weight query to be ignored, got %d hits", ignoredHits.Load())
	}
}

func TestCreateRunExecutesSavedScenario(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "scenarios.db"))

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

	runResponse := httptest.NewRecorder()
	router.ServeHTTP(runResponse, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(`{
		"scenarioId": "`+createdScenario.ID+`",
		"totalRequests": 5,
		"concurrency": 2
	}`)))

	if runResponse.Code != http.StatusOK {
		t.Fatalf("expected run status %d, got %d with body %s", http.StatusOK, runResponse.Code, runResponse.Body.String())
	}
	if hits.Load() != 5 {
		t.Fatalf("expected target to receive 5 scenario requests, got %d", hits.Load())
	}
	var result struct {
		ScenarioID      string `json:"scenarioId"`
		ScenarioName    string `json:"scenarioName"`
		Method          string `json:"method"`
		URL             string `json:"url"`
		TotalRequests   int    `json:"totalRequests"`
		SuccessRequests int    `json:"successRequests"`
	}
	if err := json.NewDecoder(runResponse.Body).Decode(&result); err != nil {
		t.Fatalf("expected run JSON, got decode error: %v", err)
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

func TestCreateRunRejectsInvalidRequest(t *testing.T) {
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
