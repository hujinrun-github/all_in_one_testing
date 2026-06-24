# Workspace Scope / 工作区隔离

The platform uses `projectId` and `environment` as the workspace ownership boundary.

平台使用 `projectId` 和 `environment` 表示工作区归属。前端点击 `Apply scope` 后，会把当前工作区应用到 Scenario、Target、Agent、Run、Report、Agent metrics、Profile Task、Profile Artifact 和 Agent token 操作。

## Request Scope / 请求方式

Normal `fetch` requests send both headers:

普通 `fetch` 请求会同时传递请求头：

```http
X-AIT-Project-ID: project-checkout
X-AIT-Environment: staging
```

Browser-link style requests also include query parameters:

列表、SSE 和下载链接等浏览器请求也会携带 query 参数：

```text
?projectId=project-checkout&environment=staging
```

`EventSource` and profile artifact download links cannot set custom headers, so the backend also accepts query scope.

`EventSource` 和 profile artifact 下载链接无法设置自定义请求头，因此后端也接受 query scope。

## Control Plane Behavior / 控制台后端行为

When a workspace scope is present, the backend filters list APIs and rejects cross-scope single-resource reads or mutations.

携带 workspace scope 后，后端会过滤列表，并拒绝跨 scope 的单资源访问或变更：

- Scenario / Target / Agent list and create.
- Scenario / Target single-resource `GET`, `PUT`, and `DELETE`.
- Target health check and health check history.
- Run create, list, detail, events, SSE, and stop.
- Reports, including run report, multi-run compare, profile artifact compare, and process trend compare.
- Agent metrics query.
- Profile Artifact list and download.
- Profile Task list, create, and retry.
- Agent token rotate and revoke.

If no header or query scope is provided, the backend keeps the prototype-compatible full-access behavior for local debugging and migration scripts.

未携带 header 或 query scope 时，后端保留原型阶段的全量兼容行为，便于本地调试和旧脚本迁移。

## Agent Token Boundary / Agent Token 边界

Agent endpoints always enforce token ownership after authentication:

Agent 侧接口在 token 认证通过后，会继续校验 token 归属：

- `/agent/v1/metrics`: the token can only submit metrics for an agent registered by the same token.
- `/agent/v1/profile-tasks`: the token can only poll tasks for its own registered agent.
- `/agent/v1/profile-tasks/{id}/complete`: the token can only complete tasks assigned to its own registered agent.
- `/agent/v1/profile-artifacts`: if the uploaded artifact references a known run, the run must be in the token's `projectId` / `environment`.

This prevents a valid token from impersonating another agent or uploading profiling artifacts into another workspace.

这可以防止“合法 token 冒充其他 Agent”或“把 profile artifact 上传到其他工作区 Run”的问题。
