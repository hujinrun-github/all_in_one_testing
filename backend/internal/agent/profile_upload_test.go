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
	"time"
)

func TestCollectAndUploadProfileFetchesPprofAndUploadsArtifact(t *testing.T) {
	payload := []byte("agent-cli-cpu-profile")
	var profileHits int
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profileHits++
		if r.URL.Path != "/debug/pprof/profile" {
			t.Fatalf("expected profile path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("seconds") != "1" {
			t.Fatalf("expected one-second profile query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	var uploadHits int
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadHits++
		if r.URL.Path != "/agent/v1/profile-artifacts" {
			t.Fatalf("expected artifact upload path, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ait_agent_token" {
			t.Fatalf("expected bearer token header, got %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart form, got %v", err)
		}
		for field, expected := range map[string]string{
			"runId":        "run-cli-1",
			"scenarioName": "checkout-smoke",
			"targetName":   "checkout-01",
			"profileType":  "cpu",
			"sourceUrl":    profileServer.URL + "/debug/pprof/profile?seconds=1",
		} {
			if actual := r.FormValue(field); actual != expected {
				t.Fatalf("expected field %s=%q, got %q", field, expected, actual)
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected uploaded file, got %v", err)
		}
		defer file.Close()
		if header.Filename != "checkout-cpu.pprof" {
			t.Fatalf("expected upload filename, got %q", header.Filename)
		}
		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded payload to be readable, got %v", err)
		}
		if !bytes.Equal(uploaded, payload) {
			t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "profile-agent-cli-1",
			"status":    "collected",
			"fileName":  "checkout-cpu.pprof",
			"sizeBytes": len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	result, err := CollectAndUploadProfile(context.Background(), controlPlane.Client(), ProfileUploadConfig{
		ControlPlaneURL: controlPlane.URL,
		Token:           "ait_agent_token",
		ProfileURL:      profileServer.URL + "/debug/pprof/profile?seconds=1",
		RunID:           "run-cli-1",
		ScenarioName:    "checkout-smoke",
		TargetName:      "checkout-01",
		ProfileType:     "cpu",
		FileName:        "checkout-cpu.pprof",
	})
	if err != nil {
		t.Fatalf("expected profile upload to succeed, got %v", err)
	}
	if profileHits != 1 || uploadHits != 1 {
		t.Fatalf("expected one profile fetch and one upload, got profile=%d upload=%d", profileHits, uploadHits)
	}
	if result.ID != "profile-agent-cli-1" || result.Status != "collected" {
		t.Fatalf("expected collected artifact response, got %#v", result)
	}
	if result.SizeBytes != int64(len(payload)) {
		t.Fatalf("expected size %d, got %d", len(payload), result.SizeBytes)
	}
}

func TestCollectAndUploadProfileBuildsProfileURLFromPprofBaseAndType(t *testing.T) {
	payload := []byte("agent-cli-heap-profile")
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/pprof/heap" {
			t.Fatalf("expected heap profile path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(profileServer.Close)

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart form, got %v", err)
		}
		for field, expected := range map[string]string{
			"runId":       "run-heap-1",
			"profileType": "heap",
			"sourceUrl":   profileServer.URL + "/debug/pprof/heap",
		} {
			if actual := r.FormValue(field); actual != expected {
				t.Fatalf("expected field %s=%q, got %q", field, expected, actual)
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected uploaded heap file, got %v", err)
		}
		defer file.Close()
		if header.Filename != "run-heap-1-heap.pprof" {
			t.Fatalf("expected derived heap filename, got %q", header.Filename)
		}
		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded payload to be readable, got %v", err)
		}
		if !bytes.Equal(uploaded, payload) {
			t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "profile-agent-heap-1",
			"profileType": "heap",
			"status":      "collected",
			"fileName":    "run-heap-1-heap.pprof",
			"sizeBytes":   len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	result, err := CollectAndUploadProfile(context.Background(), controlPlane.Client(), ProfileUploadConfig{
		ControlPlaneURL: controlPlane.URL,
		Token:           "ait_agent_token",
		PprofBaseURL:    profileServer.URL + "/debug/pprof",
		RunID:           "run-heap-1",
		ProfileType:     "heap",
	})
	if err != nil {
		t.Fatalf("expected derived heap profile upload to succeed, got %v", err)
	}
	if result.ID != "profile-agent-heap-1" || result.ProfileType != "heap" {
		t.Fatalf("expected heap artifact response, got %#v", result)
	}
}

func TestCollectAndUploadCommandProfileRunsCommandAndUploadsOutput(t *testing.T) {
	outputPath := t.TempDir() + "/command-profile.prof"
	payload := []byte("command-profile-payload")

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_agent_token" {
			t.Fatalf("expected bearer token header, got %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("expected multipart form, got %v", err)
		}
		for field, expected := range map[string]string{
			"runId":       "run-command-profiler-1",
			"profileType": "perf",
			"sourceUrl":   "command://" + os.Args[0],
		} {
			if actual := r.FormValue(field); actual != expected {
				t.Fatalf("expected field %s=%q, got %q", field, expected, actual)
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected uploaded command profile, got %v", err)
		}
		defer file.Close()
		if header.Filename != "command-perf.data" {
			t.Fatalf("expected command profile filename, got %q", header.Filename)
		}
		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("expected uploaded command profile to be readable, got %v", err)
		}
		if !bytes.Equal(uploaded, payload) {
			t.Fatalf("expected uploaded payload %q, got %q", payload, uploaded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "profile-command-profiler-1",
			"profileType": "perf",
			"status":      "collected",
			"fileName":    "command-perf.data",
			"sizeBytes":   len(payload),
		})
	}))
	t.Cleanup(controlPlane.Close)

	result, err := CollectAndUploadCommandProfile(context.Background(), controlPlane.Client(), CommandProfileConfig{
		ProfileUploadConfig: ProfileUploadConfig{
			ControlPlaneURL: controlPlane.URL,
			Token:           "ait_agent_token",
			RunID:           "run-command-profiler-1",
			ProfileType:     "perf",
			FileName:        "command-perf.data",
		},
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestCommandProfilerHelperProcess"},
		Env:        []string{"AIT_COMMAND_PROFILER_HELPER=1", "AIT_COMMAND_PROFILER_OUTPUT=" + outputPath, "AIT_COMMAND_PROFILER_PAYLOAD=" + string(payload)},
		OutputPath: outputPath,
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("expected command profile upload to succeed, got %v", err)
	}
	if result.ID != "profile-command-profiler-1" || result.ProfileType != "perf" {
		t.Fatalf("expected command profile artifact response, got %#v", result)
	}
}

func TestCommandProfilerHelperProcess(t *testing.T) {
	if os.Getenv("AIT_COMMAND_PROFILER_HELPER") != "1" {
		return
	}
	outputPath := os.Getenv("AIT_COMMAND_PROFILER_OUTPUT")
	payload := os.Getenv("AIT_COMMAND_PROFILER_PAYLOAD")
	if outputPath == "" {
		t.Fatal("AIT_COMMAND_PROFILER_OUTPUT is required")
	}
	if err := os.WriteFile(outputPath, []byte(payload), 0o600); err != nil {
		t.Fatalf("failed to write command profiler output: %v", err)
	}
	os.Exit(0)
}
