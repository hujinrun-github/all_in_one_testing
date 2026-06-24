package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type runHistoryStore struct {
	path string
	mu   sync.Mutex
}

type runScanner interface {
	Scan(dest ...any) error
}

type runEventRecord struct {
	ID              string  `json:"id"`
	RunID           string  `json:"runId"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Message         string  `json:"message"`
	SuccessRequests int     `json:"successRequests"`
	FailedRequests  int     `json:"failedRequests"`
	TotalRequests   int     `json:"totalRequests"`
	QPS             float64 `json:"qps"`
	P95LatencyMs    float64 `json:"p95LatencyMs"`
	CreatedAt       string  `json:"createdAt"`
}

type runRequestSampleRecord struct {
	ID         string  `json:"id"`
	RunID      string  `json:"runId"`
	Kind       string  `json:"kind"`
	Method     string  `json:"method"`
	URL        string  `json:"url"`
	StatusCode int     `json:"statusCode"`
	Success    bool    `json:"success"`
	LatencyMs  float64 `json:"latencyMs"`
	Error      string  `json:"error,omitempty"`
	CreatedAt  string  `json:"createdAt"`
}

type runListFilter struct {
	ProjectID   string
	Environment string
}

func newRunHistoryStoreFromEnv() *runHistoryStore {
	return &runHistoryStore{path: scenarioDatabasePathFromEnv()}
}

func handleListRuns(store *runHistoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := runListFilterFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		runs, err := store.listFiltered(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, runs)
	}
}

func handleGetRun(store *runHistoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, found, err := store.get(chi.URLParam(r, "id"))
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

		writeJSON(w, http.StatusOK, run)
	}
}

func handleListRunEvents(store *runHistoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "id")
		if run, found, err := store.get(runID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		} else if err := requireWorkspaceScopeForRecord(r, run.ProjectID, run.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		events, err := store.listEvents(runID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, events)
	}
}

func handleStreamRunEvents(store *runHistoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "id")
		if run, found, err := store.get(runID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		} else if err := requireWorkspaceScopeForRecord(r, run.ProjectID, run.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		emitted := map[string]struct{}{}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		for {
			events, err := store.listEvents(runID)
			if err != nil {
				return
			}
			wroteEvent := false
			terminalEvent := false
			for _, event := range events {
				if _, found := emitted[event.ID]; found {
					continue
				}
				emitted[event.ID] = struct{}{}
				if err := writeRunEventSSE(w, event); err != nil {
					return
				}
				wroteEvent = true
				if isTerminalRunEvent(event.Type) {
					terminalEvent = true
				}
			}
			if wroteEvent {
				flusher.Flush()
			}
			if terminalEvent {
				return
			}

			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	}
}

func runListFilterFromRequest(r *http.Request) (runListFilter, error) {
	filter := runListFilter{
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("projectId")),
		Environment: strings.TrimSpace(r.URL.Query().Get("environment")),
	}
	if err := applyWorkspaceScopeToFilter(r, &filter.ProjectID, &filter.Environment); err != nil {
		return runListFilter{}, err
	}
	return filter, nil
}

func writeRunEventSSE(w io.Writer, event runEventRecord) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, payload)
	return err
}

func isTerminalRunEvent(eventType string) bool {
	return eventType == "run_finished" || eventType == "run_canceled" || eventType == "run_aborted"
}

func (store *runHistoryStore) save(run createRunResponse) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		INSERT OR REPLACE INTO run_history (
			id, scenario_id, scenario_name, target_id, target_name, project_id, environment, name, status, protocol, method, url,
			total_requests, success_requests, failed_requests, max_error_rate_percent, max_p95_latency_ms, duration_ms, qps,
			average_latency_ms, p95_latency_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.ScenarioID, run.ScenarioName, run.TargetID, run.TargetName, run.ProjectID, run.Environment, run.Name, run.Status, run.Protocol, run.Method, run.URL,
		run.TotalRequests, run.SuccessRequests, run.FailedRequests, run.MaxErrorRatePercent, run.MaxP95LatencyMs, run.DurationMs, run.QPS,
		run.AverageLatencyMs, run.P95LatencyMs, run.CreatedAt)
	return err
}

func (store *runHistoryStore) appendEvent(event runEventRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return err
	}
	defer db.Close()

	if event.ID == "" {
		event.ID = newRunEventID(event.RunID)
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = db.Exec(`
		INSERT INTO run_events (
			id, run_id, type, status, message, success_requests, failed_requests,
			total_requests, qps, p95_latency_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.RunID, event.Type, event.Status, event.Message, event.SuccessRequests,
		event.FailedRequests, event.TotalRequests, event.QPS, event.P95LatencyMs, event.CreatedAt)
	return err
}

func (store *runHistoryStore) replaceRequestSamples(runID string, slowSamples []runRequestSampleRecord, errorSamples []runRequestSampleRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM run_request_samples WHERE run_id = ?`, runID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, sample := range append(append([]runRequestSampleRecord(nil), slowSamples...), errorSamples...) {
		if sample.ID == "" {
			sample.ID = newRunRequestSampleID(runID, sample.Kind)
		}
		if sample.RunID == "" {
			sample.RunID = runID
		}
		if sample.CreatedAt == "" {
			sample.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.Exec(`
			INSERT INTO run_request_samples (
				id, run_id, kind, method, url, status_code, success, latency_ms, error, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sample.ID, sample.RunID, sample.Kind, sample.Method, sample.URL, sample.StatusCode,
			sample.Success, sample.LatencyMs, sample.Error, sample.CreatedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (store *runHistoryStore) listEvents(runID string) ([]runEventRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, run_id, type, status, message, success_requests, failed_requests,
			total_requests, qps, p95_latency_ms, created_at
		FROM run_events
		WHERE run_id = ?
		ORDER BY created_at ASC, id ASC
		LIMIT 500
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []runEventRecord{}
	for rows.Next() {
		event, err := scanRunEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (store *runHistoryStore) listRequestSamples(runID string, kind string) ([]runRequestSampleRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	orderClause := "ORDER BY created_at ASC, id ASC"
	if kind == "slow" {
		orderClause = "ORDER BY latency_ms DESC, created_at ASC, id ASC"
	}
	rows, err := db.Query(`
		SELECT id, run_id, kind, method, url, status_code, success, latency_ms, error, created_at
		FROM run_request_samples
		WHERE run_id = ? AND kind = ?
		`+orderClause+`
		LIMIT 5
	`, runID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	samples := []runRequestSampleRecord{}
	for rows.Next() {
		sample, err := scanRunRequestSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func (store *runHistoryStore) get(id string) (createRunResponse, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return createRunResponse{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT id, scenario_id, scenario_name, target_id, target_name, project_id, environment, name, status, protocol, method, url,
			total_requests, success_requests, failed_requests, max_error_rate_percent, max_p95_latency_ms, duration_ms, qps,
			average_latency_ms, p95_latency_ms, created_at
		FROM run_history
		WHERE id = ?
	`, id)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return createRunResponse{}, false, nil
	}
	if err != nil {
		return createRunResponse{}, false, err
	}
	return run, true, nil
}

func (store *runHistoryStore) list() ([]createRunResponse, error) {
	return store.listFiltered(runListFilter{})
}

func (store *runHistoryStore) listFiltered(filter runListFilter) ([]createRunResponse, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

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
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	rows, err := db.Query(`
		SELECT id, scenario_id, scenario_name, target_id, target_name, project_id, environment, name, status, protocol, method, url,
			total_requests, success_requests, failed_requests, max_error_rate_percent, max_p95_latency_ms, duration_ms, qps,
			average_latency_ms, p95_latency_ms, created_at
		FROM run_history
		`+whereClause+`
		ORDER BY created_at DESC
		LIMIT 100
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []createRunResponse{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (store *runHistoryStore) open() (*sql.DB, error) {
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
	if err := ensureRunHistorySchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureRunHistorySchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS run_history (
			id TEXT PRIMARY KEY,
			scenario_id TEXT NOT NULL,
			scenario_name TEXT NOT NULL,
			target_id TEXT NOT NULL DEFAULT '',
			target_name TEXT NOT NULL DEFAULT '',
			project_id TEXT NOT NULL DEFAULT '',
			environment TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'HTTP',
			method TEXT NOT NULL,
			url TEXT NOT NULL,
			total_requests INTEGER NOT NULL,
			success_requests INTEGER NOT NULL,
			failed_requests INTEGER NOT NULL,
			max_error_rate_percent REAL NOT NULL DEFAULT 0,
			max_p95_latency_ms REAL NOT NULL DEFAULT 0,
			duration_ms REAL NOT NULL,
			qps REAL NOT NULL,
			average_latency_ms REAL NOT NULL,
			p95_latency_ms REAL NOT NULL,
			created_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "target_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "target_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "project_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "environment", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "protocol", "TEXT NOT NULL DEFAULT 'HTTP'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "max_error_rate_percent", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "run_history", "max_p95_latency_ms", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS run_events (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT NOT NULL,
			success_requests INTEGER NOT NULL,
			failed_requests INTEGER NOT NULL,
			total_requests INTEGER NOT NULL,
			qps REAL NOT NULL,
			p95_latency_ms REAL NOT NULL,
			created_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS run_request_samples (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			method TEXT NOT NULL,
			url TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			success INTEGER NOT NULL,
			latency_ms REAL NOT NULL,
			error TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`)
	return err
}

func scanRun(scanner runScanner) (createRunResponse, error) {
	var run createRunResponse
	err := scanner.Scan(
		&run.ID,
		&run.ScenarioID,
		&run.ScenarioName,
		&run.TargetID,
		&run.TargetName,
		&run.ProjectID,
		&run.Environment,
		&run.Name,
		&run.Status,
		&run.Protocol,
		&run.Method,
		&run.URL,
		&run.TotalRequests,
		&run.SuccessRequests,
		&run.FailedRequests,
		&run.MaxErrorRatePercent,
		&run.MaxP95LatencyMs,
		&run.DurationMs,
		&run.QPS,
		&run.AverageLatencyMs,
		&run.P95LatencyMs,
		&run.CreatedAt,
	)
	if err != nil {
		return createRunResponse{}, err
	}
	return run, nil
}

func scanRunEvent(scanner runScanner) (runEventRecord, error) {
	var event runEventRecord
	err := scanner.Scan(
		&event.ID,
		&event.RunID,
		&event.Type,
		&event.Status,
		&event.Message,
		&event.SuccessRequests,
		&event.FailedRequests,
		&event.TotalRequests,
		&event.QPS,
		&event.P95LatencyMs,
		&event.CreatedAt,
	)
	if err != nil {
		return runEventRecord{}, err
	}
	return event, nil
}

func scanRunRequestSample(scanner runScanner) (runRequestSampleRecord, error) {
	var sample runRequestSampleRecord
	err := scanner.Scan(
		&sample.ID,
		&sample.RunID,
		&sample.Kind,
		&sample.Method,
		&sample.URL,
		&sample.StatusCode,
		&sample.Success,
		&sample.LatencyMs,
		&sample.Error,
		&sample.CreatedAt,
	)
	if err != nil {
		return runRequestSampleRecord{}, err
	}
	return sample, nil
}
