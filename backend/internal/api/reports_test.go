package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetRunReportAggregatesRunEventsAndProfileArtifacts(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "report-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 3,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)
	finished := waitForRunStatus(t, router, created.ID, "finished")
	waitForRunEventType(t, router, created.ID, "run_finished")

	artifactStore := newProfileArtifactStoreFromEnv()
	savedArtifact, err := artifactStore.save(profileArtifactRecord{
		ID:           "profile-report-1",
		RunID:        finished.ID,
		ScenarioName: finished.ScenarioName,
		TargetName:   finished.TargetName,
		ProfileType:  "cpu",
		Status:       "collected",
		SourceURL:    "http://127.0.0.1:6060/debug/pprof/profile?seconds=1",
		FileName:     finished.ID + "-cpu.pprof",
		ContentType:  "application/octet-stream",
		SizeBytes:    7,
		StartedAt:    "2026-06-16T12:30:00Z",
		FinishedAt:   "2026-06-16T12:30:01Z",
	}, []byte("profile"))
	if err != nil {
		t.Fatalf("failed to save profile artifact: %v", err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+finished.ID, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var report struct {
		Run              createRunResponse       `json:"run"`
		Events           []runEventRecord        `json:"events"`
		ProfileArtifacts []profileArtifactRecord `json:"profileArtifacts"`
		Summary          struct {
			SuccessRatePercent   float64 `json:"successRatePercent"`
			ErrorRatePercent     float64 `json:"errorRatePercent"`
			EventCount           int     `json:"eventCount"`
			ProfileArtifactCount int     `json:"profileArtifactCount"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if report.Run.ID != finished.ID {
		t.Fatalf("expected report run %q, got %#v", finished.ID, report.Run)
	}
	if report.Summary.SuccessRatePercent != 100 {
		t.Fatalf("expected 100%% success rate, got %#v", report.Summary)
	}
	if report.Summary.ErrorRatePercent != 0 {
		t.Fatalf("expected 0%% error rate, got %#v", report.Summary)
	}
	if report.Summary.EventCount != len(report.Events) || report.Summary.EventCount == 0 {
		t.Fatalf("expected event count summary to match events, got %#v and %d events", report.Summary, len(report.Events))
	}
	if report.Summary.ProfileArtifactCount != 1 {
		t.Fatalf("expected one profile artifact in summary, got %#v", report.Summary)
	}
	if len(report.ProfileArtifacts) != 1 || report.ProfileArtifacts[0].ID != savedArtifact.ID {
		t.Fatalf("expected saved profile artifact in report, got %#v", report.ProfileArtifacts)
	}
}

func TestGetRunReportIncludesSlowAndErrorSamples(t *testing.T) {
	setRunTestDatabase(t)

	var requestCount int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch atomic.AddInt64(&requestCount, 1) {
		case 1:
			http.Error(w, "temporary checkout failure", http.StatusServiceUnavailable)
		case 2:
			time.Sleep(25 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "report-samples",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 3,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)
	finished := waitForRunStatus(t, router, created.ID, "finished")
	waitForRunEventType(t, router, created.ID, "run_finished")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+finished.ID, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var report struct {
		SlowSamples []struct {
			Method    string  `json:"method"`
			URL       string  `json:"url"`
			Success   bool    `json:"success"`
			LatencyMs float64 `json:"latencyMs"`
		} `json:"slowSamples"`
		ErrorSamples []struct {
			Method     string  `json:"method"`
			URL        string  `json:"url"`
			StatusCode int     `json:"statusCode"`
			Success    bool    `json:"success"`
			LatencyMs  float64 `json:"latencyMs"`
			Error      string  `json:"error"`
		} `json:"errorSamples"`
		Summary struct {
			SlowSampleCount  int `json:"slowSampleCount"`
			ErrorSampleCount int `json:"errorSampleCount"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if len(report.SlowSamples) == 0 {
		t.Fatalf("expected slow request samples in report, got %#v", report)
	}
	if report.SlowSamples[0].Method != http.MethodGet || report.SlowSamples[0].URL != target.URL || report.SlowSamples[0].LatencyMs <= 0 {
		t.Fatalf("expected slow sample to include request identity and latency, got %#v", report.SlowSamples[0])
	}
	if len(report.ErrorSamples) != 1 {
		t.Fatalf("expected one error sample in report, got %#v", report.ErrorSamples)
	}
	if report.ErrorSamples[0].StatusCode != http.StatusServiceUnavailable || report.ErrorSamples[0].Success {
		t.Fatalf("expected HTTP 503 error sample, got %#v", report.ErrorSamples[0])
	}
	if !strings.Contains(report.ErrorSamples[0].Error, "HTTP 503") {
		t.Fatalf("expected error sample message to include status code, got %#v", report.ErrorSamples[0])
	}
	if report.Summary.SlowSampleCount != len(report.SlowSamples) || report.Summary.ErrorSampleCount != len(report.ErrorSamples) {
		t.Fatalf("expected sample counts in summary to match samples, got %#v", report.Summary)
	}
}

func TestGetRunReportIncludesGuardrailAlerts(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "report-guardrail-alert",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 20,
		"concurrency": 1,
		"timeoutMs": 1000,
		"maxErrorRatePercent": 10,
		"maxP95LatencyMs": 500
	}`)
	aborted := waitForRunStatus(t, router, created.ID, "aborted")
	waitForRunEventType(t, router, created.ID, "run_aborted")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+aborted.ID, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var report struct {
		Run struct {
			ID                  string  `json:"id"`
			MaxErrorRatePercent float64 `json:"maxErrorRatePercent"`
			MaxP95LatencyMs     float64 `json:"maxP95LatencyMs"`
		} `json:"run"`
		Alerts []struct {
			ID        string  `json:"id"`
			Severity  string  `json:"severity"`
			Kind      string  `json:"kind"`
			Metric    string  `json:"metric"`
			Threshold float64 `json:"threshold"`
			Observed  float64 `json:"observed"`
			Message   string  `json:"message"`
			EventID   string  `json:"eventId"`
			CreatedAt string  `json:"createdAt"`
		} `json:"alerts"`
		Summary struct {
			AlertCount int `json:"alertCount"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if report.Run.MaxErrorRatePercent != 10 || report.Run.MaxP95LatencyMs != 500 {
		t.Fatalf("expected guardrail thresholds to persist on report run, got %#v", report.Run)
	}
	if report.Summary.AlertCount != len(report.Alerts) || report.Summary.AlertCount != 1 {
		t.Fatalf("expected one alert counted in summary, got summary %#v and alerts %#v", report.Summary, report.Alerts)
	}
	alert := report.Alerts[0]
	if alert.Severity != "critical" || alert.Kind != "guardrail" || alert.Metric != "error_rate_percent" {
		t.Fatalf("expected critical error-rate guardrail alert, got %#v", alert)
	}
	if alert.Threshold != 10 || alert.Observed <= alert.Threshold {
		t.Fatalf("expected alert to include threshold and observed error rate, got %#v", alert)
	}
	if !strings.Contains(alert.Message, "Error rate guardrail exceeded") || alert.EventID == "" || alert.CreatedAt == "" {
		t.Fatalf("expected alert to explain the guardrail event, got %#v", alert)
	}
}

func TestGetRunReportIncludesTargetMetricsWindow(t *testing.T) {
	setRunTestDatabase(t)

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeat := httptest.NewRecorder()
	router.ServeHTTP(heartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-report-window-01",
		"name": "checkout-01",
		"hostname": "checkout-host-01",
		"capabilities": ["host_metrics", "process_metrics"]
	}`))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}

	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(`{
		"name": "checkout-target",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "prod",
		"agentIds": ["agent-report-window-01"],
		"processMatch": {
			"name": "checkout",
			"cmdlineContains": "--config=/etc/checkout/prod.yaml"
		}
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	for _, payload := range []string{
		`{
			"agentId": "agent-report-window-01",
			"collectedAt": "2026-06-14T10:00:00Z",
			"cpuUsagePercent": 99,
			"memoryUsagePercent": 99,
			"processes": []
		}`,
		`{
			"agentId": "agent-report-window-01",
			"collectedAt": "2026-06-14T10:01:30Z",
			"cpuUsagePercent": 72.5,
			"memoryUsagePercent": 61.25,
			"diskReadBytesPerSec": 2048,
			"diskWriteBytesPerSec": 1024,
			"networkRxBytesPerSec": 4096,
			"networkTxBytesPerSec": 8192,
			"processes": [
				{
					"pid": 1234,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 34.5,
					"memoryRssBytes": 268435456,
					"fdCount": 88,
					"threadCount": 12
				}
			]
		}`,
		`{
			"agentId": "agent-report-window-01",
			"collectedAt": "2026-06-14T10:02:00Z",
			"cpuUsagePercent": 51,
			"memoryUsagePercent": 65,
			"diskReadBytesPerSec": 8192,
			"diskWriteBytesPerSec": 4096,
			"networkRxBytesPerSec": 16384,
			"networkTxBytesPerSec": 32768,
			"processes": [
				{
					"pid": 2233,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/staging.yaml --port=8081",
					"cpuUsagePercent": 12.5,
					"memoryRssBytes": 134217728,
					"fdCount": 64,
					"threadCount": 8
				},
				{
					"pid": 1234,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 55,
					"memoryRssBytes": 300000000,
					"fdCount": 96,
					"threadCount": 14
				}
			]
		}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, payload))
		if response.Code != http.StatusAccepted {
			t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, response.Code, response.Body.String())
		}
	}

	run := createRunResponse{
		ID:               "run-report-metrics-window",
		TargetID:         target.ID,
		TargetName:       "checkout-target",
		Name:             "checkout-report-window",
		Status:           "finished",
		Method:           http.MethodGet,
		URL:              "http://checkout.internal:8080/health",
		TotalRequests:    10,
		SuccessRequests:  10,
		DurationMs:       120000,
		QPS:              50,
		AverageLatencyMs: 20,
		P95LatencyMs:     80,
		CreatedAt:        "2026-06-14T10:01:00Z",
	}
	if err := newRunHistoryStoreFromEnv().save(run); err != nil {
		t.Fatalf("failed to save run: %v", err)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+run.ID, nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, reportResponse.Code, reportResponse.Body.String())
	}
	var report struct {
		TargetMetrics struct {
			TargetID                string               `json:"targetId"`
			TargetName              string               `json:"targetName"`
			AgentIDs                []string             `json:"agentIds"`
			From                    string               `json:"from"`
			To                      string               `json:"to"`
			SampleCount             int                  `json:"sampleCount"`
			CPUMaxPercent           float64              `json:"cpuMaxPercent"`
			MemoryMaxPercent        float64              `json:"memoryMaxPercent"`
			DiskReadMaxBytesPerSec  float64              `json:"diskReadMaxBytesPerSec"`
			DiskWriteMaxBytesPerSec float64              `json:"diskWriteMaxBytesPerSec"`
			NetworkRxMaxBytesPerSec float64              `json:"networkRxMaxBytesPerSec"`
			NetworkTxMaxBytesPerSec float64              `json:"networkTxMaxBytesPerSec"`
			ProcessMatch            targetProcessMatch   `json:"processMatch"`
			Samples                 []agentMetricsRecord `json:"samples"`
			LatestProcessSnapshot   []agentProcessMetric `json:"latestProcessSnapshot"`
			ProcessTrends           []struct {
				AgentID           string  `json:"agentId"`
				PID               int     `json:"pid"`
				Name              string  `json:"name"`
				Cmdline           string  `json:"cmdline"`
				SampleCount       int     `json:"sampleCount"`
				CPUMaxPercent     float64 `json:"cpuMaxPercent"`
				MemoryRSSMaxBytes int64   `json:"memoryRssMaxBytes"`
				FDMaxCount        int     `json:"fdMaxCount"`
				ThreadMaxCount    int     `json:"threadMaxCount"`
				FirstSeenAt       string  `json:"firstSeenAt"`
				LastSeenAt        string  `json:"lastSeenAt"`
			} `json:"processTrends"`
		} `json:"targetMetrics"`
	}
	if err := json.NewDecoder(reportResponse.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if report.TargetMetrics.TargetID != target.ID || report.TargetMetrics.TargetName != "checkout-target" {
		t.Fatalf("expected report target metrics identity, got %#v", report.TargetMetrics)
	}
	if len(report.TargetMetrics.AgentIDs) != 1 || report.TargetMetrics.AgentIDs[0] != "agent-report-window-01" {
		t.Fatalf("expected target agent binding in report metrics, got %#v", report.TargetMetrics.AgentIDs)
	}
	if report.TargetMetrics.From != "2026-06-14T10:01:00Z" || report.TargetMetrics.To != "2026-06-14T10:03:00Z" {
		t.Fatalf("expected run time window in target metrics, got %#v", report.TargetMetrics)
	}
	if report.TargetMetrics.SampleCount != 2 {
		t.Fatalf("expected only windowed target metrics samples, got %#v", report.TargetMetrics)
	}
	if report.TargetMetrics.CPUMaxPercent != 72.5 || report.TargetMetrics.MemoryMaxPercent != 65 {
		t.Fatalf("expected max CPU/MEM from windowed samples, got %#v", report.TargetMetrics)
	}
	if report.TargetMetrics.DiskReadMaxBytesPerSec != 8192 || report.TargetMetrics.DiskWriteMaxBytesPerSec != 4096 {
		t.Fatalf("expected max disk throughput from windowed samples, got %#v", report.TargetMetrics)
	}
	if report.TargetMetrics.NetworkRxMaxBytesPerSec != 16384 || report.TargetMetrics.NetworkTxMaxBytesPerSec != 32768 {
		t.Fatalf("expected max network throughput from windowed samples, got %#v", report.TargetMetrics)
	}
	if report.TargetMetrics.ProcessMatch.Name != "checkout" {
		t.Fatalf("expected target process match in report metrics, got %#v", report.TargetMetrics.ProcessMatch)
	}
	if report.TargetMetrics.ProcessMatch.CmdlineContains != "--config=/etc/checkout/prod.yaml" {
		t.Fatalf("expected target cmdline process match in report metrics, got %#v", report.TargetMetrics.ProcessMatch)
	}
	if len(report.TargetMetrics.Samples) != 2 {
		t.Fatalf("expected windowed metric samples in report, got %#v", report.TargetMetrics.Samples)
	}
	if report.TargetMetrics.Samples[0].CollectedAt != "2026-06-14T10:01:30Z" || report.TargetMetrics.Samples[1].CollectedAt != "2026-06-14T10:02:00Z" {
		t.Fatalf("expected samples to preserve window order, got %#v", report.TargetMetrics.Samples)
	}
	if report.TargetMetrics.Samples[0].CPUUsagePercent != 72.5 || report.TargetMetrics.Samples[1].MemoryUsagePercent != 65 {
		t.Fatalf("expected samples to include CPU/MEM values, got %#v", report.TargetMetrics.Samples)
	}
	if len(report.TargetMetrics.LatestProcessSnapshot) != 1 || report.TargetMetrics.LatestProcessSnapshot[0].Name != "checkout-worker" {
		t.Fatalf("expected process snapshot to be filtered by target process match, got %#v", report.TargetMetrics.LatestProcessSnapshot)
	}
	if report.TargetMetrics.LatestProcessSnapshot[0].Cmdline != "checkout-worker --config=/etc/checkout/prod.yaml --port=8080" {
		t.Fatalf("expected process snapshot to be filtered by cmdline process match, got %#v", report.TargetMetrics.LatestProcessSnapshot[0])
	}
	if len(report.TargetMetrics.ProcessTrends) != 1 {
		t.Fatalf("expected one filtered process trend, got %#v", report.TargetMetrics.ProcessTrends)
	}
	trend := report.TargetMetrics.ProcessTrends[0]
	if trend.AgentID != "agent-report-window-01" || trend.PID != 1234 || trend.Name != "checkout-worker" {
		t.Fatalf("expected trend identity to track the same pid on the bound agent, got %#v", trend)
	}
	if trend.SampleCount != 2 || trend.CPUMaxPercent != 55 || trend.MemoryRSSMaxBytes != 300000000 || trend.FDMaxCount != 96 || trend.ThreadMaxCount != 14 {
		t.Fatalf("expected trend to aggregate process resource peaks, got %#v", trend)
	}
	if trend.FirstSeenAt != "2026-06-14T10:01:30Z" || trend.LastSeenAt != "2026-06-14T10:02:00Z" {
		t.Fatalf("expected trend to preserve first/last seen timestamps, got %#v", trend)
	}
}

func TestGetRunReportIncludesTargetHealthHistory(t *testing.T) {
	setRunTestDatabase(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)

	router := NewRouter()
	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(`{
		"name": "checkout-health-target",
		"baseUrl": "`+upstream.URL+`",
		"healthCheck": {
			"enabled": true,
			"path": "/ready",
			"expectedStatus": 204,
			"timeoutMs": 1000
		}
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	checkResponse := httptest.NewRecorder()
	router.ServeHTTP(checkResponse, httptest.NewRequest(http.MethodPost, "/api/targets/"+target.ID+"/health-check", nil))
	if checkResponse.Code != http.StatusOK {
		t.Fatalf("expected health check status %d, got %d with body %s", http.StatusOK, checkResponse.Code, checkResponse.Body.String())
	}

	run := createRunResponse{
		ID:              "run-report-target-health",
		TargetID:        target.ID,
		TargetName:      "checkout-health-target",
		Name:            "checkout-health-report",
		Status:          "finished",
		Method:          http.MethodGet,
		URL:             upstream.URL + "/checkout",
		TotalRequests:   10,
		SuccessRequests: 10,
		DurationMs:      1000,
		QPS:             10,
		CreatedAt:       "2026-06-14T10:01:00Z",
	}
	if err := newRunHistoryStoreFromEnv().save(run); err != nil {
		t.Fatalf("failed to save run: %v", err)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+run.ID, nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, reportResponse.Code, reportResponse.Body.String())
	}
	var report struct {
		TargetHealthChecks []targetHealthCheckResult `json:"targetHealthChecks"`
	}
	if err := json.NewDecoder(reportResponse.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if len(report.TargetHealthChecks) != 1 {
		t.Fatalf("expected one target health check in report, got %#v", report.TargetHealthChecks)
	}
	check := report.TargetHealthChecks[0]
	if check.TargetID != target.ID || check.TargetName != "checkout-health-target" {
		t.Fatalf("expected target identity in report health check, got %#v", check)
	}
	if check.Status != "unhealthy" || check.ObservedStatus != http.StatusServiceUnavailable || check.Error == "" {
		t.Fatalf("expected unhealthy target health history in report, got %#v", check)
	}
}

func TestGetRunReportIncludesTargetMetricThresholdAlerts(t *testing.T) {
	setRunTestDatabase(t)

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeat := httptest.NewRecorder()
	router.ServeHTTP(heartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-threshold-alert-01",
		"name": "checkout-threshold-01",
		"hostname": "checkout-threshold-host-01",
		"capabilities": ["host_metrics", "process_metrics"]
	}`))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}

	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(`{
		"name": "checkout-threshold-target",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "prod",
		"agentIds": ["agent-threshold-alert-01"],
		"metricThresholds": {
			"cpuMaxPercent": 70,
			"memoryMaxPercent": 60
		}
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	metrics := httptest.NewRecorder()
	router.ServeHTTP(metrics, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, `{
		"agentId": "agent-threshold-alert-01",
		"collectedAt": "2026-06-14T10:01:30Z",
		"cpuUsagePercent": 72.5,
		"memoryUsagePercent": 65,
		"diskReadBytesPerSec": 8192,
		"diskWriteBytesPerSec": 4096,
		"networkRxBytesPerSec": 16384,
		"networkTxBytesPerSec": 32768,
		"processes": []
	}`))
	if metrics.Code != http.StatusAccepted {
		t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, metrics.Code, metrics.Body.String())
	}

	run := createRunResponse{
		ID:               "run-target-metric-alerts",
		TargetID:         target.ID,
		TargetName:       "checkout-threshold-target",
		Name:             "checkout-threshold-report",
		Status:           "finished",
		Method:           http.MethodGet,
		URL:              "http://checkout.internal:8080/health",
		TotalRequests:    10,
		SuccessRequests:  10,
		DurationMs:       60000,
		QPS:              50,
		AverageLatencyMs: 20,
		P95LatencyMs:     80,
		CreatedAt:        "2026-06-14T10:01:00Z",
	}
	if err := newRunHistoryStoreFromEnv().save(run); err != nil {
		t.Fatalf("failed to save run: %v", err)
	}

	reportResponse := httptest.NewRecorder()
	router.ServeHTTP(reportResponse, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+run.ID, nil))
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("expected report status %d, got %d with body %s", http.StatusOK, reportResponse.Code, reportResponse.Body.String())
	}
	var report struct {
		Alerts []struct {
			ID        string  `json:"id"`
			Severity  string  `json:"severity"`
			Kind      string  `json:"kind"`
			Metric    string  `json:"metric"`
			Threshold float64 `json:"threshold"`
			Observed  float64 `json:"observed"`
			Message   string  `json:"message"`
		} `json:"alerts"`
		Summary struct {
			AlertCount int `json:"alertCount"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(reportResponse.Body).Decode(&report); err != nil {
		t.Fatalf("expected report JSON, got decode error: %v", err)
	}
	if report.Summary.AlertCount != 2 || len(report.Alerts) != 2 {
		t.Fatalf("expected two target metric alerts, got summary %#v and alerts %#v", report.Summary, report.Alerts)
	}
	alertsByMetric := map[string]struct {
		ID        string
		Severity  string
		Kind      string
		Threshold float64
		Observed  float64
		Message   string
	}{}
	for _, alert := range report.Alerts {
		alertsByMetric[alert.Metric] = struct {
			ID        string
			Severity  string
			Kind      string
			Threshold float64
			Observed  float64
			Message   string
		}{ID: alert.ID, Severity: alert.Severity, Kind: alert.Kind, Threshold: alert.Threshold, Observed: alert.Observed, Message: alert.Message}
	}
	cpuAlert, found := alertsByMetric["target_cpu_percent"]
	if !found || cpuAlert.Severity != "warning" || cpuAlert.Kind != "target_metric" || cpuAlert.Threshold != 70 || cpuAlert.Observed != 72.5 {
		t.Fatalf("expected CPU threshold target metric alert, got %#v", alertsByMetric)
	}
	if !strings.Contains(cpuAlert.Message, "Target CPU threshold exceeded") {
		t.Fatalf("expected CPU alert message to explain threshold, got %#v", cpuAlert)
	}
	memoryAlert, found := alertsByMetric["target_memory_percent"]
	if !found || memoryAlert.Severity != "warning" || memoryAlert.Kind != "target_metric" || memoryAlert.Threshold != 60 || memoryAlert.Observed != 65 {
		t.Fatalf("expected memory threshold target metric alert, got %#v", alertsByMetric)
	}
}

func TestGetProcessTrendComparisonReportComparesRecentRunProcessTrends(t *testing.T) {
	setRunTestDatabase(t)

	router := NewRouter()
	token := createAgentToken(t, router)
	heartbeat := httptest.NewRecorder()
	router.ServeHTTP(heartbeat, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/heartbeat", token, `{
		"id": "agent-process-compare-01",
		"name": "checkout-compare-01",
		"hostname": "checkout-host-compare-01",
		"capabilities": ["host_metrics", "process_metrics"]
	}`))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d with body %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}

	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(`{
		"name": "checkout-target",
		"baseUrl": "http://checkout.internal:8080",
		"environment": "prod",
		"agentIds": ["agent-process-compare-01"],
		"processMatch": {
			"name": "checkout",
			"cmdlineContains": "--config=/etc/checkout/prod.yaml"
		}
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	runStore := newRunHistoryStoreFromEnv()
	for _, run := range []createRunResponse{
		{
			ID:              "run-process-older",
			TargetID:        target.ID,
			TargetName:      "checkout-target",
			Name:            "checkout-process-baseline",
			Status:          "finished",
			Method:          http.MethodGet,
			URL:             "http://checkout.internal:8080/health",
			TotalRequests:   10,
			SuccessRequests: 10,
			DurationMs:      120000,
			QPS:             50,
			CreatedAt:       "2026-06-14T10:00:00Z",
		},
		{
			ID:              "run-process-latest",
			TargetID:        target.ID,
			TargetName:      "checkout-target",
			Name:            "checkout-process-after-release",
			Status:          "finished",
			Method:          http.MethodGet,
			URL:             "http://checkout.internal:8080/health",
			TotalRequests:   10,
			SuccessRequests: 10,
			DurationMs:      120000,
			QPS:             50,
			CreatedAt:       "2026-06-14T10:05:00Z",
		},
	} {
		if err := runStore.save(run); err != nil {
			t.Fatalf("failed to save run %s: %v", run.ID, err)
		}
	}

	for _, payload := range []string{
		`{
			"agentId": "agent-process-compare-01",
			"collectedAt": "2026-06-14T10:00:15Z",
			"cpuUsagePercent": 60,
			"memoryUsagePercent": 50,
			"processes": [
				{
					"pid": 1111,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 30,
					"memoryRssBytes": 180000000,
					"fdCount": 64,
					"threadCount": 9
				}
			]
		}`,
		`{
			"agentId": "agent-process-compare-01",
			"collectedAt": "2026-06-14T10:00:45Z",
			"cpuUsagePercent": 62,
			"memoryUsagePercent": 52,
			"processes": [
				{
					"pid": 1111,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 35,
					"memoryRssBytes": 200000000,
					"fdCount": 70,
					"threadCount": 10
				}
			]
		}`,
		`{
			"agentId": "agent-process-compare-01",
			"collectedAt": "2026-06-14T10:05:15Z",
			"cpuUsagePercent": 70,
			"memoryUsagePercent": 58,
			"processes": [
				{
					"pid": 2222,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 44,
					"memoryRssBytes": 250000000,
					"fdCount": 82,
					"threadCount": 12
				},
				{
					"pid": 3333,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/staging.yaml --port=8081",
					"cpuUsagePercent": 99,
					"memoryRssBytes": 800000000,
					"fdCount": 300,
					"threadCount": 40
				}
			]
		}`,
		`{
			"agentId": "agent-process-compare-01",
			"collectedAt": "2026-06-14T10:05:45Z",
			"cpuUsagePercent": 74,
			"memoryUsagePercent": 63,
			"processes": [
				{
					"pid": 2222,
					"name": "checkout-worker",
					"cmdline": "checkout-worker --config=/etc/checkout/prod.yaml --port=8080",
					"cpuUsagePercent": 55,
					"memoryRssBytes": 300000000,
					"fdCount": 96,
					"threadCount": 14
				}
			]
		}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/metrics", token, payload))
		if response.Code != http.StatusAccepted {
			t.Fatalf("expected metrics status %d, got %d with body %s", http.StatusAccepted, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/process-trends/compare?limit=2", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected process trend comparison status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}

	var report struct {
		Summary struct {
			RunCount                int    `json:"runCount"`
			ProcessCount            int    `json:"processCount"`
			HighestCPUProcessKey    string `json:"highestCpuProcessKey"`
			HighestMemoryProcessKey string `json:"highestMemoryProcessKey"`
		} `json:"summary"`
		Groups []struct {
			Key                       string  `json:"key"`
			TargetName                string  `json:"targetName"`
			AgentID                   string  `json:"agentId"`
			Name                      string  `json:"name"`
			Cmdline                   string  `json:"cmdline"`
			RunCount                  int     `json:"runCount"`
			SampleCount               int     `json:"sampleCount"`
			CPUMaxPercent             float64 `json:"cpuMaxPercent"`
			MemoryRSSMaxBytes         int64   `json:"memoryRssMaxBytes"`
			FDMaxCount                int     `json:"fdMaxCount"`
			ThreadMaxCount            int     `json:"threadMaxCount"`
			LatestRunID               string  `json:"latestRunId"`
			LatestPID                 int     `json:"latestPid"`
			LatestCPUMaxPercent       float64 `json:"latestCpuMaxPercent"`
			LatestMemoryRSSMaxBytes   int64   `json:"latestMemoryRssMaxBytes"`
			PreviousRunID             string  `json:"previousRunId"`
			PreviousPID               int     `json:"previousPid"`
			PreviousCPUMaxPercent     float64 `json:"previousCpuMaxPercent"`
			PreviousMemoryRSSMaxBytes int64   `json:"previousMemoryRssMaxBytes"`
			CPUDeltaPercent           float64 `json:"cpuDeltaPercent"`
			MemoryRSSDeltaBytes       int64   `json:"memoryRssDeltaBytes"`
			LatestLastSeenAt          string  `json:"latestLastSeenAt"`
			Runs                      []struct {
				RunID             string  `json:"runId"`
				RunName           string  `json:"runName"`
				PID               int     `json:"pid"`
				SampleCount       int     `json:"sampleCount"`
				CPUMaxPercent     float64 `json:"cpuMaxPercent"`
				MemoryRSSMaxBytes int64   `json:"memoryRssMaxBytes"`
			} `json:"runs"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected process trend comparison JSON, got decode error: %v", err)
	}
	if report.Summary.RunCount != 2 || report.Summary.ProcessCount != 1 {
		t.Fatalf("expected comparison summary for two recent runs and one process group, got %#v", report.Summary)
	}
	if len(report.Groups) != 1 {
		t.Fatalf("expected one matched process comparison group, got %#v", report.Groups)
	}
	group := report.Groups[0]
	if group.Key == "" || report.Summary.HighestCPUProcessKey != group.Key || report.Summary.HighestMemoryProcessKey != group.Key {
		t.Fatalf("expected summary keys to point at the process group, got summary %#v and group %#v", report.Summary, group)
	}
	if group.TargetName != "checkout-target" || group.AgentID != "agent-process-compare-01" || group.Name != "checkout-worker" {
		t.Fatalf("expected group identity for the matched target process, got %#v", group)
	}
	if group.Cmdline != "checkout-worker --config=/etc/checkout/prod.yaml --port=8080" {
		t.Fatalf("expected cmdline to be preserved for cross-run matching, got %#v", group)
	}
	if group.RunCount != 2 || group.SampleCount != 4 {
		t.Fatalf("expected process group to aggregate both runs and four samples, got %#v", group)
	}
	if group.CPUMaxPercent != 55 || group.MemoryRSSMaxBytes != 300000000 || group.FDMaxCount != 96 || group.ThreadMaxCount != 14 {
		t.Fatalf("expected group resource peaks across both runs, got %#v", group)
	}
	if group.LatestRunID != "run-process-latest" || group.LatestPID != 2222 || group.PreviousRunID != "run-process-older" || group.PreviousPID != 1111 {
		t.Fatalf("expected latest and previous run identities with changed PID, got %#v", group)
	}
	if group.LatestCPUMaxPercent != 55 || group.PreviousCPUMaxPercent != 35 || group.CPUDeltaPercent != 20 {
		t.Fatalf("expected latest CPU delta from previous run, got %#v", group)
	}
	if group.LatestMemoryRSSMaxBytes != 300000000 || group.PreviousMemoryRSSMaxBytes != 200000000 || group.MemoryRSSDeltaBytes != 100000000 {
		t.Fatalf("expected latest RSS delta from previous run, got %#v", group)
	}
	if group.LatestLastSeenAt != "2026-06-14T10:05:45Z" {
		t.Fatalf("expected latest last-seen timestamp, got %#v", group)
	}
	if len(group.Runs) != 2 || group.Runs[0].RunID != "run-process-latest" || group.Runs[1].RunID != "run-process-older" {
		t.Fatalf("expected group runs in newest-first order, got %#v", group.Runs)
	}
}

func TestGetRunComparisonReportComparesLatestRuns(t *testing.T) {
	setRunTestDatabase(t)

	store := newRunHistoryStoreFromEnv()
	runs := []createRunResponse{
		{
			ID:               "run-compare-oldest",
			Name:             "checkout-baseline",
			ScenarioName:     "checkout",
			TargetName:       "checkout-prod",
			Status:           "finished",
			Method:           http.MethodGet,
			URL:              "http://checkout.internal/api/orders",
			TotalRequests:    10,
			SuccessRequests:  10,
			FailedRequests:   0,
			DurationMs:       200,
			QPS:              50,
			AverageLatencyMs: 20,
			P95LatencyMs:     100,
			CreatedAt:        "2026-06-16T10:00:00Z",
		},
		{
			ID:               "run-compare-middle",
			Name:             "checkout-regression",
			ScenarioName:     "checkout",
			TargetName:       "checkout-prod",
			Status:           "finished",
			Method:           http.MethodGet,
			URL:              "http://checkout.internal/api/orders",
			TotalRequests:    10,
			SuccessRequests:  8,
			FailedRequests:   2,
			DurationMs:       180,
			QPS:              70,
			AverageLatencyMs: 24,
			P95LatencyMs:     140,
			CreatedAt:        "2026-06-16T10:05:00Z",
		},
		{
			ID:               "run-compare-newest",
			Name:             "checkout-optimized",
			ScenarioName:     "checkout",
			TargetName:       "checkout-prod",
			Status:           "finished",
			Method:           http.MethodGet,
			URL:              "http://checkout.internal/api/orders",
			TotalRequests:    20,
			SuccessRequests:  18,
			FailedRequests:   2,
			DurationMs:       160,
			QPS:              90,
			AverageLatencyMs: 18,
			P95LatencyMs:     80,
			CreatedAt:        "2026-06-16T10:10:00Z",
		},
	}
	for _, run := range runs {
		if err := store.save(run); err != nil {
			t.Fatalf("failed to save comparison run %s: %v", run.ID, err)
		}
	}

	router := NewRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/compare?limit=3", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected comparison report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	var report struct {
		Runs []struct {
			ID                 string  `json:"id"`
			Name               string  `json:"name"`
			SuccessRatePercent float64 `json:"successRatePercent"`
			ErrorRatePercent   float64 `json:"errorRatePercent"`
			QPS                float64 `json:"qps"`
			P95LatencyMs       float64 `json:"p95LatencyMs"`
		} `json:"runs"`
		Summary struct {
			RunCount              int    `json:"runCount"`
			BestQPSRunID          string `json:"bestQpsRunId"`
			FastestP95RunID       string `json:"fastestP95RunId"`
			HighestErrorRateRunID string `json:"highestErrorRateRunId"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected comparison report JSON, got decode error: %v", err)
	}
	if len(report.Runs) != 3 {
		t.Fatalf("expected three compared runs, got %#v", report.Runs)
	}
	if report.Runs[0].ID != "run-compare-newest" || report.Runs[1].ID != "run-compare-middle" || report.Runs[2].ID != "run-compare-oldest" {
		t.Fatalf("expected newest-first comparison ordering, got %#v", report.Runs)
	}
	if report.Runs[0].SuccessRatePercent != 90 || report.Runs[1].ErrorRatePercent != 20 {
		t.Fatalf("expected success/error rate calculations, got %#v", report.Runs)
	}
	if report.Summary.RunCount != 3 {
		t.Fatalf("expected summary run count 3, got %#v", report.Summary)
	}
	if report.Summary.BestQPSRunID != "run-compare-newest" {
		t.Fatalf("expected best QPS run to be newest optimized run, got %#v", report.Summary)
	}
	if report.Summary.FastestP95RunID != "run-compare-newest" {
		t.Fatalf("expected fastest p95 run to be newest optimized run, got %#v", report.Summary)
	}
	if report.Summary.HighestErrorRateRunID != "run-compare-middle" {
		t.Fatalf("expected highest error rate run to be regression run, got %#v", report.Summary)
	}
}

func TestGetProfileArtifactComparisonReportGroupsArtifactsForLatestRuns(t *testing.T) {
	setRunTestDatabase(t)

	runStore := newRunHistoryStoreFromEnv()
	runs := []createRunResponse{
		{
			ID:              "run-artifact-old",
			Name:            "checkout-old",
			Status:          "finished",
			Method:          http.MethodGet,
			URL:             "http://checkout.internal/health",
			TotalRequests:   10,
			SuccessRequests: 10,
			DurationMs:      100,
			CreatedAt:       "2026-06-16T09:00:00Z",
		},
		{
			ID:              "run-artifact-middle",
			Name:            "checkout-middle",
			Status:          "finished",
			Method:          http.MethodGet,
			URL:             "http://checkout.internal/health",
			TotalRequests:   10,
			SuccessRequests: 10,
			DurationMs:      100,
			CreatedAt:       "2026-06-16T10:00:00Z",
		},
		{
			ID:              "run-artifact-latest",
			Name:            "checkout-latest",
			Status:          "finished",
			Method:          http.MethodGet,
			URL:             "http://checkout.internal/health",
			TotalRequests:   10,
			SuccessRequests: 10,
			DurationMs:      100,
			CreatedAt:       "2026-06-16T11:00:00Z",
		},
	}
	for _, run := range runs {
		if err := runStore.save(run); err != nil {
			t.Fatalf("failed to save run %s: %v", run.ID, err)
		}
	}

	artifactStore := newProfileArtifactStoreFromEnv()
	for _, artifact := range []profileArtifactRecord{
		{
			ID:           "profile-old-cpu",
			RunID:        "run-artifact-old",
			ScenarioName: "checkout",
			TargetName:   "checkout-target",
			ProfileType:  "cpu",
			Status:       "collected",
			FileName:     "old-cpu.pprof",
			SizeBytes:    128,
			StartedAt:    "2026-06-16T09:00:00Z",
			FinishedAt:   "2026-06-16T09:00:01Z",
		},
		{
			ID:           "profile-middle-cpu",
			RunID:        "run-artifact-middle",
			ScenarioName: "checkout",
			TargetName:   "checkout-target",
			ProfileType:  "cpu",
			Status:       "collected",
			FileName:     "middle-cpu.pprof",
			SizeBytes:    16,
			StartedAt:    "2026-06-16T10:00:00Z",
			FinishedAt:   "2026-06-16T10:00:01Z",
		},
		{
			ID:           "profile-latest-cpu",
			RunID:        "run-artifact-latest",
			ScenarioName: "checkout",
			TargetName:   "checkout-target",
			ProfileType:  "cpu",
			Status:       "collected",
			FileName:     "latest-cpu.pprof",
			SizeBytes:    32,
			StartedAt:    "2026-06-16T11:00:00Z",
			FinishedAt:   "2026-06-16T11:00:01Z",
		},
		{
			ID:           "profile-middle-heap",
			RunID:        "run-artifact-middle",
			ScenarioName: "checkout",
			TargetName:   "checkout-target",
			ProfileType:  "heap",
			Status:       "collected",
			FileName:     "middle-heap.pprof",
			SizeBytes:    8,
			StartedAt:    "2026-06-16T10:00:00Z",
			FinishedAt:   "2026-06-16T10:00:01Z",
		},
		{
			ID:           "profile-latest-heap",
			RunID:        "run-artifact-latest",
			ScenarioName: "checkout",
			TargetName:   "checkout-target",
			ProfileType:  "heap",
			Status:       "failed",
			FileName:     "latest-heap.pprof",
			SizeBytes:    0,
			Error:        "heap endpoint unavailable",
			StartedAt:    "2026-06-16T11:00:00Z",
			FinishedAt:   "2026-06-16T11:00:01Z",
		},
	} {
		if _, err := artifactStore.save(artifact, nil); err != nil {
			t.Fatalf("failed to save artifact %s: %v", artifact.ID, err)
		}
	}

	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/profile-artifacts/compare?limit=2", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected comparison status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}

	var report struct {
		Summary struct {
			RunCount       int   `json:"runCount"`
			ArtifactCount  int   `json:"artifactCount"`
			CollectedCount int   `json:"collectedCount"`
			FailedCount    int   `json:"failedCount"`
			TotalSizeBytes int64 `json:"totalSizeBytes"`
		} `json:"summary"`
		Groups []struct {
			ProfileType       string                  `json:"profileType"`
			ArtifactCount     int                     `json:"artifactCount"`
			CollectedCount    int                     `json:"collectedCount"`
			FailedCount       int                     `json:"failedCount"`
			TotalSizeBytes    int64                   `json:"totalSizeBytes"`
			LatestRunID       string                  `json:"latestRunId"`
			LatestArtifactID  string                  `json:"latestArtifactId"`
			LatestSizeBytes   int64                   `json:"latestSizeBytes"`
			PreviousSizeBytes int64                   `json:"previousSizeBytes"`
			SizeDeltaBytes    int64                   `json:"sizeDeltaBytes"`
			SizeDeltaPercent  float64                 `json:"sizeDeltaPercent"`
			LatestStatus      string                  `json:"latestStatus"`
			PreviousStatus    string                  `json:"previousStatus"`
			StatusChanged     bool                    `json:"statusChanged"`
			Artifacts         []profileArtifactRecord `json:"artifacts"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("expected profile artifact comparison JSON, got decode error: %v", err)
	}
	if report.Summary.RunCount != 2 || report.Summary.ArtifactCount != 4 || report.Summary.CollectedCount != 3 || report.Summary.FailedCount != 1 {
		t.Fatalf("expected comparison summary for latest two runs, got %#v", report.Summary)
	}
	if report.Summary.TotalSizeBytes != 56 {
		t.Fatalf("expected total size to ignore excluded old run artifact, got %#v", report.Summary)
	}
	if len(report.Groups) != 2 {
		t.Fatalf("expected cpu and heap groups, got %#v", report.Groups)
	}
	if report.Groups[0].ProfileType != "cpu" || report.Groups[0].ArtifactCount != 2 || report.Groups[0].TotalSizeBytes != 48 {
		t.Fatalf("expected cpu group to include latest and middle artifacts, got %#v", report.Groups[0])
	}
	if report.Groups[0].LatestRunID != "run-artifact-latest" || report.Groups[0].LatestArtifactID != "profile-latest-cpu" {
		t.Fatalf("expected cpu group latest artifact identity, got %#v", report.Groups[0])
	}
	if report.Groups[0].LatestSizeBytes != 32 || report.Groups[0].PreviousSizeBytes != 16 || report.Groups[0].SizeDeltaBytes != 16 || report.Groups[0].SizeDeltaPercent != 100 {
		t.Fatalf("expected cpu group size delta from latest and previous artifacts, got %#v", report.Groups[0])
	}
	if report.Groups[0].LatestStatus != "collected" || report.Groups[0].PreviousStatus != "collected" || report.Groups[0].StatusChanged {
		t.Fatalf("expected cpu group statuses to remain collected, got %#v", report.Groups[0])
	}
	if len(report.Groups[0].Artifacts) != 2 || report.Groups[0].Artifacts[0].ID != "profile-latest-cpu" || report.Groups[0].Artifacts[1].ID != "profile-middle-cpu" {
		t.Fatalf("expected cpu artifacts in latest-first order, got %#v", report.Groups[0].Artifacts)
	}
	if report.Groups[1].ProfileType != "heap" || report.Groups[1].FailedCount != 1 || report.Groups[1].LatestArtifactID != "profile-latest-heap" {
		t.Fatalf("expected heap failed group, got %#v", report.Groups[1])
	}
	if report.Groups[1].LatestSizeBytes != 0 || report.Groups[1].PreviousSizeBytes != 8 || report.Groups[1].SizeDeltaBytes != -8 || report.Groups[1].SizeDeltaPercent != -100 {
		t.Fatalf("expected heap group to show failed latest size regression, got %#v", report.Groups[1])
	}
	if report.Groups[1].LatestStatus != "failed" || report.Groups[1].PreviousStatus != "collected" || !report.Groups[1].StatusChanged {
		t.Fatalf("expected heap group to show status changed from collected to failed, got %#v", report.Groups[1])
	}
}

func TestGetRunReportSupportsJSONPCallback(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "report-jsonp-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 2,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)
	finished := waitForRunStatus(t, router, created.ID, "finished")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/runs/"+finished.ID+"?callback=window.__aitReport", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected JSONP report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/javascript" {
		t.Fatalf("expected JavaScript content type, got %q", contentType)
	}
	body := response.Body.String()
	if !strings.HasPrefix(body, "window.__aitReport(") || !strings.HasSuffix(body, ");\n") {
		t.Fatalf("expected JSONP callback wrapper, got %s", body)
	}
	if !strings.Contains(body, `"successRatePercent":100`) {
		t.Fatalf("expected report payload inside JSONP wrapper, got %s", body)
	}
}

func TestGetLatestRunReportSupportsJSONPCallback(t *testing.T) {
	setRunTestDatabase(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	router := NewRouter()
	created := createAsyncRun(t, router, `{
		"name": "latest-report-jsonp-smoke",
		"method": "GET",
		"url": "`+target.URL+`",
		"totalRequests": 2,
		"concurrency": 1,
		"timeoutMs": 1000
	}`)
	finished := waitForRunStatus(t, router, created.ID, "finished")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reports/latest?callback=window.__aitLatestReport", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected latest JSONP report status %d, got %d with body %s", http.StatusOK, response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.HasPrefix(body, "window.__aitLatestReport(") || !strings.Contains(body, `"id":"`+finished.ID+`"`) {
		t.Fatalf("expected latest report JSONP wrapper for run %q, got %s", finished.ID, body)
	}
}
