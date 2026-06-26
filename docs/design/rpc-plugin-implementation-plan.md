# RPC 插件化执行计划

## 执行概览

| 阶段 | 内容 | 预估时间 | 优先级 |
|------|------|----------|--------|
| Phase 1 | 后端基础框架 | 2 小时 | P0 |
| Phase 2 | HTTP Adapter 集成 | 1.5 小时 | P0 |
| Phase 3 | 前端适配 | 1 小时 | P1 |
| Phase 4 | 测试与文档 | 1 小时 | P1 |

**总计**: 约 5.5 小时

---

## Phase 1: 后端基础框架 (P0)

### 1.1 创建 RPC 包目录结构

```
backend/internal/rpc/
├── plugin.go       # Plugin 接口和 PluginManager
├── errors.go       # 错误码定义
├── grpc_plugin.go  # gRPC 插件实现
└── http_adapter.go # HTTP Adapter 客户端
```

**任务**: 创建目录和文件骨架

### 1.2 实现 Plugin 接口和 PluginManager

**文件**: `backend/internal/rpc/plugin.go`

**任务清单**:
- [ ] 定义 `Plugin` 接口（Name, Description, Version, ConfigSchema, ValidateConfig, Execute）
- [ ] 定义 `PluginConfigSchema`, `ConfigField` 结构
- [ ] 定义 `PluginRequest`, `PluginResponse` 结构
- [ ] 实现 `PluginManager` 结构和方法（Register, Unregister, Get, List, HasPlugin, EnablePlugin, DisablePlugin）
- [ ] 创建全局 `DefaultManager` 实例

**代码位置**: 新建 `backend/internal/rpc/plugin.go`

### 1.3 实现错误码定义

**文件**: `backend/internal/rpc/errors.go`

**任务清单**:
- [ ] 定义错误码常量（4001-5000）
- [ ] 定义 `RPCError` 结构
- [ ] 实现 `NewRPCError`, `NewPluginNotFoundError`, `NewGRPCDirectCallNotSupportedError`, `NewAdapterNotConfiguredError` 函数
- [ ] 定义错误消息映射

**代码位置**: 新建 `backend/internal/rpc/errors.go`

### 1.4 修改协议验证

**文件**: `backend/internal/api/runs.go`

**任务清单**:
- [ ] 添加 `supportedProtocols` map（HTTP, CUSTOM_RPC, gRPC, Dubbo, Thrift）
- [ ] 修改 `validateCreateRunRequest` 支持新协议验证
- [ ] 添加 `validateGRPCRequest` 函数验证 gRPC 必需字段

**代码位置**: `backend/internal/api/runs.go` 约第 626-631 行

### 1.5 集成到 Run Executor

**文件**: `backend/internal/api/runs.go`

**任务清单**:
- [ ] 添加 `import "all-in-one-testing/internal/rpc"`
- [ ] 修改 `executeRunSample` 函数支持插件路由
- [ ] 实现 `executePluginSample` 函数
- [ ] 实现 `isGRPCDirectCallNotSupported` 辅助函数
- [ ] 实现 `executeHTTPAdapterForGRPC` 函数

**代码位置**: `backend/internal/api/runs.go` 的 `executeRunSample` 函数

---

## Phase 2: HTTP Adapter 集成 (P0)

### 2.1 实现 HTTPAdapterClient

**文件**: `backend/internal/rpc/http_adapter.go`

**任务清单**:
- [ ] 定义 `HTTPAdapterClient` 结构
- [ ] 定义 `AdapterConfig`, `AdapterRequest`, `AdapterResponse` 结构
- [ ] 实现 `NewHTTPAdapterClient` 构造函数
- [ ] 实现 `Execute` 方法（带重试逻辑）
- [ ] 实现 `doExecute` 方法（单次 HTTP 调用）
- [ ] 实现 `shouldRetry` 方法（判断是否重试）

**代码位置**: 新建 `backend/internal/rpc/http_adapter.go`

### 2.2 实现 gRPC 插件

**文件**: `backend/internal/rpc/grpc_plugin.go`

**任务清单**:
- [ ] 定义 `gRPCPlugin` 结构
- [ ] 实现 Plugin 接口所有方法
- [ ] 实现 `PluginConfigurable` 接口（SetEnabled, IsEnabled）
- [ ] 实现 `ConfigSchema` 返回配置定义
- [ ] 实现 `ValidateConfig` 验证 X-GRPC-Target
- [ ] 实现 `Execute` 方法（解析配置、建立连接、调用 invokeViaGateway）
- [ ] 实现 `parseConfig` 方法
- [ ] 实现 `invokeViaGateway` 返回明确错误信息
- [ ] 在 `init()` 中注册插件

**代码位置**: 新建 `backend/internal/rpc/grpc_plugin.go`

### 2.3 初始化 Adapter 客户端

**文件**: `backend/internal/api/runs.go`

**任务清单**:
- [ ] 添加全局 `adapterClient` 变量
- [ ] 实现 `initAdapter` 函数
- [ ] 在应用启动时调用 `initAdapter`

**代码位置**: `backend/internal/api/runs.go`

---

## Phase 3: 前端适配 (P1)

### 3.1 更新协议选项

**文件**: `frontend/src/app/App.tsx`

**任务清单**:
- [ ] 找到 `protocolOptions` 常量定义
- [ ] 添加 gRPC, Dubbo, Thrift 协议选项
- [ ] 添加协议描述信息

**代码位置**: `frontend/src/app/App.tsx` 搜索 `protocolOptions`

### 3.2 更新协议验证函数

**文件**: `frontend/src/app/App.tsx` 或相关组件

**任务清单**:
- [ ] 找到 `isExecutableScenarioFlowStep` 函数
- [ ] 更新函数支持 gRPC, Dubbo, Thrift 协议
- [ ] 添加 gRPC 协议的 X-GRPC-Target 验证

**代码位置**: `frontend/src/app/App.tsx` 搜索 `isExecutableScenarioFlowStep`

---

## Phase 4: 测试与文档 (P1)

### 4.1 单元测试

**任务清单**:
- [ ] 编写 `plugin_test.go` 测试 PluginManager 功能
- [ ] 编写 `errors_test.go` 测试错误码
- [ ] 编写 `http_adapter_test.go` 测试 HTTP Adapter 客户端
- [ ] 编写 `grpc_plugin_test.go` 测试 gRPC 插件

**代码位置**: `backend/internal/rpc/*_test.go`

### 4.2 集成测试

**任务清单**:
- [ ] 测试 HTTP 协议场景执行
- [ ] 测试 CUSTOM_RPC 协议场景执行
- [ ] 测试 gRPC 协议场景执行（需要 grpc-gateway）
- [ ] 测试协议验证错误处理

### 4.3 更新文档

**任务清单**:
- [ ] 更新 `docs/usage-guide.md` 添加 RPC 插件使用说明
- [ ] 更新 `docs/getting-started.md` 添加 gRPC Adapter 配置说明

---

## 依赖检查

### Go 依赖

执行前检查是否需要添加依赖：

```bash
cd backend
go mod tidy
go mod graph | grep grpc
```

如果缺少 gRPC 依赖，添加：

```bash
go get google.golang.org/grpc
go get google.golang.org/protobuf
```

---

## 执行顺序

```
1. Phase 1.1 → 创建目录结构
       ↓
2. Phase 1.2 → 实现 Plugin 接口和 PluginManager
       ↓
3. Phase 1.3 → 实现错误码定义
       ↓
4. Phase 1.4 → 修改协议验证
       ↓
5. Phase 1.5 → 集成到 Run Executor
       ↓
6. Phase 2.1 → 实现 HTTPAdapterClient
       ↓
7. Phase 2.2 → 实现 gRPC 插件
       ↓
8. Phase 2.3 → 初始化 Adapter 客户端
       ↓
9. Phase 3.1 → 更新协议选项
       ↓
10. Phase 3.2 → 更新协议验证函数
       ↓
11. Phase 4 → 测试与文档
```

---

## 验证检查点

| 检查点 | 验证内容 | 预期结果 |
|--------|----------|----------|
| CP1 | Plugin 接口编译通过 | `go build ./internal/rpc` 无错误 |
| CP2 | Protocol 验证通过 | `gRPC` 协议可创建场景 |
| CP3 | HTTP Adapter 调用成功 | CUSTOM_RPC 场景可执行 |
| CP4 | gRPC 插件返回正确错误 | 返回 "gRPC direct call not supported" |
| CP5 | 前端协议选项显示正确 | 下拉框显示所有协议 |

---

## 回滚方案

如果实现过程中发现问题，可以回滚：

1. **Git 回滚**:
   ```bash
   git checkout -- backend/internal/rpc/
   git checkout -- backend/internal/api/runs.go
   ```

2. **编译验证**:
   ```bash
   go build ./...
   ```

3. **测试验证**:
   ```bash
   go test ./internal/rpc/...
   ```

---

## 开始执行

准备好后，按以下命令开始：

```bash
# 1. 创建目录
mkdir -p backend/internal/rpc

# 2. 编写代码（按 Phase 顺序）

# 3. 编译验证
cd backend && go build ./...

# 4. 运行测试
go test ./internal/rpc/...

# 5. 启动服务测试
go run cmd/main.go
```
