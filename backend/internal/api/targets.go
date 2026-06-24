package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type targetRecord struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	ProjectID        string                   `json:"projectId"`
	BaseURL          string                   `json:"baseUrl"`
	Environment      string                   `json:"environment"`
	AgentIDs         []string                 `json:"agentIds"`
	ProfileEndpoint  string                   `json:"profileEndpoint"`
	ProcessMatch     targetProcessMatch       `json:"processMatch"`
	HealthCheck      targetHealthCheck        `json:"healthCheck"`
	LastHealthCheck  *targetHealthCheckResult `json:"lastHealthCheck,omitempty"`
	MetricThresholds targetMetricThresholds   `json:"metricThresholds"`
	CreatedAt        string                   `json:"createdAt"`
	UpdatedAt        string                   `json:"updatedAt"`
}

type targetListFilter struct {
	ProjectID   string
	Environment string
}

type targetProcessMatch struct {
	Name            string `json:"name"`
	CmdlineContains string `json:"cmdlineContains"`
}

type targetHealthCheck struct {
	Enabled        bool   `json:"enabled"`
	Path           string `json:"path"`
	ExpectedStatus int    `json:"expectedStatus"`
	TimeoutMs      int    `json:"timeoutMs"`
}

type targetMetricThresholds struct {
	CPUMaxPercent           float64 `json:"cpuMaxPercent"`
	MemoryMaxPercent        float64 `json:"memoryMaxPercent"`
	DiskReadMaxBytesPerSec  float64 `json:"diskReadMaxBytesPerSec"`
	DiskWriteMaxBytesPerSec float64 `json:"diskWriteMaxBytesPerSec"`
	NetworkRxMaxBytesPerSec float64 `json:"networkRxMaxBytesPerSec"`
	NetworkTxMaxBytesPerSec float64 `json:"networkTxMaxBytesPerSec"`
}

type targetHealthCheckResult struct {
	TargetID       string  `json:"targetId"`
	TargetName     string  `json:"targetName"`
	Status         string  `json:"status"`
	Source         string  `json:"source"`
	URL            string  `json:"url"`
	ExpectedStatus int     `json:"expectedStatus"`
	ObservedStatus int     `json:"observedStatus"`
	LatencyMs      float64 `json:"latencyMs"`
	Error          string  `json:"error,omitempty"`
	CheckedAt      string  `json:"checkedAt"`
}

type targetStore struct {
	path string
	mu   sync.Mutex
}

const targetHealthCheckHistoryRetention = 200

type targetScanner interface {
	Scan(dest ...any) error
}

func newTargetStoreFromEnv() *targetStore {
	return &targetStore{path: scenarioDatabasePathFromEnv()}
}

func handleListTargets(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := targetListFilterFromRequest(r)
		if err := applyWorkspaceScopeToFilter(r, &filter.ProjectID, &filter.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		targets, err := store.list(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, targets)
	}
}

func targetListFilterFromRequest(r *http.Request) targetListFilter {
	query := r.URL.Query()
	return targetListFilter{
		ProjectID:   strings.TrimSpace(query.Get("projectId")),
		Environment: strings.TrimSpace(query.Get("environment")),
	}
}

func handleCreateTarget(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input targetRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		if err := applyWorkspaceScopeToRecord(r, &input.ProjectID, &input.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		target, err := store.create(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, target)
	}
}

func handleGetTarget(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, target.ProjectID, target.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, target)
	}
}

func handleUpdateTarget(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input targetRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}

		existingTarget, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, existingTarget.ProjectID, existingTarget.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if err := applyWorkspaceScopeToRecord(r, &input.ProjectID, &input.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		target, found, err := store.update(chi.URLParam(r, "id"), input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}

		writeJSON(w, http.StatusOK, target)
	}
}

func handleDeleteTarget(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, target.ProjectID, target.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		found, err = store.delete(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleTargetHealthCheck(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, target.ProjectID, target.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		result := checkTargetHealth(r.Context(), target)
		if err := store.updateLastHealthCheck(target.ID, result); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleListTargetHealthChecks(store *targetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID := strings.TrimSpace(chi.URLParam(r, "id"))
		target, found, err := store.get(targetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, target.ProjectID, target.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		limit := 20
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil || parsedLimit <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = parsedLimit
			if limit > 100 {
				limit = 100
			}
		}

		history, err := store.listHealthChecks(targetID, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, history)
	}
}

func (store *targetStore) create(input targetRecord) (targetRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return targetRecord{}, err
	}
	defer db.Close()

	input = normalizeTarget(input)
	if err := validateTarget(db, input); err != nil {
		return targetRecord{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.ID = newResourceID("target")
	input.CreatedAt = now
	input.UpdatedAt = now

	agentIDs, processMatch, healthCheck, metricThresholds, err := encodeTargetCollections(input)
	if err != nil {
		return targetRecord{}, err
	}
	_, err = db.Exec(`
		INSERT INTO targets (
			id, name, project_id, base_url, environment, agent_ids, profile_endpoint,
			process_match, health_check, metric_thresholds, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.ID, input.Name, input.ProjectID, input.BaseURL, input.Environment, agentIDs, input.ProfileEndpoint,
		processMatch, healthCheck, metricThresholds, input.CreatedAt, input.UpdatedAt)
	if err != nil {
		return targetRecord{}, err
	}
	return input, nil
}

func (store *targetStore) list(filter targetListFilter) ([]targetRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT id, name, project_id, base_url, environment, agent_ids, profile_endpoint,
			process_match, health_check, last_health_check, metric_thresholds, created_at, updated_at
		FROM targets
	`
	conditions := []string{}
	args := []any{}
	if filter.ProjectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.Environment != "" {
		conditions = append(conditions, "environment = ?")
		args = append(args, filter.Environment)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := []targetRecord{}
	for rows.Next() {
		target, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (store *targetStore) get(id string) (targetRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return targetRecord{}, false, err
	}
	defer db.Close()

	return getTargetByID(db, id)
}

func (store *targetStore) update(id string, input targetRecord) (targetRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return targetRecord{}, false, err
	}
	defer db.Close()

	id = strings.TrimSpace(id)
	existing, found, err := getTargetByID(db, id)
	if err != nil || !found {
		return targetRecord{}, found, err
	}

	input = normalizeTarget(input)
	if err := validateTarget(db, input); err != nil {
		return targetRecord{}, true, err
	}
	input.ID = id
	input.CreatedAt = existing.CreatedAt
	input.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	agentIDs, processMatch, healthCheck, metricThresholds, err := encodeTargetCollections(input)
	if err != nil {
		return targetRecord{}, true, err
	}
	_, err = db.Exec(`
		UPDATE targets
		SET name = ?, project_id = ?, base_url = ?, environment = ?, agent_ids = ?,
			profile_endpoint = ?, process_match = ?, health_check = ?, metric_thresholds = ?, updated_at = ?
		WHERE id = ?
	`, input.Name, input.ProjectID, input.BaseURL, input.Environment, agentIDs, input.ProfileEndpoint,
		processMatch, healthCheck, metricThresholds, input.UpdatedAt, id)
	if err != nil {
		return targetRecord{}, true, err
	}
	return input, true, nil
}

func (store *targetStore) delete(id string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return false, err
	}
	defer db.Close()

	result, err := db.Exec("DELETE FROM targets WHERE id = ?", strings.TrimSpace(id))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (store *targetStore) updateLastHealthCheck(id string, result targetHealthCheckResult) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return err
	}
	defer db.Close()

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	targetID := strings.TrimSpace(id)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE targets SET last_health_check = ?, updated_at = ? WHERE id = ?",
		string(data), now.Format(time.RFC3339Nano), targetID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO target_health_checks (
			id, target_id, target_name, status, source, url, expected_status,
			observed_status, latency_ms, error, checked_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newResourceID("target-health"), result.TargetID, result.TargetName, result.Status,
		result.Source, result.URL, result.ExpectedStatus, result.ObservedStatus, result.LatencyMs, result.Error,
		result.CheckedAt, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM target_health_checks
		WHERE target_id = ?
		  AND id NOT IN (
			SELECT id
			FROM target_health_checks
			WHERE target_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		  )
	`, targetID, targetID, targetHealthCheckHistoryRetention); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *targetStore) listHealthChecks(targetID string, limit int) ([]targetHealthCheckResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT target_id, target_name, status, source, url, expected_status,
			observed_status, latency_ms, error, checked_at
		FROM target_health_checks
		WHERE target_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, strings.TrimSpace(targetID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := []targetHealthCheckResult{}
	for rows.Next() {
		var result targetHealthCheckResult
		if err := rows.Scan(
			&result.TargetID,
			&result.TargetName,
			&result.Status,
			&result.Source,
			&result.URL,
			&result.ExpectedStatus,
			&result.ObservedStatus,
			&result.LatencyMs,
			&result.Error,
			&result.CheckedAt,
		); err != nil {
			return nil, err
		}
		history = append(history, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (store *targetStore) open() (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", store.path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureAgentSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureTargetSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureTargetHealthCheckHistorySchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureTargetSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS targets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT 'default',
			base_url TEXT NOT NULL,
			environment TEXT NOT NULL,
			agent_ids TEXT NOT NULL,
			profile_endpoint TEXT NOT NULL,
			process_match TEXT NOT NULL,
			health_check TEXT NOT NULL DEFAULT '{}',
			last_health_check TEXT NOT NULL DEFAULT '',
			metric_thresholds TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "targets", "health_check", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "targets", "last_health_check", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "targets", "metric_thresholds", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	return ensureSQLiteColumn(db, "targets", "project_id", "TEXT NOT NULL DEFAULT 'default'")
}

func ensureTargetHealthCheckHistorySchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS target_health_checks (
			id TEXT PRIMARY KEY,
			target_id TEXT NOT NULL,
			target_name TEXT NOT NULL,
			status TEXT NOT NULL,
			url TEXT NOT NULL,
			expected_status INTEGER NOT NULL,
			observed_status INTEGER NOT NULL,
			latency_ms REAL NOT NULL,
			error TEXT NOT NULL,
			checked_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "target_health_checks", "source", "TEXT NOT NULL DEFAULT 'control_plane'"); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_target_health_checks_target_created_at
		ON target_health_checks(target_id, created_at DESC)
	`)
	return err
}

func getTargetByID(db *sql.DB, id string) (targetRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, name, project_id, base_url, environment, agent_ids, profile_endpoint,
			process_match, health_check, last_health_check, metric_thresholds, created_at, updated_at
		FROM targets
		WHERE id = ?
	`, strings.TrimSpace(id))
	target, err := scanTarget(row)
	if err == nil {
		return target, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return targetRecord{}, false, nil
	}
	return targetRecord{}, false, err
}

func scanTarget(scanner targetScanner) (targetRecord, error) {
	var target targetRecord
	var agentIDs string
	var processMatch string
	var healthCheck string
	var lastHealthCheck string
	var metricThresholds string
	if err := scanner.Scan(
		&target.ID,
		&target.Name,
		&target.ProjectID,
		&target.BaseURL,
		&target.Environment,
		&agentIDs,
		&target.ProfileEndpoint,
		&processMatch,
		&healthCheck,
		&lastHealthCheck,
		&metricThresholds,
		&target.CreatedAt,
		&target.UpdatedAt,
	); err != nil {
		return targetRecord{}, err
	}
	if err := decodeTargetCollection("agentIds", agentIDs, &target.AgentIDs); err != nil {
		return targetRecord{}, err
	}
	if err := decodeTargetObject("processMatch", processMatch, &target.ProcessMatch); err != nil {
		return targetRecord{}, err
	}
	if err := decodeTargetObject("healthCheck", healthCheck, &target.HealthCheck); err != nil {
		return targetRecord{}, err
	}
	if err := decodeTargetOptionalObject("lastHealthCheck", lastHealthCheck, &target.LastHealthCheck); err != nil {
		return targetRecord{}, err
	}
	if err := decodeTargetObject("metricThresholds", metricThresholds, &target.MetricThresholds); err != nil {
		return targetRecord{}, err
	}
	return target, nil
}

func normalizeTarget(input targetRecord) targetRecord {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Environment = strings.TrimSpace(input.Environment)
	input.ProfileEndpoint = strings.TrimSpace(input.ProfileEndpoint)
	input.ProcessMatch.Name = strings.TrimSpace(input.ProcessMatch.Name)
	input.ProcessMatch.CmdlineContains = strings.TrimSpace(input.ProcessMatch.CmdlineContains)
	input.HealthCheck = normalizeTargetHealthCheck(input.HealthCheck)
	input.MetricThresholds = normalizeTargetMetricThresholds(input.MetricThresholds)
	if input.Environment == "" {
		input.Environment = "default"
	}
	if input.ProjectID == "" {
		input.ProjectID = "default"
	}

	agentIDs := make([]string, 0, len(input.AgentIDs))
	seen := map[string]struct{}{}
	for _, agentID := range input.AgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, exists := seen[agentID]; exists {
			continue
		}
		seen[agentID] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	input.AgentIDs = agentIDs
	return input
}

func normalizeTargetHealthCheck(input targetHealthCheck) targetHealthCheck {
	input.Path = strings.TrimSpace(input.Path)
	if input.Path != "" {
		input.Enabled = true
	}
	if input.Enabled && input.Path == "" {
		input.Path = "/health"
	}
	if input.Enabled && input.ExpectedStatus <= 0 {
		input.ExpectedStatus = http.StatusOK
	}
	if input.Enabled && input.TimeoutMs <= 0 {
		input.TimeoutMs = 1000
	}
	if input.TimeoutMs > 30000 {
		input.TimeoutMs = 30000
	}
	return input
}

func normalizeTargetMetricThresholds(input targetMetricThresholds) targetMetricThresholds {
	if input.CPUMaxPercent < 0 {
		input.CPUMaxPercent = 0
	}
	if input.MemoryMaxPercent < 0 {
		input.MemoryMaxPercent = 0
	}
	if input.DiskReadMaxBytesPerSec < 0 {
		input.DiskReadMaxBytesPerSec = 0
	}
	if input.DiskWriteMaxBytesPerSec < 0 {
		input.DiskWriteMaxBytesPerSec = 0
	}
	if input.NetworkRxMaxBytesPerSec < 0 {
		input.NetworkRxMaxBytesPerSec = 0
	}
	if input.NetworkTxMaxBytesPerSec < 0 {
		input.NetworkTxMaxBytesPerSec = 0
	}
	return input
}

func validateTarget(db *sql.DB, input targetRecord) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if input.BaseURL == "" {
		return fmt.Errorf("baseUrl is required")
	}
	for _, agentID := range input.AgentIDs {
		if _, found, err := getAgentByID(db, agentID); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("agent %q not found", agentID)
		}
	}
	return nil
}

func encodeTargetCollections(target targetRecord) (string, string, string, string, error) {
	agentIDs, err := json.Marshal(target.AgentIDs)
	if err != nil {
		return "", "", "", "", err
	}
	processMatch, err := json.Marshal(target.ProcessMatch)
	if err != nil {
		return "", "", "", "", err
	}
	healthCheck, err := json.Marshal(target.HealthCheck)
	if err != nil {
		return "", "", "", "", err
	}
	metricThresholds, err := json.Marshal(target.MetricThresholds)
	if err != nil {
		return "", "", "", "", err
	}
	return string(agentIDs), string(processMatch), string(healthCheck), string(metricThresholds), nil
}

func decodeTargetCollection(field string, data string, target any) error {
	if strings.TrimSpace(data) == "" {
		data = "[]"
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("target %s contains invalid JSON: %w", field, err)
	}
	return nil
}

func decodeTargetObject(field string, data string, target any) error {
	if strings.TrimSpace(data) == "" {
		data = "{}"
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("target %s contains invalid JSON: %w", field, err)
	}
	return nil
}

func decodeTargetOptionalObject[T any](field string, data string, target **T) error {
	data = strings.TrimSpace(data)
	if data == "" || data == "{}" || data == "null" {
		*target = nil
		return nil
	}
	var value T
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		return fmt.Errorf("target %s contains invalid JSON: %w", field, err)
	}
	*target = &value
	return nil
}

func checkTargetHealth(ctx context.Context, target targetRecord) targetHealthCheckResult {
	checkedAt := time.Now().UTC()
	result := targetHealthCheckResult{
		TargetID:       target.ID,
		TargetName:     target.Name,
		Status:         "not_configured",
		Source:         "control_plane",
		ExpectedStatus: target.HealthCheck.ExpectedStatus,
		CheckedAt:      checkedAt.Format(time.RFC3339Nano),
	}
	if !target.HealthCheck.Enabled {
		result.Error = "target health check is not configured"
		return result
	}

	healthURL, err := targetHealthCheckURL(target)
	if err != nil {
		result.Status = "unhealthy"
		result.Error = err.Error()
		return result
	}
	result.URL = healthURL
	result.ExpectedStatus = target.HealthCheck.ExpectedStatus
	timeout := time.Duration(target.HealthCheck.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		result.Status = "unhealthy"
		result.Error = err.Error()
		return result
	}

	startedAt := time.Now()
	response, err := http.DefaultClient.Do(request)
	result.LatencyMs = float64(time.Since(startedAt).Microseconds()) / 1000
	if err != nil {
		result.Status = "unhealthy"
		result.Error = err.Error()
		return result
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	result.ObservedStatus = response.StatusCode
	if response.StatusCode == target.HealthCheck.ExpectedStatus {
		result.Status = "healthy"
		return result
	}
	result.Status = "unhealthy"
	result.Error = fmt.Sprintf("expected HTTP %d, got HTTP %d", target.HealthCheck.ExpectedStatus, response.StatusCode)
	return result
}

func targetHealthCheckURL(target targetRecord) (string, error) {
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
