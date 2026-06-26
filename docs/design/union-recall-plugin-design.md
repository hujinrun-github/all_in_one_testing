# Union Recall Merge RPC 插件设计方案

## 1. 背景

`union_recall_merge_kugou_server` 提供召回服务，支持两种协议：
- **tme-protocol/ths (JCE)**: `recall_ispine.go` 实现
- **FlatBuffers**: `recall_ispine_fb.go` 实现

本方案设计一个本地插件 `UnionRecall`，直接调用 tme-protocol 客户端，无需额外部署 HTTP Adapter。

## 2. 接口分析

### 2.1 请求结构

```go
// GetRecallResultReq 请求（对应 svr_proto.GetRecallResultReq）
type GetRecallResultReq struct {
    Uid        string            `json:"uid"`        // 用户ID
    GraphName  string            `json:"graph_name"`  // 图名称（DAG 名称）
    AbtName    string            `json:"abt_name"`    // ABT 名称
    ReqID      string            `json:"req_id"`      // 请求ID
    ReqTime    int64             `json:"req_time"`      // 请求时间
    ExtraParam map[string]string `json:"extra_param"`  // 额外参数
    Debug      bool              `json:"debug"`        // 调试开关
    WebDebug   bool              `json:"web_debug"`    // Web 调试开关
}
```

### 2.2 响应结构

```go
// GetRecallResultRsp 响应（对应 svr_proto.GetRecallResultRsp）
type GetRecallResultRsp struct {
    Result       RecallResult                 `json:"result"`        // 召回结果
    RetCode      int32                        `json:"ret_code"`      // 返回码
    DebugInfo    map[string]DebugInfo         `json:"debug_info"`    // 调试信息
    GraphVersion int32                        `json:"graph_version"` // 图版本
}

// RecallResult 召回结果
type RecallResult struct {
    ItemList []RecallItem `json:"item_list"` // item 列表
    Total    uint32       `json:"total"`      // 总数
    ErrMsg   string      `json:"err_msg"`    // 错误信息
}

// RecallItem 召回项
type RecallItem struct {
    Item             string        `json:"item"`              // 物料ID
    Score            float32       `json:"score"`             // 分数
    Extra            string        `json:"extra"`             // 额外信息 JSON
    SourceId         string        `json:"source_id"`         // 召回源 ID
    MultiTriggerList [][]string    `json:"multi_trigger_list"` // 多触发器列表
    ExtraMap         map[string]string `json:"extra_map"`       // 额外字段
}

// DebugInfo 调试信息
type DebugInfo struct {
    Cost       int64   `json:"cost"`       // 耗时
    BeforeCost int64   `json:"before_cost"` // 前置耗时
    RecallCnt  int32   `json:"recall_cnt"`  // 召回数量
    Result     int32   `json:"result"`     // 结果
    Items      string  `json:"items"`      // item 信息
}
```

## 3. 插件设计

### 3.1 插件信息

| 属性 | 值 |
|------|-----|
| 名称 | `UnionRecall` |
| 描述 | Union Recall Merge 服务插件 - 支持 tme-protocol 协议 |
| 版本 | 1.0.0 |
| 协议类型 | tme-protocol (ths) |

### 3.2 配置参数

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `X-Service-Name` | string | 是 | - | 服务名称，如 `union_recall_merge_kugou` |
| `X-Target-Address` | string | 否 | - | 目标服务地址，如 `ip://10.210.0.10:19488` |
| `X-Timeout-Ms` | number | 否 | 30000 | 超时时间（毫秒） |
| `X-Module-ID` | number | 否 | 0 | 模块 ID（用于监控上报） |
| `X-Interface-ID` | number | 否 | 0 | 接口 ID（用于监控上报） |

### 3.3 Method 字段说明

Union Recall 协件的 `PluginRequest.Method` 字段在本插件中**不需要使用**。原因：

1. **单一接口**：Union Recall Merge 服务只有一个主要接口 `GetRecallResult`，不需要通过 Method 字段区分
2. **接口固定**：插件内部硬编码调用 `svr_proto.GetRecallResultReq` 和 `svr_proto.GetRecallResultRsp`
3. **简化配置**：用户只需配置服务地址和请求参数，无需指定方法名

如需扩展支持多个接口，可在后续版本中通过 Method 字段区分。

### 3.4 请求体格式

```json
{
  "uid": "123456",
  "graph_name": "kugou_default_dag",
  "abt_name": "abt_experiment_1",
  "req_id": "req_1780000000000",
  "req_time": 1780000000000,
  "extra_param": {
    "need_num": "100",
    "query_key_num": "1",
    "query_key_1": "some_key"
  },
  "debug": false,
  "web_debug": false
}
```

### 3.5 响应体格式

```json
{
  "ret_code": 0,
  "total": 50,
  "item_list": [
    {
      "item": "song_12345",
      "score": 0.95,
      "extra": "{}",
      "source_id": "recall_source_1",
      "multi_trigger_list": [["trigger_1"]],
      "extra_map": {
        "trigger_type": "hot"
      }
    }
  ],
  "graph_version": 12345,
  "err_msg": "",
  "debug_info": {
    "node_1": {
      "cost": 100,
      "recall_cnt": 50,
      "result": 0,
      "items": "[]"
    }
  }
}
```

## 4. 实现方案

### 4.1 文件结构

```
backend/internal/rpc/
├── plugin.go           # 已有：Plugin 接口定义
├── errors.go           # 已有：错误码定义（需追加 UnionRecall 错误码）
├── union_recall_plugin.go  # 新增：UnionRecall 插件实现
└── union_recall_types.go   # 新增：请求/响应类型定义
```

### 4.2 核心代码

#### union_recall_types.go

```go
// backend/internal/rpc/union_recall_types.go
package rpc

// UnionRecallRequest Union Recall 请求结构
type UnionRecallRequest struct {
	// 用户ID
	Uid string `json:"uid"`
	// 图名称（DAG 名称）
	GraphName string `json:"graph_name"`
	// ABT 名称
	AbtName string `json:"abt_name,omitempty"`
	// 请求ID
	ReqID string `json:"req_id,omitempty"`
	// 请求时间（毫秒时间戳）
	ReqTime int64 `json:"req_time,omitempty"`
	// 额外参数
	ExtraParam map[string]string `json:"extra_param,omitempty"`
	// 调试开关
	Debug bool `json:"debug,omitempty"`
	// Web 调试开关
	WebDebug bool `json:"web_debug,omitempty"`
}

// UnionRecallResponse Union Recall 响应结构
type UnionRecallResponse struct {
	// 返回码（0 表示成功）
	RetCode int32 `json:"ret_code"`
	// 召回结果总数（注意：统一使用 int32 类型，与 RecallResult.Total 保持一致）
	Total int32 `json:"total"`
	// 召回结果列表
	ItemList []UnionRecallItem `json:"item_list"`
	// 图版本
	GraphVersion int32 `json:"graph_version"`
	// 错误信息
	ErrMsg string `json:"err_msg,omitempty"`
	// 调试信息（仅 web_debug=true 时返回）
	DebugInfo map[string]UnionRecallDebugInfo `json:"debug_info,omitempty"`
}

// UnionRecallItem 召回结果项
type UnionRecallItem struct {
	// 物料ID
	Item string `json:"item"`
	// 分数
	Score float64 `json:"score"`
	// 额外信息 JSON
	Extra string `json:"extra,omitempty"`
	// 召回源 ID
	SourceId string `json:"source_id,omitempty"`
	// 多触发器列表
	MultiTriggerList [][]string `json:"multi_trigger_list,omitempty"`
	// 额外字段
	ExtraMap map[string]string `json:"extra_map,omitempty"`
}

// UnionRecallDebugInfo 调试信息
type UnionRecallDebugInfo struct {
	// 耗时（毫秒）
	Cost int64 `json:"cost"`
	// 前置耗时（毫秒）
	BeforeCost int64 `json:"before_cost,omitempty"`
	// 召回数量
	RecallCnt int32 `json:"recall_cnt"`
	// 结果码
	Result int32 `json:"result"`
	// Item 信息
	Items string `json:"items,omitempty"`
}

// UnionRecallConfig 插件配置
type UnionRecallConfig struct {
	// 服务名称
	ServiceName string
	// 目标地址
	TargetAddress string
	// 超时时间（毫秒）
	TimeoutMs int
	// 模块 ID
	ModuleID int
	// 接口 ID
	InterfaceID int
}
```

#### union_recall_plugin.go

```go
// backend/internal/rpc/union_recall_plugin.go
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"git.code.oa.com/tme/going/codec/qzh"
	"git.code.oa.com/tme/tme-protocol/th"
	"git.code.oa.com/tme/tme-protocol/thc"
	svr_proto "git.code.oa.com/tme/kgjce2go/proto_union_recall_merge_handler"
)

func init() {
	RegisterPlugin(&UnionRecallPlugin{})
}

// UnionRecallPlugin Union Recall Merge 服务插件
type UnionRecallPlugin struct {
	// 是否启用
	enabled bool
}

// Name 返回插件名称
func (p *UnionRecallPlugin) Name() string {
	return "UnionRecall"
}

// Description 返回插件描述
func (p *UnionRecallPlugin) Description() string {
	return "Union Recall Merge 服务插件 - 支持 tme-protocol (ths) 协议，用于召回服务压测"
}

// Version 返回插件版本
func (p *UnionRecallPlugin) Version() string {
	return "1.0.0"
}

// ConfigSchema 返回插件配置 Schema
func (p *UnionRecallPlugin) ConfigSchema() *PluginConfigSchema {
	return &PluginConfigSchema{
		Required: []string{"X-Service-Name"},
		Properties: map[string]ConfigField{
			"X-Service-Name": {
				Type:        "string",
				Description: "服务名称，如 union_recall_merge_kugou",
			},
			"X-Target-Address": {
				Type:        "string",
				Description: "目标服务地址，如 ip://10.210.0.10:19488 或 gl5://12312:323",
			},
			"X-Timeout-Ms": {
				Type:        "number",
				Description: "超时时间（毫秒），默认 30000",
				Default:     30000,
			},
			"X-Module-ID": {
				Type:        "number",
				Description: "模块 ID（用于监控上报）",
				Default:     0,
			},
			"X-Interface-ID": {
				Type:        "number",
				Description: "接口 ID（用于监控上报）",
				Default:     0,
			},
		},
	}
}

// ValidateConfig 验证插件配置
func (p *UnionRecallPlugin) ValidateConfig(config map[string]any) error {
	if _, ok := config["X-Service-Name"].(string); !ok || config["X-Service-Name"] == "" {
		return fmt.Errorf("X-Service-Name is required")
	}
	return nil
}

// 实现 PluginConfigurable 接口
func (p *UnionRecallPlugin) SetEnabled(enabled bool) {
	p.enabled = enabled
}

func (p *UnionRecallPlugin) IsEnabled() bool {
	return p.enabled
}

// Execute 执行 Union Recall 调用
func (p *UnionRecallPlugin) Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error) {
	// 1. 解析配置
	cfg, err := p.parseConfig(req.Headers)
	if err != nil {
		return nil, err
	}

	// 2. 解析请求体
	var recallReq UnionRecallRequest
	if req.Body != nil {
		bodyBytes, _ := json.Marshal(req.Body)
		if err := json.Unmarshal(bodyBytes, &recallReq); err != nil {
			return nil, fmt.Errorf("parse request body failed: %w", err)
		}
	}

	// 3. 设置默认值
	if recallReq.ReqID == "" {
		recallReq.ReqID = fmt.Sprintf("all_in_one_%d", time.Now().UnixNano())
	}
	if recallReq.ReqTime == 0 {
		recallReq.ReqTime = time.Now().UnixMilli()
	}

	// 4. 构造 JCE 请求
	jceReq := &svr_proto.GetRecallResultReq{
		Uid:        recallReq.Uid,
		GraphName:  recallReq.GraphName,
		AbtName:    recallReq.AbtName,
		ReqID:      recallReq.ReqID,
		ReqTime:    recallReq.ReqTime,
		ExtraParam: recallReq.ExtraParam,
		Debug:      recallReq.Debug,
		WebDebug:   recallReq.WebDebug,
	}

	jceRsp := &svr_proto.GetRecallResultRsp{}

	// 5. 创建带超时的 context
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 6. 调用 tme-protocol 服务
	start := time.Now()
	err = p.callTHS(ctx, cfg, jceReq, jceRsp)
	latency := time.Since(start)

	if err != nil {
		return &PluginResponse{
			Success:   false,
			Error:     fmt.Sprintf("union recall call failed: %v", err),
			LatencyMs: latency.Milliseconds(),
		}, nil
	}

	// 7. 转换响应
	pluginResp := p.convertResponse(jceRsp, latency)
	return pluginResp, nil
}

// parseConfig 解析插件配置
func (p *UnionRecallPlugin) parseConfig(headers map[string]string) (*UnionRecallConfig, error) {
	cfg := &UnionRecallConfig{
		TimeoutMs: 30000,
	}

	if v, ok := headers["X-Service-Name"]; ok {
		cfg.ServiceName = v
	} else {
		return nil, fmt.Errorf("X-Service-Name header is required")
	}

	if v, ok := headers["X-Target-Address"]; ok {
		cfg.TargetAddress = v
	}

	if v, ok := headers["X-Timeout-Ms"]; ok {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			cfg.TimeoutMs = ms
		}
	}

	if v, ok := headers["X-Module-ID"]; ok {
		if moduleID, err := strconv.Atoi(v); err == nil {
			cfg.ModuleID = moduleID
		}
	}

	if v, ok := headers["X-Interface-ID"]; ok {
		if interfaceID, err := strconv.Atoi(v); err == nil {
			cfg.InterfaceID = interfaceID
		}
	}

	return cfg, nil
}

// callTHS 通过 tme-protocol 调用服务
func (p *UnionRecallPlugin) callTHS(ctx context.Context, cfg *UnionRecallConfig, req *svr_proto.GetRecallResultReq, rsp *svr_proto.GetRecallResultRsp) error {
	authInfo := qzh.AuthInfo{Uin: uint32(100001)}
	callDesc := qzh.CallDesc{
		CmdId:       0, // 根据实际服务配置
		SubCmdId:    0,
		AppProtocol: "qza",
	}

	// 构造 tme-protocol 请求
	qc := qzh.NewQzClient(callDesc, authInfo, cfg.ServiceName, req, rsp)
	qc.Address = cfg.TargetAddress
	qc.ModuleID = cfg.ModuleID
	qc.InterfaceID = cfg.InterfaceID
	qc.Timeout = 200 * time.Hour // tme-protocol 内部会使用 context 的超时
	qc.ReqType = 2
	qc.Network = "tcp"

	return qc.Do(ctx)
}

// convertResponse 转换 JCE 响应为统一格式
func (p *UnionRecallPlugin) convertResponse(rsp *svr_proto.GetRecallResultRsp, latency time.Duration) *PluginResponse {
	items := make([]UnionRecallItem, 0, len(rsp.Result.ItemList))
	for _, item := range rsp.Result.ItemList {
		items = append(items, UnionRecallItem{
			Item:             item.Item,
			Score:            float64(item.Score),
			Extra:            item.Extra,
			SourceId:         item.SourceId,
			MultiTriggerList: item.MultiTriggerList,
			ExtraMap:         item.ExtraMap,
		})
	}

	debugInfo := make(map[string]UnionRecallDebugInfo)
	for k, v := range rsp.Debuginfo {
		debugInfo[k] = UnionRecallDebugInfo{
			Cost:       v.Cost,
			BeforeCost: v.BeforeCost,
			RecallCnt:  v.RecallCnt,
			Result:     v.Result,
			Items:      v.Items,
		}
	}

	return &PluginResponse{
		Success: rsp.RetCode == 0,
		StatusCode: int(rsp.RetCode),
		// 使用 map[string]any 确保 JSON 序列化正确
		Data: map[string]any{
			"ret_code":      rsp.RetCode,
			"total":         int32(rsp.Result.Total), // 明确转换为 int32
			"item_list":     items,
			"graph_version": rsp.GraphVersion,
			"err_msg":       rsp.Result.ErrMsg,
			"debug_info":    debugInfo,
		},
		LatencyMs: latency.Milliseconds(),
	}
}
```

## 5. 使用方式

### 5.1 创建场景

在 **Scenarios** 页面：

```
Protocol: UnionRecall
Method: union_recall_merge.GetRecallResult  # 注意：UnionRecall 插件中 Method 字段不使用，可留空或填写接口名
Headers:
  X-Service-Name: union_recall_merge_kugou
  X-Target-Address: ip://10.210.0.10:19488
  X-Timeout-Ms: 30000
Body Variants:
  70% {
    "uid": "123456",
    "graph_name": "kugou_default_dag",
    "extra_param": {
      "need_num": "100"
    }
  }
  30% {
    "uid": "789012",
    "graph_name": "kugou_default_dag",
    "extra_param": {
      "need_num": "50"
    }
  }
Timeout: 30000ms
```

### 5.2 断言配置

Union Recall 插件支持响应断言，用于自动化测试。断言配置示例：

```
Assertions:
  # 基础断言：检查返回码
  - type: json_path
    expression: "$.ret_code"
    operator: eq
    expected: 0
    
  # 数量断言：检查召回数量
  - type: json_path
    expression: "$.total"
    operator: gte
    expected: 10
    
  # 字段存在性断言：检查 item_list 不为空
  - type: json_path
    expression: "$.item_list"
    operator: not_empty
    
  # 性能断言：检查延迟
  - type: latency
    operator: lte
    expected: 500  # 毫秒
    
  # 自定义字段断言：检查 graph_version
  - type: json_path
    expression: "$.graph_version"
    operator: gt
    expected: 0
```

**断言类型说明**：

| 类型 | 说明 | operator | expected |
|------|------|----------|----------|
| `json_path` | JSONPath 表达式断言 | `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `not_empty` | 期望值 |
| `latency` | 延迟断言 | `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | 毫秒数 |
| `status_code` | 状态码断言 | `eq`, `ne` | 数字 |

### 5.3 API 调用

```json
POST /api/scenarios
{
  "name": "union-recall-smoke",
  "protocol": "UnionRecall",
  "method": "union_recall_merge.GetRecallResult",
  "headers": [
    {"key": "X-Service-Name", "value": "union_recall_merge_kugou"},
    {"key": "X-Target-Address", "value": "ip://10.210.0.10:19488"},
    {"key": "X-Timeout-Ms", "value": "30000"}
  ],
  "bodyVariants": [
    {
      "name": "default",
      "weight": 100,
      "body": "{\"uid\":\"123456\",\"graph_name\":\"kugou_default_dag\",\"extra_param\":{\"need_num\":\"100\"}}"
    }
  ],
  "timeoutMs": 30000,
  "assertions": [
    {
      "type": "json_path",
      "expression": "$.ret_code",
      "operator": "eq",
      "expected": 0
    },
    {
      "type": "json_path",
      "expression": "$.total",
      "operator": "gte",
      "expected": 10
    }
  ]
}
```

## 6. 前端适配

### 6.1 协议选项

在 `frontend/src/app/App.tsx` 的 `protocolOptions` 中添加：

```typescript
const protocolOptions = [
  { label: 'HTTP', value: 'HTTP', description: '标准 HTTP 协议' },
  { label: 'Custom RPC (HTTP Adapter)', value: 'CUSTOM_RPC', description: '自定义 RPC 协议，通过 HTTP Adapter 调用' },
  { label: 'Union Recall', value: 'UnionRecall', description: 'Union Recall Merge 服务，支持 tme-protocol' },
  { label: 'gRPC', value: 'gRPC', description: 'gRPC 协议，推荐使用 HTTP Adapter + grpc-gateway' },
  { label: 'Dubbo', value: 'Dubbo', description: 'Dubbo 协议，暂未实现' },
  { label: 'Thrift', value: 'Thrift', description: 'Thrift 协议，暂未实现' },
];
```

### 6.2 协议验证

更新 `isExecutableScenarioFlowStep` 函数：

```typescript
function isExecutableScenarioFlowStep(step: ScenarioFlowStep): boolean {
  const protocol = (step.protocol || 'HTTP').toUpperCase();
  const type = (step.type || '').toLowerCase();
  
  // 支持的协议列表（与 rpc-plugin-design.md 保持一致）
  const supportedProtocols = ['HTTP', 'CUSTOM_RPC', 'UNIONRECALL', 'gRPC', 'Dubbo', 'Thrift'];
  
  if (!supportedProtocols.includes(protocol)) {
    return false;
  }
  
  // 必须是 request 类型
  if (type !== 'request') {
    return false;
  }
  
  // UnionRecall 协议需要 X-Service-Name 和 graph_name
  if (protocol === 'UNIONRECALL') {
    const headers = step.headers || [];
    const hasServiceName = headers.some(h => h.key === 'X-Service-Name');
    if (!hasServiceName) {
      return false;
    }
    // 从 body 中检查 graph_name（如果有 body）
    if (step.body) {
      try {
        const body = JSON.parse(step.body);
        if (!body.graph_name) {
          return false;
        }
      } catch {
        return false;
      }
    }
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
  
  // 注意：UnionRecall 协议不需要 method 字段验证
  
  return true;
}
```

### 6.3 UnionRecall 协议配置字段

在 `getProtocolConfigFields` 函数中添加：

```typescript
// 根据协议显示不同的配置字段
const getProtocolConfigFields = (protocol: string) => {
  switch (protocol) {
    // ... 其他协议配置 ...
    
    case 'UnionRecall':
      return [
        { field: 'method', label: 'Method', placeholder: 'union_recall_merge.GetRecallResult', optional: true },
        { field: 'headers', label: 'Headers', items: [
          { key: 'X-Service-Name', value: 'union_recall_merge_kugou', description: '服务名称（必需）' },
          { key: 'X-Target-Address', value: 'ip://10.210.0.10:19488', description: '目标服务地址' },
          { key: 'X-Timeout-Ms', value: '30000', description: '超时时间（毫秒）' },
        ]},
        { field: 'body', label: 'Request Body', placeholder: '{"uid":"123456","graph_name":"kugou_default_dag"}' },
        { field: 'info', label: '注意', value: 'UnionRecall 协议不需要 Method 字段，插件内部固定调用 GetRecallResult 接口' },
      ];
    
    default:
      return [];
  }
};
```

### 6.4 断言配置界面

在场景编辑器中实现断言配置功能：

```typescript
// frontend/src/components/AssertionEditor.tsx

interface Assertion {
  type: 'json_path' | 'latency' | 'status_code';
  expression?: string;  // JSONPath 表达式
  operator: 'eq' | 'ne' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'not_empty';
  expected?: string | number;
}

interface AssertionEditorProps {
  assertions: Assertion[];
  onChange: (assertions: Assertion[]) => void;
}

// 断言类型选项
const assertionTypeOptions = [
  { label: 'JSON Path', value: 'json_path', description: '检查 JSON 响应中的特定字段' },
  { label: 'Latency', value: 'latency', description: '检查响应延迟' },
  { label: 'Status Code', value: 'status_code', description: '检查 HTTP 状态码' },
];

// 操作符选项
const operatorOptions = [
  { label: '==', value: 'eq' },
  { label: '!=', value: 'ne' },
  { label: '>', value: 'gt' },
  { label: '>=', value: 'gte' },
  { label: '<', value: 'lt' },
  { label: '<=', value: 'lte' },
  { label: 'contains', value: 'contains' },
  { label: 'not empty', value: 'not_empty' },
];

// JSONPath 常用表达式模板
const jsonPathTemplates = [
  { label: 'ret_code == 0', value: '$.ret_code', expected: 0, operator: 'eq' },
  { label: 'total >= 10', value: '$.total', expected: 10, operator: 'gte' },
  { label: 'item_list not empty', value: '$.item_list', operator: 'not_empty' },
  { label: 'graph_version > 0', value: '$.graph_version', expected: 0, operator: 'gt' },
];

function AssertionEditor({ assertions, onChange }: AssertionEditorProps) {
  const addAssertion = () => {
    onChange([
      ...assertions,
      { type: 'json_path', operator: 'eq' }
    ]);
  };
  
  const updateAssertion = (index: number, updated: Partial<Assertion>) => {
    const newAssertions = [...assertions];
    newAssertions[index] = { ...newAssertions[index], ...updated };
    onChange(newAssertions);
  };
  
  const removeAssertion = (index: number) => {
    onChange(assertions.filter((_, i) => i !== index));
  };
  
  return (
    <div className="assertion-editor">
      <h4>响应断言</h4>
      <p className="hint">配置响应验证规则，用于自动化测试</p>
      
      {assertions.length === 0 ? (
        <p className="empty">暂无断言配置，点击下方按钮添加</p>
      ) : (
        <div className="assertions-list">
          {assertions.map((assertion, index) => (
            <div key={index} className="assertion-item">
              <select
                value={assertion.type}
                onChange={(e) => updateAssertion(index, { type: e.target.value as any })}
              >
                {assertionTypeOptions.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              
              {assertion.type === 'json_path' && (
                <>
                  <input
                    type="text"
                    placeholder="JSONPath 表达式"
                    value={assertion.expression || ''}
                    onChange={(e) => updateAssertion(index, { expression: e.target.value })}
                  />
                  <select
                    value={assertion.operator}
                    onChange={(e) => updateAssertion(index, { operator: e.target.value as any })}
                  >
                    {operatorOptions.map(opt => (
                      <option key={opt.value} value={opt.value}>{opt.label}</option>
                    ))}
                  </select>
                  <input
                    type="text"
                    placeholder="期望值"
                    value={assertion.expected || ''}
                    onChange={(e) => updateAssertion(index, { expected: e.target.value })}
                  />
                </>
              )}
              
              {assertion.type === 'latency' && (
                <>
                  <select
                    value={assertion.operator}
                    onChange={(e) => updateAssertion(index, { operator: e.target.value as any })}
                  >
                    {operatorOptions.filter(o => ['eq','ne','gt','gte','lt','lte'].includes(o.value))
                      .map(opt => (
                        <option key={opt.value} value={opt.value}>{opt.label}</option>
                      ))}
                  </select>
                  <input
                    type="number"
                    placeholder="毫秒"
                    value={assertion.expected || ''}
                    onChange={(e) => updateAssertion(index, { expected: e.target.value })}
                  />
                </>
              )}
              
              <button onClick={() => removeAssertion(index)} className="remove-btn">
                删除
              </button>
            </div>
          ))}
        </div>
      )}
      
      <div className="assertion-actions">
        <button onClick={addAssertion}>+ 添加断言</button>
        <div className="templates">
          <span>快速添加：</span>
          {jsonPathTemplates.map((template, index) => (
            <button
              key={index}
              onClick={() => addAssertionWithTemplate(template)}
              className="template-btn"
            >
              {template.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function addAssertionWithTemplate(template: any) {
  onChange([
    ...assertions,
    {
      type: 'json_path',
      expression: template.value,
      operator: template.operator,
      expected: template.expected,
    }
  ]);
}
```

## 7. 依赖要求

需要在 `backend/go.mod` 中添加以下依赖：

```bash
# 使用 @latest 或指定版本号
go get git.code.oa.com/tme/going/codec/qzh@latest
go get git.code.oa.com/tme/tme-protocol/th@latest
go get git.code.oa.com/tme/tme-protocol/thc@latest
go get git.code.oa.com/tme/kgjce2go/proto_union_recall_merge_handler@latest

# 或者指定具体版本（推荐用于生产环境）
# go get git.code.oa.com/tme/going/codec/qzh@v1.2.3
go get git.code.oa.com/tme/tme-protocol/th@v1.2.3
go get git.code.oa.com/tme/tme-protocol/thc@v1.2.3
go get git.code.oa.com/tme/kgjce2go/proto_union_recall_merge_handler@v1.2.3
```

**版本管理建议**：
- **开发环境**：使用 `@latest` 获取最新版本
- **生产环境**：使用 `@vX.X.X` 指定具体版本，确保依赖一致性
- **CI/CD**：在 CI 流程中固定依赖版本，避免意外升级

## 8. 实现计划

### Phase 1: 后端插件实现（2 小时）

#### 1.1 创建插件文件

- [ ] 创建 `backend/internal/rpc/union_recall_types.go`
- [ ] 创建 `backend/internal/rpc/union_recall_plugin.go`

#### 1.2 修改现有文件

- [ ] 修改 `backend/internal/rpc/errors.go` 追加 UnionRecall 错误码（4600-4699）
- [ ] 修改 `backend/internal/api/runs.go`：
  - 在 `supportedProtocols` map 中添加 `"UnionRecall": true`
  - 确保 `executePluginSample` 函数能正确路由到 UnionRecall 插件

#### 1.3 验证

- [ ] 编译验证
- [ ] 单元测试

### Phase 2: 前端适配（0.5 小时）

#### 2.1 协议支持

- [ ] 更新 `protocolOptions` 添加 UnionRecall 选项
- [ ] 更新 `isExecutableScenarioFlowStep` 函数支持 UnionRecall 验证
- [ ] 更新 `getProtocolConfigFields` 添加 UnionRecall 配置字段

#### 2.2 FlowSteps 支持

- [ ] 修改 `executableScenarioFlowSteps` 函数，确保支持 UnionRecall 协议的 FlowStep
- [ ] 添加 UnionRecall 协议的 FlowStep 渲染组件

#### 2.3 断言配置界面

- [ ] 在场景编辑器中添加断言配置区域
- [ ] 实现断言类型选择器（json_path、latency、status_code）
- [ ] 实现 JSONPath 表达式编辑器
- [ ] 实现断言操作符选择器（eq、ne、gt、gte、lt、lte、contains、not_empty）
- [ ] 实现断言验证功能

#### 2.4 验证

- [ ] 编译验证
- [ ] UI 测试

### Phase 3: 测试验证（0.5 小时）

- [ ] 创建测试场景
- [ ] 执行压测验证
- [ ] 验证响应格式
- [ ] 验证断言配置
- [ ] 验证错误处理

## 9. 错误处理

Union Recall 插件使用统一的错误码体系，范围为 `4600-4699`，与 `rpc-plugin-design.md` 保持一致。详见第 10 章"错误码定义"。

## 10. 错误码定义

Union Recall 插件使用统一的错误码体系，范围为 `4600-4699`，与 `rpc-plugin-design.md` 保持一致。

| 错误码 | 数值 | 说明 |
|--------|------|------|
| UNIONRECALL_TARGET_NOT_SPECIFIED | 4601 | 目标服务地址未指定（缺少 X-Target-Address） |
| UNIONRECALL_CALL_FAILED | 4602 | tme-protocol 调用失败 |
| UNIONRECALL_BODY_PARSE_FAILED | 4603 | 请求体解析失败 |
| UNIONRECALL_CONFIG_MISSING | 4604 | 必需配置参数缺失（如 X-Service-Name） |
| UNIONRECALL_TIMEOUT | 4605 | 调用超时 |
| UNIONRECALL_SERVICE_NOT_FOUND | 4606 | 服务名未找到 |

### 错误码常量定义（Go 代码）

```go
// backend/internal/rpc/errors.go 追加

// Union Recall 相关错误 (4600-4699)
const (
	ErrCodeUnionRecallTargetNotSpecified = 4601
	ErrCodeUnionRecallCallFailed          = 4602
	ErrCodeUnionRecallBodyParseFailed     = 4603
	ErrCodeUnionRecallConfigMissing       = 4604
	ErrCodeUnionRecallTimeout             = 4605
	ErrCodeUnionRecallServiceNotFound     = 4606
)

// 预定义错误消息（追加）
var errorMessages = map[int]string{
	// ... 现有错误消息 ...
	ErrCodeUnionRecallTargetNotSpecified: "UnionRecall target not specified",
	ErrCodeUnionRecallCallFailed:         "UnionRecall call failed",
	ErrCodeUnionRecallBodyParseFailed:    "UnionRecall body parse failed",
	ErrCodeUnionRecallConfigMissing:      "UnionRecall config missing",
	ErrCodeUnionRecallTimeout:            "UnionRecall call timeout",
	ErrCodeUnionRecallServiceNotFound:    "UnionRecall service not found",
}

// NewUnionRecallConfigMissingError 创建 UnionRecall 配置缺失错误
func NewUnionRecallConfigMissingError(param string) *RPCError {
	return NewRPCError(ErrCodeUnionRecallConfigMissing,
		fmt.Sprintf("required parameter missing: %s", param))
}

// NewUnionRecallCallFailedError 创建 UnionRecall 调用失败错误
func NewUnionRecallCallFailedError(detail string) *RPCError {
	return NewRPCError(ErrCodeUnionRecallCallFailed, detail)
}
```

## 11. 后续优化

1. **支持 trpc-go 协议**：添加 `X-Protocol: trpc` 配置，支持 trpc-go 客户端
2. **支持 FlatBuffers**：添加 `X-Protocol: flatbuffers` 配置，支持 FlatBuffers 协议
3. **连接池管理**：复用 tme-protocol 连接，提升性能
4. **监控上报**：集成监控上报，统计调用成功率、延迟等
5. **断言配置支持**：支持响应断言，用于自动化测试
