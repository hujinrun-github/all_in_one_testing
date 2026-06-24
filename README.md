# All-in-One Testing 使用文档

完整平台使用手册见：[docs/usage-guide.md](docs/usage-guide.md)。

All-in-One Testing 是一个通用压测平台项目，目标是支持 HTTP、自定义 RPC 等多种协议，并通过 Target Agent 在压测过程中采集被压测机器状态、进程指标和 pprof / prof 产物。

当前仓库处于基础原型阶段：

- 后端已实现 Go API 服务骨架和 `/api/health` 健康检查接口。
- 后端已实现最小压测接口 `/api/runs`，可对一个 HTTP / HTTPS URL、已保存 HTTP 场景或 `CUSTOM_RPC` adapter 场景发起异步压测、返回汇总指标，并把运行历史写入 SQLite。
- 后端已实现场景接口 `/api/scenarios`，使用 SQLite 做原型阶段持久化，支持 `projectId` / `environment` 场景归属字段和列表过滤；场景协议当前支持 `HTTP` 和 `CUSTOM_RPC`，其中 `CUSTOM_RPC` 会把统一请求转发给用户自定义 adapter。
- 后端已实现 Agent token 创建、过期、吊销和轮换接口 `/api/agent-tokens`、`/api/agent-tokens/{id}/revoke`、`/api/agent-tokens/{id}/rotate`、心跳接口 `/agent/v1/heartbeat`、指标接口 `/agent/v1/metrics`、Agent 列表接口 `/api/agents` 和指标时间线接口 `/api/agent-metrics`，支持 `projectId` / `environment` token 归属字段、heartbeat 继承和 Agent 列表过滤，使用 SQLite 保存 token hash、过期时间、Agent 在线状态和指标样本；Agent daemon 在 Linux 上会通过 `/proc` 采集主机 CPU、内存、磁盘、网络和进程指标，非 Linux 环境会降级为 Agent 自身进程快照；Agent 上报需要携带 `Authorization: Bearer <token>`。
- 后端已实现 Target 创建、列表、详情、更新和删除接口 `/api/targets`，支持 `projectId` / `environment` 目标归属字段和列表过滤；可把被压测目标绑定到已注册 Agent，并保存 pprof endpoint、进程匹配规则、HTTP health check 与主机指标告警阈值；`/api/targets/{id}/health-check` 可对 Target 发起一次性健康探测。
- 后端已实现最小 profile artifact 链路：带 Target 运行场景时会从 Target 的 `profileEndpoint` 拉取 1 秒 CPU pprof；如果 Target 绑定了 Agent，Run 创建时也会自动创建一次 `cpu` Profile Task，Agent 下一轮轮询会领取并采集上传；当 Target 配置了 `metricThresholds` 且压测窗口内 Agent 指标超过阈值时，Run 结束后会额外创建 `threshold_auto` 来源的 `cpu` Profile Task；Agent 也可以通过 `/agent/v1/profile-artifacts` 上传已采集的 pprof/prof 文件；Control Plane 可以通过 `/api/profile-tasks` 创建采集任务，Agent 通过 `/agent/v1/profile-tasks` 领取并回写完成状态，超时未完成的 leased 任务会被后续轮询重新领取；`cmd/agent` 支持从 Go pprof base URL 按类型采集 `cpu`、`heap`、`goroutine`、`mutex`、`block` 等 profile 并上传，也支持执行显式配置的受控命令 profiler，把命令生成的本地文件作为 artifact 上传；也支持 `--poll-once` 处理一批任务，或用 `--daemon` 常驻上报心跳、指标并轮询 Profile Task；后端会保存文件和 SQLite 元数据，并通过 `/api/profile-artifacts` 查询、通过 `/api/profile-artifacts/{id}/download` 下载原始文件，也支持 `/api/reports/profile-artifacts/compare` 按最近 Run 聚合 profile 类型、采集状态、总大小、最新/上一次大小差和状态变化，支持 `/api/reports/process-trends/compare` 按最近 Run 对比同一 Target 进程的 CPU/RSS/fd/thread 峰值、最新/上一次 PID 和资源 delta。
- 前端已实现 React + Ant Design 控制台原型，包含 Dashboard、Targets & Agents、Scenarios、Runs、Reports 页面；顶栏提供 Project / Environment scope，点击 `Apply scope` 后会按同一组归属过滤 Scenario、Target、Agent 列表，并作为新建 Scenario、Agent token 和 Target 的默认项目/环境；前端会在 scoped API 请求中同时发送 `projectId` / `environment` 查询参数和 `X-AIT-Project-ID` / `X-AIT-Environment` 请求头，后端会据此限制列表范围并拒绝跨 scope 写入；Scenarios 页面可创建带 Project ID / Environment 的 HTTP 场景和 `CUSTOM_RPC` adapter 场景；Targets 页面可生成、轮换、吊销 Agent token 和 `/agent/install.sh` 一键安装命令，安装脚本默认从 `/agent/binaries/all-in-one-agent-linux-amd64` 下载预构建 Agent 二进制，并在 `sha256sum` 可用时使用 `/agent/binaries/all-in-one-agent-linux-amd64.sha256` 校验下载内容，读取 Agent 指标、创建带 Project ID / Environment 的 Target、绑定或解绑 Agent，并配置 CPU/MEM/磁盘/网络阈值和 HTTP health check，列表里可点击 `健康检查 / Check` 立即探测 Target，也可在 Profile templates 面板维护 SQLite 白名单模板（默认 `Go pprof` 和 `Linux perf`），并对已绑定 Agent 的 Target 选择模板、填写 profile 类型、PID 和 `1..300` 秒采集时长后点击 `采集 / Profile` 手动下发 Profile Task；Runs 页面可选择已保存场景和 Target 发起真实压测并展示后端历史运行记录，Reports 页面可读取最新 Run 报告聚合、展示多 Run 对比、跨 Run 进程趋势对比、guardrail 告警摘要、Target 指标阈值告警、慢请求样本、错误样本、按 Run 时间窗口聚合的 Target CPU/MEM/磁盘/网络峰值与趋势图、按 PID 聚合的窗口内进程趋势、按 Target `processMatch.cmdlineContains` 优先过滤并以 `processMatch.name` 兜底的最新进程快照、profile artifact 对比、Profile Task 命令详情、Agent CPU/MEM 指标趋势、进程摘要，并下载后端 profile artifacts。
- Agent deb/rpm 等系统发行包、完整权限模型、更细粒度的多指标趋势分析和高级阈值动作仍在设计规划中。

## 目录结构

```text
all_in_one_testing/
  backend/                 Go 后端 API 服务
    cmd/server/            后端启动入口
    internal/api/          API 路由与测试

  frontend/                React 前端控制台
    src/app/               页面壳、控制台 UI、样式和测试
    src/api/               前端 API client

  docs/
    superpowers/specs/     平台设计文档
    superpowers/plans/     分阶段实现计划
```

## 环境要求

- Go 1.22+
- Node.js 20+ 推荐
- npm
- Python 3 可选，用于本地静态预览 `frontend/dist`

## 启动后端

进入后端目录：

```powershell
cd backend
go run ./cmd/server
```

默认监听地址：

```text
http://127.0.0.1:8080
```

可以通过环境变量修改监听地址：

```powershell
$env:APP_ADDR=':8081'
go run ./cmd/server
```

验证健康检查：

```powershell
curl http://127.0.0.1:8080/api/health
```

预期响应：

```json
{"status":"ok"}
```

## 启动前端开发服务

首次进入前端目录后安装依赖：

```powershell
cd frontend
npm install
```

启动 Vite 开发服务：

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

因此本地联调时通常需要同时启动后端和前端。

## 生产构建与静态预览

构建前端产物：

```powershell
cd frontend
npm run build
```

构建产物位于：

```text
frontend/dist
```

使用 Python 在 `5175` 端口预览静态产物：

```powershell
python -m http.server 5175 --bind 127.0.0.1 --directory frontend/dist
```

访问：

```text
http://127.0.0.1:5175
```

当前本机也可通过 Tailscale Funnel 预览：

```text
https://tylerhu-1.tail5cec87.ts.net/
```

如果浏览器看到旧页面，先重新执行 `npm run build`，然后使用 `Ctrl+F5` 强制刷新。

## 前端页面说明

当前前端是控制台原型，使用静态示例数据展示产品形态。

### Dashboard

展示压测工作台总览：

- 运行态势
- 当前 QPS、p95 延迟、错误率、在线 Agent
- 实时压测阶段
- Agent 健康
- 画像观察
- 近期运行

### Targets & Agents

展示目标服务和被压测机器接入方式：

- 目标拓扑
- Agent 接入命令
- 后端 Agent 列表
- Agent 最新主机和进程指标
- 主机状态采集项
- Target 绑定和 pprof endpoint

当前已接入 Agent token 生成、过期时间、轮换和吊销、`cmd/agent --config` JSON 配置文件、`cmd/package-agent` Linux amd64 产物生成、`/agent/install.sh` bootstrap 安装脚本、`/agent/binaries/manifest.json` 发行产物清单、`/agent/binaries/all-in-one-agent-linux-amd64` 预构建二进制下载、`/agent/binaries/all-in-one-agent-linux-amd64.sha256` checksum 下载或动态生成、心跳鉴权、项目/环境归属、顶栏 scope 过滤、指标上报、列表展示、Target 创建/更新/删除、Agent 绑定/解绑、HTTP health check、Agent 周期性 target health 上报和 CPU/MEM/磁盘/网络阈值配置；Runs 页面已经可以选择当前 scope 内的 Target 发起场景压测，Reports 页面会基于阈值生成 `target_metric` 告警。后续会接入 deb/rpm Agent 发行包和高级阈值动作。

### Scenarios

展示场景编排入口：

- 调用链编排
- HTTP step
- 自定义 RPC / Adapter step
- 变量提取
- 断言配置
- 创建带 Project ID / Environment 的场景并通过 `/api/scenarios` 保存到后端 SQLite 数据库

当前已支持启用的 HTTP request flow steps 按顺序执行，同一个压测样本内会自动透传 `Set-Cookie` 到后续步骤；支持从上一条响应的 JSON body、Header 或 body regex 提取变量，并在后续请求的 path、Header、Query、Body 中用 `${name}` 引用；也支持 `status < 400` 这类状态码断言，以及 JSON/Header/Body response assertion flow step，包含基础正则 matches / not_matches；同时支持在 flow step 上配置 `when` 条件，来源可以是上一条响应的 JSON/Header/Body/Regex 或已提取变量，条件不满足时跳过该 step。后续会支持版本化、循环编排、复杂分支图和更完整的提取器/断言 DSL。

### Runs

展示压测执行控制台：

- 运行控制
- 选择已保存场景并发起真实压测
- 总请求数、并发、超时配置
- 运行汇总结果
- 运行历史记录
- 保护阈值展示
- 阶段时间线
- 启动与停止操作

后续会接入真实 Executor、任务调度、运行状态和实时指标。

### Reports

展示报告分析入口：

- 瓶颈雷达
- 画像产物
- 瓶颈摘要
- 报告明细

当前 Reports 页面已经会读取最新 Run 的 `/api/reports/runs/{id}` 聚合结果，展示成功率、错误率、Run event 数量、alert 数量、profile artifact 数量、慢请求样本、错误样本，以及按 Run 开始时间和运行时长自动对齐的 Target 指标窗口；当 Run 绑定了 Target 时，报告还会返回最近 `targetHealthChecks` 并展示目标健康状态、HTTP 状态码、探测时间和错误原因，便于判断目标是在压测前已经不健康还是压测期间才劣化。当 Run 触发 `maxErrorRatePercent` 或 `maxP95LatencyMs` 被中止时，报告会展示 `kind=guardrail` 告警摘要，包括严重级别、触发指标、观测值、阈值和关联 `run_aborted` 事件；当 Run 绑定了 Target 且 Target 配置了 `metricThresholds` 时，报告会基于窗口内 CPU/MEM/磁盘/网络峰值生成 `kind=target_metric` 告警；当 Run 绑定了 Target 且 Target 绑定了 Agent 时，报告会展示窗口内样本数、CPU/MEM/磁盘/网络峰值、CPU/MEM/磁盘/网络趋势图、关联 Agent、`processTrends` PID 级进程趋势摘要，以及按 Target `processMatch.cmdlineContains` 优先过滤并以 `processMatch.name` 兜底的最新进程快照。页面也会调用 `/api/reports/compare?limit=5` 对比最近多次 Run 的成功率、错误率、QPS 和 p95，并标出 best qps、fastest p95 和 highest errors；调用 `/api/reports/process-trends/compare?limit=5` 按 Target + Agent + 进程名 + cmdline 对比最近 Run 的进程峰值、最新/上一次 PID、CPU delta 和 RSS delta；调用 `/api/reports/profile-artifacts/compare?limit=5` 按 profile 类型对比最近 Run 的 artifact 数量、成功/失败、总大小、最新 artifact 相对上一次的大小 delta / 百分比变化，以及采集状态是否从成功变为失败；同时读取 `/api/profile-artifacts` 展示 pprof artifact 元数据，并为已采集成功的 artifact 提供下载入口；也会用最新 Run 的 `runId` 调用 `/api/profile-tasks?runId=...&limit=20` 展示相关 Profile Task 队列。

## 测试

运行后端测试：

```powershell
cd backend
go test ./...
```

运行前端测试：

```powershell
cd frontend
npm test -- --run
```

运行前端生产构建：

```powershell
cd frontend
npm run build
```

当前前端构建可能出现 Vite chunk-size 提示，这是 Ant Design 依赖体积导致的构建警告，不影响产物生成。后续可通过按路由拆包或手动 chunk 优化。

## 当前 API

### GET `/api/health`

用途：检查后端服务是否正常。

响应：

```json
{
  "status": "ok"
}
```

### `/api/scenarios`

用途：管理压测场景。原型阶段默认写入 SQLite 数据库 `backend/data/platform.db`，可通过 `SCENARIO_DB_PATH` 指定数据库文件。

```text
GET    /api/scenarios
POST   /api/scenarios
GET    /api/scenarios/{id}
PUT    /api/scenarios/{id}
DELETE /api/scenarios/{id}
```

`GET /api/scenarios` 可选 `projectId` 和 `environment` 查询参数，例如 `GET /api/scenarios?projectId=project-checkout&environment=staging`。`POST` / `PUT` 请求体可传 `projectId`、`environment`，不传时默认写入 `default` / `default`。

如果请求携带 `X-AIT-Project-ID` 和 `X-AIT-Environment`，后端会把 Scenario、Target 和 Agent 列表限制在该 workspace scope 内；创建 Scenario、Target 或 Agent token 时，body 里的 `projectId` / `environment` 必须与 header 一致，缺省时会由 header 填充，不一致会返回 `403`。Scenario / Target 的单资源 `GET`、`PUT`、`DELETE`，以及 Target health check 和 health check history 也会校验 header scope；跨 scope 访问会返回 `403`。未携带这两个 header 时保留原型阶段兼容行为。

### Agent API

用途：生成 Agent token、接收被压测机器上的 Agent 心跳和指标，并在控制台查询 Agent 在线状态及最新指标。原型阶段同样写入 SQLite 数据库 `backend/data/platform.db`。Agent 上报接口需要携带 `Authorization: Bearer <token>`。

Agent 发布产物默认放在 `backend/dist/agents`。执行 `go run ./cmd/package-agent --version <version> --commit <commit>` 会生成 `all-in-one-agent-linux-amd64`、对应 `.sha256` 和 `manifest.json`；后端的 `/agent/binaries/manifest.json` 会优先返回这个 manifest。`GET /api/agents` 会读取 manifest 中的最新 Agent 版本，并在每个 Agent 记录上返回 `latestVersion` 和 `upgradeAvailable`；Targets & Agents 页面会在 Agent inventory 的 version 单元格里提示 `升级可用 / Upgrade available`。目标机器升级时可重新运行 `/agent/install.sh` 并追加 `--force-download` 或 `--upgrade`，安装脚本会下载到临时文件、校验 checksum 后替换二进制，并重启 `all-in-one-agent.service`。

```text
POST /api/agent-tokens
POST /agent/v1/heartbeat
POST /agent/v1/metrics
GET  /api/agents
GET  /api/agent-metrics
```

创建 Agent token：

```powershell
$tokenResponse = Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/agent-tokens" `
  -ContentType "application/json" `
  -Body '{"name":"checkout-install","projectId":"project-checkout","environment":"staging"}'

$agentToken = $tokenResponse.token
```

Agent token 的 `projectId` 和 `environment` 会写入 SQLite；Agent 使用该 token 上报 heartbeat 后会继承同一组归属字段。`GET /api/agents` 也支持同样的查询参数，例如：

```powershell
Invoke-RestMethod "http://127.0.0.1:8080/api/agents?projectId=project-checkout&environment=staging"
```

心跳请求示例：

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
} | ConvertTo-Json -Depth 6

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/agent/v1/heartbeat" `
  -Headers @{ Authorization = "Bearer $agentToken" } `
  -ContentType "application/json" `
  -Body $heartbeat
```

指标请求示例：

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
  -Uri "http://127.0.0.1:8080/agent/v1/metrics" `
  -Headers @{ Authorization = "Bearer $agentToken" } `
  -ContentType "application/json" `
  -Body $metrics
```

查询某个 Agent 的指标时间线：

```powershell
Invoke-RestMethod "http://127.0.0.1:8080/api/agent-metrics?agentId=agent-checkout-01&limit=120"
```

### Target API

用途：保存被压测目标，并把 Target 绑定到已注册 Agent。原型阶段写入 SQLite 数据库 `backend/data/platform.db`。

```text
GET  /api/targets
POST /api/targets
GET  /api/targets/{id}
PUT  /api/targets/{id}
DELETE /api/targets/{id}
POST /api/targets/{id}/health-check
GET  /api/targets/{id}/health-checks?limit=5
```

`GET /api/targets` 可选 `projectId` 和 `environment` 查询参数，例如 `GET /api/targets?projectId=project-checkout&environment=staging`。`POST` / `PUT` 请求体可传 `projectId`、`environment`，不传时默认写入 `default` / `default`。

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

手工探测 Target：

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/targets/target-xxx/health-check"
```

健康检查结果会写回 Target 的 `lastHealthCheck` 字段；`GET /api/targets` 和 `GET /api/targets/{id}` 会返回最近一次检查状态。平台也会把结果追加到 `target_health_checks` 历史表，单个 Target 默认保留最近 200 次，`GET /api/targets/{id}/health-checks?limit=5` 会按最近优先返回历史记录。通过 `/api/runs` 启动压测时触发的 Target 健康预检会更新最近状态和历史记录；Agent daemon 也会通过 `/agent/v1/targets` 拉取当前 token scope 内绑定到自己的 Target，并把周期性 HTTP 探活结果 POST 到 `/agent/v1/target-health-checks`。

### `/api/runs`

用途：发起一次最小 HTTP 压测，并查询最近运行历史。压测接口当前是同步执行：请求会等压测完成后返回汇总结果，并写入 SQLite `run_history` 表。

```text
GET  /api/runs
POST /api/runs
```

`GET /api/runs` 返回最近 100 条运行历史，按创建时间倒序排列。

支持两种模式：

- 直接 URL 模式：传 `method`、`url`、`totalRequests`、`concurrency` 等字段。
- 场景模式：传 `scenarioId`、`totalRequests`、`concurrency`，后端会读取 SQLite 中保存的场景快照。`HTTP` 场景会执行场景里的 headers、queryVariants 和 bodyVariants；`CUSTOM_RPC` 场景会向 adapter URL 发起 `POST`，把 `protocol`、`method`、`runId`、`scenarioId`、headers、queryParams 和 body 传给 adapter。
- Target 场景模式：在场景模式基础上额外传 `targetId`，后端会使用 Target 的 `baseUrl` 加场景 path 发起压测，并把 `targetId`、`targetName`、`projectId`、`environment` 写入运行结果和历史记录。
- 如果 Target 配置了 `healthCheck`，`POST /api/runs` 会在创建 Run 前先执行一次 Target 健康预检；预检失败会返回 `409` 和 `{"error":"target health preflight failed: ..."}`，不会创建 Run，也不会向场景 URL 发起压测请求。
- 如果 Target 配置了 `profileEndpoint`，场景运行期间会同步采集一次 1 秒 CPU pprof，并把 artifact 元数据写入 `/api/profile-artifacts`，原始文件可通过 `/api/profile-artifacts/{id}/download` 下载；如果 Target 同时绑定了 Agent，平台还会为每个绑定 Agent 自动创建一个 `cpu` Profile Task，Agent daemon 或 `--poll-once` 会领取并上传采集结果。

请求示例：

```powershell
curl -X POST http://127.0.0.1:8080/api/runs `
  -H "Content-Type: application/json" `
  -d '{
    "name": "health-smoke",
    "method": "GET",
    "url": "http://127.0.0.1:8080/api/health",
    "totalRequests": 100,
    "concurrency": 10,
    "timeoutMs": 3000
  }'
```

字段说明：

```text
name           压测名称，可选
method         GET 或 POST，默认 GET
url            目标 HTTP / HTTPS 绝对地址
totalRequests  总请求数，范围 1 到 10000
concurrency    并发数，范围 1 到 256
timeoutMs      单请求超时时间，默认 3000ms
scenarioId     可选，已保存场景 ID；传入后可省略 url/method/bodyVariants/queryVariants/headers
targetId       可选，已保存 Target ID；与 scenarioId 一起使用时覆盖场景 baseUrl
headers        可选，请求头；场景模式会使用场景里保存的 Header KV
queryVariants  可选，查询参数候选列表；按正权重随机追加到 URL，weight=0 不会被选中
bodyVariants   可选，请求体候选列表；按正权重随机选择，weight=0 不会被选中
```

GET / POST 场景可以增加 `queryVariants`，例如：

```json
"queryVariants": [
  { "name": "shanghai", "weight": 80, "queryParams": "tenant=shanghai&debug=true" },
  { "name": "beijing", "weight": 20, "queryParams": "tenant=beijing&debug=false" }
]
```

POST 场景可以增加 `bodyVariants`，例如：

```json
"bodyVariants": [
  { "name": "small-order", "weight": 70, "body": "{\"amount\":100,\"currency\":\"CNY\"}" },
  { "name": "large-order", "weight": 30, "body": "{\"amount\":300,\"currency\":\"CNY\"}" }
]
```

响应示例：

```json
{
  "id": "run-1780000000000000000",
  "name": "health-smoke",
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
      "targetName": "checkout-service",
      "profileType": "cpu",
      "status": "collected",
      "fileName": "run-1780000000000000000-cpu.pprof",
      "sizeBytes": 1048576
    }
  ]
}
```

查询 profile artifact 元数据：

```powershell
curl http://127.0.0.1:8080/api/profile-artifacts
```

下载原始 pprof 文件：

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
cd backend
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

`--profile-type` 支持 `cpu`、`heap`、`goroutine`、`mutex`、`block`、`allocs`、`threadcreate`。如果需要完全自定义采集地址，也可以继续传 `--profile-url`。

如果目标没有 HTTP pprof，或者需要用 `perf` 等系统工具采集，Agent 命令行也支持受控命令 profiler。该模式不会通过 shell 执行字符串，只会执行 `--profile-command` 指定的二进制，并把每个 `--profile-command-arg` 作为独立参数传入；参数支持 `{{output}}` 和 `{{seconds}}` 占位符：

```powershell
cd backend
go run .\cmd\agent `
  --control-plane http://127.0.0.1:8080 `
  --token <agent-token> `
  --run-id run-1780000000000000000 `
  --target-name checkout-01 `
  --profile-type perf `
  --profile-seconds 30 `
  --file-name checkout-perf.data `
  --profile-command perf `
  --profile-command-output C:\tmp\checkout-perf.data `
  --profile-command-timeout 45s `
  --profile-command-arg record `
  --profile-command-arg -F `
  --profile-command-arg 99 `
  --profile-command-arg -p `
  --profile-command-arg 1234 `
  --profile-command-arg -g `
  --profile-command-arg -o `
  --profile-command-arg "{{output}}" `
  --profile-command-arg -- `
  --profile-command-arg sleep `
  --profile-command-arg "{{seconds}}"
```

命令 profiler 也可以通过 Profile Task 下发给绑定 Agent，由 `cmd/agent --poll-once` 或 `cmd/agent --daemon` 领取后执行。当前 API 接收显式命令字段，并提供 `/api/profile-task-templates` 模板 CRUD：`GET` 查询 enabled 模板，`POST` 创建模板，`PUT /api/profile-task-templates/{id}` 更新模板，`DELETE /api/profile-task-templates/{id}` 软删除模板；默认种子数据包含 `Go pprof` 和 `Linux perf`。当 Run 绑定的 Target 配置了 `metricThresholds` 且压测窗口内 Agent 指标超过阈值时，后端会在 Run 结束后自动创建 `threshold_auto` 来源的 `cpu` Profile Task；该能力需要 Target 配置 `profileEndpoint`。

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

`profileSeconds` 必须在 `1..300` 范围内。Profile Task 必须提供 `pprofBaseUrl`、`profileUrl` 或 `profileCommand` 之一；命令任务的 `profileCommandArgs` 是数组，不经过 shell，支持 `{{output}}` 和 `{{seconds}}` 占位符。`source` 表示任务来源，支持 `api`、`run_auto`、`target_manual`、`threshold_auto`；直接 API 创建默认是 `api`，Run 创建时基线任务是 `run_auto`，Targets 页面手工下发是 `target_manual`，Target 指标超过阈值后自动下发是 `threshold_auto`。`maxAttempts` 默认为 3，最大为 10。Agent 回写 `error` 时平台会把 `attempts` 加 1；未达到 `maxAttempts` 时任务会回到 `pending` 等待下一轮领取，达到上限后才进入 `failed`。`GET /api/profile-tasks` 支持 `runId`、`agentId`、`status`、`source` 和 `limit` 查询参数，例如 `/api/profile-tasks?runId=run-xxx&source=run_auto&status=pending&limit=20`；`leased` 任务响应会返回派生字段 `leaseExpiresAt`，表示当前租约到期时间。`POST /api/profile-tasks/{id}/retry` 可把已 `failed` 的任务重新置为 `pending`，清空最近错误、artifact、租约和完成时间，并把 `attempts` 重置为 0。Reports 页面会按最新 Run 过滤展示 `source`、`attempts/maxAttempts`、最近错误、租约到期时间和当前状态，并可对失败任务点击 Retry 重新入队，也可以按来源筛选任务队列。

Agent 领取并处理一批任务：

```powershell
cd backend
go run .\cmd\agent `
  --control-plane http://127.0.0.1:8080 `
  --token <agent-token> `
  --agent-id agent-checkout-01 `
  --poll-once
```

Agent 常驻模式会启动后立即上报一次 heartbeat、metrics 并领取一次 Profile Task，之后按固定间隔持续执行；如果某次上报或拉取失败，进程会继续等待下一轮重试：

```powershell
cd backend
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

默认文件位置：

```text
backend/data/profile_artifacts/
```

当前限制：

- 当前执行模型支持异步 HTTP 压测、进度轮询和停止；已支持直接 URL、已保存场景和 Target 三种入口。
- 已保存最近运行历史、Run guardrail 阈值、Run event 日志、慢请求样本、错误样本、Target 健康检查历史和 CPU pprof artifact 元数据，支持 `/api/reports/runs/{id}` 聚合单次 Run 报告并派生 guardrail 告警摘要、Target 指标阈值告警和 Target 健康历史，支持 `/api/reports/compare` 对比最近多次 Run，支持 `/api/reports/process-trends/compare` 对比最近 Run 的 Target 进程趋势和资源 delta，支持 `/api/reports/profile-artifacts/compare` 按 profile 类型对比最近 Run 的 artifact 数量、大小 delta 和状态变化，支持下载已采集成功的 pprof 原始文件。
- 已支持 `/api/runs/{id}/events` 查询运行事件，也支持 `/api/runs/{id}/events/stream` SSE 实时推送；Runs 页面会优先使用 EventSource 订阅事件流，并在不支持 SSE 时回退到轮询。
- 已支持 queryVariants 按权重随机追加查询参数、bodyVariants 按权重随机选择请求体，并支持场景中的 Header KV；已支持 `CUSTOM_RPC` adapter 场景，把统一请求转发给用户自定义协议适配器。
- 已支持启用的 HTTP request flow steps、样本内 Cookie Jar 自动透传、JSON/Header/Regex 变量提取、`${name}` 变量引用、场景级 status assertion、JSON/Header/Body response assertion flow step，以及基于上一条响应或变量的 step-level `when` 条件跳过；暂不支持循环编排、复杂分支图、完整 JSONPath 和高级断言 DSL。
- 已接收 Agent 最新机器指标和进程指标；Linux Agent daemon 会读取 `/proc` 采集 CPU、内存、磁盘、网络、进程 cmdline 和进程 RSS/fd/thread；已支持按 Agent 查询指标时间线，在 Reports 页面展示 Agent CPU/MEM 趋势，并在 `/api/reports/runs/{id}` 中按 Run 时间窗口自动聚合 Target CPU/MEM/磁盘/网络峰值、样本序列、窗口趋势图，使用 Target `metricThresholds` 派生 `target_metric` 告警，按 PID 聚合窗口内 `processTrends` 进程趋势，并按 `processMatch.cmdlineContains` / `processMatch.name` 过滤最新进程快照；已支持 `/api/reports/process-trends/compare` 跨 Run 对比同一 Target 进程的最新/上一次 PID、CPU/RSS/fd/thread 峰值和资源 delta；完整历史多维趋势报告仍是后续能力。
- 已支持 Control Plane 直接拉取 Go CPU pprof，也支持 Run 创建时为绑定 Agent 自动下发 `cpu` Profile Task；当 Target 指标超过阈值时，也支持在 Run 结束后自动下发 `threshold_auto` 的 `cpu` Profile Task；Agent 可通过 API 上传 profile artifact，并支持 `cmd/agent` 一次性按类型采集上传、受控命令 profiler、`--poll-once` 处理 pprof/命令 Profile Task、`--daemon` 常驻轮询、`--config` JSON 配置文件、`cmd/package-agent` Linux amd64 产物生成、`/agent/install.sh` 生成 `/etc/all-in-one-agent/agent.json` 的 systemd bootstrap 安装脚本、`/agent/binaries/all-in-one-agent-linux-amd64` 预构建二进制下载和 `.sha256` 校验；Targets 页面已支持从 SQLite 白名单模板下发 `Go pprof` 和 `Linux perf`，并支持基础模板创建、更新和删除；Profile Task 已支持租约超时回收、`attempts/maxAttempts` 失败重试，以及对已失败任务手动 retry 重新入队，暂不支持 deb/rpm Agent 发行包和更高级的阈值动作/模板审批。

## 后续规划

完整平台目标见：

```text
docs/superpowers/specs/2026-06-07-load-testing-platform-design.md
```

后续核心里程碑：

1. 项目、环境、Target、Agent 的权限模型和绑定关系校验。
2. Linux Target Agent：deb/rpm 发行包、正式配置文件、指标采集客户端和进程匹配规则。
3. Scenario：`CUSTOM_RPC` 场景级 adapter 已接入；后续补自定义 RPC flow step、循环、复杂分支、变量、提取器和高级断言。
4. Executor：并发、固定 QPS、阶梯加压。
5. 实时监控：Run SSE 事件流已接入，后续继续补秒级指标流和更完整的 WebSocket 双向控制。
6. Profiling：更多 pprof 类型、模板审批/权限隔离和更高级阈值动作。
7. Reports：单 Run 聚合、多 Run 对比、profile artifact 数量/大小/状态对比、慢请求样本、错误样本和按 Run 时间窗口聚合的 Target CPU/MEM/磁盘/网络指标趋势已接入，后续补完整历史趋势报告和基于 pprof 内容的深度 diff 分析。

## 常见问题

### 前端页面没有变化

生产静态预览需要先重新构建：

```powershell
cd frontend
npm run build
```

如果浏览器仍显示旧页面，使用 `Ctrl+F5` 强制刷新。

### 前端无法访问后端

确认后端已启动：

```powershell
curl http://127.0.0.1:8080/api/health
```

确认 Vite proxy 仍指向 `http://127.0.0.1:8080`：

```text
frontend/vite.config.ts
```

### 端口被占用

后端端口可通过 `APP_ADDR` 修改。前端开发服务可通过 Vite 参数指定端口：

```powershell
npm run dev -- --port 5174
```

静态预览也可换端口：

```powershell
python -m http.server 5180 --bind 127.0.0.1 --directory frontend/dist
```

## 开发约定

- 后端接口行为应优先补 Go `httptest` 测试。
- 前端 UI 行为应优先补 React Testing Library 测试。
- 前端 API client 应使用 Vitest，在网络边界 stub `fetch`。
- 新功能遵循 TDD：先写失败测试，再实现，再跑完整测试。
- 不要把规划中的功能写成已实现能力；当前 README 按实际代码状态维护。

## Workspace Scope 补充

前端点击 `Apply scope` 后，Scenario、Target、Agent、Run、Report、Agent metrics、Profile Task、Profile Artifact 和 Agent token 操作都会携带同一组 `projectId` / `environment` 查询参数或 `X-AIT-Project-ID` / `X-AIT-Environment` 请求头。后端会按该 scope 过滤列表，并对单资源读取、停止 Run、Profile Task retry、Profile Artifact 下载、Agent metrics 查询、Agent token rotate/revoke 等操作做跨 scope 拒绝。

`EventSource` 和下载链接无法设置自定义请求头，因此后端也接受 `?projectId=...&environment=...` 作为 workspace scope。未携带 header 或 query scope 时，仍保留原型阶段的全量兼容行为。
