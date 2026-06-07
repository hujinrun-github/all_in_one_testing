# 通用压测平台设计方案

日期：2026-06-07

## 1. 背景与目标

本项目要建设一个通用压测平台，前端使用 React，后端使用 Go。平台需要支持 HTTP、自定义 RPC 等多种协议，并允许用户通过统一接口扩展压测调用方式。

第一版目标是跑通一条完整链路：

```text
注册 Agent
  -> 配置 Target
  -> 创建 HTTP / Adapter RPC 场景
  -> 创建压测任务
  -> 启动执行
  -> 实时查看压测指标和目标机器指标
  -> 压测中采集 pprof / prof
  -> 查看报告
```

核心原则：

- 控制面负责定义、调度、管理和展示压测。
- 执行器负责发压、执行场景、聚合请求指标。
- Target Agent 负责采集被压测机器状态和 profiling 产物。
- HTTP、自定义 RPC、后续插件都通过统一协议驱动接入。
- MVP 单 Go 服务起步，但代码边界按后续 worker / Kubernetes 扩展设计。

## 2. 总体架构

采用“控制面 + 执行器分层”方案。

```text
React Admin
   |
   | REST / WebSocket
   v
Go Control Plane
   |-- Auth / Project / Environment
   |-- Scenario / Task / Run / Report
   |-- Scheduler
   |-- Agent Gateway
   |-- Metrics / Profiling / Artifact
   |
   | in-process in MVP
   v
Executor Engine
   |-- Load Scheduler
   |-- Scenario Runner
   |-- Protocol Dispatcher
   |-- HTTP Driver
   |-- Adapter RPC Driver
   |-- Metrics Aggregator
   |
   v
Postgres / Redis / Artifact Storage

Target Machine
   |
   | target-agent
   |-- system metrics
   |-- process metrics
   |-- health check
   |-- Go pprof collector
   |-- controlled command profiler
   v
Agent Gateway
```

MVP 中 API、scheduler、executor、agent gateway 可以运行在同一个 Go 进程内。后续可拆成：

```text
api-server
scheduler
executor-worker
agent-gateway
metrics-consumer
```

## 3. 模块划分

建议目录结构：

```text
all_in_one_testing/
  backend/
    cmd/
      server/
      agent/
    internal/
      api/
      auth/
      project/
      environment/
      target/
      agent/
      scenario/
      task/
      executor/
      protocol/
        httpdriver/
        adapterdriver/
      metrics/
      profiling/
      report/
      storage/
      websocket/
      config/
    migrations/
    scripts/

  frontend/
    src/
      app/
      pages/
      components/
      features/
        auth/
        projects/
        environments/
        targets/
        agents/
        scenarios/
        tasks/
        runs/
        reports/
        profiles/
      api/
      store/
      routes/
      charts/

  docs/
    architecture/
    api/
    agent/
    superpowers/
      specs/

  deploy/
    docker-compose.yml
    backend.Dockerfile
    frontend.Dockerfile
    agent/
```

职责边界：

- Control Plane：管理用户、项目、环境、场景、任务、报告，负责“定义压测”。
- Executor Engine：执行场景、生成负载、调用协议 driver、采集请求指标，负责“实际发压”。
- Protocol Driver：屏蔽 HTTP、Adapter RPC、后续 Go 插件之间的差异。
- Target Agent：采集目标机器状态、进程状态、健康检查和 profile artifact。
- Metrics / Report：对齐请求指标、目标机器指标和 profiling 数据，形成实时图表与最终报告。
- Frontend：提供工程控制台体验，不直接理解执行器内部细节。

## 4. 用户、项目与权限

MVP 做基础登录和项目隔离：

- 用户使用账号密码登录。
- 用户按项目管理环境、Target、Agent、场景、任务和报告。
- 项目成员有基础角色：owner、member、viewer。

细粒度 RBAC、审计日志和组织级权限后续扩展。

## 5. 场景模型

平台使用 Scenario + Step 抽象压测场景，不把系统写死成 HTTP 压测工具。

```text
Project
  -> Environment
  -> Target
  -> Scenario
      -> Step 1: HTTP login
      -> Step 2: Adapter RPC call
      -> Step 3: HTTP query
  -> Load Profile
  -> Test Run
  -> Report
```

MVP 支持单接口和简单多步骤顺序执行：

```text
Scenario
  id
  project_id
  name
  description
  version
  variables
  steps[]

Step
  name
  protocol: http | adapter_rpc
  request_config
  extractors[]
  assertions[]
  timeout
  think_time
  failure_policy
```

变量能力：

- 项目变量
- 环境变量
- 任务变量
- 步骤提取变量
- 模板引用：`{{token}}`、`{{userId}}`

提取器：

- JSONPath
- Header
- Regex

断言：

- 状态码
- 响应耗时
- JSON 字段
- Header
- 字符串包含

错误分类：

- 连接错误
- 超时
- 断言失败
- 协议错误
- Adapter 错误

MVP 暂不做条件分支、循环、事务嵌套、复杂可视化编排。这些作为后续场景编排引擎能力。

## 6. 自定义 RPC 接入

MVP 使用外部 adapter 接入自定义 RPC。平台不理解业务 RPC 细节，只把统一请求转给 adapter。

```text
Executor
  |
  | adapter_rpc step
  v
RPC Adapter Service
  |
  | user-defined protocol
  v
Internal Service
```

平台侧请求示例：

```json
{
  "protocol": "adapter_rpc",
  "adapterUrl": "http://adapter:8080/invoke",
  "operation": "CreateOrder",
  "metadata": {
    "traceId": "{{traceId}}"
  },
  "payload": {
    "userId": "{{userId}}",
    "skuId": "sku-001"
  },
  "timeoutMs": 3000
}
```

Adapter 返回统一响应：

```json
{
  "success": true,
  "statusCode": "OK",
  "latencyMs": 35,
  "headers": {},
  "body": {
    "orderId": "123"
  },
  "error": null
}
```

平台继续基于统一响应做变量提取、断言、指标统计和错误采样。

后续扩展：

- Go SDK driver：适合高性能内部协议。
- gRPC / Dubbo / Thrift driver：按协议独立实现。
- 脚本步骤：用于前后置处理和轻量自定义调用。

## 7. 压测执行器与负载模型

控制面创建 TestRun，执行器拿到不可变 TaskSpec 后开始运行。

```text
Control Plane
  |
  | TaskSpec
  v
Executor Engine
  |
  | RunEvent stream
  v
Metrics Aggregator / WebSocket / Report
```

TaskSpec：

```text
TaskSpec
  run_id
  project_id
  scenario_id
  env_id
  targets[]
  scenario_snapshot
  load_profile
  runtime_config
  metric_config
  profiling_config
```

使用 `scenario_snapshot` 固化一次压测运行的配置。压测开始后，即使用户修改场景，也不会影响正在运行的任务，历史报告也可追溯。

MVP 支持三种负载模型：

```text
并发用户模式:
  concurrency = 100
  duration = 10m
  ramp_up = 2m

固定 QPS 模式:
  qps = 500
  duration = 10m
  ramp_up = 1m

阶梯加压模式:
  stages:
    - duration: 2m, concurrency: 50
    - duration: 2m, concurrency: 100
    - duration: 2m, concurrency: 200
```

执行器内部结构：

```text
Executor Engine
  |-- Run Controller
  |-- Load Scheduler
  |-- VU Runner
  |-- Protocol Dispatcher
  |-- Result Collector
  |-- Metrics Aggregator
  |-- Event Publisher
```

事件模型：

```text
RunEvent
  run_id
  type:
    - run_started
    - metrics_tick
    - step_metrics_tick
    - target_metrics_tick
    - profile_started
    - profile_finished
    - error_sample
    - slow_sample
    - run_finished
  timestamp
  payload
```

MVP 支持：

- 用户手动停止任务。
- 达到 duration 自动停止。
- HTTP / adapter 超时控制。
- Step 失败后按策略继续或终止当前 iteration。
- 错误和慢请求采样。
- 执行器异常时将任务标记为异常结束。

## 8. Target Agent 与目标机器状态采集

由于被压测服务没有接入 Prometheus，MVP 直接实现独立 Target Agent。

```text
Linux Target Host
  |
  | target-agent
  |-- system metrics
  |-- process metrics
  |-- port / health check
  |-- custom script metrics
  |-- profiling collector
  v
Go Backend Agent Gateway
```

MVP 先支持 Linux 主机。Docker 和 Kubernetes 预留 Target 抽象：

```text
Target
  type: host | container | kubernetes
  labels: env/service/region/team
  agent_id
  metric_config
  profiling_config
```

采集范围：

- 主机指标：CPU、内存、磁盘使用率、磁盘 IO、网络 IO。
- 进程指标：按 pid、进程名、端口、启动命令关键字匹配 CPU、内存、线程数、打开文件数。
- 服务健康：端口探测、HTTP health check。
- 自定义指标：用户配置受控 shell 命令，Agent 定时执行并解析 JSON 输出。
- Agent 状态：在线、离线、版本、hostname、IP、labels、最近心跳时间。

通信方式：

```text
Agent -> Backend: HTTP / gRPC 定时心跳和指标上报
Backend -> Agent: 通过心跳响应下发采集配置
```

后续升级为 WebSocket / gRPC stream，以支持实时下发配置、远程诊断和动态调整采集频率。

## 9. Profiling 能力

MVP 支持压测过程中分析线上进程 pprof / prof。该能力属于 Target Agent 的增强能力。

```text
压测任务运行中
  |
  | executor metrics: QPS / latency / error
  |
  | target-agent metrics: CPU / memory / IO / network
  |
  | target-agent profiler:
  |    - Go pprof
  |    - controlled command profiler
  v
报告时间线 + profile artifacts
```

第一版支持两类采集：

### 9.1 Go pprof 拉取模式

适合 Go 服务已经暴露 pprof endpoint。

```text
target profiling:
  type: go_pprof
  endpoint: http://127.0.0.1:6060/debug/pprof
  profiles:
    - cpu
    - heap
    - goroutine
  trigger:
    mode: during_run
    interval: 60s
    duration: 30s
```

Agent 支持采集：

- `/debug/pprof/profile`
- `/debug/pprof/heap`
- `/debug/pprof/goroutine`
- `/debug/pprof/mutex`
- `/debug/pprof/block`

### 9.2 受控命令采集模式

适合服务没有 HTTP pprof，或者需要接入 perf、自定义 prof 工具。

```text
type: command
process_match:
  name: order-service
commands:
  cpu: "perf record -F 99 -p {{pid}} -g -- sleep 30"
  dump: "custom-prof --pid {{pid}} --duration 30 --output {{output}}"
```

Agent 负责匹配进程、执行受控采集、压缩产物并上传后端。

触发策略：

- 手动采集。
- 固定间隔采集。
- 每个阶梯阶段采集一次。

阈值触发采集后续实现，例如 P99 超过阈值、错误率升高、CPU 超过 90% 时触发。

安全限制：

- profiling 默认关闭，任务级显式开启。
- 每次采集有最大时长，例如 30s 或 60s。
- 每个 run 限制最大 profile 文件数和大小。
- command profiler 只能执行平台允许的模板，不能任意执行用户输入命令。
- pprof endpoint、token、命令模板、进程匹配规则按项目隔离。
- profile artifact 作为敏感文件管理，支持过期清理。

报告展示：

```text
时间窗口: 10:30:00 - 10:30:30
压测状态:
  QPS: 3200
  P99: 680ms
  Target CPU: 96%
Profile:
  cpu.pprof
  heap.pprof
  goroutine.txt
  perf.data / flamegraph.svg
```

## 10. 数据模型与存储

MVP 使用 Postgres + Redis。

- Postgres：存用户、项目、环境、Target、Agent、场景、任务、报告、秒级指标和采样明细。
- Redis：存运行中任务状态、实时事件缓冲、短期订阅数据，并为后续分布式队列预留。
- Artifact Storage：本地目录起步，后续切换 MinIO / S3。

核心表：

```text
users
  id, username, password_hash, status, created_at

projects
  id, name, owner_id, created_at

project_members
  project_id, user_id, role

environments
  id, project_id, name, variables_json, created_at

targets
  id, project_id, env_id, name, type, labels_json, agent_id, status

agents
  id, project_id, token_hash, hostname, ip, labels_json,
  version, status, last_seen_at

scenarios
  id, project_id, name, description, version, status, created_at

scenario_versions
  id, scenario_id, version, config_json, created_by, created_at

test_tasks
  id, project_id, scenario_id, name, load_profile_json,
  runtime_config_json, profiling_config_json, status, created_by, created_at

test_runs
  id, task_id, project_id, scenario_version_id,
  status, started_at, finished_at, summary_json, snapshot_json
```

指标与报告表：

```text
run_metric_ticks
  id, run_id, timestamp,
  qps, success_qps, error_qps,
  avg_latency_ms, p50_ms, p90_ms, p95_ms, p99_ms,
  active_users

run_step_metric_ticks
  id, run_id, step_name, timestamp,
  qps, error_qps, avg_latency_ms, p95_ms, p99_ms

run_target_metric_ticks
  id, run_id, target_id, timestamp,
  cpu_usage, memory_usage, disk_read_bps, disk_write_bps,
  net_in_bps, net_out_bps, process_metrics_json

run_error_samples
  id, run_id, step_name, timestamp,
  error_type, message, request_sample_json, response_sample_json

run_slow_samples
  id, run_id, step_name, timestamp,
  latency_ms, request_sample_json, response_sample_json

run_profile_artifacts
  id, run_id, target_id, profile_type,
  trigger_type, started_at, finished_at,
  status, file_path, size_bytes, metadata_json
```

历史报告通过 `test_runs.snapshot_json` 保存运行快照，包括场景版本、环境变量、负载模型、Target、Agent 指标配置和 profiling 配置。

## 11. 后端 API 与内部接口

API 分组：

```text
/auth
/projects
/environments
/targets
/agents
/scenarios
/tasks
/runs
/reports
/profiles
```

主要接口：

```text
POST   /api/auth/login
GET    /api/projects
POST   /api/projects

GET    /api/projects/{projectId}/environments
POST   /api/projects/{projectId}/environments

GET    /api/projects/{projectId}/agents
POST   /api/projects/{projectId}/agents/register-token
GET    /api/projects/{projectId}/targets
POST   /api/projects/{projectId}/targets

GET    /api/projects/{projectId}/scenarios
POST   /api/projects/{projectId}/scenarios
PUT    /api/projects/{projectId}/scenarios/{scenarioId}
GET    /api/projects/{projectId}/scenarios/{scenarioId}/versions

POST   /api/projects/{projectId}/tasks
POST   /api/projects/{projectId}/tasks/{taskId}/runs
POST   /api/projects/{projectId}/runs/{runId}/stop
GET    /api/projects/{projectId}/runs/{runId}

GET    /api/projects/{projectId}/reports/{runId}
GET    /api/projects/{projectId}/profiles/{artifactId}/download
```

实时事件：

```text
GET /api/projects/{projectId}/runs/{runId}/events
```

Agent 接口：

```text
POST /agent/v1/heartbeat
POST /agent/v1/metrics
POST /agent/v1/profile-artifacts
GET  /agent/v1/config
```

内部服务接口示例：

```go
type ScenarioService interface {
    Create(ctx context.Context, input CreateScenarioInput) (*Scenario, error)
    CreateVersion(ctx context.Context, scenarioID string, config ScenarioConfig) (*ScenarioVersion, error)
}

type RunService interface {
    StartRun(ctx context.Context, taskID string) (*TestRun, error)
    StopRun(ctx context.Context, runID string) error
    GetReport(ctx context.Context, runID string) (*RunReport, error)
}

type AgentService interface {
    Heartbeat(ctx context.Context, req AgentHeartbeat) (*AgentConfig, error)
    IngestMetrics(ctx context.Context, batch TargetMetricBatch) error
    SaveProfileArtifact(ctx context.Context, artifact ProfileArtifact) error
}
```

协议 driver 接口：

```go
type ProtocolDriver interface {
    Name() string
    Execute(ctx context.Context, req StepRequest) (*StepResult, error)
}
```

执行器接口：

```go
type Executor interface {
    Start(ctx context.Context, task TaskSpec) (RunID, error)
    Stop(ctx context.Context, runID RunID) error
    Subscribe(runID RunID) (<-chan RunEvent, error)
}
```

## 12. 前端页面与交互

React 前端采用工程控制台风格，默认进入项目工作台。

```text
Login
  |
Project Workspace
  |-- Dashboard
  |-- Environments
  |-- Targets & Agents
  |-- Scenarios
  |-- Test Tasks
  |-- Running Monitor
  |-- Reports
  |-- Settings
```

页面职责：

- Dashboard：最近任务、运行中压测、Agent 在线状态、最近报告。
- Environments：环境变量、全局请求头、凭证引用、adapter 地址。
- Targets & Agents：Agent 注册、Target 绑定、进程匹配、健康检查、profiling 配置。
- Scenarios：表单式多步骤编辑，支持 HTTP 和 Adapter RPC。
- Test Tasks：选择场景、环境、Target、负载模型、运行时配置和 profiling 配置。
- Running Monitor：实时展示 QPS、延迟、错误率、Step 指标、Target 指标和 profiling 状态。
- Reports：展示摘要、时间线、Step 分析、错误样本、慢请求样本、Target 指标和 profile artifact。

前端技术：

```text
React + TypeScript + Vite
React Router
TanStack Query
Zustand
Ant Design
ECharts
Monaco Editor
WebSocket
```

## 13. 技术选型

后端：

```text
Go
Web framework: chi
DB: Postgres
Cache / stream: Redis
DB access: pgx + sqlc
Migration: goose
Auth: JWT + bcrypt
Metrics aggregation: 内存聚合 + 秒级落库
Artifact storage: 本地目录起步，后续 MinIO / S3
```

前端：

```text
React + TypeScript + Vite
Ant Design
TanStack Query
Zustand
ECharts
Monaco Editor
```

Agent：

```text
Go agent
gopsutil: 主机和进程指标
net/http: health check 和 pprof 拉取
archive/zip 或 tar.gz: profile artifact 打包
systemd service: Linux 安装运行
```

## 14. MVP 范围

MVP 必做：

- 基础登录和项目隔离。
- 环境变量管理。
- Linux Target Agent。
- Agent 注册、心跳、在线状态。
- 主机指标、进程指标、HTTP health check。
- Go pprof 拉取。
- 受控 command profiler。
- HTTP step。
- Adapter RPC step。
- 多步骤顺序执行。
- 变量引用、提取器、基础断言。
- 并发用户模式、固定 QPS 模式、阶梯加压模式。
- 压测启动、停止和异常结束。
- 秒级指标聚合。
- WebSocket 实时监控。
- 错误样本和慢请求样本。
- Profile artifact 上传、下载和报告关联。

MVP 暂缓：

- 分布式 worker。
- Kubernetes Job 执行器。
- Go SDK 协议插件。
- 可视化流程编排画布。
- 细粒度 RBAC。
- Prometheus / Grafana 集成。
- 自动阈值触发 profiling。
- Java async-profiler、Node profiler、Python profiler 等语言专用 profiler。

## 15. 实施顺序

```text
Phase 1: 后端基础
  Go API 服务、Postgres、Redis、项目、用户、环境

Phase 2: Agent MVP
  Agent 注册、心跳、系统指标、进程匹配、Target 绑定

Phase 3: 场景模型
  Scenario version、HTTP step、Adapter RPC step、变量、提取器、断言

Phase 4: Executor MVP
  TaskSpec、load profile、HTTP driver、adapter driver、指标聚合

Phase 5: 实时监控
  WebSocket、运行中任务状态、实时图表数据

Phase 6: Profiling
  Go pprof 采集、command profiler、artifact 上传和下载

Phase 7: 报告
  Summary、时间线、Target metrics、error samples、slow samples、profile artifacts

Phase 8: 前端完善
  项目工作台、场景编辑器、任务配置、实时监控、报告页面
```

## 16. 风险与约束

Agent 安全：

- Agent 使用 token 鉴权。
- Agent 数据必须按项目隔离。
- 上传指标和 artifact 要限制大小和频率。
- command profiler 只能执行受控模板。

Profiling 对线上进程影响：

- profiling 默认关闭。
- 每次采集限制最大时长。
- 每个 run 限制最大采集次数和文件大小。
- 报告中记录每次采集窗口，便于追溯性能影响。

指标数据量：

- 每个请求不直接落库。
- 执行器秒级聚合后落库。
- 错误和慢请求只采样。
- Profile artifact 只存元数据，文件走 artifact storage。

Adapter 稳定性：

- Adapter 必须配置超时。
- 报告区分 adapter 错误和业务服务错误。
- Adapter RPC step 要支持健康检查和错误分类。

后续扩展：

- Executor、AgentGateway、Scheduler 都按接口拆分。
- MVP 单进程运行，后续可以平滑拆分为独立 worker 和 gateway。
- Target 抽象不绑定 Linux 主机，预留 Docker 和 Kubernetes。

## 17. 验收标准

第一版达到以下标准即可进入内部试用：

- 用户可以登录并创建项目。
- 用户可以注册 Linux Agent，并在页面看到在线状态。
- 用户可以将 Target 绑定到 Agent，并配置进程匹配和 health check。
- 用户可以创建包含 HTTP 和 Adapter RPC step 的多步骤场景。
- 用户可以配置并发、固定 QPS 或阶梯加压任务。
- 用户可以启动和停止压测任务。
- 用户可以在运行中看到 QPS、P95、P99、错误率、目标机器 CPU、内存、网络和进程指标。
- 用户可以在压测中采集 Go pprof 或受控 command profiler 产物。
- 用户可以在报告中查看压测摘要、指标时间线、错误样本、慢请求样本和 profile artifact。
- 历史报告可以追溯当时的场景快照、环境变量、Target、Agent 和 profiling 配置。
