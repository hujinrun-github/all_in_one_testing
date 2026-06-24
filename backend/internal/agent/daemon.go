package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultHeartbeatInterval    = 10 * time.Second
	defaultMetricsInterval      = 5 * time.Second
	defaultProfileTaskInterval  = 5 * time.Second
	defaultTargetHealthInterval = 10 * time.Second
)

type AgentHeartbeat struct {
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name"`
	Hostname     string            `json:"hostname"`
	IP           string            `json:"ip,omitempty"`
	Version      string            `json:"version,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}

type AgentMetrics struct {
	AgentID              string               `json:"agentId"`
	CollectedAt          string               `json:"collectedAt,omitempty"`
	CPUUsagePercent      float64              `json:"cpuUsagePercent"`
	MemoryUsagePercent   float64              `json:"memoryUsagePercent"`
	DiskReadBytesPerSec  float64              `json:"diskReadBytesPerSec"`
	DiskWriteBytesPerSec float64              `json:"diskWriteBytesPerSec"`
	NetworkRxBytesPerSec float64              `json:"networkRxBytesPerSec"`
	NetworkTxBytesPerSec float64              `json:"networkTxBytesPerSec"`
	Processes            []AgentProcessMetric `json:"processes"`
}

type AgentProcessMetric struct {
	PID             int     `json:"pid"`
	Name            string  `json:"name"`
	Cmdline         string  `json:"cmdline,omitempty"`
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	MemoryRSSBytes  int64   `json:"memoryRssBytes"`
	FDCount         int     `json:"fdCount"`
	ThreadCount     int     `json:"threadCount"`
}

type MetricsSampler interface {
	SampleMetrics(ctx context.Context, agentID string) (AgentMetrics, error)
}

type DaemonConfig struct {
	ControlPlaneURL      string
	Token                string
	AgentID              string
	Name                 string
	Hostname             string
	IP                   string
	Version              string
	Labels               map[string]string
	Capabilities         []string
	HeartbeatInterval    time.Duration
	MetricsInterval      time.Duration
	ProfileTaskInterval  time.Duration
	TargetHealthInterval time.Duration
	MetricsSampler       MetricsSampler
}

func RunDaemon(ctx context.Context, client *http.Client, config DaemonConfig) error {
	config = normalizeDaemonConfig(config)
	if err := validateDaemonConfig(config); err != nil {
		return err
	}
	if client == nil {
		client = http.DefaultClient
	}
	if config.MetricsSampler == nil {
		config.MetricsSampler = &RuntimeMetricsSampler{}
	}

	_ = runDaemonStartup(ctx, client, config)
	if ctx.Err() != nil {
		return nil
	}

	heartbeatTicker := time.NewTicker(config.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	metricsTicker := time.NewTicker(config.MetricsInterval)
	defer metricsTicker.Stop()
	taskTicker := time.NewTicker(config.ProfileTaskInterval)
	defer taskTicker.Stop()
	targetHealthTicker := time.NewTicker(config.TargetHealthInterval)
	defer targetHealthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			_ = sendHeartbeat(ctx, client, config)
		case <-metricsTicker.C:
			_ = sendMetrics(ctx, client, config)
		case <-taskTicker.C:
			_, _ = ProcessProfileTasksOnce(ctx, client, ProfileTaskPollConfig{
				ControlPlaneURL: config.ControlPlaneURL,
				Token:           config.Token,
				AgentID:         config.AgentID,
			})
		case <-targetHealthTicker.C:
			_, _ = ProcessTargetHealthChecksOnce(ctx, client, TargetHealthCheckConfig{
				ControlPlaneURL: config.ControlPlaneURL,
				Token:           config.Token,
				AgentID:         config.AgentID,
			})
		}
	}
}

func runDaemonStartup(ctx context.Context, client *http.Client, config DaemonConfig) error {
	var firstErr error
	if err := sendHeartbeat(ctx, client, config); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := sendMetrics(ctx, client, config); err != nil && firstErr == nil {
		firstErr = err
	}
	if _, err := ProcessProfileTasksOnce(ctx, client, ProfileTaskPollConfig{
		ControlPlaneURL: config.ControlPlaneURL,
		Token:           config.Token,
		AgentID:         config.AgentID,
	}); err != nil && firstErr == nil {
		firstErr = err
	}
	if _, err := ProcessTargetHealthChecksOnce(ctx, client, TargetHealthCheckConfig{
		ControlPlaneURL: config.ControlPlaneURL,
		Token:           config.Token,
		AgentID:         config.AgentID,
	}); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func sendHeartbeat(ctx context.Context, client *http.Client, config DaemonConfig) error {
	heartbeat := AgentHeartbeat{
		ID:           config.AgentID,
		Name:         config.Name,
		Hostname:     config.Hostname,
		IP:           config.IP,
		Version:      config.Version,
		Labels:       config.Labels,
		Capabilities: config.Capabilities,
	}
	return postAgentJSON(ctx, client, config, "/agent/v1/heartbeat", heartbeat)
}

func sendMetrics(ctx context.Context, client *http.Client, config DaemonConfig) error {
	metrics, err := config.MetricsSampler.SampleMetrics(ctx, config.AgentID)
	if err != nil {
		return err
	}
	metrics.AgentID = config.AgentID
	if strings.TrimSpace(metrics.CollectedAt) == "" {
		metrics.CollectedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if metrics.Processes == nil {
		metrics.Processes = []AgentProcessMetric{}
	}
	return postAgentJSON(ctx, client, config, "/agent/v1/metrics", metrics)
}

func postAgentJSON(ctx context.Context, client *http.Client, config DaemonConfig, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.ControlPlaneURL+path, bytes.NewReader(body))
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
		return fmt.Errorf("agent request %s returned HTTP %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func normalizeDaemonConfig(config DaemonConfig) DaemonConfig {
	config.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(config.ControlPlaneURL), "/")
	config.Token = strings.TrimSpace(config.Token)
	config.AgentID = strings.TrimSpace(config.AgentID)
	config.Name = strings.TrimSpace(config.Name)
	config.Hostname = strings.TrimSpace(config.Hostname)
	config.IP = strings.TrimSpace(config.IP)
	config.Version = strings.TrimSpace(config.Version)
	if config.Hostname == "" {
		if hostname, err := os.Hostname(); err == nil {
			config.Hostname = hostname
		}
	}
	if config.Name == "" {
		config.Name = config.Hostname
	}
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Capabilities = normalizeCapabilities(config.Capabilities)
	if len(config.Capabilities) == 0 {
		config.Capabilities = []string{"host_metrics", "process_metrics", "pprof"}
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeatInterval
	}
	if config.MetricsInterval <= 0 {
		config.MetricsInterval = defaultMetricsInterval
	}
	if config.ProfileTaskInterval <= 0 {
		config.ProfileTaskInterval = defaultProfileTaskInterval
	}
	if config.TargetHealthInterval <= 0 {
		config.TargetHealthInterval = defaultTargetHealthInterval
	}
	return config
}

func normalizeCapabilities(values []string) []string {
	capabilities := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			capabilities = append(capabilities, value)
		}
	}
	return capabilities
}

func validateDaemonConfig(config DaemonConfig) error {
	if config.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if config.Token == "" {
		return fmt.Errorf("agent token is required")
	}
	if config.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if config.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if config.Hostname == "" {
		return fmt.Errorf("agent hostname is required")
	}
	return nil
}
