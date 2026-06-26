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

// ExecuteWithProtocol 使用指定协议执行 HTTP Adapter 调用
func (c *HTTPAdapterClient) ExecuteWithProtocol(ctx context.Context, protocol string, req *PluginRequest) (*PluginResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("no adapter URL configured")
	}

	// 构造 Adapter 请求
	adapterReq := &AdapterRequest{
		Protocol:    protocol,
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
