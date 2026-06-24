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

const (
	scenarioDatabasePathEnv    = "SCENARIO_DB_PATH"
	defaultScenarioProjectID   = "default"
	defaultScenarioEnvironment = "default"
)

type scenarioRecord struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	ProjectID     string                 `json:"projectId"`
	Environment   string                 `json:"environment"`
	Protocol      string                 `json:"protocol"`
	Method        string                 `json:"method"`
	BaseURL       string                 `json:"baseUrl"`
	Path          string                 `json:"path"`
	QueryVariants []scenarioQueryVariant `json:"queryVariants"`
	Headers       []scenarioHeader       `json:"headers"`
	BodyVariants  []scenarioBodyVariant  `json:"bodyVariants"`
	FlowSteps     []scenarioFlowStep     `json:"flowSteps"`
	TimeoutMs     string                 `json:"timeoutMs"`
	RetryCount    string                 `json:"retryCount"`
	Assertion     string                 `json:"assertion"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type scenarioListFilter struct {
	ProjectID   string
	Environment string
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

type scenarioFlowStep struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Type       string                      `json:"type"`
	Enabled    bool                        `json:"enabled"`
	Protocol   string                      `json:"protocol,omitempty"`
	Method     string                      `json:"method,omitempty"`
	Path       string                      `json:"path,omitempty"`
	Assertion  string                      `json:"assertion,omitempty"`
	Extractors []scenarioVariableExtractor `json:"extractors,omitempty"`
	Assertions []scenarioResponseAssertion `json:"assertions,omitempty"`
	When       *scenarioResponseAssertion  `json:"when,omitempty"`
}

type scenarioVariableExtractor struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type scenarioResponseAssertion struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Path     string `json:"path,omitempty"`
	Operator string `json:"operator"`
	Expected string `json:"expected,omitempty"`
}

type scenarioStore struct {
	path string
	mu   sync.Mutex
}

func newScenarioStoreFromEnv() *scenarioStore {
	return &scenarioStore{path: scenarioDatabasePathFromEnv()}
}

func scenarioDatabasePathFromEnv() string {
	path := os.Getenv(scenarioDatabasePathEnv)
	if path == "" {
		path = filepath.Join("data", "platform.db")
	}
	return path
}

func handleListScenarios(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := scenarioListFilterFromRequest(r)
		if err := applyWorkspaceScopeToFilter(r, &filter.ProjectID, &filter.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		scenarios, err := store.list(filter)
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
		if err := applyWorkspaceScopeToRecord(r, &input.ProjectID, &input.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
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
		if err := requireWorkspaceScopeForRecord(r, scenario.ProjectID, scenario.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
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

		scenario, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, scenario.ProjectID, scenario.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if err := applyWorkspaceScopeToRecord(r, &input.ProjectID, &input.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
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
		scenario, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, scenario.ProjectID, scenario.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		found, err = store.delete(chi.URLParam(r, "id"))
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

func scenarioListFilterFromRequest(r *http.Request) scenarioListFilter {
	query := r.URL.Query()
	return scenarioListFilter{
		ProjectID:   strings.TrimSpace(query.Get("projectId")),
		Environment: strings.TrimSpace(query.Get("environment")),
	}
}

func (store *scenarioStore) list(filter scenarioListFilter) ([]scenarioRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT id, name, project_id, environment, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, flow_steps, timeout_ms, retry_count, assertion, created_at, updated_at
		FROM scenarios
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
	input.ID = newResourceID("scenario")
	input.CreatedAt = now
	input.UpdatedAt = now

	queryVariants, headers, bodyVariants, flowSteps, err := encodeScenarioCollections(input)
	if err != nil {
		return scenarioRecord{}, err
	}
	_, err = db.Exec(`
		INSERT INTO scenarios (
			id, name, project_id, environment, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, flow_steps, timeout_ms, retry_count, assertion, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.ID, input.Name, input.ProjectID, input.Environment, input.Protocol, input.Method, input.BaseURL, input.Path, queryVariants,
		headers, bodyVariants, flowSteps, input.TimeoutMs, input.RetryCount, input.Assertion, input.CreatedAt, input.UpdatedAt)
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

	queryVariants, headers, bodyVariants, flowSteps, err := encodeScenarioCollections(input)
	if err != nil {
		return scenarioRecord{}, true, err
	}
	_, err = db.Exec(`
		UPDATE scenarios
		SET name = ?, project_id = ?, environment = ?, protocol = ?, method = ?, base_url = ?, request_path = ?,
			query_variants = ?, headers = ?, body_variants = ?, flow_steps = ?, timeout_ms = ?,
			retry_count = ?, assertion = ?, updated_at = ?
		WHERE id = ?
	`, input.Name, input.ProjectID, input.Environment, input.Protocol, input.Method, input.BaseURL, input.Path,
		queryVariants, headers, bodyVariants, flowSteps, input.TimeoutMs, input.RetryCount,
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
			project_id TEXT NOT NULL DEFAULT 'default',
			environment TEXT NOT NULL DEFAULT 'default',
			protocol TEXT NOT NULL,
			method TEXT NOT NULL,
			base_url TEXT NOT NULL,
			request_path TEXT NOT NULL,
			query_variants TEXT NOT NULL,
			headers TEXT NOT NULL,
			body_variants TEXT NOT NULL,
			flow_steps TEXT NOT NULL DEFAULT '[]',
			timeout_ms TEXT NOT NULL,
			retry_count TEXT NOT NULL,
			assertion TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	if err := ensureScenarioColumn(db, "flow_steps", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureScenarioColumn(db, "project_id", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	return ensureScenarioColumn(db, "environment", "TEXT NOT NULL DEFAULT 'default'")
}

func ensureScenarioColumn(db *sql.DB, columnName string, definition string) error {
	rows, err := db.Query("PRAGMA table_info(scenarios)")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE scenarios ADD COLUMN %s %s", columnName, definition))
	return err
}

type scenarioScanner interface {
	Scan(dest ...any) error
}

func getScenarioByID(db *sql.DB, id string) (scenarioRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, name, project_id, environment, protocol, method, base_url, request_path, query_variants,
			headers, body_variants, flow_steps, timeout_ms, retry_count, assertion, created_at, updated_at
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
	var flowSteps string

	if err := scanner.Scan(
		&scenario.ID,
		&scenario.Name,
		&scenario.ProjectID,
		&scenario.Environment,
		&scenario.Protocol,
		&scenario.Method,
		&scenario.BaseURL,
		&scenario.Path,
		&queryVariants,
		&headers,
		&bodyVariants,
		&flowSteps,
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
	if err := decodeScenarioCollection("flowSteps", flowSteps, &scenario.FlowSteps); err != nil {
		return scenarioRecord{}, err
	}
	return scenario, nil
}

func encodeScenarioCollections(scenario scenarioRecord) (string, string, string, string, error) {
	queryVariants, err := json.Marshal(scenario.QueryVariants)
	if err != nil {
		return "", "", "", "", err
	}
	headers, err := json.Marshal(scenario.Headers)
	if err != nil {
		return "", "", "", "", err
	}
	bodyVariants, err := json.Marshal(scenario.BodyVariants)
	if err != nil {
		return "", "", "", "", err
	}
	flowSteps, err := json.Marshal(scenario.FlowSteps)
	if err != nil {
		return "", "", "", "", err
	}
	return string(queryVariants), string(headers), string(bodyVariants), string(flowSteps), nil
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
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Environment = strings.TrimSpace(input.Environment)
	input.Protocol = strings.ToUpper(strings.TrimSpace(input.Protocol))
	input.Method = strings.TrimSpace(input.Method)
	if input.Protocol != "CUSTOM_RPC" {
		input.Method = strings.ToUpper(input.Method)
	}
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Path = strings.TrimSpace(input.Path)
	input.TimeoutMs = strings.TrimSpace(input.TimeoutMs)
	input.RetryCount = strings.TrimSpace(input.RetryCount)
	input.Assertion = strings.TrimSpace(input.Assertion)
	if input.Protocol == "" {
		input.Protocol = "HTTP"
	}
	if input.ProjectID == "" {
		input.ProjectID = defaultScenarioProjectID
	}
	if input.Environment == "" {
		input.Environment = defaultScenarioEnvironment
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
	input.FlowSteps = normalizeScenarioFlowSteps(input.FlowSteps)
	return input
}

func normalizeScenarioFlowSteps(steps []scenarioFlowStep) []scenarioFlowStep {
	if steps == nil {
		return []scenarioFlowStep{}
	}

	normalizedSteps := make([]scenarioFlowStep, 0, len(steps))
	for _, step := range steps {
		step.ID = strings.TrimSpace(step.ID)
		step.Name = strings.TrimSpace(step.Name)
		step.Type = strings.ToLower(strings.TrimSpace(step.Type))
		step.Protocol = strings.ToUpper(strings.TrimSpace(step.Protocol))
		step.Method = strings.ToUpper(strings.TrimSpace(step.Method))
		step.Path = strings.TrimSpace(step.Path)
		step.Assertion = strings.TrimSpace(step.Assertion)
		step.Extractors = normalizeScenarioExtractors(step.Extractors)
		step.Assertions = normalizeScenarioAssertions(step.Assertions)
		step.When = normalizeScenarioCondition(step.When)
		if step.Name == "" {
			continue
		}
		if step.Type == "" {
			step.Type = "request"
		}
		normalizedSteps = append(normalizedSteps, step)
	}
	return normalizedSteps
}

func normalizeScenarioExtractors(extractors []scenarioVariableExtractor) []scenarioVariableExtractor {
	if extractors == nil {
		return []scenarioVariableExtractor{}
	}

	normalizedExtractors := make([]scenarioVariableExtractor, 0, len(extractors))
	for _, extractor := range extractors {
		extractor.Name = strings.TrimSpace(extractor.Name)
		extractor.Source = strings.ToLower(strings.TrimSpace(extractor.Source))
		extractor.Path = strings.TrimSpace(extractor.Path)
		if extractor.Source == "" {
			extractor.Source = "json"
		}
		if extractor.Name == "" || extractor.Path == "" {
			continue
		}
		normalizedExtractors = append(normalizedExtractors, extractor)
	}
	return normalizedExtractors
}

func normalizeScenarioAssertions(assertions []scenarioResponseAssertion) []scenarioResponseAssertion {
	if assertions == nil {
		return []scenarioResponseAssertion{}
	}

	normalizedAssertions := make([]scenarioResponseAssertion, 0, len(assertions))
	for _, assertion := range assertions {
		assertion.Name = strings.TrimSpace(assertion.Name)
		assertion.Source = strings.ToLower(strings.TrimSpace(assertion.Source))
		assertion.Path = strings.TrimSpace(assertion.Path)
		assertion.Operator = normalizeScenarioAssertionOperator(assertion.Operator)
		assertion.Expected = strings.TrimSpace(assertion.Expected)
		if assertion.Source == "" {
			assertion.Source = "body"
		}
		if assertion.Operator == "" {
			assertion.Operator = "equals"
		}
		if assertion.Name == "" {
			assertion.Name = assertion.Source
			if assertion.Path != "" {
				assertion.Name += " " + assertion.Path
			}
		}
		if assertion.Source != "body" && assertion.Path == "" {
			continue
		}
		if assertion.Operator != "exists" && assertion.Expected == "" {
			continue
		}
		normalizedAssertions = append(normalizedAssertions, assertion)
	}
	return normalizedAssertions
}

func normalizeScenarioCondition(condition *scenarioResponseAssertion) *scenarioResponseAssertion {
	if condition == nil {
		return nil
	}
	normalized := normalizeScenarioAssertions([]scenarioResponseAssertion{*condition})
	if len(normalized) == 0 {
		return nil
	}
	return &normalized[0]
}

func normalizeScenarioAssertionOperator(operator string) string {
	normalized := strings.ToLower(strings.TrimSpace(operator))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "", "eq", "equal", "equals", "==", "=":
		return "equals"
	case "ne", "not_equal", "not_equals", "notequals", "!=":
		return "not_equals"
	case "contains", "include", "includes":
		return "contains"
	case "not_contains", "notcontains", "excludes":
		return "not_contains"
	case "matches", "match", "regex", "=~":
		return "matches"
	case "not_matches", "notmatches", "not_match", "!~":
		return "not_matches"
	case "exists", "exist":
		return "exists"
	default:
		return normalized
	}
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
