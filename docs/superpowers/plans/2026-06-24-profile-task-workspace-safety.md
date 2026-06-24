# Profile Task Workspace Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Profile Task creation, leasing, and completion safe across workspaces while preserving Targets page manual profile capture without a synthetic Run.

**Architecture:** Profile Tasks become self-validating executable records. The store validates referenced Agent, Run, and Target resources on creation, agent lease defensively skips or fails invalid historical rows, and completion refuses mismatched persisted tasks. `target_manual` tasks use Target-Agent binding as their scope anchor and may omit `runId`.

**Tech Stack:** Go, SQLite, `net/http/httptest`, existing chi router, existing React Targets page payload.

---

## Scope

Included:

- Backend Profile Task validation rules.
- Backend Agent poll and complete defenses.
- Backend tests proving cross-workspace tasks cannot be created, leased, or completed.
- Backend tests proving `source=target_manual` can use `agentId + targetId` without `runId`.
- Existing successful Profile Task tests updated with real same-scope Run fixtures where the new rules require a Run.

Not included:

- Frontend payload changes. The current Targets page already sends `target_manual` without `runId`, and that payload is the intended contract.
- Auth/RBAC.
- New database migrations beyond existing startup schema compatibility.

## TDD Rules For Execution

- Write each new test first and run it before production changes.
- Confirm each new test fails for the expected reason.
- Implement only enough backend code to make the focused test pass.
- After each green step, run the affected package test.
- Do not loosen workspace validation to make old fixtures pass; update fixtures to represent valid resources.

## File Structure

Files modified by this plan:

```text
backend/internal/api/profile_tasks.go
backend/internal/api/profile_tasks_test.go
backend/internal/api/workspace_scope_test.go
```

Responsibilities:

- `backend/internal/api/profile_tasks.go`: Profile Task shape validation, referenced-resource consistency validation, lease filtering, completion defense.
- `backend/internal/api/profile_tasks_test.go`: Focused lifecycle, security, target manual, lease, and completion behavior tests.
- `backend/internal/api/workspace_scope_test.go`: Existing workspace-scope regression tests and shared fixture helpers used by the API test package.

## Task 1: RED Tests For Creation Invariants

**Files:**

- Modify: `backend/internal/api/profile_tasks_test.go`

- [ ] **Step 1: Add Run fixture helper for Profile Task tests**

Add this helper near the other Profile Task test helpers in `backend/internal/api/profile_tasks_test.go`:

```go
func saveProfileTaskRunFixture(t *testing.T, runID string, projectID string, environment string) {
	t.Helper()
	store := newRunHistoryStoreFromEnv()
	if err := store.save(createRunResponse{
		ID:            runID,
		Name:          runID,
		ProjectID:     projectID,
		Environment:   environment,
		Status:        "finished",
		Protocol:      "HTTP",
		Method:        http.MethodGet,
		URL:           "http://checkout.internal/ready",
		TotalRequests: 1,
		CreatedAt:     "2026-06-24T09:00:00Z",
	}); err != nil {
		t.Fatalf("expected run fixture %s save to succeed, got %v", runID, err)
	}
}
```

- [ ] **Step 2: Add failing test for store-level cross-scope create rejection**

Append this test to `backend/internal/api/profile_tasks_test.go`:

```go
func TestProfileTaskStoreRejectsCrossScopeRunAndAgent(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	saveProfileTaskRunFixture(t, "run-checkout-profile-scope", "project-checkout", "staging")
	agentStore := newAgentStoreFromEnv()
	billingAgent := upsertWorkspaceAgentRecord(t, agentStore, "agent-billing-profile-scope", "project-billing", "prod")

	_, err := newProfileTaskStoreFromEnv().create(profileTaskRecord{
		AgentID:        billingAgent.ID,
		RunID:          "run-checkout-profile-scope",
		PprofBaseURL:   "http://127.0.0.1:6060/debug/pprof",
		ProfileType:    "cpu",
		ProfileSeconds: 1,
	})
	if err == nil {
		t.Fatal("expected cross-scope profile task creation to fail")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected workspace mismatch error, got %v", err)
	}
}
```

- [ ] **Step 3: Add failing test for `target_manual` without Run**

Append this test to `backend/internal/api/profile_tasks_test.go`:

```go
func TestCreateTargetManualProfileTaskWithoutRunID(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	token := createScopedAgentToken(t, router, "manual-target-token", "project-checkout", "staging")
	agentID := registerScopedAgentWithID(t, router, token.Token, "agent-target-manual-01", "target-manual-01")

	createTarget := httptest.NewRecorder()
	router.ServeHTTP(createTarget, httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewBufferString(`{
		"name": "checkout-manual-target",
		"projectId": "project-checkout",
		"environment": "staging",
		"baseUrl": "http://checkout.internal:8080",
		"agentIds": ["`+agentID+`"],
		"profileEndpoint": "http://127.0.0.1:6060/debug/pprof"
	}`)))
	if createTarget.Code != http.StatusCreated {
		t.Fatalf("expected target creation status %d, got %d with body %s", http.StatusCreated, createTarget.Code, createTarget.Body.String())
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createTarget.Body).Decode(&target); err != nil {
		t.Fatalf("expected target JSON, got decode error: %v", err)
	}

	createTask := httptest.NewRecorder()
	router.ServeHTTP(createTask, httptest.NewRequest(http.MethodPost, "/api/profile-tasks", bytes.NewBufferString(`{
		"agentId": "`+agentID+`",
		"targetId": "`+target.ID+`",
		"targetName": "checkout-manual-target",
		"source": "target_manual",
		"pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
		"profileType": "cpu",
		"profileSeconds": 30
	}`)))
	if createTask.Code != http.StatusCreated {
		t.Fatalf("expected target_manual create status %d, got %d with body %s", http.StatusCreated, createTask.Code, createTask.Body.String())
	}
	var created profileTaskRecord
	if err := json.NewDecoder(createTask.Body).Decode(&created); err != nil {
		t.Fatalf("expected created profile task JSON, got decode error: %v", err)
	}
	if created.RunID != "" || created.TargetID != target.ID || created.AgentID != agentID || created.Source != targetManualProfileTaskSource {
		t.Fatalf("expected target-scoped manual task without run, got %#v", created)
	}
}
```

- [ ] **Step 4: Run RED tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestProfileTaskStoreRejectsCrossScopeRunAndAgent|TestCreateTargetManualProfileTaskWithoutRunID'
```

Expected before implementation:

```text
FAIL: TestProfileTaskStoreRejectsCrossScopeRunAndAgent ... expected cross-scope profile task creation to fail
FAIL: TestCreateTargetManualProfileTaskWithoutRunID ... runId is required
```

## Task 2: GREEN Creation Validation

**Files:**

- Modify: `backend/internal/api/profile_tasks.go`
- Modify: `backend/internal/api/profile_tasks_test.go`

- [ ] **Step 1: Relax shape validation only for `target_manual`**

Change `validateProfileTask` in `backend/internal/api/profile_tasks.go` to this shape:

```go
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
```

- [ ] **Step 2: Add resource consistency helper**

Add these helpers in `backend/internal/api/profile_tasks.go` near the existing workspace lookup helpers:

```go
func validateProfileTaskResourceConsistency(db *sql.DB, task profileTaskRecord) error {
	scopes := []workspaceScopeConstraint{}
	if task.RunID != "" {
		projectID, environment, found, err := lookupRunWorkspaceScope(db, task.RunID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("run %q not found", task.RunID)
		}
		scopes = append(scopes, normalizedWorkspaceScope(projectID, environment))
	}

	agentProjectID, agentEnvironment, found, err := lookupAgentWorkspaceScope(db, task.AgentID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agent %q not found", task.AgentID)
	}
	agentScope := normalizedWorkspaceScope(agentProjectID, agentEnvironment)
	scopes = append(scopes, agentScope)

	var target targetRecord
	if task.TargetID != "" {
		var found bool
		target, found, err = getTargetByID(db, task.TargetID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("target %q not found", task.TargetID)
		}
		scopes = append(scopes, normalizedWorkspaceScope(target.ProjectID, target.Environment))
	}

	for _, scope := range scopes {
		if scope.ProjectID != agentScope.ProjectID || scope.Environment != agentScope.Environment {
			return fmt.Errorf("profile task workspace mismatch: agent %s/%s cannot use resource %s/%s",
				agentScope.ProjectID, agentScope.Environment, scope.ProjectID, scope.Environment)
		}
	}
	if task.Source == targetManualProfileTaskSource && !targetHasAgent(target, task.AgentID) {
		return fmt.Errorf("agent %q is not bound to target %q", task.AgentID, task.TargetID)
	}
	return nil
}

func targetHasAgent(target targetRecord, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	for _, boundAgentID := range target.AgentIDs {
		if strings.TrimSpace(boundAgentID) == agentID {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Call resource consistency helper from store create**

In `profileTaskStore.create`, after `validateProfileTask(task)` and before assigning timestamps, add:

```go
if err := validateProfileTaskResourceConsistency(db, task); err != nil {
	return profileTaskRecord{}, err
}
```

- [ ] **Step 4: Update existing successful run-scoped task fixtures**

Before existing tests create successful non-`target_manual` tasks, save a same-scope Run fixture. Use the helper from Task 1. Insert these exact calls in `backend/internal/api/profile_tasks_test.go` before each test's successful `POST /api/profile-tasks` request or direct `store.create` call:

```go
// TestProfileTaskLifecycleCreatesLeasesAndCompletesTask
saveProfileTaskRunFixture(t, "run-profile-task-1", "default", "default")

// TestAgentTokenCannotPollOrCompleteProfileTasksForAnotherAgent
saveProfileTaskRunFixture(t, "run-billing-profile-secure", "project-billing", "prod")

// TestProfileTaskLifecycleCreatesAndLeasesCommandProfilerTask
saveProfileTaskRunFixture(t, "run-command-profile-task-1", "default", "default")

// TestProfileTaskLeaseExpiresAndCanBeReclaimed
saveProfileTaskRunFixture(t, "run-profile-task-timeout", "default", "default")

// TestListProfileTasksFiltersByRunAgentStatusAndLimit
saveProfileTaskRunFixture(t, "run-profile-filter-1", "default", "default")
saveProfileTaskRunFixture(t, "run-profile-filter-2", "default", "default")

// TestProfileTaskFailureRetriesUntilMaxAttempts
saveProfileTaskRunFixture(t, "run-profile-task-retry", "default", "default")

// TestRetryFailedProfileTaskRequeuesTask
saveProfileTaskRunFixture(t, "run-profile-task-requeue", "default", "default")
```

Do not add Run fixtures to tests that intentionally fail earlier on invalid `profileSeconds` or invalid `source`.

- [ ] **Step 5: Run focused GREEN tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestProfileTaskStoreRejectsCrossScopeRunAndAgent|TestCreateTargetManualProfileTaskWithoutRunID'
```

Expected:

```text
ok  	all_in_one_testing/backend/internal/api
```

- [ ] **Step 6: Run Profile Task package tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestProfileTask'
```

Expected:

```text
ok  	all_in_one_testing/backend/internal/api
```

## Task 3: RED Tests For Lease And Complete Defenses

**Files:**

- Modify: `backend/internal/api/profile_tasks_test.go`

- [ ] **Step 1: Add unsafe persisted task fixture helper**

Add this helper to `backend/internal/api/profile_tasks_test.go`:

```go
func insertUnsafeProfileTaskFixture(t *testing.T, task profileTaskRecord) {
	t.Helper()
	db, err := newProfileTaskStoreFromEnv().open()
	if err != nil {
		t.Fatalf("expected profile task DB open to succeed, got %v", err)
	}
	defer db.Close()
	now := "2026-06-24T09:05:00Z"
	if _, err := db.Exec(`
		INSERT INTO profile_tasks (
			id, agent_id, run_id, scenario_id, scenario_name, target_id, target_name,
			pprof_base_url, profile_url, profile_type, profile_seconds,
			source, status, attempts, max_attempts,
			artifact_id, error, created_at, updated_at, leased_at, completed_at
		) VALUES (?, ?, ?, '', '', ?, ?, ?, '', ?, ?, ?, ?, 0, 3, '', '', ?, ?, ?, '')
	`, task.ID, task.AgentID, task.RunID, task.TargetID, task.TargetName,
		task.PprofBaseURL, task.ProfileType, task.ProfileSeconds,
		task.Source, task.Status, now, now, task.LeasedAt); err != nil {
		t.Fatalf("expected unsafe profile task fixture insert to succeed, got %v", err)
	}
}
```

- [ ] **Step 2: Add failing poll defense test**

Append this test:

```go
func TestAgentPollRejectsPersistedCrossScopeRunTaskForSameAgent(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	saveProfileTaskRunFixture(t, "run-checkout-historical-task", "project-checkout", "staging")
	billingToken := createScopedAgentToken(t, router, "billing-historical-token", "project-billing", "prod")
	billingAgentID := registerScopedAgentWithID(t, router, billingToken.Token, "agent-billing-historical-01", "billing-historical-01")
	insertUnsafeProfileTaskFixture(t, profileTaskRecord{
		ID:             "profile-task-cross-scope-historical",
		AgentID:        billingAgentID,
		RunID:          "run-checkout-historical-task",
		PprofBaseURL:   "http://127.0.0.1:6060/debug/pprof",
		ProfileType:    "cpu",
		ProfileSeconds: 1,
		Source:         defaultProfileTaskSource,
		Status:         "pending",
	})

	poll := httptest.NewRecorder()
	router.ServeHTTP(poll, newAuthorizedAgentRequest(http.MethodGet, "/agent/v1/profile-tasks?agentId="+billingAgentID, billingToken.Token, ""))
	if poll.Code != http.StatusOK {
		t.Fatalf("expected poll status %d, got %d with body %s", http.StatusOK, poll.Code, poll.Body.String())
	}
	var tasks []profileTaskRecord
	if err := json.NewDecoder(poll.Body).Decode(&tasks); err != nil {
		t.Fatalf("expected poll JSON, got decode error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected cross-scope historical task not to be leased, got %#v", tasks)
	}
	stored, found, err := newProfileTaskStoreFromEnv().get("profile-task-cross-scope-historical")
	if err != nil || !found {
		t.Fatalf("expected historical task lookup, found=%v err=%v", found, err)
	}
	if stored.Status != "failed" || !strings.Contains(stored.Error, "workspace") {
		t.Fatalf("expected invalid historical task to be failed with workspace error, got %#v", stored)
	}
}
```

- [ ] **Step 3: Add failing complete defense test**

Append this test:

```go
func TestAgentCannotCompletePersistedCrossScopeRunTask(t *testing.T) {
	t.Setenv("SCENARIO_DB_PATH", filepath.Join(t.TempDir(), "platform.db"))
	router := NewRouter()
	saveProfileTaskRunFixture(t, "run-checkout-complete-task", "project-checkout", "staging")
	billingToken := createScopedAgentToken(t, router, "billing-complete-token", "project-billing", "prod")
	billingAgentID := registerScopedAgentWithID(t, router, billingToken.Token, "agent-billing-complete-01", "billing-complete-01")
	insertUnsafeProfileTaskFixture(t, profileTaskRecord{
		ID:             "profile-task-cross-scope-complete",
		AgentID:        billingAgentID,
		RunID:          "run-checkout-complete-task",
		PprofBaseURL:   "http://127.0.0.1:6060/debug/pprof",
		ProfileType:    "cpu",
		ProfileSeconds: 1,
		Source:         defaultProfileTaskSource,
		Status:         "leased",
		LeasedAt:       "2026-06-24T09:05:00Z",
	})

	complete := httptest.NewRecorder()
	router.ServeHTTP(complete, newAuthorizedAgentRequest(http.MethodPost, "/agent/v1/profile-tasks/profile-task-cross-scope-complete/complete", billingToken.Token, `{
		"artifactId": "profile-cross-scope-artifact"
	}`))
	if complete.Code != http.StatusConflict {
		t.Fatalf("expected cross-scope complete status %d, got %d with body %s", http.StatusConflict, complete.Code, complete.Body.String())
	}
	stored, found, err := newProfileTaskStoreFromEnv().get("profile-task-cross-scope-complete")
	if err != nil || !found {
		t.Fatalf("expected task lookup, found=%v err=%v", found, err)
	}
	if stored.Status == "completed" || stored.ArtifactID != "" {
		t.Fatalf("expected invalid task not to complete, got %#v", stored)
	}
}
```

- [ ] **Step 4: Run RED tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestAgentPollRejectsPersistedCrossScopeRunTaskForSameAgent|TestAgentCannotCompletePersistedCrossScopeRunTask'
```

Expected before implementation:

```text
FAIL: TestAgentPollRejectsPersistedCrossScopeRunTaskForSameAgent ... expected cross-scope historical task not to be leased
FAIL: TestAgentCannotCompletePersistedCrossScopeRunTask ... expected cross-scope complete status 409
```

## Task 4: GREEN Lease And Complete Defenses

**Files:**

- Modify: `backend/internal/api/profile_tasks.go`

- [ ] **Step 1: Add invalid task failure helper**

Add this helper near `leasePending`:

```go
func failInvalidProfileTaskCandidate(db *sql.DB, task profileTaskRecord, err error, now string) error {
	_, updateErr := db.Exec(`
		UPDATE profile_tasks
		SET status = 'failed', error = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND status IN ('pending', 'leased')
	`, err.Error(), now, now, task.ID)
	return updateErr
}
```

- [ ] **Step 2: Filter candidates in `leasePending` before leasing**

In `leasePending`, after closing rows and before the loop that sets tasks to `leased`, split candidates into valid and invalid tasks:

```go
validTasks := make([]profileTaskRecord, 0, len(tasks))
for _, task := range tasks {
	if err := validateProfileTaskResourceConsistency(db, task); err != nil {
		if updateErr := failInvalidProfileTaskCandidate(db, task, err, nowTime.Format(time.RFC3339Nano)); updateErr != nil {
			return nil, updateErr
		}
		continue
	}
	validTasks = append(validTasks, task)
}
tasks = validTasks
```

Keep the existing lease update loop after this block so only valid tasks are returned to the Agent.

- [ ] **Step 3: Defend completion in the handler**

In `handleCompleteProfileTask`, after `requireTokenForAgent` succeeds and before calling `taskStore.complete`, add:

```go
if err := taskStore.requireProfileTaskResourceConsistency(existing); err != nil {
	writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	return
}
```

Add this method:

```go
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
```

- [ ] **Step 4: Defend direct store completion**

Inside `profileTaskStore.complete`, after loading `existing` and before mutating status, add:

```go
if err := validateProfileTaskResourceConsistency(db, existing); err != nil {
	return profileTaskRecord{}, false, err
}
```

This keeps internal callers from bypassing the same rule.

- [ ] **Step 5: Run focused GREEN tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestAgentPollRejectsPersistedCrossScopeRunTaskForSameAgent|TestAgentCannotCompletePersistedCrossScopeRunTask'
```

Expected:

```text
ok  	all_in_one_testing/backend/internal/api
```

## Task 5: Regression And Verification

**Files:**

- Modify: `backend/internal/api/profile_tasks_test.go`
- Modify: `backend/internal/api/workspace_scope_test.go` only if existing shared fixture helpers need adjusted Run records.

- [ ] **Step 1: Run all Profile Task tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestProfileTask|TestAgentPollRejectsPersistedCrossScopeRunTaskForSameAgent|TestAgentCannotCompletePersistedCrossScopeRunTask'
```

Expected:

```text
ok  	all_in_one_testing/backend/internal/api
```

- [ ] **Step 2: Run workspace scope tests**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go test ./internal/api -run 'TestWorkspaceScope'
```

Expected:

```text
ok  	all_in_one_testing/backend/internal/api
```

- [ ] **Step 3: Run full backend verification**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing/backend
GOPROXY=https://proxy.golang.org,direct go build ./...
GOPROXY=https://proxy.golang.org,direct go test ./...
GOPROXY=https://proxy.golang.org,direct go vet ./...
GOPROXY=https://proxy.golang.org,direct go test -race ./...
```

Expected:

```text
go build ./... exits 0
go test ./... exits 0
go vet ./... exits 0
go test -race ./... exits 0
```

- [ ] **Step 4: Check formatting and diff hygiene**

Run:

```bash
cd /Users/hujineun/gitProject/all_in_one_testing
gofmt -w backend/internal/api/profile_tasks.go backend/internal/api/profile_tasks_test.go backend/internal/api/workspace_scope_test.go
git diff --check
git status --short
```

Expected:

```text
git diff --check exits 0
git status --short shows only intended files
```

## Self-Review Checklist

- [ ] Cross-workspace Profile Task creation is rejected by `profileTaskStore.create`, not only by request-scope middleware.
- [ ] `target_manual` Profile Tasks can be created with `agentId + targetId` and no `runId`.
- [ ] `target_manual` creation verifies the Agent is bound to the Target.
- [ ] Agent poll never leases persisted cross-workspace tasks.
- [ ] Completion rejects persisted cross-workspace tasks.
- [ ] Existing Profile Task lifecycle tests use real same-scope Run fixtures where the new invariants require a Run.
- [ ] No frontend fake `runId` is introduced.
