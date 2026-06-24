package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestListProfileTaskTemplatesSeedsDefaultTemplates(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/profile-task-templates", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected profile template list status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var templates []profileTaskTemplateRecord
	if err := json.NewDecoder(response.Body).Decode(&templates); err != nil {
		t.Fatalf("expected profile template list JSON, got decode error: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected default go pprof and linux perf templates, got %#v", templates)
	}
	if templates[0].ID != "go_pprof" || templates[0].Kind != "pprof" || !templates[0].RequiresProfileEndpoint || len(templates[0].ProfileTypes) == 0 {
		t.Fatalf("expected go pprof template first, got %#v", templates[0])
	}
	if templates[1].ID != "linux_perf" || templates[1].Kind != "command" || templates[1].ProfileCommand != "perf" {
		t.Fatalf("expected linux perf command template second, got %#v", templates[1])
	}
	if !templates[1].RequiresPID || templates[1].ProfileType != "perf" || templates[1].ProfileCommandOutput != "/tmp/{{targetNameSlug}}-perf.data" {
		t.Fatalf("expected linux perf profile metadata, got %#v", templates[1])
	}
	if len(templates[1].ProfileCommandArgs) != 11 || templates[1].ProfileCommandArgs[4] != "{{pid}}" || templates[1].ProfileCommandArgs[7] != "{{output}}" || templates[1].ProfileCommandArgs[10] != "{{seconds}}" {
		t.Fatalf("expected linux perf command placeholders, got %#v", templates[1].ProfileCommandArgs)
	}
	if templates[1].TimeoutBufferSeconds != 15 || !templates[1].Enabled || templates[1].CreatedAt == "" || templates[1].UpdatedAt == "" {
		t.Fatalf("expected linux perf template defaults, got %#v", templates[1])
	}
}

func TestProfileTaskTemplateLifecycleCreatesUpdatesAndDeletesTemplate(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/api/profile-task-templates", bytes.NewBufferString(`{
		"id": "async_profiler",
		"name": "Async Profiler",
		"description": "Collect async-profiler samples from a JVM process.",
		"kind": "command",
		"profileTypes": ["jfr"],
		"profileType": "jfr",
		"requiresPid": true,
		"profileCommand": "asprof",
		"profileCommandArgs": ["-d", "{{seconds}}", "-f", "{{output}}", "{{pid}}"],
		"profileCommandOutput": "/tmp/{{targetNameSlug}}.jfr",
		"timeoutBufferSeconds": 10,
		"displayOrder": 30
	}`)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected template create status %d, got %d with body %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created profileTaskTemplateRecord
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("expected template create JSON, got decode error: %v", err)
	}
	if created.ID != "async_profiler" || created.Kind != "command" || created.ProfileCommand != "asprof" || !created.RequiresPID || !created.Enabled {
		t.Fatalf("expected created command template metadata, got %#v", created)
	}
	if len(created.ProfileCommandArgs) != 5 || created.ProfileCommandArgs[1] != "{{seconds}}" || created.ProfileCommandArgs[4] != "{{pid}}" {
		t.Fatalf("expected created command template args, got %#v", created.ProfileCommandArgs)
	}

	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/api/profile-task-templates/async_profiler", bytes.NewBufferString(`{
		"name": "Async Profiler Wall",
		"description": "Collect wall-clock async-profiler samples.",
		"kind": "command",
		"profileTypes": ["jfr"],
		"profileType": "jfr",
		"requiresPid": true,
		"profileCommand": "asprof",
		"profileCommandArgs": ["--event", "wall", "-d", "{{seconds}}", "-f", "{{output}}", "{{pid}}"],
		"profileCommandOutput": "/var/tmp/{{targetNameSlug}}.jfr",
		"timeoutBufferSeconds": 20,
		"displayOrder": 25,
		"enabled": true
	}`)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected template update status %d, got %d with body %s", http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	}
	var updated profileTaskTemplateRecord
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil {
		t.Fatalf("expected template update JSON, got decode error: %v", err)
	}
	if updated.ID != "async_profiler" || updated.Name != "Async Profiler Wall" || updated.ProfileCommandOutput != "/var/tmp/{{targetNameSlug}}.jfr" || updated.TimeoutBufferSeconds != 20 {
		t.Fatalf("expected updated template fields, got %#v", updated)
	}
	if len(updated.ProfileCommandArgs) != 7 || updated.ProfileCommandArgs[1] != "wall" {
		t.Fatalf("expected updated command args, got %#v", updated.ProfileCommandArgs)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/profile-task-templates", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected template list status %d, got %d with body %s", http.StatusOK, listResponse.Code, listResponse.Body.String())
	}
	var templates []profileTaskTemplateRecord
	if err := json.NewDecoder(listResponse.Body).Decode(&templates); err != nil {
		t.Fatalf("expected template list JSON, got decode error: %v", err)
	}
	var foundUpdated bool
	for _, template := range templates {
		if template.ID == "async_profiler" {
			foundUpdated = true
			if template.DisplayOrder != 25 || template.Name != "Async Profiler Wall" {
				t.Fatalf("expected listed template to include update, got %#v", template)
			}
		}
	}
	if !foundUpdated {
		t.Fatalf("expected created template in enabled list, got %#v", templates)
	}

	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/profile-task-templates/async_profiler", nil))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected template delete status %d, got %d with body %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body.String())
	}

	afterDeleteResponse := httptest.NewRecorder()
	router.ServeHTTP(afterDeleteResponse, httptest.NewRequest(http.MethodGet, "/api/profile-task-templates", nil))
	if afterDeleteResponse.Code != http.StatusOK {
		t.Fatalf("expected template list after delete status %d, got %d with body %s", http.StatusOK, afterDeleteResponse.Code, afterDeleteResponse.Body.String())
	}
	var afterDeleteTemplates []profileTaskTemplateRecord
	if err := json.NewDecoder(afterDeleteResponse.Body).Decode(&afterDeleteTemplates); err != nil {
		t.Fatalf("expected template list after delete JSON, got decode error: %v", err)
	}
	for _, template := range afterDeleteTemplates {
		if template.ID == "async_profiler" {
			t.Fatalf("expected deleted template to be hidden from enabled list, got %#v", afterDeleteTemplates)
		}
	}
}

func TestCreateProfileTaskTemplateRejectsInvalidCommandTemplate(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/profile-task-templates", bytes.NewBufferString(`{
		"id": "unsafe_command",
		"name": "Unsafe command",
		"kind": "command",
		"profileTypes": ["perf"],
		"profileType": "perf",
		"requiresPid": true,
		"profileCommand": "perf record",
		"profileCommandArgs": ["-p", "{{pid}}"],
		"profileCommandOutput": "/tmp/{{targetNameSlug}}.data"
	}`)))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid template status %d, got %d with body %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "profileCommand must be a single executable") {
		t.Fatalf("expected command validation error, got body %s", response.Body.String())
	}
}
