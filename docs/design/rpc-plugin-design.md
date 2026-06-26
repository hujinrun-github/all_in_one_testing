# RPC 插件化设计方案 (v4 - 经第 2 轮审核优化)

## 1. 背景

当前 `CUSTOM_RPC` 协议通过 HTTP 调用外部 Adapter 服务来实现 RPC 协议支持。这种方式存在以下问题：

- 需要额外部署 Adapter 服务
- 网络开销和延迟
- 配置和维护成本较高

## 2. 设计目标

- 支持本地插件机制，零网络开销
- 保持与现有 HTTP Adapter 的兼容性（复用现有 `executeCustomRPCSample`）
- 插件可独立开发、注册、卸载
- 支持动态发现和配置化管理
- **类型安全**：支持协议特定的配置结构
- **健壮性**：支持重试机制、超时控制、错误恢复

## 3. 方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| **当前：HTTP Adapter** | 解耦、可独立部署、语言无关 | 需要额外部署服务、网络开销、配置复杂 |
| **插件化：本地加载** | 零网络开销、部署简单、类型安全、性能更好 | 需要 Go 编写插件、与平台版本耦合 |
| **混合方案 (推荐)** | 兼顾两者优点 | 实现复杂度稍高 |

## 4. 推荐方案：混合方案

同时支持本地插件和 HTTP Adapter，用户可选择使用方式。

### 4.1 架构设计

```
┌─────────────────────────────────────────────────────────┐
│                      Scenario                            │
│  protocol: "gRPC" | "Dubbo" | "CUSTOM_RPC" | "HTTP"     │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    Run Executor                          │
│                                                          │
│  ┌─────────────────┐    ┌─────────────────────────┐    │
│  │  Protocol Router │───▶│  Plugin Manager          │    │
│  │  (本地插件优先)    │    │  - 本地插件注册表         │    │
│  │                  │    │  - HTTP Adapter (复用)   │    │
│  └─────────────────┘    └─────────────────────────┘    │
│                           │                               │
│         ┌─────────────────┼─────────────────┐            │
│         ▼                 ▼                 ▼            │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   │
│  │ gRPC Plugin │   │ Dubbo Plugin│   │ HTTP Adapter│   │
│  │ (本地执行)   │   │ (本地执行)   │   │ (远程调用)   │   │
│  └─────────────┘   └─────────────┘   └─────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 4.2 协议验证扩展

修改 `validateCreateRunRequest` 以支持新协议：

```go
// backend/internal/api/runs.go

// 支持的协议列表
var supportedProtocols = map[string]bool{
    "HTTP":       true,
    "CUSTOM_RPC": true,
    "gRPC":       true,  // 新增：gRPC 插件
    "Dubbo":      true,  // 新增：Dubbo 插件（暂未实现）
    "Thrift":     true,  // 新增：Thrift 插件（暂未实现）
}

func validateCreateRunRequest(input createRunRequest) error {
    // ... 现有验证逻辑 ...
    
    // 协议验证
    if input.Protocol != "" && !supportedProtocols[input.Protocol] {
        return fmt.Errorf("unsupported protocol: %s", input.Protocol)
    }
    
    // gRPC 协议特定验证
    if strings.EqualFold(input.Protocol, "gRPC") {
        if err := validateGRPCRequest(input); err != nil {
            return err
        }
    }
    
    return nil
}

// validateGRPCRequest 验证 gRPC 请求的必需字段
func validateGRPCRequest(input createRunRequest) error {
    headers := runHeadersToMap(input.Headers)
    if headers["X-GRPC-Target"] == "" {
        return fmt.Errorf("gRPC protocol requires X-GRPC-Target header")
    }
    return nil
}
```

### 4.3 核心接口定义（类型安全版本）

```go
// backend/internal/rpc/plugin.go
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Plugin 是 RPC 插件接口
type Plugin interface {
	// Name 返回插件名称（唯一标识）
	Name() string
	
	// Description 返回插件描述
	Description() string
	
	// Version 返回插件版本
	Version() string
	
	// ConfigSchema 返回插件的配置 Schema（JSON Schema 格式）
	ConfigSchema() *PluginConfigSchema
	
	// ValidateConfig 验证插件配置是否有效
	ValidateConfig(config map[string]any) error
	
	// Execute 执行 RPC 调用
	Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error)
}

// PluginConfigSchema 插件配置 Schema
type PluginConfigSchema struct {
	Required []string                 `json:"required,omitempty"` // 必需的配置项
	Properties map[string]ConfigField `json:"properties"`         // 配置项定义
}

// ConfigField 配置字段定义
type ConfigField struct {
	Type        string   `json:"type"`        // "string" | "number" | "boolean" | "object"
	Description string   `json:"description"` // 字段描述
	Default     any      `json:"default,omitempty"` // 默认值
	Enum        []string `json:"enum,omitempty"`    // 可选值列表
}

// PluginRequest 是统一的 RPC 插件请求
type PluginRequest struct {
	// RPC 方法名，如 "checkout.OrderService/CreateOrder"
	Method string `json:"method"`
	
	// 请求头（插件特定配置通过自定义 Header 传递）
	Headers map[string]string `json:"headers,omitempty"`
	
	// 查询参数（已编码的字符串）
	QueryParams string `json:"queryParams,omitempty"`
	
	// 请求体（使用 any 类型以支持任意 JSON 结构）
	// 说明：虽然使用 any 类型，但各插件可通过 ConfigSchema 验证必需字段
	// 对于强类型协议（如 gRPC），建议通过 HTTP Adapter + grpc-gateway 使用
	Body any `json:"body,omitempty"`
	
	// 运行时上下文
	RunID       string `json:"runId,omitempty"`
	ScenarioID  string `json:"scenarioId,omitempty"`
	TargetID    string `json:"targetId,omitempty"`
	TargetName  string `json:"targetName,omitempty"`
	
	// 超时设置（毫秒），0 表示使用默认值
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// PluginResponse 是统一的 RPC 插件响应
type PluginResponse struct {
	// 调用是否成功
	Success bool `json:"success"`
	
	// 业务状态码（用于断言判断，0 表示无状态码）
	StatusCode int `json:"statusCode"`
	
	// 错误信息（Success=false 时填写）
	Error string `json:"error,omitempty"`
	
	// 响应数据
	Data any `json:"data,omitempty"`
	
	// 原始响应头（可选）
	Headers map[string]string `json:"headers,omitempty"`
	
	// 执行耗时（毫秒）
	LatencyMs int64 `json:"latencyMs,omitempty"`
}

// PluginManager 管理所有插件
type PluginManager struct {
	plugins   map[string]Plugin
	adapters   map[string]string // protocol -> adapter URL
	mu        sync.RWMutex
}

// NewPluginManager 创建插件管理器
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins:  make(map[string]Plugin),
		adapters: make(map[string]string),
	}
}

// Register 注册插件（显式注册，推荐方式）
func (m *PluginManager) Register(plugin Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[plugin.Name()] = plugin
}

// Unregister 注销插件
func (m *PluginManager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.plugins, name)
}

// SetAdapter 设置协议对应的 HTTP Adapter URL
func (m *PluginManager) SetAdapter(protocol, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[protocol] = url
}

// Get 获取插件
func (m *PluginManager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	return p, ok
}

// List 列出所有已注册插件
func (m *PluginManager) List() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	infos := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		infos = append(infos, PluginInfo{
			Name:        p.Name(),
			Description: p.Description(),
			Version:     p.Version(),
			Type:        "builtin",
		})
	}
	return infos
}

// HasPlugin 检查是否有对应协议的插件（并发安全）
func (m *PluginManager) HasPlugin(protocol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.plugins[protocol]
	return ok
}

// EnablePlugin 启用插件
func (m *PluginManager) EnablePlugin(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if plugin, ok := m.plugins[name]; ok {
		// 通过 Plugin 接口设置启用状态（如果插件支持）
		if configurable, ok := plugin.(PluginConfigurable); ok {
			configurable.SetEnabled(true)
		}
		return true
	}
	return false
}

// DisablePlugin 禁用插件
func (m *PluginManager) DisablePlugin(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if plugin, ok := m.plugins[name]; ok {
		// 通过 Plugin 接口设置禁用状态（如果插件支持）
		if configurable, ok := plugin.(PluginConfigurable); ok {
			configurable.SetEnabled(false)
		}
		return true
	}
	return false
}

// PluginConfigurable 可配置插件接口（可选实现）
type PluginConfigurable interface {
	SetEnabled(enabled bool)
	IsEnabled() bool
}

// GetAdapterURL 获取协议对应的 HTTP Adapter URL
func (m *PluginManager) GetAdapterURL(protocol string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adapters[protocol]
}

// PluginInfo 插件信息
type PluginInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Type        string `json:"type"` // "builtin" | "adapter"
	Enabled     bool   `json:"enabled"` // 是否启用
}

// 全局插件管理器
var DefaultManager = NewPluginManager()

// RegisterPlugin 全局注册插件
func RegisterPlugin(plugin Plugin) {
	DefaultManager.Register(plugin)
}
```

### 4.4 gRPC 插件实现（完整版）

由于 gRPC 是强类型接口驱动的，动态调用不可行。第一版采用**grpc-gateway 集成**方式：

```go
// backend/internal/rpc/grpc_plugin.go
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	RegisterPlugin(&gRPCPlugin{})
}

type gRPCPlugin struct {
	enabled bool // 是否启用
}

func (p *gRPCPlugin) Name() string {
	return "gRPC"
}

func (p *gRPCPlugin) Description() string {
	return "gRPC protocol plugin - supports unary RPC calls via grpc-gateway integration"
}

func (p *gRPCPlugin) Version() string {
	return "1.0.0"
}

// 实现 PluginConfigurable 接口
func (p *gRPCPlugin) SetEnabled(enabled bool) {
	p.enabled = enabled
}

func (p *gRPCPlugin) IsEnabled() bool {
	return p.enabled
}

func (p *gRPCPlugin) ConfigSchema() *PluginConfigSchema {
	return &PluginConfigSchema{
		Required: []string{"X-GRPC-Target"},
		Properties: map[string]ConfigField{
			"X-GRPC-Target": {
				Type:        "string",
				Description: "gRPC 服务目标地址，如 grpc-service:9080",
			},
			"X-GRPC-Timeout": {
				Type:        "number",
				Description: "超时时间（毫秒），默认 10000",
				Default:     10000,
			},
			"X-GRPC-Insecure": {
				Type:        "boolean",
				Description: "是否使用不安全连接（无 TLS）",
				Default:     true,
			},
		},
	}
}

func (p *gRPCPlugin) ValidateConfig(config map[string]any) error {
	if target, ok := config["X-GRPC-Target"].(string); ok {
		if target == "" {
			return fmt.Errorf("X-GRPC-Target is required")
		}
	}
	return nil
}

// gRPCConfig gRPC 插件配置
type gRPCConfig struct {
	Target     string // 目标地址
	TimeoutMs  int    // 超时时间
	Insecure   bool   // 是否不安全连接
}

func (p *gRPCPlugin) Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error) {
	// 解析配置
	cfg, err := p.parseConfig(req.Headers)
	if err != nil {
		return nil, err
	}
	
	// 解析方法名
	// 格式: /package.Service/Method
	method := req.Method
	if !strings.HasPrefix(method, "/") {
		method = "/" + method
	}
	
	// 创建带超时的 context
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// 建立 gRPC 连接
	conn, err := grpc.NewClient(cfg.Target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial failed for %s: %w", cfg.Target, err)
	}
	defer conn.Close()
	
	// 使用 grpc-gateway 或通用 gRPC 调用
	return p.invokeViaGateway(ctx, conn, method, req.Body)
}

func (p *gRPCPlugin) parseConfig(headers map[string]string) (*gRPCConfig, error) {
	cfg := &gRPCConfig{
		TimeoutMs: 10000, // 默认 10 秒
		Insecure:  true,
	}
	
	if target, ok := headers["X-GRPC-Target"]; ok {
		cfg.Target = target
	} else {
		return nil, fmt.Errorf("X-GRPC-Target header is required")
	}
	
	if timeout, ok := headers["X-GRPC-Timeout"]; ok {
		if ms, err := strconv.Atoi(timeout); err == nil && ms > 0 {
			cfg.TimeoutMs = ms
		}
	}
	
	if insecure, ok := headers["X-GRPC-Insecure"]; ok {
		cfg.Insecure = strings.ToLower(insecure) == "true"
	}
	
	return cfg, nil
}

// invokeViaGateway 通过 grpc-gateway 调用
// 
// 【第一版限制】由于 gRPC 是强类型协议，动态调用需要以下方案之一：
// 1. grpc-gateway: 需要部署 grpc-gateway 作为 HTTP Adapter，平台通过 CUSTOM_RPC 调用
// 2. 预生成代码: 为特定 proto 生成 Go 代码并注册为专用插件
// 3. protoreflect: 需要引入 grpc-ecosystem/grpc-gateway/v2/runtime
//
// 【推荐方案】使用 HTTP Adapter + grpc-gateway:
// - 无需修改平台代码
// - 支持任意 gRPC 服务
// - 配置简单，只需设置 Adapter URL
//
// 【错误返回】第一版 gRPC 插件不支持直接调用，返回明确的错误指引
func (p *gRPCPlugin) invokeViaGateway(ctx context.Context, conn *grpc.ClientConn, method string, body any) (*PluginResponse, error) {
	// 第一版 gRPC 插件不支持直接调用
	// 用户应使用 CUSTOM_RPC 协议 + grpc-gateway Adapter
	return nil, fmt.Errorf(
		"gRPC direct call not supported in v1.0. " +
			"Please use CUSTOM_RPC protocol with grpc-gateway Adapter instead. " +
			"See documentation for setup instructions."
	)
}
```

**gRPC 插件使用说明**：

由于 gRPC 是强类型协议，本平台的 gRPC 插件需要配合 grpc-gateway 使用。推荐方式：

1. **使用 HTTP Adapter（推荐）**：部署 grpc-gateway 作为 Adapter，平台通过 HTTP 调用
2. **使用预生成代码**：为特定 proto 生成 Go 代码，注册为专用插件

### 4.5 Dubbo 插件实现（框架）

```go
// backend/internal/rpc/dubbo_plugin.go
package rpc

import (
	"context"
	"fmt"
)

// Dubbo 插件需要引入 dubbo-go 或使用 TCP 协议解析
// 当前提供框架，完整实现需要额外依赖
type dubboPlugin struct{}

func init() {
	// 暂不注册，需要 dubbo-go 依赖
	// RegisterPlugin(&dubboPlugin{})
}

func (p *dubboPlugin) Name() string {
	return "Dubbo"
}

func (p *dubboPlugin) Description() string {
	return "Dubbo protocol plugin - requires dubbo-go dependency"
}

func (p *dubboPlugin) Version() string {
	return "0.1.0"
}

func (p *dubboPlugin) ConfigSchema() *PluginConfigSchema {
	return &PluginConfigSchema{
		Required: []string{"X-Dubbo-Target"},
		Properties: map[string]ConfigField{
			"X-Dubbo-Target": {
				Type:        "string",
				Description: "Dubbo 服务目标地址",
			},
			"X-Dubbo-Timeout": {
				Type:        "number",
				Description: "超时时间（毫秒）",
				Default:     10000,
			},
			"X-Dubbo-Version": {
				Type:        "string",
				Description: "Dubbo 协议版本",
				Default:     "2.0.0",
			},
		},
	}
}

func (p *dubboPlugin) ValidateConfig(config map[string]any) error {
	if _, ok := config["X-Dubbo-Target"]; !ok {
		return fmt.Errorf("X-Dubbo-Target is required")
	}
	return nil
}

func (p *dubboPlugin) Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error) {
	return nil, fmt.Errorf("Dubbo plugin not implemented yet")
}
```

### 4.6 HTTP Adapter 客户端（带重试机制）

```go
// backend/internal/rpc/http_adapter.go
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPAdapterClient HTTP Adapter 客户端（带重试机制）
type HTTPAdapterClient struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	timeout    time.Duration
}

// AdapterConfig Adapter 配置
type AdapterConfig struct {
	URL        string
	TimeoutMs  int
	MaxRetries int
}

// NewHTTPAdapterClient 创建 HTTP Adapter 客户端
func NewHTTPAdapterClient(cfg *AdapterConfig) *HTTPAdapterClient {
	timeout := 30 * time.Second
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}
	
	maxRetries := 2
	if cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}
	
	return &HTTPAdapterClient{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries: maxRetries,
	}
}

// AdapterRequest Adapter 请求格式
type AdapterRequest struct {
	Protocol    string                 `json:"protocol"`
	Method      string                 `json:"method"`
	RunID       string                 `json:"runId"`
	ScenarioID  string                 `json:"scenarioId"`
	TargetID    string                 `json:"targetId"`
	TargetName  string                 `json:"targetName"`
	Headers     map[string]string      `json:"headers"`
	QueryParams string                 `json:"queryParams"`
	Body        any                    `json:"body"`
	TimeoutMs   int                    `json:"timeoutMs"`
}

// AdapterResponse Adapter 响应格式
type AdapterResponse struct {
	Success    bool                   `json:"success"`
	StatusCode int                    `json:"statusCode"`
	Error      string                 `json:"error,omitempty"`
	Data       any                    `json:"data,omitempty"`
	Headers    map[string]string      `json:"headers,omitempty"`
	LatencyMs  int64                  `json:"latencyMs,omitempty"`
}

// Execute 执行 HTTP Adapter 调用（带重试）
func (c *HTTPAdapterClient) Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("no adapter URL configured")
	}
	
	// 构造 Adapter 请求
	adapterReq := &AdapterRequest{
		Protocol:    "CUSTOM_RPC",
		Method:      req.Method,
		RunID:       req.RunID,
		ScenarioID:  req.ScenarioID,
		TargetID:    req.TargetID,
		TargetName:  req.TargetName,
		Headers:     req.Headers,
		QueryParams: req.QueryParams,
		Body:        req.Body,
		TimeoutMs:   req.TimeoutMs,
	}
	
	// 重试逻辑
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避
			backoff := time.Duration(attempt*100) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		
		resp, statusCode, err := c.doExecute(ctx, adapterReq)
		if err == nil {
			return resp, nil
		}
		
		lastErr = err
		// 只对网络错误和 5xx 错误进行重试
		if !c.shouldRetry(err, statusCode) {
			return nil, err
		}
	}
	
	return nil, fmt.Errorf("adapter call failed after %d retries: %w", c.maxRetries, lastErr)
}

// doExecute 执行单次 HTTP 调用
// 返回 PluginResponse 和 HTTP 状态码（用于重试判断）
func (c *HTTPAdapterClient) doExecute(ctx context.Context, req *AdapterRequest) (*PluginResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal adapter request failed: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/invoke", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create adapter request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("adapter request failed: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read adapter response failed: %w", err)
	}
	
	var adapterResp AdapterResponse
	if err := json.Unmarshal(respBody, &adapterResp); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse adapter response failed: %w", err)
	}
	
	return &PluginResponse{
		Success:    adapterResp.Success,
		StatusCode: adapterResp.StatusCode,
		Error:      adapterResp.Error,
		Data:       adapterResp.Data,
		Headers:    adapterResp.Headers,
		LatencyMs:  adapterResp.LatencyMs,
	}, resp.StatusCode, nil
}

// shouldRetry 判断是否应该重试
// 重试条件：
// 1. 网络错误（连接超时、DNS 解析失败等）
// 2. HTTP 5xx 错误（服务器内部错误）
// 不重试条件：
// 1. HTTP 4xx 错误（客户端错误，重试无意义）
// 2. 业务错误（Success=false 但 HTTP 200）
func (c *HTTPAdapterClient) shouldRetry(err error, statusCode int) bool {
	// 网络错误重试
	if err != nil {
		return true
	}
	
	// 5xx 错误重试
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	
	// 4xx 错误不重试
	if statusCode >= 400 && statusCode < 500 {
		return false
	}
	
	// 其他情况不重试
	return false
}
```

### 4.7 集成到 Run Executor

修改 `backend/internal/api/runs.go`：

```go
// 在文件顶部添加导入
import (
	"all-in-one-testing/internal/rpc"
)

// 全局 Adapter 客户端
var adapterClient *rpc.HTTPAdapterClient

// initAdapter 初始化 Adapter 客户端
func initAdapter(adapterURL string) {
	if adapterURL != "" {
		adapterClient = rpc.NewHTTPAdapterClient(&rpc.AdapterConfig{
			URL:        adapterURL,
			TimeoutMs:  30000,
			MaxRetries: 2,
		})
	}
}

// executeRunSample 修改：支持插件机制
func executeRunSample(ctx context.Context, client *http.Client, input createRunRequest) runSample {
	// HTTP 协议使用原有逻辑
	if strings.EqualFold(input.Protocol, runProtocolHTTP) {
		return executeHTTPSample(ctx, client, input)
	}
	
	// CUSTOM_RPC 协议继续使用原有逻辑（HTTP Adapter）
	if strings.EqualFold(input.Protocol, runProtocolCustomRPC) {
		return executeCustomRPCSample(ctx, client, input)
	}
	
	// 其他协议尝试使用插件
	return executePluginSample(ctx, input)
}

// executePluginSample 新增：使用插件执行样本
func executePluginSample(ctx context.Context, input createRunRequest) runSample {
	startedAt := time.Now()
	
	// 构造插件请求
	pluginReq := &rpc.PluginRequest{
		Method:      input.Method,
		Headers:     runHeadersToMap(input.Headers),
		QueryParams: requestQuery(input, nil),
		Body:        customRPCBodyValue(selectRunBody(input, nil)),
		RunID:       input.RunID,
		ScenarioID:  input.ScenarioID,
		TargetID:    input.TargetID,
		TargetName:  input.TargetName,
	}
	
	// 检查是否有对应插件
	if !rpc.DefaultManager.HasPlugin(input.Protocol) {
		// 无插件，尝试使用 HTTP Adapter
		adapterURL := rpc.DefaultManager.GetAdapterURL(input.Protocol)
		if adapterURL == "" {
			// 既无插件也无 Adapter，返回错误
			return runSample{
				latency:   time.Since(startedAt),
				error:     fmt.Sprintf("unknown protocol: %s, no plugin or adapter configured", input.Protocol),
				success:   false,
				statusCode: 0,
			}
		}
		// 使用 HTTP Adapter（复用现有 executeCustomRPCSample 逻辑）
		// 这里需要调整 input.URL 为 adapterURL
		return executeCustomRPCSample(ctx, nil, input)
	}
	
	// 执行插件调用
	plugin, ok := rpc.DefaultManager.Get(input.Protocol)
	if !ok {
		return runSample{
			latency:   time.Since(startedAt),
			error:     fmt.Sprintf("plugin not found: %s", input.Protocol),
			success:   false,
			statusCode: 0,
		}
	}
	
	pluginResp, err := plugin.Execute(ctx, pluginReq)
	if err != nil {
		// 【gRPC 自动降级】如果 gRPC 插件返回不支持直接调用的错误，自动降级到 HTTP Adapter
		if input.Protocol == "gRPC" && isGRPCDirectCallNotSupported(err) {
			log.Printf("gRPC direct call not supported, fallback to HTTP Adapter: %v", err)
			// 检查是否有配置 HTTP Adapter
			adapterURL := rpc.DefaultManager.GetAdapterURL("gRPC")
			if adapterURL != "" {
				// 使用 HTTP Adapter 执行
				return executeHTTPAdapterForGRPC(ctx, adapterURL, pluginReq)
			}
			// 无 Adapter 配置，返回更友好的错误信息
			return runSample{
				latency:   time.Since(startedAt),
				error:     "gRPC direct call not supported. Please configure gRPC HTTP Adapter URL in settings.",
				success:   false,
				statusCode: 0,
			}
		}
		return runSample{
			latency:   time.Since(startedAt),
			error:     err.Error(),
			success:   false,
			statusCode: 0,
		}
	}
	
	return runSample{
		latency:    time.Since(startedAt),
		error:      pluginResp.Error,
		success:    pluginResp.Success,
		statusCode: pluginResp.StatusCode,
		body:       pluginResp.Data,
	}
}

// isGRPCDirectCallNotSupported 检查是否是 gRPC 直接调用不支持的错误
func isGRPCDirectCallNotSupported(err error) bool {
	return err != nil && strings.Contains(err.Error(), "gRPC direct call not supported")
}

// executeHTTPAdapterForGRPC 使用 HTTP Adapter 执行 gRPC 调用
func executeHTTPAdapterForGRPC(ctx context.Context, adapterURL string, req *rpc.PluginRequest) runSample {
	startedAt := time.Now()
	
	// 构造 Adapter 请求
	adapterReq := &rpc.AdapterRequest{
		Protocol:    "gRPC",
		Method:      req.Method,
		Headers:     req.Headers,
		QueryParams: req.QueryParams,
		Body:        req.Body,
		TimeoutMs:   req.TimeoutMs,
	}
	
	// 调用 HTTP Adapter
	resp, err := executeHTTPAdapterCall(ctx, adapterURL, adapterReq)
	if err != nil {
		return runSample{
			latency:   time.Since(startedAt),
			error:     fmt.Sprintf("gRPC adapter call failed: %v", err),
			success:   false,
			statusCode: 0,
		}
	}
	
	return runSample{
		latency:    time.Since(startedAt),
		error:      resp.Error,
		success:    resp.Success,
		statusCode: resp.StatusCode,
		body:       resp.Data,
	}
}
```

### 4.8 配置文件支持

```yaml
# config.yaml
rpc_plugins:
  # 是否启用插件机制
  enabled: true
  
  # HTTP Adapter 配置（用于 CUSTOM_RPC 和无本地插件的协议）
  http_adapter:
    # CUSTOM_RPC 默认使用此 Adapter
    default_url: "http://127.0.0.1:9090/invoke"
    timeout_ms: 30000
    retry_count: 2
  
  # 协议特定 Adapter（可选）
  adapters:
    gRPC: ""
    Dubbo: "http://dubbo-adapter:9090/invoke"
    Thrift: "http://thrift-adapter:9090/invoke"
  
  # 插件配置
  plugins:
    gRPC:
      enabled: true
      version: "1.0.0"
    Dubbo:
      enabled: false
      version: "0.1.0"
    Thrift:
      enabled: false
      version: "0.1.0"
  
  # 插件默认配置
  defaults:
    timeout_ms: 10000
    max_message_size_mb: 4
```

### 4.9 前端适配（完整协议选项）

```typescript
// frontend/src/app/App.tsx 修改

// 完整的协议选项
const protocolOptions = [
  { label: 'HTTP', value: 'HTTP', description: '标准 HTTP 协议', enabled: true },
  { label: 'Custom RPC (HTTP Adapter)', value: 'CUSTOM_RPC', description: '自定义 RPC 协议，通过 HTTP Adapter 调用', enabled: true },
  { label: 'gRPC', value: 'gRPC', description: 'gRPC 协议，推荐使用 HTTP Adapter + grpc-gateway', enabled: true },
  { label: 'Dubbo', value: 'Dubbo', description: 'Dubbo 协议，暂未实现', enabled: false },
  { label: 'Thrift', value: 'Thrift', description: 'Thrift 协议，暂未实现', enabled: false },
];

// 根据协议显示不同的配置字段
const getProtocolConfigFields = (protocol: string) => {
  switch (protocol) {
    case 'HTTP':
      return [
        { field: 'url', label: 'Target URL', placeholder: 'http://api.example.com' },
        { field: 'method', label: 'HTTP Method', placeholder: 'GET, POST, PUT, DELETE' },
      ];
    case 'CUSTOM_RPC':
      return [
        { field: 'url', label: 'Adapter URL', placeholder: 'http://127.0.0.1:9090/invoke' },
        { field: 'method', label: 'RPC Method', placeholder: 'checkout.OrderService/CreateOrder' },
      ];
    case 'gRPC':
      return [
        { field: 'method', label: 'gRPC Method', placeholder: '/package.Service/Method' },
        { field: 'headers', label: 'Headers', items: [
          { key: 'X-GRPC-Target', value: 'grpc-service:9080', description: 'gRPC 服务地址' },
          { key: 'X-GRPC-Timeout', value: '10000', description: '超时时间（毫秒）' },
        ]},
        { field: 'info', label: '注意', value: 'gRPC 插件第一版不支持直接调用，建议使用 CUSTOM_RPC + grpc-gateway' },
      ];
    case 'Dubbo':
      return [
        { field: 'method', label: 'Dubbo Method', placeholder: 'com.example.OrderService.createOrder' },
        { field: 'headers', label: 'Headers', items: [
          { key: 'X-Dubbo-Target', value: 'dubbo://127.0.0.1:12345', description: 'Dubbo 服务地址' },
          { key: 'X-Dubbo-Timeout', value: '10000', description: '超时时间（毫秒）' },
        ]},
      ];
    default:
      return [];
  }
};

// 更新 isExecutableScenarioFlowStep 函数以支持插件协议
// frontend/src/app/scenarios/ScenarioFlowEditor.tsx

function isExecutableScenarioFlowStep(step: ScenarioFlowStep): boolean {
  const protocol = step.protocol || 'HTTP';
  
  // 支持的协议列表
  const supportedProtocols = ['HTTP', 'CUSTOM_RPC', 'gRPC', 'Dubbo', 'Thrift'];
  
  // 检查协议是否支持
  if (!supportedProtocols.includes(protocol)) {
    return false;
  }
  
  // gRPC 协议需要目标地址
  if (protocol === 'gRPC') {
    const headers = step.headers || [];
    const hasTarget = headers.some(h => h.key === 'X-GRPC-Target');
    if (!hasTarget) {
      return false;
    }
  }
  
  // CUSTOM_RPC 和 gRPC 需要 method
  if (protocol === 'CUSTOM_RPC' || protocol === 'gRPC') {
    if (!step.method) {
      return false;
    }
  }
  
  return true;
}
```

## 5. 实现计划

### Phase 1：基础框架（1 天）
- [x] 实现 Plugin 接口定义（类型安全版本）
- [x] 实现 PluginManager（并发安全）
- [x] 修改 validateCreateRunRequest 支持新协议
- [x] 修改 Run Executor 集成插件机制

### Phase 2：HTTP Adapter 集成（0.5 天）
- [x] 实现 HTTPAdapterClient（带重试机制）
- [x] 复用现有 `executeCustomRPCSample` 逻辑
- [x] 添加 Adapter URL 配置支持

### Phase 3：gRPC 插件（2-3 天，可选）
- [ ] 集成 grpc-gateway
- [ ] 实现动态 proto 支持
- [ ] 添加示例 proto 和生成代码

### Phase 4：前端适配（1 天）
- [ ] 更新协议下拉框（完整协议选项）
- [ ] 添加插件配置字段
- [ ] 更新文档

## 6. 使用示例

### 6.1 使用 HTTP Adapter（推荐）

**前端配置**：
```
Protocol: CUSTOM_RPC
Method: checkout.OrderService/CreateOrder
Adapter URL: http://adapter-service:9090/invoke
Headers:
  X-Adapter-Token: secret-token
Body: {"orderId": "12345", "amount": 100.00}
```

**执行流程**：
1. Run Executor 检测协议为 `CUSTOM_RPC`
2. 使用 `executeCustomRPCSample` 调用 HTTP Adapter
3. Adapter 执行实际的 RPC 调用并返回结果
4. 平台解析响应

### 6.2 使用 gRPC 插件（需要 grpc-gateway）

**前置条件**：部署 grpc-gateway 作为 Adapter

**前端配置**：
```
Protocol: gRPC
Method: /checkout.OrderService/CreateOrder
Headers:
  X-GRPC-Target: grpc-service:9080
  X-GRPC-Timeout: 15000
Body: {"orderId": "12345", "amount": 100.00}
```

**执行流程**：
1. Run Executor 检测协议为 `gRPC`
2. 从 `PluginManager` 获取 `gRPC` 插件
3. 插件解析 `X-GRPC-Target` 获取目标地址
4. 建立 gRPC 连接并发送请求（通过 grpc-gateway）
5. 返回响应给平台

## 7. 兼容性说明

- **向后兼容**：现有 `CUSTOM_RPC` 协议继续工作，底层逻辑不变
- **配置兼容**：现有场景无需修改
- **API 兼容**：场景 API 无需修改，协议字段保持不变

## 8. 安全考虑

1. **插件隔离**：内置插件在平台进程中运行，需要严格审计
2. **网络访问**：gRPC/Dubbo 插件可能访问内网服务，需要权限控制
3. **超时控制**：所有插件调用都有超时限制（默认 10 秒），防止阻塞
4. **错误处理**：插件错误不会影响平台稳定性，会优雅降级到 HTTP Adapter
5. **重试机制**：HTTP Adapter 调用支持配置重试次数和退避策略

## 9. 错误码定义（完整版，含数值和 Go 代码）

| 错误码 | 数值 | 说明 |
|--------|------|------|
| PLUGIN_NOT_FOUND | 4001 | 未找到对应协议的插件 |
| PLUGIN_EXEC_FAILED | 4002 | 插件执行失败 |
| PLUGIN_TIMEOUT | 4003 | 插件执行超时 |
| PLUGIN_CONFIG_INVALID | 4004 | 插件配置无效（如缺少 X-GRPC-Target） |
| PLUGIN_CONFIG_VALIDATION_FAILED | 4005 | 插件配置验证失败 |
| ADAPTER_NOT_CONFIGURED | 4101 | 未配置 HTTP Adapter |
| ADAPTER_CALL_FAILED | 4102 | HTTP Adapter 调用失败 |
| ADAPTER_RETRY_EXHAUSTED | 4103 | HTTP Adapter 重试次数用尽 |
| ADAPTER_NETWORK_ERROR | 4104 | Adapter 网络错误（连接超时、DNS 解析失败等） |
| PROTOCOL_NOT_SUPPORTED | 4201 | 协议不支持 |
| PROTOCOL_VALIDATION_FAILED | 4202 | 协议验证失败 |
| GRPC_TARGET_NOT_SPECIFIED | 4301 | gRPC 目标地址未指定 |
| GRPC_DIAL_FAILED | 4302 | gRPC 连接失败 |
| GRPC_INVOKE_NOT_SUPPORTED | 4303 | gRPC 直接调用不支持（第一版限制） |
| DUBBO_TARGET_NOT_SPECIFIED | 4401 | Dubbo 目标地址未指定 |
| DUBBO_INVOKE_FAILED | 4402 | Dubbo 调用失败 |
| THRIFT_TARGET_NOT_SPECIFIED | 4501 | Thrift 目标地址未指定 |
| THRIFT_INVOKE_FAILED | 4502 | Thrift 调用失败 |
| INTERNAL_ERROR | 5000 | 内部错误 |

### 9.1 错误码常量定义（Go 代码）

```go
// backend/internal/rpc/errors.go
package rpc

import "fmt"

// 错误码常量
const (
	// 插件相关错误 (4000-4099)
	ErrCodePluginNotFound           = 4001
	ErrCodePluginExecFailed         = 4002
	ErrCodePluginTimeout            = 4003
	ErrCodePluginConfigInvalid      = 4004
	ErrCodePluginConfigValidation   = 4005
	
	// Adapter 相关错误 (4100-4199)
	ErrCodeAdapterNotConfigured     = 4101
	ErrCodeAdapterCallFailed        = 4102
	ErrCodeAdapterRetryExhausted    = 4103
	ErrCodeAdapterNetworkError      = 4104
	
	// 协议相关错误 (4200-4299)
	ErrCodeProtocolNotSupported     = 4201
	ErrCodeProtocolValidationFailed = 4202
	
	// gRPC 相关错误 (4300-4399)
	ErrCodeGRPCTargetNotSpecified   = 4301
	ErrCodeGRPCDialFailed           = 4302
	ErrCodeGRPCInvokeNotSupported   = 4303
	
	// Dubbo 相关错误 (4400-4499)
	ErrCodeDubboTargetNotSpecified  = 4401
	ErrCodeDubboInvokeFailed        = 4402
	
	// Thrift 相关错误 (4500-4599)
	ErrCodeThriftTargetNotSpecified = 4501
	ErrCodeThriftInvokeFailed       = 4502
	
	// 内部错误
	ErrCodeInternalError            = 5000
)

// 错误类型定义
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *RPCError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// 预定义错误消息
var errorMessages = map[int]string{
	ErrCodePluginNotFound:           "Plugin not found",
	ErrCodePluginExecFailed:         "Plugin execution failed",
	ErrCodePluginTimeout:            "Plugin execution timeout",
	ErrCodePluginConfigInvalid:      "Plugin configuration invalid",
	ErrCodePluginConfigValidation:   "Plugin configuration validation failed",
	ErrCodeAdapterNotConfigured:     "HTTP Adapter not configured",
	ErrCodeAdapterCallFailed:        "HTTP Adapter call failed",
	ErrCodeAdapterRetryExhausted:    "HTTP Adapter retry exhausted",
	ErrCodeAdapterNetworkError:      "Adapter network error",
	ErrCodeProtocolNotSupported:     "Protocol not supported",
	ErrCodeProtocolValidationFailed: "Protocol validation failed",
	ErrCodeGRPCTargetNotSpecified:   "gRPC target not specified",
	ErrCodeGRPCDialFailed:           "gRPC dial failed",
	ErrCodeGRPCInvokeNotSupported:   "gRPC direct invoke not supported",
	ErrCodeDubboTargetNotSpecified:  "Dubbo target not specified",
	ErrCodeDubboInvokeFailed:        "Dubbo invoke failed",
	ErrCodeThriftTargetNotSpecified: "Thrift target not specified",
	ErrCodeThriftInvokeFailed:       "Thrift invoke failed",
	ErrCodeInternalError:            "Internal error",
}

// NewRPCError 创建 RPC 错误
func NewRPCError(code int, detail string) *RPCError {
	message := errorMessages[code]
	if message == "" {
		message = "Unknown error"
	}
	return &RPCError{
		Code:    code,
		Message: message,
		Detail:  detail,
	}
}

// NewPluginNotFoundError 创建插件未找到错误
func NewPluginNotFoundError(protocol string) *RPCError {
	return NewRPCError(ErrCodePluginNotFound, fmt.Sprintf("protocol: %s", protocol))
}

// NewGRPCDirectCallNotSupportedError 创建 gRPC 直接调用不支持错误
func NewGRPCDirectCallNotSupportedError() *RPCError {
	return NewRPCError(ErrCodeGRPCInvokeNotSupported,
		"Please use CUSTOM_RPC with grpc-gateway Adapter instead")
}

// NewAdapterNotConfiguredError 创建 Adapter 未配置错误
func NewAdapterNotConfiguredError(protocol string) *RPCError {
	return NewRPCError(ErrCodeAdapterNotConfigured,
		fmt.Sprintf("protocol: %s, please configure adapter URL", protocol))
}
```

## 10. 后续优化方向

1. **插件热加载**：支持运行时加载/卸载插件
2. **插件监控**：记录插件调用日志和性能指标
3. **插件配置界面**：在 Targets & Agents 页面添加插件管理
4. **更多协议支持**：Thrift、gRPC-Web、HTTP/2 等
5. **插件市场**：支持第三方插件发布和安装
