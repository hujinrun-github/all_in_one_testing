package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TargetHealthCheckConfig struct {
	ControlPlaneURL string
	Token           string
	AgentID         string
}

type TargetHealthCheckProcessResult struct {
	Processed int `json:"processed"`
}

type targetHealthCheckTarget struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	BaseURL     string                    `json:"baseUrl"`
	HealthCheck targetHealthCheckSettings `json:"healthCheck"`
}

type targetHealthCheckSettings struct {
	Enabled        bool   `json:"enabled"`
	Path           string `json:"path"`
	ExpectedStatus int    `json:"expectedStatus"`
	TimeoutMs      int    `json:"timeoutMs"`
}

type targetHealthCheckReport struct {
	AgentID        string  `json:"agentId"`
	TargetID       string  `json:"targetId"`
	Status         string  `json:"status"`
	URL            string  `json:"url"`
	ExpectedStatus int     `json:"expectedStatus"`
	ObservedStatus int     `json:"observedStatus"`
	LatencyMs      float64 `json:"latencyMs"`
	Error          string  `json:"error,omitempty"`
	CheckedAt      string  `json:"checkedAt"`
}

func ProcessTargetHealthChecksOnce(ctx context.Context, client *http.Client, config TargetHealthCheckConfig) (TargetHealthCheckProcessResult, error) {
	config = normalizeTargetHealthCheckConfig(config)
	if err := validateTargetHealthCheckConfig(config); err != nil {
		return TargetHealthCheckProcessResult{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	targets, err := pollBoundTargets(ctx, client, config)
	if err != nil {
		return TargetHealthCheckProcessResult{}, err
	}

	result := TargetHealthCheckProcessResult{}
	for _, target := range targets {
		if !target.HealthCheck.Enabled {
			continue
		}
		report := checkBoundTargetHealth(ctx, client, config.AgentID, target)
		if err := postTargetHealthCheckReport(ctx, client, config, report); err != nil {
			return result, err
		}
		result.Processed++
	}
	return result, nil
}

func normalizeTargetHealthCheckConfig(config TargetHealthCheckConfig) TargetHealthCheckConfig {
	config.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(config.ControlPlaneURL), "/")
	config.Token = strings.TrimSpace(config.Token)
	config.AgentID = strings.TrimSpace(config.AgentID)
	return config
}

func validateTargetHealthCheckConfig(config TargetHealthCheckConfig) error {
	if config.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if config.Token == "" {
		return fmt.Errorf("agent token is required")
	}
	if config.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	return nil
}

func pollBoundTargets(ctx context.Context, client *http.Client, config TargetHealthCheckConfig) ([]targetHealthCheckTarget, error) {
	pollURL := config.ControlPlaneURL + "/agent/v1/targets?agentId=" + url.QueryEscape(config.AgentID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("target poll returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var targets []targetHealthCheckTarget
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func checkBoundTargetHealth(ctx context.Context, client *http.Client, agentID string, target targetHealthCheckTarget) targetHealthCheckReport {
	checkedAt := time.Now().UTC()
	expectedStatus := target.HealthCheck.ExpectedStatus
	if expectedStatus <= 0 {
		expectedStatus = http.StatusOK
	}
	report := targetHealthCheckReport{
		AgentID:        agentID,
		TargetID:       strings.TrimSpace(target.ID),
		Status:         "unhealthy",
		ExpectedStatus: expectedStatus,
		CheckedAt:      checkedAt.Format(time.RFC3339Nano),
	}

	healthURL, err := buildTargetHealthCheckURL(target)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.URL = healthURL

	timeout := time.Duration(target.HealthCheck.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	startedAt := time.Now()
	response, err := client.Do(request)
	report.LatencyMs = float64(time.Since(startedAt).Microseconds()) / 1000
	if err != nil {
		report.Error = err.Error()
		return report
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	report.ObservedStatus = response.StatusCode
	if response.StatusCode == expectedStatus {
		report.Status = "healthy"
		return report
	}
	report.Error = fmt.Sprintf("expected HTTP %d, got HTTP %d", expectedStatus, response.StatusCode)
	return report
}

func buildTargetHealthCheckURL(target targetHealthCheckTarget) (string, error) {
	path := strings.TrimSpace(target.HealthCheck.Path)
	if path == "" {
		return "", fmt.Errorf("health check path is required")
	}
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		return parsed.String(), nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("target baseUrl is required")
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return "", fmt.Errorf("target baseUrl must be an absolute URL")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path, nil
}

func postTargetHealthCheckReport(ctx context.Context, client *http.Client, config TargetHealthCheckConfig, report targetHealthCheckReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.ControlPlaneURL+"/agent/v1/target-health-checks", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("target health report returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
