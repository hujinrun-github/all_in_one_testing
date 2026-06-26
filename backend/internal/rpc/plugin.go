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
	Required    []string          `json:"required,omitempty"` // 必需的配置项
	Properties  map[string]ConfigField `json:"properties"`      // 配置项定义
}

// ConfigField 配置字段定义
type ConfigField struct {
	Type        string   `json:"type"`                  // "string" | "number" | "boolean" | "object"
	Description string   `json:"description"`           // 字段描述
	Default     any      `json:"default,omitempty"`     // 默认值
	Enum        []string `json:"enum,omitempty"`        // 可选值列表
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
	Body any `json:"body,omitempty"`

	// 运行时上下文
	RunID      string `json:"runId,omitempty"`
	ScenarioID string `json:"scenarioId,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	TargetName string `json:"targetName,omitempty"`

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
	adapters  map[string]string // protocol -> adapter URL
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

// UnmarshalPluginConfig 从 JSON 反序列化插件配置
func UnmarshalPluginConfig(data []byte) (map[string]any, error) {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plugin config: %w", err)
	}
	return config, nil
}
