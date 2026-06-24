package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestRunUploadsProfileFromFlags(t *testing.T) {
	payload := []byte("agent-command-profile")
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	var uploaded []byte
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_command_token" {
			t.Fatalf("expected command token header, got %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart request, got %v", err)
		}
		if r.FormValue("runId") != "run-command-1" || r.FormValue("targetName") != "checkout-command" {
			t.Fatalf("expected command metadata, got runId=%q targetName=%q", r.FormValue("runId"), r.FormValue("targetName"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected profile file upload, got %v", err)
		}
		defer file.Close()
		uploaded, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded profile to be readable, got %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "profile-command-1",
			"status":    "collected",
			"fileName":  "command-cpu.pprof",
			"sizeBytes": len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	output := &bytes.Buffer{}
	err := run(context.Background(), []string{
		"--control-plane", controlPlane.URL,
		"--token", "ait_command_token",
		"--profile-url", profileServer.URL + "/debug/pprof/profile?seconds=1",
		"--run-id", "run-command-1",
		"--target-name", "checkout-command",
		"--profile-type", "cpu",
		"--file-name", "command-cpu.pprof",
	}, output)
	if err != nil {
		t.Fatalf("expected command to upload profile, got %v", err)
	}
	if !bytes.Equal(uploaded, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"id":"profile-command-1"`)) {
		t.Fatalf("expected command output to include artifact response, got %s", output.String())
	}
}

func TestRunPrintsBuildInfo(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "v0.4.0", "abc123", "2026-06-23T00:00:00Z"
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})

	output := &bytes.Buffer{}
	if err := run(context.Background(), []string{"--build-info"}, output); err != nil {
		t.Fatalf("expected build info command to succeed, got %v", err)
	}

	var info map[string]string
	if err := json.NewDecoder(output).Decode(&info); err != nil {
		t.Fatalf("expected build info JSON, got %v", err)
	}
	if info["version"] != "v0.4.0" || info["commit"] != "abc123" || info["date"] != "2026-06-23T00:00:00Z" {
		t.Fatalf("expected injected build info, got %#v", info)
	}
}

func TestRunUploadsProfileFromPprofBaseURL(t *testing.T) {
	payload := []byte("agent-command-heap-profile")
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/pprof/heap" {
			t.Fatalf("expected heap profile path, got %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart request, got %v", err)
		}
		if r.FormValue("profileType") != "heap" || r.FormValue("sourceUrl") != profileServer.URL+"/debug/pprof/heap" {
			t.Fatalf("expected derived heap metadata, got profileType=%q sourceUrl=%q", r.FormValue("profileType"), r.FormValue("sourceUrl"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected heap profile file upload, got %v", err)
		}
		defer file.Close()
		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded profile to be readable, got %v", err)
		}
		if !bytes.Equal(uploaded, payload) {
			t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "profile-command-heap-1",
			"profileType": "heap",
			"status":      "collected",
			"fileName":    "run-command-heap-1-heap.pprof",
			"sizeBytes":   len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	output := &bytes.Buffer{}
	err := run(context.Background(), []string{
		"--control-plane", controlPlane.URL,
		"--token", "ait_command_token",
		"--pprof-base-url", profileServer.URL + "/debug/pprof",
		"--run-id", "run-command-heap-1",
		"--profile-type", "heap",
	}, output)
	if err != nil {
		t.Fatalf("expected command to upload heap profile from base URL, got %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"id":"profile-command-heap-1"`)) {
		t.Fatalf("expected command output to include heap artifact response, got %s", output.String())
	}
}

func TestRunUploadsControlledCommandProfileFromFlags(t *testing.T) {
	payload := []byte("cli-command-profile-payload")
	outputPath := t.TempDir() + "/cli-command-profile.prof"

	var uploaded []byte
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_command_token" {
			t.Fatalf("expected command token header, got %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart request, got %v", err)
		}
		for field, expected := range map[string]string{
			"runId":       "run-cli-command-profiler-1",
			"profileType": "perf",
			"sourceUrl":   "command://" + os.Args[0],
		} {
			if actual := r.FormValue(field); actual != expected {
				t.Fatalf("expected field %s=%q, got %q", field, expected, actual)
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected command profile upload, got %v", err)
		}
		defer file.Close()
		if header.Filename != "cli-command-perf.data" {
			t.Fatalf("expected command profile filename, got %q", header.Filename)
		}
		uploaded, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded command profile to be readable, got %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "profile-cli-command-profiler-1",
			"profileType": "perf",
			"status":      "collected",
			"fileName":    "cli-command-perf.data",
			"sizeBytes":   len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	output := &bytes.Buffer{}
	err := run(context.Background(), []string{
		"--control-plane", controlPlane.URL,
		"--token", "ait_command_token",
		"--run-id", "run-cli-command-profiler-1",
		"--profile-type", "perf",
		"--file-name", "cli-command-perf.data",
		"--profile-command", os.Args[0],
		"--profile-command-output", outputPath,
		"--profile-command-timeout", "2s",
		"--profile-command-arg", "-test.run=TestControlledCommandProfilerHelperProcess",
		"--profile-command-arg", "--",
		"--profile-command-arg", "--ait-command-profiler-helper",
		"--profile-command-arg", "{{output}}",
		"--profile-command-arg", string(payload),
	}, output)
	if err != nil {
		t.Fatalf("expected command profiler upload to succeed, got %v", err)
	}
	if !bytes.Equal(uploaded, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"id":"profile-cli-command-profiler-1"`)) {
		t.Fatalf("expected command output to include artifact response, got %s", output.String())
	}
}

func TestControlledCommandProfilerHelperProcess(t *testing.T) {
	helperIndex := -1
	for index, arg := range os.Args {
		if arg == "--ait-command-profiler-helper" {
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

func TestRunPollsProfileTasksOnce(t *testing.T) {
	payload := []byte("agent-command-polled-profile")
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/pprof/heap" {
			t.Fatalf("expected heap profile path, got %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	var completedArtifactID string
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.URL.Query().Get("agentId") != "agent-command-01" {
				t.Fatalf("expected agent id query, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":             "profile-task-command-1",
					"runId":          "run-command-polled-1",
					"targetName":     "checkout-command",
					"pprofBaseUrl":   profileServer.URL + "/debug/pprof",
					"profileType":    "heap",
					"profileSeconds": 1,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-artifacts":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Fatalf("expected artifact multipart request, got %v", err)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected uploaded artifact file, got %v", err)
			}
			defer file.Close()
			uploaded, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("expected upload payload to be readable, got %v", err)
			}
			if !bytes.Equal(uploaded, payload) {
				t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "profile-command-polled-artifact-1",
				"profileType": "heap",
				"status":      "collected",
				"fileName":    "run-command-polled-1-heap.pprof",
				"sizeBytes":   len(payload),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/profile-tasks/profile-task-command-1/complete":
			var input struct {
				ArtifactID string `json:"artifactId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("expected completion JSON, got %v", err)
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

	output := &bytes.Buffer{}
	err := run(context.Background(), []string{
		"--control-plane", controlPlane.URL,
		"--token", "ait_command_token",
		"--agent-id", "agent-command-01",
		"--poll-once",
	}, output)
	if err != nil {
		t.Fatalf("expected command to process profile task once, got %v", err)
	}
	if completedArtifactID != "profile-command-polled-artifact-1" {
		t.Fatalf("expected task completion to include artifact id, got %q", completedArtifactID)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"processed":1`)) {
		t.Fatalf("expected command output to include processed count, got %s", output.String())
	}
}

func TestRunDaemonFromFlagsSendsStartupAgentRequests(t *testing.T) {
	var cancel context.CancelFunc
	var mu sync.Mutex
	seen := map[string]bool{}

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_daemon_command_token" {
			t.Fatalf("expected daemon command token, got %q", r.Header.Get("Authorization"))
		}

		markSeen := func(name string) {
			mu.Lock()
			seen[name] = true
			ready := seen["heartbeat"] && seen["metrics"] && seen["tasks"]
			mu.Unlock()
			if ready {
				cancel()
			}
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/heartbeat":
			var heartbeat struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Hostname string `json:"hostname"`
			}
			if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
				t.Fatalf("expected daemon heartbeat JSON, got %v", err)
			}
			if heartbeat.ID != "agent-command-daemon-01" || heartbeat.Name != "checkout-daemon" || heartbeat.Hostname != "checkout-host" {
				t.Fatalf("expected daemon heartbeat from flags, got %#v", heartbeat)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": heartbeat.ID, "status": "online"})
			markSeen("heartbeat")
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/metrics":
			var metrics struct {
				AgentID string `json:"agentId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
				t.Fatalf("expected daemon metrics JSON, got %v", err)
			}
			if metrics.AgentID != "agent-command-daemon-01" {
				t.Fatalf("expected daemon metrics agent id from flags, got %#v", metrics)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(metrics)
			markSeen("metrics")
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.URL.Query().Get("agentId") != "agent-command-daemon-01" {
				t.Fatalf("expected daemon task poll agent id from flags, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
			markSeen("tasks")
		default:
			t.Fatalf("unexpected daemon command request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	ctx, stop := context.WithCancel(context.Background())
	cancel = stop
	defer cancel()

	output := &bytes.Buffer{}
	err := run(ctx, []string{
		"--daemon",
		"--control-plane", controlPlane.URL,
		"--token", "ait_daemon_command_token",
		"--agent-id", "agent-command-daemon-01",
		"--name", "checkout-daemon",
		"--hostname", "checkout-host",
		"--heartbeat-interval", "1h",
		"--metrics-interval", "1h",
		"--profile-task-interval", "1h",
	}, output)
	if err != nil {
		t.Fatalf("expected daemon command to stop cleanly after context cancellation, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"heartbeat", "metrics", "tasks"} {
		if !seen[name] {
			t.Fatalf("expected daemon command to send %s request, seen=%#v", name, seen)
		}
	}
}

func TestRunDaemonFromConfigSendsStartupAgentRequests(t *testing.T) {
	var cancel context.CancelFunc
	var mu sync.Mutex
	seen := map[string]bool{}

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_config_token" {
			t.Fatalf("expected config token, got %q", r.Header.Get("Authorization"))
		}

		markSeen := func(name string) {
			mu.Lock()
			seen[name] = true
			ready := seen["heartbeat"] && seen["metrics"] && seen["tasks"]
			mu.Unlock()
			if ready {
				cancel()
			}
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/heartbeat":
			var heartbeat struct {
				ID           string            `json:"id"`
				Name         string            `json:"name"`
				Hostname     string            `json:"hostname"`
				Labels       map[string]string `json:"labels"`
				Capabilities []string          `json:"capabilities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
				t.Fatalf("expected daemon heartbeat JSON, got %v", err)
			}
			if heartbeat.ID != "agent-config-daemon-01" || heartbeat.Name != "config-daemon" || heartbeat.Hostname != "config-host" {
				t.Fatalf("expected daemon heartbeat from config, got %#v", heartbeat)
			}
			if heartbeat.Labels["service"] != "checkout" || heartbeat.Labels["zone"] != "shanghai-a" {
				t.Fatalf("expected config labels, got %#v", heartbeat.Labels)
			}
			if len(heartbeat.Capabilities) != 4 || heartbeat.Capabilities[3] != "profile_tasks" {
				t.Fatalf("expected config capabilities, got %#v", heartbeat.Capabilities)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": heartbeat.ID, "status": "online"})
			markSeen("heartbeat")
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/metrics":
			var metrics struct {
				AgentID string `json:"agentId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
				t.Fatalf("expected daemon metrics JSON, got %v", err)
			}
			if metrics.AgentID != "agent-config-daemon-01" {
				t.Fatalf("expected daemon metrics agent id from config, got %#v", metrics)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(metrics)
			markSeen("metrics")
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.URL.Query().Get("agentId") != "agent-config-daemon-01" {
				t.Fatalf("expected daemon task poll agent id from config, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
			markSeen("tasks")
		default:
			t.Fatalf("unexpected daemon config request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	configPath := t.TempDir() + "/agent.json"
	configBody := `{
		"controlPlaneUrl": "` + controlPlane.URL + `",
		"token": "ait_config_token",
		"agentId": "agent-config-daemon-01",
		"name": "config-daemon",
		"hostname": "config-host",
		"labels": {
			"service": "checkout",
			"zone": "shanghai-a"
		},
		"capabilities": ["host_metrics", "process_metrics", "pprof", "profile_tasks"],
		"heartbeatInterval": "1h",
		"metricsInterval": "1h",
		"profileTaskInterval": "1h"
	}`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("expected config file to be written, got %v", err)
	}

	ctx, stop := context.WithCancel(context.Background())
	cancel = stop
	defer cancel()

	output := &bytes.Buffer{}
	err := run(ctx, []string{
		"--daemon",
		"--config", configPath,
	}, output)
	if err != nil {
		t.Fatalf("expected daemon command to read config and stop cleanly after cancellation, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"heartbeat", "metrics", "tasks"} {
		if !seen[name] {
			t.Fatalf("expected daemon command to send %s request from config, seen=%#v", name, seen)
		}
	}
}
