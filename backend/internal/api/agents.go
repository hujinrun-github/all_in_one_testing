package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
)

const (
	defaultAgentProjectID   = "default"
	defaultAgentEnvironment = "default"
)

type agentRecord struct {
	ID               string              `json:"id"`
	TokenID          string              `json:"tokenId"`
	ProjectID        string              `json:"projectId"`
	Environment      string              `json:"environment"`
	Name             string              `json:"name"`
	Hostname         string              `json:"hostname"`
	IP               string              `json:"ip"`
	Version          string              `json:"version"`
	Status           string              `json:"status"`
	Labels           map[string]string   `json:"labels"`
	Capabilities     []string            `json:"capabilities"`
	LastSeenAt       string              `json:"lastSeenAt"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
	LatestMetrics    *agentMetricsRecord `json:"latestMetrics,omitempty"`
	LatestVersion    string              `json:"latestVersion,omitempty"`
	UpgradeAvailable bool                `json:"upgradeAvailable"`
}

type agentMetricsRecord struct {
	AgentID              string               `json:"agentId"`
	CollectedAt          string               `json:"collectedAt"`
	CPUUsagePercent      float64              `json:"cpuUsagePercent"`
	MemoryUsagePercent   float64              `json:"memoryUsagePercent"`
	DiskReadBytesPerSec  float64              `json:"diskReadBytesPerSec"`
	DiskWriteBytesPerSec float64              `json:"diskWriteBytesPerSec"`
	NetworkRxBytesPerSec float64              `json:"networkRxBytesPerSec"`
	NetworkTxBytesPerSec float64              `json:"networkTxBytesPerSec"`
	Processes            []agentProcessMetric `json:"processes"`
}

type agentTokenRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProjectID   string `json:"projectId"`
	Environment string `json:"environment"`
	Token       string `json:"token,omitempty"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	LastUsedAt  string `json:"lastUsedAt,omitempty"`
	RotatedAt   string `json:"rotatedAt,omitempty"`
}

type createAgentTokenRequest struct {
	Name             string `json:"name"`
	ProjectID        string `json:"projectId"`
	Environment      string `json:"environment"`
	ExpiresAt        string `json:"expiresAt"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

type agentListFilter struct {
	ProjectID   string
	Environment string
}

type agentProcessMetric struct {
	PID             int     `json:"pid"`
	Name            string  `json:"name"`
	Cmdline         string  `json:"cmdline,omitempty"`
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	MemoryRSSBytes  int64   `json:"memoryRssBytes"`
	FDCount         int     `json:"fdCount"`
	ThreadCount     int     `json:"threadCount"`
}

type agentTargetHealthCheckReport struct {
	AgentID        string  `json:"agentId"`
	TargetID       string  `json:"targetId"`
	Status         string  `json:"status"`
	URL            string  `json:"url"`
	ExpectedStatus int     `json:"expectedStatus"`
	ObservedStatus int     `json:"observedStatus"`
	LatencyMs      float64 `json:"latencyMs"`
	Error          string  `json:"error"`
	CheckedAt      string  `json:"checkedAt"`
}

type agentStore struct {
	path string
	mu   sync.Mutex
}

type agentScanner interface {
	Scan(dest ...any) error
}

func newAgentStoreFromEnv() *agentStore {
	return &agentStore{path: scenarioDatabasePathFromEnv()}
}

func handleCreateAgentToken(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createAgentTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		if err := applyWorkspaceScopeToRecord(r, &input.ProjectID, &input.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		token, err := store.createToken(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, token)
	}
}

func handleRevokeAgentToken(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenID := chi.URLParam(r, "id")
		existing, found, err := store.getToken(tokenID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent token not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, existing.ProjectID, existing.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		token, found, err := store.revokeToken(tokenID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent token not found"})
			return
		}

		writeJSON(w, http.StatusOK, token)
	}
}

func handleRotateAgentToken(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createAgentTokenRequest
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
				return
			}
		}

		tokenID := chi.URLParam(r, "id")
		existing, found, err := store.getToken(tokenID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent token not found"})
			return
		}
		if err := requireWorkspaceScopeForRecord(r, existing.ProjectID, existing.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		token, found, err := store.rotateToken(tokenID, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent token not found"})
			return
		}

		writeJSON(w, http.StatusOK, token)
	}
}

func handleAgentHeartbeat(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok, err := store.authenticateRequest(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "valid agent token is required"})
			return
		}

		var input agentRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		input.TokenID = token.ID
		input.ProjectID = token.ProjectID
		input.Environment = token.Environment

		agent, err := store.upsertHeartbeat(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, agent)
	}
}

func handleListAgents(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := agentListFilterFromRequest(r)
		if err := applyWorkspaceScopeToFilter(r, &filter.ProjectID, &filter.Environment); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		agents, err := store.list(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		latestVersion, err := latestAgentReleaseVersion()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		enrichAgentsWithUpgradeStatus(agents, latestVersion)

		writeJSON(w, http.StatusOK, agents)
	}
}

func enrichAgentsWithUpgradeStatus(agents []agentRecord, latestVersion string) {
	latestVersion = strings.TrimSpace(latestVersion)
	if latestVersion == "" {
		return
	}
	for index := range agents {
		agents[index].LatestVersion = latestVersion
		agents[index].UpgradeAvailable = strings.TrimSpace(agents[index].Version) != latestVersion
	}
}

func agentListFilterFromRequest(r *http.Request) agentListFilter {
	query := r.URL.Query()
	return agentListFilter{
		ProjectID:   strings.TrimSpace(query.Get("projectId")),
		Environment: strings.TrimSpace(query.Get("environment")),
	}
}

func handleListAgentMetrics(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
		if agentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agentId is required"})
			return
		}
		limit := 500
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil || parsedLimit <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = parsedLimit
			if limit > 1000 {
				limit = 1000
			}
		}
		if status, err := store.requireWorkspaceScopeForAgent(r, agentID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		metrics, err := store.listMetrics(agentID, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to")), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, metrics)
	}
}

func handleAgentMetrics(store *agentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok, err := store.authenticateRequest(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "valid agent token is required"})
			return
		}

		var input agentMetricsRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		if status, err := store.requireTokenForAgent(token, input.AgentID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		metrics, err := store.saveMetrics(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusAccepted, metrics)
	}
}

func handleAgentTargets(agentStore *agentStore, targetStore *targetStore) http.HandlerFunc {
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

		targets, err := targetStore.list(targetListFilter{
			ProjectID:   normalizeAgentDimension(token.ProjectID, defaultAgentProjectID),
			Environment: normalizeAgentDimension(token.Environment, defaultAgentEnvironment),
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		boundTargets := make([]targetRecord, 0, len(targets))
		for _, target := range targets {
			if targetBoundToAgent(target, agentID) {
				boundTargets = append(boundTargets, target)
			}
		}

		writeJSON(w, http.StatusOK, boundTargets)
	}
}

func handleAgentTargetHealthChecks(agentStore *agentStore, targetStore *targetStore) http.HandlerFunc {
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

		var input agentTargetHealthCheckReport
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		agentID := strings.TrimSpace(input.AgentID)
		if status, err := agentStore.requireTokenForAgent(token, agentID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		target, found, err := targetStore.get(input.TargetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target not found"})
			return
		}
		if !agentTokenCanAccessWorkspace(token, target.ProjectID, target.Environment) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": agentTokenWorkspaceError(token, target.ProjectID, target.Environment).Error()})
			return
		}
		if !targetBoundToAgent(target, agentID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("target %s is not bound to agent %s", target.ID, agentID)})
			return
		}

		result, err := normalizeAgentTargetHealthCheckReport(input, target)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := targetStore.updateLastHealthCheck(target.ID, result); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusAccepted, result)
	}
}

func (store *agentStore) createToken(input createAgentTokenRequest) (agentTokenRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentTokenRecord{}, err
	}
	defer db.Close()

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return agentTokenRecord{}, fmt.Errorf("name is required")
	}
	input.ProjectID = normalizeAgentDimension(input.ProjectID, defaultAgentProjectID)
	input.Environment = normalizeAgentDimension(input.Environment, defaultAgentEnvironment)
	nowTime := time.Now().UTC()
	expiresAt, err := normalizeAgentTokenExpiresAt(input, nowTime)
	if err != nil {
		return agentTokenRecord{}, err
	}

	rawToken, err := generateAgentToken()
	if err != nil {
		return agentTokenRecord{}, err
	}

	now := nowTime.Format(time.RFC3339Nano)
	token := agentTokenRecord{
		ID:          newResourceID("agent-token"),
		Name:        input.Name,
		ProjectID:   input.ProjectID,
		Environment: input.Environment,
		Token:       rawToken,
		Status:      "active",
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = db.Exec(`
		INSERT INTO agent_tokens (
			id, name, project_id, environment, token_hash, status, expires_at, created_at, updated_at,
			last_used_at, revoked_at, rotated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '')
	`, token.ID, token.Name, token.ProjectID, token.Environment, hashAgentToken(rawToken), token.Status, token.ExpiresAt, token.CreatedAt, token.UpdatedAt)
	if err != nil {
		return agentTokenRecord{}, err
	}

	return token, nil
}

func (store *agentStore) getToken(id string) (agentTokenRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentTokenRecord{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT id, name, project_id, environment, status, expires_at, created_at, updated_at, last_used_at, rotated_at
		FROM agent_tokens
		WHERE id = ?
	`, strings.TrimSpace(id))
	var token agentTokenRecord
	if err := row.Scan(&token.ID, &token.Name, &token.ProjectID, &token.Environment, &token.Status, &token.ExpiresAt, &token.CreatedAt, &token.UpdatedAt, &token.LastUsedAt, &token.RotatedAt); err != nil {
		if err == sql.ErrNoRows {
			return agentTokenRecord{}, false, nil
		}
		return agentTokenRecord{}, false, err
	}
	return token, true, nil
}

func (store *agentStore) revokeToken(id string) (agentTokenRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentTokenRecord{}, false, err
	}
	defer db.Close()

	id = strings.TrimSpace(id)
	row := db.QueryRow(`
		SELECT id, name, project_id, environment, status, expires_at, created_at, updated_at, last_used_at, rotated_at
		FROM agent_tokens
		WHERE id = ?
	`, id)
	var token agentTokenRecord
	if err := row.Scan(&token.ID, &token.Name, &token.ProjectID, &token.Environment, &token.Status, &token.ExpiresAt, &token.CreatedAt, &token.UpdatedAt, &token.LastUsedAt, &token.RotatedAt); err != nil {
		if err == sql.ErrNoRows {
			return agentTokenRecord{}, false, nil
		}
		return agentTokenRecord{}, false, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		UPDATE agent_tokens
		SET status = 'revoked', revoked_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, id)
	if err != nil {
		return agentTokenRecord{}, false, err
	}

	token.Status = "revoked"
	token.UpdatedAt = now
	token.Token = ""
	return token, true, nil
}

func (store *agentStore) rotateToken(id string, input createAgentTokenRequest) (agentTokenRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentTokenRecord{}, false, err
	}
	defer db.Close()

	id = strings.TrimSpace(id)
	row := db.QueryRow(`
		SELECT id, name, project_id, environment, status, expires_at, created_at, updated_at, last_used_at, rotated_at
		FROM agent_tokens
		WHERE id = ?
	`, id)
	var token agentTokenRecord
	if err := row.Scan(&token.ID, &token.Name, &token.ProjectID, &token.Environment, &token.Status, &token.ExpiresAt, &token.CreatedAt, &token.UpdatedAt, &token.LastUsedAt, &token.RotatedAt); err != nil {
		if err == sql.ErrNoRows {
			return agentTokenRecord{}, false, nil
		}
		return agentTokenRecord{}, false, err
	}
	if token.Status != "active" {
		return agentTokenRecord{}, true, fmt.Errorf("only active agent tokens can be rotated")
	}

	nowTime := time.Now().UTC()
	expiresAt, err := normalizeAgentTokenExpiresAt(input, nowTime)
	if err != nil {
		return agentTokenRecord{}, true, err
	}
	if expiresAt == "" {
		expiresAt = token.ExpiresAt
	}

	rawToken, err := generateAgentToken()
	if err != nil {
		return agentTokenRecord{}, true, err
	}

	now := nowTime.Format(time.RFC3339Nano)
	_, err = db.Exec(`
		UPDATE agent_tokens
		SET token_hash = ?, status = 'active', expires_at = ?, rotated_at = ?, revoked_at = '', updated_at = ?
		WHERE id = ?
	`, hashAgentToken(rawToken), expiresAt, now, now, token.ID)
	if err != nil {
		return agentTokenRecord{}, true, err
	}

	token.Token = rawToken
	token.Status = "active"
	token.ExpiresAt = expiresAt
	token.RotatedAt = now
	token.UpdatedAt = now
	return token, true, nil
}

func (store *agentStore) authenticateRequest(r *http.Request) (agentTokenRecord, bool, error) {
	rawToken := bearerTokenFromHeader(r.Header.Get("Authorization"))
	if rawToken == "" {
		return agentTokenRecord{}, false, nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentTokenRecord{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT id, name, project_id, environment, status, expires_at, created_at, updated_at, last_used_at, rotated_at
		FROM agent_tokens
		WHERE token_hash = ? AND status = 'active' AND revoked_at = ''
	`, hashAgentToken(rawToken))

	var token agentTokenRecord
	if err := row.Scan(&token.ID, &token.Name, &token.ProjectID, &token.Environment, &token.Status, &token.ExpiresAt, &token.CreatedAt, &token.UpdatedAt, &token.LastUsedAt, &token.RotatedAt); err != nil {
		if err == sql.ErrNoRows {
			return agentTokenRecord{}, false, nil
		}
		return agentTokenRecord{}, false, err
	}
	if agentTokenExpired(token.ExpiresAt, time.Now().UTC()) {
		return agentTokenRecord{}, false, nil
	}

	token.LastUsedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		UPDATE agent_tokens
		SET last_used_at = ?, updated_at = ?
		WHERE id = ?
	`, token.LastUsedAt, token.LastUsedAt, token.ID)
	if err != nil {
		return agentTokenRecord{}, false, err
	}

	return token, true, nil
}

func normalizeAgentTokenExpiresAt(input createAgentTokenRequest, now time.Time) (string, error) {
	expiresAt := strings.TrimSpace(input.ExpiresAt)
	if expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return "", fmt.Errorf("expiresAt must be RFC3339")
		}
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}
	if input.ExpiresInSeconds > 0 {
		return now.Add(time.Duration(input.ExpiresInSeconds) * time.Second).UTC().Format(time.RFC3339Nano), nil
	}
	return "", nil
}

func agentTokenExpired(expiresAt string, now time.Time) bool {
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return true
	}
	return !parsed.After(now)
}

func (store *agentStore) upsertHeartbeat(input agentRecord) (agentRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentRecord{}, err
	}
	defer db.Close()

	input = normalizeAgent(input)
	if err := validateAgent(input); err != nil {
		return agentRecord{}, err
	}

	existing, found, err := getAgentByID(db, input.ID)
	if err != nil {
		return agentRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.Status = "online"
	input.LastSeenAt = now
	input.UpdatedAt = now
	if found {
		input.CreatedAt = existing.CreatedAt
	} else {
		input.CreatedAt = now
	}

	labels, capabilities, err := encodeAgentCollections(input)
	if err != nil {
		return agentRecord{}, err
	}
	_, err = db.Exec(`
		INSERT INTO agents (
			id, token_id, project_id, environment, name, hostname, ip, version, status, labels, capabilities,
			last_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			token_id = excluded.token_id,
			project_id = excluded.project_id,
			environment = excluded.environment,
			name = excluded.name,
			hostname = excluded.hostname,
			ip = excluded.ip,
			version = excluded.version,
			status = excluded.status,
			labels = excluded.labels,
			capabilities = excluded.capabilities,
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at
	`, input.ID, input.TokenID, input.ProjectID, input.Environment, input.Name, input.Hostname, input.IP, input.Version, input.Status, labels, capabilities,
		input.LastSeenAt, input.CreatedAt, input.UpdatedAt)
	if err != nil {
		return agentRecord{}, err
	}
	return input, nil
}

func (store *agentStore) list(filter agentListFilter) ([]agentRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT id, token_id, project_id, environment, name, hostname, ip, version, status, labels, capabilities,
			last_seen_at, created_at, updated_at
		FROM agents
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
	query += " ORDER BY name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	agents := []agentRecord{}
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	for index := range agents {
		latestMetrics, found, err := getLatestAgentMetrics(db, agents[index].ID)
		if err != nil {
			return nil, err
		}
		if found {
			agents[index].LatestMetrics = &latestMetrics
		}
	}
	return agents, nil
}

func (store *agentStore) requireWorkspaceScopeForAgent(r *http.Request, agentID string) (int, error) {
	if _, ok := workspaceScopeFromRequest(r); !ok {
		return 0, nil
	}

	agent, found, err := store.get(agentID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !found {
		return http.StatusNotFound, fmt.Errorf("agent not found")
	}
	if err := requireWorkspaceScopeForRecord(r, agent.ProjectID, agent.Environment); err != nil {
		return http.StatusForbidden, err
	}
	return 0, nil
}

func (store *agentStore) requireTokenForAgent(token agentTokenRecord, agentID string) (int, error) {
	agent, found, err := store.get(agentID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !found {
		return http.StatusNotFound, fmt.Errorf("agent not found")
	}

	if strings.TrimSpace(agent.TokenID) != strings.TrimSpace(token.ID) {
		return http.StatusForbidden, fmt.Errorf("agent token cannot access agent %s", agent.ID)
	}
	if !agentTokenCanAccessWorkspace(token, agent.ProjectID, agent.Environment) {
		return http.StatusForbidden, agentTokenWorkspaceError(token, agent.ProjectID, agent.Environment)
	}
	return 0, nil
}

func (store *agentStore) requireTokenForRunIfKnown(token agentTokenRecord, runID string) (int, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return http.StatusBadRequest, fmt.Errorf("runId is required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer db.Close()
	if err := ensureRunHistorySchema(db); err != nil {
		return http.StatusInternalServerError, err
	}

	projectID, environment, found, err := lookupRunWorkspaceScope(db, runID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !found {
		return 0, nil
	}
	if !agentTokenCanAccessWorkspace(token, projectID, environment) {
		return http.StatusForbidden, agentTokenWorkspaceError(token, projectID, environment)
	}
	return 0, nil
}

func agentTokenCanAccessWorkspace(token agentTokenRecord, projectID string, environment string) bool {
	tokenProjectID := normalizeAgentDimension(token.ProjectID, defaultAgentProjectID)
	tokenEnvironment := normalizeAgentDimension(token.Environment, defaultAgentEnvironment)
	recordProjectID := normalizeAgentDimension(projectID, defaultAgentProjectID)
	recordEnvironment := normalizeAgentDimension(environment, defaultAgentEnvironment)
	return tokenProjectID == recordProjectID && tokenEnvironment == recordEnvironment
}

func agentTokenWorkspaceError(token agentTokenRecord, projectID string, environment string) error {
	tokenProjectID := normalizeAgentDimension(token.ProjectID, defaultAgentProjectID)
	tokenEnvironment := normalizeAgentDimension(token.Environment, defaultAgentEnvironment)
	recordProjectID := normalizeAgentDimension(projectID, defaultAgentProjectID)
	recordEnvironment := normalizeAgentDimension(environment, defaultAgentEnvironment)
	return fmt.Errorf("agent token scope %s/%s cannot access %s/%s", tokenProjectID, tokenEnvironment, recordProjectID, recordEnvironment)
}

func targetBoundToAgent(target targetRecord, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	for _, boundAgentID := range target.AgentIDs {
		if strings.TrimSpace(boundAgentID) == agentID {
			return true
		}
	}
	return false
}

func normalizeAgentTargetHealthCheckReport(input agentTargetHealthCheckReport, target targetRecord) (targetHealthCheckResult, error) {
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return targetHealthCheckResult{}, fmt.Errorf("targetId is required")
	}
	if targetID != target.ID {
		return targetHealthCheckResult{}, fmt.Errorf("targetId does not match target")
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case "healthy", "unhealthy", "not_configured":
	case "":
		return targetHealthCheckResult{}, fmt.Errorf("status is required")
	default:
		return targetHealthCheckResult{}, fmt.Errorf("status must be healthy, unhealthy, or not_configured")
	}

	checkedAt := strings.TrimSpace(input.CheckedAt)
	if checkedAt == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		parsed, err := time.Parse(time.RFC3339Nano, checkedAt)
		if err != nil {
			return targetHealthCheckResult{}, fmt.Errorf("checkedAt must be RFC3339")
		}
		checkedAt = parsed.UTC().Format(time.RFC3339Nano)
	}

	expectedStatus := input.ExpectedStatus
	if expectedStatus <= 0 {
		expectedStatus = target.HealthCheck.ExpectedStatus
	}
	if expectedStatus <= 0 {
		expectedStatus = http.StatusOK
	}
	latencyMs := input.LatencyMs
	if latencyMs < 0 {
		latencyMs = 0
	}

	return targetHealthCheckResult{
		TargetID:       target.ID,
		TargetName:     target.Name,
		Status:         status,
		Source:         "agent_daemon",
		URL:            strings.TrimSpace(input.URL),
		ExpectedStatus: expectedStatus,
		ObservedStatus: input.ObservedStatus,
		LatencyMs:      latencyMs,
		Error:          strings.TrimSpace(input.Error),
		CheckedAt:      checkedAt,
	}, nil
}

func (store *agentStore) get(id string) (agentRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentRecord{}, false, err
	}
	defer db.Close()

	return getAgentByID(db, id)
}

func (store *agentStore) listMetrics(agentID string, from string, to string, limit int) ([]agentMetricsRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if limit <= 0 {
		limit = 500
	}
	query := `
		SELECT agent_id, collected_at, cpu_usage_percent, memory_usage_percent,
			disk_read_bytes_per_sec, disk_write_bytes_per_sec, network_rx_bytes_per_sec, network_tx_bytes_per_sec, processes
		FROM agent_metrics
		WHERE agent_id = ?
	`
	args := []any{strings.TrimSpace(agentID)}
	if from != "" {
		query += " AND collected_at >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND collected_at <= ?"
		args = append(args, to)
	}
	query += " ORDER BY collected_at ASC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metrics := []agentMetricsRecord{}
	for rows.Next() {
		sample, err := scanAgentMetrics(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (store *agentStore) saveMetrics(input agentMetricsRecord) (agentMetricsRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return agentMetricsRecord{}, err
	}
	defer db.Close()

	input = normalizeAgentMetrics(input)
	if err := validateAgentMetrics(input); err != nil {
		return agentMetricsRecord{}, err
	}
	if _, found, err := getAgentByID(db, input.AgentID); err != nil {
		return agentMetricsRecord{}, err
	} else if !found {
		return agentMetricsRecord{}, fmt.Errorf("agent not found")
	}

	processes, err := json.Marshal(input.Processes)
	if err != nil {
		return agentMetricsRecord{}, err
	}
	_, err = db.Exec(`
		INSERT INTO agent_metrics (
			agent_id, collected_at, cpu_usage_percent, memory_usage_percent,
			disk_read_bytes_per_sec, disk_write_bytes_per_sec,
			network_rx_bytes_per_sec, network_tx_bytes_per_sec, processes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.AgentID, input.CollectedAt, input.CPUUsagePercent, input.MemoryUsagePercent,
		input.DiskReadBytesPerSec, input.DiskWriteBytesPerSec,
		input.NetworkRxBytesPerSec, input.NetworkTxBytesPerSec, string(processes))
	if err != nil {
		return agentMetricsRecord{}, err
	}
	return input, nil
}

func (store *agentStore) open() (*sql.DB, error) {
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
	return db, nil
}

func ensureAgentSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL DEFAULT '',
			project_id TEXT NOT NULL DEFAULT 'default',
			environment TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			hostname TEXT NOT NULL,
			ip TEXT NOT NULL,
			version TEXT NOT NULL,
			status TEXT NOT NULL,
			labels TEXT NOT NULL,
			capabilities TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agents", "token_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agents", "project_id", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agents", "environment", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_tokens (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT 'default',
			environment TEXT NOT NULL DEFAULT 'default',
			token_hash TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT '',
			revoked_at TEXT NOT NULL DEFAULT '',
			rotated_at TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agent_tokens", "expires_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agent_tokens", "project_id", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agent_tokens", "environment", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(db, "agent_tokens", "rotated_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_metrics (
			agent_id TEXT NOT NULL,
			collected_at TEXT NOT NULL,
			cpu_usage_percent REAL NOT NULL,
			memory_usage_percent REAL NOT NULL,
			disk_read_bytes_per_sec REAL NOT NULL,
			disk_write_bytes_per_sec REAL NOT NULL,
			network_rx_bytes_per_sec REAL NOT NULL,
			network_tx_bytes_per_sec REAL NOT NULL,
			processes TEXT NOT NULL,
			PRIMARY KEY (agent_id, collected_at)
		)
	`)
	return err
}

func getAgentByID(db *sql.DB, id string) (agentRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, token_id, project_id, environment, name, hostname, ip, version, status, labels, capabilities,
			last_seen_at, created_at, updated_at
		FROM agents
		WHERE id = ?
	`, id)
	agent, err := scanAgent(row)
	if err == nil {
		return agent, true, nil
	}
	if err == sql.ErrNoRows {
		return agentRecord{}, false, nil
	}
	return agentRecord{}, false, err
}

func scanAgent(scanner agentScanner) (agentRecord, error) {
	var agent agentRecord
	var labels string
	var capabilities string
	err := scanner.Scan(
		&agent.ID,
		&agent.TokenID,
		&agent.ProjectID,
		&agent.Environment,
		&agent.Name,
		&agent.Hostname,
		&agent.IP,
		&agent.Version,
		&agent.Status,
		&labels,
		&capabilities,
		&agent.LastSeenAt,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)
	if err != nil {
		return agentRecord{}, err
	}
	if err := decodeAgentCollection("labels", labels, &agent.Labels); err != nil {
		return agentRecord{}, err
	}
	if err := decodeAgentCollection("capabilities", capabilities, &agent.Capabilities); err != nil {
		return agentRecord{}, err
	}
	return agent, nil
}

func getLatestAgentMetrics(db *sql.DB, agentID string) (agentMetricsRecord, bool, error) {
	row := db.QueryRow(`
		SELECT agent_id, collected_at, cpu_usage_percent, memory_usage_percent,
			disk_read_bytes_per_sec, disk_write_bytes_per_sec,
			network_rx_bytes_per_sec, network_tx_bytes_per_sec, processes
		FROM agent_metrics
		WHERE agent_id = ?
		ORDER BY collected_at DESC
		LIMIT 1
	`, agentID)
	metrics, err := scanAgentMetrics(row)
	if err == nil {
		return metrics, true, nil
	}
	if err == sql.ErrNoRows {
		return agentMetricsRecord{}, false, nil
	}
	return agentMetricsRecord{}, false, err
}

func scanAgentMetrics(scanner agentScanner) (agentMetricsRecord, error) {
	var metrics agentMetricsRecord
	var processes string
	err := scanner.Scan(
		&metrics.AgentID,
		&metrics.CollectedAt,
		&metrics.CPUUsagePercent,
		&metrics.MemoryUsagePercent,
		&metrics.DiskReadBytesPerSec,
		&metrics.DiskWriteBytesPerSec,
		&metrics.NetworkRxBytesPerSec,
		&metrics.NetworkTxBytesPerSec,
		&processes,
	)
	if err != nil {
		return agentMetricsRecord{}, err
	}
	if err := decodeAgentCollection("processes", processes, &metrics.Processes); err != nil {
		return agentMetricsRecord{}, err
	}
	return metrics, nil
}

func normalizeAgent(input agentRecord) agentRecord {
	input.ID = strings.TrimSpace(input.ID)
	input.TokenID = strings.TrimSpace(input.TokenID)
	input.ProjectID = normalizeAgentDimension(input.ProjectID, defaultAgentProjectID)
	input.Environment = normalizeAgentDimension(input.Environment, defaultAgentEnvironment)
	input.Name = strings.TrimSpace(input.Name)
	input.Hostname = strings.TrimSpace(input.Hostname)
	input.IP = strings.TrimSpace(input.IP)
	input.Version = strings.TrimSpace(input.Version)
	if input.ID == "" {
		input.ID = agentIDFromName(input.Name)
	}
	if input.Labels == nil {
		input.Labels = map[string]string{}
	}
	capabilities := make([]string, 0, len(input.Capabilities))
	for _, capability := range input.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			capabilities = append(capabilities, capability)
		}
	}
	input.Capabilities = capabilities
	return input
}

func normalizeAgentMetrics(input agentMetricsRecord) agentMetricsRecord {
	input.AgentID = strings.TrimSpace(input.AgentID)
	if strings.TrimSpace(input.CollectedAt) == "" {
		input.CollectedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	processes := make([]agentProcessMetric, 0, len(input.Processes))
	for _, process := range input.Processes {
		process.Name = strings.TrimSpace(process.Name)
		process.Cmdline = strings.TrimSpace(process.Cmdline)
		if process.PID <= 0 && process.Name == "" {
			continue
		}
		processes = append(processes, process)
	}
	input.Processes = processes
	return input
}

func validateAgent(input agentRecord) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if input.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	return nil
}

func validateAgentMetrics(input agentMetricsRecord) error {
	if input.AgentID == "" {
		return fmt.Errorf("agentId is required")
	}
	return nil
}

func normalizeAgentDimension(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func agentIDFromName(name string) string {
	var builder strings.Builder
	for _, value := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			builder.WriteRune(value)
			continue
		}
		builder.WriteByte('-')
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		return ""
	}
	return "agent-" + id
}

func encodeAgentCollections(agent agentRecord) (string, string, error) {
	labels, err := json.Marshal(agent.Labels)
	if err != nil {
		return "", "", err
	}
	capabilities, err := json.Marshal(agent.Capabilities)
	if err != nil {
		return "", "", err
	}
	return string(labels), string(capabilities), nil
}

func decodeAgentCollection(field string, data string, target any) error {
	if strings.TrimSpace(data) == "" {
		data = "[]"
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("agent %s contains invalid JSON: %w", field, err)
	}
	return nil
}

func ensureSQLiteColumn(db *sql.DB, tableName string, columnName string, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition))
	return err
}

func bearerTokenFromHeader(authorization string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
}

func generateAgentToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return "ait_" + base64.RawURLEncoding.EncodeToString(data), nil
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
