package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestScenarioAPIPersistsScenariosToSQLite(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "scenarios.db")
	t.Setenv("SCENARIO_DB_PATH", databasePath)

	router := NewRouter()
	createBody := bytes.NewBufferString(`{
		"name": "checkout-payment",
		"protocol": "HTTP",
		"method": "POST",
		"baseUrl": "https://api.example.test",
		"path": "/api/payments",
		"queryVariants": [
			{ "name": "shanghai", "weight": "80", "queryParams": "tenant=shanghai" }
		],
		"headers": [
			{ "key": "Content-Type", "value": "application/json" }
		],
		"bodyVariants": [
			{ "name": "small-order", "weight": "70", "body": "{\"amount\":100}" }
		],
		"flowSteps": [
			{ "id": "step-login", "name": "HTTP login", "type": "request", "protocol": "HTTP", "method": "POST", "path": "/login" },
			{ "id": "step-assert", "name": "assert status", "type": "assertion", "assertion": "status < 400" }
		],
		"timeoutMs": "2500",
		"retryCount": "2",
		"assertion": "status < 400"
	}`)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/scenarios", createBody)
	createResponse := httptest.NewRecorder()

	router.ServeHTTP(createResponse, createRequest)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated scenario id")
	}
	if created.Name != "checkout-payment" {
		t.Fatalf("expected stored scenario name, got %q", created.Name)
	}

	restartedRouter := NewRouter()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/scenarios", nil)
	listResponse := httptest.NewRecorder()

	restartedRouter.ServeHTTP(listResponse, listRequest)

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var listed []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		FlowSteps []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Type      string `json:"type"`
			Protocol  string `json:"protocol"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Assertion string `json:"assertion"`
		} `json:"flowSteps"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("expected scenario list JSON, got decode error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one stored scenario, got %d", len(listed))
	}
	if listed[0].ID != created.ID || listed[0].Name != "checkout-payment" {
		t.Fatalf("expected persisted scenario %q, got %#v", created.ID, listed[0])
	}
	if len(listed[0].FlowSteps) != 2 {
		t.Fatalf("expected two persisted flow steps, got %#v", listed[0].FlowSteps)
	}
	if listed[0].FlowSteps[0].Name != "HTTP login" || listed[0].FlowSteps[0].Protocol != "HTTP" || listed[0].FlowSteps[1].Assertion != "status < 400" {
		t.Fatalf("expected persisted flow step details, got %#v", listed[0].FlowSteps)
	}

	databaseHeader := make([]byte, 16)
	databaseFile, err := os.Open(databasePath)
	if err != nil {
		t.Fatalf("expected SQLite database file at %s, got error: %v", databasePath, err)
	}
	defer databaseFile.Close()
	if _, err := databaseFile.Read(databaseHeader); err != nil {
		t.Fatalf("expected readable SQLite database header, got error: %v", err)
	}
	if string(databaseHeader) != "SQLite format 3\x00" {
		t.Fatalf("expected SQLite database header, got %q", string(databaseHeader))
	}
}

func TestScenarioAPIStoresAndFiltersProjectEnvironment(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "scenarios.db"))
	router := NewRouter()

	createScenario := func(body string) struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	} {
		t.Helper()

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d with body %s", http.StatusCreated, response.Code, response.Body.String())
		}

		var created struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			ProjectID   string `json:"projectId"`
			Environment string `json:"environment"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			t.Fatalf("expected created scenario JSON, got decode error: %v", err)
		}
		return created
	}

	checkout := createScenario(`{
		"name": "checkout-staging-smoke",
		"projectId": "project-checkout",
		"environment": "staging",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://checkout.example.test",
		"path": "/api/health"
	}`)
	createScenario(`{
		"name": "billing-prod-smoke",
		"projectId": "project-billing",
		"environment": "prod",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://billing.example.test",
		"path": "/api/health"
	}`)

	if checkout.ProjectID != "project-checkout" || checkout.Environment != "staging" {
		t.Fatalf("expected created scenario to preserve project/environment, got %#v", checkout)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/scenarios?projectId=project-checkout&environment=staging", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}

	var listed []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		ProjectID   string `json:"projectId"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("expected scenario list JSON, got decode error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one matching scenario, got %d: %#v", len(listed), listed)
	}
	if listed[0].ID != checkout.ID || listed[0].Name != "checkout-staging-smoke" ||
		listed[0].ProjectID != "project-checkout" || listed[0].Environment != "staging" {
		t.Fatalf("expected filtered checkout staging scenario, got %#v", listed[0])
	}
}

func TestScenarioAPISupportsReadUpdateAndDelete(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "scenarios.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewBufferString(`{
		"name": "gateway-smoke",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://127.0.0.1:8080",
		"path": "/api/health",
		"timeoutMs": "1000",
		"retryCount": "0",
		"assertion": "status < 400"
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected created scenario JSON, got decode error: %v", err)
	}

	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/api/scenarios/"+created.ID, bytes.NewBufferString(`{
		"name": "gateway-smoke-updated",
		"protocol": "HTTP",
		"method": "GET",
		"baseUrl": "http://127.0.0.1:8080",
		"path": "/api/health",
		"timeoutMs": "1500",
		"retryCount": "1",
		"assertion": "status < 500"
	}`)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/scenarios/"+created.ID, nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, getResponse.Code, getResponse.Body.String())
	}
	var found struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		TimeoutMs  string `json:"timeoutMs"`
		RetryCount string `json:"retryCount"`
		Assertion  string `json:"assertion"`
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&found); err != nil {
		t.Fatalf("expected scenario JSON, got decode error: %v", err)
	}
	if found.ID != created.ID || found.Name != "gateway-smoke-updated" || found.TimeoutMs != "1500" || found.RetryCount != "1" || found.Assertion != "status < 500" {
		t.Fatalf("expected updated scenario, got %#v", found)
	}

	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/scenarios/"+created.ID, nil))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body.String())
	}

	deletedGetResponse := httptest.NewRecorder()
	router.ServeHTTP(deletedGetResponse, httptest.NewRequest(http.MethodGet, "/api/scenarios/"+created.ID, nil))
	if deletedGetResponse.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusNotFound, deletedGetResponse.Code, deletedGetResponse.Body.String())
	}
}
