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
