package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDownloadProfileArtifactReturnsStoredPayload(t *testing.T) {
	setRunTestDatabase(t)

	payload := []byte("pprof-cpu-download-sample")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store := newProfileArtifactStoreFromEnv()
	saved, err := store.save(profileArtifactRecord{
		ID:           "profile-download-1",
		RunID:        "run-download-1",
		ScenarioID:   "scenario-download-1",
		ScenarioName: "download-checkout",
		TargetID:     "target-download-1",
		TargetName:   "checkout-target",
		ProfileType:  "cpu",
		Status:       "collected",
		SourceURL:    "http://127.0.0.1:6060/debug/pprof/profile?seconds=1",
		FileName:     "run-download-1-cpu.pprof",
		ContentType:  "application/octet-stream",
		SizeBytes:    int64(len(payload)),
		StartedAt:    now,
		FinishedAt:   now,
	}, payload)
	if err != nil {
		t.Fatalf("expected artifact save to succeed, got %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/profile-artifacts/"+saved.ID+"/download", nil)
	NewRouter().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected download status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if !bytes.Equal(response.Body.Bytes(), payload) {
		t.Fatalf("expected downloaded payload %q, got %q", payload, response.Body.Bytes())
	}
	if response.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("expected pprof content type, got %q", response.Header().Get("Content-Type"))
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), `filename=run-download-1-cpu.pprof`) {
		t.Fatalf("expected content disposition to include filename, got %q", response.Header().Get("Content-Disposition"))
	}
}

func TestProfileArtifactsWithSameFileNameKeepSeparatePayloads(t *testing.T) {
	setRunTestDatabase(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	store := newProfileArtifactStoreFromEnv()
	firstPayload := []byte("first-run-profile")
	first, err := store.save(profileArtifactRecord{
		ID:          "profile-same-name-first",
		RunID:       "run-same-name-first",
		ProfileType: "cpu",
		Status:      "collected",
		SourceURL:   "http://127.0.0.1:6060/debug/pprof/profile?seconds=1",
		FileName:    "cpu.pprof",
		ContentType: "application/octet-stream",
		SizeBytes:   int64(len(firstPayload)),
		StartedAt:   now,
		FinishedAt:  now,
	}, firstPayload)
	if err != nil {
		t.Fatalf("expected first artifact save to succeed, got %v", err)
	}
	secondPayload := []byte("second-run-profile")
	second, err := store.save(profileArtifactRecord{
		ID:          "profile-same-name-second",
		RunID:       "run-same-name-second",
		ProfileType: "cpu",
		Status:      "collected",
		SourceURL:   "http://127.0.0.1:6060/debug/pprof/profile?seconds=1",
		FileName:    "cpu.pprof",
		ContentType: "application/octet-stream",
		SizeBytes:   int64(len(secondPayload)),
		StartedAt:   now,
		FinishedAt:  now,
	}, secondPayload)
	if err != nil {
		t.Fatalf("expected second artifact save to succeed, got %v", err)
	}
	if first.StoragePath == second.StoragePath {
		t.Fatalf("expected same display filename to use distinct storage paths, got %q", first.StoragePath)
	}

	for _, testCase := range []struct {
		name     string
		id       string
		expected []byte
	}{
		{name: "first", id: first.ID, expected: firstPayload},
		{name: "second", id: second.ID, expected: secondPayload},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/profile-artifacts/"+testCase.id+"/download", nil)
			NewRouter().ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("expected download status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
			}
			if !bytes.Equal(response.Body.Bytes(), testCase.expected) {
				t.Fatalf("expected downloaded payload %q, got %q", testCase.expected, response.Body.Bytes())
			}
			if !strings.Contains(response.Header().Get("Content-Disposition"), `filename=cpu.pprof`) {
				t.Fatalf("expected download filename to preserve display filename, got %q", response.Header().Get("Content-Disposition"))
			}
		})
	}
}
