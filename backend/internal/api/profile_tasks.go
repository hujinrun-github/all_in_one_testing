package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type profileTaskRecord struct {
	ID                      string   `json:"id"`
	AgentID                 string   `json:"agentId"`
	RunID                   string   `json:"runId"`
	ScenarioID              string   `json:"scenarioId,omitempty"`
	ScenarioName            string   `json:"scenarioName,omitempty"`
	TargetID                string   `json:"targetId,omitempty"`
	TargetName              string   `json:"targetName,omitempty"`
	PprofBaseURL            string   `json:"pprofBaseUrl,omitempty"`
	ProfileURL              string   `json:"profileUrl,omitempty"`
	ProfileType             string   `json:"profileType"`
	ProfileSeconds          int      `json:"profileSeconds"`
	ProfileCommand          string   `json:"profileCommand,omitempty"`
	ProfileCommandArgs      []string `json:"profileCommandArgs,omitempty"`
	ProfileCommandOutput    string   `json:"profileCommandOutput,omitempty"`
	ProfileCommandTimeoutMs int      `json:"profileCommandTimeoutMs,omitempty"`
	Source                  string   `json:"source"`
	Status                  string   `json:"status"`
	Attempts                int      `json:"attempts"`
	MaxAttempts             int      `json:"maxAttempts"`
	ArtifactID              string   `json:"artifactId,omitempty"`
	Error                   string   `json:"error,omitempty"`
	CreatedAt               string   `json:"createdAt"`
	UpdatedAt               string   `json:"updatedAt"`
	LeasedAt                string   `json:"leasedAt,omitempty"`
	LeaseExpiresAt          string   `json:"leaseExpiresAt,omitempty"`
	CompletedAt             string   `json:"completedAt,omitempty"`
}

type completeProfileTaskRequest struct {
	ArtifactID string `json:"artifactId"`
	Error      string `json:"error"`
}

type profileTaskListFilter struct {
	RunID       string
	AgentID     string
	Status      string
	Source      string
	ProjectID   string
	Environment string
	Limit       int
}

type profileTaskStore struct {
	path         string
	leaseTimeout time.Duration
	mu           sync.Mutex
}

type profileTaskScanner interface {
	Scan(dest ...any) error
}

const defaultProfileTaskLeaseTimeout = 2 * time.Minute
const defaultProfileTaskMaxAttempts = 3
const maxProfileTaskMaxAttempts = 10
const maxProfileTaskSeconds = 300
const defaultProfileTaskSource = "api"
const runAutoProfileTaskSource = "run_auto"
const targetManualProfileTaskSource = "target_manual"
const thresholdAutoProfileTaskSource = "threshold_auto"

func newProfileTaskStoreFromEnv() *profileTaskStore {
	return &profileTaskStore{
		path:         scenarioDatabasePathFromEnv(),
		leaseTimeout: profileTaskLeaseTimeoutFromEnv(),
	}
}

func handleCreateProfileTask(store *profileTaskStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input profileTaskRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		normalized := normalizeProfileTask(input)
		if err := validateProfileTask(normalized); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if status, err := store.requireWorkspaceScopeForTask(r, normalized); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		task, err := store.create(normalized)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, task)
	}
}

func handleListProfileTasks(store *profileTaskStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := profileTaskListFilterFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := applyWorkspaceScopeToFilter(r, &filter.ProjectID, &filter.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		tasks, err := store.list(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func profileTaskListFilterFromRequest(r *http.Request) (profileTaskListFilter, error) {
	filter := profileTaskListFilter{
		RunID:       strings.TrimSpace(r.URL.Query().Get("runId")),
		AgentID:     strings.TrimSpace(r.URL.Query().Get("agentId")),
		Status:      strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))),
		Source:      strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source"))),
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("projectId")),
		Environment: strings.TrimSpace(r.URL.Query().Get("environment")),
		Limit:       100,
	}
	if filter.Source != "" && !isValidProfileTaskSource(filter.Source) {
		return profileTaskListFilter{}, fmt.Errorf("source must be api, run_auto, target_manual, or threshold_auto")
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return profileTaskListFilter{}, fmt.Errorf("limit must be a positive integer")
		}
		filter.Limit = limit
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	return filter, nil
}

func handleAgentPollProfileTasks(agentStore *agentStore, taskStore *profileTaskStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok, err := agentStore.authenticateRequest(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "valid agent token is required"})
			return
		}

		agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
		if agentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
			return
		}
		if status, err := agentStore.requireTokenForAgent(token, agentID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		tasks, err := taskStore.leasePending(agentID, 10)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func handleCompleteProfileTask(agentStore *agentStore, taskStore *profileTaskStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok, err := agentStore.authenticateRequest(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "valid agent token is required"})
			return
		}

		var input completeProfileTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		taskID := chi.URLParam(r, "id")
		existing, found, err := taskStore.get(taskID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task not found"})
			return
		}
		if status, err := agentStore.requireTokenForAgent(token, existing.AgentID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if err := taskStore.requireProfileTaskResourceConsistency(existing); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}

		task, found, err := taskStore.complete(taskID, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task not found"})
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func handleRetryProfileTask(taskStore *profileTaskStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := chi.URLParam(r, "id")
		existing, found, err := taskStore.get(taskID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task not found"})
			return
		}
		if status, err := taskStore.requireWorkspaceScopeForTask(r, existing); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		task, found, err := taskStore.retry(taskID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task not found"})
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func (store *profileTaskStore) create(input profileTaskRecord) (profileTaskRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskRecord{}, err
	}
	defer db.Close()

	task := normalizeProfileTask(input)
	if err := validateProfileTask(task); err != nil {
		return profileTaskRecord{}, err
	}
	if err := validateProfileTaskResourceConsistency(db, task); err != nil {
		return profileTaskRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task.ID = newResourceID("profile-task")
	task.Status = "pending"
	task.Attempts = 0
	task.CreatedAt = now
	task.UpdatedAt = now

	_, err = db.Exec(`
		INSERT INTO profile_tasks (
			id, agent_id, run_id, scenario_id, scenario_name, target_id, target_name,
			pprof_base_url, profile_url, profile_type, profile_seconds,
			profile_command, profile_command_args, profile_command_output, profile_command_timeout_ms,
			source, status, attempts, max_attempts,
			artifact_id, error, created_at, updated_at, leased_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, '', '')
	`, task.ID, task.AgentID, task.RunID, task.ScenarioID, task.ScenarioName, task.TargetID, task.TargetName,
		task.PprofBaseURL, task.ProfileURL, task.ProfileType, task.ProfileSeconds,
		task.ProfileCommand, encodeProfileCommandArgs(task.ProfileCommandArgs), task.ProfileCommandOutput, task.ProfileCommandTimeoutMs,
		task.Source, task.Status, task.Attempts, task.MaxAttempts,
		task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return profileTaskRecord{}, err
	}
	return task, nil
}

func (store *profileTaskStore) list(filter profileTaskListFilter) ([]profileTaskRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}

	query := `
		SELECT id, agent_id, run_id, scenario_id, scenario_name, target_id, target_name,
			pprof_base_url, profile_url, profile_type, profile_seconds,
			profile_command, profile_command_args, profile_command_output, profile_command_timeout_ms,
			source, status,
			attempts, max_attempts, artifact_id, error, created_at, updated_at, leased_at, completed_at
		FROM profile_tasks
	`
	conditions := []string{}
	args := []any{}
	if filter.RunID != "" {
		conditions = append(conditions, "run_id = ?")
		args = append(args, filter.RunID)
	}
	if filter.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, filter.Source)
	}
	if filter.ProjectID != "" || filter.Environment != "" {
		scopeCondition, scopeArgs := profileTaskScopeCondition(filter)
		conditions = append(conditions, scopeCondition)
		args = append(args, scopeArgs...)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += `
		ORDER BY created_at DESC
		LIMIT ?
	`
	args = append(args, filter.Limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []profileTaskRecord{}
	for rows.Next() {
		task, err := scanProfileTask(rows)
		if err != nil {
			return nil, err
		}
		task = withProfileTaskLeaseExpiry(task, store.effectiveLeaseTimeout())
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func profileTaskScopeCondition(filter profileTaskListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	conditions = append(conditions, "(profile_tasks.run_id <> '' OR profile_tasks.agent_id <> '' OR profile_tasks.target_id <> '')")
	conditions = append(conditions, `(profile_tasks.run_id = '' OR EXISTS (
			SELECT 1 FROM run_history scoped_runs WHERE `+profileTaskScopedResourceConditions("scoped_runs", "profile_tasks.run_id", filter, &args)+`
		))`)
	conditions = append(conditions, `(profile_tasks.agent_id = '' OR EXISTS (
			SELECT 1 FROM agents scoped_agents WHERE `+profileTaskScopedResourceConditions("scoped_agents", "profile_tasks.agent_id", filter, &args)+`
		))`)
	conditions = append(conditions, `(profile_tasks.target_id = '' OR EXISTS (
			SELECT 1 FROM targets scoped_targets WHERE `+profileTaskScopedResourceConditions("scoped_targets", "profile_tasks.target_id", filter, &args)+`
		))`)
	return "(" + strings.Join(conditions, " AND ") + ")", args
}

func profileTaskScopedResourceConditions(tableAlias string, idExpression string, filter profileTaskListFilter, args *[]any) string {
	conditions := []string{tableAlias + ".id = " + idExpression}
	if filter.ProjectID != "" {
		conditions = append(conditions, tableAlias+".project_id = ?")
		*args = append(*args, filter.ProjectID)
	}
	if filter.Environment != "" {
		conditions = append(conditions, tableAlias+".environment = ?")
		*args = append(*args, filter.Environment)
	}
	return strings.Join(conditions, " AND ")
}

func (store *profileTaskStore) leasePending(agentID string, limit int) ([]profileTaskRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if limit <= 0 {
		limit = 10
	}
	nowTime := time.Now().UTC()
	leaseTimeout := store.effectiveLeaseTimeout()
	rows, err := db.Query(`
		SELECT id, agent_id, run_id, scenario_id, scenario_name, target_id, target_name,
			pprof_base_url, profile_url, profile_type, profile_seconds,
			profile_command, profile_command_args, profile_command_output, profile_command_timeout_ms,
			source, status,
			attempts, max_attempts, artifact_id, error, created_at, updated_at, leased_at, completed_at
		FROM profile_tasks
		WHERE agent_id = ? AND status IN ('pending', 'leased')
		ORDER BY
			CASE status WHEN 'pending' THEN 0 ELSE 1 END,
			created_at ASC
	`, strings.TrimSpace(agentID))
	if err != nil {
		return nil, err
	}

	tasks := []profileTaskRecord{}
	for rows.Next() {
		task, err := scanProfileTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if task.Status != "pending" && !isProfileTaskLeaseExpired(task.LeasedAt, nowTime, leaseTimeout) {
			continue
		}
		tasks = append(tasks, task)
		if len(tasks) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	now := nowTime.Format(time.RFC3339Nano)
	validTasks := make([]profileTaskRecord, 0, len(tasks))
	for _, task := range tasks {
		if err := validateProfileTaskResourceConsistency(db, task); err != nil {
			if updateErr := failInvalidProfileTaskCandidate(db, task, err, now); updateErr != nil {
				return nil, updateErr
			}
			continue
		}
		validTasks = append(validTasks, task)
	}
	tasks = validTasks
	for index := range tasks {
		tasks[index].Status = "leased"
		tasks[index].LeasedAt = now
		tasks[index].UpdatedAt = now
		tasks[index] = withProfileTaskLeaseExpiry(tasks[index], leaseTimeout)
		if _, err := db.Exec(`
			UPDATE profile_tasks
			SET status = 'leased', leased_at = ?, updated_at = ?
			WHERE id = ? AND status IN ('pending', 'leased')
		`, now, now, tasks[index].ID); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func failInvalidProfileTaskCandidate(db *sql.DB, task profileTaskRecord, err error, now string) error {
	_, updateErr := db.Exec(`
		UPDATE profile_tasks
		SET status = 'failed', error = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND status IN ('pending', 'leased')
	`, err.Error(), now, now, task.ID)
	return updateErr
}

func profileTaskLeaseTimeoutFromEnv() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("PROFILE_TASK_LEASE_TIMEOUT")); raw != "" {
		if timeout, err := time.ParseDuration(raw); err == nil && timeout > 0 {
			return timeout
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PROFILE_TASK_LEASE_TIMEOUT_MS")); raw != "" {
		if timeoutMs, err := strconv.Atoi(raw); err == nil && timeoutMs > 0 {
			return time.Duration(timeoutMs) * time.Millisecond
		}
	}
	return defaultProfileTaskLeaseTimeout
}

func (store *profileTaskStore) effectiveLeaseTimeout() time.Duration {
	if store.leaseTimeout > 0 {
		return store.leaseTimeout
	}
	return defaultProfileTaskLeaseTimeout
}

func isProfileTaskLeaseExpired(leasedAt string, now time.Time, timeout time.Duration) bool {
	leasedAt = strings.TrimSpace(leasedAt)
	if leasedAt == "" {
		return true
	}
	leaseTime, err := time.Parse(time.RFC3339Nano, leasedAt)
	if err != nil {
		return true
	}
	return !leaseTime.Add(timeout).After(now)
}

func withProfileTaskLeaseExpiry(task profileTaskRecord, timeout time.Duration) profileTaskRecord {
	task.LeaseExpiresAt = ""
	if task.Status != "leased" || strings.TrimSpace(task.LeasedAt) == "" || timeout <= 0 {
		return task
	}
	leasedAt, err := time.Parse(time.RFC3339Nano, task.LeasedAt)
	if err != nil {
		return task
	}
	task.LeaseExpiresAt = leasedAt.Add(timeout).UTC().Format(time.RFC3339Nano)
	return task
}

func encodeProfileCommandArgs(args []string) string {
	if args == nil {
		args = []string{}
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(payload)
}

func decodeProfileCommandArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	return args, nil
}

func (store *profileTaskStore) complete(id string, input completeProfileTaskRequest) (profileTaskRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskRecord{}, false, err
	}
	defer db.Close()

	existing, found, err := getProfileTaskByID(db, id)
	if err != nil || !found {
		return profileTaskRecord{}, found, err
	}
	if err := validateProfileTaskResourceConsistency(db, existing); err != nil {
		return profileTaskRecord{}, true, err
	}
	input.ArtifactID = strings.TrimSpace(input.ArtifactID)
	input.Error = strings.TrimSpace(input.Error)
	if input.ArtifactID == "" && input.Error == "" {
		return profileTaskRecord{}, true, fmt.Errorf("artifactId or error is required")
	}

	attempts := existing.Attempts + 1
	maxAttempts := existing.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultProfileTaskMaxAttempts
	}
	status := "completed"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	artifactID := input.ArtifactID
	errorText := ""
	leasedAt := existing.LeasedAt
	completedAt := now
	if input.Error != "" {
		artifactID = ""
		errorText = input.Error
		if attempts < maxAttempts {
			status = "pending"
			leasedAt = ""
			completedAt = ""
		} else {
			status = "failed"
		}
	}
	_, err = db.Exec(`
		UPDATE profile_tasks
		SET status = ?, attempts = ?, max_attempts = ?, artifact_id = ?, error = ?,
			leased_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, status, attempts, maxAttempts, artifactID, errorText, leasedAt, completedAt, now, existing.ID)
	if err != nil {
		return profileTaskRecord{}, true, err
	}

	existing.Status = status
	existing.Attempts = attempts
	existing.MaxAttempts = maxAttempts
	existing.ArtifactID = artifactID
	existing.Error = errorText
	existing.LeasedAt = leasedAt
	existing.CompletedAt = completedAt
	existing.UpdatedAt = now
	return existing, true, nil
}

func (store *profileTaskStore) retry(id string) (profileTaskRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskRecord{}, false, err
	}
	defer db.Close()

	existing, found, err := getProfileTaskByID(db, id)
	if err != nil || !found {
		return profileTaskRecord{}, found, err
	}
	if existing.Status != "failed" {
		return profileTaskRecord{}, true, fmt.Errorf("only failed profile tasks can be retried")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		UPDATE profile_tasks
		SET status = 'pending', attempts = 0, artifact_id = '', error = '',
			leased_at = '', completed_at = '', updated_at = ?
		WHERE id = ?
	`, now, existing.ID)
	if err != nil {
		return profileTaskRecord{}, true, err
	}

	existing.Status = "pending"
	existing.Attempts = 0
	existing.ArtifactID = ""
	existing.Error = ""
	existing.LeasedAt = ""
	existing.CompletedAt = ""
	existing.UpdatedAt = now
	return existing, true, nil
}

func (store *profileTaskStore) get(id string) (profileTaskRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskRecord{}, false, err
	}
	defer db.Close()

	return getProfileTaskByID(db, id)
}

func (store *profileTaskStore) requireProfileTaskResourceConsistency(task profileTaskRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return validateProfileTaskResourceConsistency(db, task)
}

func (store *profileTaskStore) requireWorkspaceScopeForTask(r *http.Request, task profileTaskRecord) (int, error) {
	scope, ok := workspaceScopeFromRequest(r)
	if !ok {
		return 0, nil
	}

	scopes, err := store.lookupTaskWorkspaceScopes(task)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if len(scopes) == 0 {
		return http.StatusForbidden, workspaceScopeError("", "", scope)
	}
	for _, resourceScope := range scopes {
		if resourceScope.ProjectID != scope.ProjectID || resourceScope.Environment != scope.Environment {
			return http.StatusForbidden, workspaceScopeError(resourceScope.ProjectID, resourceScope.Environment, scope)
		}
	}
	return 0, nil
}

func (store *profileTaskStore) lookupTaskWorkspaceScopes(task profileTaskRecord) ([]workspaceScopeConstraint, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	scopes := []workspaceScopeConstraint{}
	if task.RunID != "" {
		projectID, environment, found, err := lookupRunWorkspaceScope(db, task.RunID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))
	}
	if task.AgentID != "" {
		projectID, environment, found, err := lookupAgentWorkspaceScope(db, task.AgentID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))
	}
	if task.TargetID != "" {
		projectID, environment, found, err := lookupTargetWorkspaceScope(db, task.TargetID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))
	}
	return scopes, nil
}

func lookupRunWorkspaceScope(db *sql.DB, runID string) (string, string, bool, error) {
	var projectID string
	var environment string
	err := db.QueryRow(`
		SELECT project_id, environment
		FROM run_history
		WHERE id = ?
	`, strings.TrimSpace(runID)).Scan(&projectID, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return projectID, environment, true, nil
}

func lookupAgentWorkspaceScope(db *sql.DB, agentID string) (string, string, bool, error) {
	var projectID string
	var environment string
	err := db.QueryRow(`
		SELECT project_id, environment
		FROM agents
		WHERE id = ?
	`, strings.TrimSpace(agentID)).Scan(&projectID, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return projectID, environment, true, nil
}

func lookupTargetWorkspaceScope(db *sql.DB, targetID string) (string, string, bool, error) {
	var projectID string
	var environment string
	err := db.QueryRow(`
		SELECT project_id, environment
		FROM targets
		WHERE id = ?
	`, strings.TrimSpace(targetID)).Scan(&projectID, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return projectID, environment, true, nil
}

func validateProfileTaskResourceConsistency(db *sql.DB, task profileTaskRecord) error {
	scopes := []workspaceScopeConstraint{}

	projectID, environment, found, err := lookupAgentWorkspaceScope(db, task.AgentID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("workspace agent %q not found", task.AgentID)
	}
	scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))

	if task.RunID != "" {
		projectID, environment, found, err = lookupRunWorkspaceScope(db, task.RunID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("workspace run %q not found", task.RunID)
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))
	}

	var target targetRecord
	if task.TargetID != "" {
		projectID, environment, found, err = lookupTargetWorkspaceScope(db, task.TargetID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("workspace target %q not found", task.TargetID)
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))

		target, found, err = getTargetByID(db, task.TargetID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("workspace target %q not found", task.TargetID)
		}
	}

	baseScope := scopes[0]
	for _, scope := range scopes[1:] {
		if scope.ProjectID != baseScope.ProjectID || scope.Environment != baseScope.Environment {
			return workspaceScopeError(scope.ProjectID, scope.Environment, baseScope)
		}
	}
	if task.Source == targetManualProfileTaskSource && !targetHasAgent(target, task.AgentID) {
		return fmt.Errorf("agent %q is not assigned to target %q for target_manual profile task", task.AgentID, task.TargetID)
	}
	return nil
}

func targetHasAgent(target targetRecord, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	for _, targetAgentID := range target.AgentIDs {
		if strings.TrimSpace(targetAgentID) == agentID {
			return true
		}
	}
	return false
}

func (store *profileTaskStore) open() (*sql.DB, error) {
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
	if err := ensureProfileTaskSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureRunHistorySchema(db); err != nil {
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
	return db, nil
}

func ensureProfileTaskSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profile_tasks (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			scenario_id TEXT NOT NULL,
			scenario_name TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_name TEXT NOT NULL,
			pprof_base_url TEXT NOT NULL,
			profile_url TEXT NOT NULL,
			profile_type TEXT NOT NULL,
			profile_seconds INTEGER NOT NULL,
			profile_command TEXT NOT NULL DEFAULT '',
			profile_command_args TEXT NOT NULL DEFAULT '[]',
			profile_command_output TEXT NOT NULL DEFAULT '',
			profile_command_timeout_ms INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT 'api',
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			artifact_id TEXT NOT NULL,
			error TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			leased_at TEXT NOT NULL,
			completed_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "attempts", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "source", "TEXT NOT NULL DEFAULT 'api'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "profile_command", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "profile_command_args", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "profile_command_output", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "profile_tasks", "profile_command_timeout_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return ensureSQLiteColumn(db, "profile_tasks", "max_attempts", "INTEGER NOT NULL DEFAULT 3")
}

func getProfileTaskByID(db *sql.DB, id string) (profileTaskRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, agent_id, run_id, scenario_id, scenario_name, target_id, target_name,
			pprof_base_url, profile_url, profile_type, profile_seconds,
			profile_command, profile_command_args, profile_command_output, profile_command_timeout_ms,
			source, status,
			attempts, max_attempts, artifact_id, error, created_at, updated_at, leased_at, completed_at
		FROM profile_tasks
		WHERE id = ?
	`, strings.TrimSpace(id))
	task, err := scanProfileTask(row)
	if err == nil {
		return task, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return profileTaskRecord{}, false, nil
	}
	return profileTaskRecord{}, false, err
}

func scanProfileTask(scanner profileTaskScanner) (profileTaskRecord, error) {
	var task profileTaskRecord
	var profileCommandArgs string
	err := scanner.Scan(
		&task.ID,
		&task.AgentID,
		&task.RunID,
		&task.ScenarioID,
		&task.ScenarioName,
		&task.TargetID,
		&task.TargetName,
		&task.PprofBaseURL,
		&task.ProfileURL,
		&task.ProfileType,
		&task.ProfileSeconds,
		&task.ProfileCommand,
		&profileCommandArgs,
		&task.ProfileCommandOutput,
		&task.ProfileCommandTimeoutMs,
		&task.Source,
		&task.Status,
		&task.Attempts,
		&task.MaxAttempts,
		&task.ArtifactID,
		&task.Error,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.LeasedAt,
		&task.CompletedAt,
	)
	if err != nil {
		return profileTaskRecord{}, err
	}
	task.ProfileCommandArgs, err = decodeProfileCommandArgs(profileCommandArgs)
	if err != nil {
		return profileTaskRecord{}, err
	}
	return task, nil
}

func normalizeProfileTask(input profileTaskRecord) profileTaskRecord {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.ScenarioID = strings.TrimSpace(input.ScenarioID)
	input.ScenarioName = strings.TrimSpace(input.ScenarioName)
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.TargetName = strings.TrimSpace(input.TargetName)
	input.PprofBaseURL = strings.TrimRight(strings.TrimSpace(input.PprofBaseURL), "/")
	input.ProfileURL = strings.TrimSpace(input.ProfileURL)
	input.ProfileCommand = strings.TrimSpace(input.ProfileCommand)
	input.ProfileCommandOutput = strings.TrimSpace(input.ProfileCommandOutput)
	if input.ProfileCommandTimeoutMs < 0 {
		input.ProfileCommandTimeoutMs = 0
	}
	input.ProfileType = strings.ToLower(strings.TrimSpace(input.ProfileType))
	if input.ProfileType == "" {
		if input.ProfileCommand != "" {
			input.ProfileType = "command"
		} else {
			input.ProfileType = "cpu"
		}
	}
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = defaultProfileTaskSource
	}
	if input.Attempts < 0 {
		input.Attempts = 0
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = defaultProfileTaskMaxAttempts
	}
	if input.MaxAttempts > maxProfileTaskMaxAttempts {
		input.MaxAttempts = maxProfileTaskMaxAttempts
	}
	return input
}

func validateProfileTask(input profileTaskRecord) error {
	if input.AgentID == "" {
		return fmt.Errorf("agentId is required")
	}
	if input.Source == targetManualProfileTaskSource {
		if input.TargetID == "" {
			return fmt.Errorf("targetId is required for target_manual profile task")
		}
	} else if input.RunID == "" {
		return fmt.Errorf("runId is required")
	}
	if input.PprofBaseURL == "" && input.ProfileURL == "" && input.ProfileCommand == "" {
		return fmt.Errorf("pprofBaseUrl, profileUrl, or profileCommand is required")
	}
	if input.ProfileSeconds < 1 || input.ProfileSeconds > maxProfileTaskSeconds {
		return fmt.Errorf("profileSeconds must be between 1 and %d", maxProfileTaskSeconds)
	}
	if !isValidProfileTaskSource(input.Source) {
		return fmt.Errorf("source must be api, run_auto, target_manual, or threshold_auto")
	}
	return nil
}

func isValidProfileTaskSource(source string) bool {
	return source == defaultProfileTaskSource ||
		source == runAutoProfileTaskSource ||
		source == targetManualProfileTaskSource ||
		source == thresholdAutoProfileTaskSource
}
