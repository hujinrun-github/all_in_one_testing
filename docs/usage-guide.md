# All-in-One Testing 平台使用手册

本文档面向平台使用者和运维接入人员，覆盖从启动平台、创建压测场景、发起实际压测，到在被压测机器上安装 Agent、采集机器指标和 pprof / prof 数据的完整流程。

> 当前代码处于原型阶段：前端已经具备控制台页面、HTTP 场景表单和场景 API 接入，后端已经具备 `/api/health`、`/api/scenarios` 和同步 HTTP 压测接口 `/api/runs`。Agent 注册、机器指标上报、实时监控、报告落库和 profile artifact 上传仍是目标能力，本文会明确标注“当前可用”和“目标流程”。

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
- 使用 `/api/runs` 对一个 HTTP / HTTPS URL 发起同步压测。
- 使用 `/api/scenarios` 创建、查询、更新和删除 HTTP 场景；原型阶段默认写入 SQLite 数据库。
- 启动 React 前端控制台。
- 在前端查看 Dashboard、Targets & Agents、Scenarios、Runs、Reports 页面。
- 在 Scenarios 页面创建 HTTP 场景配置，前端会优先保存到后端，后端不可用时保留浏览器本地缓存兜底。
- 在 Runs 页面选择已保存场景，配置请求数、并发和超时后发起真实压测。
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

### 当前尚未接入真实后端的能力

- Target / Agent 注册和心跳。
- Agent 安装包和安装脚本。
- 主机指标、进程指标、health check 数据上报。
- pprof / prof artifact 上传和下载。
- 压测过程实时事件流。
- 历史报告和趋势分析。

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

当前 Dashboard 使用前端静态示例数据。后续接入后端后，应从 Run、Agent、Metrics、Profile Artifact 等数据源实时读取。

### 4.2 Targets & Agents

Targets & Agents 用于管理被压测目标和机器 Agent：

- 查看服务拓扑。
- 查看 Agent 在线状态。
- 查看主机采集项。
- 配置 Target 与 Agent 的绑定关系。
- 配置 pprof / prof endpoint。

当前页面是原型展示。目标流程如下：

1. 在平台创建 Target。
2. 生成 Agent 注册 token。
3. 在被压测机器安装 Agent。
4. Agent 上报心跳。
5. 平台将 Agent 绑定到 Target。
6. 配置进程匹配、端口检查、HTTP health check、pprof endpoint。

### 4.3 Scenarios

Scenarios 用于创建压测场景。

当前前端支持创建一个本地 HTTP 场景，字段说明如下：

| 字段 | 说明 | 示例 |
|---|---|---|
| Scenario name | 场景名称 | `payment-http-smoke` |
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

创建后，场景会显示在页面的 `Created scenarios` 区域，并通过后端 `/api/scenarios` 保存到 SQLite 数据库。刷新页面、重启前端或换浏览器后，只要连接同一个后端并使用同一个数据库文件，场景都会从后端恢复。当前前端仍保留 `localStorage` 兜底：当后端不可用时，场景会暂存在当前浏览器。

### 4.4 Scenario API

后端已经提供原型阶段的场景 CRUD：

```text
GET    /api/scenarios
POST   /api/scenarios
GET    /api/scenarios/{id}
PUT    /api/scenarios/{id}
DELETE /api/scenarios/{id}
```

创建场景示例：

```powershell
$scenario = @{
  name = "gateway-health-smoke"
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

默认持久化文件为 SQLite 数据库 `backend/data/platform.db`。这仍是原型存储方式，适合本地联调和验证接口；后续生产化建议在 SQLite 表结构上增加项目、环境、用户和版本字段，或在多节点部署时替换为 PostgreSQL。

### 4.5 Runs

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

当前 Runs 页面已经接入真实 Run API：可以选择已保存场景，填写总请求数、并发数和超时时间，点击 `Start run` 后同步调用后端 `/api/runs`。当前仍未接入 Target / Agent、保护阈值自动中止、实时事件流和历史报告。

### 4.5 Reports

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

当前页面为静态展示。后续应从后端报告数据和 artifact 存储读取。

## 5. 当前如何进行实际压测

当前可用的真实压测入口有两个：

```text
POST /api/runs
```

它会同步执行一次 HTTP 压测，完成后直接返回汇总结果。可以直接传 `url` 做临时压测，也可以传 `scenarioId` 执行已经保存到 SQLite 的场景快照。前端 Runs 页面使用的是 `scenarioId` 模式。

### 5.1 请求字段

| 字段 | 类型 | 必填 | 说明 |
|---|---:|---:|---|
| name | string | 否 | 压测名称，默认 `ad-hoc-http-run`。 |
| scenarioId | string | 否 | 已保存场景 ID。传入后，后端会读取场景中的 method、URL、headers、queryVariants、bodyVariants 和 timeout。 |
| method | string | 否 | 当前仅支持 `GET` 和 `POST`，默认 `GET`。 |
| url | string | 直接 URL 模式必填 | 绝对 HTTP / HTTPS URL。传 `scenarioId` 时可省略。 |
| totalRequests | number | 是 | 总请求数，范围 `1..10000`。 |
| concurrency | number | 是 | 并发数，范围 `1..256`。 |
| timeoutMs | number | 否 | 单请求超时时间，默认 `3000`。 |
| headers | array | 否 | 直接 URL 模式可传请求头；场景模式会使用场景里保存的 Header KV。 |
| queryVariants | array | 否 | 查询参数候选列表。每项包含 `name`、`weight`、`queryParams`，执行时按正权重随机选择并追加到 URL；`weight=0` 的候选不会被选中。 |
| bodyVariants | array | 否 | POST 请求体候选列表。每项包含 `name`、`weight`、`body`，执行时按正权重随机选择一个 body；`weight=0` 的候选不会被选中。 |

> 注意：当前 `/api/runs` 已支持直接 URL 压测，也支持按 `scenarioId` 读取保存场景并执行场景里的 Headers、Query variants 和 Body variants。仍未支持多步骤场景、复杂断言 DSL 和 Agent 指标关联。

### 5.2 PowerShell 示例

使用已保存场景发起压测：

```powershell
$run = @{
  scenarioId = "scenario-xxx"
  totalRequests = 100
  concurrency = 10
  timeoutMs = 1000
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:8080/api/runs" `
  -ContentType "application/json" `
  -Body $run
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
  "p95LatencyMs": 5.62
}
```

## 6. Agent 安装和接入

本节描述目标使用方式。当前仓库还没有实现 Agent 二进制、安装脚本和后端 Agent 接口。

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

### 6.3 在平台生成 Agent Token

目标流程：

1. 打开 `Targets & Agents`。
2. 选择项目和环境。
3. 点击 `Generate install command`。
4. 平台生成一次性或长期 Agent 注册 token。
5. 复制安装命令到被压测机器执行。

token 应按项目隔离，并支持过期、吊销和轮换。

### 6.4 一键安装命令

目标命令形态：

```bash
curl -fsSL http://control-plane:8080/agent/install.sh | sudo bash -s -- \
  --server http://control-plane:8080 \
  --token <AGENT_REGISTER_TOKEN> \
  --agent-name checkout-01 \
  --labels env=prod,service=checkout,zone=shanghai-a
```

安装脚本应完成：

- 下载 `all-in-one-agent` 二进制。
- 创建运行用户，例如 `alltesting-agent`。
- 写入配置文件。
- 注册 systemd service。
- 启动 Agent。
- 向平台发送首次注册请求。

### 6.5 手动安装方式

目标目录结构：

```text
/usr/local/bin/all-in-one-agent
/etc/all-in-one-agent/agent.yaml
/var/lib/all-in-one-agent/artifacts/
/var/log/all-in-one-agent/
/etc/systemd/system/all-in-one-agent.service
```

示例配置：

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
ExecStart=/usr/local/bin/all-in-one-agent --config /etc/all-in-one-agent/agent.yaml
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

目标验证方式：

```bash
curl http://control-plane:8080/api/projects/<projectId>/agents
```

预期能看到：

```json
[
  {
    "name": "checkout-01",
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
POST /agent/v1/heartbeat
```

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

当服务没有 HTTP pprof，或者需要使用系统工具时，可以使用受控命令 profiler。

示例：

```yaml
profiling:
  enabled: true
  type: "command"
  process_match:
    name: "checkout"
  commands:
    cpu: "perf record -F 99 -p {{pid}} -g -- sleep 30"
    flamegraph: "profile-to-flamegraph --input {{input}} --output {{output}}"
```

安全要求：

- 命令必须来自平台白名单模板。
- 用户不能在页面上输入任意 shell 命令。
- 需要限制采样时长。
- 需要限制 artifact 文件大小。
- 需要记录执行命令、开始时间、结束时间、退出码。

### 8.4 Artifact 上传

Agent 采集完成后应上传 artifact。

目标接口：

```text
POST /agent/v1/profile-artifacts
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
9. 配置进程匹配、health check 和 profiling。
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

- Agent token 必须按项目隔离。
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

在 Runs 页面选择已保存场景发起压测时，后端会通过 `scenarioId` 读取场景快照，并使用场景里的 Header KV、Query variants 和 Body variants。直接 URL 模式也支持传 `headers` 字段。当前还不支持多步骤场景和复杂断言 DSL。

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

1. Agent 注册和 heartbeat。
2. Agent 主机指标和进程指标上报。
3. Run 实时事件流。
4. pprof artifact 采集和上传。
5. Reports 页面接入真实报告数据。
