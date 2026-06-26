package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"all_in_one_testing/backend/internal/rpc"

	"github.com/go-chi/chi/v5"
)

const (
	defaultRunTimeoutMs = 3000
	maxRunRequests      = 10000
	maxRunConcurrency   = 256
	maxProfileBytes     = 64 * 1024 * 1024
	maxRunSamples       = 5
	maxRunResponseBytes = 1024 * 1024
)

const (
	runProtocolHTTP      = "HTTP"
	runProtocolCustomRPC = "CUSTOM_RPC"
)

// supportedProtocols 支持的协议列表
var supportedProtocols = map[string]bool{
	"HTTP":       true,
	"CUSTOM_RPC": true,
	"gRPC":       true, // 新增：gRPC 插件
	"Dubbo":      true, // 新增：Dubbo 插件（暂未实现）
	"Thrift":     true, // 新增：Thrift 插件（暂未实现）
}

type createRunRequest struct {
	ScenarioID          string             `json:"scenarioId,omitempty"`
	ScenarioName        string             `json:"-"`
	TargetID            string             `json:"targetId,omitempty"`
	TargetName          string             `json:"-"`
	TargetProfile       string             `json:"-"`
	TargetAgentIDs      []string           `json:"-"`
	RunID               string             `json:"-"`
	CreatedAt           string             `json:"-"`
	ProjectID           string             `json:"projectId,omitempty"`
	Environment         string             `json:"environment,omitempty"`
	Name                string             `json:"name"`
	Protocol            string             `json:"protocol,omitempty"`
	Method              string             `json:"method"`
	URL                 string             `json:"url"`
	TotalRequests       int                `json:"totalRequests"`
	Concurrency         int                `json:"concurrency"`
	TimeoutMs           int                `json:"timeoutMs"`
	MaxErrorRatePercent float64            `json:"maxErrorRatePercent,omitempty"`
	MaxP95LatencyMs     float64            `json:"maxP95LatencyMs,omitempty"`
	Headers             []runHeader        `json:"headers,omitempty"`
	BodyVariants        []bodyVariant      `json:"bodyVariants,omitempty"`
	QueryVariants       []queryVariant     `json:"queryVariants,omitempty"`
	FlowSteps           []scenarioFlowStep `json:"flowSteps,omitempty"`
	Assertion           string             `json:"assertion,omitempty"`
}

type bodyVariant struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Body   string `json:"body"`
}

type queryVariant struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	QueryParams string `json:"queryParams"`
}

type runHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type createRunResponse struct {
	ID                  string                   `json:"id"`
	ScenarioID          string                   `json:"scenarioId,omitempty"`
	ScenarioName        string                   `json:"scenarioName,omitempty"`
	TargetID            string                   `json:"targetId,omitempty"`
	TargetName          string                   `json:"targetName,omitempty"`
	ProjectID           string                   `json:"projectId,omitempty"`
	Environment         string                   `json:"environment,omitempty"`
	Name                string                   `json:"name"`
	Status              string                   `json:"status"`
	Protocol            string                   `json:"protocol"`
	Method              string                   `json:"method"`
	URL                 string                   `json:"url"`
	TotalRequests       int                      `json:"totalRequests"`
	SuccessRequests     int                      `json:"successRequests"`
	FailedRequests      int                      `json:"failedRequests"`
	MaxErrorRatePercent float64                  `json:"maxErrorRatePercent,omitempty"`
	MaxP95LatencyMs     float64                  `json:"maxP95LatencyMs,omitempty"`
	DurationMs          float64                  `json:"durationMs"`
	QPS                 float64                  `json:"qps"`
	AverageLatencyMs    float64                  `json:"averageLatencyMs"`
	P95LatencyMs        float64                  `json:"p95LatencyMs"`
	CreatedAt           string                   `json:"createdAt"`
	ProfileArtifacts    []profileArtifactRecord  `json:"profileArtifacts,omitempty"`
	SlowSamples         []runRequestSampleRecord `json:"-"`
	ErrorSamples        []runRequestSampleRecord `json:"-"`
}

type runSample struct {
	latency    time.Duration
	method     string
	url        string
	statusCode int
	success    bool
	error      string
	body       []byte
}

type httpRequestOutcome struct {
	method     string
	url        string
	statusCode int
	headers    http.Header
	body       []byte
	success    bool
	err        error
}

type runProgress struct {
	input          createRunRequest
	startedAt      time.Time
	latencies      []float64
	success        int
	failed         int
	totalLatencyMs float64
	slowSamples    []runRequestSampleRecord
	errorSamples   []runRequestSampleRecord
}

type runManager struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newRunManager() *runManager {
	return &runManager{cancels: map[string]context.CancelFunc{}}
}

func (manager *runManager) start(input createRunRequest, runStore *runHistoryStore, profileStore *profileArtifactStore, targetStore *targetStore, agentStore *agentStore, profileTaskStore *profileTaskStore) createRunResponse {
	runCtx, cancel := context.WithCancel(context.Background())
	manager.mu.Lock()
	manager.cancels[input.RunID] = cancel
	manager.mu.Unlock()

	go func() {
		defer manager.forget(input.RunID)
		profileArtifacts := startRunProfileCapture(runCtx, profileStore, input)
		result := executeHTTPRunWithProgress(runCtx, input, func(progress createRunResponse) {
			_ = runStore.save(progress)
			_ = runStore.appendEvent(runEventFromResponse("run_progress", "Run progress updated", progress))
		})
		result.ProfileArtifacts = <-profileArtifacts
		if runCtx.Err() != nil {
			result.Status = "canceled"
		}
		_ = runStore.save(result)
		_ = runStore.replaceRequestSamples(result.ID, result.SlowSamples, result.ErrorSamples)
		_ = runStore.appendEvent(runEventFromResponse(runTerminalEventType(result.Status), runTerminalEventMessage(result.Status), result))
		if err := scheduleThresholdProfileTasks(targetStore, agentStore, profileTaskStore, result); err != nil {
			_ = runStore.appendEvent(runEventFromResponse("profile_task_schedule_failed", "Profile task threshold scheduling failed: "+err.Error(), result))
		}
	}()

	return pendingRunResponse(input, "running")
}

func (manager *runManager) stop(runID string) bool {
	manager.mu.Lock()
	cancel, found := manager.cancels[runID]
	manager.mu.Unlock()
	if !found {
		return false
	}
	cancel()
	return true
}

func (manager *runManager) forget(runID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	delete(manager.cancels, runID)
}

func pendingRunResponse(input createRunRequest, status string) createRunResponse {
	return createRunResponse{
		ID:                  input.RunID,
		ScenarioID:          input.ScenarioID,
		ScenarioName:        input.ScenarioName,
		TargetID:            input.TargetID,
		TargetName:          input.TargetName,
		ProjectID:           input.ProjectID,
		Environment:         input.Environment,
		Name:                input.Name,
		Status:              status,
		Protocol:            normalizedRunProtocol(input.Protocol),
		Method:              input.Method,
		URL:                 input.URL,
		TotalRequests:       input.TotalRequests,
		MaxErrorRatePercent: input.MaxErrorRatePercent,
		MaxP95LatencyMs:     input.MaxP95LatencyMs,
		CreatedAt:           input.CreatedAt,
	}
}

func handleCreateRun(scenarioStore *scenarioStore, targetStore *targetStore, agentStore *agentStore, runStore *runHistoryStore, profileStore *profileArtifactStore, profileTaskStore *profileTaskStore, manager *runManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createRunRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}

		normalized, status, err := resolveCreateRunRequest(scenarioStore, targetStore, input)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if err := applyWorkspaceScopeToRecord(r, &normalized.ProjectID, &normalized.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if status, err := preflightTargetHealth(r.Context(), targetStore, normalized); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		normalized.RunID = newRunID()
		normalized.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result := pendingRunResponse(normalized, "running")
		if err := runStore.save(result); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := runStore.appendEvent(runEventFromResponse("run_started", "Run started", result)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := scheduleRunProfileTasks(profileTaskStore, normalized); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result = manager.start(normalized, runStore, profileStore, targetStore, agentStore, profileTaskStore)
		writeJSON(w, http.StatusAccepted, result)
	}
}

func scheduleRunProfileTasks(store *profileTaskStore, input createRunRequest) error {
	if store == nil || strings.TrimSpace(input.TargetProfile) == "" || strings.TrimSpace(input.RunID) == "" {
		return nil
	}
	for _, agentID := range input.TargetAgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		_, err := store.create(profileTaskRecord{
			AgentID:        agentID,
			RunID:          input.RunID,
			ScenarioID:     input.ScenarioID,
			ScenarioName:   input.ScenarioName,
			TargetID:       input.TargetID,
			TargetName:     input.TargetName,
			PprofBaseURL:   input.TargetProfile,
			ProfileType:    "cpu",
			ProfileSeconds: 1,
			Source:         runAutoProfileTaskSource,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func scheduleThresholdProfileTasks(targetStore *targetStore, agentStore *agentStore, taskStore *profileTaskStore, run createRunResponse) error {
	if targetStore == nil || agentStore == nil || taskStore == nil {
		return nil
	}
	if run.Status == "canceled" || strings.TrimSpace(run.TargetID) == "" || strings.TrimSpace(run.ID) == "" {
		return nil
	}
	target, found, err := targetStore.get(run.TargetID)
	if err != nil || !found {
		return err
	}
	if strings.TrimSpace(target.ProfileEndpoint) == "" || len(target.AgentIDs) == 0 {
		return nil
	}

	targetMetrics, err := loadRunTargetMetricsReport(targetStore, agentStore, run)
	if err != nil {
		return err
	}
	if len(buildTargetMetricThresholdAlerts(run, targetMetrics)) == 0 {
		return nil
	}

	breachedAgentIDs := thresholdBreachedAgentIDs(targetMetrics)
	for _, agentID := range breachedAgentIDs {
		_, err := taskStore.create(profileTaskRecord{
			AgentID:        agentID,
			RunID:          run.ID,
			ScenarioID:     run.ScenarioID,
			ScenarioName:   run.ScenarioName,
			TargetID:       target.ID,
			TargetName:     target.Name,
			PprofBaseURL:   target.ProfileEndpoint,
			ProfileType:    "cpu",
			ProfileSeconds: 1,
			Source:         thresholdAutoProfileTaskSource,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func thresholdBreachedAgentIDs(targetMetrics *runTargetMetricsReport) []string {
	if targetMetrics == nil || targetMetrics.SampleCount == 0 {
		return nil
	}
	agentIDs := []string{}
	seen := map[string]struct{}{}
	for _, sample := range targetMetrics.Samples {
		agentID := strings.TrimSpace(sample.AgentID)
		if agentID == "" || !agentMetricExceedsThreshold(sample, targetMetrics.MetricThresholds) {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs
}

func agentMetricExceedsThreshold(sample agentMetricsRecord, thresholds targetMetricThresholds) bool {
	return (thresholds.CPUMaxPercent > 0 && sample.CPUUsagePercent > thresholds.CPUMaxPercent) ||
		(thresholds.MemoryMaxPercent > 0 && sample.MemoryUsagePercent > thresholds.MemoryMaxPercent) ||
		(thresholds.DiskReadMaxBytesPerSec > 0 && sample.DiskReadBytesPerSec > thresholds.DiskReadMaxBytesPerSec) ||
		(thresholds.DiskWriteMaxBytesPerSec > 0 && sample.DiskWriteBytesPerSec > thresholds.DiskWriteMaxBytesPerSec) ||
		(thresholds.NetworkRxMaxBytesPerSec > 0 && sample.NetworkRxBytesPerSec > thresholds.NetworkRxMaxBytesPerSec) ||
		(thresholds.NetworkTxMaxBytesPerSec > 0 && sample.NetworkTxBytesPerSec > thresholds.NetworkTxMaxBytesPerSec)
}

func preflightTargetHealth(ctx context.Context, targetStore *targetStore, input createRunRequest) (int, error) {
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return 0, nil
	}
	target, found, err := targetStore.get(targetID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !found {
		return http.StatusNotFound, fmt.Errorf("target not found")
	}
	if !target.HealthCheck.Enabled && strings.TrimSpace(target.HealthCheck.Path) == "" {
		return 0, nil
	}

	result := checkTargetHealth(ctx, target)
	if err := targetStore.updateLastHealthCheck(target.ID, result); err != nil {
		return http.StatusInternalServerError, err
	}
	if result.Status == "healthy" {
		return 0, nil
	}
	reason := strings.TrimSpace(result.Error)
	if reason == "" {
		reason = result.Status
	}
	return http.StatusConflict, fmt.Errorf("target health preflight failed: %s", reason)
}

func handleStopRun(store *runHistoryStore, manager *runManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "id")
		run, found, err := store.get(runID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, run.ProjectID, run.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if run.Status == "finished" || run.Status == "canceled" || run.Status == "aborted" {
			writeJSON(w, http.StatusOK, run)
			return
		}

		manager.stop(runID)
		run.Status = "canceled"
		if err := store.save(run); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = store.appendEvent(runEventFromResponse("run_canceled", "Run canceled", run))
		writeJSON(w, http.StatusOK, run)
	}
}

func resolveCreateRunRequest(scenarioStore *scenarioStore, targetStore *targetStore, input createRunRequest) (createRunRequest, int, error) {
	scenarioID := strings.TrimSpace(input.ScenarioID)
	if scenarioID == "" {
		normalized, err := validateCreateRunRequest(input)
		return normalized, http.StatusBadRequest, err
	}

	scenario, found, err := scenarioStore.get(scenarioID)
	if err != nil {
		return input, http.StatusInternalServerError, err
	}
	if !found {
		return input, http.StatusNotFound, fmt.Errorf("scenario not found")
	}

	input = applyScenarioToRunRequest(input, scenario)
	targetID := strings.TrimSpace(input.TargetID)
	if targetID != "" {
		target, found, err := targetStore.get(targetID)
		if err != nil {
			return input, http.StatusInternalServerError, err
		}
		if !found {
			return input, http.StatusNotFound, fmt.Errorf("target not found")
		}
		if !workspaceScopesMatch(scenario.ProjectID, scenario.Environment, target.ProjectID, target.Environment) {
			return input, http.StatusForbidden, workspaceScopeError(scenario.ProjectID, scenario.Environment, normalizedWorkspaceScope(target.ProjectID, target.Environment))
		}
		input = applyTargetToRunRequest(input, target, scenario)
	}
	normalized, err := validateCreateRunRequest(input)
	return normalized, http.StatusBadRequest, err
}

func applyScenarioToRunRequest(input createRunRequest, scenario scenarioRecord) createRunRequest {
	input.ScenarioID = scenario.ID
	input.ScenarioName = scenario.Name
	input.ProjectID = scenario.ProjectID
	input.Environment = scenario.Environment
	if strings.TrimSpace(input.Name) == "" {
		input.Name = scenario.Name
	}
	input.Protocol = scenario.Protocol
	input.Method = scenario.Method
	input.URL = scenarioRunURL(scenario)
	input.Headers = scenarioHeadersToRunHeaders(scenario.Headers)
	input.QueryVariants = scenarioQueryVariantsToRunVariants(scenario.QueryVariants)
	input.BodyVariants = scenarioBodyVariantsToRunVariants(scenario.BodyVariants)
	input.FlowSteps = executableScenarioFlowSteps(scenario.FlowSteps, scenario.Method)
	input.Assertion = strings.TrimSpace(scenario.Assertion)
	if input.TimeoutMs <= 0 {
		input.TimeoutMs = parseScenarioInt(scenario.TimeoutMs)
	}
	return input
}

func scenarioRunURL(scenario scenarioRecord) string {
	baseURL := strings.TrimRight(strings.TrimSpace(scenario.BaseURL), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(scenario.Path), "/")
	return baseURL + path
}

func applyTargetToRunRequest(input createRunRequest, target targetRecord, scenario scenarioRecord) createRunRequest {
	input.TargetID = target.ID
	input.TargetName = target.Name
	input.ProjectID = target.ProjectID
	input.Environment = target.Environment
	input.TargetProfile = target.ProfileEndpoint
	input.TargetAgentIDs = append([]string(nil), target.AgentIDs...)
	input.URL = targetRunURL(target, scenario)
	return input
}

func targetRunURL(target targetRecord, scenario scenarioRecord) string {
	baseURL := strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(scenario.Path), "/")
	return baseURL + path
}

func scenarioHeadersToRunHeaders(headers []scenarioHeader) []runHeader {
	runHeaders := make([]runHeader, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			continue
		}
		runHeaders = append(runHeaders, runHeader{Key: key, Value: strings.TrimSpace(header.Value)})
	}
	return runHeaders
}

func scenarioQueryVariantsToRunVariants(variants []scenarioQueryVariant) []queryVariant {
	runVariants := make([]queryVariant, 0, len(variants))
	for _, variant := range variants {
		queryParams := strings.TrimSpace(variant.QueryParams)
		if queryParams == "" {
			continue
		}
		runVariants = append(runVariants, queryVariant{
			Name:        strings.TrimSpace(variant.Name),
			Weight:      parseScenarioInt(variant.Weight),
			QueryParams: queryParams,
		})
	}
	return runVariants
}

func scenarioBodyVariantsToRunVariants(variants []scenarioBodyVariant) []bodyVariant {
	runVariants := make([]bodyVariant, 0, len(variants))
	for _, variant := range variants {
		body := strings.TrimSpace(variant.Body)
		if body == "" {
			continue
		}
		runVariants = append(runVariants, bodyVariant{
			Name:   strings.TrimSpace(variant.Name),
			Weight: parseScenarioInt(variant.Weight),
			Body:   body,
		})
	}
	return runVariants
}

func executableScenarioFlowSteps(steps []scenarioFlowStep, fallbackMethod string) []scenarioFlowStep {
	runSteps := make([]scenarioFlowStep, 0, len(steps))
	for _, step := range steps {
		if !step.Enabled {
			continue
		}
		stepType := strings.ToLower(strings.TrimSpace(step.Type))
		if stepType == "assertion" {
			assertion := strings.TrimSpace(step.Assertion)
			assertions := normalizeScenarioAssertions(step.Assertions)
			if assertion == "" && len(assertions) == 0 {
				continue
			}
			runSteps = append(runSteps, scenarioFlowStep{
				ID:         strings.TrimSpace(step.ID),
				Name:       strings.TrimSpace(step.Name),
				Type:       "assertion",
				Enabled:    true,
				Assertion:  assertion,
				Assertions: assertions,
				When:       normalizeScenarioCondition(step.When),
			})
			continue
		}
		if stepType == "extract" {
			extractors := normalizeScenarioExtractors(step.Extractors)
			if len(extractors) == 0 {
				continue
			}
			runSteps = append(runSteps, scenarioFlowStep{
				ID:         strings.TrimSpace(step.ID),
				Name:       strings.TrimSpace(step.Name),
				Type:       "extract",
				Enabled:    true,
				Extractors: extractors,
				When:       normalizeScenarioCondition(step.When),
			})
			continue
		}
		if stepType != "request" {
			continue
		}
		protocol := strings.ToUpper(strings.TrimSpace(step.Protocol))
		if protocol != "" && protocol != "HTTP" {
			continue
		}
		path := strings.TrimSpace(step.Path)
		if path == "" {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(step.Method))
		if method == "" {
			method = strings.ToUpper(strings.TrimSpace(fallbackMethod))
		}
		if method == "" {
			method = http.MethodGet
		}
		runSteps = append(runSteps, scenarioFlowStep{
			ID:         strings.TrimSpace(step.ID),
			Name:       strings.TrimSpace(step.Name),
			Type:       "request",
			Enabled:    true,
			Protocol:   "HTTP",
			Method:     method,
			Path:       path,
			Assertion:  strings.TrimSpace(step.Assertion),
			Extractors: normalizeScenarioExtractors(step.Extractors),
			Assertions: normalizeScenarioAssertions(step.Assertions),
			When:       normalizeScenarioCondition(step.When),
		})
	}
	return runSteps
}

func parseScenarioInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func validateCreateRunRequest(input createRunRequest) (createRunRequest, error) {
	input.Protocol = strings.ToUpper(strings.TrimSpace(input.Protocol))
	if input.Protocol == "" {
		input.Protocol = runProtocolHTTP
	}
	if !supportedProtocols[input.Protocol] {
		return input, fmt.Errorf("unsupported protocol: %s, supported protocols: HTTP, CUSTOM_RPC, gRPC, Dubbo, Thrift", input.Protocol)
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		return input, fmt.Errorf("url is required")
	}
	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return input, fmt.Errorf("url must be an absolute http or https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return input, fmt.Errorf("url must use http or https")
	}

	input.Method = strings.TrimSpace(input.Method)
	if input.Protocol == runProtocolHTTP {
		input.Method = strings.ToUpper(input.Method)
		if input.Method == "" {
			input.Method = http.MethodGet
		}
		if input.Method != http.MethodGet && input.Method != http.MethodPost {
			return input, fmt.Errorf("method must be GET or POST")
		}
	} else if input.Method == "" {
		return input, fmt.Errorf("custom RPC method is required")
	}
	if input.TotalRequests <= 0 || input.TotalRequests > maxRunRequests {
		return input, fmt.Errorf("totalRequests must be between 1 and %d", maxRunRequests)
	}
	if input.Concurrency <= 0 || input.Concurrency > maxRunConcurrency {
		return input, fmt.Errorf("concurrency must be between 1 and %d", maxRunConcurrency)
	}
	if input.Concurrency > input.TotalRequests {
		input.Concurrency = input.TotalRequests
	}
	if input.TimeoutMs <= 0 {
		input.TimeoutMs = defaultRunTimeoutMs
	}
	if input.MaxErrorRatePercent < 0 || input.MaxErrorRatePercent > 100 {
		return input, fmt.Errorf("maxErrorRatePercent must be between 0 and 100")
	}
	if input.MaxP95LatencyMs < 0 {
		return input, fmt.Errorf("maxP95LatencyMs must be greater than or equal to 0")
	}
	if input.Name == "" {
		if input.Protocol == runProtocolCustomRPC {
			input.Name = "ad-hoc-custom-rpc-run"
		} else {
			input.Name = "ad-hoc-http-run"
		}
	}

	// gRPC 协议特定验证
	if strings.EqualFold(input.Protocol, "gRPC") {
		if err := validateGRPCRequest(input); err != nil {
			return input, err
		}
	}

	input.Headers = normalizeRunHeaders(input.Headers)
	if err := validateBodyVariants(input.BodyVariants); err != nil {
		return input, err
	}
	if err := validateQueryVariants(input.QueryVariants); err != nil {
		return input, err
	}
	input.Assertion = strings.TrimSpace(input.Assertion)
	return input, nil
}

// validateGRPCRequest 验证 gRPC 请求的必需字段
func validateGRPCRequest(input createRunRequest) error {
	headers := runHeadersToMap(input.Headers)
	if headers["X-GRPC-Target"] == "" {
		return fmt.Errorf("gRPC protocol requires X-GRPC-Target header")
	}
	return nil
}

func normalizeRunHeaders(headers []runHeader) []runHeader {
	runHeaders := make([]runHeader, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			continue
		}
		runHeaders = append(runHeaders, runHeader{Key: key, Value: strings.TrimSpace(header.Value)})
	}
	return runHeaders
}

func executeHTTPRun(ctx context.Context, input createRunRequest) createRunResponse {
	return executeHTTPRunWithProgress(ctx, input, nil)
}

func executeHTTPRunWithProgress(ctx context.Context, input createRunRequest, onProgress func(createRunResponse)) createRunResponse {
	return executeRunWithProgress(ctx, input, onProgress)
}

func executeRunWithProgress(ctx context.Context, input createRunRequest, onProgress func(createRunResponse)) createRunResponse {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := &http.Client{Timeout: time.Duration(input.TimeoutMs) * time.Millisecond}
	jobs := make(chan struct{}, input.TotalRequests)
	samples := make(chan runSample, input.TotalRequests)
	startedAt := time.Now()
	progress := runProgress{
		input:     input,
		startedAt: startedAt,
		latencies: make([]float64, 0, input.TotalRequests),
	}

	for requestIndex := 0; requestIndex < input.TotalRequests; requestIndex++ {
		jobs <- struct{}{}
	}
	close(jobs)

	var workers sync.WaitGroup
	for worker := 0; worker < input.Concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-runCtx.Done():
					return
				case _, ok := <-jobs:
					if !ok {
						return
					}
					if runCtx.Err() != nil {
						return
					}
					sample := executeRunSample(runCtx, client, input)
					samples <- sample
				}
			}
		}()
	}

	go func() {
		workers.Wait()
		close(samples)
	}()

	status := "finished"
	for sample := range samples {
		progress.add(sample)
		progressStatus := "running"
		if status == "aborted" {
			progressStatus = "aborted"
		}
		snapshot := progress.snapshot(progressStatus)
		if status == "finished" && guardrailExceeded(input, snapshot) {
			status = "aborted"
			cancel()
			snapshot = progress.snapshot(status)
		}
		if onProgress != nil && ctx.Err() == nil {
			onProgress(snapshot)
		}
	}

	if ctx.Err() != nil {
		status = "canceled"
	}
	return progress.snapshot(status)
}

func executeRunSample(ctx context.Context, client *http.Client, input createRunRequest) runSample {
	// HTTP 协议使用原有逻辑
	if strings.EqualFold(input.Protocol, runProtocolHTTP) {
		return executeHTTPSample(ctx, client, input)
	}

	// CUSTOM_RPC 协议继续使用原有逻辑（HTTP Adapter）
	if strings.EqualFold(input.Protocol, runProtocolCustomRPC) {
		return executeCustomRPCSample(ctx, client, input)
	}

	// 其他协议尝试使用插件
	return executePluginSample(ctx, input)
}

// executePluginSample 使用插件执行样本
func executePluginSample(ctx context.Context, input createRunRequest) runSample {
	startedAt := time.Now()

	// 构造插件请求
	pluginReq := &rpc.PluginRequest{
		Method:      input.Method,
		Headers:     runHeadersToMap(input.Headers),
		QueryParams: requestQuery(input, nil),
		Body:        customRPCBodyValue(selectRunBody(input, nil)),
		RunID:       input.RunID,
		ScenarioID:  input.ScenarioID,
		TargetID:    input.TargetID,
		TargetName:  input.TargetName,
	}

	// 检查是否有对应插件
	if !rpc.DefaultManager.HasPlugin(input.Protocol) {
		// 无插件，尝试使用 HTTP Adapter
		adapterURL := rpc.DefaultManager.GetAdapterURL(input.Protocol)
		if adapterURL == "" {
			// 既无插件也无 Adapter，返回错误
			return runSample{
				latency:   time.Since(startedAt),
				error:     fmt.Sprintf("unknown protocol: %s, no plugin or adapter configured", input.Protocol),
				success:   false,
				statusCode: 0,
			}
		}
		// 使用 HTTP Adapter（复用现有 executeCustomRPCSample 逻辑）
		return executeCustomRPCSample(ctx, nil, input)
	}

	// 执行插件调用
	plugin, ok := rpc.DefaultManager.Get(input.Protocol)
	if !ok {
		return runSample{
			latency:   time.Since(startedAt),
			error:     fmt.Sprintf("plugin not found: %s", input.Protocol),
			success:   false,
			statusCode: 0,
		}
	}

	pluginResp, err := plugin.Execute(ctx, pluginReq)
	if err != nil {
		// 【gRPC 自动降级】如果 gRPC 插件返回不支持直接调用的错误，自动降级到 HTTP Adapter
		if input.Protocol == "gRPC" && isGRPCDirectCallNotSupported(err) {
			log.Printf("gRPC direct call not supported, fallback to HTTP Adapter: %v", err)
			// 检查是否有配置 HTTP Adapter
			adapterURL := rpc.DefaultManager.GetAdapterURL("gRPC")
			if adapterURL != "" && adapterClient != nil {
				// 使用 HTTP Adapter 执行
				return executeHTTPAdapterForGRPC(ctx, adapterURL, pluginReq)
			}
			// 无 Adapter 配置，返回更友好的错误信息
			return runSample{
				latency:   time.Since(startedAt),
				error:     "gRPC direct call not supported. Please configure gRPC HTTP Adapter URL in settings.",
				success:   false,
				statusCode: 0,
			}
		}
		return runSample{
			latency:   time.Since(startedAt),
			error:     err.Error(),
			success:   false,
			statusCode: 0,
		}
	}

	return runSample{
		latency:    time.Since(startedAt),
		error:      pluginResp.Error,
		success:    pluginResp.Success,
		statusCode: pluginResp.StatusCode,
		body:       []byte(fmt.Sprintf("%v", pluginResp.Data)),
	}
}

// isGRPCDirectCallNotSupported 检查是否是 gRPC 直接调用不支持的错误
func isGRPCDirectCallNotSupported(err error) bool {
	return err != nil && strings.Contains(err.Error(), "gRPC direct call not supported")
}

// executeHTTPAdapterForGRPC 使用 HTTP Adapter 执行 gRPC 调用
func executeHTTPAdapterForGRPC(ctx context.Context, adapterURL string, req *rpc.PluginRequest) runSample {
	startedAt := time.Now()

	// 如果没有 adapterClient，返回错误
	if adapterClient == nil {
		return runSample{
			latency:   time.Since(startedAt),
			error:     "HTTP Adapter client not initialized",
			success:   false,
			statusCode: 0,
		}
	}

	// 调用 HTTP Adapter
	resp, err := adapterClient.ExecuteWithProtocol(ctx, "gRPC", req)
	if err != nil {
		return runSample{
			latency:   time.Since(startedAt),
			error:     fmt.Sprintf("gRPC adapter call failed: %v", err),
			success:   false,
			statusCode: 0,
		}
	}

	return runSample{
		latency:    time.Since(startedAt),
		error:      resp.Error,
		success:    resp.Success,
		statusCode: resp.StatusCode,
		body:       []byte(fmt.Sprintf("%v", resp.Data)),
	}
}

func guardrailExceeded(input createRunRequest, snapshot createRunResponse) bool {
	completedRequests := snapshot.SuccessRequests + snapshot.FailedRequests
	if completedRequests == 0 {
		return false
	}
	if input.MaxErrorRatePercent > 0 {
		errorRate := (float64(snapshot.FailedRequests) / float64(completedRequests)) * 100
		if errorRate > input.MaxErrorRatePercent {
			return true
		}
	}
	if input.MaxP95LatencyMs > 0 && snapshot.P95LatencyMs > input.MaxP95LatencyMs {
		return true
	}
	return false
}

func newRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
}

func newRunEventID(runID string) string {
	return fmt.Sprintf("run-event-%s-%d-%d", strings.TrimSpace(runID), time.Now().UTC().UnixNano(), rand.Int63())
}

func newRunRequestSampleID(runID string, kind string) string {
	return fmt.Sprintf("run-sample-%s-%s-%d-%d", strings.TrimSpace(runID), strings.TrimSpace(kind), time.Now().UTC().UnixNano(), rand.Int63())
}

func runEventFromResponse(eventType string, message string, run createRunResponse) runEventRecord {
	return runEventRecord{
		ID:              newRunEventID(run.ID),
		RunID:           run.ID,
		Type:            eventType,
		Status:          run.Status,
		Message:         message,
		SuccessRequests: run.SuccessRequests,
		FailedRequests:  run.FailedRequests,
		TotalRequests:   run.TotalRequests,
		QPS:             run.QPS,
		P95LatencyMs:    run.P95LatencyMs,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func runTerminalEventType(status string) string {
	if status == "aborted" {
		return "run_aborted"
	}
	if status == "canceled" {
		return "run_canceled"
	}
	return "run_finished"
}

func runTerminalEventMessage(status string) string {
	if status == "aborted" {
		return "Run aborted by guardrail"
	}
	if status == "canceled" {
		return "Run canceled"
	}
	return "Run finished"
}

func startRunProfileCapture(ctx context.Context, store *profileArtifactStore, input createRunRequest) <-chan []profileArtifactRecord {
	done := make(chan []profileArtifactRecord, 1)
	if strings.TrimSpace(input.TargetProfile) == "" || strings.TrimSpace(input.TargetID) == "" {
		done <- nil
		return done
	}

	go func() {
		artifact := captureCPUProfileArtifact(ctx, store, input)
		done <- []profileArtifactRecord{artifact}
	}()
	return done
}

func captureCPUProfileArtifact(ctx context.Context, store *profileArtifactStore, input createRunRequest) profileArtifactRecord {
	startedAt := time.Now().UTC()
	artifact := profileArtifactRecord{
		ID:           fmt.Sprintf("profile-%d", startedAt.UnixNano()),
		RunID:        input.RunID,
		ScenarioID:   input.ScenarioID,
		ScenarioName: input.ScenarioName,
		TargetID:     input.TargetID,
		TargetName:   input.TargetName,
		ProfileType:  "cpu",
		Status:       "failed",
		SourceURL:    cpuProfileURL(input.TargetProfile),
		FileName:     fmt.Sprintf("%s-cpu.pprof", input.RunID),
		StartedAt:    startedAt.Format(time.RFC3339Nano),
	}

	payload, contentType, err := fetchProfileArtifact(ctx, artifact.SourceURL)
	finishedAt := time.Now().UTC()
	artifact.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	if err != nil {
		artifact.Error = err.Error()
		saved, saveErr := store.save(artifact, nil)
		if saveErr != nil {
			artifact.Error = saveErr.Error()
			return artifact
		}
		return saved
	}

	artifact.Status = "collected"
	artifact.ContentType = contentType
	artifact.SizeBytes = int64(len(payload))
	saved, err := store.save(artifact, payload)
	if err != nil {
		artifact.Status = "failed"
		artifact.Error = err.Error()
		return artifact
	}
	return saved
}

func cpuProfileURL(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/") + "/profile?seconds=1"
}

func fetchProfileArtifact(ctx context.Context, profileURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, "", err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return nil, response.Header.Get("Content-Type"), fmt.Errorf("profile endpoint returned HTTP %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, maxProfileBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, response.Header.Get("Content-Type"), err
	}
	if len(payload) > maxProfileBytes {
		return nil, response.Header.Get("Content-Type"), fmt.Errorf("profile artifact exceeds %d bytes", maxProfileBytes)
	}
	return payload, response.Header.Get("Content-Type"), nil
}

func executeHTTPSample(ctx context.Context, client *http.Client, input createRunRequest) runSample {
	startedAt := time.Now()
	if len(input.FlowSteps) > 0 {
		flowClient := httpClientWithCookieJar(client)
		var lastOutcome httpRequestOutcome
		variables := map[string]string{}
		for _, step := range input.FlowSteps {
			shouldRun, err := shouldRunHTTPFlowStep(lastOutcome, step.When, variables)
			if err != nil {
				lastOutcome = httpRequestOutcome{method: "WHEN", url: input.URL, success: false, err: err}
				return runSampleFromOutcome(startedAt, lastOutcome)
			}
			if !shouldRun {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(step.Type)) {
			case "assertion":
				lastOutcome = executeHTTPAssertionStep(lastOutcome, step.Assertion, step.Assertions, variables)
			case "extract":
				lastOutcome = executeHTTPExtractStep(lastOutcome, step.Extractors, variables)
			default:
				lastOutcome = executeHTTPFlowStep(ctx, flowClient, input, step, variables)
				if lastOutcome.success && len(step.Extractors) > 0 {
					lastOutcome = executeHTTPExtractStep(lastOutcome, step.Extractors, variables)
				}
			}
			if !lastOutcome.success {
				return runSampleFromOutcome(startedAt, lastOutcome)
			}
		}
		if lastOutcome.method == "" {
			lastOutcome = httpRequestOutcome{method: input.Method, url: input.URL, success: true}
		}
		return runSampleFromOutcome(startedAt, lastOutcome)
	}

	outcome := executeHTTPRequest(ctx, client, input.Method, input.URL, input.Headers, requestQuery(input, nil), requestBody(input, nil), input.Assertion, nil, nil)
	return runSampleFromOutcome(startedAt, outcome)
}

type customRPCAdapterRequest struct {
	Protocol    string            `json:"protocol"`
	Method      string            `json:"method"`
	RunID       string            `json:"runId,omitempty"`
	ScenarioID  string            `json:"scenarioId,omitempty"`
	TargetID    string            `json:"targetId,omitempty"`
	TargetName  string            `json:"targetName,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams string            `json:"queryParams,omitempty"`
	Body        any               `json:"body,omitempty"`
}

type customRPCAdapterResponse struct {
	Success    *bool  `json:"success,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

func executeCustomRPCSample(ctx context.Context, client *http.Client, input createRunRequest) runSample {
	startedAt := time.Now()
	outcome := httpRequestOutcome{method: input.Method, url: input.URL}
	payload, err := json.Marshal(customRPCAdapterPayload(input))
	if err != nil {
		outcome.err = err
		return runSampleFromOutcome(startedAt, outcome)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, input.URL, bytes.NewReader(payload))
	if err != nil {
		outcome.err = err
		return runSampleFromOutcome(startedAt, outcome)
	}
	request.Header.Set("Content-Type", "application/json")
	for _, header := range input.Headers {
		request.Header.Set(header.Key, header.Value)
	}

	response, err := client.Do(request)
	if err != nil {
		outcome.err = err
		return runSampleFromOutcome(startedAt, outcome)
	}
	defer response.Body.Close()

	outcome.statusCode = response.StatusCode
	body, err := readRunResponseBody(response.Body)
	if err != nil {
		outcome.err = err
		return runSampleFromOutcome(startedAt, outcome)
	}
	outcome.body = body
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		outcome.err = fmt.Errorf("custom RPC adapter returned HTTP %d", response.StatusCode)
		return runSampleFromOutcome(startedAt, outcome)
	}

	var adapterResponse customRPCAdapterResponse
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &adapterResponse); err != nil {
			outcome.err = fmt.Errorf("custom RPC adapter response must be JSON: %w", err)
			return runSampleFromOutcome(startedAt, outcome)
		}
	}
	if adapterResponse.StatusCode > 0 {
		outcome.statusCode = adapterResponse.StatusCode
	}
	if adapterResponse.Success != nil && !*adapterResponse.Success {
		if strings.TrimSpace(adapterResponse.Error) == "" {
			adapterResponse.Error = "custom RPC adapter reported failure"
		}
		outcome.err = fmt.Errorf("%s", adapterResponse.Error)
		return runSampleFromOutcome(startedAt, outcome)
	}
	if adapterResponse.StatusCode >= http.StatusBadRequest {
		outcome.err = fmt.Errorf("custom RPC status %d", adapterResponse.StatusCode)
		return runSampleFromOutcome(startedAt, outcome)
	}

	outcome.success = true
	return runSampleFromOutcome(startedAt, outcome)
}

func customRPCAdapterPayload(input createRunRequest) customRPCAdapterRequest {
	return customRPCAdapterRequest{
		Protocol:    runProtocolCustomRPC,
		Method:      strings.TrimSpace(input.Method),
		RunID:       strings.TrimSpace(input.RunID),
		ScenarioID:  strings.TrimSpace(input.ScenarioID),
		TargetID:    strings.TrimSpace(input.TargetID),
		TargetName:  strings.TrimSpace(input.TargetName),
		Headers:     runHeadersToMap(input.Headers),
		QueryParams: requestQuery(input, nil),
		Body:        customRPCBodyValue(selectRunBody(input, nil)),
	}
}

func selectRunBody(input createRunRequest, variables map[string]string) string {
	if len(input.BodyVariants) == 0 {
		return ""
	}
	return substituteVariables(selectWeightedBody(input.BodyVariants), variables)
}

func customRPCBodyValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return raw
}

func runHeadersToMap(headers []runHeader) map[string]string {
	headerMap := map[string]string{}
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			continue
		}
		headerMap[key] = strings.TrimSpace(header.Value)
	}
	if len(headerMap) == 0 {
		return nil
	}
	return headerMap
}

func httpClientWithCookieJar(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	flowClient := *client
	jar, err := cookiejar.New(nil)
	if err == nil {
		flowClient.Jar = jar
	}
	return &flowClient
}

func shouldRunHTTPFlowStep(previous httpRequestOutcome, condition *scenarioResponseAssertion, variables map[string]string) (bool, error) {
	condition = normalizeScenarioCondition(condition)
	if condition == nil {
		return true, nil
	}
	actual, err := responseAssertionActualValue(previous, *condition, variables)
	if err != nil {
		return false, nil
	}
	expected := substituteVariables(condition.Expected, variables)
	matched, err := responseAssertionMatches(*condition, actual, expected)
	if err != nil {
		return false, fmt.Errorf("when condition failed: %s: %w", condition.Name, err)
	}
	return matched, nil
}

func runSampleFromOutcome(startedAt time.Time, outcome httpRequestOutcome) runSample {
	errMessage := ""
	if outcome.err != nil {
		errMessage = outcome.err.Error()
	}
	return runSample{
		latency:    time.Since(startedAt),
		method:     outcome.method,
		url:        outcome.url,
		statusCode: outcome.statusCode,
		success:    outcome.success,
		error:      errMessage,
	}
}

func executeHTTPFlowStep(ctx context.Context, client *http.Client, input createRunRequest, step scenarioFlowStep, variables map[string]string) httpRequestOutcome {
	method := strings.ToUpper(strings.TrimSpace(step.Method))
	if method == "" {
		method = input.Method
	}
	assertion := strings.TrimSpace(step.Assertion)
	if assertion == "" {
		assertion = input.Assertion
	}
	stepPath := substituteVariables(step.Path, variables)
	return executeHTTPRequest(ctx, client, method, flowStepURL(input.URL, stepPath), input.Headers, requestQuery(input, variables), requestBodyForMethod(input, method, variables), assertion, step.Assertions, variables)
}

func executeHTTPAssertionStep(previous httpRequestOutcome, assertion string, assertions []scenarioResponseAssertion, variables map[string]string) httpRequestOutcome {
	assertion = strings.TrimSpace(assertion)
	if previous.method == "" {
		previous.method = "ASSERT"
	}
	if previous.statusCode == 0 {
		previous.success = false
		previous.err = fmt.Errorf("status assertion failed: no previous HTTP response")
		return previous
	}
	if assertion != "" {
		matched, err := evaluateStatusAssertion(assertion, previous.statusCode)
		previous.success = matched && err == nil
		if err != nil {
			previous.err = err
			return previous
		}
		if !matched {
			previous.err = fmt.Errorf("status assertion failed: expected %s, got %d", assertion, previous.statusCode)
			return previous
		}
	}
	if err := evaluateResponseAssertions(previous, assertions, variables); err != nil {
		previous.success = false
		previous.err = err
		return previous
	}
	previous.success = true
	previous.err = nil
	return previous
}

func executeHTTPExtractStep(previous httpRequestOutcome, extractors []scenarioVariableExtractor, variables map[string]string) httpRequestOutcome {
	if previous.method == "" {
		previous.method = "EXTRACT"
		previous.success = false
		previous.err = fmt.Errorf("extract failed: no previous HTTP response")
		return previous
	}
	for _, extractor := range normalizeScenarioExtractors(extractors) {
		value, err := extractVariable(previous, extractor)
		if err != nil {
			previous.success = false
			previous.err = fmt.Errorf("extractor %q failed: %w", extractor.Name, err)
			return previous
		}
		variables[extractor.Name] = value
	}
	previous.err = nil
	previous.success = true
	return previous
}

func requestQuery(input createRunRequest, variables map[string]string) string {
	if len(input.QueryVariants) > 0 {
		return substituteVariables(selectWeightedQuery(input.QueryVariants), variables)
	}
	return ""
}

func requestBody(input createRunRequest, variables map[string]string) io.Reader {
	if len(input.BodyVariants) == 0 {
		return nil
	}
	return strings.NewReader(selectRunBody(input, variables))
}

func requestBodyForMethod(input createRunRequest, method string, variables map[string]string) io.Reader {
	if strings.EqualFold(method, http.MethodGet) {
		return nil
	}
	return requestBody(input, variables)
}

func executeHTTPRequest(ctx context.Context, client *http.Client, method string, targetURL string, headers []runHeader, queryParams string, requestBody io.Reader, assertion string, assertions []scenarioResponseAssertion, variables map[string]string) httpRequestOutcome {
	targetURL = substituteVariables(targetURL, variables)
	if queryParams != "" {
		targetURL = appendQueryParams(targetURL, substituteVariables(queryParams, variables))
	}
	outcome := httpRequestOutcome{method: method, url: targetURL}
	request, err := http.NewRequestWithContext(ctx, method, targetURL, requestBody)
	if err != nil {
		outcome.err = err
		return outcome
	}
	for _, header := range headers {
		request.Header.Set(header.Key, substituteVariables(header.Value, variables))
	}
	response, err := client.Do(request)
	if err != nil {
		outcome.err = err
		return outcome
	}
	defer response.Body.Close()

	outcome.statusCode = response.StatusCode
	outcome.headers = response.Header.Clone()
	body, err := readRunResponseBody(response.Body)
	if err != nil {
		outcome.err = err
		return outcome
	}
	outcome.body = body
	assertion = substituteVariables(assertion, variables)
	if strings.TrimSpace(assertion) != "" {
		matched, err := evaluateStatusAssertion(assertion, response.StatusCode)
		outcome.success = matched && err == nil
		if err != nil {
			outcome.err = err
		} else if !matched {
			outcome.err = fmt.Errorf("status assertion failed: expected %s, got %d", strings.TrimSpace(assertion), response.StatusCode)
		}
		if !outcome.success {
			return outcome
		}
	} else {
		outcome.success = response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest
		if !outcome.success {
			outcome.err = fmt.Errorf("HTTP %d", response.StatusCode)
			return outcome
		}
	}

	if err := evaluateResponseAssertions(outcome, assertions, variables); err != nil {
		outcome.success = false
		outcome.err = err
		return outcome
	}
	outcome.success = true
	return outcome
}

func readRunResponseBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxRunResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxRunResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxRunResponseBytes)
	}
	return payload, nil
}

func extractVariable(outcome httpRequestOutcome, extractor scenarioVariableExtractor) (string, error) {
	switch extractor.Source {
	case "header":
		value := strings.TrimSpace(outcome.headers.Get(extractor.Path))
		if value == "" {
			return "", fmt.Errorf("header %q is empty", extractor.Path)
		}
		return value, nil
	case "json":
		return extractJSONPath(outcome.body, extractor.Path)
	case "regex":
		return extractRegexPath(outcome.body, extractor.Path)
	default:
		return "", fmt.Errorf("unsupported extractor source %q", extractor.Source)
	}
}

func extractRegexPath(body []byte, pattern string) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("response body is empty")
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	matches := expression.FindStringSubmatch(string(body))
	if len(matches) == 0 {
		return "", fmt.Errorf("pattern %q not found", pattern)
	}
	value := matches[0]
	if len(matches) > 1 {
		value = matches[1]
	}
	if value == "" {
		return "", fmt.Errorf("matched value is empty")
	}
	return value, nil
}

func extractJSONPath(body []byte, path string) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("JSON response body is empty")
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	current := payload
	normalizedPath := strings.TrimPrefix(strings.TrimSpace(path), "$.")
	for _, segment := range strings.Split(normalizedPath, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]
			if !ok {
				return "", fmt.Errorf("path %q not found", path)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return "", fmt.Errorf("array index %q not found in path %q", segment, path)
			}
			current = typed[index]
		default:
			return "", fmt.Errorf("path %q cannot descend into %T", path, current)
		}
	}
	return extractedValueString(current)
}

func extractedValueString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", fmt.Errorf("extracted value is null")
	case string:
		if typed == "" {
			return "", fmt.Errorf("extracted value is empty")
		}
		return typed, nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return fmt.Sprint(typed), nil
	}
}

func evaluateResponseAssertions(outcome httpRequestOutcome, assertions []scenarioResponseAssertion, variables map[string]string) error {
	for _, assertion := range normalizeScenarioAssertions(assertions) {
		if err := evaluateResponseAssertion(outcome, assertion, variables); err != nil {
			return err
		}
	}
	return nil
}

func evaluateResponseAssertion(outcome httpRequestOutcome, assertion scenarioResponseAssertion, variables map[string]string) error {
	actual, err := responseAssertionActualValue(outcome, assertion, variables)
	if err != nil {
		return fmt.Errorf("response assertion failed: %s: %w", assertion.Name, err)
	}
	expected := substituteVariables(assertion.Expected, variables)
	matched, err := responseAssertionMatches(assertion, actual, expected)
	if err != nil {
		return fmt.Errorf("response assertion failed: %s: %w", assertion.Name, err)
	}
	if !matched {
		return fmt.Errorf("response assertion failed: %s expected %q %s %q", assertion.Name, actual, assertion.Operator, expected)
	}
	return nil
}

func responseAssertionMatches(assertion scenarioResponseAssertion, actual string, expected string) (bool, error) {
	switch assertion.Operator {
	case "equals":
		return actual == expected, nil
	case "not_equals":
		return actual != expected, nil
	case "contains":
		return strings.Contains(actual, expected), nil
	case "not_contains":
		return !strings.Contains(actual, expected), nil
	case "matches":
		matched, err := regexp.MatchString(expected, actual)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", expected, err)
		}
		return matched, nil
	case "not_matches":
		matched, err := regexp.MatchString(expected, actual)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", expected, err)
		}
		return !matched, nil
	case "exists":
		return strings.TrimSpace(actual) != "", nil
	default:
		return false, fmt.Errorf("unsupported operator %q", assertion.Operator)
	}
}

func responseAssertionActualValue(outcome httpRequestOutcome, assertion scenarioResponseAssertion, variables map[string]string) (string, error) {
	switch assertion.Source {
	case "body":
		if len(outcome.body) == 0 {
			return "", fmt.Errorf("response body is empty")
		}
		return string(outcome.body), nil
	case "header":
		value := strings.TrimSpace(outcome.headers.Get(assertion.Path))
		if value == "" {
			return "", fmt.Errorf("header %q is empty", assertion.Path)
		}
		return value, nil
	case "json":
		return extractJSONPath(outcome.body, assertion.Path)
	case "regex":
		return extractRegexPath(outcome.body, assertion.Path)
	case "variable":
		value := strings.TrimSpace(variables[assertion.Path])
		if value == "" {
			return "", fmt.Errorf("variable %q is empty", assertion.Path)
		}
		return value, nil
	default:
		return "", fmt.Errorf("unsupported source %q", assertion.Source)
	}
}

func substituteVariables(value string, variables map[string]string) string {
	if value == "" || len(variables) == 0 {
		return value
	}
	result := value
	for name, variableValue := range variables {
		result = strings.ReplaceAll(result, "${"+name+"}", variableValue)
	}
	return result
}

func evaluateStatusAssertion(assertion string, statusCode int) (bool, error) {
	parts := strings.Fields(strings.TrimSpace(assertion))
	if len(parts) != 3 || !strings.EqualFold(parts[0], "status") {
		return false, fmt.Errorf("unsupported status assertion %q", strings.TrimSpace(assertion))
	}
	expectedStatus, err := strconv.Atoi(parts[2])
	if err != nil {
		return false, fmt.Errorf("unsupported status assertion %q", strings.TrimSpace(assertion))
	}

	switch parts[1] {
	case "<":
		return statusCode < expectedStatus, nil
	case "<=":
		return statusCode <= expectedStatus, nil
	case "=", "==":
		return statusCode == expectedStatus, nil
	case "!=":
		return statusCode != expectedStatus, nil
	case ">=":
		return statusCode >= expectedStatus, nil
	case ">":
		return statusCode > expectedStatus, nil
	default:
		return false, fmt.Errorf("unsupported status assertion %q", strings.TrimSpace(assertion))
	}
}

func flowStepURL(baseURL string, stepPath string) string {
	path := strings.TrimSpace(stepPath)
	if path == "" {
		return baseURL
	}

	parsedStepURL, err := url.Parse(path)
	if err == nil && parsedStepURL.IsAbs() {
		return parsedStepURL.String()
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	if err == nil && parsedStepURL != nil {
		parsedBaseURL.Path = "/" + strings.TrimLeft(parsedStepURL.Path, "/")
		parsedBaseURL.RawQuery = parsedStepURL.RawQuery
		return parsedBaseURL.String()
	}

	parsedBaseURL.Path = "/" + strings.TrimLeft(path, "/")
	parsedBaseURL.RawQuery = ""
	return parsedBaseURL.String()
}

func (progress *runProgress) add(sample runSample) {
	latencyMs := float64(sample.latency.Microseconds()) / 1000
	progress.latencies = append(progress.latencies, latencyMs)
	progress.totalLatencyMs += latencyMs
	progress.recordRequestSample(sample, latencyMs)
	if sample.success {
		progress.success++
	} else {
		progress.failed++
	}
}

func (progress *runProgress) recordRequestSample(sample runSample, latencyMs float64) {
	record := runRequestSampleRecord{
		RunID:      progress.input.RunID,
		Method:     sample.method,
		URL:        sample.url,
		StatusCode: sample.statusCode,
		Success:    sample.success,
		LatencyMs:  latencyMs,
		Error:      sample.error,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}

	slowRecord := record
	slowRecord.Kind = "slow"
	slowRecord.ID = newRunRequestSampleID(record.RunID, slowRecord.Kind)
	progress.slowSamples = append(progress.slowSamples, slowRecord)
	sort.SliceStable(progress.slowSamples, func(left, right int) bool {
		return progress.slowSamples[left].LatencyMs > progress.slowSamples[right].LatencyMs
	})
	if len(progress.slowSamples) > maxRunSamples {
		progress.slowSamples = progress.slowSamples[:maxRunSamples]
	}

	if sample.success {
		return
	}
	errorRecord := record
	errorRecord.Kind = "error"
	errorRecord.ID = newRunRequestSampleID(record.RunID, errorRecord.Kind)
	progress.errorSamples = append(progress.errorSamples, errorRecord)
	if len(progress.errorSamples) > maxRunSamples {
		progress.errorSamples = progress.errorSamples[:maxRunSamples]
	}
}

func (progress *runProgress) snapshot(status string) createRunResponse {
	now := time.Now().UTC()
	createdAt := progress.input.CreatedAt
	if createdAt == "" {
		createdAt = now.Format(time.RFC3339Nano)
	}
	durationStart := progress.startedAt
	if parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt); err == nil && parsedCreatedAt.Before(durationStart) {
		durationStart = parsedCreatedAt
	}
	duration := now.Sub(durationStart)
	if duration < 0 {
		duration = 0
	}
	durationMs := float64(duration.Microseconds()) / 1000
	durationSeconds := math.Max(duration.Seconds(), 0.001)
	sortedLatencies := append([]float64(nil), progress.latencies...)
	sort.Float64s(sortedLatencies)
	runID := progress.input.RunID
	if runID == "" {
		runID = newRunID()
	}

	return createRunResponse{
		ID:                  runID,
		ScenarioID:          progress.input.ScenarioID,
		ScenarioName:        progress.input.ScenarioName,
		TargetID:            progress.input.TargetID,
		TargetName:          progress.input.TargetName,
		ProjectID:           progress.input.ProjectID,
		Environment:         progress.input.Environment,
		Name:                progress.input.Name,
		Status:              status,
		Protocol:            normalizedRunProtocol(progress.input.Protocol),
		Method:              progress.input.Method,
		URL:                 progress.input.URL,
		TotalRequests:       progress.input.TotalRequests,
		SuccessRequests:     progress.success,
		FailedRequests:      progress.failed,
		MaxErrorRatePercent: progress.input.MaxErrorRatePercent,
		MaxP95LatencyMs:     progress.input.MaxP95LatencyMs,
		DurationMs:          durationMs,
		QPS:                 float64(len(progress.latencies)) / durationSeconds,
		AverageLatencyMs:    averageLatency(progress.totalLatencyMs, len(progress.latencies)),
		P95LatencyMs:        percentile(sortedLatencies, 0.95),
		CreatedAt:           createdAt,
		SlowSamples:         append([]runRequestSampleRecord(nil), progress.slowSamples...),
		ErrorSamples:        append([]runRequestSampleRecord(nil), progress.errorSamples...),
	}
}

// 全局 Adapter 客户端
var adapterClient *rpc.HTTPAdapterClient

// initAdapter 初始化 Adapter 客户端
func initAdapter(adapterURL string) {
	if adapterURL != "" {
		adapterClient = rpc.NewHTTPAdapterClient(&rpc.AdapterConfig{
			URL:        adapterURL,
			TimeoutMs:  30000,
			MaxRetries: 2,
		})
	}
}

func normalizedRunProtocol(protocol string) string {
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	if protocol == "" {
		return runProtocolHTTP
	}
	return protocol
}

func averageLatency(total float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func validateBodyVariants(variants []bodyVariant) error {
	if len(variants) == 0 {
		return nil
	}

	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight < 0 {
			return fmt.Errorf("bodyVariants weight must be greater than or equal to 0")
		}
		totalWeight += variant.Weight
	}
	if totalWeight == 0 {
		return fmt.Errorf("bodyVariants must include at least one positive weight")
	}

	return nil
}

func validateQueryVariants(variants []queryVariant) error {
	if len(variants) == 0 {
		return nil
	}

	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight < 0 {
			return fmt.Errorf("queryVariants weight must be greater than or equal to 0")
		}
		totalWeight += variant.Weight
	}
	if totalWeight == 0 {
		return fmt.Errorf("queryVariants must include at least one positive weight")
	}

	return nil
}

func selectWeightedBody(variants []bodyVariant) string {
	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight > 0 {
			totalWeight += variant.Weight
		}
	}
	if totalWeight == 0 {
		return ""
	}

	pick := rand.Intn(totalWeight)
	cumulativeWeight := 0
	for _, variant := range variants {
		if variant.Weight <= 0 {
			continue
		}
		cumulativeWeight += variant.Weight
		if pick < cumulativeWeight {
			return variant.Body
		}
	}

	return ""
}

func selectWeightedQuery(variants []queryVariant) string {
	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight > 0 {
			totalWeight += variant.Weight
		}
	}
	if totalWeight == 0 {
		return ""
	}

	pick := rand.Intn(totalWeight)
	cumulativeWeight := 0
	for _, variant := range variants {
		if variant.Weight <= 0 {
			continue
		}
		cumulativeWeight += variant.Weight
		if pick < cumulativeWeight {
			return variant.QueryParams
		}
	}

	return ""
}

func appendQueryParams(rawURL string, queryParams string) string {
	queryParams = strings.TrimPrefix(strings.TrimSpace(queryParams), "?")
	if queryParams == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = queryParams
	} else {
		parsed.RawQuery = parsed.RawQuery + "&" + queryParams
	}

	return parsed.String()
}

func percentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	if callback := jsonpCallbackFor(w); callback != "" {
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(payload)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "%s(%s);\n", callback, strings.TrimSpace(body.String()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func jsonpCallbackFor(w http.ResponseWriter) string {
	provider, ok := w.(interface{ jsonpCallback() string })
	if !ok {
		return ""
	}
	return provider.jsonpCallback()
}
