# Run Guardrails

`POST /api/runs` supports optional guardrails that can abort a load test before all requests are sent.

## Fields

| Field | Type | Behavior |
|---|---:|---|
| `maxErrorRatePercent` | number | Aborts the run when completed request error rate is greater than this percentage. |
| `maxP95LatencyMs` | number | Aborts the run when current p95 latency is greater than this value in milliseconds. |

Use `0` or omit the field to disable that guardrail.

## Terminal State

When a guardrail trips:

- The run status becomes `aborted`.
- Remaining unsent requests are canceled.
- `/api/runs/{id}/events` includes a `run_aborted` event.
- The configured guardrail thresholds are persisted with the run history.
- `/api/reports/runs/{id}` returns `alerts` and `summary.alertCount`; each alert includes `severity`, `kind`, `metric`, `observed`, `threshold`, `message`, and the related `run_aborted` event id when available.

## Example

```json
{
  "scenarioId": "scenario-xxx",
  "targetId": "target-xxx",
  "totalRequests": 100,
  "concurrency": 10,
  "timeoutMs": 1000,
  "maxErrorRatePercent": 2.5,
  "maxP95LatencyMs": 300
}
```

The Runs page exposes both guardrails as:

- `Max error rate percent`
- `Max p95 latency ms`

The Reports page renders triggered guardrails in `Alert summary`, for example:

```json
{
  "id": "run-xxx-guardrail-error-rate",
  "severity": "critical",
  "kind": "guardrail",
  "metric": "error_rate_percent",
  "observed": 50,
  "threshold": 10,
  "message": "Error rate guardrail exceeded: 50.0% > 10.0%",
  "eventId": "run-event-run-xxx-..."
}
```
