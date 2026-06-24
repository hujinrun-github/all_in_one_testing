package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeMetricsSampler struct {
	metrics AgentMetrics
}

func (sampler fakeMetricsSampler) SampleMetrics(_ context.Context, agentID string) (AgentMetrics, error) {
	metrics := sampler.metrics
	metrics.AgentID = agentID
	return metrics, nil
}

func TestRunDaemonSendsStartupHeartbeatMetricsAndPollsProfileTasks(t *testing.T) {
	var cancel context.CancelFunc
	var mu sync.Mutex
	seen := map[string]bool{}

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Fatalf("expected target health path /ready, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(targetServer.Close)

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ait_daemon_token" {
			t.Fatalf("expected daemon bearer token, got %q", r.Header.Get("Authorization"))
		}

		markSeen := func(name string) {
			mu.Lock()
			seen[name] = true
			ready := seen["heartbeat"] && seen["metrics"] && seen["tasks"] && seen["targetHealth"]
			mu.Unlock()
			if ready {
				cancel()
			}
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/heartbeat":
			var heartbeat AgentHeartbeat
			if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
				t.Fatalf("expected heartbeat JSON, got %v", err)
			}
			if heartbeat.ID != "agent-daemon-01" || heartbeat.Name != "checkout-daemon" || heartbeat.Hostname != "checkout-host" {
				t.Fatalf("expected daemon heartbeat metadata, got %#v", heartbeat)
			}
			if heartbeat.Labels["service"] != "checkout" || len(heartbeat.Capabilities) != 3 {
				t.Fatalf("expected daemon labels and capabilities, got %#v", heartbeat)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": heartbeat.ID, "status": "online"})
			markSeen("heartbeat")
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/metrics":
			var metrics AgentMetrics
			if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
				t.Fatalf("expected metrics JSON, got %v", err)
			}
			if metrics.AgentID != "agent-daemon-01" || metrics.CPUUsagePercent != 42.5 || metrics.MemoryUsagePercent != 63 {
				t.Fatalf("expected sampled metrics, got %#v", metrics)
			}
			if len(metrics.Processes) != 1 || metrics.Processes[0].Name != "checkout" {
				t.Fatalf("expected process metrics, got %#v", metrics.Processes)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(metrics)
			markSeen("metrics")
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			if r.URL.Query().Get("agentId") != "agent-daemon-01" {
				t.Fatalf("expected profile task agent id query, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]profileTask{})
			markSeen("tasks")
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/targets":
			if r.URL.Query().Get("agentId") != "agent-daemon-01" {
				t.Fatalf("expected target poll agent id query, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":      "target-daemon-health-01",
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
			var report targetHealthCheckReport
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("expected target health report JSON, got %v", err)
			}
			if report.AgentID != "agent-daemon-01" || report.TargetID != "target-daemon-health-01" || report.Status != "healthy" {
				t.Fatalf("expected daemon target health report, got %#v", report)
			}
			if report.ExpectedStatus != http.StatusNoContent || report.ObservedStatus != http.StatusNoContent {
				t.Fatalf("expected daemon target health 204 report, got %#v", report)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(report)
			markSeen("targetHealth")
		default:
			t.Fatalf("unexpected daemon request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	cancel = stop
	defer cancel()

	err := RunDaemon(ctx, controlPlane.Client(), DaemonConfig{
		ControlPlaneURL:      controlPlane.URL,
		Token:                "ait_daemon_token",
		AgentID:              "agent-daemon-01",
		Name:                 "checkout-daemon",
		Hostname:             "checkout-host",
		IP:                   "10.0.0.88",
		Version:              "0.3.0",
		Labels:               map[string]string{"service": "checkout"},
		Capabilities:         []string{"host_metrics", "process_metrics", "pprof"},
		HeartbeatInterval:    time.Hour,
		MetricsInterval:      time.Hour,
		ProfileTaskInterval:  time.Hour,
		TargetHealthInterval: time.Hour,
		MetricsSampler: fakeMetricsSampler{metrics: AgentMetrics{
			CPUUsagePercent:    42.5,
			MemoryUsagePercent: 63,
			Processes: []AgentProcessMetric{
				{PID: 101, Name: "checkout", CPUUsagePercent: 17.5, MemoryRSSBytes: 268435456},
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected daemon to stop cleanly after context cancellation, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"heartbeat", "metrics", "tasks", "targetHealth"} {
		if !seen[name] {
			t.Fatalf("expected daemon to send %s request, seen=%#v", name, seen)
		}
	}
}

func TestRunDaemonRetriesAfterTransientStartupHeartbeatError(t *testing.T) {
	var cancel context.CancelFunc
	var mu sync.Mutex
	heartbeatAttempts := 0

	controlPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/heartbeat":
			mu.Lock()
			heartbeatAttempts++
			attempt := heartbeatAttempts
			mu.Unlock()
			if attempt == 1 {
				http.Error(w, "temporary heartbeat failure", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "agent-daemon-retry-01", "status": "online"})
			cancel()
		case r.Method == http.MethodPost && r.URL.Path == "/agent/v1/metrics":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"agentId": "agent-daemon-retry-01"})
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/profile-tasks":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]profileTask{})
		case r.Method == http.MethodGet && r.URL.Path == "/agent/v1/targets":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]targetHealthCheckTarget{})
		default:
			t.Fatalf("unexpected daemon request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(controlPlane.Close)

	ctx, stop := context.WithCancel(context.Background())
	cancel = stop
	defer cancel()

	err := RunDaemon(ctx, controlPlane.Client(), DaemonConfig{
		ControlPlaneURL:      controlPlane.URL,
		Token:                "ait_daemon_token",
		AgentID:              "agent-daemon-retry-01",
		Name:                 "checkout-daemon",
		Hostname:             "checkout-host",
		HeartbeatInterval:    time.Millisecond,
		MetricsInterval:      time.Hour,
		ProfileTaskInterval:  time.Hour,
		TargetHealthInterval: time.Hour,
		MetricsSampler:       fakeMetricsSampler{},
	})
	if err != nil {
		t.Fatalf("expected daemon to keep running after transient heartbeat error, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if heartbeatAttempts < 2 {
		t.Fatalf("expected heartbeat retry after startup failure, got %d attempts", heartbeatAttempts)
	}
}
