# Run Events

The platform records every load run event in SQLite and exposes the events through both JSON polling and Server-Sent Events.

## Endpoints

```text
GET /api/runs/{id}/events
GET /api/runs/{id}/events/stream
```

`/events` returns the latest event list as JSON.

`/events/stream` returns `text/event-stream` and pushes existing plus newly appended events until a terminal event is emitted.

## Event Types

| Event | Meaning |
|---|---|
| `run_started` | The run record was created and accepted. |
| `run_progress` | A request sample completed and metrics were updated. |
| `run_finished` | The run completed normally. |
| `run_canceled` | The run was stopped by the user. |
| `run_aborted` | A guardrail aborted the run. |

## SSE Format

```text
id: run-event-run-xxx-...
event: run_progress
data: {"id":"run-event-run-xxx-...","runId":"run-xxx","type":"run_progress","status":"running","message":"Run progress updated","successRequests":10,"failedRequests":0,"totalRequests":20,"qps":240,"p95LatencyMs":3,"createdAt":"2026-06-12T08:30:01Z"}
```

The frontend Runs page prefers `EventSource` against `/events/stream`; if the browser does not support `EventSource`, it falls back to polling `/events`.
