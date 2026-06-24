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

The MVP builder writes minimal package files directly in Go:

- Debian package: `ar` archive containing `debian-binary`, `control.tar.gz`, and `data.tar.gz`.
- RPM package: minimal RPM v3/v4-compatible file with metadata and payload sufficient for `rpm -qip` and installation on common RPM systems.

If direct RPM generation becomes too risky during implementation, the acceptable fallback is to generate a `.tar.gz` package plus an RPM metadata stub only after documenting that limitation in the implementation plan. The preferred design remains real `.rpm` output.

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

The response is a zip file containing matching stored files. The zip should include a `manifest.json` with artifact metadata and original file names. Missing files are skipped and recorded in the manifest under `missingArtifacts`.

### Retention Policy

Add retention policy APIs:

```text
GET  /api/profile-artifact-retention
PUT  /api/profile-artifact-retention
POST /api/profile-artifacts/cleanup
```

Policy fields:

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

`POST /api/profile-artifacts/cleanup?dryRun=true` performs a dry run. `dryRun=false` deletes files and marks records as archived or deleted according to the store model chosen during implementation.

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
- `profile`: create a Profile Task for each bound Agent.
- `stop_run`: stop the active Run when a threshold breach is observed.
- `retain_artifacts`: tag artifacts from the Run with a retention reason so cleanup skips them until the configured age expires.

### Execution Semantics

- Actions run after a metric breach is detected during or immediately after a Run.
- `alert` is always safe and should remain the default behavior.
- `profile` reuses the existing Profile Task creation path and controlled template rules.
- `stop_run` is only valid for active runs and is idempotent.
- `retain_artifacts` affects cleanup policy, not immediate storage.
- Each action creates a run event with type `threshold_action`.

### Failure Handling

Action failures must not crash the Run worker. The run event log records:

- action type
- metric
- observed value
- threshold value
- status: `succeeded` or `failed`
- error message when failed

Reports include these events in the alert/action timeline.

## Workspace Scope And Security

Every new REST endpoint and WebSocket endpoint must enforce workspace scope consistently with existing APIs:

- Header scope for REST where possible.
- Query scope for download, archive, EventSource, and WebSocket links.
- Cross-scope single-resource access returns `403`.
- Unscoped requests preserve local prototype compatibility.

Package downloads remain unscoped because Agent install artifacts are release assets, not workspace data.

Artifact archive and cleanup are sensitive operations. The MVP relies on workspace scope and local deployment trust. Future auth/RBAC can add user-level authorization around the same handler boundaries.

## Data Persistence

SQLite remains the only database for this slice.

Expected schema additions:

- Profile artifacts: archival and deletion metadata.
- Retention policy: singleton key/value table or typed table.
- Threshold actions: JSON field embedded in Target metric threshold config.
- Run events: no new table needed; add `threshold_action` event type.

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

- Package builder creates binary, checksum, manifest, deb, and rpm entries.
- `/agent/binaries/{name}` serves package files and checksums.
- WebSocket endpoint sends initial state, streams events, handles `ping`, and handles `stop`.
- Trend endpoints aggregate seeded run, target metric, process, and artifact records.
- Archive endpoint returns a zip with expected files and manifest.
- Retention cleanup dry run reports matches without deleting files.
- Retention cleanup delete mode removes files and updates metadata.
- Threshold actions create expected Profile Tasks, stop active runs, and append `threshold_action` events.
- Workspace scope rejects cross-scope archive, cleanup, WebSocket, and trend access.

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
