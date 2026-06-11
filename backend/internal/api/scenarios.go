package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

const scenarioDatabasePathEnv = "SCENARIO_DB_PATH"

type scenarioRecord struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Protocol      string                 `json:"protocol"`
	Method        string                 `json:"method"`
	BaseURL       string                 `json:"baseUrl"`
	Path          string                 `json:"path"`
	QueryVariants []scenarioQueryVariant `json:"queryVariants"`
	Headers       []scenarioHeader       `json:"headers"`
	BodyVariants  []scenarioBodyVariant  `json:"bodyVariants"`
	TimeoutMs     string                 `json:"timeoutMs"`
	RetryCount    string                 `json:"retryCount"`
	Assertion     string                 `json:"assertion"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type scenarioQueryVariant struct {
	Name        string `json:"name"`
	Weight      string `json:"weight"`
	QueryParams string `json:"queryParams"`
}

type scenarioHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type scenarioBodyVariant struct {
	Name   string `json:"name"`
	Weight string `json:"weight"`
	Body   string `json:"body"`
}

type scenarioStore struct {
	path string
	mu   sync.Mutex
}

func newScenarioStoreFromEnv() *scenarioStore {
	path := os.Getenv(scenarioDatabasePathEnv)
	if path == "" {
		path = filepath.Join("data", "platform.db")
	}
	return &scenarioStore{path: path}
}

func handleListScenarios(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		scenarios, err := store.list()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, scenarios)
	}
}

func handleCreateScenario(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input scenarioRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}

		created, err := store.create(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

func handleGetScenario(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scenario, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found"})
			return
		}

		writeJSON(w, http.StatusOK, scenario)
	}
}

func handleUpdateScenario(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input scenarioRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}

		updated, found, err := store.update(chi.URLParam(r, "id"), input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found"})
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

func handleDeleteScenario(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		found, err := store.delete(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (store *scenarioStore) list() ([]scenarioRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, name, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, timeout_ms, retry_count, assertion, created_at, updated_at
		FROM scenarios
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scenarios := []scenarioRecord{}
	for rows.Next() {
		scenario, err := scanScenario(rows)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, scenario)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scenarios, nil
}

func (store *scenarioStore) create(input scenarioRecord) (scenarioRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return scenarioRecord{}, err
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	input = normalizeScenario(input)
	if err := validateScenario(input); err != nil {
		return scenarioRecord{}, err
	}
	input.ID = fmt.Sprintf("scenario-%d", time.Now().UnixNano())
	input.CreatedAt = now
	input.UpdatedAt = now

	queryVariants, headers, bodyVariants, err := encodeScenarioCollections(input)
	if err != nil {
		return scenarioRecord{}, err
	}
	_, err = db.Exec(`
		INSERT INTO scenarios (
			id, name, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, timeout_ms, retry_count, assertion, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.ID, input.Name, input.Protocol, input.Method, input.BaseURL, input.Path, queryVariants,
		headers, bodyVariants, input.TimeoutMs, input.RetryCount, input.Assertion, input.CreatedAt, input.UpdatedAt)
	if err != nil {
		return scenarioRecord{}, err
	}
	return input, nil
}

func (store *scenarioStore) get(id string) (scenarioRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return scenarioRecord{}, false, err
	}
	defer db.Close()

	return getScenarioByID(db, id)
}

func (store *scenarioStore) update(id string, input scenarioRecord) (scenarioRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return scenarioRecord{}, false, err
	}
	defer db.Close()

	existing, found, err := getScenarioByID(db, id)
	if err != nil || !found {
		return scenarioRecord{}, found, err
	}

	input = normalizeScenario(input)
	if err := validateScenario(input); err != nil {
		return scenarioRecord{}, true, err
	}
	input.ID = id
	input.CreatedAt = existing.CreatedAt
	input.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	queryVariants, headers, bodyVariants, err := encodeScenarioCollections(input)
	if err != nil {
		return scenarioRecord{}, true, err
	}
	_, err = db.Exec(`
		UPDATE scenarios
		SET name = ?, protocol = ?, method = ?, base_url = ?, request_path = ?,
			query_variants = ?, headers = ?, body_variants = ?, timeout_ms = ?,
			retry_count = ?, assertion = ?, updated_at = ?
		WHERE id = ?
	`, input.Name, input.Protocol, input.Method, input.BaseURL, input.Path,
		queryVariants, headers, bodyVariants, input.TimeoutMs, input.RetryCount,
		input.Assertion, input.UpdatedAt, id)
	if err != nil {
		return scenarioRecord{}, true, err
	}
	return input, true, nil
}

func (store *scenarioStore) delete(id string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return false, err
	}
	defer db.Close()

	result, err := db.Exec("DELETE FROM scenarios WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (store *scenarioStore) open() (*sql.DB, error) {
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
	if err := ensureScenarioSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureScenarioSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scenarios (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			protocol TEXT NOT NULL,
			method TEXT NOT NULL,
			base_url TEXT NOT NULL,
			request_path TEXT NOT NULL,
			query_variants TEXT NOT NULL,
			headers TEXT NOT NULL,
			body_variants TEXT NOT NULL,
			timeout_ms TEXT NOT NULL,
			retry_count TEXT NOT NULL,
			assertion TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	return err
}

type scenarioScanner interface {
	Scan(dest ...any) error
}

func getScenarioByID(db *sql.DB, id string) (scenarioRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, name, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, timeout_ms, retry_count, assertion, created_at, updated_at
		FROM scenarios
		WHERE id = ?
	`, id)
	scenario, err := scanScenario(row)
	if err == nil {
		return scenario, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return scenarioRecord{}, false, nil
	}
	return scenarioRecord{}, false, err
}

func scanScenario(scanner scenarioScanner) (scenarioRecord, error) {
	var scenario scenarioRecord
	var queryVariants string
	var headers string
	var bodyVariants string

	if err := scanner.Scan(
		&scenario.ID,
		&scenario.Name,
		&scenario.Protocol,
		&scenario.Method,
		&scenario.BaseURL,
		&scenario.Path,
		&queryVariants,
		&headers,
		&bodyVariants,
		&scenario.TimeoutMs,
		&scenario.RetryCount,
		&scenario.Assertion,
		&scenario.CreatedAt,
		&scenario.UpdatedAt,
	); err != nil {
		return scenarioRecord{}, err
	}
	if err := decodeScenarioCollection("queryVariants", queryVariants, &scenario.QueryVariants); err != nil {
		return scenarioRecord{}, err
	}
	if err := decodeScenarioCollection("headers", headers, &scenario.Headers); err != nil {
		return scenarioRecord{}, err
	}
	if err := decodeScenarioCollection("bodyVariants", bodyVariants, &scenario.BodyVariants); err != nil {
		return scenarioRecord{}, err
	}
	return scenario, nil
}

func encodeScenarioCollections(scenario scenarioRecord) (string, string, string, error) {
	queryVariants, err := json.Marshal(scenario.QueryVariants)
	if err != nil {
		return "", "", "", err
	}
	headers, err := json.Marshal(scenario.Headers)
	if err != nil {
		return "", "", "", err
	}
	bodyVariants, err := json.Marshal(scenario.BodyVariants)
	if err != nil {
		return "", "", "", err
	}
	return string(queryVariants), string(headers), string(bodyVariants), nil
}

func decodeScenarioCollection(field string, data string, target any) error {
	if strings.TrimSpace(data) == "" {
		data = "[]"
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("scenario %s contains invalid JSON: %w", field, err)
	}
	return nil
}

func normalizeScenario(input scenarioRecord) scenarioRecord {
	input.Name = strings.TrimSpace(input.Name)
	input.Protocol = strings.ToUpper(strings.TrimSpace(input.Protocol))
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Path = strings.TrimSpace(input.Path)
	input.TimeoutMs = strings.TrimSpace(input.TimeoutMs)
	input.RetryCount = strings.TrimSpace(input.RetryCount)
	input.Assertion = strings.TrimSpace(input.Assertion)
	if input.Protocol == "" {
		input.Protocol = "HTTP"
	}
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	if input.Path == "" {
		input.Path = "/"
	}
	if input.TimeoutMs == "" {
		input.TimeoutMs = "1000"
	}
	if input.RetryCount == "" {
		input.RetryCount = "0"
	}
	if input.Assertion == "" {
		input.Assertion = "status < 400"
	}
	if input.QueryVariants == nil {
		input.QueryVariants = []scenarioQueryVariant{}
	}
	if input.Headers == nil {
		input.Headers = []scenarioHeader{}
	}
	if input.BodyVariants == nil {
		input.BodyVariants = []scenarioBodyVariant{}
	}
	return input
}

func validateScenario(input scenarioRecord) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if input.BaseURL == "" {
		return fmt.Errorf("baseUrl is required")
	}
	return nil
}
