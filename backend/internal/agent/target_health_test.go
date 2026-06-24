package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessTargetHealthChecksOncePollsBoundTargetsChecksAndReports(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Fatalf("expected target health path /ready, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(targetServer.Close)

	var reported struct {
		AgentID        string  `json:"agentId"`
		TargetID       string  `json:"targetId"`
		Status         string  `json:"status"`
		URL            string  `json:"url"`
		ExpectedStatus int     `json:"expectedStatus"`
		ObservedStatus int     `json:"observedStatus"`
		LatencyMs      float64 `json:"latencyMs"`
		Error          string  `json:"error"`
		CheckedAt      string  `json:"checkedAt"`
	}
	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_health_token" {
			t.Fatalf("expected target health bearer token, got %q", r.Header.Get("Authorization"))
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/targets":
			if r.URL.Query().Get("agentId") != "agent-checkout-health-01" {
				t.Fatalf("expected agent id query, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":      "target-checkout-health-01",
					"name":    "checkout-service",
					"baseUrl": targetServer.URL,
					"healthCheck": map[string]any{
						"enabled":        true,
						"path":           "/ready",
						"expectedStatus": http.StatusNoContent,
						"timeoutMs":      1000,
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/target-health-checks":
			if err := json.NewDecoder(r.Body).Decode(&reported); err != nil {
				t.Fatalf("expected target health report JSON, got %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(reported)
		default:
			t.Fatalf("unexpected target health request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	result, err := ProcessTargetHealthChecksOnce(context.Background(), controlPlane.Client(), TargetHealthCheckConfig{
		ControlPlaneURL: controlPlane.URL,
		Token:           "ait_health_token",
		AgentID:         "agent-checkout-health-01",
	})
	if err != nil {
		t.Fatalf("expected target health processing to succeed, got %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("expected one target health check to be processed, got %#v", result)
	}
	if reported.AgentID != "agent-checkout-health-01" || reported.TargetID != "target-checkout-health-01" {
		t.Fatalf("expected reported agent and target ids, got %#v", reported)
	}
	if reported.Status != "healthy" || reported.ExpectedStatus != http.StatusNoContent || reported.ObservedStatus != http.StatusNoContent {
		t.Fatalf("expected healthy 204 report, got %#v", reported)
	}
	if reported.URL != targetServer.URL+"/ready" || reported.CheckedAt == "" || reported.LatencyMs < 0 || reported.Error != "" {
		t.Fatalf("expected target health report details, got %#v", reported)
	}
}
