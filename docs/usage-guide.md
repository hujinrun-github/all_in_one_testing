# All-in-One Testing 平台使用手册

本文档面向平台使用者和运维接入人员，覆盖从启动平台、创建压测场景、发起实际压测，到在被压测机器上安装 Agent、采集机器指标和 pprof / prof 数据的完整流程。

> 当前代码处于原型阶段：前端已经具备控制台页面、HTTP / CUSTOM_RPC 场景表单、场景 API 接入、Agent token 生成/轮换/吊销、Agent 列表和最新指标展示、Run 历史展示、Profile Artifact 展示与下载、最新 Run 报告聚合展示、guardrail 告警摘要、多 Run 对比展示、跨 Run 进程趋势对比展示、profile artifact 对比展示、Profile Task 命令详情、慢请求样本展示、错误样本展示、Target HTTP health check 和按 Run 时间窗口聚合的 Target CPU/MEM/磁盘/网络指标摘要与趋势图，后端已经具备 `/api/health`、`/api/scenarios`、异步 HTTP / CUSTOM_RPC adapter 压测接口 `/api/runs`、Target health check 接口 `/api/targets/{id}/health-check`、Run 报告聚合接口 `/api/reports/runs/{id}`、Run 对比接口 `/api/reports/compare`、进程趋势对比接口 `/api/reports/process-trends/compare`、profile artifact 对比接口 `/api/reports/profile-artifacts/compare`、Agent token 创建/过期/轮换/吊销接口、Agent 安装脚本接口 `/agent/install.sh`、Agent 发行产物清单接口 `/agent/binaries/manifest.json`、Agent 二进制下载接口 `/agent/binaries/all-in-one-agent-linux-amd64`、Agent checksum 下载接口 `/agent/binaries/all-in-one-agent-linux-amd64.sha256`、Agent 心跳接口 `/agent/v1/heartbeat`、Agent 指标接口 `/agent/v1/metrics`、Agent 列表接口 `/api/agents`、Agent 指标时间线接口 `/api/agent-metrics`、Agent profile artifact 上传接口 `/agent/v1/profile-artifacts`、Profile Task 创建/轮询/完成接口、Profile Task 模板 CRUD 接口 `/api/profile-task-templates`、profile artifact 查询接口 `/api/profile-artifacts` 和下载接口 `/api/profile-artifacts/{id}/download`，并会把这些数据写入 SQLite。`cmd/agent` 已支持从 Go pprof base URL 按 `cpu`、`heap`、`goroutine`、`mutex`、`block` 等类型一次性采集并上传 profile artifact，也支持执行显式配置的受控命令 profiler、`--poll-once` 领取并处理一批 Profile Task，以及 `--daemon` 常驻上报心跳、指标并轮询任务；`cmd/package-agent` 已支持生成 Linux amd64 Agent 二进制和 `.sha256`；Targets 页面已支持从 SQLite 白名单模板下发 `Go pprof` 和 `Linux perf`，并支持基础模板创建、更新和删除；当 Target 指标超过阈值时，Run 结束后会自动下发 `threshold_auto` Profile Task；当前已支持 `/agent/install.sh` systemd bootstrap 安装，deb/rpm 等系统发行包仍是目标能力，本文会明确标注“当前可用”和“目标流程”。

## 1. 核心概念

| 概念 | 说明 |
|---|---|
| Control Plane | 平台控制面，包含 Go 后端 API、调度、场景管理、运行管理、报告管理等能力。 |
| Web Console | React 前端控制台，用于配置 Target、Agent、Scenario、Run 和 Report。 |
| Target | 被压测对象，可以是服务、接口、机器、集群或进程。 |
| Target Agent | 部署在被压测机器上的采集进程，负责主机指标、进程指标、健康检查和 profiling 产物采集。 |
| Scenario | 压测场景，描述一次或多次 HTTP / RPC 调用、参数、断言和变量提取。 |
| Run | 一次实际压测运行，包含场景快照、负载模型、目标机器、采集配置和运行结果。 |
| Metrics | 压测侧指标和目标机器指标，包括 QPS、错误率、延迟、CPU、内存、磁盘 IO、网络 IO、进程资源等。 |
| Profile Artifact | 压测过程中采集的 CPU pprof、heap、goroutine、perf.data、火焰图等性能分析产物。 |

## 2. 当前能力状态

### 当前已经可用

- 启动 Go 后端 API 服务。
- 访问 `/api/health` 做健康检查。
- 使用 `/api/runs` 对一个 HTTP / HTTPS URL 发起同步压测，并查询最近运行历史。
- 使用 `/api/scenarios` 创建、查询、更新和删除 HTTP 场景和 `CUSTOM_RPC` adapter 场景；原型阶段默认写入 SQLite 数据库。
- 使用 `/api/agent-tokens` 创建 Agent token，使用 `/agent/v1/heartbeat` 上报 Agent 心跳，使用 `/agent/v1/metrics` 上报最新主机/进程指标，使用 `/api/agents` 查询已注册 Agent 和最新指标，使用 `/api/agent-metrics` 查询指标时间线；Linux Agent daemon 会通过 `/proc` 采集主机 CPU、内存、磁盘、网络和进程指标，非 Linux 环境会降级为 Agent 自身进程快照；Agent 上报接口需要携带 `Authorization: Bearer <token>`。
- 使用 `/agent/v1/profile-artifacts` 上传 Agent 已采集的 pprof / prof 文件，后端保存 artifact 文件和元数据。
- 使用 `cmd/agent` 一次性从 Go pprof base URL 按类型采集 profile，或者执行显式配置的受控命令 profiler，并上传到 Control Plane。
- 使用 `cmd/agent --daemon` 常驻运行，周期性上报 heartbeat、metrics，并轮询 Profile Task；单次上报或拉取失败会等待下一轮重试。
- 使用 `/api/profile-tasks` 创建采集任务，任务可以是 Go pprof URL 采集，也可以是受控命令 profiler；Agent 使用 `/agent/v1/profile-tasks` 领取任务，采集上传后使用 `/agent/v1/profile-tasks/{id}/complete` 回写完成状态；任务支持 `attempts/maxAttempts` 失败重试，并可用 `runId`、`agentId`、`status`、`limit` 过滤查询；已 `failed` 的任务可通过 `/api/profile-tasks/{id}/retry` 手动重新入队；Reports 页面会按最新 Run 展示相关任务队列和最近错误，并可对失败任务点击 Retry。
- 使用 `/api/targets` 创建、查询、更新和删除 Target，把 Target 绑定到已注册 Agent，并保存 CPU/MEM/磁盘/网络指标告警阈值和 HTTP health check 配置；可调用 `/api/targets/{id}/health-check` 对 Target 做一次性健康探测。
- 使用 `/api/runs` 执行带 Target 的场景时，平台会在压测期间从 Target 的 `profileEndpoint` 拉取一次 1 秒 CPU pprof，保存到 SQLite 数据库同目录下的 `profile_artifacts` 子目录，并通过 `/api/profile-artifacts` 查询元数据、通过 `/api/profile-artifacts/{id}/download` 下载原始文件；如果 Target 绑定了 Agent，Run 创建时也会自动为每个绑定 Agent 创建一个 `cpu` Profile Task，由 Agent daemon 或 `--poll-once` 领取并上传采集结果；如果 Target 配置了 `metricThresholds` 且窗口内 Agent 指标超过阈值，Run 结束后还会创建 `threshold_auto` 来源的 `cpu` Profile Task。
- 使用 `/api/reports/runs/{id}` 查询单次 Run 报告时，平台会返回 `alerts` / `summary.alertCount` 展示 guardrail 告警摘要和 Target 指标阈值告警；当 `maxErrorRatePercent` 或 `maxP95LatencyMs` 触发中止时，alert 会包含严重级别、触发指标、观测值、阈值和关联 `run_aborted` 事件。平台也会按 Run 的开始时间和运行时长对齐 Target 绑定 Agent 的指标样本，并返回 `targetMetrics`，包含采样窗口、样本数、CPU/MEM/磁盘/网络峰值、窗口内 samples、关联 Agent 列表、Target 进程匹配规则、Target `metricThresholds`、按 PID 聚合的窗口内 `processTrends` 进程趋势和按 `processMatch.cmdlineContains` 优先过滤并以 `processMatch.name` 兜底的最新进程快照；当窗口内峰值超过 Target 阈值时，报告会派生 `kind=target_metric` 的 warning 告警；当 Run 绑定了 Target 时，报告还会返回最近 `targetHealthChecks`，用于展示目标健康状态、HTTP 状态码、探测时间和错误原因；Reports 页面会基于 samples 绘制 CPU/MEM/磁盘/网络趋势图，并展示进程 CPU/RSS/fd/thread 峰值和 Target 健康历史。
- 使用 `/api/reports/process-trends/compare?limit=5` 对比最近 Run 的 Target 进程趋势，平台会按 Target + Agent + 进程名 + cmdline 分组，保留最新/上一次 Run 的 PID，并计算 CPU max、RSS max、fd/thread max、CPU delta 和 RSS delta。
- 使用 `/api/reports/profile-artifacts/compare?limit=5` 对比最近 Run 的 profile artifacts，按 `profileType` 聚合 artifact 数量、采集成功/失败数量、总大小、最新/上一次大小差和状态变化。
- 启动 React 前端控制台。
- 在前端查看 Dashboard、Targets & Agents、Scenarios、Runs、Reports 页面。
- 在 Targets & Agents 页面查看后端返回的 Agent 列表和最新 CPU / MEM / 进程指标，创建 Target 并绑定 Agent。
- 在 Scenarios 页面创建 HTTP 场景配置，前端会优先保存到后端，后端不可用时保留浏览器本地缓存兜底。
- 在 Runs 页面选择已保存场景，配置请求数、并发和超时后发起真实压测，并查看后端保存的历史运行记录。
- HTTP 场景表单支持：
  - 协议下拉框。
  - 方法下拉框。
  - Base URL。
  - Request path。
  - Query params 候选和权重随机选择。
  - Header KV 输入。
  - Body JSON 候选、权重随机选择和 JSON 格式校验。
  - Timeout。
  - Retry count。
  - Success assertion。
  - 已启用 HTTP flow request steps 和 status assertion steps。
- CUSTOM_RPC 场景表单支持：
  - Adapter URL。
  - Adapter path。
  - RPC method。
  - Header KV。
  - Body JSON 候选、权重随机选择和 JSON 格式校验。

### 当前尚未接入真实后端的能力

- Agent deb/rpm 等系统发行包。
- 项目/环境隔离和完整历史多维趋势报告。
- 完整历史多维趋势报告和高级阈值动作。
- Agent deb/rpm 等系统发行包和控制台模板化 profiler 调度。
- 压测过程 SSE 事件流推送，WebSocket 双向控制仍为后续增强。
- 历史报告、趋势分析和 artifact 归档。

## 3. 本地启动平台

### 3.1 启动后端

进入后端目录：

```powershell
cd D:\MyGitProject\all_in_one_testing\backend
go run .\cmd\server
```

默认监听：

```text
http://127.0.0.1:8080
```

如需修改监听地址：

```powershell
$env:APP_ADDR='127.0.0.1:8081'
go run .\cmd\server
```

场景存储默认写入 SQLite 数据库：

```text
backend/data/platform.db
```

如需指定数据库文件：

```powershell
$env:SCENARIO_DB_PATH='D:\MyGitProject\all_in_one_testing\backend\data\platform.local.db'
go run .\cmd\server
```

验证后端健康：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/health
```

预期返回：

```json
{
  "status": "ok"
}
```

### 3.2 启动前端开发服务

首次安装依赖：

```powershell
cd D:\MyGitProject\all_in_one_testing\frontend
npm install
```

启动 Vite：

```powershell
npm run dev
```

默认访问：

```text
http://127.0.0.1:5173
```

开发服务会将 `/api` 请求代理到：

```text
http://127.0.0.1:8080
```

### 3.3 构建并预览静态页面

构建：

```powershell
cd D:\MyGitProject\all_in_one_testing\frontend
npm run build
```

预览：

```powershell
python -m http.server 5175 --bind 127.0.0.1 --directory D:\MyGitProject\all_in_one_testing\frontend\dist
```

访问：

```text
http://127.0.0.1:5175
```

如果页面没有变化，先重新执行 `npm run build`，再用 `Ctrl+F5` 强制刷新。

## 4. 平台页面使用说明

### 4.1 Dashboard

Dashboard 用于查看平台总览：

- 当前 QPS。
- p95 延迟。
- 错误率。
- 在线 Agent 数。
- 运行中的压测阶段。
- Agent 健康状态。
- profile 采样状态。
- 最近运行记录。

当前 Dashboard 会优先读取后端 Run 历史和 Agent 数据；当后端不可用或暂无数据时，会保留静态示例用于展示页面结构。运行中的 Run 会定时刷新进度，并可从 Dashboard 直接停止。

顶栏的 `Project` / `Env` 是当前工作区 scope。填写例如 `project-checkout` / `staging` 后点击 `Apply scope`，前端会使用同一组 `projectId` / `environment` 重新读取 Scenario、Target 和 Agent 列表，并在 scoped API 请求中同时携带 `X-AIT-Project-ID` / `X-AIT-Environment` 请求头；创建新的 Scenario、Agent token 或 Target 时，也会默认带入这组项目和环境。后端收到这两个 header 后，会限制列表范围，并在创建 Scenario、Target 或 Agent token 时拒绝 body 中项目/环境与 header 不一致的跨 scope 写入；未点击 `Apply scope` 时页面保持全量读取，表单默认使用 `default` / `default`。

### 4.2 Targets & Agents

Targets & Agents 用于管理被压测目标和机器 Agent：

- 查看服务拓扑。
- 查看 Agent 在线状态。
- 查看后端 `/api/agents` 返回的 Agent 列表。
- 查看主机采集项。
- 配置 Target 与 Agent 的绑定关系。
- 配置 pprof / prof endpoint。

当前页面已接入 Agent token 生成、过期时间展示、轮换、吊销、Agent 列表、最新指标展示、Target 创建/更新/删除、Agent 绑定/解绑、Target 指标阈值配置和 Target HTTP health check：顶栏应用 scope 后，页面会调用 `/api/agents?projectId=...&environment=...` 和 `/api/targets?projectId=...&environment=...` 读取当前项目/环境的数据，并在请求头中携带 `X-AIT-Project-ID` / `X-AIT-Environment`；页面也会预填 Agent token 与 Target 表单的项目/环境。点击 `Generate token` 会调用 `/api/agent-tokens` 创建一次性展示的 token 和基于 `/agent/install.sh` 的安装命令；当 scope 已应用时，请求头和 body 里的项目/环境必须一致，否则后端会返回 `403`。点击 `Rotate` 会调用 `/api/agent-tokens/{id}/rotate` 替换明文密钥并立即使旧密钥失效；点击 `Revoke` 会调用 `/api/agent-tokens/{id}/revoke` 吊销 token。Agent 携带有效 token 通过 `/agent/v1/heartbeat` 上报后，会写入 SQLite，并出现在页面的 `Agent inventory` 区域；Agent 携带同一个 token 通过 `/agent/v1/metrics` 上报后，页面会展示 CPU、MEM 和 CPU 最高的进程；在 `Target binding` 面板填写 Target 名称、base URL、pprof endpoint、进程匹配规则、Health check path、Expected health status、Health timeout ms 和 CPU/MEM/磁盘/网络阈值后，平台会调用 `/api/targets` 保存 Target，并默认绑定当前第一个在线 Agent。Target 列表中的 `Unbind` 会调用 `/api/targets/{id}` 更新 `agentIds=[]`，解绑后不会继续显示手工 `采集 / Profile` 入口；`健康检查 / Check` 会调用 `/api/targets/{id}/health-check`，用 `baseUrl + healthCheck.path` 发起一次 GET，并展示 healthy / unhealthy 和实际 HTTP 状态码；Profile templates 面板会调用 `/api/profile-task-templates` 读取 SQLite 白名单模板，支持创建、编辑和删除模板，默认支持 `Go pprof` 和 `Linux perf`。选择 `Go pprof` 时需要配置 pprof endpoint 并选择 profile 类型；选择 `Linux perf` 时填写 PID，页面会按模板生成 `perf record -F 99 -p <pid> -g -o {{output}} -- sleep {{seconds}}` 参数。采集秒数必须为 `1..300`，点击 `采集 / Profile` 后调用 `/api/profile-tasks` 手动创建 Profile Task，默认 `maxAttempts=3`，由绑定 Agent 下一轮轮询采集。Runs 页面已经可以选择当前 scope 内的已保存 Target，Reports 页面会用这些阈值生成 `target_metric` 告警；Agent daemon 会通过 `/agent/v1/targets` 拉取绑定 Target，并通过 `/agent/v1/target-health-checks` 周期性上报 Target health 结果。

当前页面是原型展示。目标流程如下：

1. 在平台创建 Target。
2. 生成 Agent token。
3. 在被压测机器安装 Agent。
4. Agent 上报心跳。
5. 平台将 Agent 绑定到 Target。
6. 配置进程匹配、端口检查、HTTP health check、pprof endpoint。

### 4.3 Scenarios

Scenarios 用于创建压测场景。

当前前端支持创建 HTTP 场景和 `CUSTOM_RPC` adapter 场景，字段说明如下：

| 字段 | 说明 | 示例 |
|---|---|---|
| Scenario name | 场景名称 | `payment-http-smoke` |
| Project ID | 场景所属项目，用于后端保存和列表过滤 | `project-checkout` |
| Environment | 场景所属环境，用于区分 `dev` / `staging` / `prod` 等压测配置 | `staging` |
| Protocol | 协议，下拉选择 | `HTTP` |
| Method | HTTP 方法，下拉选择 | `GET`、`POST` |
| Base URL | 目标服务基础地址 | `https://api.example.test` |
| Request path | 请求路径 | `/api/payments` |
| Query variants | 多个查询参数候选，每个候选配置权重，压测时按权重随机选择；不需要开头的 `?` | `80% tenant=shanghai&debug=true`、`20% tenant=beijing&debug=false` |
| Headers | KV 形式请求头 | `Authorization` = `Bearer ${token}` |
| Body variants | 多个 JSON 请求体候选，每个候选配置权重，压测时按权重随机选择 | `70% { "amount": 100 }`、`30% { "amount": 300 }` |
| Timeout | 单请求超时时间，毫秒 | `2500` |
| Retry count | 单请求失败重试次数 | `2` |
| Success assertion | 成功断言 | `status < 400` |

创建后，场景会显示在页面的 `Created scenarios` 区域，卡片会展示 `project <Project ID>` 和 `env <Environment>`，并通过后端 `/api/scenarios` 保存到 SQLite 数据库。顶栏应用 Project / Environment scope 后，Scenarios 页面会调用 `/api/scenarios?projectId=...&environment=...` 并携带 `X-AIT-Project-ID` / `X-AIT-Environment`，只展示当前 scope 的场景；新建场景表单也会默认带入这组项目和环境，提交时 body 与 header 必须一致。刷新页面、重启前端或换浏览器后，只要连接同一个后端并使用同一个数据库文件，场景都会从后端恢复。当前前端仍保留 `localStorage` 兜底：当后端不可用时，场景会暂存在当前浏览器。

当前后端会执行已启用的 HTTP flow request steps。同一个压测样本内会使用独立 Cookie Jar：上一步响应里的 `Set-Cookie` 会自动带到后续 HTTP step；不同并发样本之间不会共享 Cookie。后端也支持有限的状态码断言语法：

```text
status < 400
status <= 399
status == 200
status != 500
status >= 200
status > 199
```

场景级 `Success assertion` 会作用于普通单请求场景，也会作为未单独配置断言的 request step 的默认断言。`type=assertion` 的 flow step 不会发起 HTTP 请求，而是校验上一条 HTTP response；断言不满足时该 sample 计入 failed request，并会进入错误样本。

`assertion` 字符串用于状态码断言；`assertions` 数组用于响应内容断言，当前支持 `source=json`、`source=header`、`source=body`，支持 `equals`、`not_equals`、`contains`、`not_contains`、`matches`、`not_matches`、`exists`。`expected` 中可以引用前面提取出的变量，例如 `${token}`；`matches` 和 `not_matches` 的 `expected` 是 Go regexp 正则表达式。

```json
{
  "type": "assertion",
  "enabled": true,
  "assertions": [
    { "name": "state ok", "source": "json", "path": "state", "operator": "equals", "expected": "ok" },
    { "name": "trace header", "source": "header", "path": "X-Trace-Id", "operator": "exists" },
    { "name": "body marker", "source": "body", "operator": "contains", "expected": "success" },
    { "name": "order regex", "source": "body", "operator": "matches", "expected": "order=ORD-[0-9]+" }
  ]
}
```

变量提取当前支持三种来源：

```json
{
  "type": "extract",
  "enabled": true,
  "extractors": [
    { "name": "token", "source": "json", "path": "data.token" },
    { "name": "session", "source": "header", "path": "X-Session-Id" },
    { "name": "order", "source": "regex", "path": "order=(ORD-[0-9]+)" }
  ]
}
```

`source=json` 使用点号路径从上一条 HTTP 响应 JSON body 中提取字段；`source=header` 从上一条响应 Header 中提取值；`source=regex` 会对上一条响应 body 执行 Go regexp，优先返回第一个捕获组，没有捕获组时返回完整匹配。提取出的变量可在后续 request step 的 path、Header value、Query variant 和 Body variant 中通过 `${token}`、`${session}`、`${order}` 引用。

flow step 也可以配置 `when` 条件，用于在压测样本内按上一条响应或已提取变量决定是否执行当前 step。条件不满足、上一条响应不存在或变量不存在时，该 step 会被跳过，不会把 sample 记为失败；如果条件配置本身无效，例如正则表达式错误或操作符不支持，sample 会失败并记录错误。

```json
{
  "type": "request",
  "enabled": true,
  "method": "POST",
  "path": "/checkout/${token}",
  "when": {
    "name": "mode is run",
    "source": "variable",
    "path": "mode",
    "operator": "equals",
    "expected": "run"
  }
}
```

`when.source` 支持 `json`、`header`、`body`、`regex` 和 `variable`。`json/header/body/regex` 会读取上一条 HTTP 响应；`variable` 会读取前面 extract step 提取出的变量。`when.operator` 支持 `equals`、`not_equals`、`contains`、`not_contains`、`matches`、`not_matches`、`exists`。

### 4.4 Scenario API

后端已经提供原型阶段的场景 CRUD：

```text
GET    /api/scenarios
POST   /api/scenarios
GET    /api/scenarios/{id}
PUT    /api/scenarios/{id}
DELETE /api/scenarios/{id}
```

列表接口可按项目和环境过滤：

```text
GET /api/scenarios?projectId=project-checkout&environment=staging
```

`projectId` 和 `environment` 都是可选参数；只传一个时按单字段过滤，不传时返回当前数据库里的全部场景。老数据和未填写字段的新数据会归到 `default` / `default`。

如果需要让后端执行 workspace 隔离校验，可以同时带上请求头：

```powershell
$workspaceHeaders = @{
  "X-AIT-Project-ID" = "project-checkout"
  "X-AIT-Environment" = "staging"
}

Invoke-RestMethod `
  -Uri "http://127.0.0.1:8080/api/scenarios?projectId=project-checkout&environment=staging" `
  -Headers $workspaceHeaders
```

带上这两个 header 后，后端会把列表请求限制在该 scope 内；创建 Scenario 时，如果 body 中的 `projectId` / `environment` 与 header 不一致会返回 `403`，缺省时会使用 header 填充。Scenario 单资源 `GET`、`PUT`、`DELETE` 也会校验 header scope，跨 scope 访问返回 `403`。未带 header 时保留原型阶段的兼容行为。

创建场景示例：

```powershell
$scenario = @{
  name = "gateway-health-smoke"
  projectId = "project-checkout"
  environment = "staging"
  protocol = "HTTP"
  method = "GET"
  baseUrl = "http://127.0.0.1:8080"
  path = "/api/health"
  queryVariants = @()
  headers = @()
  bodyVariants = @()
  timeoutMs = "1000"
  retryCount = "0"
  assertion = "status < 400"
} | ConvertTo-Json -Depth 8

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/scenarios" `
  -ContentType "application/json" `
  -Body $scenario
```

默认持久化文件为 SQLite 数据库 `backend/data/platform.db`。这仍是原型存储方式，适合本地联调和验证接口；当前 Scenario、Target、Agent token、Agent inventory 和 Run history 已保存 `projectId` 和 `environment`，Report 会沿用 Run / Target 归属做聚合展示；后续生产化还需要补齐权限模型，或在多节点部署时替换为 PostgreSQL。

### 4.5 CUSTOM_RPC adapter 契约

`CUSTOM_RPC` 用于接入平台不直接理解的业务协议。平台不会在 Go 进程里实现具体 RPC 协议，而是把每个压测样本转成统一 JSON 请求，发送给用户提供的 adapter；adapter 再负责调用真实 gRPC、Dubbo、Thrift、私有 TCP 协议或 SDK。

创建场景时：

- `protocol` 填 `CUSTOM_RPC`。
- `baseUrl + path` 是 adapter 的 HTTP 地址，例如 `http://127.0.0.1:9090/invoke`。
- `method` 是业务 RPC 方法名，会保留大小写，例如 `checkout.OrderService/CreateOrder`。
- `bodyVariants` 是业务请求 payload 候选，平台按权重随机选择后作为 JSON body 透传给 adapter。

场景示例：

```powershell
$scenario = @{
  name = "checkout-custom-rpc"
  protocol = "CUSTOM_RPC"
  method = "checkout.OrderService/CreateOrder"
  baseUrl = "http://127.0.0.1:9090"
  path = "/invoke"
  headers = @(
    @{ key = "X-Adapter-Token"; value = "local-dev" }
  )
  queryVariants = @()
  bodyVariants = @(
    @{ name = "book"; weight = "100"; body = '{"sku":"book"}' }
  )
  timeoutMs = "1000"
  retryCount = "0"
} | ConvertTo-Json -Depth 8

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/scenarios" `
  -ContentType "application/json" `
  -Body $scenario
```

平台调用 adapter 时固定使用 `POST`，请求体格式如下：

```json
{
  "protocol": "CUSTOM_RPC",
  "method": "checkout.OrderService/CreateOrder",
  "runId": "run-1780000000000000000",
  "scenarioId": "scenario-1780000000000000000",
  "headers": {
    "X-Adapter-Token": "local-dev"
  },
  "queryParams": "",
  "body": {
    "sku": "book"
  }
}
```

adapter 返回示例：

```json
{
  "success": true,
  "statusCode": 200
}
```

判定规则：

- adapter HTTP 状态码必须是 `2xx`。
- `success=false` 会被记为失败样本，`error` 字段会进入错误样本。
- `statusCode >= 400` 会被记为失败样本。
- `success` 缺省时，只要 adapter HTTP 状态码是 `2xx` 且 `statusCode < 400`，平台会认为本次样本成功。

### 4.6 Target API

后端已提供原型阶段的 Target 创建、查询、更新和删除接口：

```text
GET  /api/targets
POST /api/targets
GET  /api/targets/{id}
PUT  /api/targets/{id}
DELETE /api/targets/{id}
POST /api/targets/{id}/health-check
GET  /api/targets/{id}/health-checks?limit=5
```

Target 用于描述被压测服务，并绑定到已注册 Agent。创建 Target 时，`agentIds` 中的每个 Agent 必须已经通过 `/agent/v1/heartbeat` 注册成功。

`GET /api/targets` 可按项目和环境过滤：

```text
GET /api/targets?projectId=project-checkout&environment=staging
```

`projectId` 和 `environment` 都是可选参数；只传一个时按单字段过滤，不传时返回当前数据库里的全部 Target。老数据和未填写字段的新 Target 会归到 `default` / `default`。

`X-AIT-Project-ID` / `X-AIT-Environment` 对 Target API 同样生效：带 header 查询时只返回该 scope 的 Target；创建 Target 时 body 与 header 的项目/环境必须一致，否则返回 `403`。Target 单资源 `GET`、`PUT`、`DELETE`、`POST /api/targets/{id}/health-check` 和 `GET /api/targets/{id}/health-checks` 也会校验 header scope，跨 scope 访问返回 `403`。前端点击 `Apply scope` 后会自动同时传 query 参数和这两个 header。

创建 Target 示例：

```powershell
$target = @{
  name = "checkout-service"
  projectId = "project-checkout"
  baseUrl = "http://checkout.internal:8080"
  environment = "staging"
  agentIds = @("agent-checkout-01")
  profileEndpoint = "http://127.0.0.1:6060/debug/pprof"
  processMatch = @{
    name = "checkout"
    cmdlineContains = "--config=/etc/checkout/config.yaml"
  }
  healthCheck = @{
    enabled = $true
    path = "/ready"
    expectedStatus = 204
    timeoutMs = 1500
  }
  metricThresholds = @{
    cpuMaxPercent = 70
    memoryMaxPercent = 75
    diskReadMaxBytesPerSec = 10485760
    diskWriteMaxBytesPerSec = 10485760
    networkRxMaxBytesPerSec = 104857600
    networkTxMaxBytesPerSec = 104857600
  }
} | ConvertTo-Json -Depth 6

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/targets" `
  -ContentType "application/json" `
  -Body $target
```

查询 Target：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/targets
```

手工触发 Target 健康检查：

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/targets/target-xxx/health-check"
```

返回示例：

```json
{
  "targetId": "target-xxx",
  "targetName": "checkout-service",
  "status": "healthy",
  "url": "http://checkout.internal:8080/ready",
  "expectedStatus": 204,
  "observedStatus": 204,
  "latencyMs": 12.5,
  "checkedAt": "2026-06-17T21:00:00+08:00"
}
```

`healthCheck.path` 可以填写相对路径，例如 `/ready`，平台会拼接 Target `baseUrl`；也可以填写完整 URL。`expectedStatus` 默认 `200`，`timeoutMs` 默认 `1000`，最大 `30000`。当前健康检查既可以由 Control Plane 通过 `/api/targets/{id}/health-check` 主动发起一次 GET，也可以由 Agent daemon 周期性拉取绑定 Target 后在被压测机器侧执行 HTTP 探活并上报。

健康检查结果会同时写入 Target 的 `lastHealthCheck` 和 `target_health_checks` 历史表。单个 Target 默认保留最近 200 次检查；`GET /api/targets/{id}/health-checks?limit=5` 会按最近优先返回历史记录，`limit` 最大为 100。Targets 页面会加载最近 5 次记录，帮助判断目标服务是在压测前就不稳定，还是压测过程中才变差。

当前限制：高级阈值动作仍是后续增强；Target 已保存 `projectId` / `environment` 并支持列表过滤，Runs 页已经可以选择 Target，Reports 页已经会把 Target 绑定 Agent 的指标按 Run 时间窗口聚合为摘要，展示 CPU/MEM/磁盘/网络窗口趋势图，并在窗口峰值超过 `metricThresholds` 时派生 `target_metric` 告警。

### 4.7 Runs

Runs 用于启动、观察和停止压测任务。

目标流程：

1. 选择 Scenario。
2. 选择 Target / Agent。
3. 配置负载模型：
   - 并发数。
   - 总请求数。
   - 固定 QPS。
   - 阶段式升压。
   - 运行时长。
4. 配置保护阈值：
   - 错误率阈值。
   - p99 延迟阈值。
   - CPU 阈值。
   - 内存阈值。
5. 选择是否开启 profiling。
6. 启动压测。
7. 实时观察 QPS、延迟、错误率、目标机器状态和 profile 状态。
8. 结束后进入 Report。

当前 Runs 页面已经接入真实 Run API：可以选择已保存场景和已保存 Target，填写总请求数、并发数、超时时间和 Guardrails，点击 `Start run` 后调用后端 `/api/runs` 异步启动压测，并通过 `/api/runs/{id}` 轮询进度；如果选择了 Target，后端会使用 Target 的 `baseUrl` 拼接场景 path 发起压测。Target 配置了 `healthCheck` 时，后端会在创建 Run 前先执行一次健康预检；预检失败会返回 `409` 和 `target health preflight failed: ...`，Runs 页面会直接展示这个错误，不会创建 Run，也不会向业务接口发起压测请求。Target 配置了 `profileEndpoint` 时，平台会在压测期间同步拉取一次 CPU pprof artifact；如果 Target 绑定了 Agent，平台还会自动创建 `cpu` Profile Task，Agent daemon 下一轮轮询会领取并上传采集结果。运行结果、Guardrail 阈值、运行历史、Run event 日志、Profile Task 和 profile artifact 元数据都会写入 SQLite，Runs 页面会优先通过 `/api/runs/{id}/events/stream` SSE 展示 `run_started`、`run_progress`、`run_finished`、`run_canceled`、`run_aborted` 等事件；浏览器不支持 EventSource 时回退到 `/api/runs/{id}/events` 轮询。Run 报告已经按 Target 绑定 Agent 关联压测窗口内 CPU/MEM/磁盘/网络指标摘要和趋势，并会从 `run_aborted` 事件和已保存阈值派生 guardrail 告警摘要；WebSocket 双向控制和完整历史趋势报告仍是后续增强。

### 4.8 Reports

Reports 用于查看压测结果：

- 汇总结果。
- QPS 时间线。
- 延迟时间线。
- 错误率。
- 慢请求样本。
- 错误样本。
- Target 主机指标。
- 进程指标。
- profile artifacts。
- 瓶颈摘要。

当前页面已经会先读取 `/api/runs` 找到最新 Run，再调用 `/api/reports/runs/{id}` 聚合单次压测报告，展示成功率、错误率、Run event 数量、alert 数量、profile artifact 数量、慢请求样本、错误样本、QPS 和 p95；当 Run 因 `maxErrorRatePercent` 或 `maxP95LatencyMs` 中止时，报告会展示 `kind=guardrail` 告警摘要，包括严重级别、触发指标、观测值、阈值和关联事件；当 Run 绑定了 Target 且 Target 配置了 `metricThresholds` 时，报告会基于 Run 时间窗口内 Target 指标峰值派生 `kind=target_metric` 告警，支持 CPU、MEM、disk read/write 和 network RX/TX；当 Run 绑定了 Target 且 Target 绑定了 Agent 时，报告中的 `targetMetrics` 会展示 Run 时间窗口内的样本数、CPU/MEM/磁盘/网络峰值、CPU/MEM/磁盘/网络趋势图、关联 Agent、Target 进程匹配规则、`processTrends` PID 级进程趋势和按 `processMatch.cmdlineContains` 优先过滤并以 `processMatch.name` 兜底的最新进程快照；当 Run 绑定了 Target 时，报告中的 `targetHealthChecks` 会展示最近 Target 健康检查状态、HTTP 状态码、探测时间和错误原因，帮助判断压测失败是否由目标预先不健康导致；页面也会调用 `/api/reports/compare?limit=5` 对比最近多次 Run 的成功率、错误率、QPS 和 p95，并标出 best qps、fastest p95 和 highest errors；调用 `/api/reports/process-trends/compare?limit=5` 按 Target + Agent + 进程名 + cmdline 展示最近 Run 的进程峰值、最新/上一次 PID、CPU delta 和 RSS delta；调用 `/api/reports/profile-artifacts/compare?limit=5` 按 profile 类型展示最近 Run 的 artifact 数量、采集成功/失败、总大小、最新 artifact 相对上一次的大小 delta / 百分比变化，以及采集状态是否变化；同时会从 `/api/profile-artifacts` 读取最近 profile artifact 元数据，并为已采集成功的 artifact 提供下载入口；也会读取 `/api/agents` 和 `/api/agent-metrics`，展示第一个在线 Agent 的 CPU/MEM 指标趋势和最新进程摘要。没有后端数据时显示静态占位。

## 5. 当前如何进行实际压测

当前可用的真实压测入口有两个：

```text
POST /api/runs
GET  /api/runs
GET  /api/runs/{id}
GET  /api/runs/{id}/events
GET  /api/runs/{id}/events/stream
POST /api/runs/{id}/stop
```

`POST /api/runs` 会异步启动一次 HTTP 压测，立即返回 `running` 的 Run 摘要，并把运行状态写入 SQLite `run_history` 表。可以直接传 `url` 做临时压测，也可以传 `scenarioId` 执行已经保存到 SQLite 的场景快照；传入 `targetId` 时，后端会用 Target 的 `baseUrl` 覆盖场景中的 base URL，并保留场景 path、headers、queryVariants 和 bodyVariants，同时把 Target 的 `projectId` 和 `environment` 写入 Run 响应与历史记录。如果不传 `targetId`，Run 会继承 Scenario 的 `projectId` 和 `environment`。如果 Target 配置了 `healthCheck`，平台会先执行一次健康预检；预检失败时返回 `409 Conflict` 和 `{"error":"target health preflight failed: ..."}`，不会写入 Run 历史，也不会对场景 URL 产生压测流量。前端 Runs 页面使用的是 `scenarioId` + 可选 `targetId` 模式；顶栏应用 scope 后，Runs 页面只展示当前项目/环境内的场景，并通过 `/api/targets?projectId=...&environment=...` 读取当前 scope 的 Target。`GET /api/runs` 返回最近 100 条运行历史，按创建时间倒序排列；`GET /api/runs/{id}` 返回单个 Run 的最新进度；`GET /api/runs/{id}/events` 返回 `run_started`、`run_progress`、`run_finished`、`run_canceled`、`run_aborted` 等事件；`GET /api/runs/{id}/events/stream` 以 `text/event-stream` 持续推送同一批事件。

### 5.1 请求字段

| 字段 | 类型 | 必填 | 说明 |
|---|---:|---:|---|
| name | string | 否 | 压测名称，默认 `ad-hoc-http-run`。 |
| scenarioId | string | 否 | 已保存场景 ID。传入后，后端会读取场景中的 method、URL、headers、queryVariants、bodyVariants 和 timeout。 |
| targetId | string | 否 | 已保存 Target ID。与 `scenarioId` 一起使用时，后端使用 Target 的 `baseUrl` 加场景 path 执行压测，并在响应和历史记录里返回 `targetId`、`targetName`、`projectId`、`environment`。 |
| method | string | 否 | 当前仅支持 `GET` 和 `POST`，默认 `GET`。 |
| url | string | 直接 URL 模式必填 | 绝对 HTTP / HTTPS URL。传 `scenarioId` 时可省略。 |
| totalRequests | number | 是 | 总请求数，范围 `1..10000`。 |
| concurrency | number | 是 | 并发数，范围 `1..256`。 |
| timeoutMs | number | 否 | 单请求超时时间，默认 `3000`。 |
| headers | array | 否 | 直接 URL 模式可传请求头；场景模式会使用场景里保存的 Header KV。 |
| queryVariants | array | 否 | 查询参数候选列表。每项包含 `name`、`weight`、`queryParams`，执行时按正权重随机选择并追加到 URL；`weight=0` 的候选不会被选中。 |
| bodyVariants | array | 否 | POST 请求体候选列表。每项包含 `name`、`weight`、`body`，执行时按正权重随机选择一个 body；`weight=0` 的候选不会被选中。 |
| assertion | string | 否 | HTTP 状态码断言。直接 URL 模式可传；场景模式默认读取场景的 `Success assertion`。当前支持 `status <|<=|==|!=|>=|> N`。 |
| flowSteps.extractors | array | 否 | flow extract step 的变量提取器，当前支持 `source=json/header/regex`，提取后可用 `${name}` 引用。 |
| flowSteps.assertions | array | 否 | flow assertion step 或 request step 的响应断言。当前支持 `source=json/header/body` 和 `equals/not_equals/contains/not_contains/matches/not_matches/exists`。 |
| flowSteps.when | object | 否 | step-level 条件执行规则。支持 `source=json/header/body/regex/variable` 和 `equals/not_equals/contains/not_contains/matches/not_matches/exists`；条件不满足时跳过当前 step。 |

> 注意：当前 `/api/runs` 已支持直接 URL 压测，也支持按 `scenarioId` 读取保存场景，并可通过 `targetId` 选择实际被压测 Target。选择的 Target 如果配置了 `healthCheck`，平台会在启动前做健康预检，不健康时返回 `409` 并中止启动。场景里的 Headers、Query variants、Body variants、已启用 HTTP flow request steps、样本内 Cookie Jar、JSON/Header/Regex 变量提取、`${name}` 变量引用、场景级 status assertion、JSON/Header/Body response assertions 和 step-level `when` 条件会继续生效。选择的 Target 如果配置了 `profileEndpoint`，平台会采集一次 CPU pprof；如果 Target 绑定了 Agent，平台还会自动创建 `cpu` Profile Task。Run 报告会按压测时间窗口关联 Target 绑定 Agent 的 CPU/MEM/磁盘/网络指标摘要和趋势，并会展示 guardrail 告警摘要和 Target 健康检查历史。仍未支持循环编排、复杂分支图、完整 JSONPath、高级断言 DSL 和完整历史趋势报告。

### 5.2 PowerShell 示例

使用已保存场景发起压测：

```powershell
$run = @{
  scenarioId = "scenario-xxx"
  targetId = "target-xxx"
  totalRequests = 100
  concurrency = 10
  timeoutMs = 1000
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/runs" `
  -ContentType "application/json" `
  -Body $run

Invoke-RestMethod `
  -Method Get `
  -Uri "http://127.0.0.1:8080/api/runs/run-xxx/events"
```

直接 URL 发起临时压测：

```powershell
$body = @{
  name = "gateway-health-smoke"
  method = "GET"
  url = "http://127.0.0.1:8080/api/health"
  totalRequests = 100
  concurrency = 10
  timeoutMs = 1000
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/runs" `
  -ContentType "application/json" `
  -Body $body
```

### 5.3 curl 示例

```powershell
curl.exe -X POST http://127.0.0.1:8080/api/runs `
  -H "Content-Type: application/json" `
  --data-binary "{""name"":""gateway-health-smoke"",""method"":""GET"",""url"":""http://127.0.0.1:8080/api/health"",""totalRequests"":100,""concurrency"":10,""timeoutMs"":1000}"
```

### 5.4 响应示例

```json
{
  "id": "run-1780000000000000000",
  "scenarioId": "scenario-xxx",
  "scenarioName": "gateway-health-smoke",
  "targetId": "target-xxx",
  "targetName": "checkout-service",
  "name": "gateway-health-smoke",
  "status": "finished",
  "method": "GET",
  "url": "http://127.0.0.1:8080/api/health",
  "totalRequests": 100,
  "successRequests": 100,
  "failedRequests": 0,
  "durationMs": 53.12,
  "qps": 1882.39,
  "averageLatencyMs": 2.41,
  "p95LatencyMs": 5.62,
  "createdAt": "2026-06-12T08:30:00Z",
  "profileArtifacts": [
    {
      "id": "profile-1780000000000000001",
      "runId": "run-1780000000000000000",
      "targetId": "target-xxx",
      "targetName": "checkout-service",
      "profileType": "cpu",
      "status": "collected",
      "sourceUrl": "http://127.0.0.1:6060/debug/pprof/profile?seconds=1",
      "fileName": "run-1780000000000000000-cpu.pprof",
      "contentType": "application/octet-stream",
      "sizeBytes": 1048576,
      "startedAt": "2026-06-12T08:30:00Z",
      "finishedAt": "2026-06-12T08:30:01Z"
    }
  ]
}
```

### 5.5 查询运行历史

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/runs
```

返回内容是 Run 汇总结果数组，字段与 `POST /api/runs` 的响应一致。前端 Runs 页刷新后会调用这个接口恢复历史记录。

### 5.6 查询 Profile Artifacts

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/profile-artifacts
```

当前返回最近 100 条 profile artifact 元数据，按采集开始时间倒序排列。文件默认保存在 SQLite 数据库同目录的 `profile_artifacts` 子目录，例如：

```text
backend/data/profile_artifacts/run-1780000000000000000-cpu.pprof
```

下载单个 artifact 原始文件：

```powershell
curl -OJ http://127.0.0.1:8080/api/profile-artifacts/profile-1780000000000000000/download
```

Agent 上传 profile artifact：

```powershell
curl -X POST http://127.0.0.1:8080/agent/v1/profile-artifacts `
  -H "Authorization: Bearer <agent-token>" `
  -F "runId=run-1780000000000000000" `
  -F "scenarioName=checkout-smoke" `
  -F "targetName=checkout-01" `
  -F "profileType=cpu" `
  -F "sourceUrl=agent://checkout-01/debug/pprof/profile" `
  -F "file=@cpu.pprof;type=application/octet-stream"
```

Agent 命令行一次性采集并上传：

```powershell
cd D:\MyGitProject\all_in_one_testing\backend
go run .\cmd\agent `
  --control-plane http://127.0.0.1:8080 `
  --token <agent-token> `
  --pprof-base-url "http://127.0.0.1:6060/debug/pprof" `
  --run-id run-1780000000000000000 `
  --target-name checkout-01 `
  --profile-type cpu `
  --profile-seconds 1 `
  --file-name checkout-cpu.pprof
```

`--profile-type` 支持 `cpu`、`heap`、`goroutine`、`mutex`、`block`、`allocs`、`threadcreate`。如果要接入非 Go pprof 或自定义地址，也可以直接传 `--profile-url`。

Control Plane 创建 Profile Task：

```powershell
curl -X POST http://127.0.0.1:8080/api/profile-tasks `
  -H "Content-Type: application/json" `
  -d '{
    "agentId": "agent-checkout-01",
    "runId": "run-1780000000000000000",
    "targetName": "checkout-01",
    "pprofBaseUrl": "http://127.0.0.1:6060/debug/pprof",
    "profileType": "heap",
    "profileSeconds": 1,
    "maxAttempts": 3
  }'
```

创建受控命令 profiler Profile Task：

```powershell
curl -X POST http://127.0.0.1:8080/api/profile-tasks `
  -H "Content-Type: application/json" `
  -d '{
    "agentId": "agent-checkout-01",
    "runId": "run-1780000000000000000",
    "targetName": "checkout-01",
    "profileType": "perf",
    "profileSeconds": 30,
    "profileCommand": "perf",
    "profileCommandArgs": ["record", "-F", "99", "-p", "1234", "-g", "-o", "{{output}}", "--", "sleep", "{{seconds}}"],
    "profileCommandOutput": "/tmp/checkout-perf.data",
    "profileCommandTimeoutMs": 45000,
    "maxAttempts": 3
  }'
```

`profileSeconds` 必须在 `1..300` 范围内，超出范围的 API 请求会返回 `400`。Profile Task 必须提供 `pprofBaseUrl`、`profileUrl` 或 `profileCommand` 之一；命令任务的 `profileCommandArgs` 是数组，不经过 shell，支持 `{{output}}` 和 `{{seconds}}` 占位符。`source` 表示任务来源，支持 `api`、`run_auto`、`target_manual`、`threshold_auto`；直接 API 创建默认是 `api`，Run 创建时基线任务是 `run_auto`，Targets 页面手工下发是 `target_manual`，Target 指标超过阈值后自动下发是 `threshold_auto`。`maxAttempts` 默认为 3，最大为 10。Agent 回写 `error` 时平台会把 `attempts` 加 1；未达到 `maxAttempts` 时任务会回到 `pending` 等待下一轮领取，达到上限后才进入 `failed`。`GET /api/profile-tasks` 支持 `runId`、`agentId`、`status`、`source` 和 `limit` 查询参数，例如 `/api/profile-tasks?runId=run-xxx&source=run_auto&status=pending&limit=20`；`leased` 任务响应会返回派生字段 `leaseExpiresAt`，表示当前租约到期时间。`POST /api/profile-tasks/{id}/retry` 可把已 `failed` 的任务重新置为 `pending`，清空最近错误、artifact、租约和完成时间，并把 `attempts` 重置为 0。Targets 页面会读取 `/api/profile-task-templates` 的 SQLite 白名单模板，在 Target 行选择模板、profile 类型或 PID 和 `1..300` 秒采集时长后点击 `采集 / Profile`，快速创建 Profile Task。Reports 页面会按最新 Run 过滤展示 `source`、`attempts/maxAttempts`、最近错误、租约到期时间和当前状态，并可对失败任务点击 Retry 重新入队，也可以按来源筛选任务队列。

Agent 领取并处理一批任务：

```powershell
cd D:\MyGitProject\all_in_one_testing\backend
go run .\cmd\agent `
  --control-plane http://127.0.0.1:8080 `
  --token <agent-token> `
  --agent-id agent-checkout-01 `
  --poll-once
```

Agent 常驻模式：

```powershell
cd D:\MyGitProject\all_in_one_testing\backend
go run .\cmd\agent `
  --daemon `
  --control-plane http://127.0.0.1:8080 `
  --token <agent-token> `
  --agent-id agent-checkout-01 `
  --name checkout-01 `
  --hostname checkout-host-01 `
  --labels service=checkout,zone=shanghai-a `
  --heartbeat-interval 10s `
  --metrics-interval 5s `
  --profile-task-interval 5s
```

`--daemon` 会启动后立即上报一次心跳、指标并领取一次 Profile Task，之后按间隔持续执行。单次上报或任务拉取失败不会退出，Agent 会等待下一轮重试。

当前原型支持 Control Plane 在压测期间直接从 Target 的 Go pprof endpoint 拉取 1 秒 CPU profile，也支持 Run 创建时为 Target 绑定的 Agent 自动下发 `cpu` Profile Task；当 Target 配置了 `metricThresholds` 且压测窗口内 Agent 指标超过阈值时，也支持 Run 结束后自动下发 `threshold_auto` 的 `cpu` Profile Task。Agent 可以通过 API 上传已采集的 pprof / prof 文件，支持 `cmd/agent` 一次性按类型采集上传、受控命令 profiler、`cmd/agent --poll-once` 领取并处理一批 pprof/命令 Profile Task，以及 `cmd/agent --daemon` 常驻轮询任务。Targets 页面已支持从 `/api/profile-task-templates` 读取 SQLite 白名单模板并下发 `Go pprof` 和 `Linux perf`，也支持基础模板创建、更新和删除；Reports 页面会展示命令任务摘要。Profile Task 已支持租约超时后重新领取、`attempts/maxAttempts` 失败重试，以及已失败任务的手动 Retry 重新入队；`cmd/package-agent` 已支持生成 Linux amd64 Agent 二进制和 `.sha256`；`/agent/install.sh` 已支持 systemd bootstrap 安装，默认可从 `/agent/binaries/all-in-one-agent-linux-amd64` 下载预构建二进制，并在目标机器存在 `sha256sum` 时使用 `.sha256` checksum 校验下载内容，deb/rpm 等系统发行包和更高级阈值动作仍是后续能力。

## 6. Agent 安装和接入

本节描述当前可用方式和目标使用方式。当前仓库已经实现 Agent token 创建/过期/轮换/吊销、`projectId` / `environment` token 归属、`cmd/agent --config` JSON 配置文件、`cmd/package-agent` Linux amd64 产物生成、`/agent/install.sh` systemd bootstrap 安装脚本、`/agent/binaries/manifest.json` 发行产物清单、`/agent/binaries/all-in-one-agent-linux-amd64` 预构建二进制下载、`/agent/binaries/all-in-one-agent-linux-amd64.sha256` checksum 下载或动态生成、心跳、指标上报、列表查询 API 和 `cmd/agent` CLI；可以在被压测机器上通过源码运行、`go build` 或 `cmd/package-agent` 生成最小 Agent 二进制。当前还没有提供 deb/rpm 等系统发行包。

### 6.1 Agent 部署位置

Agent 应部署在被压测机器上，而不是部署在发压机器上。原因是 Agent 需要读取本机：

- `/proc` 进程信息。
- CPU、内存、磁盘、网络指标。
- 本机服务端口。
- 只监听在 `127.0.0.1` 的 pprof endpoint。
- 本机 profiler 工具输出。

### 6.2 安装前准备

被压测机器需要满足：

- Linux x86_64 或 arm64。
- systemd，推荐用于托管 Agent 服务。
- 能访问平台后端地址，例如 `http://control-plane:8080`。
- 允许 Agent 出站访问平台 API。
- 如需采集 pprof，目标进程需要暴露 pprof endpoint。
- 如需执行 perf / 自定义 prof，机器需要安装对应工具并配置最小权限。

如果使用 `/agent/install.sh` 自动下载二进制，需要先在 Control Plane 所在机器准备 Linux amd64 Agent 产物。默认目录是后端工作目录下的 `dist/agents`，也可以通过 `AGENT_BINARY_DIR` 覆盖：

```bash
cd backend
go run ./cmd/package-agent --version v0.4.0 --commit local --date 2026-06-23T00:00:00Z
```

在 Windows PowerShell 上构建 Linux amd64 产物：

```powershell
cd backend
go run .\cmd\package-agent --version v0.4.0 --commit local --date 2026-06-23T00:00:00Z
```

打包命令会在输出目录生成 `all-in-one-agent-linux-amd64`、`all-in-one-agent-linux-amd64.sha256` 和 `manifest.json`。Control Plane 的 `/agent/binaries/manifest.json` 会优先读取这个 manifest；如果只有二进制文件没有 manifest，则接口会动态计算 checksum 和文件大小，但不会包含版本信息。`GET /api/agents` 会使用 manifest 里的最新版本给每个 Agent 返回 `latestVersion` 和 `upgradeAvailable`，Targets & Agents 页会在 Agent inventory 的 version 单元格提示 `升级可用 / Upgrade available`。因此生产发布新 Agent 时，建议始终通过 `cmd/package-agent --version ... --commit ... --date ...` 生成完整产物。

目标机器升级 Agent 时，可以重新运行安装脚本并追加 `--force-download` 或 `--upgrade`：

```bash
curl -fsSL http://control-plane:8080/agent/install.sh | sudo bash -s -- \
  --server http://control-plane:8080 \
  --token <AGENT_REGISTER_TOKEN> \
  --agent-id agent-checkout-01 \
  --force-download
```

升级模式会把新二进制下载到临时文件，使用 `.sha256` 校验通过后再替换 `/usr/local/bin/all-in-one-agent`，最后重启 `all-in-one-agent.service`。如果没有追加 `--force-download` / `--upgrade`，本地已有可执行二进制时脚本只会刷新配置和 systemd service。

### 6.3 在平台生成 Agent Token

当前可用流程：

1. 打开 `Targets & Agents`。
2. 填写 Agent token 的 Project ID 和 Environment，例如 `project-checkout` / `staging`。
3. 点击 `Generate token`。
4. 平台调用 `/api/agent-tokens` 生成一个只在创建响应中展示的 token。
5. 复制页面中的 `--server`、`--token` 和 `Authorization: Bearer <token>` 参数，用于 Agent 配置或手工调试。

当前后端只保存 token hash，不保存明文 token。页面生成 token 时默认申请 24 小时有效期，并展示 `Expires` 时间；API 也支持传入 `expiresAt` 或 `expiresInSeconds`。Agent 使用该 token 上报 heartbeat 后，会继承 token 的 `projectId` 和 `environment`，并可通过 `/api/agents?projectId=...&environment=...` 过滤查询。页面可以点击 `Rotate` 调用 `/api/agent-tokens/{id}/rotate` 生成新密钥，旧密钥会立即失效；也可以点击 `Revoke` 调用 `/api/agent-tokens/{id}/revoke` 吊销 token。旧密钥、过期 token 或吊销后的 token 不能再用于 heartbeat、metrics、profile artifact 上传或 Profile Task 轮询。

API 示例：

```powershell
$workspaceHeaders = @{
  "X-AIT-Project-ID" = "project-checkout"
  "X-AIT-Environment" = "staging"
}

$tokenResponse = Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/api/agent-tokens" `
  -Headers $workspaceHeaders `
  -ContentType "application/json" `
  -Body '{"name":"checkout-install","projectId":"project-checkout","environment":"staging"}'

$agentToken = $tokenResponse.token

$rotatedTokenResponse = Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/api/agent-tokens/$($tokenResponse.id)/rotate" `
  -ContentType "application/json" `
  -Body '{"expiresInSeconds":86400}'

$agentToken = $rotatedTokenResponse.token

Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/api/agent-tokens/$($tokenResponse.id)/revoke"
```

### 6.4 一键安装命令

当前可用命令形态。后端会通过 `/agent/install.sh` 返回 bootstrap 脚本，该脚本接收 Control Plane 地址、Agent token 和可选 Agent 元数据参数：

```bash
curl -fsSL http://control-plane:8080/agent/install.sh | sudo bash -s -- \
  --server http://control-plane:8080 \
  --token <AGENT_REGISTER_TOKEN> \
  --name checkout-01 \
  --labels env=prod,service=checkout,zone=shanghai-a
```

安装脚本会完成：

- 从 `--binary-url` 或 `${server}/agent/binaries/all-in-one-agent-linux-amd64` 下载 `all-in-one-agent` 二进制。
- 写入 `/etc/all-in-one-agent/agent.json`。
- 如果目标机器存在 `sha256sum`，下载 `.sha256` checksum 并校验二进制。
- 注册 systemd service。
- 启动 Agent。
- Agent daemon 启动后按配置上报 heartbeat、metrics 并轮询 Profile Task。

### 6.5 手动安装方式

目标目录结构：

```text
/usr/local/bin/all-in-one-agent
/etc/all-in-one-agent/agent.yaml
/var/lib/all-in-one-agent/artifacts/
/var/log/all-in-one-agent/
/etc/systemd/system/all-in-one-agent.service
```

当前源码构建方式：

```bash
cd /opt/all_in_one_testing/backend
go build -o /usr/local/bin/all-in-one-agent ./cmd/agent
```

后续正式安装包会支持独立配置文件。目标配置形态如下，当前 CLI 暂不支持直接读取该 YAML：

```yaml
server_url: "http://control-plane:8080"
token: "<AGENT_REGISTER_TOKEN>"
agent_name: "checkout-01"
labels:
  env: "prod"
  service: "checkout"
  zone: "shanghai-a"

heartbeat_interval: "10s"
metric_interval: "5s"

targets:
  - name: "checkout-service"
    process_match:
      name: "checkout"
      cmdline_contains: "--config=/etc/checkout/config.yaml"
    health_checks:
      - type: "tcp"
        address: "127.0.0.1:8080"
      - type: "http"
        url: "http://127.0.0.1:8080/health"
        timeout: "2s"
    profiling:
      enabled: true
      type: "go_pprof"
      endpoint: "http://127.0.0.1:6060/debug/pprof"
      profiles:
        - cpu
        - heap
        - goroutine
```

systemd 示例：

```ini
[Unit]
Description=All-in-One Testing Target Agent
After=network-online.target
Wants=network-online.target

[Service]
User=alltesting-agent
Group=alltesting-agent
ExecStart=/usr/local/bin/all-in-one-agent --daemon --config /etc/all-in-one-agent/agent.json
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now all-in-one-agent
sudo systemctl status all-in-one-agent
```

查看日志：

```bash
journalctl -u all-in-one-agent -f
```

### 6.6 验证 Agent 是否接入成功

当前验证方式：

```bash
curl http://control-plane:8080/api/agents
```

预期能看到：

```json
[
  {
    "name": "checkout-01",
    "tokenId": "agent-token-1780000000000000000",
    "status": "online",
    "version": "0.1.0",
    "lastSeenAt": "2026-06-11T15:00:00+08:00",
    "labels": {
      "env": "prod",
      "service": "checkout"
    }
  }
]
```

前端 `Targets & Agents` 页面也应显示 Agent 在线。

## 7. Agent 数据采集

### 7.1 心跳数据

Agent 按固定间隔上报心跳。

建议间隔：

```text
10s
```

心跳包含：

- agent id。
- hostname。
- IP。
- Agent version。
- labels。
- 当前时间。
- 运行状态。
- 可用采集能力。

目标接口：

```text
POST /api/agent-tokens
POST /agent/v1/heartbeat
GET  /api/agents
```

当前原型已经实现 token 鉴权和最小心跳落库。先创建 token：

```powershell
$tokenResponse = Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/api/agent-tokens" `
  -ContentType "application/json" `
  -Body '{"name":"checkout-install","projectId":"project-checkout","environment":"staging"}'

$agentToken = $tokenResponse.token
```

再上报心跳：

```powershell
$heartbeat = @{
  name = "checkout-01"
  hostname = "checkout-host-01"
  ip = "10.0.0.13"
  version = "0.2.0"
  labels = @{
    service = "checkout"
    zone = "shanghai-b"
  }
  capabilities = @("host_metrics", "process_metrics", "pprof")
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/agent/v1/heartbeat" `
  -Headers @{ Authorization = "Bearer $agentToken" } `
  -ContentType "application/json" `
  -Body $heartbeat
```

Agent 心跳会绑定 tokenId，并继承 token 的 `projectId` / `environment`。查询 Agent 时可以按项目和环境过滤：

```powershell
Invoke-RestMethod "http://control-plane:8080/api/agents?projectId=project-checkout&environment=staging"
```

当前限制：心跳接口负责登记 Agent 基础身份、tokenId、项目/环境、标签、能力和 `lastSeenAt`；Target 绑定仍通过 `/api/targets` 的 `agentIds` 字段维护。

### 7.2 主机指标

建议间隔：

```text
5s
```

采集内容：

| 指标 | 说明 |
|---|---|
| CPU | 总 CPU 使用率、每核使用率、load average。 |
| Memory | 总内存、已用内存、可用内存、swap。 |
| Disk IO | read/write bytes、read/write ops、io wait。 |
| Network | inbound/outbound bytes、packets、errors。 |
| Filesystem | 磁盘容量、使用率、inode。 |

目标接口：

```text
POST /agent/v1/metrics
```

当前原型已经实现最小指标落库和 Agent daemon 采样。Linux 上 daemon 会读取 `/proc/stat`、`/proc/meminfo`、`/proc/diskstats`、`/proc/net/dev` 和 `/proc/<pid>`，上报主机 CPU 使用率、内存使用率、磁盘读写速率、网络收发速率，以及最多 20 个进程的 CPU、RSS、fd 数、线程数和 cmdline；非 Linux 环境暂时降级为 Agent 自身进程快照。请求示例，继续使用上一步生成的 `$agentToken`：

```powershell
$metrics = @{
  agentId = "agent-checkout-01"
  cpuUsagePercent = 72.5
  memoryUsagePercent = 61.25
  diskReadBytesPerSec = 1048576
  diskWriteBytesPerSec = 2097152
  networkRxBytesPerSec = 32768
  networkTxBytesPerSec = 65536
  processes = @(
    @{
      pid = 1234
      name = "checkout"
      cpuUsagePercent = 34.5
      memoryRssBytes = 268435456
      fdCount = 88
      threadCount = 12
    }
  )
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
  -Method Post `
  -Uri "http://control-plane:8080/agent/v1/metrics" `
  -Headers @{ Authorization = "Bearer $agentToken" } `
  -ContentType "application/json" `
  -Body $metrics
```

`GET /api/agents` 会把最新一条指标放在每个 Agent 的 `latestMetrics` 字段中。也可以查询时间线：

```powershell
Invoke-RestMethod "http://control-plane:8080/api/agent-metrics?agentId=agent-checkout-01&limit=120"
```

支持参数：`agentId` 必填，`from`、`to` 可选，使用 RFC3339 时间字符串；`limit` 默认 500，最大 1000。当前已有按 Agent 的时间线查询、Reports 页 Agent CPU/MEM 趋势展示，以及 Run 报告中的 Target 指标窗口聚合、CPU/MEM/磁盘/网络窗口趋势图、guardrail 告警摘要、基于 Target `metricThresholds` 的 `target_metric` 告警、按 PID 聚合的窗口内 `processTrends` 进程趋势和按 `processMatch.cmdlineContains` / `processMatch.name` 过滤的最新进程快照；跨 Run 进程趋势对比由 `/api/reports/process-trends/compare` 提供，当前 daemon 会上报 CPU 最高的进程快照。

### 7.3 进程指标

Agent 通过进程匹配规则定位目标进程。

匹配方式：

- 进程名。
- cmdline 包含关键字。
- pid file。
- 监听端口。
- systemd service name。

采集内容：

| 指标 | 说明 |
|---|---|
| pid | 当前进程 ID。 |
| cpu | 进程 CPU 使用率。 |
| rss | 常驻内存。 |
| vms | 虚拟内存。 |
| fd_count | 打开的文件描述符数。 |
| thread_count | 线程数。 |
| start_time | 进程启动时间。 |
| restart_count | 进程重启次数，后续可由 Agent 维护。 |

### 7.4 健康检查

当前可用：Target 可以保存 HTTP health check 配置，平台通过 `/api/targets/{id}/health-check` 发起一次性探测。这个能力适合在压测前确认目标服务已经启动、路由可达、状态码符合预期。它不依赖被压测服务上报 Prometheus，也不要求 Agent 入站开放端口。健康检查结果会持久化到 Target 的 `lastHealthCheck` 字段和 `target_health_checks` 历史表，`GET /api/targets` 和 `GET /api/targets/{id}` 都会返回最近一次检查状态，`GET /api/targets/{id}/health-checks?limit=5` 会返回最近历史，Targets 页面会直接展示这些状态。`POST /api/runs` 携带 `targetId` 且 Target 配置了 `healthCheck` 时，会自动执行同样的启动前预检并更新 `lastHealthCheck` 与历史记录；如果状态不符合预期，接口返回 `409 Conflict`，Runs 页面展示后端返回的原因，并且不会创建 Run 或产生压测流量。

配置位置：

- 前端：`Targets & Agents` -> `Target binding` -> `Health check path`、`Expected health status`、`Health timeout ms`。
- API：Target 的 `healthCheck` 字段。

当前 Agent 常驻模式会周期性执行绑定 Target 的 HTTP health check，并把探测结果写入 Target `lastHealthCheck` 和 `target_health_checks` 历史表；后续仍可扩展 TCP / process health check、实时告警、压测中自动停止和报告归因。

支持三类检查：

```yaml
health_checks:
  - type: tcp
    address: "127.0.0.1:8080"
  - type: http
    url: "http://127.0.0.1:8080/health"
    timeout: "2s"
  - type: process
    name: "checkout"
```

健康检查结果用于：

- 判断 Target 是否可压测。
- 在压测中触发告警或自动停止。
- 在报告中解释错误率和延迟异常。

## 8. pprof / prof 采集

### 8.1 Go 服务开启 pprof

Go 服务中引入：

```go
import _ "net/http/pprof"
```

启动 pprof 监听：

```go
go func() {
    _ = http.ListenAndServe("127.0.0.1:6060", nil)
}()
```

建议只监听 `127.0.0.1`，由本机 Agent 拉取，不要直接暴露到公网或办公网。

### 8.2 Agent 拉取 pprof

配置示例：

```yaml
profiling:
  enabled: true
  type: "go_pprof"
  endpoint: "http://127.0.0.1:6060/debug/pprof"
  profiles:
    - cpu
    - heap
    - goroutine
    - mutex
    - block
  trigger:
    mode: "during_run"
    interval: "60s"
    duration: "30s"
```

Agent 需要支持：

- `/debug/pprof/profile?seconds=30`
- `/debug/pprof/heap`
- `/debug/pprof/goroutine?debug=2`
- `/debug/pprof/mutex`
- `/debug/pprof/block`

### 8.3 受控命令 profiler

当服务没有 HTTP pprof，或者需要使用 `perf` 等系统工具时，可以使用受控命令 profiler。当前已实现 Agent CLI 和 Profile Task 两种入口：命令由 Agent 启动参数或 Control Plane 任务字段显式给出，执行时不经过 shell，不拼接命令字符串，每个参数都作为独立参数传入。

Linux 上用 `perf` 采集指定 PID 的示例：

```bash
go run ./cmd/agent \
  --control-plane http://control-plane:8080 \
  --token "$AGENT_TOKEN" \
  --run-id "$RUN_ID" \
  --target-name checkout-01 \
  --profile-type perf \
  --profile-seconds 30 \
  --file-name checkout-perf.data \
  --profile-command perf \
  --profile-command-output /tmp/checkout-perf.data \
  --profile-command-timeout 45s \
  --profile-command-arg record \
  --profile-command-arg -F \
  --profile-command-arg 99 \
  --profile-command-arg -p \
  --profile-command-arg 1234 \
  --profile-command-arg -g \
  --profile-command-arg -o \
  --profile-command-arg "{{output}}" \
  --profile-command-arg -- \
  --profile-command-arg sleep \
  --profile-command-arg "{{seconds}}"
```

占位符：

- `{{output}}` 会替换为 `--profile-command-output` 指定的本地文件路径；如果不指定，Agent 会创建临时文件。
- `{{seconds}}` 会替换为 `--profile-seconds`。

当前边界：

- CLI 会把命令生成的本地文件按 `application/octet-stream` 上传到 `/agent/v1/profile-artifacts`。
- Profile Task 可以携带 `profileCommand`、`profileCommandArgs`、`profileCommandOutput` 和 `profileCommandTimeoutMs`，Agent `--poll-once` 或 `--daemon` 领取后会执行同样的受控命令采集路径。
- 单个 artifact 最大 64 MiB。
- `sourceUrl` 默认记录为 `command://<profile-command>`。
- Targets 页面目前从 `/api/profile-task-templates` 读取 SQLite 白名单模板，默认种子数据包含 `Linux perf`，也可在页面创建、更新和删除自定义模板；生产环境如需更强隔离，还应继续收敛到 Agent 安装配置、项目权限和平台模板审批流程。

安全要求：

- 生产环境应把命令收敛到 Agent 安装配置或平台白名单模板。
- 用户不能在页面上输入任意 shell 命令。
- 需要限制采样时长。
- 需要限制 artifact 文件大小。
- 需要记录执行命令、开始时间、结束时间、退出码；当前 artifact 元数据已保存 profile 类型、来源、文件名和大小，命令审计日志仍需继续补齐。

### 8.4 Artifact 保存与上传

当前已实现的原型能力：当 `/api/runs` 请求携带 `targetId`，且 Target 配置了 `profileEndpoint`，Control Plane 会在压测期间请求：

```text
{profileEndpoint}/profile?seconds=1
```

采集到的 CPU pprof 文件会写入本地 `profile_artifacts` 目录，元数据会写入 SQLite `profile_artifacts` 表，并可通过以下接口查询：

如果这个 Target 同时绑定了 Agent，Run 创建成功后还会自动为每个绑定 Agent 创建一个 `cpu` Profile Task，字段会带上 `runId`、`scenarioId`、`targetId`、`targetName` 和 `pprofBaseUrl`。如果 Target 配置了 `metricThresholds` 且 Run 时间窗口内 Agent 指标超过阈值，Run 结束后会再创建 `threshold_auto` 来源的 `cpu` Profile Task。Agent daemon 或 `cmd/agent --poll-once` 领取后会采集并上传，Reports 页面会通过 `/api/profile-tasks?runId=<latest-run-id>&limit=20` 展示当前 Run 的任务状态、`attempts/maxAttempts` 和最近错误。

```text
GET /api/profile-artifacts
GET /api/profile-artifacts/{id}/download
GET /api/reports/profile-artifacts/compare?limit=5
GET /api/profile-tasks?runId=<run-id>&agentId=<agent-id>&status=pending&limit=20
POST /api/profile-tasks
POST /api/profile-tasks/{id}/retry
GET /agent/v1/profile-tasks?agentId=<agent-id>
POST /agent/v1/profile-tasks/{id}/complete
```

`/api/reports/profile-artifacts/compare?limit=5` 会只对最近 `limit` 次 Run 内的 artifact 做分组对比。返回结果按 `profileType` 分组，包含 artifact 总数、成功/失败数量、总大小、最新 artifact、上一次 artifact、大小 delta、百分比变化和状态变化标记；Reports 页面会把这些字段展示为 `delta`、`change` 和 `status previous -> latest`，用于快速发现同类 pprof 产物突然变大或采集从成功变为失败。

### GET `/api/reports/process-trends/compare`

```text
GET /api/reports/process-trends/compare?limit=5
GET /api/reports/process-trends/compare?runIds=run-a,run-b
```

`/api/reports/process-trends/compare?limit=5` 会对最近 `limit` 次 Run 的 Target 指标窗口重新聚合进程趋势。返回结果按 Target + Agent + 进程名 + cmdline 分组，不把 PID 放进分组 key，因此服务重启导致 PID 变化时仍能把同一个进程串起来。每个 group 包含 `runs` 明细、`latestRunId`、`latestPid`、`previousRunId`、`previousPid`、`cpuDeltaPercent`、`memoryRssDeltaBytes`、`cpuMaxPercent`、`memoryRssMaxBytes`、`fdMaxCount` 和 `threadMaxCount`；Reports 页面会展示最新/上一次 PID、CPU delta、RSS delta 和 samples。

当前可用：Agent 采集完成后可以用 token 认证上传 artifact。

接口：

```text
POST /agent/v1/profile-artifacts
```

请求格式为 `multipart/form-data`：
- `file`：profile artifact 原始文件，必填。
- `runId`：关联 run id，必填。
- `profileType`：例如 `cpu`、`heap`、`goroutine`。
- `scenarioId`、`scenarioName`、`targetId`、`targetName`、`sourceUrl`、`startedAt`、`finishedAt`：可选元数据。

也可以使用当前仓库内的最小 Agent 命令：

```bash
go run ./cmd/agent \
  --control-plane http://control-plane:8080 \
  --token "$AGENT_TOKEN" \
  --pprof-base-url "http://127.0.0.1:6060/debug/pprof" \
  --run-id "$RUN_ID" \
  --target-name checkout-01 \
  --profile-type cpu \
  --profile-seconds 1 \
  --file-name checkout-cpu.pprof
```

也可以使用受控命令 profiler，把本机命令生成的文件上传为 artifact：

```bash
go run ./cmd/agent \
  --control-plane http://control-plane:8080 \
  --token "$AGENT_TOKEN" \
  --run-id "$RUN_ID" \
  --target-name checkout-01 \
  --profile-type perf \
  --profile-seconds 30 \
  --file-name checkout-perf.data \
  --profile-command perf \
  --profile-command-output /tmp/checkout-perf.data \
  --profile-command-timeout 45s \
  --profile-command-arg record \
  --profile-command-arg -F \
  --profile-command-arg 99 \
  --profile-command-arg -p \
  --profile-command-arg 1234 \
  --profile-command-arg -g \
  --profile-command-arg -o \
  --profile-command-arg "{{output}}" \
  --profile-command-arg -- \
  --profile-command-arg sleep \
  --profile-command-arg "{{seconds}}"
```

使用 Profile Task 队列时，Agent 可以单次领取一批任务：

```bash
go run ./cmd/agent \
  --control-plane http://control-plane:8080 \
  --token "$AGENT_TOKEN" \
  --agent-id "$AGENT_ID" \
  --poll-once
```

`--poll-once` 会领取当前 Agent 的 pending Profile Task。pprof 任务会把 `pprofBaseUrl/profileType/profileSeconds/runId` 转成一次采集上传；命令任务会执行 `profileCommand/profileCommandArgs`，把 `profileCommandOutput` 指向的文件上传。上传成功后 Agent 会把 artifact id 写回任务完成状态。Control Plane 会把领取到的任务标记为 `leased`；如果 Agent 领取后没有完成，超过租约超时时间后，后续轮询可以重新领取同一个任务。任务达到 `maxAttempts` 进入 `failed` 后，可以在 Reports 页面点击 Retry，或调用 `POST /api/profile-tasks/{id}/retry` 手动重新入队。

常驻运行时使用：

```bash
go run ./cmd/agent \
  --daemon \
  --control-plane http://control-plane:8080 \
  --token "$AGENT_TOKEN" \
  --agent-id "$AGENT_ID" \
  --name checkout-01 \
  --hostname "$(hostname)" \
  --labels service=checkout,env=prod \
  --heartbeat-interval 10s \
  --metrics-interval 5s \
  --profile-task-interval 5s
```

常驻模式会周期性上报心跳和指标，并轮询 Profile Task。某一轮 Control Plane 请求失败时不会退出，下一轮会继续重试。

Profile Task 租约超时默认是 2 分钟，可以通过后端环境变量调整：

```powershell
$env:PROFILE_TASK_LEASE_TIMEOUT = "30s"
# 或
$env:PROFILE_TASK_LEASE_TIMEOUT_MS = "30000"
```

artifact 元数据：

```json
{
  "runId": "run-xxx",
  "targetId": "target-xxx",
  "agentId": "agent-xxx",
  "profileType": "cpu",
  "triggerType": "during_run",
  "startedAt": "2026-06-11T15:00:00+08:00",
  "finishedAt": "2026-06-11T15:00:30+08:00",
  "fileName": "cpu.pprof",
  "sizeBytes": 1048576
}
```

报告页应支持下载：

```text
GET /api/projects/{projectId}/profiles/{artifactId}/download
```

## 9. 压测和采集的完整目标流程

1. 启动 Control Plane。
2. 打开 Web Console。
3. 创建项目和环境。
4. 在 `Targets & Agents` 创建 Target。
5. 生成 Agent token。
6. 在被压测机器安装 Agent。
7. Agent 上报心跳和指标。
8. 给 Target 绑定 Agent。
9. 配置进程匹配、host metric 阈值、health check 和 profiling。
10. 在 `Scenarios` 创建 HTTP / RPC 场景。
11. 在 `Runs` 选择场景、目标、负载模型和采集策略。
12. 启动压测。
13. 压测过程中观察：
    - QPS。
    - 延迟。
    - 错误率。
    - 目标机器 CPU / 内存 / IO / 网络。
    - 目标进程 CPU / 内存 / fd / thread。
    - pprof / prof 采集状态。
14. 压测结束后进入 `Reports`。
15. 下载 profile artifact，结合时间线定位瓶颈。

## 10. 数据流

```text
Web Console
  -> 创建 Scenario
  -> 创建 Run
  -> 查看实时指标和报告

Control Plane API
  -> 保存场景和任务
  -> 调度压测
  -> 接收 Agent 心跳、指标和 artifact
  -> 聚合报告

Executor
  -> 执行 HTTP / RPC 请求
  -> 生成 QPS、延迟、错误率、样本

Target Agent
  -> 上报 heartbeat
  -> 上报 host metrics
  -> 上报 process metrics
  -> 拉取 pprof / 执行受控 profiler
  -> 上传 profile artifact

Report
  -> 对齐压测指标、机器指标、进程指标和 profile artifact
```

## 11. 安全建议

- Agent token 已支持按 `projectId` / `environment` 归属，生产环境还需要叠加权限校验和审计。
- token 支持过期、吊销和轮换。
- 生产环境建议使用 HTTPS。
- pprof endpoint 建议只监听 `127.0.0.1`。
- Agent 只需要出站访问 Control Plane，避免开放入站端口。
- command profiler 必须使用白名单模板。
- profile artifact 可能包含敏感函数名、路径、参数和内存信息，需要按敏感文件管理。
- artifact 应配置保留周期和自动清理。
- 采集频率不宜过高，避免 Agent 自身影响被压测服务。

## 12. 常见问题

### 前端创建的场景刷新后还在吗

当前前端会优先调用后端 `/api/scenarios` 保存场景，后端默认写入 SQLite 数据库 `backend/data/platform.db`。同一个后端和同一个数据库文件下，刷新页面、重启前端或换浏览器都能恢复。清理数据库文件或换一套后端环境后不会自动同步。

### `/api/runs` 如何使用前端里配置的 Headers

在 Runs 页面选择已保存场景发起压测时，后端会通过 `scenarioId` 读取场景快照，并使用场景里的 Header KV、Query variants、Body variants、已启用 HTTP flow request steps、样本内 Cookie Jar、JSON/Header/Regex 变量提取、status assertion、JSON/Header/Body response assertions 和 step-level `when` 条件。如果同时选择 Target，前端会把 `targetId` 发给 `/api/runs`，后端使用 Target 的 `baseUrl` 加场景 path 执行请求。直接 URL 模式也支持传 `headers` 和 `assertion` 字段。当前还不支持循环编排、复杂分支图、完整 JSONPath 和高级断言 DSL。

### Agent 页面显示在线，但没有进程指标

目标排查顺序：

1. 检查进程匹配规则是否能匹配到 PID。
2. 检查 Agent 运行用户是否有读取 `/proc/<pid>` 的权限。
3. 检查目标进程是否频繁重启。
4. 查看 Agent 日志。

### pprof 采集失败

目标排查顺序：

1. 在被压测机器本机执行：

   ```bash
   curl http://127.0.0.1:6060/debug/pprof/
   ```

2. 确认服务已引入 `net/http/pprof`。
3. 确认 pprof 监听地址和 Agent 配置一致。
4. 确认 CPU profile duration 没有超过平台限制。
5. 查看 Agent 日志中的 HTTP 状态码和错误信息。

### 压测时机器指标和请求指标对不上

检查：

- Agent 和 Control Plane 时间是否同步。
- 指标上报间隔是否过大。
- Run 开始和结束时间是否正确。
- Target 是否绑定到了正确 Agent。
- 报告是否使用 run snapshot 对齐数据。

## 13. 当前开发优先级建议

为了让这份使用手册真正落地，建议后续按以下顺序实现：

1. Agent deb/rpm 等系统发行包、正式配置文件和权限模型。
2. 高级阈值动作、完整历史趋势报告和跨环境对比报告。
3. Agent 指标秒级流、Run WebSocket 双向控制和实时订阅增强。
4. Agent deb/rpm 等系统发行包、模板审批/权限隔离、更高级阈值动作和更多 pprof / prof 类型。
5. Reports 页面补 profile artifact 深度 diff 分析、完整历史趋势报告和更多瓶颈诊断结论。
