package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	defaultRunComparisonLimit = 5
	maxRunComparisonLimit     = 10
)

type runReportRecord struct {
	Run                createRunResponse         `json:"run"`
	Events             []runEventRecord          `json:"events"`
	ProfileArtifacts   []profileArtifactRecord   `json:"profileArtifacts"`
	SlowSamples        []runRequestSampleRecord  `json:"slowSamples"`
	ErrorSamples       []runRequestSampleRecord  `json:"errorSamples"`
	Alerts             []runReportAlert          `json:"alerts"`
	TargetMetrics      *runTargetMetricsReport   `json:"targetMetrics,omitempty"`
	TargetHealthChecks []targetHealthCheckResult `json:"targetHealthChecks,omitempty"`
	Summary            runReportSummary          `json:"summary"`
}

type runReportSummary struct {
	SuccessRatePercent   float64 `json:"successRatePercent"`
	ErrorRatePercent     float64 `json:"errorRatePercent"`
	EventCount           int     `json:"eventCount"`
	ProfileArtifactCount int     `json:"profileArtifactCount"`
	SlowSampleCount      int     `json:"slowSampleCount"`
	ErrorSampleCount     int     `json:"errorSampleCount"`
	AlertCount           int     `json:"alertCount"`
}

type runReportAlert struct {
	ID        string  `json:"id"`
	Severity  string  `json:"severity"`
	Kind      string  `json:"kind"`
	Metric    string  `json:"metric"`
	Threshold float64 `json:"threshold"`
	Observed  float64 `json:"observed"`
	Message   string  `json:"message"`
	EventID   string  `json:"eventId,omitempty"`
	CreatedAt string  `json:"createdAt,omitempty"`
}

type runComparisonReportRecord struct {
	GeneratedAt string                     `json:"generatedAt"`
	Runs        []runComparisonRunRecord   `json:"runs"`
	Summary     runComparisonReportSummary `json:"summary"`
}

type runComparisonRunRecord struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	ScenarioName       string  `json:"scenarioName"`
	TargetName         string  `json:"targetName"`
	Status             string  `json:"status"`
	TotalRequests      int     `json:"totalRequests"`
	SuccessRequests    int     `json:"successRequests"`
	FailedRequests     int     `json:"failedRequests"`
	SuccessRatePercent float64 `json:"successRatePercent"`
	ErrorRatePercent   float64 `json:"errorRatePercent"`
	DurationMs         float64 `json:"durationMs"`
	QPS                float64 `json:"qps"`
	AverageLatencyMs   float64 `json:"averageLatencyMs"`
	P95LatencyMs       float64 `json:"p95LatencyMs"`
	CreatedAt          string  `json:"createdAt"`
}

type runComparisonReportSummary struct {
	RunCount              int    `json:"runCount"`
	BestQPSRunID          string `json:"bestQpsRunId"`
	FastestP95RunID       string `json:"fastestP95RunId"`
	HighestErrorRateRunID string `json:"highestErrorRateRunId"`
}

type profileArtifactComparisonReport struct {
	GeneratedAt string                           `json:"generatedAt"`
	Groups      []profileArtifactComparisonGroup `json:"groups"`
	Summary     profileArtifactComparisonSummary `json:"summary"`
}

type profileArtifactComparisonGroup struct {
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
}

type profileArtifactComparisonSummary struct {
	RunCount       int   `json:"runCount"`
	ArtifactCount  int   `json:"artifactCount"`
	CollectedCount int   `json:"collectedCount"`
	FailedCount    int   `json:"failedCount"`
	TotalSizeBytes int64 `json:"totalSizeBytes"`
}

type processTrendComparisonReport struct {
	GeneratedAt string                        `json:"generatedAt"`
	Groups      []processTrendComparisonGroup `json:"groups"`
	Summary     processTrendComparisonSummary `json:"summary"`
}

type processTrendComparisonSummary struct {
	RunCount                int    `json:"runCount"`
	ProcessCount            int    `json:"processCount"`
	HighestCPUProcessKey    string `json:"highestCpuProcessKey"`
	HighestMemoryProcessKey string `json:"highestMemoryProcessKey"`
}

type processTrendComparisonGroup struct {
	Key                       string                      `json:"key"`
	TargetID                  string                      `json:"targetId"`
	TargetName                string                      `json:"targetName"`
	AgentID                   string                      `json:"agentId"`
	Name                      string                      `json:"name"`
	Cmdline                   string                      `json:"cmdline,omitempty"`
	RunCount                  int                         `json:"runCount"`
	SampleCount               int                         `json:"sampleCount"`
	CPUMaxPercent             float64                     `json:"cpuMaxPercent"`
	MemoryRSSMaxBytes         int64                       `json:"memoryRssMaxBytes"`
	FDMaxCount                int                         `json:"fdMaxCount"`
	ThreadMaxCount            int                         `json:"threadMaxCount"`
	LatestRunID               string                      `json:"latestRunId"`
	LatestPID                 int                         `json:"latestPid"`
	LatestCPUMaxPercent       float64                     `json:"latestCpuMaxPercent"`
	LatestMemoryRSSMaxBytes   int64                       `json:"latestMemoryRssMaxBytes"`
	PreviousRunID             string                      `json:"previousRunId,omitempty"`
	PreviousPID               int                         `json:"previousPid,omitempty"`
	PreviousCPUMaxPercent     float64                     `json:"previousCpuMaxPercent,omitempty"`
	PreviousMemoryRSSMaxBytes int64                       `json:"previousMemoryRssMaxBytes,omitempty"`
	CPUDeltaPercent           float64                     `json:"cpuDeltaPercent,omitempty"`
	MemoryRSSDeltaBytes       int64                       `json:"memoryRssDeltaBytes,omitempty"`
	LatestLastSeenAt          string                      `json:"latestLastSeenAt,omitempty"`
	Runs                      []processTrendComparisonRun `json:"runs"`
}

type processTrendComparisonRun struct {
	RunID             string  `json:"runId"`
	RunName           string  `json:"runName"`
	TargetID          string  `json:"targetId"`
	TargetName        string  `json:"targetName"`
	AgentID           string  `json:"agentId"`
	PID               int     `json:"pid"`
	SampleCount       int     `json:"sampleCount"`
	CPUMaxPercent     float64 `json:"cpuMaxPercent"`
	MemoryRSSMaxBytes int64   `json:"memoryRssMaxBytes"`
	FDMaxCount        int     `json:"fdMaxCount"`
	ThreadMaxCount    int     `json:"threadMaxCount"`
	FirstSeenAt       string  `json:"firstSeenAt"`
	LastSeenAt        string  `json:"lastSeenAt"`
	CreatedAt         string  `json:"createdAt"`
}

type runTargetMetricsReport struct {
	TargetID                string                 `json:"targetId"`
	TargetName              string                 `json:"targetName"`
	AgentIDs                []string               `json:"agentIds"`
	From                    string                 `json:"from"`
	To                      string                 `json:"to"`
	SampleCount             int                    `json:"sampleCount"`
	CPUMaxPercent           float64                `json:"cpuMaxPercent"`
	MemoryMaxPercent        float64                `json:"memoryMaxPercent"`
	DiskReadMaxBytesPerSec  float64                `json:"diskReadMaxBytesPerSec"`
	DiskWriteMaxBytesPerSec float64                `json:"diskWriteMaxBytesPerSec"`
	NetworkRxMaxBytesPerSec float64                `json:"networkRxMaxBytesPerSec"`
	NetworkTxMaxBytesPerSec float64                `json:"networkTxMaxBytesPerSec"`
	ProcessMatch            targetProcessMatch     `json:"processMatch"`
	MetricThresholds        targetMetricThresholds `json:"metricThresholds"`
	Samples                 []agentMetricsRecord   `json:"samples"`
	LatestProcessSnapshot   []agentProcessMetric   `json:"latestProcessSnapshot"`
	ProcessTrends           []runProcessTrend      `json:"processTrends"`
}

type runProcessTrend struct {
	AgentID           string  `json:"agentId"`
	PID               int     `json:"pid"`
	Name              string  `json:"name"`
	Cmdline           string  `json:"cmdline,omitempty"`
	SampleCount       int     `json:"sampleCount"`
	CPUMaxPercent     float64 `json:"cpuMaxPercent"`
	MemoryRSSMaxBytes int64   `json:"memoryRssMaxBytes"`
	FDMaxCount        int     `json:"fdMaxCount"`
	ThreadMaxCount    int     `json:"threadMaxCount"`
	FirstSeenAt       string  `json:"firstSeenAt"`
	LastSeenAt        string  `json:"lastSeenAt"`
}

func handleGetRunReport(runStore *runHistoryStore, artifactStore *profileArtifactStore, targetStore *targetStore, agentStore *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "id")
		run, found, err := runStore.get(runID)
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

		report, err := loadRunReport(runStore, artifactStore, targetStore, agentStore, run)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func handleGetRunComparisonReport(runStore *runHistoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, status, err := comparisonRunsFromRequest(runStore, r)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if len(runs) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runs not found"})
			return
		}

		writeJSON(w, http.StatusOK, buildRunComparisonReport(runs))
	}
}

func handleGetProfileArtifactComparisonReport(runStore *runHistoryStore, artifactStore *profileArtifactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, status, err := comparisonRunsFromRequest(runStore, r)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if len(runs) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runs not found"})
			return
		}

		artifacts, err := artifactStore.listByRunIDs(runIDsFromResponses(runs))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, buildProfileArtifactComparisonReport(runs, artifacts))
	}
}

func handleGetProcessTrendComparisonReport(runStore *runHistoryStore, targetStore *targetStore, agentStore *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, status, err := comparisonRunsFromRequest(runStore, r)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if len(runs) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runs not found"})
			return
		}

		report, err := buildProcessTrendComparisonReport(runs, targetStore, agentStore)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func handleGetLatestRunReport(runStore *runHistoryStore, artifactStore *profileArtifactStore, targetStore *targetStore, agentStore *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := runListFilterFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		runs, err := runStore.listFiltered(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if len(runs) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}

		report, err := loadRunReport(runStore, artifactStore, targetStore, agentStore, runs[0])
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func comparisonRunsFromRequest(runStore *runHistoryStore, r *http.Request) ([]createRunResponse, int, error) {
	runIDs := splitRunIDs(r.URL.Query().Get("runIds"))
	if len(runIDs) > 0 {
		if len(runIDs) < 2 {
			return nil, http.StatusBadRequest, fmt.Errorf("at least two runIds are required")
		}
		runs := make([]createRunResponse, 0, len(runIDs))
		for _, runID := range runIDs {
			run, found, err := runStore.get(runID)
			if err != nil {
				return nil, http.StatusInternalServerError, err
			}
			if !found {
				return nil, http.StatusNotFound, fmt.Errorf("run %s not found", runID)
			}
			if err := requireWorkspaceScopeForRecord(r, run.ProjectID, run.Environment); err != nil {
				return nil, http.StatusForbidden, err
			}
			runs = append(runs, run)
		}
		return runs, http.StatusOK, nil
	}

	limit, err := comparisonLimitFromRequest(r)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	filter, err := runListFilterFromRequest(r)
	if err != nil {
		return nil, http.StatusForbidden, err
	}
	runs, err := runStore.listFiltered(filter)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, http.StatusOK, nil
}

func splitRunIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	runIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		runID := strings.TrimSpace(part)
		if runID != "" {
			runIDs = append(runIDs, runID)
		}
	}
	return runIDs
}

func runIDsFromResponses(runs []createRunResponse) []string {
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runID := strings.TrimSpace(run.ID)
		if runID != "" {
			runIDs = append(runIDs, runID)
		}
	}
	return runIDs
}

func comparisonLimitFromRequest(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultRunComparisonLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if limit > maxRunComparisonLimit {
		return maxRunComparisonLimit, nil
	}
	return limit, nil
}

func buildRunComparisonReport(runs []createRunResponse) runComparisonReportRecord {
	comparisonRuns := make([]runComparisonRunRecord, 0, len(runs))
	for _, run := range runs {
		comparisonRuns = append(comparisonRuns, runComparisonRecordFromRun(run))
	}
	return runComparisonReportRecord{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Runs:        comparisonRuns,
		Summary:     buildRunComparisonSummary(comparisonRuns),
	}
}

func runComparisonRecordFromRun(run createRunResponse) runComparisonRunRecord {
	successRate, errorRate := runSuccessErrorRates(run)
	return runComparisonRunRecord{
		ID:                 run.ID,
		Name:               run.Name,
		ScenarioName:       run.ScenarioName,
		TargetName:         run.TargetName,
		Status:             run.Status,
		TotalRequests:      run.TotalRequests,
		SuccessRequests:    run.SuccessRequests,
		FailedRequests:     run.FailedRequests,
		SuccessRatePercent: successRate,
		ErrorRatePercent:   errorRate,
		DurationMs:         run.DurationMs,
		QPS:                run.QPS,
		AverageLatencyMs:   run.AverageLatencyMs,
		P95LatencyMs:       run.P95LatencyMs,
		CreatedAt:          run.CreatedAt,
	}
}

func buildRunComparisonSummary(runs []runComparisonRunRecord) runComparisonReportSummary {
	summary := runComparisonReportSummary{RunCount: len(runs)}
	if len(runs) == 0 {
		return summary
	}

	bestQPS := runs[0]
	fastestP95 := runs[0]
	highestErrorRate := runs[0]
	for _, run := range runs[1:] {
		if run.QPS > bestQPS.QPS {
			bestQPS = run
		}
		if run.P95LatencyMs < fastestP95.P95LatencyMs {
			fastestP95 = run
		}
		if run.ErrorRatePercent > highestErrorRate.ErrorRatePercent {
			highestErrorRate = run
		}
	}
	summary.BestQPSRunID = bestQPS.ID
	summary.FastestP95RunID = fastestP95.ID
	summary.HighestErrorRateRunID = highestErrorRate.ID
	return summary
}

func buildProfileArtifactComparisonReport(runs []createRunResponse, artifacts []profileArtifactRecord) profileArtifactComparisonReport {
	runOrder := make(map[string]int, len(runs))
	for index, run := range runs {
		runOrder[run.ID] = index
	}

	groupsByType := map[string]*profileArtifactComparisonGroup{}
	summary := profileArtifactComparisonSummary{RunCount: len(runs)}
	for _, artifact := range artifacts {
		if _, ok := runOrder[artifact.RunID]; !ok {
			continue
		}
		profileType := strings.TrimSpace(artifact.ProfileType)
		if profileType == "" {
			profileType = "unknown"
		}
		group, ok := groupsByType[profileType]
		if !ok {
			group = &profileArtifactComparisonGroup{ProfileType: profileType}
			groupsByType[profileType] = group
		}

		group.Artifacts = append(group.Artifacts, artifact)
		group.ArtifactCount++
		group.TotalSizeBytes += artifact.SizeBytes
		summary.ArtifactCount++
		summary.TotalSizeBytes += artifact.SizeBytes
		if artifact.Status == "collected" {
			group.CollectedCount++
			summary.CollectedCount++
		}
		if artifact.Status == "failed" {
			group.FailedCount++
			summary.FailedCount++
		}
	}

	groups := make([]profileArtifactComparisonGroup, 0, len(groupsByType))
	for _, group := range groupsByType {
		sort.SliceStable(group.Artifacts, func(left, right int) bool {
			leftOrder := runOrder[group.Artifacts[left].RunID]
			rightOrder := runOrder[group.Artifacts[right].RunID]
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
			if group.Artifacts[left].StartedAt != group.Artifacts[right].StartedAt {
				return group.Artifacts[left].StartedAt > group.Artifacts[right].StartedAt
			}
			return group.Artifacts[left].ID < group.Artifacts[right].ID
		})
		if len(group.Artifacts) > 0 {
			latest := group.Artifacts[0]
			group.LatestRunID = latest.RunID
			group.LatestArtifactID = latest.ID
			group.LatestSizeBytes = latest.SizeBytes
			group.LatestStatus = latest.Status
		}
		if len(group.Artifacts) > 1 {
			previous := group.Artifacts[1]
			group.PreviousSizeBytes = previous.SizeBytes
			group.PreviousStatus = previous.Status
			group.SizeDeltaBytes = group.LatestSizeBytes - previous.SizeBytes
			if previous.SizeBytes > 0 {
				group.SizeDeltaPercent = (float64(group.SizeDeltaBytes) / float64(previous.SizeBytes)) * 100
			}
			group.StatusChanged = group.LatestStatus != "" && previous.Status != "" && group.LatestStatus != previous.Status
		}
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(left, right int) bool {
		return groups[left].ProfileType < groups[right].ProfileType
	})

	return profileArtifactComparisonReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Groups:      groups,
		Summary:     summary,
	}
}

func buildProcessTrendComparisonReport(runs []createRunResponse, targetStore *targetStore, agentStore *agentStore) (processTrendComparisonReport, error) {
	runOrder := make(map[string]int, len(runs))
	for index, run := range runs {
		runOrder[run.ID] = index
	}

	groupsByKey := map[string]*processTrendComparisonGroup{}
	for _, run := range runs {
		targetMetrics, err := loadRunTargetMetricsReport(targetStore, agentStore, run)
		if err != nil {
			return processTrendComparisonReport{}, err
		}
		if targetMetrics == nil || len(targetMetrics.ProcessTrends) == 0 {
			continue
		}
		for _, trend := range targetMetrics.ProcessTrends {
			name := processTrendComparisonName(trend.Name)
			cmdline := strings.TrimSpace(trend.Cmdline)
			matchKey := processTrendComparisonMatchKey(targetMetrics.TargetID, trend.AgentID, name, cmdline)
			group, ok := groupsByKey[matchKey]
			if !ok {
				group = &processTrendComparisonGroup{
					Key:        processTrendComparisonDisplayKey(targetMetrics.TargetID, trend.AgentID, name, cmdline),
					TargetID:   targetMetrics.TargetID,
					TargetName: targetMetrics.TargetName,
					AgentID:    trend.AgentID,
					Name:       name,
					Cmdline:    cmdline,
				}
				groupsByKey[matchKey] = group
			}

			group.SampleCount += trend.SampleCount
			if trend.CPUMaxPercent > group.CPUMaxPercent {
				group.CPUMaxPercent = trend.CPUMaxPercent
			}
			if trend.MemoryRSSMaxBytes > group.MemoryRSSMaxBytes {
				group.MemoryRSSMaxBytes = trend.MemoryRSSMaxBytes
			}
			if trend.FDMaxCount > group.FDMaxCount {
				group.FDMaxCount = trend.FDMaxCount
			}
			if trend.ThreadMaxCount > group.ThreadMaxCount {
				group.ThreadMaxCount = trend.ThreadMaxCount
			}
			mergeProcessTrendComparisonRun(group, processTrendComparisonRun{
				RunID:             run.ID,
				RunName:           run.Name,
				TargetID:          targetMetrics.TargetID,
				TargetName:        targetMetrics.TargetName,
				AgentID:           trend.AgentID,
				PID:               trend.PID,
				SampleCount:       trend.SampleCount,
				CPUMaxPercent:     trend.CPUMaxPercent,
				MemoryRSSMaxBytes: trend.MemoryRSSMaxBytes,
				FDMaxCount:        trend.FDMaxCount,
				ThreadMaxCount:    trend.ThreadMaxCount,
				FirstSeenAt:       trend.FirstSeenAt,
				LastSeenAt:        trend.LastSeenAt,
				CreatedAt:         run.CreatedAt,
			})
		}
	}

	groups := make([]processTrendComparisonGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		sort.SliceStable(group.Runs, func(left, right int) bool {
			leftOrder := runOrder[group.Runs[left].RunID]
			rightOrder := runOrder[group.Runs[right].RunID]
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
			return group.Runs[left].LastSeenAt > group.Runs[right].LastSeenAt
		})
		group.RunCount = len(group.Runs)
		if len(group.Runs) > 0 {
			latest := group.Runs[0]
			group.LatestRunID = latest.RunID
			group.LatestPID = latest.PID
			group.LatestCPUMaxPercent = latest.CPUMaxPercent
			group.LatestMemoryRSSMaxBytes = latest.MemoryRSSMaxBytes
			group.LatestLastSeenAt = latest.LastSeenAt
		}
		if len(group.Runs) > 1 {
			previous := group.Runs[1]
			group.PreviousRunID = previous.RunID
			group.PreviousPID = previous.PID
			group.PreviousCPUMaxPercent = previous.CPUMaxPercent
			group.PreviousMemoryRSSMaxBytes = previous.MemoryRSSMaxBytes
			group.CPUDeltaPercent = group.LatestCPUMaxPercent - previous.CPUMaxPercent
			group.MemoryRSSDeltaBytes = group.LatestMemoryRSSMaxBytes - previous.MemoryRSSMaxBytes
		}
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(left, right int) bool {
		if groups[left].CPUMaxPercent != groups[right].CPUMaxPercent {
			return groups[left].CPUMaxPercent > groups[right].CPUMaxPercent
		}
		if groups[left].MemoryRSSMaxBytes != groups[right].MemoryRSSMaxBytes {
			return groups[left].MemoryRSSMaxBytes > groups[right].MemoryRSSMaxBytes
		}
		return groups[left].Key < groups[right].Key
	})

	summary := processTrendComparisonSummary{
		RunCount:     len(runs),
		ProcessCount: len(groups),
	}
	for index, group := range groups {
		if index == 0 || group.CPUMaxPercent > groups[index-1].CPUMaxPercent {
			summary.HighestCPUProcessKey = group.Key
		}
		if summary.HighestMemoryProcessKey == "" || group.MemoryRSSMaxBytes > memoryRSSMaxByKey(groups, summary.HighestMemoryProcessKey) {
			summary.HighestMemoryProcessKey = group.Key
		}
	}

	return processTrendComparisonReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Groups:      groups,
		Summary:     summary,
	}, nil
}

func processTrendComparisonName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func processTrendComparisonMatchKey(targetID string, agentID string, name string, cmdline string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(targetID)),
		strings.ToLower(strings.TrimSpace(agentID)),
		strings.ToLower(strings.TrimSpace(name)),
		strings.ToLower(strings.TrimSpace(cmdline)),
	}
	return strings.Join(parts, "\x00")
}

func processTrendComparisonDisplayKey(targetID string, agentID string, name string, cmdline string) string {
	return strings.Join([]string{
		processTrendComparisonKeyPart(targetID, "unknown-target"),
		processTrendComparisonKeyPart(agentID, "unknown-agent"),
		processTrendComparisonKeyPart(name, "unknown-process"),
		strings.TrimSpace(cmdline),
	}, "|")
}

func processTrendComparisonKeyPart(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func mergeProcessTrendComparisonRun(group *processTrendComparisonGroup, next processTrendComparisonRun) {
	for index := range group.Runs {
		if group.Runs[index].RunID == next.RunID {
			current := &group.Runs[index]
			current.SampleCount += next.SampleCount
			current.LastSeenAt = maxTimestampString(current.LastSeenAt, next.LastSeenAt)
			if current.FirstSeenAt == "" || (next.FirstSeenAt != "" && next.FirstSeenAt < current.FirstSeenAt) {
				current.FirstSeenAt = next.FirstSeenAt
			}
			if next.CPUMaxPercent > current.CPUMaxPercent {
				current.CPUMaxPercent = next.CPUMaxPercent
				current.PID = next.PID
			}
			if next.MemoryRSSMaxBytes > current.MemoryRSSMaxBytes {
				current.MemoryRSSMaxBytes = next.MemoryRSSMaxBytes
				current.PID = next.PID
			}
			if next.FDMaxCount > current.FDMaxCount {
				current.FDMaxCount = next.FDMaxCount
			}
			if next.ThreadMaxCount > current.ThreadMaxCount {
				current.ThreadMaxCount = next.ThreadMaxCount
			}
			return
		}
	}
	group.Runs = append(group.Runs, next)
}

func maxTimestampString(left string, right string) string {
	if strings.TrimSpace(left) == "" || right > left {
		return right
	}
	return left
}

func memoryRSSMaxByKey(groups []processTrendComparisonGroup, key string) int64 {
	for _, group := range groups {
		if group.Key == key {
			return group.MemoryRSSMaxBytes
		}
	}
	return 0
}

func loadRunReport(runStore *runHistoryStore, artifactStore *profileArtifactStore, targetStore *targetStore, agentStore *agentStore, run createRunResponse) (runReportRecord, error) {
	events, err := runStore.listEvents(run.ID)
	if err != nil {
		return runReportRecord{}, err
	}
	artifacts, err := artifactStore.listByRun(run.ID)
	if err != nil {
		return runReportRecord{}, err
	}
	slowSamples, err := runStore.listRequestSamples(run.ID, "slow")
	if err != nil {
		return runReportRecord{}, err
	}
	errorSamples, err := runStore.listRequestSamples(run.ID, "error")
	if err != nil {
		return runReportRecord{}, err
	}
	targetMetrics, err := loadRunTargetMetricsReport(targetStore, agentStore, run)
	if err != nil {
		return runReportRecord{}, err
	}
	targetHealthChecks, err := loadRunTargetHealthChecks(targetStore, run)
	if err != nil {
		return runReportRecord{}, err
	}

	alerts := buildRunReportAlerts(run, events, targetMetrics)
	return runReportRecord{
		Run:                run,
		Events:             events,
		ProfileArtifacts:   artifacts,
		SlowSamples:        slowSamples,
		ErrorSamples:       errorSamples,
		Alerts:             alerts,
		TargetMetrics:      targetMetrics,
		TargetHealthChecks: targetHealthChecks,
		Summary:            buildRunReportSummary(run, events, artifacts, slowSamples, errorSamples, alerts),
	}, nil
}

func loadRunTargetHealthChecks(targetStore *targetStore, run createRunResponse) ([]targetHealthCheckResult, error) {
	if strings.TrimSpace(run.TargetID) == "" {
		return nil, nil
	}
	return targetStore.listHealthChecks(run.TargetID, 5)
}

func loadRunTargetMetricsReport(targetStore *targetStore, agentStore *agentStore, run createRunResponse) (*runTargetMetricsReport, error) {
	if strings.TrimSpace(run.TargetID) == "" {
		return nil, nil
	}
	target, found, err := targetStore.get(run.TargetID)
	if err != nil {
		return nil, err
	}
	if !found || len(target.AgentIDs) == 0 {
		return nil, nil
	}

	from, to, ok := runMetricsWindow(run)
	if !ok {
		return nil, nil
	}
	report := &runTargetMetricsReport{
		TargetID:         target.ID,
		TargetName:       target.Name,
		AgentIDs:         append([]string(nil), target.AgentIDs...),
		From:             from,
		To:               to,
		ProcessMatch:     target.ProcessMatch,
		MetricThresholds: target.MetricThresholds,
	}
	for _, agentID := range target.AgentIDs {
		metrics, err := agentStore.listMetrics(agentID, from, to, 1000)
		if err != nil {
			return nil, err
		}
		mergeAgentMetricsIntoRunTargetReport(report, metrics)
	}
	return report, nil
}

func runMetricsWindow(run createRunResponse) (string, string, bool) {
	startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(run.CreatedAt))
	if err != nil {
		return "", "", false
	}
	duration := time.Duration(run.DurationMs * float64(time.Millisecond))
	if duration < 0 {
		duration = 0
	}
	finishedAt := startedAt.Add(duration)
	return startedAt.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano), true
}

func mergeAgentMetricsIntoRunTargetReport(report *runTargetMetricsReport, metrics []agentMetricsRecord) {
	for _, sample := range metrics {
		report.SampleCount++
		report.Samples = append(report.Samples, sample)
		if sample.CPUUsagePercent > report.CPUMaxPercent {
			report.CPUMaxPercent = sample.CPUUsagePercent
		}
		if sample.MemoryUsagePercent > report.MemoryMaxPercent {
			report.MemoryMaxPercent = sample.MemoryUsagePercent
		}
		if sample.DiskReadBytesPerSec > report.DiskReadMaxBytesPerSec {
			report.DiskReadMaxBytesPerSec = sample.DiskReadBytesPerSec
		}
		if sample.DiskWriteBytesPerSec > report.DiskWriteMaxBytesPerSec {
			report.DiskWriteMaxBytesPerSec = sample.DiskWriteBytesPerSec
		}
		if sample.NetworkRxBytesPerSec > report.NetworkRxMaxBytesPerSec {
			report.NetworkRxMaxBytesPerSec = sample.NetworkRxBytesPerSec
		}
		if sample.NetworkTxBytesPerSec > report.NetworkTxMaxBytesPerSec {
			report.NetworkTxMaxBytesPerSec = sample.NetworkTxBytesPerSec
		}
		filteredProcesses := filterAgentProcesses(sample.Processes, report.ProcessMatch)
		report.LatestProcessSnapshot = filteredProcesses
		mergeProcessTrendsIntoRunTargetReport(report, sample, filteredProcesses)
	}
}

func mergeProcessTrendsIntoRunTargetReport(report *runTargetMetricsReport, sample agentMetricsRecord, processes []agentProcessMetric) {
	for _, process := range processes {
		trendIndex := findProcessTrendIndex(report.ProcessTrends, sample.AgentID, process)
		if trendIndex < 0 {
			report.ProcessTrends = append(report.ProcessTrends, runProcessTrend{
				AgentID:           sample.AgentID,
				PID:               process.PID,
				Name:              process.Name,
				Cmdline:           process.Cmdline,
				SampleCount:       1,
				CPUMaxPercent:     process.CPUUsagePercent,
				MemoryRSSMaxBytes: process.MemoryRSSBytes,
				FDMaxCount:        process.FDCount,
				ThreadMaxCount:    process.ThreadCount,
				FirstSeenAt:       sample.CollectedAt,
				LastSeenAt:        sample.CollectedAt,
			})
			continue
		}

		trend := &report.ProcessTrends[trendIndex]
		trend.SampleCount++
		trend.LastSeenAt = sample.CollectedAt
		if process.CPUUsagePercent > trend.CPUMaxPercent {
			trend.CPUMaxPercent = process.CPUUsagePercent
		}
		if process.MemoryRSSBytes > trend.MemoryRSSMaxBytes {
			trend.MemoryRSSMaxBytes = process.MemoryRSSBytes
		}
		if process.FDCount > trend.FDMaxCount {
			trend.FDMaxCount = process.FDCount
		}
		if process.ThreadCount > trend.ThreadMaxCount {
			trend.ThreadMaxCount = process.ThreadCount
		}
	}
}

func findProcessTrendIndex(trends []runProcessTrend, agentID string, process agentProcessMetric) int {
	for index, trend := range trends {
		if trend.AgentID == agentID && trend.PID == process.PID && trend.Name == process.Name && trend.Cmdline == process.Cmdline {
			return index
		}
	}
	return -1
}

func filterAgentProcesses(processes []agentProcessMetric, processMatch targetProcessMatch) []agentProcessMetric {
	cmdlineContains := strings.ToLower(strings.TrimSpace(processMatch.CmdlineContains))
	if cmdlineContains != "" {
		filteredProcesses := make([]agentProcessMetric, 0, len(processes))
		for _, process := range processes {
			cmdline := strings.ToLower(strings.TrimSpace(process.Cmdline))
			if strings.Contains(cmdline, cmdlineContains) {
				filteredProcesses = append(filteredProcesses, process)
			}
		}
		return filteredProcesses
	}

	name := strings.ToLower(strings.TrimSpace(processMatch.Name))
	if name == "" {
		return append([]agentProcessMetric(nil), processes...)
	}
	filteredProcesses := make([]agentProcessMetric, 0, len(processes))
	for _, process := range processes {
		processName := strings.ToLower(strings.TrimSpace(process.Name))
		if strings.Contains(processName, name) {
			filteredProcesses = append(filteredProcesses, process)
		}
	}
	return filteredProcesses
}

func buildRunReportAlerts(run createRunResponse, events []runEventRecord, targetMetrics *runTargetMetricsReport) []runReportAlert {
	alerts := []runReportAlert{}

	abortedEvent, found := latestRunEventOfType(events, "run_aborted")
	if run.Status == "aborted" || found {
		guardrailAlerts := []runReportAlert{}
		_, errorRate := runSuccessErrorRates(run)
		if run.MaxErrorRatePercent > 0 && errorRate > run.MaxErrorRatePercent {
			guardrailAlerts = append(guardrailAlerts, runReportAlert{
				ID:        run.ID + "-guardrail-error-rate",
				Severity:  "critical",
				Kind:      "guardrail",
				Metric:    "error_rate_percent",
				Threshold: run.MaxErrorRatePercent,
				Observed:  errorRate,
				Message:   fmt.Sprintf("Error rate guardrail exceeded: %.1f%% > %.1f%%", errorRate, run.MaxErrorRatePercent),
				EventID:   abortedEvent.ID,
				CreatedAt: abortedEvent.CreatedAt,
			})
		}
		if run.MaxP95LatencyMs > 0 && run.P95LatencyMs > run.MaxP95LatencyMs {
			guardrailAlerts = append(guardrailAlerts, runReportAlert{
				ID:        run.ID + "-guardrail-p95-latency",
				Severity:  "critical",
				Kind:      "guardrail",
				Metric:    "p95_latency_ms",
				Threshold: run.MaxP95LatencyMs,
				Observed:  run.P95LatencyMs,
				Message:   fmt.Sprintf("P95 latency guardrail exceeded: %.2f ms > %.2f ms", run.P95LatencyMs, run.MaxP95LatencyMs),
				EventID:   abortedEvent.ID,
				CreatedAt: abortedEvent.CreatedAt,
			})
		}
		if len(guardrailAlerts) == 0 && found {
			guardrailAlerts = append(guardrailAlerts, runReportAlert{
				ID:        run.ID + "-guardrail-aborted",
				Severity:  "critical",
				Kind:      "guardrail",
				Metric:    "run_status",
				Message:   strings.TrimSpace(abortedEvent.Message),
				EventID:   abortedEvent.ID,
				CreatedAt: abortedEvent.CreatedAt,
			})
		}
		alerts = append(alerts, guardrailAlerts...)
	}
	alerts = append(alerts, buildTargetMetricThresholdAlerts(run, targetMetrics)...)
	return alerts
}

func buildTargetMetricThresholdAlerts(run createRunResponse, targetMetrics *runTargetMetricsReport) []runReportAlert {
	if targetMetrics == nil || targetMetrics.SampleCount == 0 {
		return nil
	}
	thresholds := targetMetrics.MetricThresholds
	alerts := []runReportAlert{}
	addAlert := func(suffix string, metric string, threshold float64, observed float64, message string) {
		if threshold <= 0 || observed <= threshold {
			return
		}
		alerts = append(alerts, runReportAlert{
			ID:        run.ID + "-target-" + suffix,
			Severity:  "warning",
			Kind:      "target_metric",
			Metric:    metric,
			Threshold: threshold,
			Observed:  observed,
			Message:   message,
			CreatedAt: targetMetrics.To,
		})
	}
	addAlert("cpu-percent", "target_cpu_percent", thresholds.CPUMaxPercent, targetMetrics.CPUMaxPercent,
		fmt.Sprintf("Target CPU threshold exceeded: %.1f%% > %.1f%%", targetMetrics.CPUMaxPercent, thresholds.CPUMaxPercent))
	addAlert("memory-percent", "target_memory_percent", thresholds.MemoryMaxPercent, targetMetrics.MemoryMaxPercent,
		fmt.Sprintf("Target memory threshold exceeded: %.1f%% > %.1f%%", targetMetrics.MemoryMaxPercent, thresholds.MemoryMaxPercent))
	addAlert("disk-read-bytes-per-sec", "target_disk_read_bytes_per_sec", thresholds.DiskReadMaxBytesPerSec, targetMetrics.DiskReadMaxBytesPerSec,
		fmt.Sprintf("Target disk read threshold exceeded: %.0f B/s > %.0f B/s", targetMetrics.DiskReadMaxBytesPerSec, thresholds.DiskReadMaxBytesPerSec))
	addAlert("disk-write-bytes-per-sec", "target_disk_write_bytes_per_sec", thresholds.DiskWriteMaxBytesPerSec, targetMetrics.DiskWriteMaxBytesPerSec,
		fmt.Sprintf("Target disk write threshold exceeded: %.0f B/s > %.0f B/s", targetMetrics.DiskWriteMaxBytesPerSec, thresholds.DiskWriteMaxBytesPerSec))
	addAlert("network-rx-bytes-per-sec", "target_network_rx_bytes_per_sec", thresholds.NetworkRxMaxBytesPerSec, targetMetrics.NetworkRxMaxBytesPerSec,
		fmt.Sprintf("Target network RX threshold exceeded: %.0f B/s > %.0f B/s", targetMetrics.NetworkRxMaxBytesPerSec, thresholds.NetworkRxMaxBytesPerSec))
	addAlert("network-tx-bytes-per-sec", "target_network_tx_bytes_per_sec", thresholds.NetworkTxMaxBytesPerSec, targetMetrics.NetworkTxMaxBytesPerSec,
		fmt.Sprintf("Target network TX threshold exceeded: %.0f B/s > %.0f B/s", targetMetrics.NetworkTxMaxBytesPerSec, thresholds.NetworkTxMaxBytesPerSec))
	return alerts
}

func latestRunEventOfType(events []runEventRecord, eventType string) (runEventRecord, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Type == eventType {
			return events[index], true
		}
	}
	return runEventRecord{}, false
}

func buildRunReportSummary(run createRunResponse, events []runEventRecord, artifacts []profileArtifactRecord, slowSamples []runRequestSampleRecord, errorSamples []runRequestSampleRecord, alerts []runReportAlert) runReportSummary {
	summary := runReportSummary{
		EventCount:           len(events),
		ProfileArtifactCount: len(artifacts),
		SlowSampleCount:      len(slowSamples),
		ErrorSampleCount:     len(errorSamples),
		AlertCount:           len(alerts),
	}
	successRate, errorRate := runSuccessErrorRates(run)
	if successRate == 0 && errorRate == 0 {
		return summary
	}
	summary.SuccessRatePercent = successRate
	summary.ErrorRatePercent = errorRate
	return summary
}

func runSuccessErrorRates(run createRunResponse) (float64, float64) {
	totalCompleted := run.SuccessRequests + run.FailedRequests
	if totalCompleted == 0 {
		return 0, 0
	}
	successRate := (float64(run.SuccessRequests) / float64(totalCompleted)) * 100
	errorRate := (float64(run.FailedRequests) / float64(totalCompleted)) * 100
	return successRate, errorRate
}
