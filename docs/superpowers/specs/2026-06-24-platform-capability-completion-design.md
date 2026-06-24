# Platform Capability Completion Design

## Background

All-in-One Testing already has a usable prototype for the core load testing workflow:

- Go control plane with SQLite persistence.
- React console for Dashboard, Targets & Agents, Scenarios, Runs, and Reports.
- Scenario, Target, Agent token, Run, Report, Profile Task, Profile Artifact, and workspace scope APIs.
- Run event polling and SSE streaming through `/api/runs/{id}/events` and `/api/runs/{id}/events/stream`.
- Agent binary packaging for Linux amd64, checksum generation, manifest serving, and `/agent/install.sh` systemd bootstrap.
- Profile Task templates, controlled command profiler execution, and basic report comparison APIs.

The remaining completion work should therefore avoid reimplementing existing capabilities. The MVP completion scope is:

1. Agent `.deb` and `.rpm` release artifacts.
2. Run WebSocket bidirectional control.
3. Historical multi-dimensional trend reporting.
4. Profile artifact archive and retention management.
5. Advanced threshold actions.
6. Documentation cleanup so outdated "not connected" sections match reality.

## Goals

- Keep the prototype deployable with the existing Go + React + SQLite stack.
- Add release packages that install the existing Agent daemon as a system service.
- Add a WebSocket API that can subscribe to run events and issue safe run control commands.
- Provide historical trend APIs for run metrics, target metrics, process metrics, and profile artifacts.
- Provide zip archive download and retention cleanup for profile artifacts.
- Extend threshold handling from passive alerts into configurable actions.
- Preserve workspace scope enforcement across all new APIs.
- Keep each capability testable with focused Go `httptest` and React Testing Library tests.

## Non-Goals

- No PostgreSQL, Redis, MinIO, S3, Kafka, or external queue migration in this slice.
- No browser-authenticated user model, RBAC, or organization account system.
- No arbitrary command execution from the console; profiler commands continue to use controlled templates.
- No full WebSocket replacement for every polling path. WebSocket starts with Runs and can be extended later.
- No native system package build tools required on the developer machine. The package builder writes simple Debian and RPM package files directly from Go.

## Existing Capability Reconciliation

The following items are already implemented and should be treated as baseline behavior, not new feature work:

- Workspace scope: `projectId` / `environment` filtering, `X-AIT-Project-ID`, `X-AIT-Environment`, query scope fallback for links and EventSource.
- Run SSE: `/api/runs/{id}/events/stream`, with frontend EventSource usage and polling fallback.
- Profile Task templates: `/api/profile-task-templates` CRUD and Targets page manual dispatch.
- Basic trend comparisons: `/api/reports/compare`, `/api/reports/process-trends/compare`, and `/api/reports/profile-artifacts/compare`.
- Agent binary release: `cmd/package-agent` creates `all-in-one-agent-linux-amd64`, `.sha256`, and `manifest.json`.

The implementation should update `docs/usage-guide.md` and README wording so those capabilities are no longer listed as absent.

## Architecture

The completion work stays inside the existing single-process control plane.

```text
React Console
  |-- REST APIs for setup, history, archive, retention
  |-- EventSource for one-way run events
  |-- WebSocket for run events + run control commands

Go Control Plane
  |-- API router
  |-- Run manager
  |-- SQLite stores
  |-- Local profile artifact storage
  |-- Agent release package builder

Target Agent
  |-- Existing daemon and profile task worker
  |-- Installed by install.sh, deb, or rpm
```

New code should follow existing package boundaries:

- `backend/internal/agentrelease`: build binary, checksum, manifest, `.deb`, and `.rpm` artifacts.
- `backend/internal/api`: expose package downloads, WebSocket control, trend reports, archive downloads, retention cleanup, and threshold action persistence.
- `frontend/src/app/App.tsx`: add console surfaces using the existing compact operational UI style.

## Profile Task Execution Safety

Profile Tasks are executable work delivered to Agents, so workspace safety cannot depend only on optional REST request scope. The following invariants apply to every Profile Task creation and execution path, including unscoped local-prototype API calls, internal store calls, automatically scheduled tasks, and historical rows already persisted in SQLite.

### Resource Consistency Invariants

`profileTaskStore.create` must validate all referenced resources before inserting a row:

- `agentId` must reference an existing Agent.
- Non-`target_manual` tasks must include a `runId`, and that Run must exist.
- `target_manual` tasks may omit `runId`, but must include `targetId`, and that Target must exist.
- When `targetId` is present, the Target must be in the same workspace as the Agent and any Run.
- For `target_manual`, the Agent must be currently bound to the Target's `agentIds`; otherwise a caller could dispatch arbitrary work to an unrelated Agent in the same workspace.
- If `runId`, `agentId`, and `targetId` are all present, all referenced resources must normalize to the same `projectId` and `environment`.
- Missing referenced resources or mismatched resource scopes reject creation with a `400` API response and no database insert.

These checks run even when the request has no `X-AIT-Project-ID`, `X-AIT-Environment`, or query scope. Header/query workspace scope remains an access-control filter, not the source of truth for task executability.

### Agent Lease And Complete Defenses

Agent polling must never return an invalid persisted task. `leasePending(agentId, limit)` must re-check each candidate task against the same resource consistency rules before leasing it. Tasks that fail this defensive check are not returned to the Agent. The implementation may mark those rows `failed` with an explanatory error so they do not remain in the pending queue forever.

Task completion is also defensive. Before accepting `/agent/v1/profile-tasks/{id}/complete`, the backend must verify that the task's Agent, Run, and Target references still resolve to one workspace. A mismatched historical task cannot be completed successfully, even if the caller presents a valid token for the task's `agentId`.

### Target Manual Task Semantics

The Targets page manually creates profile tasks from a saved Target row. This is not tied to a load-test Run and should remain a Target-scoped operation:

```json
{
  "agentId": "agent-checkout-01",
  "targetId": "target-checkout",
  "targetName": "checkout-service",
  "source": "target_manual",
  "pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
  "profileType": "cpu",
  "profileSeconds": 30
}
```

For `target_manual`, `runId` is optional and normally absent. The backend validates Target-Agent binding and workspace consistency instead of requiring a synthetic Run. Frontend code should keep sending the current Target-scoped payload and should not invent a fake `runId`.

## Agent Deb/RPM Packages

### Package Outputs

`go run ./cmd/package-agent --version <version> --commit <commit> --date <date>` should continue producing the existing binary artifacts and additionally produce:

- `all-in-one-agent_<version>_linux_amd64.deb`
- `all-in-one-agent-<version>-1.x86_64.rpm`
- matching `.sha256` files
- manifest entries for each package with:
  - `name`
  - `os`
  - `arch`
  - `format`
  - `version`
  - `commit`
  - `date`
  - `url`
  - `checksumUrl`
  - `sha256`
  - `sizeBytes`

The existing `/agent/binaries/manifest.json` remains the release index. `/agent/binaries/{name}` serves binary, checksum, deb, rpm, and package checksums from the configured `AGENT_BINARY_DIR`.

### Package Contents

Both package formats install:

- `/usr/local/bin/all-in-one-agent`
- `/etc/all-in-one-agent/agent.json.example`
- `/etc/systemd/system/all-in-one-agent.service`

The packages should not embed a secret token. Operators can copy the example config to `/etc/all-in-one-agent/agent.json`, fill in `controlPlaneUrl`, `token`, `agentId`, and metadata, then run:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now all-in-one-agent.service
```

### Package Generation Approach

The MVP builder writes package files directly in Go:

- Debian package: `ar` archive containing `debian-binary`, `control.tar.gz`, and `data.tar.gz`.
- RPM package: RPM output that is recognized by `rpm -qip`, `rpm -qlp`, and installs on common RPM systems.

RPM is part of the MVP acceptance criteria. Do not replace it with a `.tar.gz`, metadata stub, or UI-only placeholder. If real RPM generation proves too risky during implementation, stop and revise this design before changing the deliverable.

## WebSocket Run Control

### Endpoint

Add:

```text
GET /api/runs/{id}/ws
```

The endpoint accepts workspace scope using query parameters because browser WebSocket connections cannot set arbitrary headers reliably:

```text
/api/runs/run-xxx/ws?projectId=project-checkout&environment=staging
```

The backend rejects cross-scope access with `403` before upgrading.

### Server-to-Client Messages

The server sends JSON envelopes:

```json
{
  "type": "run_event",
  "runId": "run-xxx",
  "event": {
    "id": "run-event-run-xxx-1",
    "type": "run_progress",
    "status": "running",
    "successRequests": 10,
    "failedRequests": 0,
    "totalRequests": 20
  }
}
```

It also sends:

```json
{ "type": "hello", "runId": "run-xxx", "status": "running" }
{ "type": "ack", "command": "stop", "runId": "run-xxx" }
{ "type": "error", "message": "run is already terminal" }
```

### Client-to-Server Commands

The MVP supports:

```json
{ "type": "ping" }
{ "type": "stop" }
```

`stop` calls the same run manager path as `POST /api/runs/{id}/stop`. It must be idempotent for terminal runs and return an error envelope when the stop request cannot be applied.

### Implementation Constraint

Do not introduce a heavyweight WebSocket framework. Use a small, maintained Go WebSocket dependency only if implementing the upgrade and frame handling with the standard library would distract from the product work. If a dependency is added, verify it supports Go 1.22.

## Historical Trend Reports

### Endpoint Set

Add REST endpoints under `/api/reports/trends`:

```text
GET /api/reports/trends/runs
GET /api/reports/trends/targets
GET /api/reports/trends/processes
GET /api/reports/trends/profile-artifacts
```

All endpoints accept:

- `projectId`
- `environment`
- `targetId`
- `agentId`
- `scenarioId`
- `from`
- `to`
- `bucket`
- `limit`

`bucket` supports `run`, `minute`, `hour`, and `day`. If omitted, the default is `run` for run-level summaries and `hour` for time-series metrics.

### Run Trends

`/api/reports/trends/runs` returns ordered points with:

- run count
- total requests
- success rate
- error rate
- average QPS
- max QPS
- average p95
- max p95
- aborted count
- canceled count

### Historical Ownership Snapshot

Target and process trends must not infer historical Target ownership from the current Target-Agent binding. Target bindings can change after a Run, and recalculating old metrics with the latest binding would produce incorrect history.

Add a run-scoped binding snapshot at Run creation:

- `runId`
- `projectId`
- `environment`
- `targetId`
- `targetName`
- `targetBaseUrl`
- `agentIds`
- `processMatch`
- `metricThresholds`
- `createdAt`

The snapshot is immutable. Target and process trend APIs only attribute Agent metric samples to a Target when the sample falls inside a Run window that has a matching run/target/agent snapshot. Free-running Agent metrics outside a Run window can still appear in Agent metric APIs, but they are not used for Target or Process historical trends unless a future design adds explicit long-lived Target ownership intervals.

### Target Trends

`/api/reports/trends/targets` aggregates Agent metric samples aligned to Target ownership:

- CPU peak and average
- memory peak and average
- disk read/write peak
- network RX/TX peak
- sample count
- threshold breach count

### Process Trends

`/api/reports/trends/processes` groups by:

- target ID
- agent ID
- process name
- command line fingerprint

Each point returns CPU max, RSS max, fd max, thread max, first seen, last seen, and sample count.

### Profile Artifact Trends

`/api/reports/trends/profile-artifacts` groups by profile type and status:

- artifact count
- collected count
- failed count
- total size bytes
- average size bytes
- latest artifact ID

### Frontend Surface

Reports adds a "Historical trends" section with compact charts/tables:

- Run throughput and latency trend.
- Target CPU/MEM trend.
- Process resource trend table.
- Profile artifact volume trend.

The initial frontend can use Ant Design tables and lightweight inline chart blocks already used in the app; no new charting dependency is required.

## Artifact Archive And Retention

### Archive Download

Add:

```text
GET /api/profile-artifacts/archive
```

Query filters:

- `runId`
- `targetId`
- `profileType`
- `status`
- `projectId`
- `environment`

The response is a zip file containing matching stored files. The zip should include a `manifest.json` with artifact metadata and original file names. Missing files are skipped and recorded in the manifest under `missingArtifacts`; the HTTP response remains `200` when at least one artifact or manifest entry is produced, and the manifest is the authoritative partial-success report. If no artifact matches the filter, return `404`.

Archive safety rules:

- Maximum artifacts per archive: 200.
- Maximum total uncompressed bytes per archive: 512 MiB.
- If either limit would be exceeded, return `413` with a JSON error before writing the zip response.
- Zip entries must be generated from sanitized names only: no absolute paths, no drive letters, no `..`, no empty path segments, and `/` is the only directory separator.
- Duplicate file names are disambiguated with the artifact ID, for example `cpu.pprof` and `profile-123-cpu.pprof`.
- Stored `storagePath` is used only as the file source on disk, never as the zip entry name.
- Archive generation should stream to the response with `archive/zip`; it must not load all artifact payloads into memory at once.
- The manifest records skipped artifacts with `reason` values such as `missing_file`, `deleted`, or `unsafe_name`.

### Retention Policy

Add retention policy APIs:

```text
GET  /api/profile-artifact-retention
PUT  /api/profile-artifact-retention
POST /api/profile-artifacts/cleanup
```

Retention policy is workspace-scoped, not global. The policy table is keyed by `projectId` and `environment`, and cleanup always operates inside exactly one workspace.

Policy fields:

- `projectId`
- `environment`
- `enabled`
- `maxAgeDays`
- `maxTotalBytes`
- `deleteFailedArtifacts`
- `dryRunDefault`

Cleanup response fields:

- `dryRun`
- `matchedCount`
- `deletedCount`
- `freedBytes`
- `errors`
- `artifacts`

`GET` and `PUT /api/profile-artifact-retention` require header or query workspace scope and read/write only that workspace policy. `POST /api/profile-artifacts/cleanup?dryRun=true` performs a dry run for the scoped workspace. `dryRun=false` deletes files and marks records as archived or deleted according to the store model chosen during implementation.

Unscoped cleanup is not allowed in the MVP. Requests without `projectId` / `environment` scope return `400` with `workspace scope is required for artifact cleanup`. A future administrator-only global policy can be added after real auth/RBAC exists.

### Store Model

Add metadata fields to profile artifact records:

- `archivedAt`
- `deletedAt`
- `retentionReason`

Downloads for deleted artifacts return `404`. List APIs keep metadata visible by default but include `status` or deletion fields so the UI can explain that the file is no longer available.

## Advanced Threshold Actions

### Threshold Model

Targets currently support metric thresholds and reports derive `target_metric` alerts. Extend the threshold config with actions:

```json
{
  "cpuMaxPercent": 70,
  "memoryMaxPercent": 75,
  "actions": [
    { "type": "alert", "severity": "warning" },
    { "type": "profile", "profileType": "cpu", "profileSeconds": 30 },
    { "type": "stop_run" },
    { "type": "retain_artifacts", "maxAgeDays": 30 }
  ]
}
```

Supported MVP action types:

- `alert`: create report alert only.
- `profile`: create a Profile Task only for the Agent whose metric breached the threshold. If a breach is derived from Target-level data without an Agent identity, create one task per currently snapshotted bound Agent and record that fan-out in event metadata.
- `stop_run`: stop the active Run when a threshold breach is observed.
- `retain_artifacts`: tag artifacts from the Run with a retention reason so cleanup skips them until the configured age expires.

### Execution Semantics

- A real-time threshold evaluator runs while the Run is active. It evaluates metric samples for the run's snapshotted Target-Agent bindings at a fixed interval, using the Run start time as the lower bound and the current time as the upper bound.
- The existing after-run threshold profile scheduling becomes the fallback/final sweep for actions that are still meaningful after completion, such as `alert`, `profile`, and `retain_artifacts`.
- `stop_run` is only executed by the real-time evaluator while the Run is active. During the final sweep, `stop_run` records a skipped action event instead of pretending to stop an already-terminal Run.
- `alert` is always safe and should remain the default behavior.
- `profile` reuses the existing Profile Task creation path and controlled template rules.
- `stop_run` is only valid for active runs and is idempotent.
- `retain_artifacts` affects cleanup policy, not immediate storage.
- Each action creates a run event with type `threshold_action`.

### Idempotency And Cooldown

Each threshold action has a dedupe key:

```text
runId | targetId | agentId | metric | actionType | thresholdValue
```

The same action key can be executed once per Run by default. `profile` may optionally define `cooldownSeconds`; when present, the dedupe key is extended with a cooldown window bucket so long runs can take repeated samples without creating unbounded tasks. The default cooldown is one execution per Run.

`stop_run` is idempotent: repeated attempts against an already stopping or terminal run create no additional stop request and append at most one `threshold_action` event for that action key.

### Failure Handling

Action failures must not crash the Run worker. The run event log records:

- action type
- metric
- observed value
- threshold value
- status: `succeeded` or `failed`
- error message when failed

Reports include these events in the alert/action timeline.

`threshold_action` events store structured fields in `metadataJson`; Reports must read metadata instead of parsing the human-readable message.

## Workspace Scope And Security

Every new REST endpoint and WebSocket endpoint must enforce workspace scope consistently with existing APIs:

- Header scope for REST where possible.
- Query scope for download, archive, EventSource, and WebSocket links.
- Cross-scope single-resource access returns `403`.
- Unscoped requests preserve local prototype compatibility except for sensitive retention cleanup, which requires explicit workspace scope.
- Unscoped compatibility never bypasses executable-resource invariants. Profile Task create, lease, and complete paths always validate referenced Agent, Run, and Target workspace consistency.

Package downloads remain unscoped because Agent install artifacts are release assets, not workspace data.

Artifact archive and cleanup are sensitive operations. Archive requires either explicit workspace scope or a `runId` / artifact set that resolves to one workspace; cross-scope results are rejected. Cleanup always requires explicit workspace scope. Future auth/RBAC can add user-level authorization around the same handler boundaries.

## Data Persistence

SQLite remains the only database for this slice.

Expected schema additions:

- Profile artifacts: archival and deletion metadata.
- Profile artifact retention policy: typed table keyed by `project_id` and `environment`.
- Threshold actions: JSON field embedded in Target metric threshold config.
- Run target-agent snapshots: immutable Run creation snapshot for historical Target and Process trends.
- Run events: add `metadata_json TEXT NOT NULL DEFAULT '{}'` and support `threshold_action` event type.

Existing migration style should be preserved: startup should add missing columns or tables without requiring a separate migration command.

## Frontend Design

The frontend should stay operational and dense rather than marketing-like.

Targets & Agents:

- Show release package links from `/agent/binaries/manifest.json`.
- Show install choices: install script, deb, rpm.
- Keep token handling separate from package download because packages do not include secrets.

Runs:

- Prefer WebSocket when available.
- Keep EventSource and polling fallbacks.
- Add a visible stop action state when WebSocket acknowledges `stop`.

Reports:

- Add a Historical trends section.
- Add threshold action timeline rows.
- Add archive download controls filtered by latest run or current report filters.

Settings or Reports retention panel:

- Show retention policy fields.
- Provide dry-run cleanup and explicit cleanup actions.
- Display matched/deleted/freed summary.

## Testing Strategy

Backend tests:

- Package builder creates binary, checksum, manifest, deb, and rpm entries. The RPM test must verify the file has an RPM header and can expose package metadata; if the test environment has `rpm`, also run `rpm -qip` and `rpm -qlp` against the generated artifact.
- `/agent/binaries/{name}` serves package files and checksums.
- WebSocket endpoint sends initial state, streams events, handles `ping`, and handles `stop`.
- Trend endpoints aggregate seeded run, target metric, process, and artifact records using run target-agent snapshots, not the current Target binding.
- Archive endpoint returns a zip with expected files and manifest, rejects unsafe zip entry names, disambiguates duplicate names, skips missing files in the manifest, and returns `413` when file count or byte limits would be exceeded.
- Retention cleanup dry run requires workspace scope and reports matches without deleting files.
- Retention cleanup delete mode deletes only scoped workspace files and updates metadata.
- Threshold actions create expected Profile Tasks for breached Agents, stop active runs during real-time evaluation, skip `stop_run` during final sweeps, apply dedupe/cooldown semantics, and append `threshold_action` events with `metadataJson`.
- Workspace scope rejects cross-scope archive, cleanup, WebSocket, and trend access.
- Profile Task creation rejects cross-workspace Run/Agent/Target combinations even without workspace headers.
- Agent polling does not lease persisted cross-workspace Profile Tasks, and completion rejects such tasks defensively.
- Targets page `target_manual` Profile Tasks can be created with `agentId + targetId` and no `runId` when the Agent is bound to the Target.

Frontend tests:

- Targets & Agents renders deb/rpm package choices from manifest.
- Runs uses WebSocket when available and still falls back when unavailable.
- Reports renders historical trend summaries.
- Reports or retention panel performs dry-run cleanup and displays results.
- Threshold action timeline appears in report details.

Verification commands:

```bash
cd backend
GOPROXY=https://proxy.golang.org,direct go test ./...

cd ../frontend
npm test -- --run
npm run build
```

The local environment may have a company `GOPROXY` that returns an HTTPS mismatch. The verified workaround is to run backend tests with `GOPROXY=https://proxy.golang.org,direct`.

## Rollout Plan

1. Correct stale documentation so the baseline is truthful.
2. Add package outputs and manifest expansion.
3. Add WebSocket run control while preserving EventSource and polling.
4. Add trend report backend APIs and frontend report section.
5. Add artifact archive download and retention cleanup.
6. Add advanced threshold actions.
7. Run full backend and frontend verification.

Each step should be independently shippable and should not require data loss or manual database migration.

## Open Decisions Resolved

- Storage remains SQLite plus local files.
- Package generation is implemented in Go and does not require external packaging tools.
- WebSocket is added only for Run events and stop control in the MVP.
- Artifact archive format is zip with a metadata manifest.
- Advanced threshold actions are limited to alert, profile, stop_run, and retain_artifacts.
- Existing SSE remains supported after WebSocket is added.
