package rpc

import "fmt"

// 错误码常量
const (
	// 插件相关错误 (4000-4099)
	ErrCodePluginNotFound         = 4001
	ErrCodePluginExecFailed       = 4002
	ErrCodePluginTimeout          = 4003
	ErrCodePluginConfigInvalid    = 4004
	ErrCodePluginConfigValidation = 4005

	// Adapter 相关错误 (4100-4199)
	ErrCodeAdapterNotConfigured = 4101
	ErrCodeAdapterCallFailed    = 4102
	ErrCodeAdapterRetryExhausted = 4103
	ErrCodeAdapterNetworkError  = 4104

	// 协议相关错误 (4200-4299)
	ErrCodeProtocolNotSupported     = 4201
	ErrCodeProtocolValidationFailed = 4202

	// gRPC 相关错误 (4300-4399)
	ErrCodeGRPCTargetNotSpecified   = 4301
	ErrCodeGRPCDialFailed           = 4302
	ErrCodeGRPCInvokeNotSupported   = 4303

	// Dubbo 相关错误 (4400-4499)
	ErrCodeDubboTargetNotSpecified = 4401
	ErrCodeDubboInvokeFailed      = 4402

	// Thrift 相关错误 (4500-4599)
	ErrCodeThriftTargetNotSpecified = 4501
	ErrCodeThriftInvokeFailed       = 4502

	// 内部错误
	ErrCodeInternalError = 5000
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

// NewGRPCTargetNotSpecifiedError 创建 gRPC 目标未指定错误
func NewGRPCTargetNotSpecifiedError() *RPCError {
	return NewRPCError(ErrCodeGRPCTargetNotSpecified, "X-GRPC-Target header is required")
}
