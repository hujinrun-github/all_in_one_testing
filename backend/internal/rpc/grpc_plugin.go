package rpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	Target    string // 目标地址
	TimeoutMs int    // 超时时间
	Insecure  bool    // 是否不安全连接
}

func (p *gRPCPlugin) Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error) {
	// 解析配置
	_, err := p.parseConfig(req.Headers)
	if err != nil {
		return nil, err
	}

	// 解析方法名
	// 格式: /package.Service/Method
	method := req.Method
	if !strings.HasPrefix(method, "/") {
		method = "/" + method
	}

	// 第一版 gRPC 插件不支持直接调用，返回明确的错误
	return p.invokeViaGateway(ctx, method, req.Body)
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
func (p *gRPCPlugin) invokeViaGateway(ctx context.Context, method string, body any) (*PluginResponse, error) {
	// 第一版 gRPC 插件不支持直接调用
	// 用户应使用 CUSTOM_RPC 协议 + grpc-gateway Adapter
	return nil, fmt.Errorf("gRPC direct call not supported in v1.0. "+
		"Please use CUSTOM_RPC protocol with grpc-gateway Adapter instead. "+
		"See documentation for setup instructions.")
}
