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

type profileTaskTemplateRecord struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	Description             string   `json:"description"`
	Kind                    string   `json:"kind"`
	ProfileTypes            []string `json:"profileTypes"`
	ProfileType             string   `json:"profileType,omitempty"`
	RequiresProfileEndpoint bool     `json:"requiresProfileEndpoint"`
	RequiresPID             bool     `json:"requiresPid"`
	ProfileCommand          string   `json:"profileCommand,omitempty"`
	ProfileCommandArgs      []string `json:"profileCommandArgs,omitempty"`
	ProfileCommandOutput    string   `json:"profileCommandOutput,omitempty"`
	TimeoutBufferSeconds    int      `json:"timeoutBufferSeconds"`
	Enabled                 bool     `json:"enabled"`
	DisplayOrder            int      `json:"displayOrder"`
	CreatedAt               string   `json:"createdAt"`
	UpdatedAt               string   `json:"updatedAt"`
}

type profileTaskTemplateStore struct {
	path string
	mu   sync.Mutex
}

type profileTaskTemplateScanner interface {
	Scan(dest ...any) error
}

func newProfileTaskTemplateStoreFromEnv() *profileTaskTemplateStore {
	return &profileTaskTemplateStore{path: scenarioDatabasePathFromEnv()}
}

func handleListProfileTaskTemplates(store *profileTaskTemplateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		templates, err := store.list()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, templates)
	}
}

func handleCreateProfileTaskTemplate(store *profileTaskTemplateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input profileTaskTemplateRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		template, err := store.create(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, template)
	}
}

func handleUpdateProfileTaskTemplate(store *profileTaskTemplateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input profileTaskTemplateRecord
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}
		template, found, err := store.update(chi.URLParam(r, "id"), input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task template not found"})
			return
		}
		writeJSON(w, http.StatusOK, template)
	}
}

func handleDeleteProfileTaskTemplate(store *profileTaskTemplateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		found, err := store.delete(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile task template not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (store *profileTaskTemplateStore) list() ([]profileTaskTemplateRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, name, description, kind, profile_types, profile_type, requires_profile_endpoint,
			requires_pid, profile_command, profile_command_args, profile_command_output,
			timeout_buffer_seconds, enabled, display_order, created_at, updated_at
		FROM profile_task_templates
		WHERE enabled = 1
		ORDER BY display_order ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []profileTaskTemplateRecord{}
	for rows.Next() {
		template, err := scanProfileTaskTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return templates, nil
}

func (store *profileTaskTemplateStore) create(input profileTaskTemplateRecord) (profileTaskTemplateRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskTemplateRecord{}, err
	}
	defer db.Close()

	template := normalizeProfileTaskTemplate(input)
	if err := validateProfileTaskTemplate(template); err != nil {
		return profileTaskTemplateRecord{}, err
	}
	if _, found, err := getProfileTaskTemplateByID(db, template.ID); err != nil {
		return profileTaskTemplateRecord{}, err
	} else if found {
		return profileTaskTemplateRecord{}, fmt.Errorf("profile task template %q already exists", template.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	template.CreatedAt = now
	template.UpdatedAt = now

	if err := insertProfileTaskTemplate(db, template); err != nil {
		return profileTaskTemplateRecord{}, err
	}
	return template, nil
}

func (store *profileTaskTemplateStore) update(id string, input profileTaskTemplateRecord) (profileTaskTemplateRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return profileTaskTemplateRecord{}, false, err
	}
	defer db.Close()

	id = strings.TrimSpace(id)
	existing, found, err := getProfileTaskTemplateByID(db, id)
	if err != nil || !found {
		return profileTaskTemplateRecord{}, found, err
	}
	input.ID = id
	template := normalizeProfileTaskTemplate(input)
	template.CreatedAt = existing.CreatedAt
	template.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := validateProfileTaskTemplate(template); err != nil {
		return profileTaskTemplateRecord{}, true, err
	}

	profileTypes, err := encodeTemplateStringSlice(template.ProfileTypes)
	if err != nil {
		return profileTaskTemplateRecord{}, true, err
	}
	commandArgs, err := encodeTemplateStringSlice(template.ProfileCommandArgs)
	if err != nil {
		return profileTaskTemplateRecord{}, true, err
	}
	_, err = db.Exec(`
		UPDATE profile_task_templates
		SET name = ?, description = ?, kind = ?, profile_types = ?, profile_type = ?,
			requires_profile_endpoint = ?, requires_pid = ?, profile_command = ?, profile_command_args = ?,
			profile_command_output = ?, timeout_buffer_seconds = ?, enabled = ?, display_order = ?, updated_at = ?
		WHERE id = ?
	`, template.Name, template.Description, template.Kind, profileTypes, template.ProfileType,
		boolToSQLiteInt(template.RequiresProfileEndpoint), boolToSQLiteInt(template.RequiresPID),
		template.ProfileCommand, commandArgs, template.ProfileCommandOutput, template.TimeoutBufferSeconds,
		boolToSQLiteInt(template.Enabled), template.DisplayOrder, template.UpdatedAt, template.ID)
	if err != nil {
		return profileTaskTemplateRecord{}, true, err
	}
	return template, true, nil
}

func (store *profileTaskTemplateStore) delete(id string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	db, err := store.open()
	if err != nil {
		return false, err
	}
	defer db.Close()

	existing, found, err := getProfileTaskTemplateByID(db, strings.TrimSpace(id))
	if err != nil || !found {
		return found, err
	}
	_, err = db.Exec(`
		UPDATE profile_task_templates
		SET enabled = 0, updated_at = ?
		WHERE id = ?
	`, time.Now().UTC().Format(time.RFC3339Nano), existing.ID)
	return true, err
}

func (store *profileTaskTemplateStore) open() (*sql.DB, error) {
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
	if err := ensureProfileTaskTemplateSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := seedDefaultProfileTaskTemplates(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func insertProfileTaskTemplate(db *sql.DB, template profileTaskTemplateRecord) error {
	profileTypes, err := encodeTemplateStringSlice(template.ProfileTypes)
	if err != nil {
		return err
	}
	commandArgs, err := encodeTemplateStringSlice(template.ProfileCommandArgs)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO profile_task_templates (
			id, name, description, kind, profile_types, profile_type,
			requires_profile_endpoint, requires_pid, profile_command, profile_command_args,
			profile_command_output, timeout_buffer_seconds, enabled, display_order, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, template.ID, template.Name, template.Description, template.Kind, profileTypes, template.ProfileType,
		boolToSQLiteInt(template.RequiresProfileEndpoint), boolToSQLiteInt(template.RequiresPID),
		template.ProfileCommand, commandArgs, template.ProfileCommandOutput, template.TimeoutBufferSeconds,
		boolToSQLiteInt(template.Enabled), template.DisplayOrder, template.CreatedAt, template.UpdatedAt)
	return err
}

func ensureProfileTaskTemplateSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profile_task_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			kind TEXT NOT NULL,
			profile_types TEXT NOT NULL DEFAULT '[]',
			profile_type TEXT NOT NULL DEFAULT '',
			requires_profile_endpoint INTEGER NOT NULL DEFAULT 0,
			requires_pid INTEGER NOT NULL DEFAULT 0,
			profile_command TEXT NOT NULL DEFAULT '',
			profile_command_args TEXT NOT NULL DEFAULT '[]',
			profile_command_output TEXT NOT NULL DEFAULT '',
			timeout_buffer_seconds INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			display_order INTEGER NOT NULL DEFAULT 100,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	return err
}

func seedDefaultProfileTaskTemplates(db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	defaults := []profileTaskTemplateRecord{
		{
			ID:                      "go_pprof",
			Name:                    "Go pprof",
			Description:             "Collect Go runtime profiles from a target pprof endpoint.",
			Kind:                    "pprof",
			ProfileTypes:            []string{"cpu", "heap", "goroutine", "allocs", "mutex", "block"},
			RequiresProfileEndpoint: true,
			Enabled:                 true,
			DisplayOrder:            10,
			CreatedAt:               now,
			UpdatedAt:               now,
		},
		{
			ID:                   "linux_perf",
			Name:                 "Linux perf",
			Description:          "Run a fixed perf record command against a target process PID.",
			Kind:                 "command",
			ProfileTypes:         []string{"perf"},
			ProfileType:          "perf",
			RequiresPID:          true,
			ProfileCommand:       "perf",
			ProfileCommandArgs:   []string{"record", "-F", "99", "-p", "{{pid}}", "-g", "-o", "{{output}}", "--", "sleep", "{{seconds}}"},
			ProfileCommandOutput: "/tmp/{{targetNameSlug}}-perf.data",
			TimeoutBufferSeconds: 15,
			Enabled:              true,
			DisplayOrder:         20,
			CreatedAt:            now,
			UpdatedAt:            now,
		},
	}

	for _, template := range defaults {
		if _, err := db.Exec(`
			INSERT INTO profile_task_templates (
				id, name, description, kind, profile_types, profile_type,
				requires_profile_endpoint, requires_pid, profile_command, profile_command_args,
				profile_command_output, timeout_buffer_seconds, enabled, display_order, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`, template.ID, template.Name, template.Description, template.Kind, mustEncodeTemplateStringSlice(template.ProfileTypes), template.ProfileType,
			boolToSQLiteInt(template.RequiresProfileEndpoint), boolToSQLiteInt(template.RequiresPID),
			template.ProfileCommand, mustEncodeTemplateStringSlice(template.ProfileCommandArgs), template.ProfileCommandOutput, template.TimeoutBufferSeconds,
			boolToSQLiteInt(template.Enabled), template.DisplayOrder, template.CreatedAt, template.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func getProfileTaskTemplateByID(db *sql.DB, id string) (profileTaskTemplateRecord, bool, error) {
	row := db.QueryRow(`
		SELECT id, name, description, kind, profile_types, profile_type, requires_profile_endpoint,
			requires_pid, profile_command, profile_command_args, profile_command_output,
			timeout_buffer_seconds, enabled, display_order, created_at, updated_at
		FROM profile_task_templates
		WHERE id = ?
	`, strings.TrimSpace(id))
	template, err := scanProfileTaskTemplate(row)
	if err == nil {
		return template, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return profileTaskTemplateRecord{}, false, nil
	}
	return profileTaskTemplateRecord{}, false, err
}

func scanProfileTaskTemplate(scanner profileTaskTemplateScanner) (profileTaskTemplateRecord, error) {
	var template profileTaskTemplateRecord
	var profileTypes string
	var commandArgs string
	var requiresProfileEndpoint int
	var requiresPID int
	var enabled int
	if err := scanner.Scan(
		&template.ID,
		&template.Name,
		&template.Description,
		&template.Kind,
		&profileTypes,
		&template.ProfileType,
		&requiresProfileEndpoint,
		&requiresPID,
		&template.ProfileCommand,
		&commandArgs,
		&template.ProfileCommandOutput,
		&template.TimeoutBufferSeconds,
		&enabled,
		&template.DisplayOrder,
		&template.CreatedAt,
		&template.UpdatedAt,
	); err != nil {
		return profileTaskTemplateRecord{}, err
	}
	if err := decodeTemplateStringSlice("profileTypes", profileTypes, &template.ProfileTypes); err != nil {
		return profileTaskTemplateRecord{}, err
	}
	if err := decodeTemplateStringSlice("profileCommandArgs", commandArgs, &template.ProfileCommandArgs); err != nil {
		return profileTaskTemplateRecord{}, err
	}
	template.RequiresProfileEndpoint = requiresProfileEndpoint == 1
	template.RequiresPID = requiresPID == 1
	template.Enabled = enabled == 1
	return template, nil
}

func encodeTemplateStringSlice(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func mustEncodeTemplateStringSlice(values []string) string {
	data, err := encodeTemplateStringSlice(values)
	if err != nil {
		return "[]"
	}
	return data
}

func decodeTemplateStringSlice(field string, data string, target *[]string) error {
	if strings.TrimSpace(data) == "" {
		data = "[]"
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("profile task template %s contains invalid JSON: %w", field, err)
	}
	return nil
}

func boolToSQLiteInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeProfileTaskTemplate(input profileTaskTemplateRecord) profileTaskTemplateRecord {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	if input.ID == "" {
		input.ID = profileTaskTemplateIDFromName(input.Name)
	}
	input.Description = strings.TrimSpace(input.Description)
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if input.Kind == "" {
		input.Kind = "pprof"
	}
	input.ProfileTypes = normalizeTemplateStringValues(input.ProfileTypes, true)
	input.ProfileType = strings.ToLower(strings.TrimSpace(input.ProfileType))
	if input.ProfileType == "" && len(input.ProfileTypes) > 0 && input.Kind == "command" {
		input.ProfileType = input.ProfileTypes[0]
	}
	input.ProfileCommand = strings.TrimSpace(input.ProfileCommand)
	input.ProfileCommandArgs = normalizeTemplateStringValues(input.ProfileCommandArgs, false)
	input.ProfileCommandOutput = strings.TrimSpace(input.ProfileCommandOutput)
	if input.TimeoutBufferSeconds < 0 {
		input.TimeoutBufferSeconds = 0
	}
	if input.TimeoutBufferSeconds > 3600 {
		input.TimeoutBufferSeconds = 3600
	}
	if input.DisplayOrder <= 0 {
		input.DisplayOrder = 100
	}
	if !input.Enabled {
		input.Enabled = true
	}
	if input.Kind == "pprof" {
		input.RequiresProfileEndpoint = true
		if len(input.ProfileTypes) == 0 {
			input.ProfileTypes = []string{"cpu"}
		}
	}
	return input
}

func normalizeTemplateStringValues(values []string, lower bool) []string {
	if values == nil {
		return []string{}
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func profileTaskTemplateIDFromName(name string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-' || char == '_' || char == '.':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	id := strings.Trim(builder.String(), "_.-")
	if id == "" {
		return "profile_template_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return id
}

func validateProfileTaskTemplate(input profileTaskTemplateRecord) error {
	if input.ID == "" {
		return fmt.Errorf("id is required")
	}
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if input.Kind != "pprof" && input.Kind != "command" {
		return fmt.Errorf("kind must be pprof or command")
	}
	if len(input.ProfileTypes) == 0 {
		return fmt.Errorf("profileTypes is required")
	}
	if input.Kind == "command" {
		if input.ProfileType == "" {
			return fmt.Errorf("profileType is required for command templates")
		}
		if input.ProfileCommand == "" {
			return fmt.Errorf("profileCommand is required for command templates")
		}
		if len(strings.Fields(input.ProfileCommand)) != 1 {
			return fmt.Errorf("profileCommand must be a single executable")
		}
		if input.ProfileCommandOutput == "" {
			return fmt.Errorf("profileCommandOutput is required for command templates")
		}
		if input.RequiresPID && !templateStringValuesContain(input.ProfileCommandArgs, "{{pid}}") {
			return fmt.Errorf("profileCommandArgs must contain {{pid}} when requiresPid is true")
		}
	}
	return nil
}

func templateStringValuesContain(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
