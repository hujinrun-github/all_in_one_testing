# All-in-One Testing 使用文档

完整平台使用手册见：[docs/usage-guide.md](docs/usage-guide.md)。

All-in-One Testing 是一个通用压测平台项目，目标是支持 HTTP、自定义 RPC 等多种协议，并通过 Target Agent 在压测过程中采集被压测机器状态、进程指标和 pprof / prof 产物。

当前仓库处于基础原型阶段：

- 后端已实现 Go API 服务骨架和 `/api/health` 健康检查接口。
- 后端已实现最小 HTTP 压测接口 `/api/runs`，可对一个 HTTP / HTTPS URL 或已保存场景发起同步压测并返回汇总指标。
- 后端已实现场景接口 `/api/scenarios`，使用 SQLite 做原型阶段持久化。
- 前端已实现 React + Ant Design 控制台原型，包含 Dashboard、Targets & Agents、Scenarios、Runs、Reports 页面；Runs 页面可选择已保存场景发起真实压测。
- Agent、报告落库、pprof 采集等能力仍在设计规划中，尚未接入真实后端数据。

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
- 主机状态采集项
- Target 绑定和 pprof endpoint

后续会接入真实 Target Agent 注册、心跳、主机指标和进程指标。

### Scenarios

展示场景编排入口：

- 调用链编排
- HTTP step
- 自定义 RPC / Adapter step
- 变量提取
- 断言配置
- 创建场景并通过 `/api/scenarios` 保存到后端 SQLite 数据库

后续会支持多步骤编排、版本化和更完整的变量提取。

### Runs

展示压测执行控制台：

- 运行控制
- 选择已保存场景并发起真实压测
- 总请求数、并发、超时配置
- 运行汇总结果
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

后续会关联真实压测结果、Target 指标、错误样本、慢请求样本和 pprof / prof 文件。

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

### POST `/api/runs`

用途：发起一次最小 HTTP 压测。该接口当前是同步执行：请求会等压测完成后返回汇总结果。

支持两种模式：

- 直接 URL 模式：传 `method`、`url`、`totalRequests`、`concurrency` 等字段。
- 场景模式：传 `scenarioId`、`totalRequests`、`concurrency`，后端会读取 SQLite 中保存的场景快照，并执行场景里的 headers、queryVariants 和 bodyVariants。

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
  "p95LatencyMs": 5.62
}
```

当前限制：

- 只支持直接压测单个 HTTP / HTTPS URL。
- 暂不保存历史运行记录。
- 暂不支持异步启动、停止、实时事件流。
- 已支持 queryVariants 按权重随机追加查询参数、bodyVariants 按权重随机选择请求体，并支持场景中的 Header KV。
- 暂不支持 Cookie、变量、复杂断言 DSL 和多步骤场景。
- 暂不关联 Target Agent、机器指标或 pprof / prof。

## 后续规划

完整平台目标见：

```text
docs/superpowers/specs/2026-06-07-load-testing-platform-design.md
```

后续核心里程碑：

1. 项目、环境、Target、Agent 数据模型。
2. Linux Target Agent：心跳、主机指标、进程指标。
3. Scenario：HTTP step、自定义 RPC step、变量、提取器、断言。
4. Executor：并发、固定 QPS、阶梯加压。
5. 实时监控：WebSocket 事件流和秒级指标。
6. Profiling：Go pprof 拉取和受控 command profiler。
7. Reports：指标时间线、错误样本、慢请求样本、profile artifacts。

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
