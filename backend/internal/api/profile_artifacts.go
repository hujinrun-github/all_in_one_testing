package api

import (
	"database/sql"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type profileArtifactRecord struct {
	ID           string `json:"id"`
	RunID        string `json:"runId"`
	ScenarioID   string `json:"scenarioId,omitempty"`
	ScenarioName string `json:"scenarioName,omitempty"`
	TargetID     string `json:"targetId,omitempty"`
	TargetName   string `json:"targetName,omitempty"`
	ProfileType  string `json:"profileType"`
	Status       string `json:"status"`
	SourceURL    string `json:"sourceUrl"`
	FileName     string `json:"fileName,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	SizeBytes    int64  `json:"sizeBytes"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt"`
	StoragePath  string `json:"-"`
}

type profileArtifactStore struct {
	path string
	mu   sync.Mutex
}

type profileArtifactScanner interface {
	Scan(dest ...any) error
}

func newProfileArtifactStoreFromEnv() *profileArtifactStore {
	return &profileArtifactStore{path: scenarioDatabasePathFromEnv()}
}

func handleListProfileArtifacts(store *profileArtifactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := runListFilterFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		artifacts, err := store.listFiltered(filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, artifacts)
	}
}

func handleDownloadProfileArtifact(store *profileArtifactStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifact, found, err := store.get(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found || artifact.StoragePath == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile artifact not found"})
			return
		}
		if status, err := store.requireWorkspaceScopeForArtifact(r, artifact); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		file, err := os.Open(artifact.StoragePath)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile artifact file not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		contentType := artifact.ContentType
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/octet-stream"
		}
		fileName := artifact.FileName
		if strings.TrimSpace(fileName) == "" {
			fileName = filepath.Base(artifact.StoragePath)
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
		http.ServeContent(w, r, fileName, stat.ModTime(), file)
	}
}

func handleAgentProfileArtifactUpload(agentStore *agentStore, artifactStore *profileArtifactStore) http.HandlerFunc {
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

		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be multipart/form-data"})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
			return
		}
		defer file.Close()

		payload, err := readProfileUploadPayload(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if len(payload) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file must not be empty"})
			return
		}

		artifact := profileArtifactRecord{
			ID:           fmt.Sprintf("profile-%d", time.Now().UTC().UnixNano()),
			RunID:        strings.TrimSpace(r.FormValue("runId")),
			ScenarioID:   strings.TrimSpace(r.FormValue("scenarioId")),
			ScenarioName: strings.TrimSpace(r.FormValue("scenarioName")),
			TargetID:     strings.TrimSpace(r.FormValue("targetId")),
			TargetName:   strings.TrimSpace(r.FormValue("targetName")),
			ProfileType:  strings.TrimSpace(r.FormValue("profileType")),
			Status:       "collected",
			SourceURL:    strings.TrimSpace(r.FormValue("sourceUrl")),
			FileName:     filepath.Base(strings.TrimSpace(header.Filename)),
			ContentType:  strings.TrimSpace(header.Header.Get("Content-Type")),
			SizeBytes:    int64(len(payload)),
			StartedAt:    strings.TrimSpace(r.FormValue("startedAt")),
			FinishedAt:   strings.TrimSpace(r.FormValue("finishedAt")),
		}
		artifact = normalizeUploadedProfileArtifact(artifact)
		if err := validateUploadedProfileArtifact(artifact); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if status, err := agentStore.requireTokenForRunIfKnown(token, artifact.RunID); err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		saved, err := artifactStore.save(artifact, payload)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

func readProfileUploadPayload(file io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(file, maxProfileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxProfileBytes {
		return nil, fmt.Errorf("profile artifact exceeds %d bytes", maxProfileBytes)
	}
	return payload, nil
}

func normalizeUploadedProfileArtifact(artifact profileArtifactRecord) profileArtifactRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if artifact.ProfileType == "" {
		artifact.ProfileType = "cpu"
	}
	if artifact.FileName == "." || artifact.FileName == string(filepath.Separator) || artifact.FileName == "" {
		artifact.FileName = fmt.Sprintf("%s-%s.pprof", artifact.RunID, artifact.ProfileType)
	}
	if artifact.ContentType == "" {
		artifact.ContentType = "application/octet-stream"
	}
	if artifact.StartedAt == "" {
		artifact.StartedAt = now
	}
	if artifact.FinishedAt == "" {
		artifact.FinishedAt = now
	}
	return artifact
}

func validateUploadedProfileArtifact(artifact profileArtifactRecord) error {
	if artifact.RunID == "" {
		return fmt.Errorf("runId is required")
	}
	if artifact.ProfileType == "" {
		return fmt.Errorf("profileType is required")
	}
	if artifact.FileName == "" {
		return fmt.Errorf("fileName is required")
	}
	return nil
}

func (store *profileArtifactStore) save(artifact profileArtifactRecord, payload []byte) (profileArtifactRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileArtifactRecord{}, err
	}
	defer db.Close()

	if strings.TrimSpace(artifact.ID) == "" {
		artifact.ID = fmt.Sprintf("profile-%d", time.Now().UTC().UnixNano())
	}
	if len(payload) > 0 {
		if artifact.FileName == "" {
			artifact.FileName = fmt.Sprintf("%s-%s.pprof", artifact.RunID, artifact.ProfileType)
		}
		artifact.StoragePath = filepath.Join(filepath.Dir(store.path), "profile_artifacts", artifact.ID, filepath.Base(artifact.FileName))
		if err := os.MkdirAll(filepath.Dir(artifact.StoragePath), 0o755); err != nil {
			return profileArtifactRecord{}, err
		}
		if err := os.WriteFile(artifact.StoragePath, payload, 0o600); err != nil {
			return profileArtifactRecord{}, err
		}
	}

	_, err = db.Exec(`
		INSERT INTO profile_artifacts (
			id, run_id, scenario_id, scenario_name, target_id, target_name,
			profile_type, status, source_url, file_name, content_type, size_bytes,
			error, started_at, finished_at, storage_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, artifact.ID, artifact.RunID, artifact.ScenarioID, artifact.ScenarioName, artifact.TargetID, artifact.TargetName,
		artifact.ProfileType, artifact.Status, artifact.SourceURL, artifact.FileName, artifact.ContentType, artifact.SizeBytes,
		artifact.Error, artifact.StartedAt, artifact.FinishedAt, artifact.StoragePath)
	if err != nil {
		return profileArtifactRecord{}, err
	}
	return artifact, nil
}

func (store *profileArtifactStore) list() ([]profileArtifactRecord, error) {
	return store.listFiltered(runListFilter{})
}

func (store *profileArtifactStore) listFiltered(filter runListFilter) ([]profileArtifactRecord, error) {
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
		conditions = append(conditions, "EXISTS (SELECT 1 FROM run_history scoped_runs WHERE scoped_runs.id = profile_artifacts.run_id AND scoped_runs.project_id = ?)")
		args = append(args, filter.ProjectID)
	}
	if filter.Environment != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM run_history scoped_runs WHERE scoped_runs.id = profile_artifacts.run_id AND scoped_runs.environment = ?)")
		args = append(args, filter.Environment)
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	rows, err := db.Query(`
		SELECT id, run_id, scenario_id, scenario_name, target_id, target_name,
			profile_type, status, source_url, file_name, content_type, size_bytes,
			error, started_at, finished_at, storage_path
		FROM profile_artifacts
		`+whereClause+`
		ORDER BY started_at DESC
		LIMIT 100
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artifacts := []profileArtifactRecord{}
	for rows.Next() {
		artifact, err := scanProfileArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (store *profileArtifactStore) listByRun(runID string) ([]profileArtifactRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, run_id, scenario_id, scenario_name, target_id, target_name,
			profile_type, status, source_url, file_name, content_type, size_bytes,
			error, started_at, finished_at, storage_path
		FROM profile_artifacts
		WHERE run_id = ?
		ORDER BY started_at DESC
		LIMIT 100
	`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artifacts := []profileArtifactRecord{}
	for rows.Next() {
		artifact, err := scanProfileArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (store *profileArtifactStore) listByRunIDs(runIDs []string) ([]profileArtifactRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	normalizedRunIDs := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		runID = strings.TrimSpace(runID)
		if runID != "" {
			normalizedRunIDs = append(normalizedRunIDs, runID)
		}
	}
	if len(normalizedRunIDs) == 0 {
		return nil, nil
	}

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	placeholders := make([]string, len(normalizedRunIDs))
	args := make([]any, len(normalizedRunIDs))
	for index, runID := range normalizedRunIDs {
		placeholders[index] = "?"
		args[index] = runID
	}
	rows, err := db.Query(`
		SELECT id, run_id, scenario_id, scenario_name, target_id, target_name,
			profile_type, status, source_url, file_name, content_type, size_bytes,
			error, started_at, finished_at, storage_path
		FROM profile_artifacts
		WHERE run_id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY started_at DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artifacts := []profileArtifactRecord{}
	for rows.Next() {
		artifact, err := scanProfileArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (store *profileArtifactStore) get(id string) (profileArtifactRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileArtifactRecord{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT id, run_id, scenario_id, scenario_name, target_id, target_name,
			profile_type, status, source_url, file_name, content_type, size_bytes,
			error, started_at, finished_at, storage_path
		FROM profile_artifacts
		WHERE id = ?
	`, strings.TrimSpace(id))
	artifact, err := scanProfileArtifact(row)
	if err == nil {
		return artifact, true, nil
	}
	if err == sql.ErrNoRows {
		return profileArtifactRecord{}, false, nil
	}
	return profileArtifactRecord{}, false, err
}

func (store *profileArtifactStore) requireWorkspaceScopeForArtifact(r *http.Request, artifact profileArtifactRecord) (int, error) {
	scope, ok := workspaceScopeFromRequest(r)
	if !ok {
		return 0, nil
	}

	projectID, environment, found, err := store.lookupArtifactWorkspaceScope(artifact)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !found {
		return http.StatusForbidden, workspaceScopeError("", "", scope)
	}
	if err := requireWorkspaceScopeForRecord(r, projectID, environment); err != nil {
		return http.StatusForbidden, err
	}
	return 0, nil
}

func (store *profileArtifactStore) lookupArtifactWorkspaceScope(artifact profileArtifactRecord) (string, string, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return "", "", false, err
	}
	defer db.Close()

	return lookupRunWorkspaceScope(db, artifact.RunID)
}

func (store *profileArtifactStore) open() (*sql.DB, error) {
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
	if err := ensureProfileArtifactSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureRunHistorySchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureProfileArtifactSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profile_artifacts (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			scenario_id TEXT NOT NULL,
			scenario_name TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_name TEXT NOT NULL,
			profile_type TEXT NOT NULL,
			status TEXT NOT NULL,
			source_url TEXT NOT NULL,
			file_name TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			error TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			storage_path TEXT NOT NULL
		)
	`)
	return err
}

func scanProfileArtifact(scanner profileArtifactScanner) (profileArtifactRecord, error) {
	var artifact profileArtifactRecord
	err := scanner.Scan(
		&artifact.ID,
		&artifact.RunID,
		&artifact.ScenarioID,
		&artifact.ScenarioName,
		&artifact.TargetID,
		&artifact.TargetName,
		&artifact.ProfileType,
		&artifact.Status,
		&artifact.SourceURL,
		&artifact.FileName,
		&artifact.ContentType,
		&artifact.SizeBytes,
		&artifact.Error,
		&artifact.StartedAt,
		&artifact.FinishedAt,
		&artifact.StoragePath,
	)
	if err != nil {
		return profileArtifactRecord{}, err
	}
	return artifact, nil
}
