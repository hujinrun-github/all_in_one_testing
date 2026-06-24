package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestProcessProfileTasksOncePollsUploadsAndCompletesTask(t *testing.T) {
	payload := []byte("polled-heap-profile")
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/pprof/heap" {
			t.Fatalf("expected heap profile path, got %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	var uploaded []byte
	var completedArtifactID string
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.Header.Get("Authorization") != "Bearer ait_task_token" {
				t.Fatalf("expected poll bearer token, got %q", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("agentId") != "agent-checkout-01" {
				t.Fatalf("expected agent id query, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":             "profile-task-1",
					"runId":          "run-polled-1",
					"scenarioName":   "checkout-smoke",
					"targetName":     "checkout-01",
					"pprofBaseUrl":   profileServer.URL + "/debug/pprof",
					"profileType":    "heap",
					"profileSeconds": 1,
					"status":         "leased",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-artifacts":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Fatalf("expected artifact multipart request, got %v", err)
			}
			if r.FormValue("runId") != "run-polled-1" || r.FormValue("profileType") != "heap" {
				t.Fatalf("expected polled task metadata, got runId=%q profileType=%q", r.FormValue("runId"), r.FormValue("profileType"))
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected uploaded artifact file, got %v", err)
			}
			defer file.Close()
			uploaded, err = io.ReadAll(file)
			if err != nil {
				t.Fatalf("expected uploaded artifact to be readable, got %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "profile-polled-artifact-1",
				"profileType": "heap",
				"status":      "collected",
				"fileName":    "run-polled-1-heap.pprof",
				"sizeBytes":   len(payload),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-tasks/profile-task-1/complete":
			var input struct {
				ArtifactID string `json:"artifactId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("expected task completion JSON, got %v", err)
			}
			completedArtifactID = input.ArtifactID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "profile-task-1",
				"status":     "completed",
				"artifactId": input.ArtifactID,
			})
		default:
			t.Fatalf("unexpected control plane request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	result, err := ProcessProfileTasksOnce(context.Background(), controlPlane.Client(), ProfileTaskPollConfig{
		ControlPlaneURL: controlPlane.URL,
		Token:           "ait_task_token",
		AgentID:         "agent-checkout-01",
	})
	if err != nil {
		t.Fatalf("expected task processing to succeed, got %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("expected one processed task, got %#v", result)
	}
	if !bytes.Equal(uploaded, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
	}
	if completedArtifactID != "profile-polled-artifact-1" {
		t.Fatalf("expected completion to reference uploaded artifact, got %q", completedArtifactID)
	}
}

func TestProcessProfileTasksOnceRunsCommandProfilerTask(t *testing.T) {
	payload := []byte("polled-command-profile")
	outputPath := t.TempDir() + "/polled-command-profile.prof"

	var uploaded []byte
	var completedArtifactID string
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.Header.Get("Authorization") != "Bearer ait_task_token" {
				t.Fatalf("expected poll bearer token, got %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":                      "profile-task-command-1",
					"runId":                   "run-polled-command-1",
					"targetName":              "checkout-01",
					"profileType":             "perf",
					"profileSeconds":          2,
					"profileCommand":          os.Args[0],
					"profileCommandArgs":      []string{"-test.run=TestProfileTaskCommandProfilerHelperProcess", "--", "--ait-agent-command-task-helper", "{{output}}", string(payload)},
					"profileCommandOutput":    outputPath,
					"profileCommandTimeoutMs": 2000,
					"status":                  "leased",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-artifacts":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Fatalf("expected artifact multipart request, got %v", err)
			}
			if r.FormValue("runId") != "run-polled-command-1" || r.FormValue("profileType") != "perf" || r.FormValue("sourceUrl") != "command://"+os.Args[0] {
				t.Fatalf("expected command task metadata, got runId=%q profileType=%q sourceUrl=%q", r.FormValue("runId"), r.FormValue("profileType"), r.FormValue("sourceUrl"))
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected uploaded command artifact file, got %v", err)
			}
			defer file.Close()
			uploaded, err = io.ReadAll(file)
			if err != nil {
				t.Fatalf("expected uploaded command artifact to be readable, got %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "profile-polled-command-artifact-1",
				"profileType": "perf",
				"status":      "collected",
				"fileName":    "run-polled-command-1-perf.pprof",
				"sizeBytes":   len(payload),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-tasks/profile-task-command-1/complete":
			var input struct {
				ArtifactID string `json:"artifactId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("expected command task completion JSON, got %v", err)
			}
			completedArtifactID = input.ArtifactID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "profile-task-command-1",
				"status":     "completed",
				"artifactId": input.ArtifactID,
			})
		default:
			t.Fatalf("unexpected control plane request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	result, err := ProcessProfileTasksOnce(context.Background(), controlPlane.Client(), ProfileTaskPollConfig{
		ControlPlaneURL: controlPlane.URL,
		Token:           "ait_task_token",
		AgentID:         "agent-checkout-01",
	})
	if err != nil {
		t.Fatalf("expected command task processing to succeed, got %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("expected one processed command task, got %#v", result)
	}
	if !bytes.Equal(uploaded, payload) {
		t.Fatalf("expected uploaded command payload %q, got %q", payload, uploaded)
	}
	if completedArtifactID != "profile-polled-command-artifact-1" {
		t.Fatalf("expected command completion to reference uploaded artifact, got %q", completedArtifactID)
	}
}

func TestProfileTaskCommandProfilerHelperProcess(t *testing.T) {
	helperIndex := -1
	for index, arg := range os.Args {
		if arg == "--ait-agent-command-task-helper" {
			helperIndex = index
			break
		}
	}
	if helperIndex == -1 {
		return
	}
	if len(os.Args) <= helperIndex+2 {
		t.Fatal("helper requires output path and payload arguments")
	}
	if err := os.WriteFile(os.Args[helperIndex+1], []byte(os.Args[helperIndex+2]), 0o600); err != nil {
		t.Fatalf("failed to write command profiler output: %v", err)
	}
	os.Exit(0)
}
