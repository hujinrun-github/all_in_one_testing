# All-in-One Testing 新手入门指南

本指南帮助你快速上手 All-in-One Testing 压测平台，从环境搭建到完成第一次压测。

## 目录

- [1. 平台概述](#1-平台概述)
- [2. 环境搭建](#2-环境搭建)
- [3. 前端页面操作指南](#3-前端页面操作指南)
- [4. 完整压测流程](#4-完整压测流程)
- [5. 常见问题](#5-常见问题)

---

## 1. 平台概述

All-in-One Testing 是一个通用压测平台，支持：

- **HTTP 压测**：对 HTTP/HTTPS 接口发起并发请求
- **CUSTOM_RPC 协议**：通过适配器支持 gRPC、Dubbo 等协议
- **Agent 采集**：在被压测机器上采集主机指标、进程指标和 pprof/perf 产物
- **报告分析**：展示 QPS、延迟、错误率、机器指标和性能画像

### 核心概念

| 概念 | 说明 |
|------|------|
| **Scenario（场景）** | 描述一次压测的请求配置，包括 URL、方法、Headers、Body 等 |
| **Target（目标）** | 被压测的服务，可配置健康检查、告警阈值和 pprof 端点 |
| **Agent** | 部署在被压测机器上的采集进程，负责上报指标和采集 profile |
| **Run** | 一次实际压测执行，包含场景、并发数、总请求数等配置 |
| **Report** | 压测结果报告，包含性能指标、告警和 profile 产物 |

---

## 2. 环境搭建

### 2.1 环境要求

- **Go 1.22+**（后端）
- **Node.js 20+**（前端）
- **npm**

### 2.2 启动后端服务

```bash
cd backend
go run ./cmd/server
```

后端默认监听 `http://127.0.0.1:8080`

验证后端是否正常：

```bash
curl http://127.0.0.1:8080/api/health
# 预期返回：{"status":"ok"}
```

### 2.3 启动前端服务

```bash
cd frontend
npm install    # 首次需要安装依赖
npm run dev
```

前端默认访问 `http://127.0.0.1:5173`

开发服务会自动将 `/api` 请求代理到后端 8080 端口。

### 2.4 验证安装

打开浏览器访问 `http://127.0.0.1:5173`，应该能看到平台控制台界面。

---

## 3. 前端页面操作指南

平台前端包含以下页面：

| 页面 | 功能 |
|------|------|
| **Dashboard** | 平台总览，显示运行状态、Agent 健康、近期运行 |
| **Targets & Agents** | 管理被压测目标和 Agent |
| **Scenarios** | 创建和管理压测场景 |
| **Runs** | 发起压测、查看运行历史 |
| **Reports** | 查看压测报告、告警和 profile 产物 |

### 3.1 顶栏操作

顶栏提供以下功能：

- **Project**：项目 ID，默认为 `default`
- **Environment**：环境，默认为 `default`
- **Apply scope**：应用当前项目/环境过滤，后续所有列表查询都会按此范围过滤

### 3.2 Targets & Agents 页面

#### Agent Token 管理

1. 填写 Project ID 和 Environment
2. 点击 **Generate token** 生成 Agent token
3. 复制 token 和安装命令，用于在被压测机器上安装 Agent

#### Agent 安装命令

生成 token 后，页面会显示一键安装命令：

```bash
curl -fsSL http://control-plane:8080/agent/install.sh | sudo bash -s -- \
  --server http://control-plane:8080 \
  --token <agent-token> \
  --name checkout-01
```

#### Target 配置

在 Target binding 面板中：

1. 填写 Target 名称
2. 配置 Base URL（被压测服务地址）
3. 配置 pprof endpoint（可选，用于采集 CPU profile）
4. 配置进程匹配规则（进程名和命令行关键字）
5. 配置健康检查（路径、期望状态码、超时时间）
6. 配置告警阈值（CPU、内存、磁盘、网络上限）
7. 点击 **Bind agent** 绑定已在线的 Agent

#### 画像模板（Profile templates）

用于配置 Agent 采集 pprof/perf 的模板：

- **Go pprof**：采集 Go 服务的 CPU、heap、goroutine 等 profile
- **Linux perf**：采集 Linux 系统的 perf.data

选择模板后，填写 profile 类型和采集时长，点击 **采集 / Profile** 下发采集任务。

### 3.3 Scenarios 页面

#### 创建 HTTP 场景

1. 填写场景名称
2. 选择协议（HTTP）和方法（GET/POST）
3. 配置 Base URL 和请求路径
4. 添加 Headers（可选）
5. 添加 Query variants（查询参数候选，按权重随机选择）
6. 添加 Body variants（POST 请求体候选，按权重随机选择）
7. 配置超时时间和重试次数
8. 配置成功断言（如 `status < 400`）
9. 点击 **Save** 保存场景

#### 场景表单字段说明

| 字段 | 说明 | 示例 |
|------|------|------|
| Scenario name | 场景名称 | `payment-http-smoke` |
| Project ID | 项目 ID | `project-checkout` |
| Environment | 环境 | `staging` |
| Protocol | 协议 | `HTTP` |
| Method | HTTP 方法 | `GET`、`POST` |
| Base URL | 服务地址 | `https://api.example.com` |
| Request path | 请求路径 | `/api/payments` |
| Query variants | 查询参数候选 | `80% tenant=shanghai`、`20% tenant=beijing` |
| Headers | 请求头 | `Authorization: Bearer xxx` |
| Body variants | 请求体候选 | `70% {"amount":100}`、`30% {"amount":300}` |
| Timeout | 超时时间（ms） | `2500` |
| Retry count | 重试次数 | `2` |
| Success assertion | 成功断言 | `status < 400` |

#### Flow Steps（高级）

场景支持多步骤编排：

- **HTTP request**：发送 HTTP 请求
- **Extract**：从响应中提取变量（支持 JSON/Header/Regex）
- **Assertion**：响应断言（状态码、JSON/Header/Body 内容校验）

变量提取后可在后续步骤中通过 `${variableName}` 引用。

### 3.4 Runs 页面

#### 发起压测

1. 从下拉列表选择已保存的场景
2. 选择目标 Target（可选）
3. 配置压测参数：
   - **Total requests**：总请求数
   - **Concurrency**：并发数
   - **Timeout**：单请求超时时间
4. 配置保护阈值（可选）：
   - **Max error rate %**：超过此错误率自动停止
   - **Max p95 latency ms**：超过此延迟自动停止
5. 点击 **Start run** 启动压测

#### 查看运行状态

- 页面会实时更新压测进度
- 可以查看当前 QPS、成功/失败请求数
- 点击 **Stop** 可以手动停止压测

#### 运行历史

- 页面下方显示历史运行记录
- 点击记录可跳转到 Reports 页面查看详情

### 3.5 Reports 页面

#### 报告内容

- **Run summary**：运行汇总（成功率、错误率、QPS、p95 延迟）
- **Alerts**：告警摘要（guardrail 告警、target_metric 告警）
- **Target metrics**：被压测机器的 CPU/内存/磁盘/网络指标趋势
- **Process trends**：目标进程的 CPU/RSS/fd/thread 趋势
- **Profile artifacts**：采集的 pprof/perf 文件列表和下载

#### 多 Run 对比

页面会自动对比最近多次 Run 的：
- 成功率、错误率
- QPS、p95 延迟
- 标出 best qps、fastest p95、highest errors

---

## 4. 完整压测流程

以下是端到端的压测流程示例：

### 步骤 1：创建场景

1. 进入 **Scenarios** 页面
2. 填写场景配置：
   ```
   Name: demo-health-check
   Protocol: HTTP
   Method: GET
   Base URL: http://127.0.0.1:8080
   Path: /api/health
   Total requests: 100
   Concurrency: 10
   Success assertion: status < 400
   ```
3. 点击 **Save** 保存场景

### 步骤 2：发起压测

1. 进入 **Runs** 页面
2. 选择刚才创建的场景
3. 确认压测参数
4. 点击 **Start run**

### 步骤 3：查看报告

1. 压测完成后自动跳转到 Reports 页面
2. 查看：
   - 运行汇总
   - QPS 和延迟趋势
   - 慢请求和错误样本

### 步骤 4：（可选）接入 Agent

如果需要采集被压测机器的指标和 profile：

1. 进入 **Targets & Agents** 页面
2. 生成 Agent token
3. 在被压测机器上执行安装命令
4. 创建 Target 并绑定 Agent
5. 发起压测时选择 Target
6. Reports 页面会展示机器指标和 profile 产物

---

## 5. 常见问题

### Q: 前端页面没有变化？

**A:** 尝试以下方法：
1. 强制刷新：`Ctrl+Shift+R` (Windows/Linux) 或 `Cmd+Shift+R` (Mac)
2. 清除浏览器缓存后刷新
3. 重启前端服务：在运行 `npm run dev` 的终端按 `Ctrl+C` 停止，然后重新运行

### Q: 无法连接后端？

**A:** 检查以下内容：
1. 后端是否已启动：`curl http://127.0.0.1:8080/api/health`
2. 前端代理配置是否正确（`frontend/vite.config.ts`）

### Q: 场景保存后刷新就没了？

**A:** 场景会保存到后端 SQLite 数据库。如果刷新后看不到：
1. 确认后端服务正常运行
2. 检查浏览器控制台是否有错误信息

### Q: Agent 显示离线？

**A:** 检查：
1. Agent 进程是否在运行：`systemctl status all-in-one-agent`
2. Agent 是否使用正确的 token
3. Agent 是否能访问 Control Plane：`curl http://control-plane:8080/api/health`

### Q: 压测失败，提示 "target health preflight failed"？

**A:** Target 健康检查失败，可能原因：
1. Target 服务未启动
2. 健康检查路径配置错误
3. 期望状态码配置错误

### Q: 如何下载 pprof 文件？

**A:** 在 Reports 页面：
1. 找到 Profile artifacts 区域
2. 点击 artifact 旁边的 **Download** 按钮

### Q: 如何配置 Agent 采集 perf 数据？

**A:** 在 Targets & Agents 页面：
1. 选择 Profile templates 中的 **Linux perf** 模板
2. 填写目标进程 PID
3. 配置采集时长（1-300 秒）
4. 点击 **采集 / Profile**

---

## 附录：API 快速参考

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/health` | GET | 健康检查 |
| `/api/scenarios` | GET/POST | 场景列表/创建 |
| `/api/scenarios/{id}` | GET/PUT/DELETE | 场景详情/更新/删除 |
| `/api/targets` | GET/POST | Target 列表/创建 |
| `/api/targets/{id}` | GET/PUT/DELETE | Target 详情/更新/删除 |
| `/api/runs` | GET/POST | 运行历史/创建运行 |
| `/api/runs/{id}` | GET | 运行详情 |
| `/api/runs/{id}/events` | GET | 运行事件 |
| `/api/reports/runs/{id}` | GET | 运行报告 |
| `/api/agents` | GET | Agent 列表 |
| `/api/agent-tokens` | POST | 创建 Agent token |
| `/api/profile-artifacts` | GET | Profile 产物列表 |
| `/api/profile-artifacts/{id}/download` | GET | 下载 Profile 产物 |

---

更多详细信息请参考：
- [平台使用手册](./usage-guide.md)
- [Workspace Scope 说明](./workspace-scope.md)
- [Run Events 说明](./run-events.md)
- [Run Guardrails 说明](./run-guardrails.md)
