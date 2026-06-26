import {
  CopyOutlined,
  DashboardOutlined,
  DownloadOutlined,
  FileTextOutlined,
  NodeIndexOutlined,
  PlayCircleOutlined,
  ProfileOutlined,
} from '@ant-design/icons';
import { Badge, Button, Checkbox, ConfigProvider, Form, Input, Layout, Menu, Modal, Select, Tag, Typography } from 'antd';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import './App.css';

type PageKey = 'dashboard' | 'targets' | 'scenarios' | 'runs' | 'reports';

type BilingualText = {
  zh: string;
  en: string;
};

type WorkspaceModule = {
  title: BilingualText;
  description: BilingualText;
  status: BilingualText;
  meta: string;
};

type WorkspacePage = {
  key: PageKey;
  label: BilingualText;
  hash: string;
  icon: ReactNode;
  summary: BilingualText;
  modules: WorkspaceModule[];
};

type HeaderPair = {
  key: string;
  value: string;
};

type QueryVariant = {
  name: string;
  weight: string;
  queryParams: string;
};

type BodyVariant = {
  name: string;
  weight: string;
  body: string;
};

type ScenarioVariableExtractor = {
  name: string;
  source: string;
  path: string;
};

type CopyCommandStatus = 'copied' | 'failed';

function copyCommandStatusLabel(status?: CopyCommandStatus): string {
  if (status === 'copied') {
    return '已复制 / Copied';
  }
  if (status === 'failed') {
    return '复制失败 / Copy failed';
  }
  return '';
}

type ScenarioResponseAssertion = {
  name: string;
  source: string;
  path?: string;
  operator: string;
  expected?: string;
};

type ScenarioFlowStep = {
  id: string;
  name: string;
  type: string;
  enabled?: boolean;
  protocol?: string;
  method?: string;
  path?: string;
  assertion?: string;
  headers?: HeaderPair[];
  extractors?: ScenarioVariableExtractor[];
  assertions?: ScenarioResponseAssertion[];
  when?: ScenarioResponseAssertion;
};

type ScenarioDraft = {
  id: string;
  name: string;
  projectId: string;
  environment: string;
  protocol: string;
  method: string;
  baseUrl: string;
  path: string;
  queryVariants: QueryVariant[];
  headers: HeaderPair[];
  bodyVariants: BodyVariant[];
  flowSteps: ScenarioFlowStep[];
  timeoutMs: string;
  retryCount: string;
  assertion: string;
};

type ScenarioFormValues = Omit<ScenarioDraft, 'id' | 'flowSteps'>;

type CreateRunPayload = {
  scenarioId: string;
  targetId?: string;
  totalRequests: number;
  concurrency: number;
  timeoutMs: number;
  maxErrorRatePercent?: number;
  maxP95LatencyMs?: number;
};

type RunResult = {
  id: string;
  scenarioId?: string;
  scenarioName?: string;
  targetId?: string;
  targetName?: string;
  name: string;
  status: string;
  method: string;
  url: string;
  totalRequests: number;
  successRequests: number;
  failedRequests: number;
  durationMs: number;
  qps: number;
  averageLatencyMs: number;
  p95LatencyMs: number;
  createdAt?: string;
  profileArtifacts?: ProfileArtifactRecord[];
};

type RunEventRecord = {
  id: string;
  runId: string;
  type: string;
  status: string;
  message: string;
  successRequests: number;
  failedRequests: number;
  totalRequests: number;
  qps: number;
  p95LatencyMs: number;
  createdAt: string;
};

type RunReportRecord = {
  run: RunResult;
  events: RunEventRecord[];
  profileArtifacts: ProfileArtifactRecord[];
  alerts?: RunReportAlert[];
  targetMetrics?: RunTargetMetricsReport | null;
  slowSamples?: RunRequestSampleRecord[];
  errorSamples?: RunRequestSampleRecord[];
  targetHealthChecks?: TargetHealthCheckResult[];
  summary: {
    successRatePercent: number;
    errorRatePercent: number;
    eventCount: number;
    profileArtifactCount: number;
    slowSampleCount?: number;
    errorSampleCount?: number;
    alertCount?: number;
  };
};

type RunReportAlert = {
  id: string;
  severity: string;
  kind: string;
  metric: string;
  threshold?: number;
  observed?: number;
  message: string;
  eventId?: string;
  createdAt?: string;
};

type RunTargetMetricsReport = {
  targetId: string;
  targetName: string;
  agentIds: string[];
  from: string;
  to: string;
  sampleCount: number;
  metricThresholds?: TargetMetricThresholds;
  cpuMaxPercent?: number;
  memoryMaxPercent?: number;
  diskReadMaxBytesPerSec?: number;
  diskWriteMaxBytesPerSec?: number;
  networkRxMaxBytesPerSec?: number;
  networkTxMaxBytesPerSec?: number;
  processMatch?: TargetProcessMatch;
  samples?: AgentMetrics[];
  processTrends?: RunProcessTrend[];
  latestProcessSnapshot: AgentProcessMetric[];
};

type RunProcessTrend = {
  agentId: string;
  pid: number;
  name: string;
  cmdline?: string;
  sampleCount: number;
  cpuMaxPercent?: number;
  memoryRssMaxBytes?: number;
  fdMaxCount?: number;
  threadMaxCount?: number;
  firstSeenAt?: string;
  lastSeenAt?: string;
};

type RunRequestSampleRecord = {
  id: string;
  runId: string;
  kind: string;
  method: string;
  url: string;
  statusCode: number;
  success: boolean;
  latencyMs: number;
  error?: string;
  createdAt: string;
};

type RunComparisonReport = {
  generatedAt: string;
  runs: RunComparisonRunRecord[];
  summary: {
    runCount: number;
    bestQpsRunId: string;
    fastestP95RunId: string;
    highestErrorRateRunId: string;
  };
};

type RunComparisonRunRecord = {
  id: string;
  name: string;
  scenarioName?: string;
  targetName?: string;
  status: string;
  totalRequests: number;
  successRequests: number;
  failedRequests: number;
  successRatePercent: number;
  errorRatePercent: number;
  durationMs: number;
  qps: number;
  averageLatencyMs: number;
  p95LatencyMs: number;
  createdAt: string;
};

type ProfileArtifactRecord = {
  id: string;
  runId: string;
  scenarioId?: string;
  scenarioName?: string;
  targetId?: string;
  targetName?: string;
  profileType: string;
  status: string;
  sourceUrl?: string;
  fileName: string;
  contentType?: string;
  sizeBytes: number;
  error?: string;
  startedAt: string;
  finishedAt: string;
};

type ProfileTaskRecord = {
  id: string;
  agentId: string;
  runId: string;
  scenarioId?: string;
  scenarioName?: string;
  targetId?: string;
  targetName?: string;
  pprofBaseUrl?: string;
  profileUrl?: string;
  profileType: string;
  profileSeconds: number;
  profileCommand?: string;
  profileCommandArgs?: string[];
  profileCommandOutput?: string;
  profileCommandTimeoutMs?: number;
  source?: string;
  status: string;
  attempts?: number;
  maxAttempts?: number;
  artifactId?: string;
  error?: string;
  createdAt: string;
  updatedAt: string;
  leasedAt?: string;
  leaseExpiresAt?: string;
  completedAt?: string;
};

type ProfileTaskTemplateRecord = {
  id: string;
  name: string;
  description?: string;
  kind: 'pprof' | 'command' | string;
  profileTypes?: string[];
  profileType?: string;
  requiresProfileEndpoint?: boolean;
  requiresPid?: boolean;
  profileCommand?: string;
  profileCommandArgs?: string[];
  profileCommandOutput?: string;
  timeoutBufferSeconds?: number;
  enabled?: boolean;
  displayOrder?: number;
  createdAt?: string;
  updatedAt?: string;
};

type ProfileArtifactComparisonReport = {
  generatedAt: string;
  groups: ProfileArtifactComparisonGroup[];
  summary: {
    runCount: number;
    artifactCount: number;
    collectedCount: number;
    failedCount: number;
    totalSizeBytes: number;
  };
};

type ProfileArtifactComparisonGroup = {
  profileType: string;
  artifactCount: number;
  collectedCount: number;
  failedCount: number;
  totalSizeBytes: number;
  latestRunId: string;
  latestArtifactId: string;
  latestSizeBytes?: number;
  previousSizeBytes?: number;
  sizeDeltaBytes?: number;
  sizeDeltaPercent?: number;
  latestStatus?: string;
  previousStatus?: string;
  statusChanged?: boolean;
  artifacts: ProfileArtifactRecord[];
};

type ProcessTrendComparisonReport = {
  generatedAt: string;
  groups: ProcessTrendComparisonGroup[];
  summary: {
    runCount: number;
    processCount: number;
    highestCpuProcessKey: string;
    highestMemoryProcessKey: string;
  };
};

type ProcessTrendComparisonGroup = {
  key: string;
  targetId?: string;
  targetName?: string;
  agentId: string;
  name: string;
  cmdline?: string;
  runCount: number;
  sampleCount: number;
  cpuMaxPercent?: number;
  memoryRssMaxBytes?: number;
  fdMaxCount?: number;
  threadMaxCount?: number;
  latestRunId: string;
  latestPid: number;
  latestCpuMaxPercent?: number;
  latestMemoryRssMaxBytes?: number;
  previousRunId?: string;
  previousPid?: number;
  previousCpuMaxPercent?: number;
  previousMemoryRssMaxBytes?: number;
  cpuDeltaPercent?: number;
  memoryRssDeltaBytes?: number;
  latestLastSeenAt?: string;
  runs: ProcessTrendComparisonRun[];
};

type ProcessTrendComparisonRun = {
  runId: string;
  runName: string;
  targetId?: string;
  targetName?: string;
  agentId: string;
  pid: number;
  sampleCount: number;
  cpuMaxPercent?: number;
  memoryRssMaxBytes?: number;
  fdMaxCount?: number;
  threadMaxCount?: number;
  firstSeenAt?: string;
  lastSeenAt?: string;
  createdAt?: string;
};

declare global {
  interface Window {
    __AIT_LATEST_RUN_REPORT__?: RunReportRecord;
    __AIT_REPORT_BOOTSTRAP__?: (payload: RunReportRecord) => void;
  }
}

type AgentRecord = {
  id: string;
  tokenId?: string;
  projectId?: string;
  environment?: string;
  name: string;
  hostname: string;
  ip: string;
  version: string;
  latestVersion?: string;
  upgradeAvailable?: boolean;
  status: string;
  labels: Record<string, string>;
  capabilities: string[];
  lastSeenAt: string;
  latestMetrics?: AgentMetrics;
};

type AgentTokenRecord = {
  id: string;
  name: string;
  projectId?: string;
  environment?: string;
  token?: string;
  status: string;
  expiresAt?: string;
  createdAt: string;
  updatedAt?: string;
  lastUsedAt?: string;
  rotatedAt?: string;
};

type AgentMetrics = {
  agentId: string;
  collectedAt: string;
  cpuUsagePercent: number;
  memoryUsagePercent: number;
  diskReadBytesPerSec: number;
  diskWriteBytesPerSec: number;
  networkRxBytesPerSec: number;
  networkTxBytesPerSec: number;
  processes: AgentProcessMetric[];
};

type AgentProcessMetric = {
  pid: number;
  name: string;
  cmdline?: string;
  cpuUsagePercent: number;
  memoryRssBytes: number;
  fdCount: number;
  threadCount: number;
};

type DashboardAgentRow = {
  name: string;
  zone: string;
  cpu: string;
  mem: string;
  io: string;
  state: BilingualText;
  status?: string;
};

type DashboardRecentRun = {
  id: string;
  scenario: string;
  result: BilingualText;
  latency: string;
  status?: string;
};

type TargetRecord = {
  id: string;
  name: string;
  projectId: string;
  baseUrl: string;
  environment: string;
  agentIds: string[];
  profileEndpoint: string;
  processMatch: TargetProcessMatch;
  healthCheck?: TargetHealthCheck;
  lastHealthCheck?: TargetHealthCheckResult;
  metricThresholds?: TargetMetricThresholds;
  createdAt?: string;
  updatedAt?: string;
};

type TargetHealthCheck = {
  enabled?: boolean;
  path?: string;
  expectedStatus?: number;
  timeoutMs?: number;
};

type TargetHealthCheckResult = {
  targetId: string;
  targetName: string;
  status: string;
  source?: string;
  url: string;
  expectedStatus: number;
  observedStatus: number;
  latencyMs: number;
  error?: string;
  checkedAt?: string;
};

type TargetProcessMatch = {
  name: string;
  cmdlineContains: string;
};

type TargetMetricThresholds = {
  cpuMaxPercent?: number;
  memoryMaxPercent?: number;
  diskReadMaxBytesPerSec?: number;
  diskWriteMaxBytesPerSec?: number;
  networkRxMaxBytesPerSec?: number;
  networkTxMaxBytesPerSec?: number;
};

type WorkspaceScope = {
  projectId: string;
  environment: string;
};

const defaultWorkspaceScope: WorkspaceScope = {
  projectId: 'default',
  environment: 'default',
};

const defaultScenarioValues: ScenarioFormValues = {
  name: '',
  projectId: defaultWorkspaceScope.projectId,
  environment: defaultWorkspaceScope.environment,
  protocol: 'HTTP',
  method: 'GET',
  baseUrl: 'http://127.0.0.1:8080',
  path: '/api/health',
  queryVariants: [{ name: 'default', weight: '100', queryParams: '' }],
  headers: [{ key: 'Content-Type', value: 'application/json' }],
  bodyVariants: [{ name: 'default', weight: '100', body: '' }],
  timeoutMs: '1000',
  retryCount: '0',
  assertion: 'status < 400',
};

const defaultScenarioFlowSteps: ScenarioFlowStep[] = [
  { id: 'step-http-login', name: 'HTTP login', type: 'request', protocol: 'HTTP', method: 'POST', path: '/login' },
  {
    id: 'step-extract-token',
    name: 'extract token',
    type: 'extract',
    enabled: false,
    extractors: [{ name: 'token', source: 'json', path: 'token' }],
  },
  { id: 'step-rpc-create-order', name: 'RPC createOrder', type: 'request', protocol: 'CUSTOM_RPC', method: 'POST', path: '/createOrder' },
  {
    id: 'step-assert-status',
    name: 'assert status',
    type: 'assertion',
    enabled: false,
    assertion: 'status < 400',
    assertions: [{ name: 'assert status', source: 'json', path: 'state', operator: 'equals', expected: 'ok' }],
  },
];

const scenarioDraftsStorageKey = 'all-in-one-testing.scenario-drafts.v1';
const scenarioApiBaseURL = (import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const agentInstallScriptURL = `${scenarioApiBaseURL}/agent/install.sh`;

function normalizeWorkspaceScope(scope: Partial<WorkspaceScope> | null | undefined): WorkspaceScope {
  return {
    projectId: scope?.projectId?.trim() || defaultWorkspaceScope.projectId,
    environment: scope?.environment?.trim() || defaultWorkspaceScope.environment,
  };
}

function backendURLWithWorkspaceScope(baseURL: string, scope?: WorkspaceScope | null): string {
  if (!scope) {
    return baseURL;
  }
  const query = new URLSearchParams();
  const normalizedScope = normalizeWorkspaceScope(scope);
  query.set('projectId', normalizedScope.projectId);
  query.set('environment', normalizedScope.environment);
  return `${baseURL}${baseURL.includes('?') ? '&' : '?'}${query.toString()}`;
}

function recordMatchesWorkspaceScope(record: { projectId?: string; environment?: string }, scope?: WorkspaceScope | null): boolean {
  if (!scope) {
    return true;
  }
  const normalizedScope = normalizeWorkspaceScope(scope);
  return (record.projectId || defaultWorkspaceScope.projectId) === normalizedScope.projectId
    && (record.environment || defaultWorkspaceScope.environment) === normalizedScope.environment;
}

function workspaceScopeHeaders(scope?: WorkspaceScope | null): Record<string, string> | undefined {
  if (!scope) {
    return undefined;
  }
  const normalizedScope = normalizeWorkspaceScope(scope);
  return {
    'X-AIT-Project-ID': normalizedScope.projectId,
    'X-AIT-Environment': normalizedScope.environment,
  };
}

function headersToRecord(headers?: HeadersInit): Record<string, string> {
  if (!headers) {
    return {};
  }
  if (typeof Headers !== 'undefined' && headers instanceof Headers) {
    const record: Record<string, string> = {};
    headers.forEach((value, key) => {
      record[key] = value;
    });
    return record;
  }
  if (Array.isArray(headers)) {
    return Object.fromEntries(headers);
  }
  const recordHeaders = headers as Record<string, string>;
  return Object.fromEntries(Object.entries(recordHeaders).filter(([, value]) => typeof value === 'string'));
}

function fetchWithWorkspaceScope(input: RequestInfo | URL, scope?: WorkspaceScope | null, init?: RequestInit) {
  const scopeHeaders = workspaceScopeHeaders(scope);
  if (!scopeHeaders) {
    return init === undefined ? fetch(input) : fetch(input, init);
  }
  return fetch(input, {
    ...init,
    headers: {
      ...headersToRecord(init?.headers),
      ...scopeHeaders,
    },
  });
}

function agentInstallCommand(token?: string): string {
  const baseCommand = `curl -fsSL ${agentInstallScriptURL} | sudo bash -s -- --server ${scenarioApiBaseURL}`;
  return token ? `${baseCommand} --token ${token}` : `${baseCommand} --token <agent-token>`;
}

function agentUpgradeCommand(agent: AgentRecord): string {
  return `${agentInstallCommand()} --agent-id ${agent.id} --force-download`;
}

function agentInstallCopyKey(token: AgentTokenRecord | null): string {
  if (!token?.token) {
    return '';
  }
  const tokenVersion = token.updatedAt || token.rotatedAt || token.expiresAt || token.createdAt;
  return `agent-install-${token.id}-${tokenVersion}`;
}

async function copyToClipboard(text: string): Promise<boolean> {
  if (!navigator.clipboard?.writeText) {
    return false;
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Clipboard writes can be denied by browser permissions; copying is best-effort.
    return false;
  }
}

type PlatformResponse = Pick<Response, 'ok' | 'status' | 'json' | 'text'>;
let jsonpRequestCounter = 0;

function requestURL(input: RequestInfo | URL): string {
  if (typeof Request !== 'undefined' && input instanceof Request) {
    return input.url;
  }
  return String(input);
}

function requestMethod(input: RequestInfo | URL, init?: RequestInit): string {
  if (init?.method) {
    return init.method;
  }
  if (typeof Request !== 'undefined' && input instanceof Request) {
    return input.method;
  }
  return 'GET';
}

function applyRequestHeaders(xhr: XMLHttpRequest, headers?: HeadersInit) {
  if (!headers) {
    return;
  }
  if (typeof Headers !== 'undefined' && headers instanceof Headers) {
    headers.forEach((value, key) => xhr.setRequestHeader(key, value));
    return;
  }
  if (Array.isArray(headers)) {
    headers.forEach(([key, value]) => xhr.setRequestHeader(key, value));
    return;
  }
  Object.entries(headers).forEach(([key, value]) => xhr.setRequestHeader(key, value));
}

function xhrFetch(input: RequestInfo | URL, init?: RequestInit): Promise<PlatformResponse> {
  if (typeof XMLHttpRequest === 'undefined') {
    return Promise.reject(new Error('No HTTP request API is available in this browser.'));
  }

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(requestMethod(input, init), requestURL(input), true);
    applyRequestHeaders(xhr, init?.headers);
    xhr.onload = () => {
      const responseText = xhr.responseText || '';
      resolve({
        ok: xhr.status >= 200 && xhr.status < 300,
        status: xhr.status,
        json: async () => (responseText ? JSON.parse(responseText) : null),
        text: async () => responseText,
      });
    };
    xhr.onerror = () => reject(new Error(`Network request failed for ${requestURL(input)}`));
    xhr.send(init?.body as XMLHttpRequestBodyInit | null | undefined);
  });
}

function appendJSONPCallback(rawURL: string, callbackName: string): string {
  const hashIndex = rawURL.indexOf('#');
  const baseURL = hashIndex >= 0 ? rawURL.slice(0, hashIndex) : rawURL;
  const hash = hashIndex >= 0 ? rawURL.slice(hashIndex) : '';
  const separator = baseURL.includes('?') ? '&' : '?';
  return `${baseURL}${separator}callback=${encodeURIComponent(callbackName)}${hash}`;
}

function jsonpFetch(input: RequestInfo | URL, init?: RequestInit): Promise<PlatformResponse> {
  if (requestMethod(input, init).toUpperCase() !== 'GET') {
    return Promise.reject(new Error('JSONP fallback only supports GET requests.'));
  }
  if (typeof document === 'undefined') {
    return Promise.reject(new Error('JSONP fallback requires a document.'));
  }

  return new Promise((resolve, reject) => {
    const callbackName = `__ait_jsonp_${Date.now()}_${jsonpRequestCounter++}`;
    const jsonpGlobal = globalThis as typeof globalThis & Record<string, (payload: unknown) => void>;
    const script = document.createElement('script');
    const requestURLValue = requestURL(input);

    const cleanup = () => {
      delete jsonpGlobal[callbackName];
      script.remove();
    };

    jsonpGlobal[callbackName] = (payload: unknown) => {
      cleanup();
      const responseText = JSON.stringify(payload);
      resolve({
        ok: true,
        status: 200,
        json: async () => payload,
        text: async () => responseText,
      });
    };
    script.onerror = () => {
      cleanup();
      reject(new Error(`JSONP request failed for ${requestURL(input)}`));
    };
    script.src = appendJSONPCallback(requestURLValue, callbackName);
    (document.head || document.documentElement).appendChild(script);
  });
}

function platformFetch(input: RequestInfo | URL, init?: RequestInit): Promise<PlatformResponse> {
  if (typeof globalThis.fetch === 'function') {
    return init === undefined ? globalThis.fetch(input) : globalThis.fetch(input, init);
  }
  if (typeof XMLHttpRequest !== 'undefined') {
    return xhrFetch(input, init);
  }
  return jsonpFetch(input, init);
}

const fetch = platformFetch;

function readBootstrapRunReport(): RunReportRecord | null {
  const bootstrapGlobal = globalThis as typeof globalThis & { __AIT_LATEST_RUN_REPORT__?: RunReportRecord };
  return bootstrapGlobal.__AIT_LATEST_RUN_REPORT__ || null;
}

const protocolOptions = [
  { label: 'HTTP', value: 'HTTP', description: '标准 HTTP 协议' },
  { label: 'Custom RPC (HTTP Adapter)', value: 'CUSTOM_RPC', description: '自定义 RPC 协议，通过 HTTP Adapter 调用' },
  { label: 'gRPC', value: 'gRPC', description: 'gRPC 协议，推荐使用 HTTP Adapter + grpc-gateway' },
  { label: 'Dubbo', value: 'Dubbo', description: 'Dubbo 协议，暂未实现' },
  { label: 'Thrift', value: 'Thrift', description: 'Thrift 协议，暂未实现' },
];

const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((method) => ({
  label: method,
  value: method,
}));

const extractorSourceOptions = [
  { label: 'JSON', value: 'json' },
  { label: 'Header', value: 'header' },
  { label: 'Regex', value: 'regex' },
];

const assertionSourceOptions = [
  { label: 'JSON', value: 'json' },
  { label: 'Header', value: 'header' },
  { label: 'Body', value: 'body' },
];

const conditionSourceOptions = [
  { label: 'JSON', value: 'json' },
  { label: 'Header', value: 'header' },
  { label: 'Body', value: 'body' },
  { label: 'Regex', value: 'regex' },
  { label: 'Variable', value: 'variable' },
];

const assertionOperatorOptions = [
  { label: 'equals', value: 'equals' },
  { label: 'not equals', value: 'not_equals' },
  { label: 'contains', value: 'contains' },
  { label: 'not contains', value: 'not_contains' },
  { label: 'matches', value: 'matches' },
  { label: 'not matches', value: 'not_matches' },
  { label: 'exists', value: 'exists' },
];

const productTitle = { zh: '一体化压测平台', en: 'All-in-One Testing' };
const schedulerStatus = { zh: '调度器在线', en: 'Scheduler online' };

const workspacePages: WorkspacePage[] = [
  {
    key: 'dashboard',
    label: { zh: '仪表盘', en: 'Dashboard' },
    hash: '#dashboard',
    icon: <DashboardOutlined />,
    summary: { zh: '查看实时压测、目标机器、Agent 与画像采样状态。', en: 'Monitor live runs, target machines, agents, and profile sampling.' },
    modules: [],
  },
  {
    key: 'targets',
    label: { zh: '目标与 Agent', en: 'Targets & Agents' },
    hash: '#targets',
    icon: <NodeIndexOutlined />,
    summary: { zh: '管理被压测服务、主机 Agent、状态采集和画像端口。', en: 'Manage target services, host agents, status collection, and profile endpoints.' },
    modules: [
      {
        title: { zh: 'Agent 接入', en: 'Agent onboarding' },
        description: { zh: '生成安装命令、登记实例身份，并确认心跳上报。', en: 'Generate install commands, register instance identity, and confirm heartbeat reporting.' },
        status: { zh: '8 台在线', en: '8 online' },
        meta: 'agentd 0.2.1',
      },
      {
        title: { zh: '主机状态采集', en: 'Host status collection' },
        description: { zh: '采集 CPU、内存、磁盘 IO、网络吞吐和进程级资源占用。', en: 'Collect CPU, memory, disk IO, network throughput, and process-level resource usage.' },
        status: { zh: '30 秒间隔', en: '30s interval' },
        meta: 'cpu mem io net proc',
      },
      {
        title: { zh: '目标绑定', en: 'Target binding' },
        description: { zh: '把服务、机器、端口和压测场景绑定到同一拓扑。', en: 'Bind services, machines, ports, and scenarios into one topology.' },
        status: { zh: '3 个服务', en: '3 services' },
        meta: 'gateway search checkout',
      },
      {
        title: { zh: '画像端口', en: 'Profile endpoints' },
        description: { zh: '登记 pprof 或 prof 入口，压测中按策略抓取样本。', en: 'Register pprof or prof endpoints and collect samples during load runs.' },
        status: { zh: '2 个入口', en: '2 endpoints' },
        meta: ':6060/debug/pprof',
      },
    ],
  },
  {
    key: 'scenarios',
    label: { zh: '场景', en: 'Scenarios' },
    hash: '#scenarios',
    icon: <ProfileOutlined />,
    summary: { zh: '编排 HTTP 与自定义 RPC 调用，配置变量、断言和数据提取。', en: 'Compose HTTP and custom RPC calls with variables, assertions, and extractors.' },
    modules: [
      {
        title: { zh: 'HTTP 步骤', en: 'HTTP steps' },
        description: { zh: '配置方法、路径、请求体、Header、Cookie 和重试策略。', en: 'Configure method, path, body, headers, cookies, and retry policy.' },
        status: { zh: '可编辑', en: 'Editable' },
        meta: 'GET POST PUT DELETE',
      },
      {
        title: { zh: '自定义 RPC', en: 'Custom RPC' },
        description: { zh: '通过适配器执行用户自定义协议，并复用统一压测接口。', en: 'Run user-defined protocols through adapters while reusing the shared load API.' },
        status: { zh: '适配器模式', en: 'Adapter mode' },
        meta: 'grpc dubbo thrift custom',
      },
      {
        title: { zh: '变量流转', en: 'Variable flow' },
        description: { zh: '从响应中提取 token、订单号或业务字段给后续步骤使用。', en: 'Extract tokens, order IDs, or business fields for later steps.' },
        status: { zh: '链路级', en: 'Flow-level' },
        meta: 'jsonpath regex header',
      },
      {
        title: { zh: '断言', en: 'Assertions' },
        description: { zh: '校验状态码、响应字段、错误率阈值和业务成功条件。', en: 'Validate status codes, response fields, error budgets, and business success rules.' },
        status: { zh: '已启用', en: 'Enabled' },
        meta: 'status json latency',
      },
    ],
  },
  {
    key: 'runs',
    label: { zh: '压测运行', en: 'Runs' },
    hash: '#runs',
    icon: <PlayCircleOutlined />,
    summary: { zh: '启动、观察和停止压测任务，跟踪实时指标与阶段计划。', en: 'Start, observe, and stop load runs while tracking live metrics and stage plans.' },
    modules: [
      {
        title: { zh: '启动控制', en: 'Launch control' },
        description: { zh: '选择场景、目标集群、压测模式和运行时长。', en: 'Choose scenario, target cluster, load mode, and run duration.' },
        status: { zh: '待执行', en: 'Ready' },
        meta: 'concurrency qps ramp',
      },
      {
        title: { zh: '阶段计划', en: 'Stage schedule' },
        description: { zh: '支持预热、升压、保持、降压和冷却阶段。', en: 'Support warm-up, ramp-up, hold, ramp-down, and cooldown stages.' },
        status: { zh: '4 阶段', en: '4 stages' },
        meta: '5m 10m 20m 5m',
      },
      {
        title: { zh: '实时指标', en: 'Live metrics' },
        description: { zh: '持续观察 QPS、p95、p99、错误率和目标机器状态。', en: 'Continuously watch QPS, p95, p99, error rate, and target machine status.' },
        status: { zh: '刷新中', en: 'Refreshing' },
        meta: '2s refresh',
      },
      {
        title: { zh: '中止策略', en: 'Abort policy' },
        description: { zh: '当错误率、延迟或机器资源超过阈值时自动停止。', en: 'Stop automatically when error, latency, or host resource thresholds are breached.' },
        status: { zh: '已配置', en: 'Configured' },
        meta: 'error p99 cpu mem',
      },
    ],
  },
  {
    key: 'reports',
    label: { zh: '报告', en: 'Reports' },
    hash: '#reports',
    icon: <FileTextOutlined />,
    summary: { zh: '复盘压测结果、目标状态、错误样本和性能画像产物。', en: 'Review run results, target status, error samples, and profile artifacts.' },
    modules: [
      {
        title: { zh: '延迟时间线', en: 'Latency timeline' },
        description: { zh: '按阶段对比 p50、p95、p99 与错误率的变化。', en: 'Compare p50, p95, p99, and error-rate changes by stage.' },
        status: { zh: '已生成', en: 'Generated' },
        meta: 'p50 p95 p99',
      },
      {
        title: { zh: '画像产物', en: 'Profile artifacts' },
        description: { zh: '归档 CPU、内存、goroutine、火焰图和阻塞分析。', en: 'Archive CPU, memory, goroutine, flamegraph, and blocking analysis.' },
        status: { zh: '6 个样本', en: '6 samples' },
        meta: 'cpu heap goroutine block',
      },
      {
        title: { zh: '目标机器指标', en: 'Target metrics' },
        description: { zh: '展示 Agent 采集到的主机资源和关键进程指标。', en: 'Show host resources and key process metrics collected by agents.' },
        status: { zh: '已关联', en: 'Linked' },
        meta: 'cpu mem io network',
      },
      {
        title: { zh: '瓶颈摘要', en: 'Bottleneck summary' },
        description: { zh: '聚合慢接口、热点函数、资源峰值和建议动作。', en: 'Summarize slow endpoints, hot functions, resource peaks, and recommended actions.' },
        status: { zh: '待确认', en: 'Review needed' },
        meta: 'hot path cpu bound',
      },
    ],
  },
];

const statusMetrics = [
  { label: { zh: '当前 QPS', en: 'Current QPS' }, value: '12,480', note: '+8.4%', tone: 'green' },
  { label: { zh: 'p95 延迟', en: 'p95 latency' }, value: '184 ms', note: '-21 ms', tone: 'blue' },
  { label: { zh: '错误率', en: 'Error rate' }, value: '0.18%', note: 'SLO 0.5%', tone: 'amber' },
  { label: { zh: '在线 Agent', en: 'Online agents' }, value: '8 / 9', note: '1 degraded', tone: 'red' },
];

const runStages = [
  { name: { zh: '预热', en: 'Warm-up' }, value: 100 },
  { name: { zh: '升压', en: 'Ramp-up' }, value: 100 },
  { name: { zh: '保持', en: 'Hold' }, value: 64 },
  { name: { zh: '冷却', en: 'Cooldown' }, value: 0 },
];

const agentRows: DashboardAgentRow[] = [
  { name: 'api-gateway-01', zone: 'shanghai-a', cpu: '42%', mem: '61%', io: '18 MB/s', state: { zh: '健康', en: 'Healthy' } },
  { name: 'checkout-02', zone: 'shanghai-b', cpu: '78%', mem: '69%', io: '31 MB/s', state: { zh: '观察', en: 'Watching' } },
  { name: 'search-03', zone: 'shanghai-c', cpu: '35%', mem: '54%', io: '11 MB/s', state: { zh: '健康', en: 'Healthy' } },
];

const profileRows = [
  { label: { zh: 'CPU pprof', en: 'CPU pprof' }, value: { zh: '采样中 60s', en: 'Sampling 60s' } },
  { label: { zh: 'Heap prof', en: 'Heap prof' }, value: { zh: '下次 14:35', en: 'Next 14:35' } },
  { label: { zh: '火焰图', en: 'Flamegraph' }, value: { zh: '等待聚合', en: 'Aggregation queued' } },
];

const recentRuns: DashboardRecentRun[] = [
  { id: 'RUN-2407', scenario: 'checkout-mixed-rpc', result: { zh: '通过', en: 'Passed' }, latency: 'p95 184 ms' },
  { id: 'RUN-2406', scenario: 'search-http-burst', result: { zh: '需复盘', en: 'Review' }, latency: 'p95 431 ms' },
  { id: 'RUN-2405', scenario: 'gateway-smoke', result: { zh: '通过', en: 'Passed' }, latency: 'p95 96 ms' },
];

const pageKeys = new Set<PageKey>(workspacePages.map((page) => page.key));

function pageFromHash(hash: string): PageKey {
  const normalized = hash.replace('#', '') as PageKey;
  return pageKeys.has(normalized) ? normalized : 'dashboard';
}

function bilingual(text: BilingualText): string {
  return `${text.zh} / ${text.en}`;
}

function normalizePath(path: string): string {
  const trimmedPath = path.trim();
  return trimmedPath.startsWith('/') ? trimmedPath : `/${trimmedPath}`;
}

function normalizeQueryParams(queryParams: string): string {
  return queryParams.trim().replace(/^\?/, '');
}

function normalizeQueryVariants(queryVariants: QueryVariant[] = []): QueryVariant[] {
  return queryVariants
    .map((variant, index) => ({
      name: (variant.name || `query-${index + 1}`).trim(),
      weight: (variant.weight || '100').trim(),
      queryParams: normalizeQueryParams(variant.queryParams || ''),
    }))
    .filter((variant) => variant.queryParams.length > 0);
}

function normalizeHeaders(headers: HeaderPair[] = []): HeaderPair[] {
  return headers
    .map((header) => ({
      key: (header.key || '').trim(),
      value: (header.value || '').trim(),
    }))
    .filter((header) => header.key.length > 0 || header.value.length > 0);
}

function normalizeBodyVariants(bodyVariants: BodyVariant[] = []): BodyVariant[] {
  return bodyVariants
    .map((variant, index) => ({
      name: (variant.name || `body-${index + 1}`).trim(),
      weight: (variant.weight || '100').trim(),
      body: (variant.body || '').trim(),
    }))
    .filter((variant) => variant.body.length > 0);
}

function normalizeScenarioFlowSteps(flowSteps: ScenarioFlowStep[] = []): ScenarioFlowStep[] {
  return flowSteps
    .map((step, index) => ({
      id: (step.id || `step-${index + 1}`).trim(),
      name: (step.name || `step ${index + 1}`).trim(),
      type: (step.type || 'request').trim().toLowerCase(),
      enabled: Boolean(step.enabled),
      protocol: step.protocol?.trim().toUpperCase(),
      method: step.method?.trim().toUpperCase(),
      path: step.path?.trim(),
      assertion: step.assertion?.trim(),
      extractors: normalizeScenarioExtractors(step.extractors),
      assertions: normalizeScenarioAssertions(step.assertions),
      when: normalizeScenarioCondition(step.when),
    }))
    .filter((step) => step.name.length > 0);
}

function normalizeScenarioExtractors(extractors: ScenarioVariableExtractor[] = []): ScenarioVariableExtractor[] {
  return extractors
    .map((extractor) => ({
      name: (extractor.name || '').trim(),
      source: (extractor.source || 'json').trim().toLowerCase(),
      path: (extractor.path || '').trim(),
    }))
    .filter((extractor) => extractor.name.length > 0 && extractor.path.length > 0);
}

function primaryScenarioExtractor(step: ScenarioFlowStep): ScenarioVariableExtractor {
  const extractor = step.extractors?.[0];
  return {
    name: extractor?.name ?? 'token',
    source: extractor?.source ?? 'json',
    path: extractor?.path ?? 'token',
  };
}

function patchPrimaryScenarioExtractor(step: ScenarioFlowStep, patch: Partial<ScenarioVariableExtractor>): ScenarioVariableExtractor[] {
  return [{ ...primaryScenarioExtractor(step), ...patch }];
}

function normalizeScenarioAssertionOperator(operator = ''): string {
  const normalized = operator.trim().toLowerCase().replace(/[-\s]+/g, '_');
  if (['', 'eq', 'equal', 'equals', '==', '='].includes(normalized)) {
    return 'equals';
  }
  if (['ne', 'not_equal', 'not_equals', 'notequals', '!='].includes(normalized)) {
    return 'not_equals';
  }
  if (['contains', 'include', 'includes'].includes(normalized)) {
    return 'contains';
  }
  if (['not_contains', 'notcontains', 'excludes'].includes(normalized)) {
    return 'not_contains';
  }
  if (['matches', 'match', 'regex', '=~'].includes(normalized)) {
    return 'matches';
  }
  if (['not_matches', 'notmatches', 'not_match', '!~'].includes(normalized)) {
    return 'not_matches';
  }
  if (['exists', 'exist'].includes(normalized)) {
    return 'exists';
  }
  return normalized;
}

function normalizeScenarioAssertions(assertions: ScenarioResponseAssertion[] = []): ScenarioResponseAssertion[] {
  return assertions
    .map((assertion) => {
      const source = (assertion.source || 'json').trim().toLowerCase();
      const path = (assertion.path || '').trim();
      return {
        name: (assertion.name || [source, path].filter(Boolean).join(' ') || 'response assertion').trim(),
        source,
        path,
        operator: normalizeScenarioAssertionOperator(assertion.operator),
        expected: (assertion.expected || '').trim(),
      };
    })
    .filter((assertion) => {
      if (assertion.source !== 'body' && assertion.path.length === 0) {
        return false;
      }
      return assertion.operator === 'exists' || assertion.expected.length > 0;
    });
}

function normalizeScenarioCondition(condition?: ScenarioResponseAssertion): ScenarioResponseAssertion | undefined {
  if (!condition) {
    return undefined;
  }
  const normalized = normalizeScenarioAssertions([{ ...condition, name: condition.name || 'when condition' }]);
  return normalized[0];
}

function primaryScenarioAssertion(step: ScenarioFlowStep): ScenarioResponseAssertion {
  const assertion = step.assertions?.[0];
  return {
    name: assertion?.name ?? step.name ?? 'response assertion',
    source: assertion?.source ?? 'json',
    path: assertion?.path ?? 'state',
    operator: assertion?.operator ?? 'equals',
    expected: assertion?.expected ?? 'ok',
  };
}

function patchPrimaryScenarioAssertion(step: ScenarioFlowStep, patch: Partial<ScenarioResponseAssertion>): ScenarioResponseAssertion[] {
  return [{ ...primaryScenarioAssertion(step), ...patch }];
}

function primaryScenarioCondition(step: ScenarioFlowStep): ScenarioResponseAssertion {
  const condition = step.when;
  return {
    name: condition?.name ?? 'when condition',
    source: condition?.source ?? 'json',
    path: condition?.path ?? 'state',
    operator: condition?.operator ?? 'equals',
    expected: condition?.expected ?? 'ok',
  };
}

function patchScenarioCondition(step: ScenarioFlowStep, patch: Partial<ScenarioResponseAssertion>): ScenarioResponseAssertion {
  return { ...primaryScenarioCondition(step), ...patch };
}

function cloneScenarioFlowSteps(flowSteps: ScenarioFlowStep[]): ScenarioFlowStep[] {
  return flowSteps.map((step) => ({
    ...step,
    extractors: normalizeScenarioExtractors(step.extractors),
    assertions: normalizeScenarioAssertions(step.assertions),
    when: normalizeScenarioCondition(step.when),
  }));
}

function createCustomScenarioFlowStep(index: number): ScenarioFlowStep {
  return {
    id: `step-custom-${index}`,
    name: 'custom request',
    type: 'request',
    enabled: false,
    protocol: 'HTTP',
    method: 'GET',
    path: '/',
  };
}

function scenarioFlowStepLabel(step: ScenarioFlowStep, index: number): string {
  return `${String(index + 1).padStart(2, '0')} ${step.name}`;
}

function isExecutableScenarioFlowStep(step: ScenarioFlowStep): boolean {
  const type = (step.type || '').toLowerCase();
  const protocol = (step.protocol || 'HTTP').toUpperCase();
  
  // 支持的协议列表
  const supportedProtocols = ['HTTP', 'CUSTOM_RPC', 'gRPC', 'Dubbo', 'Thrift'];
  
  // 检查协议是否支持
  if (!supportedProtocols.includes(protocol)) {
    return false;
  }
  
  // 必须是 request 类型
  if (type !== 'request') {
    return false;
  }
  
  // gRPC 协议需要目标地址
  if (protocol === 'gRPC') {
    const headers = step.headers || [];
    const hasTarget = headers.some((h: { key: string; value: string }) => h.key === 'X-GRPC-Target');
    if (!hasTarget) {
      return false;
    }
  }
  
  // CUSTOM_RPC 和 gRPC 需要 method
  if (protocol === 'CUSTOM_RPC' || protocol === 'gRPC') {
    if (!step.method?.trim()) {
      return false;
    }
  }
  
  // HTTP 协议需要 path
  if (protocol === 'HTTP') {
    return Boolean(step.enabled) && Boolean(step.path?.trim());
  }
  
  // 其他协议需要 path 或 method
  return Boolean(step.enabled) && (Boolean(step.path?.trim()) || Boolean(step.method?.trim()));
}

function isActiveScenarioFlowStep(step: ScenarioFlowStep): boolean {
  const type = (step.type || '').toLowerCase();
  if (isExecutableScenarioFlowStep(step)) {
    return true;
  }
  if (type === 'extract') {
    return Boolean(step.enabled) && normalizeScenarioExtractors(step.extractors).length > 0;
  }
  if (type === 'assertion') {
    return Boolean(step.enabled) && (Boolean(step.assertion?.trim()) || normalizeScenarioAssertions(step.assertions).length > 0);
  }
  return false;
}

function executableScenarioFlowSteps(flowSteps: ScenarioFlowStep[] = []): ScenarioFlowStep[] {
  return flowSteps.filter(isExecutableScenarioFlowStep);
}

function runFlowStepLabel(step: ScenarioFlowStep, index: number): string {
  return `${String(index + 1).padStart(2, '0')} ${(step.method || 'GET').toUpperCase()} ${step.path || '/'}`;
}

function scenarioFlowStepDisplayLabel(step: ScenarioFlowStep, index: number): string {
  return (step.type || '').toLowerCase() === 'request' ? runFlowStepLabel(step, index) : scenarioFlowStepLabel(step, index);
}

function shouldShowScenarioFlowStepName(step: ScenarioFlowStep): boolean {
  return (step.type || '').toLowerCase() === 'request' && Boolean(step.name?.trim());
}

function scenarioFlowStepStatusLabel(step: ScenarioFlowStep): string {
  const type = (step.type || '').toLowerCase();
  if (type === 'extract' && isActiveScenarioFlowStep(step)) {
    return '提取 / Extract';
  }
  if (type === 'assertion' && isActiveScenarioFlowStep(step)) {
    return '断言 / Assert';
  }
  return isExecutableScenarioFlowStep(step) ? '执行 / Execute' : '草稿 / Draft';
}

function scenarioFlowStepExtractorSummary(step: ScenarioFlowStep): string {
  return normalizeScenarioExtractors(step.extractors)
    .map((extractor) => `${extractor.name} <- ${extractor.source} ${extractor.path}`)
    .join(', ');
}

function scenarioFlowStepAssertionSummary(step: ScenarioFlowStep): string {
  return normalizeScenarioAssertions(step.assertions)
    .map((assertion) => {
      const target = assertion.source === 'body' ? 'body' : `${assertion.source} ${assertion.path}`;
      return `${assertion.name}: ${target} ${assertion.operator} ${assertion.expected || ''}`.trim();
    })
    .join(', ');
}

function scenarioFlowStepWhenSummary(step: ScenarioFlowStep): string {
  const condition = normalizeScenarioCondition(step.when);
  if (!condition) {
    return '';
  }
  const target = condition.source === 'body' ? 'body' : `${condition.source} ${condition.path}`;
  return `when ${target} ${condition.operator} ${condition.expected || ''}`.trim();
}

function normalizeScenarioDraft(draft: ScenarioDraft): ScenarioDraft {
  return {
    ...draft,
    projectId: (draft.projectId || defaultScenarioValues.projectId).trim(),
    environment: (draft.environment || defaultScenarioValues.environment).trim(),
    queryVariants: Array.isArray(draft.queryVariants) ? draft.queryVariants : [],
    headers: Array.isArray(draft.headers) ? draft.headers : [],
    bodyVariants: Array.isArray(draft.bodyVariants) ? draft.bodyVariants : [],
    flowSteps: normalizeScenarioFlowSteps(Array.isArray(draft.flowSteps) ? draft.flowSteps : []),
  };
}

function loadStoredScenarioDrafts(): ScenarioDraft[] {
  try {
    const rawDrafts = window.localStorage.getItem(scenarioDraftsStorageKey);
    if (!rawDrafts) {
      return [];
    }

    const parsedDrafts = JSON.parse(rawDrafts);
    if (!Array.isArray(parsedDrafts)) {
      return [];
    }

    return parsedDrafts
      .filter((draft): draft is ScenarioDraft => Boolean(draft) && typeof draft === 'object')
      .map(normalizeScenarioDraft);
  } catch {
    return [];
  }
}

function saveScenarioDrafts(scenarioDrafts: ScenarioDraft[]) {
  try {
    window.localStorage.setItem(scenarioDraftsStorageKey, JSON.stringify(scenarioDrafts));
  } catch {
    // Local persistence is a convenience layer; storage failures should not block editing.
  }
}

function mergeScenarioDrafts(primaryDrafts: ScenarioDraft[], fallbackDrafts: ScenarioDraft[]): ScenarioDraft[] {
  const seen = new Set<string>();
  const mergedDrafts: ScenarioDraft[] = [];

  for (const draft of [...primaryDrafts, ...fallbackDrafts]) {
    const key = draft.id || draft.name;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    mergedDrafts.push(draft);
  }

  return mergedDrafts;
}

async function loadBackendScenarioDrafts(scope?: WorkspaceScope | null): Promise<ScenarioDraft[]> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/scenarios`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load scenarios with HTTP ${response.status}`);
  }
  const scenarioDrafts = await response.json() as ScenarioDraft[];
  return Array.isArray(scenarioDrafts) ? scenarioDrafts.map(normalizeScenarioDraft) : [];
}

async function createBackendScenarioDraft(scenario: ScenarioDraft, scope?: WorkspaceScope | null): Promise<ScenarioDraft> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/scenarios`, scope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(scenario),
  });
  if (!response.ok) {
    throw new Error(`Failed to create scenario with HTTP ${response.status}`);
  }
  const createdScenario = await response.json() as ScenarioDraft;
  return normalizeScenarioDraft(createdScenario);
}

async function createBackendRun(payload: CreateRunPayload, scope?: WorkspaceScope | null): Promise<RunResult> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs`, scope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    let message = `Failed to create run with HTTP ${response.status}`;
    try {
      const body = await response.json() as { error?: string };
      if (typeof body.error === 'string' && body.error.trim()) {
        message = body.error.trim();
      }
    } catch {
      // Fall back to the status-only message when the backend did not return JSON.
    }
    throw new Error(message);
  }
  return response.json() as Promise<RunResult>;
}

async function loadBackendRuns(scope?: WorkspaceScope | null): Promise<RunResult[]> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load runs with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunResult[]>;
}

async function loadBackendRun(runID: string, scope?: WorkspaceScope | null): Promise<RunResult> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs/${runID}`, scope);
  if (!response.ok) {
    throw new Error(`Failed to load run ${runID} with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunResult>;
}

async function stopBackendRun(runID: string, scope?: WorkspaceScope | null): Promise<RunResult> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs/${runID}/stop`, scope, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(`Failed to stop run ${runID} with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunResult>;
}

async function loadBackendRunEvents(runID: string, scope?: WorkspaceScope | null): Promise<RunEventRecord[]> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs/${runID}/events`, scope);
  if (!response.ok) {
    throw new Error(`Failed to load run events ${runID} with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunEventRecord[]>;
}

async function loadBackendRunReport(runID: string, scope?: WorkspaceScope | null): Promise<RunReportRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/reports/runs/${runID}`, scope);
  if (!response.ok) {
    throw new Error(`Failed to load run report ${runID} with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunReportRecord>;
}

async function loadBackendRunComparison(limit = 5, scope?: WorkspaceScope | null): Promise<RunComparisonReport> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/reports/compare?limit=${limit}`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load run comparison with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunComparisonReport>;
}

async function loadBackendProfileArtifactComparison(limit = 5, scope?: WorkspaceScope | null): Promise<ProfileArtifactComparisonReport> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/reports/profile-artifacts/compare?limit=${limit}`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load profile artifact comparison with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileArtifactComparisonReport>;
}

async function loadBackendProcessTrendComparison(limit = 5, scope?: WorkspaceScope | null): Promise<ProcessTrendComparisonReport> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/reports/process-trends/compare?limit=${limit}`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load process trend comparison with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProcessTrendComparisonReport>;
}

const runEventStreamTypes = ['run_started', 'run_progress', 'run_finished', 'run_canceled', 'run_aborted'] as const;

function backendRunEventsStreamURL(runID: string, scope?: WorkspaceScope | null): string {
  return backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/runs/${runID}/events/stream`, scope);
}

function mergeRunEvent(currentEvents: RunEventRecord[], nextEvent: RunEventRecord): RunEventRecord[] {
  const existingIndex = currentEvents.findIndex((event) => event.id === nextEvent.id);
  if (existingIndex >= 0) {
    return currentEvents.map((event, index) => (index === existingIndex ? nextEvent : event));
  }
  return [...currentEvents, nextEvent];
}

function isRunActive(status: string) {
  return status === 'queued' || status === 'running' || status === 'stopping';
}

async function loadBackendAgents(scope?: WorkspaceScope | null): Promise<AgentRecord[]> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/agents`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load agents with HTTP ${response.status}`);
  }
  return response.json() as Promise<AgentRecord[]>;
}

async function loadBackendAgentMetrics(agentID: string, scope?: WorkspaceScope | null): Promise<AgentMetrics[]> {
  const query = new URLSearchParams({
    agentId: agentID,
    limit: '120',
  });
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/agent-metrics?${query.toString()}`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load agent metrics with HTTP ${response.status}`);
  }
  return response.json() as Promise<AgentMetrics[]>;
}

async function loadBackendTargets(scope?: WorkspaceScope | null): Promise<TargetRecord[]> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load targets with HTTP ${response.status}`);
  }
  return response.json() as Promise<TargetRecord[]>;
}

async function loadBackendProfileArtifacts(scope?: WorkspaceScope | null): Promise<ProfileArtifactRecord[]> {
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/profile-artifacts`, scope), scope);
  if (!response.ok) {
    throw new Error(`Failed to load profile artifacts with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileArtifactRecord[]>;
}

async function loadBackendProfileTasks(
  options: { runId?: string; agentId?: string; status?: string; source?: string; limit?: number; workspaceScope?: WorkspaceScope | null } = {},
): Promise<ProfileTaskRecord[]> {
  const query = new URLSearchParams();
  if (options.runId) {
    query.set('runId', options.runId);
  }
  if (options.agentId) {
    query.set('agentId', options.agentId);
  }
  if (options.status) {
    query.set('status', options.status);
  }
  if (options.source) {
    query.set('source', options.source);
  }
  if (options.limit) {
    query.set('limit', String(options.limit));
  }
  const requestURL = `${scenarioApiBaseURL}/api/profile-tasks${query.size > 0 ? `?${query.toString()}` : ''}`;
  const response = await fetchWithWorkspaceScope(backendURLWithWorkspaceScope(requestURL, options.workspaceScope), options.workspaceScope);
  if (!response.ok) {
    throw new Error(`Failed to load profile tasks with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskRecord[]>;
}

async function retryBackendProfileTask(taskID: string, scope?: WorkspaceScope | null): Promise<ProfileTaskRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/profile-tasks/${encodeURIComponent(taskID)}/retry`, scope, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(`Failed to retry profile task with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskRecord>;
}

async function createBackendProfileTask(task: Partial<ProfileTaskRecord>, scope?: WorkspaceScope | null): Promise<ProfileTaskRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/profile-tasks`, scope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(task),
  });
  if (!response.ok) {
    throw new Error(`Failed to create profile task with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskRecord>;
}

async function loadBackendProfileTaskTemplates(): Promise<ProfileTaskTemplateRecord[]> {
  const response = await fetch(`${scenarioApiBaseURL}/api/profile-task-templates`);
  if (!response.ok) {
    throw new Error(`Failed to load profile task templates with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskTemplateRecord[]>;
}

async function createBackendProfileTaskTemplate(template: Partial<ProfileTaskTemplateRecord>): Promise<ProfileTaskTemplateRecord> {
  const response = await fetch(`${scenarioApiBaseURL}/api/profile-task-templates`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(template),
  });
  if (!response.ok) {
    throw new Error(`Failed to create profile task template with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskTemplateRecord>;
}

async function updateBackendProfileTaskTemplate(templateID: string, template: Partial<ProfileTaskTemplateRecord>): Promise<ProfileTaskTemplateRecord> {
  const response = await fetch(`${scenarioApiBaseURL}/api/profile-task-templates/${encodeURIComponent(templateID)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(template),
  });
  if (!response.ok) {
    throw new Error(`Failed to update profile task template with HTTP ${response.status}`);
  }
  return response.json() as Promise<ProfileTaskTemplateRecord>;
}

async function deleteBackendProfileTaskTemplate(templateID: string): Promise<void> {
  const response = await fetch(`${scenarioApiBaseURL}/api/profile-task-templates/${encodeURIComponent(templateID)}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw new Error(`Failed to delete profile task template with HTTP ${response.status}`);
  }
}

function profileArtifactDownloadURL(artifactID: string, scope?: WorkspaceScope | null): string {
  return backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/profile-artifacts/${encodeURIComponent(artifactID)}/download`, scope);
}

const agentTokenDefaultExpiresInSeconds = 24 * 60 * 60;
const profileTaskDefaultMaxAttempts = 3;
const profileTaskDefaultSeconds = '30';
type ProfileTaskDraft = {
  template: string;
  profileType: string;
  profileSeconds: string;
  commandPid: string;
};
type ProfileTaskTemplateDraft = {
  id: string;
  name: string;
  description: string;
  kind: string;
  profileTypes: string;
  profileType: string;
  requiresProfileEndpoint: boolean;
  requiresPid: boolean;
  profileCommand: string;
  profileCommandArgs: string;
  profileCommandOutput: string;
  timeoutBufferSeconds: string;
  displayOrder: string;
};
const profileTaskDefaultDraft: ProfileTaskDraft = {
  template: 'go_pprof',
  profileType: 'cpu',
  profileSeconds: profileTaskDefaultSeconds,
  commandPid: '',
};
const profileTaskTemplateDefaultDraft: ProfileTaskTemplateDraft = {
  id: '',
  name: '',
  description: '',
  kind: 'command',
  profileTypes: 'perf',
  profileType: 'perf',
  requiresProfileEndpoint: false,
  requiresPid: true,
  profileCommand: '',
  profileCommandArgs: '["record","-F","99","-p","{{pid}}","-g","-o","{{output}}","--","sleep","{{seconds}}"]',
  profileCommandOutput: '/tmp/{{targetNameSlug}}-profile.data',
  timeoutBufferSeconds: '15',
  displayOrder: '100',
};
const profileTaskTypeOptions = ['cpu', 'heap', 'goroutine', 'mutex', 'block', 'allocs', 'threadcreate'];
const fallbackProfileTaskTemplates: ProfileTaskTemplateRecord[] = [
  {
    id: 'go_pprof',
    name: 'Go pprof',
    description: 'Collect Go runtime profiles from a target pprof endpoint.',
    kind: 'pprof',
    profileTypes: ['cpu', 'heap', 'goroutine', 'allocs', 'mutex', 'block'],
    requiresProfileEndpoint: true,
    requiresPid: false,
    enabled: true,
    displayOrder: 10,
  },
  {
    id: 'linux_perf',
    name: 'Linux perf',
    description: 'Run a fixed perf record command against a target process PID.',
    kind: 'command',
    profileTypes: ['perf'],
    profileType: 'perf',
    requiresProfileEndpoint: false,
    requiresPid: true,
    profileCommand: 'perf',
    profileCommandArgs: ['record', '-F', '99', '-p', '{{pid}}', '-g', '-o', '{{output}}', '--', 'sleep', '{{seconds}}'],
    profileCommandOutput: '/tmp/{{targetNameSlug}}-perf.data',
    timeoutBufferSeconds: 15,
    enabled: true,
    displayOrder: 20,
  },
];

function profileCommandOutputName(targetName: string): string {
  const normalizedName = targetName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return normalizedName || 'target';
}

function renderProfileTaskTemplateValue(value: string, targetName: string, pid: string): string {
  return value.replaceAll('{{targetNameSlug}}', profileCommandOutputName(targetName)).replaceAll('{{pid}}', pid);
}

function profileTaskTemplateProfileTypes(template: ProfileTaskTemplateRecord): string[] {
  const profileTypes = template.profileTypes?.filter(Boolean) || [];
  if (profileTypes.length > 0) {
    return profileTypes;
  }
  if (template.profileType) {
    return [template.profileType];
  }
  return profileTaskTypeOptions;
}

function buildCommandTemplateTaskFields(template: ProfileTaskTemplateRecord, targetName: string, profileSeconds: number, pid: string): Pick<
  ProfileTaskRecord,
  'profileType' | 'profileCommand' | 'profileCommandArgs' | 'profileCommandOutput' | 'profileCommandTimeoutMs'
> {
  const timeoutBufferSeconds = Math.max(0, template.timeoutBufferSeconds ?? 0);
  return {
    profileType: template.profileType || profileTaskTemplateProfileTypes(template)[0] || 'command',
    profileCommand: template.profileCommand || '',
    profileCommandArgs: (template.profileCommandArgs || []).map((arg) => renderProfileTaskTemplateValue(arg, targetName, pid)),
    profileCommandOutput: renderProfileTaskTemplateValue(template.profileCommandOutput || '', targetName, pid),
    profileCommandTimeoutMs: (profileSeconds + timeoutBufferSeconds) * 1000,
  };
}

function profileTaskTemplateDraftFromRecord(template: ProfileTaskTemplateRecord): ProfileTaskTemplateDraft {
  return {
    id: template.id,
    name: template.name,
    description: template.description || '',
    kind: template.kind || 'command',
    profileTypes: (template.profileTypes || []).join(', '),
    profileType: template.profileType || profileTaskTemplateProfileTypes(template)[0] || '',
    requiresProfileEndpoint: Boolean(template.requiresProfileEndpoint),
    requiresPid: Boolean(template.requiresPid),
    profileCommand: template.profileCommand || '',
    profileCommandArgs: JSON.stringify(template.profileCommandArgs || []),
    profileCommandOutput: template.profileCommandOutput || '',
    timeoutBufferSeconds: optionalNumberInputValue(template.timeoutBufferSeconds),
    displayOrder: optionalNumberInputValue(template.displayOrder),
  };
}

function parseProfileTaskTemplateTypes(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseProfileTaskTemplateCommandArgs(value: string): string[] {
  const parsed = JSON.parse(value || '[]');
  if (!Array.isArray(parsed) || parsed.some((item) => typeof item !== 'string')) {
    throw new Error('Profile template command args must be a JSON string array.');
  }
  return parsed;
}

async function createBackendAgentToken(
  name: string,
  options: { projectId?: string; environment?: string; expiresInSeconds?: number; workspaceScope?: WorkspaceScope | null } = {},
): Promise<AgentTokenRecord> {
  const projectId = options.projectId?.trim() || 'default';
  const environment = options.environment?.trim() || 'default';
  const expiresInSeconds = options.expiresInSeconds ?? agentTokenDefaultExpiresInSeconds;
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/agent-tokens`, options.workspaceScope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name, projectId, environment, expiresInSeconds }),
  });
  if (!response.ok) {
    throw new Error(`Failed to create agent token with HTTP ${response.status}`);
  }
  return response.json() as Promise<AgentTokenRecord>;
}

async function revokeBackendAgentToken(tokenID: string, scope?: WorkspaceScope | null): Promise<AgentTokenRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/agent-tokens/${encodeURIComponent(tokenID)}/revoke`, scope, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(`Failed to revoke agent token with HTTP ${response.status}`);
  }
  return response.json() as Promise<AgentTokenRecord>;
}

async function rotateBackendAgentToken(
  tokenID: string,
  expiresInSeconds = agentTokenDefaultExpiresInSeconds,
  scope?: WorkspaceScope | null,
): Promise<AgentTokenRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/agent-tokens/${encodeURIComponent(tokenID)}/rotate`, scope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ expiresInSeconds }),
  });
  if (!response.ok) {
    throw new Error(`Failed to rotate agent token with HTTP ${response.status}`);
  }
  return response.json() as Promise<AgentTokenRecord>;
}

async function createBackendTarget(target: Omit<TargetRecord, 'id'>, scope?: WorkspaceScope | null): Promise<TargetRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets`, scope, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(target),
  });
  if (!response.ok) {
    throw new Error(`Failed to create target with HTTP ${response.status}`);
  }
  return response.json() as Promise<TargetRecord>;
}

async function updateBackendTarget(
  targetID: string,
  target: Omit<TargetRecord, 'id'>,
  scope?: WorkspaceScope | null,
): Promise<TargetRecord> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets/${encodeURIComponent(targetID)}`, scope, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(target),
  });
  if (!response.ok) {
    throw new Error(`Failed to update target with HTTP ${response.status}`);
  }
  return response.json() as Promise<TargetRecord>;
}

async function deleteBackendTarget(targetID: string, scope?: WorkspaceScope | null): Promise<void> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets/${encodeURIComponent(targetID)}`, scope, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw new Error(`Failed to delete target with HTTP ${response.status}`);
  }
}

async function checkBackendTargetHealth(targetID: string, scope?: WorkspaceScope | null): Promise<TargetHealthCheckResult> {
  const response = await fetchWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets/${encodeURIComponent(targetID)}/health-check`, scope, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(`Failed to check target health with HTTP ${response.status}`);
  }
  return response.json() as Promise<TargetHealthCheckResult>;
}

async function loadBackendTargetHealthChecks(targetID: string, limit = 5, scope?: WorkspaceScope | null): Promise<TargetHealthCheckResult[]> {
  const response = await fetchWithWorkspaceScope(
    backendURLWithWorkspaceScope(`${scenarioApiBaseURL}/api/targets/${encodeURIComponent(targetID)}/health-checks?limit=${limit}`, scope),
    scope,
  );
  if (!response.ok) {
    throw new Error(`Failed to load target health history with HTTP ${response.status}`);
  }
  return response.json() as Promise<TargetHealthCheckResult[]>;
}

function createDefaultScenarioValues(scope?: WorkspaceScope | null): ScenarioFormValues {
  const normalizedScope = normalizeWorkspaceScope(scope);
  return {
    ...defaultScenarioValues,
    projectId: normalizedScope.projectId,
    environment: normalizedScope.environment,
    queryVariants: defaultScenarioValues.queryVariants.map((variant) => ({ ...variant })),
    headers: defaultScenarioValues.headers.map((header) => ({ ...header })),
    bodyVariants: defaultScenarioValues.bodyVariants.map((variant) => ({ ...variant })),
  };
}

function validateJsonBody(_: unknown, value?: string) {
  if (!value || value.trim().length === 0) {
    return Promise.resolve();
  }

  try {
    JSON.parse(value);
    return Promise.resolve();
  } catch {
    return Promise.reject(new Error('Body must be valid JSON'));
  }
}

function scenarioRequestLine(scenario: ScenarioDraft): string {
  const baseUrl = scenario.baseUrl.trim().replace(/\/$/, '');
  const path = normalizePath(scenario.path);

  return `${scenario.method} ${baseUrl}${path}`;
}

function parseRunNumber(value: string, fallback: number): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function parseOptionalRunDecimal(value: string): number | undefined {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }
  return parsed;
}

function parseOptionalRunInteger(value: string): number | undefined {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }
  return parsed;
}

function optionalNumberInputValue(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return '';
  }
  return String(value);
}

function formatRunCreatedAt(createdAt?: string): string {
  if (!createdAt) {
    return 'just now';
  }

  const parsed = new Date(createdAt);
  if (Number.isNaN(parsed.getTime())) {
    return createdAt;
  }

  return parsed.toLocaleString();
}

function formatPercent(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '-';
  }
  return `${value.toFixed(1)}%`;
}

function formatBytes(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return '0 B';
  }
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function formatByteRate(value?: number): string {
  return `${formatBytes(value)}/s`;
}

function formatSignedBytes(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value === 0) {
    return '0 B';
  }
  return `${value > 0 ? '+' : '-'}${formatBytes(Math.abs(value))}`;
}

function formatSignedPercent(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value === 0) {
    return '0.0%';
  }
  return `${value > 0 ? '+' : ''}${value.toFixed(1)}%`;
}

function formatProfileArtifactStatus(group: ProfileArtifactComparisonGroup): string {
  const latestStatus = group.latestStatus || group.artifacts[0]?.status || '-';
  const previousStatus = group.previousStatus || '-';
  if (group.statusChanged) {
    return `status ${previousStatus} -> ${latestStatus}`;
  }
  return `status ${latestStatus}`;
}

function formatMetricNumber(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '-';
  }

  return value.toLocaleString('en-US', {
    minimumFractionDigits: Number.isInteger(value) ? 0 : 2,
    maximumFractionDigits: 2,
  });
}

function formatLatencyMetric(value?: number): string {
  const formattedValue = formatMetricNumber(value);
  return formattedValue === '-' ? '-' : `${formattedValue} ms`;
}

function formatErrorRateMetric(run?: RunResult): string {
  if (!run) {
    return '-';
  }

  const completedRequests = run.successRequests + run.failedRequests;
  const denominator = run.totalRequests > 0 ? run.totalRequests : completedRequests;
  if (denominator <= 0) {
    return '0.00%';
  }

  return `${((run.failedRequests / denominator) * 100).toFixed(2)}%`;
}

function runDisplayName(run?: RunResult): string {
  if (!run) {
    return 'checkout-mixed-rpc';
  }
  return run.scenarioName || run.name || run.id;
}

function runStatusLabel(status: string): BilingualText {
  switch (status) {
    case 'finished':
      return { zh: '完成', en: 'Finished' };
    case 'running':
      return { zh: '运行中', en: 'Running' };
    case 'queued':
      return { zh: '排队中', en: 'Queued' };
    case 'stopping':
      return { zh: '停止中', en: 'Stopping' };
    case 'canceled':
      return { zh: '已取消', en: 'Canceled' };
    case 'aborted':
      return { zh: 'Aborted', en: 'Aborted' };
    case 'failed':
      return { zh: '失败', en: 'Failed' };
    default:
      return { zh: status, en: status };
  }
}

function runStatusColor(status: string): string {
  if (status === 'finished') {
    return 'success';
  }
  if (isRunActive(status)) {
    return 'processing';
  }
  if (status === 'failed') {
    return 'error';
  }
  if (status === 'canceled' || status === 'aborted') {
    return 'warning';
  }
  return 'default';
}

function agentStatusLabel(status: string): BilingualText {
  if (status === 'online') {
    return { zh: '在线', en: 'Online' };
  }
  if (status === 'offline') {
    return { zh: '离线', en: 'Offline' };
  }
  return { zh: '观察', en: 'Watching' };
}

function agentStatusColor(status: string): string {
  if (status === 'online') {
    return 'success';
  }
  if (status === 'offline') {
    return 'default';
  }
  return 'warning';
}

function profileTaskStatusColor(status: string): string {
  if (status === 'completed') {
    return 'success';
  }
  if (status === 'leased') {
    return 'processing';
  }
  if (status === 'failed') {
    return 'error';
  }
  return 'warning';
}

function profileTaskDetailLabel(task: ProfileTaskRecord): string {
  if (task.error) {
    return task.error;
  }
  if (task.artifactId) {
    return task.artifactId;
  }
  if (task.pprofBaseUrl) {
    return task.pprofBaseUrl;
  }
  if (task.profileUrl) {
    return task.profileUrl;
  }
  if (task.profileCommand) {
    return [task.profileCommand, ...(task.profileCommandArgs || [])].join(' ');
  }
  return '-';
}

function buildDashboardStatusMetrics(run: RunResult | undefined, agents: AgentRecord[]) {
  const onlineAgents = agents.filter((agent) => agent.status === 'online').length;
  const offlineAgents = Math.max(agents.length - onlineAgents, 0);

  return [
    {
      label: statusMetrics[0].label,
      value: formatMetricNumber(run?.qps),
      note: run ? run.status : 'no run',
      tone: 'green',
    },
    {
      label: statusMetrics[1].label,
      value: formatLatencyMetric(run?.p95LatencyMs),
      note: run ? `avg ${formatLatencyMetric(run.averageLatencyMs)}` : 'no run',
      tone: 'blue',
    },
    {
      label: statusMetrics[2].label,
      value: formatErrorRateMetric(run),
      note: run ? `${run.failedRequests} failed` : 'no run',
      tone: run && run.failedRequests > 0 ? 'amber' : 'green',
    },
    {
      label: statusMetrics[3].label,
      value: agents.length > 0 ? `${onlineAgents} / ${agents.length}` : '0 / 0',
      note: agents.length > 0 ? `${offlineAgents} offline` : 'no agents',
      tone: agents.length === 0 || offlineAgents > 0 ? 'red' : 'green',
    },
  ];
}

function runStagesFromResult(run?: RunResult) {
  if (!run) {
    return runStages;
  }

  const totalRequests = Math.max(run.totalRequests, 1);
  const completedRequests = Math.min(run.successRequests + run.failedRequests, totalRequests);
  const successPercent = Math.round((run.successRequests / totalRequests) * 100);
  const failedPercent = Math.round((run.failedRequests / totalRequests) * 100);
  const completedPercent = Math.round((completedRequests / totalRequests) * 100);

  return [
    { name: { zh: '已完成', en: 'Completed' }, value: completedPercent },
    { name: { zh: '成功', en: 'Success' }, value: successPercent },
    { name: { zh: '失败', en: 'Failed' }, value: failedPercent },
    { name: { zh: '剩余', en: 'Remaining' }, value: Math.max(100 - completedPercent, 0) },
  ];
}

function agentToDashboardRow(agent: AgentRecord): DashboardAgentRow {
  const metrics = agent.latestMetrics;
  const ioBytesPerSecond = metrics
    ? (metrics.diskReadBytesPerSec || 0) + (metrics.diskWriteBytesPerSec || 0)
    : undefined;

  return {
    name: agent.name,
    zone: agent.labels.zone || agent.labels.region || agent.labels.service || agent.hostname || agent.ip,
    cpu: formatPercent(metrics?.cpuUsagePercent),
    mem: formatPercent(metrics?.memoryUsagePercent),
    io: typeof ioBytesPerSecond === 'number' ? `${formatBytes(ioBytesPerSecond)}/s` : '-',
    state: agentStatusLabel(agent.status),
    status: agent.status,
  };
}

function runToDashboardRecentRun(run: RunResult): DashboardRecentRun {
  const latency = formatLatencyMetric(run.p95LatencyMs);
  return {
    id: run.id,
    scenario: runDisplayName(run),
    result: runStatusLabel(run.status),
    latency: latency === '-' ? 'p95 -' : `p95 ${latency}`,
    status: run.status,
  };
}

function topAgentProcess(metrics?: AgentMetrics): AgentProcessMetric | undefined {
  if (!metrics || metrics.processes.length === 0) {
    return undefined;
  }
  return metrics.processes.reduce((currentTop, process) =>
    process.cpuUsagePercent > currentTop.cpuUsagePercent ? process : currentTop,
  );
}

function targetProcessMatchLabel(processMatch?: TargetProcessMatch): string {
  const cmdline = processMatch?.cmdlineContains?.trim();
  if (cmdline) {
    return `cmdline ${cmdline}`;
  }
  const name = processMatch?.name?.trim();
  if (name) {
    return `process ${name}`;
  }
  return '';
}

function targetMetricThresholdsLabel(thresholds?: TargetMetricThresholds): string {
  const parts: string[] = [];
  if (typeof thresholds?.cpuMaxPercent === 'number' && thresholds.cpuMaxPercent > 0) {
    parts.push(`CPU ${formatPercent(thresholds.cpuMaxPercent)}`);
  }
  if (typeof thresholds?.memoryMaxPercent === 'number' && thresholds.memoryMaxPercent > 0) {
    parts.push(`MEM ${formatPercent(thresholds.memoryMaxPercent)}`);
  }
  if (typeof thresholds?.diskReadMaxBytesPerSec === 'number' && thresholds.diskReadMaxBytesPerSec > 0) {
    parts.push(`DR ${formatByteRate(thresholds.diskReadMaxBytesPerSec)}`);
  }
  if (typeof thresholds?.diskWriteMaxBytesPerSec === 'number' && thresholds.diskWriteMaxBytesPerSec > 0) {
    parts.push(`DW ${formatByteRate(thresholds.diskWriteMaxBytesPerSec)}`);
  }
  if (typeof thresholds?.networkRxMaxBytesPerSec === 'number' && thresholds.networkRxMaxBytesPerSec > 0) {
    parts.push(`RX ${formatByteRate(thresholds.networkRxMaxBytesPerSec)}`);
  }
  if (typeof thresholds?.networkTxMaxBytesPerSec === 'number' && thresholds.networkTxMaxBytesPerSec > 0) {
    parts.push(`TX ${formatByteRate(thresholds.networkTxMaxBytesPerSec)}`);
  }
  return parts.join(', ') || '-';
}

function targetHealthCheckLabel(healthCheck?: TargetHealthCheck): string {
  const path = healthCheck?.path?.trim();
  if (!healthCheck?.enabled && !path) {
    return '-';
  }
  return `${path || '/health'} -> ${healthCheck?.expectedStatus || 200}`;
}

function targetHealthCheckResultLabel(result?: TargetHealthCheckResult): string {
  if (!result) {
    return '';
  }
  const observedStatus = result.observedStatus > 0 ? result.observedStatus : '-';
  return `${result.status} ${observedStatus}`;
}

function targetHealthCheckSourceLabel(result?: TargetHealthCheckResult): string {
  const source = result?.source?.trim();
  if (source === 'agent_daemon') {
    return 'Agent daemon';
  }
  if (source === 'control_plane') {
    return 'Control plane';
  }
  return source || '';
}

function targetHealthCheckResultClass(status?: string): string {
  if (status === 'healthy') {
    return 'target-health-result-healthy';
  }
  if (status === 'unhealthy') {
    return 'target-health-result-unhealthy';
  }
  return 'target-health-result-muted';
}

function filterProcessesByTargetMatch(processes: AgentProcessMetric[] = [], processMatch?: TargetProcessMatch): AgentProcessMetric[] {
  const cmdline = processMatch?.cmdlineContains?.trim().toLowerCase();
  if (cmdline) {
    return processes.filter((process) => (process.cmdline || '').trim().toLowerCase().includes(cmdline));
  }
  const name = processMatch?.name?.trim().toLowerCase();
  if (!name) {
    return processes;
  }
  return processes.filter((process) => process.name.trim().toLowerCase().includes(name));
}

function maxMetricValue(metrics: AgentMetrics[], selector: (metrics: AgentMetrics) => number): number | undefined {
  if (metrics.length === 0) {
    return undefined;
  }
  return metrics.reduce((currentMax, sample) => Math.max(currentMax, selector(sample)), Number.NEGATIVE_INFINITY);
}

function requestSampleStatus(sample: RunRequestSampleRecord): string {
  if (sample.statusCode > 0) {
    return String(sample.statusCode);
  }
  return sample.success ? 'OK' : 'ERR';
}

function renderRequestSampleList(
  title: string,
  emptyText: string,
  ariaPrefix: 'slow sample' | 'error sample',
  samples: RunRequestSampleRecord[],
) {
  return (
    <div className="request-sample-group">
      <div className="request-sample-heading">
        <strong>{title}</strong>
        <Tag color={samples.length > 0 ? 'processing' : 'default'}>{samples.length}</Tag>
      </div>
      <div className="request-sample-list">
        {samples.length === 0 ? (
          <p>{emptyText}</p>
        ) : (
          samples.map((sample) => (
            <div className="request-sample-row" key={sample.id} aria-label={`${ariaPrefix} ${sample.id}`}>
              <div className="request-sample-main">
                <div className="request-sample-metrics">
                  <Tag>{sample.method || '-'}</Tag>
                  <strong>{formatLatencyMetric(sample.latencyMs)}</strong>
                  <span>{requestSampleStatus(sample)}</span>
                </div>
                <span className="request-sample-url">{sample.url || '-'}</span>
                {sample.error ? <small>{sample.error}</small> : null}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function runComparisonName(run: RunComparisonRunRecord): string {
  return run.name || run.scenarioName || run.id;
}

function renderRunComparisonSummary(comparison: RunComparisonReport) {
  return (
    <div className="run-comparison-summary">
      <span>{`best qps ${comparison.summary.bestQpsRunId || '-'}`}</span>
      <span>{`fastest p95 ${comparison.summary.fastestP95RunId || '-'}`}</span>
      <span>{`highest errors ${comparison.summary.highestErrorRateRunId || '-'}`}</span>
    </div>
  );
}

function processTrendNameForKey(comparison: ProcessTrendComparisonReport, key?: string): string {
  if (!key) {
    return '-';
  }
  const group = comparison.groups.find((candidate) => candidate.key === key);
  return group?.name || key;
}

function renderProcessTrendComparisonSummary(comparison: ProcessTrendComparisonReport) {
  return (
    <div className="process-trend-comparison-summary" aria-label="process trend comparison summary">
      <span>{`runs ${comparison.summary.runCount}`}</span>
      <span>{`highest cpu ${processTrendNameForKey(comparison, comparison.summary.highestCpuProcessKey)}`}</span>
      <span>{`highest rss ${processTrendNameForKey(comparison, comparison.summary.highestMemoryProcessKey)}`}</span>
    </div>
  );
}

const percentAlertMetrics = new Set(['error_rate_percent', 'target_cpu_percent', 'target_memory_percent']);
const byteRateAlertMetrics = new Set([
  'target_disk_read_bytes_per_sec',
  'target_disk_write_bytes_per_sec',
  'target_network_rx_bytes_per_sec',
  'target_network_tx_bytes_per_sec',
]);

function formatAlertMetricValue(metric: string, value?: number): string {
  if (value === undefined || value === null) {
    return '-';
  }
  if (percentAlertMetrics.has(metric)) {
    return formatPercent(value);
  }
  if (byteRateAlertMetrics.has(metric)) {
    return formatByteRate(value);
  }
  if (metric === 'p95_latency_ms') {
    return formatLatencyMetric(value);
  }
  return formatMetricNumber(value);
}

function metricTrendPoints(metrics: AgentMetrics[], selector: (metrics: AgentMetrics) => number): string {
  if (metrics.length === 0) {
    return '';
  }
  const width = 180;
  const height = 52;
  const values = metrics.map(selector);
  const maxValue = Math.max(...values, 100);
  const denominator = Math.max(1, metrics.length - 1);
  return values
    .map((value, index) => {
      const x = (index / denominator) * width;
      const y = height - (Math.max(0, value) / maxValue) * height;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}

function MetricTrendChart({
  ariaLabel,
  label,
  samples,
  selector,
}: {
  ariaLabel: string;
  label: string;
  samples: AgentMetrics[];
  selector: (metrics: AgentMetrics) => number;
}) {
  const points = metricTrendPoints(samples, selector);
  return (
    <div className="metric-trend-card">
      <span>{label}</span>
      <svg className="metric-trend-chart" aria-label={ariaLabel} role="img" viewBox="0 0 180 52" preserveAspectRatio="none">
        <polyline points={points} />
      </svg>
    </div>
  );
}

function agentNameByID(agents: AgentRecord[], agentID: string): string {
  return agents.find((agent) => agent.id === agentID)?.name || agentID;
}

export default function App() {
  const [activePage, setActivePage] = useState<PageKey>(() => pageFromHash(window.location.hash));
  const [workspaceProjectId, setWorkspaceProjectId] = useState(defaultWorkspaceScope.projectId);
  const [workspaceEnvironment, setWorkspaceEnvironment] = useState(defaultWorkspaceScope.environment);
  const [activeWorkspaceScope, setActiveWorkspaceScope] = useState<WorkspaceScope | null>(null);
  const [isCreateScenarioOpen, setIsCreateScenarioOpen] = useState(false);
  const [scenarioDrafts, setScenarioDrafts] = useState<ScenarioDraft[]>(loadStoredScenarioDrafts);
  const [backendScenarioIds, setBackendScenarioIds] = useState<Set<string>>(() => new Set());
  const [scenarioFlowSteps, setScenarioFlowSteps] = useState<ScenarioFlowStep[]>(() => cloneScenarioFlowSteps(defaultScenarioFlowSteps));
  const [scenarioForm] = Form.useForm<ScenarioFormValues>();
  const selectedScenarioProtocol = Form.useWatch('protocol', scenarioForm) || defaultScenarioValues.protocol;
  const isCustomRPCScenario = selectedScenarioProtocol === 'CUSTOM_RPC';
  const currentPage = workspacePages.find((page) => page.key === activePage) ?? workspacePages[0];
  const effectiveWorkspaceScope = normalizeWorkspaceScope(activeWorkspaceScope);
  const scopedScenarioDrafts = scenarioDrafts.filter((scenario) => recordMatchesWorkspaceScope(scenario, activeWorkspaceScope));

  useEffect(() => {
    const handleHashChange = () => {
      setActivePage(pageFromHash(window.location.hash));
    };

    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

  useEffect(() => {
    saveScenarioDrafts(scenarioDrafts);
  }, [scenarioDrafts]);

  useEffect(() => {
    let isMounted = true;

    loadBackendScenarioDrafts(activeWorkspaceScope)
      .then((backendDrafts) => {
        if (!isMounted) {
          return;
        }
        setBackendScenarioIds(new Set(backendDrafts.map((draft) => draft.id).filter(Boolean)));
        setScenarioDrafts((currentDrafts) => mergeScenarioDrafts(backendDrafts, currentDrafts));
      })
      .catch(() => {
        // Keep the local cache as a fallback when the backend is not reachable.
      });

    return () => {
      isMounted = false;
    };
  }, [activeWorkspaceScope]);

  const navItems = useMemo(
    () =>
      workspacePages.map((page) => ({
        key: page.key,
        icon: page.icon,
        label: <a href={page.hash}>{bilingual(page.label)}</a>,
      })),
    [],
  );

  const navigateToPage = (nextPage: PageKey) => {
    const nextHash = workspacePages.find((page) => page.key === nextPage)?.hash;
    setActivePage(nextPage);
    if (nextHash) {
      window.location.hash = nextHash;
    }
  };

  const applyWorkspaceScope = () => {
    setActiveWorkspaceScope(normalizeWorkspaceScope({
      projectId: workspaceProjectId,
      environment: workspaceEnvironment,
    }));
  };

  const openCreateScenario = () => {
    navigateToPage('scenarios');
    scenarioForm.setFieldsValue(createDefaultScenarioValues(effectiveWorkspaceScope));
    setIsCreateScenarioOpen(true);
  };

  const closeCreateScenario = () => {
    setIsCreateScenarioOpen(false);
    scenarioForm.resetFields();
  };

  const addScenarioFlowStep = () => {
    setScenarioFlowSteps((currentSteps) => [...currentSteps, createCustomScenarioFlowStep(currentSteps.length + 1)]);
  };

  const updateScenarioFlowStep = (index: number, patch: Partial<ScenarioFlowStep>) => {
    setScenarioFlowSteps((currentSteps) => currentSteps.map((step, stepIndex) => (stepIndex === index ? { ...step, ...patch } : step)));
  };

  const handleCreateScenario = async (values: ScenarioFormValues) => {
    const protocol = (values.protocol || 'HTTP').trim().toUpperCase();
    const method = (values.method || (protocol === 'CUSTOM_RPC' ? '' : 'GET')).trim();
    const nextScenario: ScenarioDraft = {
      id: `scenario-${Date.now()}`,
      name: values.name.trim(),
      projectId: (values.projectId || defaultScenarioValues.projectId).trim(),
      environment: (values.environment || defaultScenarioValues.environment).trim(),
      protocol,
      method: protocol === 'CUSTOM_RPC' ? method : method.toUpperCase(),
      baseUrl: values.baseUrl.trim(),
      path: normalizePath(values.path),
      queryVariants: normalizeQueryVariants(values.queryVariants),
      headers: normalizeHeaders(values.headers),
      bodyVariants: normalizeBodyVariants(values.bodyVariants),
      flowSteps: normalizeScenarioFlowSteps(scenarioFlowSteps),
      timeoutMs: (values.timeoutMs || defaultScenarioValues.timeoutMs).trim(),
      retryCount: (values.retryCount || defaultScenarioValues.retryCount).trim(),
      assertion: (values.assertion || 'status < 400').trim(),
    };

    try {
      const createdScenario = await createBackendScenarioDraft(nextScenario, activeWorkspaceScope);
      setBackendScenarioIds((currentIds) => new Set([...currentIds, createdScenario.id]));
      setScenarioDrafts((drafts) => mergeScenarioDrafts([...drafts, createdScenario], []));
    } catch {
      setScenarioDrafts((drafts) => mergeScenarioDrafts([...drafts, nextScenario], []));
    } finally {
      closeCreateScenario();
    }
  };

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#0f766e',
          borderRadius: 8,
          fontFamily: '"Segoe UI", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif',
        },
      }}
    >
      <Layout className="app-shell">
        <Layout.Sider className="app-sider" width={264} breakpoint="lg" collapsedWidth={0}>
          <div className="brand-block" aria-label={bilingual(productTitle)}>
            <div className="brand-mark">AIT</div>
            <div>
              <div className="brand-title">{bilingual(productTitle)}</div>
              <div className="brand-caption">Load Lab Console</div>
            </div>
          </div>
          <nav aria-label="项目工作区 / Project workspace">
            <Menu
              className="workspace-menu"
              theme="dark"
              mode="inline"
              selectedKeys={[activePage]}
              items={navItems}
              onClick={({ key }) => {
                navigateToPage(key as PageKey);
              }}
            />
          </nav>
        </Layout.Sider>
        <Layout className="app-main">
          <Layout.Header className="topbar">
            <div>
              <Typography.Text className="topbar-kicker">控制台 / Console</Typography.Text>
              <Typography.Title level={1} className="app-title">
                {bilingual(productTitle)}
              </Typography.Title>
            </div>
            <div className="topbar-actions">
              <div className="workspace-scope-controls" aria-label="Workspace scope">
                <label className="workspace-scope-field">
                  <span>项目 / Project</span>
                  <Input
                    aria-label="Workspace project ID"
                    value={workspaceProjectId}
                    onChange={(event) => setWorkspaceProjectId(event.target.value)}
                  />
                </label>
                <label className="workspace-scope-field">
                  <span>环境 / Env</span>
                  <Input
                    aria-label="Workspace environment"
                    value={workspaceEnvironment}
                    onChange={(event) => setWorkspaceEnvironment(event.target.value)}
                  />
                </label>
                <Button size="small" onClick={applyWorkspaceScope}>
                  应用范围 / Apply scope
                </Button>
              </div>
              <Badge status="success" text={bilingual(schedulerStatus)} />
              <Button type="primary" onClick={openCreateScenario}>
                新建场景 / New scenario
              </Button>
            </div>
          </Layout.Header>
          <Layout.Content className="workspace-content">
            <section className="page-intro">
              <div>
                <Typography.Title level={2} className="page-title">
                  {bilingual(currentPage.label)}
                </Typography.Title>
                <Typography.Paragraph className="page-summary">
                  {bilingual(currentPage.summary)}
                </Typography.Paragraph>
              </div>
              <div className="intro-meta" aria-label="workspace summary">
                <span>Asia/Shanghai</span>
                <span>{`scope ${activeWorkspaceScope ? `${effectiveWorkspaceScope.projectId}/${effectiveWorkspaceScope.environment}` : 'all'}`}</span>
                <span>8 agents</span>
                <span>3 targets</span>
              </div>
            </section>

            <ActiveWorkspacePage
              page={currentPage}
              workspaceScope={activeWorkspaceScope}
              effectiveWorkspaceScope={effectiveWorkspaceScope}
              scenarioDrafts={scopedScenarioDrafts}
              backendScenarioIds={backendScenarioIds}
              scenarioFlowSteps={scenarioFlowSteps}
              onAddScenarioFlowStep={addScenarioFlowStep}
              onUpdateScenarioFlowStep={updateScenarioFlowStep}
            />
          </Layout.Content>
        </Layout>
      </Layout>
      <Modal
        title="创建场景 / Create scenario"
        open={isCreateScenarioOpen}
        onCancel={closeCreateScenario}
        footer={null}
        width={760}
        destroyOnHidden
      >
        <Form
          form={scenarioForm}
          layout="vertical"
          className="scenario-form"
          initialValues={defaultScenarioValues}
          onFinish={handleCreateScenario}
        >
          <Form.Item
            label="场景名称 / Scenario name"
            name="name"
            rules={[{ required: true, whitespace: true, message: 'Please enter a scenario name' }]}
          >
            <Input placeholder="payment-http-smoke" autoFocus />
          </Form.Item>

          <div className="scenario-form-grid">
            <Form.Item
              label="项目 / Project ID"
              name="projectId"
              rules={[{ required: true, whitespace: true, message: 'Please enter a project id' }]}
            >
              <Input placeholder="project-checkout" />
            </Form.Item>
            <Form.Item
              label="环境 / Environment"
              name="environment"
              rules={[{ required: true, whitespace: true, message: 'Please enter an environment' }]}
            >
              <Input placeholder="staging" />
            </Form.Item>
          </div>
          <Form.Item
            label={isCustomRPCScenario ? '适配器地址 / Adapter URL' : '基础地址 / Base URL'}
            name="baseUrl"
            rules={[{ required: true, whitespace: true, message: 'Please enter a base URL' }]}
          >
            <Input placeholder={isCustomRPCScenario ? 'http://127.0.0.1:9090' : 'http://127.0.0.1:8080'} />
          </Form.Item>

          <div className="scenario-form-grid">
            <Form.Item label="协议 / Protocol" name="protocol">
              <select
                aria-label="协议 / Protocol"
                className="scenario-native-select"
                role="combobox"
                onChange={(event) => {
                  const protocol = event.target.value;
                  if (protocol === 'CUSTOM_RPC') {
                    const currentMethod = scenarioForm.getFieldValue('method');
                    if (methodOptions.some((option) => option.value === currentMethod)) {
                      scenarioForm.setFieldValue('method', '');
                    }
                    return;
                  }
                  if (!scenarioForm.getFieldValue('method')) {
                    scenarioForm.setFieldValue('method', 'GET');
                  }
                }}
              >
                {protocolOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </Form.Item>
            {isCustomRPCScenario ? (
              <Form.Item
                label="RPC 方法 / RPC method"
                name="method"
                rules={[{ required: true, whitespace: true, message: 'Please enter an RPC method' }]}
              >
                <Input placeholder="checkout.OrderService/CreateOrder" />
              </Form.Item>
            ) : (
              <Form.Item label="方法 / Method" name="method">
                <select aria-label="方法 / Method" className="scenario-native-select" role="combobox">
                  {methodOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </Form.Item>
            )}
          </div>

          <Form.Item
            label={isCustomRPCScenario ? '适配器路径 / Adapter path' : '请求路径 / Request path'}
            name="path"
            rules={[{ required: true, whitespace: true, message: 'Please enter a request path' }]}
          >
            <Input placeholder={isCustomRPCScenario ? '/invoke' : '/api/health'} />
          </Form.Item>

          <Form.List name="queryVariants">
            {(fields, { add, remove }) => (
              <div className="query-variant-list" aria-label="查询参数候选 / Query variants">
                <div className="query-variant-heading">
                  <Typography.Text className="section-title">查询参数候选 / Query variants</Typography.Text>
                  <Button
                    size="small"
                    onClick={() => add({ name: `query-${fields.length + 1}`, weight: '1', queryParams: '' })}
                  >
                    添加 Query / Add query variant
                  </Button>
                </div>
                {fields.map(({ key, name, ...restField }, index) => (
                  <div className="query-variant-row" key={key}>
                    <Form.Item {...restField} name={[name, 'name']}>
                      <Input aria-label={`Query name ${index + 1}`} placeholder={`query-${index + 1}`} />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'queryParams']}
                    >
                      <Input
                        aria-label={`Query params variant ${index + 1}`}
                        placeholder="tenant=shanghai&debug=true"
                      />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'weight']}
                      rules={[{ pattern: /^(0|[1-9]\d*)$/, message: 'Weight must be a non-negative number' }]}
                    >
                      <Input aria-label={`Query weight ${index + 1}`} inputMode="numeric" placeholder="100" />
                    </Form.Item>
                    <Button
                      aria-label={`Remove query variant ${index + 1}`}
                      disabled={fields.length === 1}
                      onClick={() => remove(name)}
                    >
                      删除 / Remove
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </Form.List>

          <div className="header-kv-list" aria-label="请求头 / Headers">
            <div className="header-kv-heading">
              <Typography.Text className="section-title">请求头 / Headers</Typography.Text>
              <Button
                size="small"
                onClick={() => {
                  const headers = scenarioForm.getFieldValue('headers') ?? [];
                  scenarioForm.setFieldValue('headers', [...headers, { key: '', value: '' }]);
                }}
              >
                添加请求头 / Add header
              </Button>
            </div>
            <Form.List name="headers">
              {(fields, { remove }) => (
                <>
                  {fields.map(({ key, name, ...restField }, index) => (
                    <div className="header-kv-row" key={key}>
                      <Form.Item
                        {...restField}
                        name={[name, 'key']}
                        rules={[{ required: true, whitespace: true, message: 'Header key is required' }]}
                      >
                        <Input aria-label={`Header key ${index + 1}`} placeholder="Authorization" />
                      </Form.Item>
                      <Form.Item
                        {...restField}
                        name={[name, 'value']}
                        rules={[{ required: true, whitespace: true, message: 'Header value is required' }]}
                      >
                        <Input aria-label={`Header value ${index + 1}`} placeholder="Bearer ${token}" />
                      </Form.Item>
                      <Button
                        aria-label={`Remove header ${index + 1}`}
                        disabled={fields.length === 1}
                        onClick={() => remove(name)}
                      >
                        删除 / Remove
                      </Button>
                    </div>
                  ))}
                </>
              )}
            </Form.List>
          </div>

          <Form.List name="bodyVariants">
            {(fields, { add, remove }) => (
              <div className="body-variant-list" aria-label="请求体候选 / Body variants">
                <div className="body-variant-heading">
                  <Typography.Text className="section-title">请求体候选 / Body variants</Typography.Text>
                  <Button
                    size="small"
                    onClick={() => add({ name: `body-${fields.length + 1}`, weight: '1', body: '' })}
                  >
                    添加 Body / Add body variant
                  </Button>
                </div>
                {fields.map(({ key, name, ...restField }, index) => (
                  <div className="body-variant-row" key={key}>
                    <div className="body-variant-topline">
                      <Form.Item {...restField} name={[name, 'name']}>
                        <Input aria-label={`Body name ${index + 1}`} placeholder={`body-${index + 1}`} />
                      </Form.Item>
                      <Form.Item
                        {...restField}
                        name={[name, 'weight']}
                        rules={[{ pattern: /^(0|[1-9]\d*)$/, message: 'Weight must be a non-negative number' }]}
                      >
                        <Input aria-label={`Body weight ${index + 1}`} inputMode="numeric" placeholder="100" />
                      </Form.Item>
                      <Button
                        aria-label={`Remove body variant ${index + 1}`}
                        disabled={fields.length === 1}
                        onClick={() => remove(name)}
                      >
                        删除 / Remove
                      </Button>
                    </div>
                    <Form.Item {...restField} name={[name, 'body']} rules={[{ validator: validateJsonBody }]}>
                      <Input.TextArea
                        aria-label={`Body JSON variant ${index + 1}`}
                        className="json-input"
                        rows={5}
                        placeholder='{ "amount": 100, "currency": "CNY" }'
                      />
                    </Form.Item>
                  </div>
                ))}
              </div>
            )}
          </Form.List>

          <div className="scenario-form-grid">
            <Form.Item
              label="超时毫秒 / Timeout"
              name="timeoutMs"
              rules={[{ pattern: /^\d+$/, message: 'Timeout must be a number' }]}
            >
              <Input inputMode="numeric" />
            </Form.Item>
            <Form.Item
              label="重试次数 / Retry count"
              name="retryCount"
              rules={[{ pattern: /^\d+$/, message: 'Retry count must be a number' }]}
            >
              <Input inputMode="numeric" />
            </Form.Item>
          </div>

          <Form.Item label="成功断言 / Success assertion" name="assertion">
            <Input placeholder="status < 400" />
          </Form.Item>

          <div className="modal-actions">
            <Button onClick={closeCreateScenario}>取消 / Cancel</Button>
            <Button type="primary" htmlType="submit">
              创建 / Create
            </Button>
          </div>
        </Form>
      </Modal>
    </ConfigProvider>
  );
}

function ActiveWorkspacePage({
  page,
  workspaceScope,
  effectiveWorkspaceScope,
  scenarioDrafts,
  backendScenarioIds,
  scenarioFlowSteps,
  onAddScenarioFlowStep,
  onUpdateScenarioFlowStep,
}: {
  page: WorkspacePage;
  workspaceScope: WorkspaceScope | null;
  effectiveWorkspaceScope: WorkspaceScope;
  scenarioDrafts: ScenarioDraft[];
  backendScenarioIds: Set<string>;
  scenarioFlowSteps: ScenarioFlowStep[];
  onAddScenarioFlowStep: () => void;
  onUpdateScenarioFlowStep: (index: number, patch: Partial<ScenarioFlowStep>) => void;
}) {
  switch (page.key) {
    case 'dashboard':
      return <DashboardPage workspaceScope={workspaceScope} />;
    case 'targets':
      return <TargetsPage page={page} workspaceScope={workspaceScope} effectiveWorkspaceScope={effectiveWorkspaceScope} />;
    case 'scenarios':
      return (
        <ScenariosPage
          page={page}
          scenarioDrafts={scenarioDrafts}
          scenarioFlowSteps={scenarioFlowSteps}
          onAddScenarioFlowStep={onAddScenarioFlowStep}
          onUpdateScenarioFlowStep={onUpdateScenarioFlowStep}
        />
      );
    case 'runs':
      return (
        <RunsPage
          page={page}
          workspaceScope={workspaceScope}
          scenarioDrafts={scenarioDrafts}
          backendScenarioIds={backendScenarioIds}
        />
      );
    case 'reports':
      return <ReportsPage page={page} workspaceScope={workspaceScope} />;
    default:
      return null;
  }
}

function DashboardPage({ workspaceScope }: { workspaceScope: WorkspaceScope | null }) {
  const [dashboardRuns, setDashboardRuns] = useState<RunResult[]>([]);
  const [dashboardAgents, setDashboardAgents] = useState<AgentRecord[]>([]);
  const [dashboardStopError, setDashboardStopError] = useState('');
  const [isStoppingDashboardRun, setIsStoppingDashboardRun] = useState(false);

  useEffect(() => {
    let isMounted = true;

    Promise.allSettled([loadBackendRuns(workspaceScope), loadBackendAgents(workspaceScope)])
      .then(([runsResult, agentsResult]) => {
        if (!isMounted) {
          return;
        }
        if (runsResult.status === 'fulfilled') {
          setDashboardRuns(runsResult.value);
        }
        if (agentsResult.status === 'fulfilled') {
          setDashboardAgents(agentsResult.value);
        }
      })
      .catch(() => {
        // The dashboard keeps its static overview if the backend is not reachable.
      });

    return () => {
      isMounted = false;
    };
  }, [workspaceScope]);

  const latestRun = dashboardRuns[0];
  const hasBackendDashboardData = dashboardRuns.length > 0 || dashboardAgents.length > 0;
  const dashboardMetrics = hasBackendDashboardData
    ? buildDashboardStatusMetrics(latestRun, dashboardAgents)
    : statusMetrics;
  const dashboardAgentRows = dashboardAgents.length > 0 ? dashboardAgents.slice(0, 3).map(agentToDashboardRow) : agentRows;
  const dashboardRecentRuns = dashboardRuns.length > 0 ? dashboardRuns.slice(0, 3).map(runToDashboardRecentRun) : recentRuns;
  const onlineAgentCount = dashboardAgents.filter((agent) => agent.status === 'online').length;
  const agentHealthText = dashboardAgents.length > 0 ? `${onlineAgentCount} / ${dashboardAgents.length}` : '8 / 9';
  const liveRunStatus = latestRun?.status || 'running';
  const liveRunTitle = runDisplayName(latestRun);
  const liveRunMeta = latestRun
    ? `${latestRun.successRequests + latestRun.failedRequests} / ${latestRun.totalRequests} completed`
    : 'fixed QPS - 42m remaining';
  const runSubtitle = latestRun ? `${latestRun.id} - ${liveRunTitle}` : 'RUN-2407 - checkout-mixed-rpc';
  const dashboardStages = runStagesFromResult(latestRun);
  const canStopDashboardRun = Boolean(latestRun && isRunActive(latestRun.status));

  const openRunsPage = () => {
    window.location.hash = '#runs';
  };

  const rememberDashboardRun = (run: RunResult) => {
    setDashboardRuns((currentRuns) => [run, ...currentRuns.filter((currentRun) => currentRun.id !== run.id)]);
  };

  useEffect(() => {
    if (!latestRun || !isRunActive(latestRun.status)) {
      return;
    }

    let isMounted = true;
    let timerID: number | undefined;

    const pollDashboardRun = async () => {
      try {
        const updatedRun = await loadBackendRun(latestRun.id, workspaceScope);
        if (!isMounted) {
          return;
        }
        rememberDashboardRun(updatedRun);
        if (isRunActive(updatedRun.status)) {
          timerID = window.setTimeout(pollDashboardRun, 500);
        }
      } catch {
        if (isMounted) {
          timerID = window.setTimeout(pollDashboardRun, 1000);
        }
      }
    };

    timerID = window.setTimeout(pollDashboardRun, 250);

    return () => {
      isMounted = false;
      if (timerID !== undefined) {
        window.clearTimeout(timerID);
      }
    };
  }, [latestRun?.id, latestRun?.status, workspaceScope]);

  useEffect(() => {
    let isMounted = true;
    let timerID: number | undefined;

    const pollDashboardAgents = async () => {
      try {
        const agents = await loadBackendAgents(workspaceScope);
        if (isMounted) {
          setDashboardAgents(agents);
        }
      } catch {
        // Keep the last known agent snapshot visible when the backend is temporarily unreachable.
      } finally {
        if (isMounted) {
          timerID = window.setTimeout(pollDashboardAgents, 1000);
        }
      }
    };

    timerID = window.setTimeout(pollDashboardAgents, 250);

    return () => {
      isMounted = false;
      if (timerID !== undefined) {
        window.clearTimeout(timerID);
      }
    };
  }, [workspaceScope]);

  const stopDashboardRun = async () => {
    if (!latestRun || !isRunActive(latestRun.status) || isStoppingDashboardRun) {
      return;
    }

    setDashboardStopError('');
    setIsStoppingDashboardRun(true);
    try {
      const stoppedRun = await stopBackendRun(latestRun.id, workspaceScope);
      rememberDashboardRun(stoppedRun);
    } catch (error) {
      setDashboardStopError(error instanceof Error ? error.message : 'Failed to stop dashboard run.');
    } finally {
      setIsStoppingDashboardRun(false);
    }
  };

  return (
    <>
      <section className="status-section" aria-label="运行态势 / Run status">
        <div className="section-heading">
          <Typography.Text className="section-title">运行态势 / Run status</Typography.Text>
          <Typography.Text className="section-subtitle">{runSubtitle}</Typography.Text>
        </div>
        <div className="metric-grid">
          {dashboardMetrics.map((metric) => (
            <article className={`metric-card metric-card-${metric.tone}`} key={metric.label.en}>
              <span>{bilingual(metric.label)}</span>
              <strong>{metric.value}</strong>
              <small>{metric.note}</small>
            </article>
          ))}
        </div>
      </section>

      <section className="dashboard-grid">
        <article className="control-panel panel-large">
          <div className="panel-heading">
            <div>
              <Typography.Text className="section-title">实时压测 / Live run</Typography.Text>
              <p>{`${liveRunTitle} - ${liveRunMeta}`}</p>
            </div>
            <Tag color={runStatusColor(liveRunStatus)}>{liveRunStatus}</Tag>
          </div>
          <div className="stage-list">
            {dashboardStages.map((stage) => (
              <div className="stage-row" key={stage.name.en}>
                <div className="stage-label">
                  <span>{bilingual(stage.name)}</span>
                  <span>{stage.value}%</span>
                </div>
                <div className="stage-track" aria-label={bilingual(stage.name)}>
                  <span style={{ width: `${stage.value}%` }} />
                </div>
              </div>
            ))}
          </div>
          <div className="control-actions">
            <Button type="primary" onClick={openRunsPage}>
              启动压测 / Start run
            </Button>
            <Button danger loading={isStoppingDashboardRun} disabled={!canStopDashboardRun} onClick={stopDashboardRun}>
              停止 / Stop
            </Button>
            <Button>保存模板 / Save template</Button>
          </div>
          {dashboardStopError ? <p className="run-error">{dashboardStopError}</p> : null}
        </article>

        <article className="agent-panel">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">Agent 健康 / Agent health</Typography.Text>
            <Badge status={dashboardAgents.length === 0 || onlineAgentCount === dashboardAgents.length ? 'success' : 'warning'} text={agentHealthText} />
          </div>
          <div className="agent-list">
            {dashboardAgentRows.map((agent) => (
              <div className="agent-row" key={agent.name}>
                <div>
                  <strong>{agent.name}</strong>
                  <span>{agent.zone}</span>
                </div>
                <div className="agent-metrics">
                  <span>CPU {agent.cpu}</span>
                  <span>MEM {agent.mem}</span>
                  <span>IO {agent.io}</span>
                </div>
                <div className="agent-row-status">
                  <Tag color={agent.status ? agentStatusColor(agent.status) : agent.state.en === 'Healthy' ? 'success' : 'warning'}>
                    {bilingual(agent.state)}
                  </Tag>
                </div>
              </div>
            ))}
          </div>
        </article>

        <article className="profile-panel">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">画像观察 / Profile watch</Typography.Text>
            <Tag color="geekblue">pprof</Tag>
          </div>
          <div className="profile-lanes">
            {profileRows.map((row) => (
              <div className="profile-row" key={row.label.en}>
                <span>{bilingual(row.label)}</span>
                <strong>{bilingual(row.value)}</strong>
              </div>
            ))}
          </div>
        </article>

        <article className="recent-panel panel-large">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">近期运行 / Recent runs</Typography.Text>
            <Button size="small" onClick={openRunsPage}>
              查看全部 / View all
            </Button>
          </div>
          <div className="run-table">
            {dashboardRecentRuns.map((run) => (
              <div className="run-row" key={run.id}>
                <span>{run.id}</span>
                <strong>{run.scenario}</strong>
                <span>{run.latency}</span>
                <div className="run-row-status">
                  <Tag color={run.status ? runStatusColor(run.status) : run.result.en === 'Passed' ? 'success' : 'warning'}>
                    {bilingual(run.result)}
                  </Tag>
                </div>
              </div>
            ))}
          </div>
        </article>
      </section>
    </>
  );
}

function TargetsPage({
  page,
  workspaceScope,
  effectiveWorkspaceScope,
}: {
  page: WorkspacePage;
  workspaceScope: WorkspaceScope | null;
  effectiveWorkspaceScope: WorkspaceScope;
}) {
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [targets, setTargets] = useState<TargetRecord[]>([]);
  const [createdAgentToken, setCreatedAgentToken] = useState<AgentTokenRecord | null>(null);
  const [isCreatingAgentToken, setIsCreatingAgentToken] = useState(false);
  const [isRevokingAgentToken, setIsRevokingAgentToken] = useState(false);
  const [isRotatingAgentToken, setIsRotatingAgentToken] = useState(false);
  const [agentTokenError, setAgentTokenError] = useState('');
  const [agentTokenProjectId, setAgentTokenProjectId] = useState(effectiveWorkspaceScope.projectId);
  const [agentTokenEnvironment, setAgentTokenEnvironment] = useState(effectiveWorkspaceScope.environment);
  const [targetName, setTargetName] = useState('');
  const [targetProjectId, setTargetProjectId] = useState(effectiveWorkspaceScope.projectId);
  const [targetEnvironment, setTargetEnvironment] = useState(effectiveWorkspaceScope.environment);
  const [targetBaseURL, setTargetBaseURL] = useState('');
  const [targetProfileEndpoint, setTargetProfileEndpoint] = useState('');
  const [targetProcessName, setTargetProcessName] = useState('');
  const [targetCmdlineContains, setTargetCmdlineContains] = useState('');
  const [targetHealthPath, setTargetHealthPath] = useState('');
  const [targetHealthExpectedStatus, setTargetHealthExpectedStatus] = useState('');
  const [targetHealthTimeoutMs, setTargetHealthTimeoutMs] = useState('');
  const [targetCpuMaxPercent, setTargetCpuMaxPercent] = useState('');
  const [targetMemoryMaxPercent, setTargetMemoryMaxPercent] = useState('');
  const [targetDiskReadMaxBytesPerSec, setTargetDiskReadMaxBytesPerSec] = useState('');
  const [targetDiskWriteMaxBytesPerSec, setTargetDiskWriteMaxBytesPerSec] = useState('');
  const [targetNetworkRxMaxBytesPerSec, setTargetNetworkRxMaxBytesPerSec] = useState('');
  const [targetNetworkTxMaxBytesPerSec, setTargetNetworkTxMaxBytesPerSec] = useState('');
  const [targetError, setTargetError] = useState('');
  const [isCreatingTarget, setIsCreatingTarget] = useState(false);
  const [editingTargetId, setEditingTargetId] = useState('');
  const [deletingTargetId, setDeletingTargetId] = useState('');
  const [unbindingTargetId, setUnbindingTargetId] = useState('');
  const [checkingTargetId, setCheckingTargetId] = useState('');
  const [creatingProfileTaskTargetId, setCreatingProfileTaskTargetId] = useState('');
  const [createdProfileTasksByTargetId, setCreatedProfileTasksByTargetId] = useState<Record<string, ProfileTaskRecord>>({});
  const [profileTaskDraftsByTargetId, setProfileTaskDraftsByTargetId] = useState<Record<string, ProfileTaskDraft>>({});
  const [profileTaskTemplates, setProfileTaskTemplates] = useState<ProfileTaskTemplateRecord[]>(fallbackProfileTaskTemplates);
  const [profileTaskTemplateDraft, setProfileTaskTemplateDraft] = useState<ProfileTaskTemplateDraft>(profileTaskTemplateDefaultDraft);
  const [editingProfileTaskTemplateId, setEditingProfileTaskTemplateId] = useState('');
  const [savingProfileTaskTemplate, setSavingProfileTaskTemplate] = useState(false);
  const [deletingProfileTaskTemplateId, setDeletingProfileTaskTemplateId] = useState('');
  const [profileTaskTemplateError, setProfileTaskTemplateError] = useState('');
  const [copyCommandStatusByKey, setCopyCommandStatusByKey] = useState<Record<string, CopyCommandStatus>>({});
  const [targetHealthChecks, setTargetHealthChecks] = useState<Record<string, TargetHealthCheckResult>>({});
  const [targetHealthHistories, setTargetHealthHistories] = useState<Record<string, TargetHealthCheckResult[]>>({});
  const [onboarding, hostCollection, binding, profileEndpoints] = page.modules;

  useEffect(() => {
    setAgentTokenProjectId(effectiveWorkspaceScope.projectId);
    setAgentTokenEnvironment(effectiveWorkspaceScope.environment);
    if (!editingTargetId) {
      setTargetProjectId(effectiveWorkspaceScope.projectId);
      setTargetEnvironment(effectiveWorkspaceScope.environment);
    }
  }, [effectiveWorkspaceScope.projectId, effectiveWorkspaceScope.environment, editingTargetId]);

  useEffect(() => {
    let isMounted = true;

    Promise.allSettled([loadBackendAgents(workspaceScope), loadBackendTargets(workspaceScope), loadBackendProfileTaskTemplates()])
      .then(([agentsResult, targetsResult, templateResult]) => {
        if (!isMounted) {
          return;
        }
        if (agentsResult.status === 'fulfilled') {
          setAgents(agentsResult.value);
        }
        if (templateResult.status === 'fulfilled' && templateResult.value.length > 0) {
          setProfileTaskTemplates(templateResult.value);
        }
        if (targetsResult.status === 'fulfilled') {
          const loadedTargets = targetsResult.value;
          setTargets(loadedTargets);
          Promise.allSettled(
            loadedTargets.map(async (target) => ({
              targetID: target.id,
              history: await loadBackendTargetHealthChecks(target.id, 5, workspaceScope),
            })),
          ).then((historyResults) => {
            if (!isMounted) {
              return;
            }
            const nextHistories: Record<string, TargetHealthCheckResult[]> = {};
            historyResults.forEach((historyResult) => {
              if (historyResult.status === 'fulfilled') {
                nextHistories[historyResult.value.targetID] = historyResult.value.history;
              }
            });
            setTargetHealthHistories(nextHistories);
          });
        }
      })
      .catch(() => {
        // Keep the static topology visible when the backend is not reachable.
      });

    return () => {
      isMounted = false;
    };
  }, [workspaceScope]);

  const handleCreateAgentToken = async () => {
    setIsCreatingAgentToken(true);
    setAgentTokenError('');
    try {
      const token = await createBackendAgentToken(`agent-install-${Date.now()}`, {
        projectId: agentTokenProjectId,
        environment: agentTokenEnvironment,
        workspaceScope,
      });
      setCreatedAgentToken(token);
    } catch (error) {
      setAgentTokenError(error instanceof Error ? error.message : 'Failed to create agent token.');
    } finally {
      setIsCreatingAgentToken(false);
    }
  };

  const handleRevokeAgentToken = async () => {
    if (!createdAgentToken) {
      return;
    }

    setIsRevokingAgentToken(true);
    setAgentTokenError('');
    try {
      const token = await revokeBackendAgentToken(createdAgentToken.id, workspaceScope);
      setCreatedAgentToken(token);
    } catch (error) {
      setAgentTokenError(error instanceof Error ? error.message : 'Failed to revoke agent token.');
    } finally {
      setIsRevokingAgentToken(false);
    }
  };

  const handleRotateAgentToken = async () => {
    if (!createdAgentToken) {
      return;
    }

    setIsRotatingAgentToken(true);
    setAgentTokenError('');
    try {
      const token = await rotateBackendAgentToken(createdAgentToken.id, agentTokenDefaultExpiresInSeconds, workspaceScope);
      setCreatedAgentToken(token);
    } catch (error) {
      setAgentTokenError(error instanceof Error ? error.message : 'Failed to rotate agent token.');
    } finally {
      setIsRotatingAgentToken(false);
    }
  };

  const handleCopyCommand = async (key: string, command: string) => {
    const copied = await copyToClipboard(command);
    setCopyCommandStatusByKey((currentStatuses) => ({
      ...currentStatuses,
      [key]: copied ? 'copied' : 'failed',
    }));
  };

  const resetTargetForm = () => {
    setEditingTargetId('');
    setTargetName('');
    setTargetProjectId(effectiveWorkspaceScope.projectId);
    setTargetEnvironment(effectiveWorkspaceScope.environment);
    setTargetBaseURL('');
    setTargetProfileEndpoint('');
    setTargetProcessName('');
    setTargetCmdlineContains('');
    setTargetHealthPath('');
    setTargetHealthExpectedStatus('');
    setTargetHealthTimeoutMs('');
    setTargetCpuMaxPercent('');
    setTargetMemoryMaxPercent('');
    setTargetDiskReadMaxBytesPerSec('');
    setTargetDiskWriteMaxBytesPerSec('');
    setTargetNetworkRxMaxBytesPerSec('');
    setTargetNetworkTxMaxBytesPerSec('');
  };

  const handleEditTarget = (target: TargetRecord) => {
    setEditingTargetId(target.id);
    setTargetName(target.name);
    setTargetProjectId(target.projectId || 'default');
    setTargetEnvironment(target.environment || 'default');
    setTargetBaseURL(target.baseUrl);
    setTargetProfileEndpoint(target.profileEndpoint || '');
    setTargetProcessName(target.processMatch?.name || '');
    setTargetCmdlineContains(target.processMatch?.cmdlineContains || '');
    setTargetHealthPath(target.healthCheck?.path || '');
    setTargetHealthExpectedStatus(optionalNumberInputValue(target.healthCheck?.expectedStatus));
    setTargetHealthTimeoutMs(optionalNumberInputValue(target.healthCheck?.timeoutMs));
    setTargetCpuMaxPercent(optionalNumberInputValue(target.metricThresholds?.cpuMaxPercent));
    setTargetMemoryMaxPercent(optionalNumberInputValue(target.metricThresholds?.memoryMaxPercent));
    setTargetDiskReadMaxBytesPerSec(optionalNumberInputValue(target.metricThresholds?.diskReadMaxBytesPerSec));
    setTargetDiskWriteMaxBytesPerSec(optionalNumberInputValue(target.metricThresholds?.diskWriteMaxBytesPerSec));
    setTargetNetworkRxMaxBytesPerSec(optionalNumberInputValue(target.metricThresholds?.networkRxMaxBytesPerSec));
    setTargetNetworkTxMaxBytesPerSec(optionalNumberInputValue(target.metricThresholds?.networkTxMaxBytesPerSec));
    setTargetError('');
  };

  const handleSaveTarget = async () => {
    const selectedAgent = agents[0];
    const editingTarget = targets.find((target) => target.id === editingTargetId);
    if (!editingTarget && !selectedAgent) {
      setTargetError('Register an agent before creating a target.');
      return;
    }

    setIsCreatingTarget(true);
    setTargetError('');
    try {
      const targetInput: Omit<TargetRecord, 'id'> = {
        name: targetName.trim(),
        projectId: targetProjectId.trim() || 'default',
        baseUrl: targetBaseURL.trim(),
        environment: targetEnvironment.trim() || editingTarget?.environment || 'default',
        agentIds: editingTarget?.agentIds.length ? editingTarget.agentIds : selectedAgent ? [selectedAgent.id] : [],
        profileEndpoint: targetProfileEndpoint.trim(),
        processMatch: {
          name: targetProcessName.trim(),
          cmdlineContains: targetCmdlineContains.trim(),
        },
        healthCheck: {
          enabled: targetHealthPath.trim().length > 0,
          path: targetHealthPath.trim(),
          expectedStatus: parseOptionalRunInteger(targetHealthExpectedStatus),
          timeoutMs: parseOptionalRunInteger(targetHealthTimeoutMs),
        },
        metricThresholds: {
          cpuMaxPercent: parseOptionalRunDecimal(targetCpuMaxPercent),
          memoryMaxPercent: parseOptionalRunDecimal(targetMemoryMaxPercent),
          diskReadMaxBytesPerSec: parseOptionalRunDecimal(targetDiskReadMaxBytesPerSec),
          diskWriteMaxBytesPerSec: parseOptionalRunDecimal(targetDiskWriteMaxBytesPerSec),
          networkRxMaxBytesPerSec: parseOptionalRunDecimal(targetNetworkRxMaxBytesPerSec),
          networkTxMaxBytesPerSec: parseOptionalRunDecimal(targetNetworkTxMaxBytesPerSec),
        },
      };
      const target = editingTarget
        ? await updateBackendTarget(editingTarget.id, targetInput, workspaceScope)
        : await createBackendTarget(targetInput, workspaceScope);
      setTargets((currentTargets) => [target, ...currentTargets.filter((currentTarget) => currentTarget.id !== target.id)]);
      resetTargetForm();
    } catch (error) {
      setTargetError(error instanceof Error ? error.message : 'Failed to save target.');
    } finally {
      setIsCreatingTarget(false);
    }
  };

  const handleDeleteTarget = async (target: TargetRecord) => {
    setDeletingTargetId(target.id);
    setTargetError('');
    try {
      await deleteBackendTarget(target.id, workspaceScope);
      setTargets((currentTargets) => currentTargets.filter((currentTarget) => currentTarget.id !== target.id));
    } catch (error) {
      setTargetError(error instanceof Error ? error.message : 'Failed to delete target.');
    } finally {
      setDeletingTargetId('');
    }
  };

  const handleUnbindTargetAgent = async (target: TargetRecord) => {
    setUnbindingTargetId(target.id);
    setTargetError('');
    try {
      const updatedTarget = await updateBackendTarget(
        target.id,
        {
          name: target.name,
          projectId: target.projectId || 'default',
          baseUrl: target.baseUrl,
          environment: target.environment || 'default',
          agentIds: [],
          profileEndpoint: target.profileEndpoint,
          processMatch: target.processMatch,
          healthCheck: target.healthCheck,
          metricThresholds: target.metricThresholds,
          createdAt: target.createdAt,
          updatedAt: target.updatedAt,
        },
        workspaceScope,
      );
      setTargets((currentTargets) => currentTargets.map((currentTarget) => (currentTarget.id === updatedTarget.id ? updatedTarget : currentTarget)));
      setCreatedProfileTasksByTargetId((currentTasks) => {
        const nextTasks = { ...currentTasks };
        delete nextTasks[target.id];
        return nextTasks;
      });
    } catch (error) {
      setTargetError(error instanceof Error ? error.message : 'Failed to unbind target agent.');
    } finally {
      setUnbindingTargetId('');
    }
  };

  const handleCheckTargetHealth = async (target: TargetRecord) => {
    setCheckingTargetId(target.id);
    setTargetError('');
    try {
      const result = await checkBackendTargetHealth(target.id, workspaceScope);
      setTargetHealthChecks((currentResults) => ({
        ...currentResults,
        [target.id]: result,
      }));
      setTargetHealthHistories((currentHistories) => ({
        ...currentHistories,
        [target.id]: [result, ...(currentHistories[target.id] || [])].slice(0, 5),
      }));
      setTargets((currentTargets) =>
        currentTargets.map((currentTarget) =>
          currentTarget.id === target.id ? { ...currentTarget, lastHealthCheck: result } : currentTarget,
        ),
      );
    } catch (error) {
      setTargetError(error instanceof Error ? error.message : 'Failed to check target health.');
    } finally {
      setCheckingTargetId('');
    }
  };

  const handleCreateProfileTask = async (target: TargetRecord) => {
    const agentID = target.agentIds[0] || '';
    if (!agentID) {
      setTargetError('Bind an agent before creating a profile task.');
      return;
    }
    const draft = profileTaskDraftsByTargetId[target.id] || profileTaskDefaultDraft;
    const rawProfileSeconds = draft.profileSeconds.trim();
    const profileSeconds = Number.parseInt(rawProfileSeconds, 10);
    if (!/^\d+$/.test(rawProfileSeconds) || profileSeconds < 1 || profileSeconds > 300) {
      setTargetError('Profile task seconds must be between 1 and 300.');
      return;
    }
    const selectedTemplate =
      profileTaskTemplates.find((template) => template.id === draft.template) ||
      fallbackProfileTaskTemplates.find((template) => template.id === draft.template) ||
      fallbackProfileTaskTemplates[0];
    const isCommandTemplate = selectedTemplate.kind === 'command' || Boolean(selectedTemplate.profileCommand);
    if (!isCommandTemplate && selectedTemplate.requiresProfileEndpoint !== false && !target.profileEndpoint) {
      setTargetError('Set a profile endpoint before creating a pprof profile task.');
      return;
    }
    if (isCommandTemplate && selectedTemplate.requiresPid !== false) {
      const commandPid = draft.commandPid.trim();
      const parsedPID = Number.parseInt(commandPid, 10);
      if (!/^\d+$/.test(commandPid) || parsedPID <= 0) {
        setTargetError('Command profiler PID must be a positive integer.');
        return;
      }
      if (!selectedTemplate.profileCommand) {
        setTargetError('Command profiler template must define a command.');
        return;
      }
    }

    setCreatingProfileTaskTargetId(target.id);
    setTargetError('');
    try {
      const taskBase = {
        agentId: agentID,
        targetId: target.id,
        targetName: target.name,
        profileSeconds,
        source: 'target_manual',
        maxAttempts: profileTaskDefaultMaxAttempts,
      };
      const task =
        isCommandTemplate
          ? await createBackendProfileTask({
              ...taskBase,
              ...buildCommandTemplateTaskFields(selectedTemplate, target.name, profileSeconds, draft.commandPid.trim()),
            }, workspaceScope)
          : await createBackendProfileTask({
              ...taskBase,
              pprofBaseUrl: target.profileEndpoint,
              profileType: draft.profileType || profileTaskTemplateProfileTypes(selectedTemplate)[0] || profileTaskDefaultDraft.profileType,
            }, workspaceScope);
      setCreatedProfileTasksByTargetId((currentTasks) => ({
        ...currentTasks,
        [target.id]: task,
      }));
    } catch (error) {
      setTargetError(error instanceof Error ? error.message : 'Failed to create profile task.');
    } finally {
      setCreatingProfileTaskTargetId('');
    }
  };

  const resetProfileTaskTemplateForm = () => {
    setEditingProfileTaskTemplateId('');
    setProfileTaskTemplateDraft(profileTaskTemplateDefaultDraft);
    setProfileTaskTemplateError('');
  };

  const profileTaskTemplatePayloadFromDraft = (): Partial<ProfileTaskTemplateRecord> => {
    const profileTypes = parseProfileTaskTemplateTypes(profileTaskTemplateDraft.profileTypes);
    const commandArgs =
      profileTaskTemplateDraft.kind === 'command' ? parseProfileTaskTemplateCommandArgs(profileTaskTemplateDraft.profileCommandArgs) : [];
    return {
      id: profileTaskTemplateDraft.id.trim(),
      name: profileTaskTemplateDraft.name.trim(),
      description: profileTaskTemplateDraft.description.trim(),
      kind: profileTaskTemplateDraft.kind,
      profileTypes,
      profileType: profileTaskTemplateDraft.profileType.trim(),
      requiresProfileEndpoint: profileTaskTemplateDraft.kind === 'pprof' ? true : profileTaskTemplateDraft.requiresProfileEndpoint,
      requiresPid: profileTaskTemplateDraft.kind === 'command' ? profileTaskTemplateDraft.requiresPid : false,
      profileCommand: profileTaskTemplateDraft.kind === 'command' ? profileTaskTemplateDraft.profileCommand.trim() : '',
      profileCommandArgs: commandArgs,
      profileCommandOutput: profileTaskTemplateDraft.kind === 'command' ? profileTaskTemplateDraft.profileCommandOutput.trim() : '',
      timeoutBufferSeconds: parseOptionalRunInteger(profileTaskTemplateDraft.timeoutBufferSeconds) || 0,
      displayOrder: parseOptionalRunInteger(profileTaskTemplateDraft.displayOrder) || 100,
      enabled: true,
    };
  };

  const handleSaveProfileTaskTemplate = async () => {
    setSavingProfileTaskTemplate(true);
    setProfileTaskTemplateError('');
    try {
      const payload = profileTaskTemplatePayloadFromDraft();
      const template = editingProfileTaskTemplateId
        ? await updateBackendProfileTaskTemplate(editingProfileTaskTemplateId, payload)
        : await createBackendProfileTaskTemplate(payload);
      setProfileTaskTemplates((currentTemplates) => [
        ...currentTemplates.filter((currentTemplate) => currentTemplate.id !== template.id),
        template,
      ]);
      resetProfileTaskTemplateForm();
    } catch (error) {
      setProfileTaskTemplateError(error instanceof Error ? error.message : 'Failed to save profile task template.');
    } finally {
      setSavingProfileTaskTemplate(false);
    }
  };

  const handleEditProfileTaskTemplate = (template: ProfileTaskTemplateRecord) => {
    setEditingProfileTaskTemplateId(template.id);
    setProfileTaskTemplateDraft(profileTaskTemplateDraftFromRecord(template));
    setProfileTaskTemplateError('');
  };

  const handleDeleteProfileTaskTemplate = async (template: ProfileTaskTemplateRecord) => {
    setDeletingProfileTaskTemplateId(template.id);
    setProfileTaskTemplateError('');
    try {
      await deleteBackendProfileTaskTemplate(template.id);
      setProfileTaskTemplates((currentTemplates) => currentTemplates.filter((currentTemplate) => currentTemplate.id !== template.id));
      if (editingProfileTaskTemplateId === template.id) {
        resetProfileTaskTemplateForm();
      }
      setProfileTaskDraftsByTargetId((currentDrafts) => {
        const nextDrafts = { ...currentDrafts };
        Object.entries(nextDrafts).forEach(([targetID, draft]) => {
          if (draft.template === template.id) {
            nextDrafts[targetID] = { ...profileTaskDefaultDraft };
          }
        });
        return nextDrafts;
      });
    } catch (error) {
      setProfileTaskTemplateError(error instanceof Error ? error.message : 'Failed to delete profile task template.');
    } finally {
      setDeletingProfileTaskTemplateId('');
    }
  };

  const editingTarget = targets.find((target) => target.id === editingTargetId);
  const boundAgentLabel = editingTarget?.agentIds.length
    ? editingTarget.agentIds.map((agentID) => agentNameByID(agents, agentID)).join(', ')
    : agents[0]?.name || 'No agent';
  const createdAgentInstallCommand = createdAgentToken?.token ? agentInstallCommand(createdAgentToken.token) : '';
  const createdAgentInstallCopyKey = agentInstallCopyKey(createdAgentToken);
  const createdAgentInstallCopyStatus = copyCommandStatusByKey[createdAgentInstallCopyKey];

  return (
    <section className="targets-layout" aria-label={bilingual(page.label)}>
      <article className="topology-panel">
        <div className="panel-heading compact">
          <Typography.Text className="section-title">目标拓扑 / Target topology</Typography.Text>
          <Tag color="success">3 services</Tag>
        </div>
        <div className="topology-map">
          {['gateway', 'checkout', 'search'].map((service, index) => (
            <div className="topology-node" key={service}>
              <span>{service}</span>
              <strong>{index + 2} agents</strong>
              <small>pprof :6060</small>
            </div>
          ))}
        </div>
        <div className="agent-inventory-list" aria-label="Agent inventory / Agent inventory">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">Agent 列表 / Agent inventory</Typography.Text>
            <Tag color={agents.length > 0 ? 'success' : 'default'}>{`${agents.length} agents`}</Tag>
          </div>
          {agents.length === 0 ? (
            <p>暂无 Agent / No registered agents</p>
          ) : (
            agents.map((agent) => {
              const topProcess = topAgentProcess(agent.latestMetrics);
              const upgradeCommand = agentUpgradeCommand(agent);
              const upgradeCopyKey = `agent-upgrade-${agent.id}`;
              const upgradeCopyStatus = copyCommandStatusByKey[upgradeCopyKey];

              return (
                <div className="agent-inventory-row" key={agent.id} aria-label={`agent ${agent.name}`}>
                  <div>
                    <strong>{agent.name}</strong>
                    <span>{agent.hostname}</span>
                  </div>
                  <span>
                    <small>IP</small>
                    <strong>{agent.ip || '-'}</strong>
                  </span>
                  <span>
                    <small>version</small>
                    <strong>{agent.version || '-'}</strong>
                    {agent.upgradeAvailable ? (
                      <>
                        <em className="agent-version-upgrade">升级可用 / Upgrade available</em>
                        <small>{agent.latestVersion ? `latest ${agent.latestVersion}` : 'latest -'}</small>
                      </>
                    ) : null}
                  </span>
                  <span>
                    <small>project</small>
                    <strong>{agent.projectId || 'default'}</strong>
                  </span>
                  <span>
                    <small>env</small>
                    <strong>{agent.environment || 'default'}</strong>
                  </span>
                  <span>
                    <small>zone</small>
                    <strong>{agent.labels?.zone || '-'}</strong>
                  </span>
                  <Tag color={agent.status === 'online' ? 'success' : 'default'}>{agent.status}</Tag>
                  {agent.upgradeAvailable ? (
                    <div className="agent-upgrade-command" aria-label={`agent upgrade command ${agent.id}`}>
                      <div className="agent-upgrade-command-heading">
                        <small>升级命令 / Upgrade command</small>
                        <Button
                          size="small"
                          icon={<CopyOutlined />}
                          aria-label={`Copy upgrade command for ${agent.id}`}
                          onClick={() => {
                            void handleCopyCommand(upgradeCopyKey, upgradeCommand);
                          }}
                        >
                          复制 / Copy
                        </Button>
                        {upgradeCopyStatus ? (
                          <small className={`copy-command-status copy-command-status-${upgradeCopyStatus}`}>{copyCommandStatusLabel(upgradeCopyStatus)}</small>
                        ) : null}
                      </div>
                      <code>{upgradeCommand}</code>
                    </div>
                  ) : null}
                  {agent.latestMetrics ? (
                    <div className="agent-metric-strip">
                      <span>{`CPU ${formatPercent(agent.latestMetrics.cpuUsagePercent)}`}</span>
                      <span>{`MEM ${formatPercent(agent.latestMetrics.memoryUsagePercent)}`}</span>
                      <span>{topProcess ? `${topProcess.name} ${formatPercent(topProcess.cpuUsagePercent)}` : 'process -'}</span>
                    </div>
                  ) : (
                    <div className="agent-metric-strip muted">No metrics yet</div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </article>

      <article className="target-setup-panel">
        <div className="target-setup-heading">
          <Typography.Text className="section-title">{bilingual(onboarding.title)}</Typography.Text>
          <Button size="small" loading={isCreatingAgentToken} onClick={handleCreateAgentToken}>
            生成 Token / Generate token
          </Button>
        </div>
        <p>{bilingual(onboarding.description)}</p>
        <div className="agent-token-scope-form">
          <label className="target-field">
            <span>项目 ID / Project ID</span>
            <Input
              aria-label="Agent token project ID"
              value={agentTokenProjectId}
              onChange={(event) => setAgentTokenProjectId(event.target.value)}
              placeholder="default"
            />
          </label>
          <label className="target-field">
            <span>环境 / Environment</span>
            <Input
              aria-label="Agent token environment"
              value={agentTokenEnvironment}
              onChange={(event) => setAgentTokenEnvironment(event.target.value)}
              placeholder="default"
            />
          </label>
        </div>
        <code>{agentInstallCommand()}</code>
        {createdAgentToken ? (
          <div className="agent-token-panel" aria-label={`agent token ${createdAgentToken.id}`}>
            <div className="agent-token-meta">
              <span>一次性 Token / One-time token</span>
              <Tag color={createdAgentToken.status === 'active' ? 'success' : 'default'}>{createdAgentToken.status}</Tag>
              <span>{`Project ${createdAgentToken.projectId || 'default'}`}</span>
              <span>{`Env ${createdAgentToken.environment || 'default'}`}</span>
              {createdAgentToken.expiresAt ? <span>{`Expires ${createdAgentToken.expiresAt}`}</span> : null}
              {createdAgentToken.rotatedAt ? <span>{`Rotated ${createdAgentToken.rotatedAt}`}</span> : null}
            </div>
            {createdAgentToken.token ? (
              <>
                <strong>{createdAgentToken.token}</strong>
                <code>{createdAgentInstallCommand}</code>
                <code>{`Authorization: Bearer ${createdAgentToken.token}`}</code>
                <div className="agent-token-actions">
                  <Button
                    size="small"
                    icon={<CopyOutlined />}
                    aria-label={`Copy install command for ${createdAgentToken.id}`}
                    onClick={() => {
                      void handleCopyCommand(createdAgentInstallCopyKey, createdAgentInstallCommand);
                    }}
                  >
                    复制安装命令 / Copy install command
                  </Button>
                  {createdAgentInstallCopyStatus ? (
                    <small className={`copy-command-status copy-command-status-${createdAgentInstallCopyStatus}`}>
                      {copyCommandStatusLabel(createdAgentInstallCopyStatus)}
                    </small>
                  ) : null}
                  <Button
                    size="small"
                    loading={isRotatingAgentToken}
                    aria-label={`Rotate agent token ${createdAgentToken.id}`}
                    onClick={handleRotateAgentToken}
                  >
                    轮换 / Rotate
                  </Button>
                  <Button
                    danger
                    size="small"
                    loading={isRevokingAgentToken}
                    aria-label={`Revoke agent token ${createdAgentToken.id}`}
                    onClick={handleRevokeAgentToken}
                  >
                    撤销 / Revoke
                  </Button>
                </div>
                <small>只在创建时显示，请写入被压测机器的 Agent 配置。 / Visible only on creation; save it into the target host agent config.</small>
              </>
            ) : (
              <small>Token 已撤销，明文密钥不再显示。 / Token revoked; the secret is no longer shown.</small>
            )}
          </div>
        ) : null}
        {agentTokenError ? (
          <p className="agent-token-error" role="alert">
            {agentTokenError}
          </p>
        ) : null}
      </article>

      <article className="target-collector-panel">
        <Typography.Text className="section-title">{bilingual(hostCollection.title)}</Typography.Text>
        <div className="collector-grid">
          {['CPU', 'MEM', 'DISK IO', 'NET', 'PROC'].map((item) => (
            <span key={item}>{item}</span>
          ))}
        </div>
      </article>

      <article className="profile-template-panel" aria-label="profile task templates">
        <div className="panel-heading compact">
          <div>
            <Typography.Text className="section-title">画像模板 / Profile templates</Typography.Text>
            <p>SQLite whitelist templates used by manual Target profiling.</p>
          </div>
          <Tag color="processing">{`${profileTaskTemplates.length} templates`}</Tag>
        </div>
        <div className="profile-template-form">
          <label className="target-field">
            <span>模板 ID / Template ID</span>
            <Input
              aria-label="Profile template id"
              placeholder="async_profiler"
              value={profileTaskTemplateDraft.id}
              disabled={Boolean(editingProfileTaskTemplateId)}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  id: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field">
            <span>模板名称 / Template name</span>
            <Input
              aria-label="Profile template name"
              placeholder="Async Profiler"
              value={profileTaskTemplateDraft.name}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  name: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field">
            <span>类型 / Kind</span>
            <select
              aria-label="Profile template kind"
              className="target-profile-task-select"
              value={profileTaskTemplateDraft.kind}
              onChange={(event) => {
                const nextKind = event.target.value;
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  kind: nextKind,
                  requiresProfileEndpoint: nextKind === 'pprof',
                  requiresPid: nextKind === 'command' ? currentDraft.requiresPid : false,
                  profileTypes: nextKind === 'pprof' && currentDraft.profileTypes === 'perf' ? 'cpu, heap, goroutine' : currentDraft.profileTypes,
                  profileType: nextKind === 'pprof' && currentDraft.profileType === 'perf' ? 'cpu' : currentDraft.profileType,
                }));
              }}
            >
              <option value="command">command</option>
              <option value="pprof">pprof</option>
            </select>
          </label>
          <label className="target-field">
            <span>Profile types</span>
            <Input
              aria-label="Profile template profile types"
              placeholder="perf"
              value={profileTaskTemplateDraft.profileTypes}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  profileTypes: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field">
            <span>默认类型 / Default type</span>
            <Input
              aria-label="Profile template profile type"
              placeholder="perf"
              value={profileTaskTemplateDraft.profileType}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  profileType: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field">
            <span>超时缓冲秒 / Timeout buffer</span>
            <Input
              aria-label="Profile template timeout buffer"
              inputMode="numeric"
              placeholder="15"
              value={profileTaskTemplateDraft.timeoutBufferSeconds}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  timeoutBufferSeconds: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field">
            <span>排序 / Display order</span>
            <Input
              aria-label="Profile template display order"
              inputMode="numeric"
              placeholder="100"
              value={profileTaskTemplateDraft.displayOrder}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  displayOrder: event.target.value,
                }))
              }
            />
          </label>
          <label className="target-field target-field-wide">
            <span>描述 / Description</span>
            <Input
              aria-label="Profile template description"
              placeholder="Collect profiler output from a target process."
              value={profileTaskTemplateDraft.description}
              onChange={(event) =>
                setProfileTaskTemplateDraft((currentDraft) => ({
                  ...currentDraft,
                  description: event.target.value,
                }))
              }
            />
          </label>
          {profileTaskTemplateDraft.kind === 'command' ? (
            <>
              <label className="target-field">
                <span>命令 / Command</span>
                <Input
                  aria-label="Profile template command"
                  placeholder="perf"
                  value={profileTaskTemplateDraft.profileCommand}
                  onChange={(event) =>
                    setProfileTaskTemplateDraft((currentDraft) => ({
                      ...currentDraft,
                      profileCommand: event.target.value,
                    }))
                  }
                />
              </label>
              <label className="target-field">
                <span>输出路径 / Output</span>
                <Input
                  aria-label="Profile template output"
                  placeholder="/tmp/{{targetNameSlug}}.data"
                  value={profileTaskTemplateDraft.profileCommandOutput}
                  onChange={(event) =>
                    setProfileTaskTemplateDraft((currentDraft) => ({
                      ...currentDraft,
                      profileCommandOutput: event.target.value,
                    }))
                  }
                />
              </label>
              <label className="target-field target-field-wide">
                <span>参数 JSON / Args JSON</span>
                <Input.TextArea
                  aria-label="Profile template command args"
                  autoSize={{ minRows: 2, maxRows: 4 }}
                  value={profileTaskTemplateDraft.profileCommandArgs}
                  onChange={(event) =>
                    setProfileTaskTemplateDraft((currentDraft) => ({
                      ...currentDraft,
                      profileCommandArgs: event.target.value,
                    }))
                  }
                />
              </label>
              <Checkbox
                aria-label="Profile template requires PID"
                checked={profileTaskTemplateDraft.requiresPid}
                onChange={(event) =>
                  setProfileTaskTemplateDraft((currentDraft) => ({
                    ...currentDraft,
                    requiresPid: event.target.checked,
                  }))
                }
              >
                PID required
              </Checkbox>
            </>
          ) : null}
          <div className="profile-template-actions">
            <Button type="primary" loading={savingProfileTaskTemplate} onClick={handleSaveProfileTaskTemplate}>
              {editingProfileTaskTemplateId ? 'Update profile template' : 'Create profile template'}
            </Button>
            {editingProfileTaskTemplateId ? <Button onClick={resetProfileTaskTemplateForm}>Cancel template edit</Button> : null}
          </div>
        </div>
        {profileTaskTemplateError ? (
          <p className="target-error" role="alert">
            {profileTaskTemplateError}
          </p>
        ) : null}
        <div className="profile-template-list">
          {profileTaskTemplates.map((template) => (
            <div className="profile-template-row" key={template.id} aria-label={`profile task template ${template.id}`}>
              <div>
                <strong>{template.name}</strong>
                <small>{template.id}</small>
              </div>
              <Tag color={template.kind === 'command' ? 'gold' : 'geekblue'}>{template.kind}</Tag>
              <span>{profileTaskTemplateProfileTypes(template).join(', ')}</span>
              <span>{template.profileCommand || template.profileType || '-'}</span>
              <small>{template.profileCommandOutput || template.description || '-'}</small>
              <div className="profile-template-row-actions">
                <Button size="small" aria-label={`Edit profile template ${template.id}`} onClick={() => handleEditProfileTaskTemplate(template)}>
                  编辑 / Edit
                </Button>
                <Button
                  danger
                  size="small"
                  loading={deletingProfileTaskTemplateId === template.id}
                  aria-label={`Delete profile template ${template.id}`}
                  onClick={() => handleDeleteProfileTaskTemplate(template)}
                >
                  删除 / Delete
                </Button>
              </div>
            </div>
          ))}
        </div>
      </article>

      <article className="target-binding-panel">
        <div className="panel-heading compact">
          <div>
            <Typography.Text className="section-title">{bilingual(binding.title)}</Typography.Text>
            <p>{bilingual(binding.description)}</p>
          </div>
          <Tag color={targets.length > 0 ? 'processing' : 'default'}>{`${targets.length} targets`}</Tag>
        </div>
        <div className="target-binding-form">
          <label className="target-field">
            <span>目标名称 / Target name</span>
            <Input
              aria-label="Target name"
              placeholder="checkout-service"
              value={targetName}
              onChange={(event) => setTargetName(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>项目 / Target project ID</span>
            <Input
              aria-label="Target project ID"
              placeholder="project-checkout"
              value={targetProjectId}
              onChange={(event) => setTargetProjectId(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>环境 / Target environment</span>
            <Input
              aria-label="Target environment"
              placeholder="staging"
              value={targetEnvironment}
              onChange={(event) => setTargetEnvironment(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>目标地址 / Target base URL</span>
            <Input
              aria-label="Target base URL"
              placeholder="http://checkout.internal:8080"
              value={targetBaseURL}
              onChange={(event) => setTargetBaseURL(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>画像端点 / Profile endpoint</span>
            <Input
              aria-label="Profile endpoint"
              placeholder="http://127.0.0.1:6060/debug/pprof"
              value={targetProfileEndpoint}
              onChange={(event) => setTargetProfileEndpoint(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>进程名 / Process name</span>
            <Input
              aria-label="Process name"
              placeholder="checkout"
              value={targetProcessName}
              onChange={(event) => setTargetProcessName(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>健康检查路径 / Health check path</span>
            <Input
              aria-label="Health check path"
              placeholder="/health"
              value={targetHealthPath}
              onChange={(event) => setTargetHealthPath(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>期望状态码 / Expected health status</span>
            <Input
              aria-label="Expected health status"
              inputMode="numeric"
              placeholder="200"
              value={targetHealthExpectedStatus}
              onChange={(event) => setTargetHealthExpectedStatus(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>健康检查超时 ms / Health timeout ms</span>
            <Input
              aria-label="Health timeout ms"
              inputMode="numeric"
              placeholder="1000"
              value={targetHealthTimeoutMs}
              onChange={(event) => setTargetHealthTimeoutMs(event.target.value)}
            />
          </label>
          <label className="target-field target-field-wide">
            <span>命令行匹配 / Cmdline contains</span>
            <Input
              aria-label="Cmdline contains"
              placeholder="--config=/etc/checkout/config.yaml"
              value={targetCmdlineContains}
              onChange={(event) => setTargetCmdlineContains(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>CPU 告警阈值 % / CPU alert threshold %</span>
            <Input
              aria-label="CPU alert threshold"
              inputMode="decimal"
              placeholder="70"
              value={targetCpuMaxPercent}
              onChange={(event) => setTargetCpuMaxPercent(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>内存告警阈值 % / Memory alert threshold %</span>
            <Input
              aria-label="Memory alert threshold"
              inputMode="decimal"
              placeholder="75"
              value={targetMemoryMaxPercent}
              onChange={(event) => setTargetMemoryMaxPercent(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>磁盘读告警 B/s / Disk read alert threshold B/s</span>
            <Input
              aria-label="Disk read alert threshold"
              inputMode="decimal"
              placeholder="4096"
              value={targetDiskReadMaxBytesPerSec}
              onChange={(event) => setTargetDiskReadMaxBytesPerSec(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>磁盘写告警 B/s / Disk write alert threshold B/s</span>
            <Input
              aria-label="Disk write alert threshold"
              inputMode="decimal"
              placeholder="4096"
              value={targetDiskWriteMaxBytesPerSec}
              onChange={(event) => setTargetDiskWriteMaxBytesPerSec(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>网络接收告警 B/s / Network RX alert threshold B/s</span>
            <Input
              aria-label="Network RX alert threshold"
              inputMode="decimal"
              placeholder="1048576"
              value={targetNetworkRxMaxBytesPerSec}
              onChange={(event) => setTargetNetworkRxMaxBytesPerSec(event.target.value)}
            />
          </label>
          <label className="target-field">
            <span>网络发送告警 B/s / Network TX alert threshold B/s</span>
            <Input
              aria-label="Network TX alert threshold"
              inputMode="decimal"
              placeholder="1048576"
              value={targetNetworkTxMaxBytesPerSec}
              onChange={(event) => setTargetNetworkTxMaxBytesPerSec(event.target.value)}
            />
          </label>
          <div className="target-selected-agent" aria-label="selected target agent">
            <span>绑定 Agent / Bound agent</span>
            <strong>{boundAgentLabel}</strong>
          </div>
          {!editingTarget ? (
            <Button type="primary" loading={isCreatingTarget} onClick={handleSaveTarget}>
            创建 Target / Create target
          </Button>
          ) : (
            <>
              <Button type="primary" loading={isCreatingTarget} onClick={handleSaveTarget}>
                更新 Target / Update target
              </Button>
              <Button onClick={resetTargetForm}>取消 / Cancel</Button>
            </>
          )}
        </div>
        {targetError ? (
          <p className="target-error" role="alert">
            {targetError}
          </p>
        ) : null}
        <div className="target-binding-list" aria-label="Target bindings / Target bindings">
          {targets.length === 0 ? (
            <p>暂无 Target / No targets yet</p>
          ) : (
            targets.map((target) => {
              const healthResult = targetHealthChecks[target.id] || target.lastHealthCheck;
              const healthHistory = targetHealthHistories[target.id] || [];
              const createdProfileTask = createdProfileTasksByTargetId[target.id];
              const profileTaskDraft = profileTaskDraftsByTargetId[target.id] || profileTaskDefaultDraft;
              const availableProfileTaskTemplates = profileTaskTemplates.length > 0 ? profileTaskTemplates : fallbackProfileTaskTemplates;
              const selectedProfileTaskTemplate =
                availableProfileTaskTemplates.find((template) => template.id === profileTaskDraft.template) ||
                availableProfileTaskTemplates[0] ||
                fallbackProfileTaskTemplates[0];
              const isCommandProfileTaskTemplate =
                selectedProfileTaskTemplate.kind === 'command' || Boolean(selectedProfileTaskTemplate.profileCommand);
              const selectedProfileTaskTypeOptions = profileTaskTemplateProfileTypes(selectedProfileTaskTemplate);
              const hasBoundAgent = target.agentIds.length > 0;
              const canCreateProfileTask = hasBoundAgent;

              return (
                <div className="target-binding-row" key={target.id} aria-label={`target ${target.name}`}>
                  <div>
                    <strong>{target.name}</strong>
                    <span>{target.baseUrl}</span>
                  </div>
                  <span>
                    <small>scope</small>
                    <strong>{`project ${target.projectId || 'default'}`}</strong>
                    <small>{`env ${target.environment || 'default'}`}</small>
                  </span>
                  <span>
                    <small>agent</small>
                    <strong>{target.agentIds.map((agentID) => agentNameByID(agents, agentID)).join(', ') || '-'}</strong>
                  </span>
                  <span>
                    <small>process</small>
                    <strong>{target.processMatch?.name || '-'}</strong>
                  </span>
                  <span>
                    <small>alerts</small>
                    <strong>{targetMetricThresholdsLabel(target.metricThresholds)}</strong>
                  </span>
                  <span className="target-health-cell">
                    <small>health</small>
                    <strong>{targetHealthCheckLabel(target.healthCheck)}</strong>
                    {healthResult ? (
                      <small className={`target-health-result ${targetHealthCheckResultClass(healthResult.status)}`}>
                        {targetHealthCheckResultLabel(healthResult)}
                      </small>
                    ) : null}
                    {targetHealthCheckSourceLabel(healthResult) ? (
                      <small className="target-health-source">{targetHealthCheckSourceLabel(healthResult)}</small>
                    ) : null}
                    {healthHistory.length > 0 ? (
                      <div className="target-health-history" aria-label={`target health history ${target.id}`}>
                        {healthHistory.slice(0, 3).map((historyItem, index) => (
                          <span
                            className={`target-health-history-item ${targetHealthCheckResultClass(historyItem.status)}`}
                            key={`${historyItem.checkedAt || index}-${historyItem.observedStatus}`}
                          >
                            <strong>{targetHealthCheckResultLabel(historyItem)}</strong>
                            {targetHealthCheckSourceLabel(historyItem) ? (
                              <small className="target-health-source">{targetHealthCheckSourceLabel(historyItem)}</small>
                            ) : null}
                            <small>{historyItem.checkedAt || '-'}</small>
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </span>
                  <div className="target-row-actions">
                    {target.profileEndpoint ? <Tag color="geekblue">pprof</Tag> : <Tag>profile -</Tag>}
                    {canCreateProfileTask ? (
                      <div className="target-profile-task-controls">
                        <select
                          aria-label={`Profile task template ${target.name}`}
                          className="target-profile-task-select"
                          value={selectedProfileTaskTemplate.id}
                          onChange={(event) =>
                            setProfileTaskDraftsByTargetId((currentDrafts) => ({
                              ...currentDrafts,
                              [target.id]: {
                                ...profileTaskDraft,
                                template: event.target.value,
                                profileType:
                                  profileTaskTemplateProfileTypes(
                                    availableProfileTaskTemplates.find((template) => template.id === event.target.value) || selectedProfileTaskTemplate,
                                  )[0] || profileTaskDefaultDraft.profileType,
                              },
                            }))
                          }
                        >
                          {availableProfileTaskTemplates.map((template) => (
                            <option key={template.id} value={template.id}>
                              {template.name}
                            </option>
                          ))}
                        </select>
                        {isCommandProfileTaskTemplate ? <Tag color="gold">cmd</Tag> : null}
                        {!isCommandProfileTaskTemplate ? (
                        <select
                          aria-label={`Profile task type ${target.name}`}
                          className="target-profile-task-select"
                          value={profileTaskDraft.profileType}
                          onChange={(event) =>
                            setProfileTaskDraftsByTargetId((currentDrafts) => ({
                              ...currentDrafts,
                              [target.id]: {
                                ...profileTaskDraft,
                                profileType: event.target.value,
                              },
                            }))
                          }
                        >
                          {selectedProfileTaskTypeOptions.map((profileType) => (
                            <option key={profileType} value={profileType}>
                              {profileType}
                            </option>
                          ))}
                        </select>
                        ) : selectedProfileTaskTemplate.requiresPid !== false ? (
                          <Input
                            aria-label={`Command profiler PID ${target.name}`}
                            size="small"
                            inputMode="numeric"
                            placeholder="PID"
                            value={profileTaskDraft.commandPid}
                            onChange={(event) =>
                              setProfileTaskDraftsByTargetId((currentDrafts) => ({
                                ...currentDrafts,
                                [target.id]: {
                                  ...profileTaskDraft,
                                  commandPid: event.target.value,
                                },
                              }))
                            }
                          />
                        ) : null}
                        <Input
                          aria-label={`Profile task seconds ${target.name}`}
                          size="small"
                          inputMode="numeric"
                          min={1}
                          max={300}
                          value={profileTaskDraft.profileSeconds}
                          onChange={(event) =>
                            setProfileTaskDraftsByTargetId((currentDrafts) => ({
                              ...currentDrafts,
                              [target.id]: {
                                ...profileTaskDraft,
                                profileSeconds: event.target.value,
                              },
                            }))
                          }
                        />
                      </div>
                    ) : null}
                    {canCreateProfileTask ? (
                      <Button
                        size="small"
                        loading={creatingProfileTaskTargetId === target.id}
                        aria-label={`Create profile task ${target.name}`}
                        onClick={() => handleCreateProfileTask(target)}
                      >
                        采集 / Profile
                      </Button>
                    ) : null}
                    <Button
                      size="small"
                      loading={checkingTargetId === target.id}
                      aria-label={`Check target health ${target.name}`}
                      onClick={() => handleCheckTargetHealth(target)}
                    >
                      健康检查 / Check
                    </Button>
                    <Button size="small" aria-label={`Edit target ${target.name}`} onClick={() => handleEditTarget(target)}>
                      编辑 / Edit
                    </Button>
                    {hasBoundAgent ? (
                      <Button
                        size="small"
                        loading={unbindingTargetId === target.id}
                        aria-label={`Unbind agent ${target.name}`}
                        onClick={() => handleUnbindTargetAgent(target)}
                      >
                        瑙ｇ粦 / Unbind
                      </Button>
                    ) : null}
                    {createdProfileTask ? (
                      <small className="target-profile-task-status">{`profile task ${createdProfileTask.id} queued`}</small>
                    ) : null}
                    <Button
                      danger
                      size="small"
                      loading={deletingTargetId === target.id}
                      aria-label={`Delete target ${target.name}`}
                      onClick={() => handleDeleteTarget(target)}
                    >
                      删除 / Delete
                    </Button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </article>
    </section>
  );
}

function ScenariosPage({
  page,
  scenarioDrafts,
  scenarioFlowSteps,
  onAddScenarioFlowStep,
  onUpdateScenarioFlowStep,
}: {
  page: WorkspacePage;
  scenarioDrafts: ScenarioDraft[];
  scenarioFlowSteps: ScenarioFlowStep[];
  onAddScenarioFlowStep: () => void;
  onUpdateScenarioFlowStep: (index: number, patch: Partial<ScenarioFlowStep>) => void;
}) {
  return (
    <section className="scenario-layout" aria-label={bilingual(page.label)}>
      <article className="flow-builder-panel">
        <div className="panel-heading compact">
          <Typography.Text className="section-title">调用链编排 / Request flow builder</Typography.Text>
          <Button size="small" onClick={onAddScenarioFlowStep}>
            新增步骤 / Add step
          </Button>
        </div>
        <div className="flow-lane">
          {scenarioFlowSteps.map((step, index) => (
            <div className="flow-step" key={step.id || `${step.name}-${index}`}>
              <div className="flow-step-main">
                <span>{scenarioFlowStepLabel(step, index)}</span>
                {step.type === 'request' ? (
                  <div className="flow-step-controls">
                    <Checkbox
                      aria-label={`Enable flow step ${index + 1}`}
                      checked={Boolean(step.enabled)}
                      onChange={(event) => onUpdateScenarioFlowStep(index, { enabled: event.target.checked })}
                    >
                      执行 / Execute
                    </Checkbox>
                    <select
                      aria-label={`Flow step method ${index + 1}`}
                      className="flow-step-method"
                      value={step.method || 'GET'}
                      onChange={(event) => onUpdateScenarioFlowStep(index, { method: event.target.value })}
                    >
                      {methodOptions.map((option) => (
                        <option key={option.value} value={option.value} aria-label={`Flow step method option ${option.label}`}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <Input
                      aria-label={`Flow step path ${index + 1}`}
                      size="small"
                      value={step.path || '/'}
                      onChange={(event) => onUpdateScenarioFlowStep(index, { path: event.target.value })}
                    />
                  </div>
                ) : step.type === 'extract' ? (
                  <div className="flow-step-controls">
                    <Checkbox
                      aria-label={`Enable extract flow step ${index + 1}`}
                      checked={Boolean(step.enabled)}
                      onChange={(event) => onUpdateScenarioFlowStep(index, { enabled: event.target.checked })}
                    >
                      提取 / Extract
                    </Checkbox>
                    <Input
                      aria-label={`Extractor variable name ${index + 1}`}
                      size="small"
                      value={primaryScenarioExtractor(step).name}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { extractors: patchPrimaryScenarioExtractor(step, { name: event.target.value }) })
                      }
                    />
                    <select
                      aria-label={`Extractor source ${index + 1}`}
                      className="flow-step-method"
                      value={primaryScenarioExtractor(step).source}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { extractors: patchPrimaryScenarioExtractor(step, { source: event.target.value }) })
                      }
                    >
                      {extractorSourceOptions.map((option) => (
                        <option key={option.value} value={option.value} aria-label={`Extractor source option ${option.label}`}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <Input
                      aria-label={`Extractor path ${index + 1}`}
                      size="small"
                      value={primaryScenarioExtractor(step).path}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { extractors: patchPrimaryScenarioExtractor(step, { path: event.target.value }) })
                      }
                    />
                  </div>
                ) : step.type === 'assertion' ? (
                  <div className="flow-step-controls flow-step-assertion-controls">
                    <Checkbox
                      aria-label={`Enable assertion flow step ${index + 1}`}
                      checked={Boolean(step.enabled)}
                      onChange={(event) => onUpdateScenarioFlowStep(index, { enabled: event.target.checked })}
                    >
                      Assert
                    </Checkbox>
                    <select
                      aria-label={`Assertion source ${index + 1}`}
                      className="flow-step-method"
                      value={primaryScenarioAssertion(step).source}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { assertions: patchPrimaryScenarioAssertion(step, { source: event.target.value }) })
                      }
                    >
                      {assertionSourceOptions.map((option) => (
                        <option key={option.value} value={option.value} aria-label={`Assertion source option ${option.label}`}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <Input
                      aria-label={`Assertion path ${index + 1}`}
                      size="small"
                      value={primaryScenarioAssertion(step).path}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { assertions: patchPrimaryScenarioAssertion(step, { path: event.target.value }) })
                      }
                    />
                    <select
                      aria-label={`Assertion operator ${index + 1}`}
                      className="flow-step-method"
                      value={primaryScenarioAssertion(step).operator}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { assertions: patchPrimaryScenarioAssertion(step, { operator: event.target.value }) })
                      }
                    >
                      {assertionOperatorOptions.map((option) => (
                        <option key={option.value} value={option.value} aria-label={`Assertion operator option ${option.label}`}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <Input
                      aria-label={`Assertion expected ${index + 1}`}
                      size="small"
                      value={primaryScenarioAssertion(step).expected}
                      onChange={(event) =>
                        onUpdateScenarioFlowStep(index, { assertions: patchPrimaryScenarioAssertion(step, { expected: event.target.value }) })
                      }
                    />
                  </div>
                ) : null}
                <div className="flow-step-controls flow-step-when-controls">
                  <Checkbox
                    aria-label={`Enable when condition ${index + 1}`}
                    checked={Boolean(step.when)}
                    onChange={(event) =>
                      onUpdateScenarioFlowStep(index, {
                        when: event.target.checked ? primaryScenarioCondition(step) : undefined,
                      })
                    }
                  >
                    条件 / When
                  </Checkbox>
                  {step.when ? (
                    <>
                      <select
                        aria-label={`When source ${index + 1}`}
                        className="flow-step-method"
                        value={primaryScenarioCondition(step).source}
                        onChange={(event) => onUpdateScenarioFlowStep(index, { when: patchScenarioCondition(step, { source: event.target.value }) })}
                      >
                        {conditionSourceOptions.map((option) => (
                          <option key={option.value} value={option.value} aria-label={`When source option ${option.label}`}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                      <Input
                        aria-label={`When path ${index + 1}`}
                        size="small"
                        value={primaryScenarioCondition(step).path}
                        onChange={(event) => onUpdateScenarioFlowStep(index, { when: patchScenarioCondition(step, { path: event.target.value }) })}
                      />
                      <select
                        aria-label={`When operator ${index + 1}`}
                        className="flow-step-method"
                        value={primaryScenarioCondition(step).operator}
                        onChange={(event) => onUpdateScenarioFlowStep(index, { when: patchScenarioCondition(step, { operator: event.target.value }) })}
                      >
                        {assertionOperatorOptions.map((option) => (
                          <option key={option.value} value={option.value} aria-label={`When operator option ${option.label}`}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                      <Input
                        aria-label={`When expected ${index + 1}`}
                        size="small"
                        value={primaryScenarioCondition(step).expected}
                        onChange={(event) => onUpdateScenarioFlowStep(index, { when: patchScenarioCondition(step, { expected: event.target.value }) })}
                      />
                    </>
                  ) : null}
                </div>
              </div>
            </div>
          ))}
        </div>
        <div className="scenario-draft-list" aria-label="创建的场景 / Created scenarios">
          <Typography.Text className="section-title">创建的场景 / Created scenarios</Typography.Text>
          {scenarioDrafts.length === 0 ? (
            <p>暂无自定义场景 / No custom scenarios yet</p>
          ) : (
            scenarioDrafts.map((scenario) => (
              <div className="scenario-draft-row" key={scenario.id} aria-label={`scenario ${scenario.name}`}>
                <div>
                  <strong>{scenario.name}</strong>
                  <span>{scenarioRequestLine(scenario)}</span>
                  <div className="scenario-draft-meta">
                    <small>{`project ${scenario.projectId}`}</small>
                    <small>{`env ${scenario.environment}`}</small>
                    <small>{`timeout ${scenario.timeoutMs} ms`}</small>
                    <small>{`retry ${scenario.retryCount}`}</small>
                  </div>
                  {scenario.queryVariants.map((variant, index) => (
                    <div className="scenario-query-variant" key={`${variant.name}-${index}`}>
                      <small>{`query weight ${variant.weight}`}</small>
                      <pre>{variant.queryParams}</pre>
                    </div>
                  ))}
                  {scenario.headers.length > 0 ? (
                    <pre>{scenario.headers.map((header) => `${header.key}: ${header.value}`).join('\n')}</pre>
                  ) : null}
                  {scenario.bodyVariants.map((variant, index) => (
                    <div className="scenario-body-variant" key={`${variant.name}-${index}`}>
                      <small>{`body weight ${variant.weight}`}</small>
                      <pre>{variant.body}</pre>
                    </div>
                  ))}
                  {scenario.flowSteps.length > 0 ? (
                    <div className="scenario-flow-steps" aria-label={`flow steps for ${scenario.name}`}>
                      {scenario.flowSteps.map((step, index) => (
                        <small
                          className={`scenario-flow-step ${
                            isActiveScenarioFlowStep(step) ? 'scenario-flow-step-executable' : 'scenario-flow-step-draft'
                          }`}
                          key={step.id || `${step.name}-${index}`}
                        >
                          <span>{scenarioFlowStepDisplayLabel(step, index)}</span>
                          {shouldShowScenarioFlowStepName(step) ? (
                            <span className="scenario-flow-step-name">{scenarioFlowStepLabel(step, index)}</span>
                          ) : null}
                          {scenarioFlowStepExtractorSummary(step) ? (
                            <span className="scenario-flow-step-name">{scenarioFlowStepExtractorSummary(step)}</span>
                          ) : null}
                          {scenarioFlowStepAssertionSummary(step) ? (
                            <span className="scenario-flow-step-name">{scenarioFlowStepAssertionSummary(step)}</span>
                          ) : null}
                          {scenarioFlowStepWhenSummary(step) ? (
                            <span className="scenario-flow-step-name">{scenarioFlowStepWhenSummary(step)}</span>
                          ) : null}
                          <em>{scenarioFlowStepStatusLabel(step)}</em>
                        </small>
                      ))}
                    </div>
                  ) : null}
                  <small>{scenario.assertion}</small>
                </div>
                <Tag color="blue">{scenario.protocol}</Tag>
              </div>
            ))
          )}
        </div>
      </article>

      <article className="adapter-panel">
        <Typography.Text className="section-title">协议适配器 / Protocol adapters</Typography.Text>
        <div className="adapter-grid">
          {['HTTP', 'gRPC', 'Dubbo', 'Thrift', 'Custom RPC', 'Script hook'].map((adapter) => (
            <span key={adapter}>{adapter}</span>
          ))}
        </div>
      </article>

      <article className="variables-panel">
        <Typography.Text className="section-title">变量与断言 / Variables & assertions</Typography.Text>
        <div className="rule-list">
          {page.modules.slice(2).map((module) => (
            <div key={module.title.en}>
              <strong>{bilingual(module.title)}</strong>
              <span>{module.meta}</span>
            </div>
          ))}
        </div>
      </article>
    </section>
  );
}

function RunsPage({
  page,
  workspaceScope,
  scenarioDrafts,
  backendScenarioIds,
}: {
  page: WorkspacePage;
  workspaceScope: WorkspaceScope | null;
  scenarioDrafts: ScenarioDraft[];
  backendScenarioIds: Set<string>;
}) {
  const [selectedScenarioId, setSelectedScenarioId] = useState('');
  const [totalRequests, setTotalRequests] = useState('20');
  const [concurrency, setConcurrency] = useState('4');
  const [timeoutMs, setTimeoutMs] = useState('1000');
  const [maxErrorRatePercent, setMaxErrorRatePercent] = useState('0.5');
  const [maxP95LatencyMs, setMaxP95LatencyMs] = useState('800');
  const [runResult, setRunResult] = useState<RunResult | null>(null);
  const [runEvents, setRunEvents] = useState<RunEventRecord[]>([]);
  const [runHistory, setRunHistory] = useState<RunResult[]>([]);
  const [runTargets, setRunTargets] = useState<TargetRecord[]>([]);
  const [selectedTargetId, setSelectedTargetId] = useState('');
  const [runError, setRunError] = useState('');
  const [isRunning, setIsRunning] = useState(false);

  useEffect(() => {
    const backendScenarioDrafts = scenarioDrafts.filter((scenario) => backendScenarioIds.has(scenario.id));
    const preferredScenarioDrafts = backendScenarioDrafts.length > 0 ? backendScenarioDrafts : scenarioDrafts;

    if (preferredScenarioDrafts.length === 0) {
      setSelectedScenarioId('');
      return;
    }

    setSelectedScenarioId((currentScenarioId) => {
      if (currentScenarioId && preferredScenarioDrafts.some((scenario) => scenario.id === currentScenarioId)) {
        return currentScenarioId;
      }
      return preferredScenarioDrafts[0].id;
    });
  }, [scenarioDrafts, backendScenarioIds]);

  useEffect(() => {
    let isMounted = true;

    Promise.allSettled([loadBackendRuns(workspaceScope), loadBackendTargets(workspaceScope)]).then(([runsResult, targetsResult]) => {
      if (!isMounted) {
        return;
      }
      if (runsResult.status === 'fulfilled') {
        setRunHistory(runsResult.value);
      }
      if (targetsResult.status === 'fulfilled') {
        setRunTargets(targetsResult.value);
      }
    });

    return () => {
      isMounted = false;
    };
  }, [workspaceScope]);

  useEffect(() => {
    if (runTargets.length === 0) {
      setSelectedTargetId('');
      return;
    }

    setSelectedTargetId((currentTargetId) => {
      if (currentTargetId && runTargets.some((target) => target.id === currentTargetId)) {
        return currentTargetId;
      }
      return runTargets[0].id;
    });
  }, [runTargets]);

  const selectedScenario = scenarioDrafts.find((scenario) => scenario.id === selectedScenarioId);
  const selectedTarget = runTargets.find((target) => target.id === selectedTargetId);
  const selectedScenarioExecutableFlowSteps = selectedScenario ? executableScenarioFlowSteps(selectedScenario.flowSteps) : [];
  const scenarioOptions = scenarioDrafts.map((scenario) => ({
    label: scenario.name,
    value: scenario.id,
  }));
  const targetOptions = runTargets.map((target) => ({
    label: `${target.name} (${target.environment})`,
    value: target.id,
  }));

  const rememberRunResult = (run: RunResult) => {
    setRunResult(run);
    setRunHistory((currentHistory) => [run, ...currentHistory.filter((historyRun) => historyRun.id !== run.id)]);
  };

  useEffect(() => {
    if (!runResult || !isRunActive(runResult.status)) {
      return;
    }

    let isMounted = true;
    let timerID: number | undefined;

    const pollRun = async () => {
      try {
        const updatedRun = await loadBackendRun(runResult.id, workspaceScope);
        if (!isMounted) {
          return;
        }
        rememberRunResult(updatedRun);
        const active = isRunActive(updatedRun.status);
        setIsRunning(active);
        if (active) {
          timerID = window.setTimeout(pollRun, 500);
        }
      } catch (error) {
        if (!isMounted) {
          return;
        }
        setRunError(error instanceof Error ? error.message : 'Failed to refresh run status.');
        timerID = window.setTimeout(pollRun, 1000);
      }
    };

    timerID = window.setTimeout(pollRun, 250);

    return () => {
      isMounted = false;
      if (timerID !== undefined) {
        window.clearTimeout(timerID);
      }
    };
  }, [runResult?.id, runResult?.status, workspaceScope]);

  useEffect(() => {
    if (!runResult) {
      setRunEvents([]);
      return;
    }

    if (typeof window.EventSource === 'function') {
      const eventSource = new window.EventSource(backendRunEventsStreamURL(runResult.id, workspaceScope));
      const handleRunEvent = (event: MessageEvent<string>) => {
        try {
          const runEvent = JSON.parse(event.data) as RunEventRecord;
          setRunEvents((currentEvents) => mergeRunEvent(currentEvents, runEvent));
        } catch {
          // Ignore malformed stream events; the status poll still keeps the run summary current.
        }
      };

      for (const eventType of runEventStreamTypes) {
        eventSource.addEventListener(eventType, handleRunEvent);
      }

      return () => {
        for (const eventType of runEventStreamTypes) {
          eventSource.removeEventListener(eventType, handleRunEvent);
        }
        eventSource.close();
      };
    }

    let isMounted = true;
    let timerID: number | undefined;

    const pollRunEvents = async () => {
      try {
        const events = await loadBackendRunEvents(runResult.id, workspaceScope);
        if (!isMounted) {
          return;
        }
        setRunEvents(events);
        if (isRunActive(runResult.status)) {
          timerID = window.setTimeout(pollRunEvents, 500);
        }
      } catch {
        if (isMounted && isRunActive(runResult.status)) {
          timerID = window.setTimeout(pollRunEvents, 1000);
        }
      }
    };

    pollRunEvents();

    return () => {
      isMounted = false;
      if (timerID !== undefined) {
        window.clearTimeout(timerID);
      }
    };
  }, [runResult?.id, runResult?.status, workspaceScope]);

  const startRun = async () => {
    if (!selectedScenario) {
      setRunError('Please create or load a scenario before starting a run.');
      return;
    }

    setIsRunning(true);
    setRunError('');
    setRunEvents([]);
    let runIsActive = false;
    try {
      const result = await createBackendRun(
        {
          scenarioId: selectedScenario.id,
          targetId: selectedTarget?.id,
          totalRequests: parseRunNumber(totalRequests, 1),
          concurrency: parseRunNumber(concurrency, 1),
          timeoutMs: parseRunNumber(timeoutMs, Number.parseInt(selectedScenario.timeoutMs, 10) || 1000),
          maxErrorRatePercent: parseOptionalRunDecimal(maxErrorRatePercent),
          maxP95LatencyMs: parseOptionalRunDecimal(maxP95LatencyMs),
        },
        workspaceScope,
      );
      runIsActive = isRunActive(result.status);
      rememberRunResult(result);
      setIsRunning(runIsActive);
    } catch (error) {
      setRunError(error instanceof Error ? error.message : 'Failed to start run.');
    } finally {
      if (!runIsActive) {
        setIsRunning(false);
      }
    }
  };

  const stopRun = async () => {
    if (!runResult || !isRunActive(runResult.status)) {
      return;
    }

    setRunError('');
    try {
      const stoppedRun = await stopBackendRun(runResult.id, workspaceScope);
      rememberRunResult(stoppedRun);
      setIsRunning(isRunActive(stoppedRun.status));
    } catch (error) {
      setRunError(error instanceof Error ? error.message : 'Failed to stop run.');
    }
  };

  return (
    <section className="runs-layout" aria-label={bilingual(page.label)}>
      <article className="run-control-panel">
        <div className="panel-heading">
          <div>
            <Typography.Text className="section-title">运行控制台 / Run control</Typography.Text>
            <p>{selectedScenario ? scenarioRequestLine(selectedScenario) : 'Create a scenario before starting a run'}</p>
          </div>
          <Tag color={selectedScenario ? 'processing' : 'default'}>
            {selectedScenario ? 'armed' : 'no scenario'}
          </Tag>
        </div>

        <div className="run-form-grid">
          <label className="run-field">
            <span>场景 / Scenario</span>
            <Select
              aria-label="Run scenario"
              value={selectedScenarioId || undefined}
              placeholder="Select scenario"
              options={scenarioOptions}
              onChange={setSelectedScenarioId}
            />
          </label>
          <label className="run-field">
            <span>目标 / Target</span>
            <Select
              aria-label="Run target"
              value={selectedTargetId || undefined}
              placeholder="Use scenario base URL"
              options={targetOptions}
              disabled={runTargets.length === 0}
              onChange={setSelectedTargetId}
            />
          </label>
          <label className="run-field">
            <span>请求数 / Total requests</span>
            <Input
              aria-label="Total requests"
              inputMode="numeric"
              value={totalRequests}
              onChange={(event) => setTotalRequests(event.target.value)}
            />
          </label>
          <label className="run-field">
            <span>并发 / Concurrency</span>
            <Input
              aria-label="Concurrency"
              inputMode="numeric"
              value={concurrency}
              onChange={(event) => setConcurrency(event.target.value)}
            />
          </label>
          <label className="run-field">
            <span>超时毫秒 / Timeout ms</span>
            <Input
              aria-label="Run timeout"
              inputMode="numeric"
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </label>
          <label className="run-field">
            <span>Error guard / Max error %</span>
            <Input
              aria-label="Max error rate percent"
              inputMode="decimal"
              value={maxErrorRatePercent}
              onChange={(event) => setMaxErrorRatePercent(event.target.value)}
            />
          </label>
          <label className="run-field">
            <span>Latency guard / Max p95 ms</span>
            <Input
              aria-label="Max p95 latency ms"
              inputMode="decimal"
              value={maxP95LatencyMs}
              onChange={(event) => setMaxP95LatencyMs(event.target.value)}
            />
          </label>
        </div>

        {selectedScenario ? (
          <div className="selected-run-scenario" aria-label={`selected run scenario ${selectedScenario.name}`}>
            <strong>{selectedScenario.name}</strong>
            <span>{scenarioRequestLine(selectedScenario)}</span>
            {selectedScenarioExecutableFlowSteps.length > 0 ? (
              <div className="selected-run-flow" aria-label={`executable flow for ${selectedScenario.name}`}>
                <small>可执行链路 / Executable flow</small>
                <div className="selected-run-flow-steps">
                  {selectedScenarioExecutableFlowSteps.map((step, index) => (
                    <span key={step.id || `${step.path}-${index}`}>{runFlowStepLabel(step, index)}</span>
                  ))}
                </div>
              </div>
            ) : null}
            <small>{`${selectedScenario.method} · timeout ${selectedScenario.timeoutMs} ms · ${selectedScenario.assertion}`}</small>
          </div>
        ) : (
          <div className="selected-run-scenario empty">暂无可运行场景 / No runnable scenarios</div>
        )}

        {selectedTarget ? (
          <div className="selected-run-target" aria-label={`selected run target ${selectedTarget.name}`}>
            <strong>{selectedTarget.name}</strong>
            <span>{selectedTarget.baseUrl}</span>
            <small>{`${selectedTarget.environment} · profile ${selectedTarget.profileEndpoint || '-'}`}</small>
          </div>
        ) : (
          <div className="selected-run-target empty">未选择 Target，将使用场景 baseUrl / No target selected; using scenario baseUrl</div>
        )}

        <div className="control-actions">
          <Button type="primary" loading={isRunning} disabled={!selectedScenario} onClick={startRun}>
            启动压测 / Start run
          </Button>
          <Button danger disabled={!runResult || !isRunActive(runResult.status)} onClick={stopRun}>
            停止 / Stop
          </Button>
        </div>

        {runError ? <p className="run-error">{runError}</p> : null}
        {runResult ? (
          <div className="run-result-panel" aria-label={`run result ${runResult.id}`}>
            <div className="run-result-heading">
              <strong>{runResult.id}</strong>
              <Tag color={runResult.status === 'finished' ? 'success' : 'processing'}>{runResult.status}</Tag>
            </div>
            <div className="run-result-grid">
              <span>
                <small>scenario</small>
                <strong>{runResult.scenarioName || runResult.name}</strong>
              </span>
              <span>
                <small>target</small>
                <strong>{runResult.targetName || '-'}</strong>
              </span>
              <span>
                <small>success</small>
                <strong>{`${runResult.successRequests} / ${runResult.totalRequests}`}</strong>
              </span>
              <span>
                <small>QPS</small>
                <strong>{runResult.qps.toFixed(2)}</strong>
              </span>
              <span>
                <small>p95</small>
                <strong>{`${runResult.p95LatencyMs.toFixed(2)} ms`}</strong>
              </span>
            </div>
          </div>
        ) : null}

        {runResult ? (
          <div className="run-events-panel" aria-label={`run events ${runResult.id}`}>
            <div className="panel-heading compact">
              <Typography.Text className="section-title">事件流 / Event stream</Typography.Text>
              <Tag color={runEvents.length > 0 ? 'processing' : 'default'}>{`${runEvents.length} events`}</Tag>
            </div>
            <div className="run-events-list">
              {runEvents.length === 0 ? (
                <p>暂无运行事件 / No run events yet</p>
              ) : (
                runEvents.map((event) => (
                  <div className="run-event-row" key={event.id} aria-label={`run event ${event.id}`}>
                    <div>
                      <strong>{event.type}</strong>
                      <span>{event.message}</span>
                      <small>{formatRunCreatedAt(event.createdAt)}</small>
                    </div>
                    <span>
                      <small>status</small>
                      <strong>{event.status}</strong>
                    </span>
                    <span>
                      <small>success</small>
                      <strong>{`${event.successRequests} / ${event.totalRequests}`}</strong>
                    </span>
                    <span>
                      <small>QPS</small>
                      <strong>{event.qps.toFixed(2)}</strong>
                    </span>
                    <span>
                      <small>p95</small>
                      <strong>{`${event.p95LatencyMs.toFixed(2)} ms`}</strong>
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>
        ) : null}

        <div className="run-history-panel" aria-label="运行历史 / Run history">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">运行历史 / Run history</Typography.Text>
            <Tag color="default">{`${runHistory.length} runs`}</Tag>
          </div>
          <div className="run-history-list">
            {runHistory.length === 0 ? (
              <p>暂无运行历史 / No run history yet</p>
            ) : (
              runHistory.map((run) => (
                <div className="run-history-row" key={run.id} aria-label={`run history ${run.id}`}>
                  <div>
                    <strong>{run.id}</strong>
                    <span>{run.scenarioName || run.name}</span>
                    <small>{formatRunCreatedAt(run.createdAt)}</small>
                  </div>
                  <span>
                    <small>target</small>
                    <strong>{run.targetName || '-'}</strong>
                  </span>
                  <span>
                    <small>success</small>
                    <strong>{`${run.successRequests} / ${run.totalRequests}`}</strong>
                  </span>
                  <span>
                    <small>QPS</small>
                    <strong>{run.qps.toFixed(2)}</strong>
                  </span>
                  <span>
                    <small>p95</small>
                    <strong>{`${run.p95LatencyMs.toFixed(2)} ms`}</strong>
                  </span>
                  <div className="run-history-status" aria-label={`run history status ${run.id}`}>
                    <Tag color={run.status === 'finished' ? 'success' : 'processing'}>{run.status}</Tag>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </article>

      <article className="stage-timeline-panel">
        <Typography.Text className="section-title">阶段时间线 / Stage timeline</Typography.Text>
        <div className="timeline-rail">
          {page.modules.slice(1, 4).map((module, index) => (
            <div className="timeline-node" key={module.title.en}>
              <span>{index + 1}</span>
              <strong>{bilingual(module.title)}</strong>
              <small>{module.meta}</small>
            </div>
          ))}
        </div>
      </article>

      <article className="guardrail-panel">
        <Typography.Text className="section-title">保护阈值 / Guardrails</Typography.Text>
        <div className="guardrail-list">
          {['error rate > 0.5%', 'p99 > 800 ms', 'CPU > 90%', 'MEM > 85%'].map((item) => (
            <span key={item}>{item}</span>
          ))}
        </div>
      </article>
    </section>
  );
}

function ReportsPage({ page, workspaceScope }: { page: WorkspacePage; workspaceScope: WorkspaceScope | null }) {
  const [profileArtifacts, setProfileArtifacts] = useState<ProfileArtifactRecord[]>([]);
  const [profileTasks, setProfileTasks] = useState<ProfileTaskRecord[]>([]);
  const [profileTaskSourceFilter, setProfileTaskSourceFilter] = useState('');
  const [retryingProfileTaskID, setRetryingProfileTaskID] = useState<string | null>(null);
  const [runReport, setRunReport] = useState<RunReportRecord | null>(null);
  const [runComparison, setRunComparison] = useState<RunComparisonReport | null>(null);
  const [profileArtifactComparison, setProfileArtifactComparison] = useState<ProfileArtifactComparisonReport | null>(null);
  const [processTrendComparison, setProcessTrendComparison] = useState<ProcessTrendComparisonReport | null>(null);
  const [reportAgents, setReportAgents] = useState<AgentRecord[]>([]);
  const [agentMetricsTimeline, setAgentMetricsTimeline] = useState<AgentMetrics[]>([]);

  useEffect(() => {
    let isMounted = true;
    const bootstrapReport = readBootstrapRunReport();
    if (bootstrapReport) {
      setRunReport(bootstrapReport);
      setProfileArtifacts(bootstrapReport.profileArtifacts);
    }

    Promise.allSettled([
      loadBackendProfileArtifacts(workspaceScope),
      loadBackendAgents(workspaceScope),
      loadBackendRuns(workspaceScope),
      loadBackendRunComparison(5, workspaceScope),
      loadBackendProfileArtifactComparison(5, workspaceScope),
      loadBackendProcessTrendComparison(5, workspaceScope),
    ]).then(([artifactsResult, agentsResult, runsResult, comparisonResult, artifactComparisonResult, processTrendComparisonResult]) => {
      if (!isMounted) {
        return;
      }
      if (artifactsResult.status === 'fulfilled') {
        setProfileArtifacts(artifactsResult.value);
      }
      if (agentsResult.status === 'fulfilled') {
        setReportAgents(agentsResult.value);
        const firstAgent = agentsResult.value[0];
        if (firstAgent) {
          loadBackendAgentMetrics(firstAgent.id, workspaceScope)
            .then((metrics) => {
              if (isMounted) {
                setAgentMetricsTimeline(metrics);
              }
            })
            .catch(() => {
              // Reports keep their profile artifact content when metrics timeline is not reachable.
            });
        }
      }
      if (runsResult.status === 'fulfilled') {
        const latestRun = runsResult.value[0];
        if (latestRun) {
          loadBackendRunReport(latestRun.id, workspaceScope)
            .then((report) => {
              if (isMounted) {
                setRunReport(report);
                if (report.profileArtifacts.length > 0) {
                  setProfileArtifacts((currentArtifacts) => (currentArtifacts.length > 0 ? currentArtifacts : report.profileArtifacts));
                }
                loadBackendProfileTasks({ runId: report.run.id, limit: 20, workspaceScope })
                  .then((tasks) => {
                    if (isMounted) {
                      setProfileTasks(tasks);
                    }
                  })
                  .catch(() => {
                    // Profile task queue is secondary to the report summary.
                  });
              }
            })
            .catch(() => {
              // Reports can still show profile artifacts and agent metrics if report aggregation is unavailable.
            });
        }
      }
      if (comparisonResult.status === 'fulfilled') {
        setRunComparison(comparisonResult.value);
      }
      if (artifactComparisonResult.status === 'fulfilled') {
        setProfileArtifactComparison(artifactComparisonResult.value);
      }
      if (processTrendComparisonResult.status === 'fulfilled') {
        setProcessTrendComparison(processTrendComparisonResult.value);
      }
    });

    return () => {
      isMounted = false;
    };
  }, [workspaceScope]);

  const selectedReportAgent = reportAgents[0];
  const latestMetricsSample = agentMetricsTimeline[agentMetricsTimeline.length - 1];
  const latestTimelineProcess = topAgentProcess(latestMetricsSample);
  const cpuMax = maxMetricValue(agentMetricsTimeline, (metrics) => metrics.cpuUsagePercent);
  const memoryMax = maxMetricValue(agentMetricsTimeline, (metrics) => metrics.memoryUsagePercent);
  const reportRunName = runReport ? runDisplayName(runReport.run) : 'No run report';
  const slowSamples = runReport?.slowSamples ?? [];
  const errorSamples = runReport?.errorSamples ?? [];
  const reportAlerts = runReport?.alerts ?? [];
  const targetMetrics = runReport?.targetMetrics ?? null;
  const targetHealthChecks = runReport?.targetHealthChecks ?? [];
  const targetProcessSnapshot = targetMetrics ? filterProcessesByTargetMatch(targetMetrics.latestProcessSnapshot, targetMetrics.processMatch) : [];
  const targetProcessTrends = targetMetrics?.processTrends ?? [];
  const targetProcessLabel = targetProcessMatchLabel(targetMetrics?.processMatch);
  const hasRequestSamples = Boolean(runReport && (slowSamples.length > 0 || errorSamples.length > 0));

  const retryProfileTask = (taskID: string) => {
    setRetryingProfileTaskID(taskID);
    retryBackendProfileTask(taskID, workspaceScope)
      .then((retriedTask) => {
        setProfileTasks((currentTasks) => currentTasks.map((task) => (task.id === retriedTask.id ? retriedTask : task)));
      })
      .catch(() => {
        // The failed task remains visible so the operator can retry again after checking the backend.
      })
      .finally(() => {
        setRetryingProfileTaskID((currentTaskID) => (currentTaskID === taskID ? null : currentTaskID));
      });
  };

  const handleProfileTaskSourceFilterChange = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const nextSource = event.target.value;
    setProfileTaskSourceFilter(nextSource);
    const runID = runReport?.run.id;
    if (!runID) {
      return;
    }
    loadBackendProfileTasks({ runId: runID, source: nextSource || undefined, limit: 20, workspaceScope })
      .then((tasks) => {
        setProfileTasks(tasks);
      })
      .catch(() => {
        // Profile task source filtering is secondary to the report summary.
      });
  };

  return (
    <section className="reports-layout" aria-label={bilingual(page.label)}>
      {runReport ? (
        <article className="run-report-panel" aria-label={`run report ${runReport.run.id}`}>
          <div className="panel-heading compact">
            <div>
              <Typography.Text className="section-title">Run report</Typography.Text>
              <p>{reportRunName}</p>
            </div>
            <Tag color={runStatusColor(runReport.run.status)}>{runReport.run.status}</Tag>
          </div>
          <div className="run-report-grid">
            <span>{`success ${formatPercent(runReport.summary.successRatePercent)}`}</span>
            <span>{`errors ${formatPercent(runReport.summary.errorRatePercent)}`}</span>
            <span>{`events ${runReport.summary.eventCount}`}</span>
            <span>{`artifacts ${runReport.summary.profileArtifactCount}`}</span>
            <span>{`slow ${runReport.summary.slowSampleCount ?? slowSamples.length}`}</span>
            <span>{`error samples ${runReport.summary.errorSampleCount ?? errorSamples.length}`}</span>
            <span>{`alerts ${runReport.summary.alertCount ?? reportAlerts.length}`}</span>
            <span>{`qps ${formatMetricNumber(runReport.run.qps)}`}</span>
            <span>{`p95 ${formatLatencyMetric(runReport.run.p95LatencyMs)}`}</span>
          </div>
        </article>
      ) : null}
      {runReport && reportAlerts.length > 0 ? (
        <article className="run-alerts-panel" aria-label={`run report alerts ${runReport.run.id}`}>
          <div className="panel-heading compact">
            <Typography.Text className="section-title">告警摘要 / Alert summary</Typography.Text>
            <Tag color="error">{`${reportAlerts.length} alerts`}</Tag>
          </div>
          <div className="run-alert-list">
            {reportAlerts.map((alert) => (
              <div className="run-alert-row" key={alert.id} aria-label={`run report alert ${alert.id}`}>
                <div>
                  <strong>{alert.message}</strong>
                  <small>{alert.createdAt || 'no timestamp'}</small>
                </div>
                <span>{alert.severity}</span>
                <span>{alert.kind}</span>
                <span>{alert.metric}</span>
                <span>{`observed ${formatAlertMetricValue(alert.metric, alert.observed)}`}</span>
                <span>{`threshold ${formatAlertMetricValue(alert.metric, alert.threshold)}`}</span>
                {alert.eventId ? <span>{`event ${alert.eventId}`}</span> : null}
              </div>
            ))}
          </div>
        </article>
      ) : null}
      {runReport && targetMetrics ? (
        <article className="run-window-metrics-panel" aria-label={`run window target metrics ${runReport.run.id}`}>
          <div className="panel-heading compact">
            <div>
              <Typography.Text className="section-title">压测窗口指标 / Run window metrics</Typography.Text>
              <p>{targetMetrics.targetName || targetMetrics.targetId}</p>
            </div>
            <Tag color={targetMetrics.sampleCount > 0 ? 'processing' : 'default'}>{`samples ${targetMetrics.sampleCount}`}</Tag>
          </div>
          <div className="run-window-metrics-grid">
            <span>{`CPU max ${formatPercent(targetMetrics.cpuMaxPercent)}`}</span>
            <span>{`MEM max ${formatPercent(targetMetrics.memoryMaxPercent)}`}</span>
            <span>{`Disk read max ${formatByteRate(targetMetrics.diskReadMaxBytesPerSec)}`}</span>
            <span>{`Disk write max ${formatByteRate(targetMetrics.diskWriteMaxBytesPerSec)}`}</span>
            <span>{`Net RX max ${formatByteRate(targetMetrics.networkRxMaxBytesPerSec)}`}</span>
            <span>{`Net TX max ${formatByteRate(targetMetrics.networkTxMaxBytesPerSec)}`}</span>
            <span>{`agents ${targetMetrics.agentIds.length}`}</span>
            <span>{`${targetMetrics.from} - ${targetMetrics.to}`}</span>
          </div>
          <div className="run-window-agent-list" aria-label={`run window agents ${runReport.run.id}`}>
            {targetProcessLabel ? <span>{targetProcessLabel}</span> : null}
            {targetMetrics.agentIds.length > 0 ? (
              targetMetrics.agentIds.map((agentID) => <span key={agentID}>{agentID}</span>)
            ) : (
              <span>暂无绑定 Agent / No bound agent</span>
            )}
          </div>
          {targetMetrics.samples && targetMetrics.samples.length > 0 ? (
            <div className="run-window-trend-grid">
              <MetricTrendChart
                ariaLabel="run window cpu trend"
                label="CPU trend"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.cpuUsagePercent}
              />
              <MetricTrendChart
                ariaLabel="run window memory trend"
                label="MEM trend"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.memoryUsagePercent}
              />
              <MetricTrendChart
                ariaLabel="run window disk read trend"
                label="Disk read"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.diskReadBytesPerSec}
              />
              <MetricTrendChart
                ariaLabel="run window disk write trend"
                label="Disk write"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.diskWriteBytesPerSec}
              />
              <MetricTrendChart
                ariaLabel="run window network rx trend"
                label="Net RX"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.networkRxBytesPerSec}
              />
              <MetricTrendChart
                ariaLabel="run window network tx trend"
                label="Net TX"
                samples={targetMetrics.samples}
                selector={(metrics) => metrics.networkTxBytesPerSec}
              />
            </div>
          ) : null}
          {targetProcessTrends.length > 0 ? (
            <div className="process-trend-panel" aria-label={`process trends ${runReport.run.id}`}>
              <Typography.Text className="section-title">进程趋势 / Process trends</Typography.Text>
              <div className="process-trend-list">
                {targetProcessTrends.map((trend) => (
                  <div className="process-trend-row" key={`${trend.agentId}-${trend.pid}-${trend.name}`} aria-label={`process trend ${trend.agentId}-${trend.pid}`}>
                    <div>
                      <strong>{`${trend.name} pid ${trend.pid}`}</strong>
                      <small>{trend.cmdline || trend.agentId}</small>
                    </div>
                    <span>{`process samples ${trend.sampleCount}`}</span>
                    <span>{`CPU max ${formatPercent(trend.cpuMaxPercent)}`}</span>
                    <span>{`RSS max ${formatBytes(trend.memoryRssMaxBytes)}`}</span>
                    <span>{`fd max ${trend.fdMaxCount ?? 0}`}</span>
                    <span>{`threads max ${trend.threadMaxCount ?? 0}`}</span>
                    <small>{`${trend.firstSeenAt || '-'} - ${trend.lastSeenAt || '-'}`}</small>
                  </div>
                ))}
              </div>
            </div>
          ) : null}
          <div className="agent-process-list">
            {targetProcessSnapshot.length > 0 ? (
              targetProcessSnapshot.map((process) => (
                <div className="agent-process-row" key={`${process.pid}-${process.name}`}>
                  <strong>{`${process.name} ${formatPercent(process.cpuUsagePercent)}`}</strong>
                  {process.cmdline ? <span>{process.cmdline}</span> : null}
                  <span>{`RSS ${formatBytes(process.memoryRssBytes)}`}</span>
                  <span>{`fd ${process.fdCount}`}</span>
                  <span>{`threads ${process.threadCount}`}</span>
                </div>
              ))
            ) : (
              <p>暂无进程指标 / No process metrics yet</p>
            )}
          </div>
        </article>
      ) : null}
      {runReport && targetHealthChecks.length > 0 ? (
        <article className="run-target-health-panel" aria-label={`run target health checks ${runReport.run.id}`}>
          <div className="panel-heading compact">
            <div>
              <Typography.Text className="section-title">目标健康 / Target health</Typography.Text>
              <p>{runReport.run.targetName || runReport.run.targetId || 'No target'}</p>
            </div>
            <Tag color={targetHealthChecks.some((check) => check.status === 'unhealthy') ? 'error' : 'success'}>
              {`${targetHealthChecks.length} checks`}
            </Tag>
          </div>
          <div className="run-target-health-list">
            {targetHealthChecks.map((check, index) => (
              <div
                className="run-target-health-row"
                key={`${check.targetId || runReport.run.targetId}-${check.checkedAt || index}`}
                aria-label={`run target health check ${check.targetId || runReport.run.targetId}-${check.checkedAt || index}`}
              >
                <div>
                  <strong className={targetHealthCheckResultClass(check.status)}>{targetHealthCheckResultLabel(check)}</strong>
                  <small>{check.checkedAt || 'no timestamp'}</small>
                </div>
                <span>{`expected ${check.expectedStatus || '-'}`}</span>
                <span>{formatLatencyMetric(check.latencyMs)}</span>
                <span>{check.url || '-'}</span>
                {check.error ? <small>{check.error}</small> : null}
              </div>
            ))}
          </div>
        </article>
      ) : null}
      {runReport && hasRequestSamples ? (
        <article className="request-samples-panel" aria-label={`request samples ${runReport.run.id}`}>
          <div className="panel-heading compact">
            <Typography.Text className="section-title">请求样本 / Request samples</Typography.Text>
            <Tag color={errorSamples.length > 0 ? 'error' : 'processing'}>{`${slowSamples.length + errorSamples.length} samples`}</Tag>
          </div>
          <div className="request-sample-columns">
            {renderRequestSampleList('慢请求样本 / Slow samples', '暂无慢请求样本 / No slow samples yet', 'slow sample', slowSamples)}
            {renderRequestSampleList('错误样本 / Error samples', '暂无错误样本 / No error samples yet', 'error sample', errorSamples)}
          </div>
        </article>
      ) : null}
      {runComparison && runComparison.runs.length > 0 ? (
        <article className="run-comparison-panel" aria-label="run comparison report">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">多 Run 对比 / Run comparison</Typography.Text>
            <Tag color="processing">{`${runComparison.summary.runCount} runs`}</Tag>
          </div>
          {renderRunComparisonSummary(runComparison)}
          <div className="run-comparison-list">
            {runComparison.runs.map((run) => (
              <div className="run-comparison-row" key={run.id} aria-label={`run comparison ${run.id}`}>
                <div>
                  <strong>{runComparisonName(run)}</strong>
                  <small>{run.id}</small>
                </div>
                <span>{`success ${formatPercent(run.successRatePercent)}`}</span>
                <span>{`errors ${formatPercent(run.errorRatePercent)}`}</span>
                <span>{`qps ${formatMetricNumber(run.qps)}`}</span>
                <span>{`p95 ${formatLatencyMetric(run.p95LatencyMs)}`}</span>
                <span>{`target ${run.targetName || '-'}`}</span>
              </div>
            ))}
          </div>
        </article>
      ) : null}
      {processTrendComparison && processTrendComparison.groups.length > 0 ? (
        <article className="process-trend-comparison-panel" aria-label="process trend comparison report">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">跨 Run 进程趋势 / Process trend comparison</Typography.Text>
            <Tag color="processing">{`${processTrendComparison.summary.processCount} processes`}</Tag>
          </div>
          {renderProcessTrendComparisonSummary(processTrendComparison)}
          <div className="process-trend-comparison-list">
            {processTrendComparison.groups.map((group) => (
              <div className="process-trend-comparison-row" key={group.key} aria-label={`process trend comparison ${group.key}`}>
                <div>
                  <strong>{group.name}</strong>
                  <small>{group.cmdline || group.agentId}</small>
                </div>
                <span>{`latest ${group.latestRunId || '-'} pid ${group.latestPid || '-'}`}</span>
                <span>{`previous ${group.previousRunId || '-'} pid ${group.previousPid ?? '-'}`}</span>
                <span>{`CPU max ${formatPercent(group.cpuMaxPercent)}`}</span>
                <span>{`CPU delta ${formatSignedPercent(group.cpuDeltaPercent)}`}</span>
                <span>{`RSS max ${formatBytes(group.memoryRssMaxBytes)}`}</span>
                <span>{`RSS delta ${formatSignedBytes(group.memoryRssDeltaBytes)}`}</span>
                <span>{`samples ${group.sampleCount}`}</span>
              </div>
            ))}
          </div>
        </article>
      ) : null}
      {profileArtifactComparison && profileArtifactComparison.groups.length > 0 ? (
        <article className="profile-artifact-comparison-panel" aria-label="profile artifact comparison report">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">画像对比 / Profile artifact comparison</Typography.Text>
            <Tag color="processing">{`${profileArtifactComparison.summary.runCount} runs`}</Tag>
          </div>
          <div className="profile-artifact-summary" aria-label="profile artifact comparison summary">
            <span>{`artifacts ${profileArtifactComparison.summary.artifactCount}`}</span>
            <span>{`collected ${profileArtifactComparison.summary.collectedCount}`}</span>
            <span>{`failed ${profileArtifactComparison.summary.failedCount}`}</span>
            <span>{`size ${formatBytes(profileArtifactComparison.summary.totalSizeBytes)}`}</span>
          </div>
          <div className="profile-artifact-comparison-list">
            {profileArtifactComparison.groups.map((group) => (
              <div className="profile-artifact-comparison-row" key={group.profileType} aria-label={`profile artifact comparison ${group.profileType}`}>
                <div>
                  <strong>{group.profileType}</strong>
                  <small>{`latest ${group.latestRunId || '-'}`}</small>
                </div>
                <span>{`${group.artifactCount} artifacts`}</span>
                <span>{`collected ${group.collectedCount}`}</span>
                <span>{`failed ${group.failedCount}`}</span>
                <span>{`size ${formatBytes(group.totalSizeBytes)}`}</span>
                <span>{`delta ${formatSignedBytes(group.sizeDeltaBytes)}`}</span>
                <span>{`change ${formatSignedPercent(group.sizeDeltaPercent)}`}</span>
                <div>
                  <strong>{group.artifacts[0]?.fileName || group.latestArtifactId || '-'}</strong>
                  <small>{group.artifacts[0]?.error || group.latestArtifactId || 'no artifact'}</small>
                  <small>{formatProfileArtifactStatus(group)}</small>
                </div>
              </div>
            ))}
          </div>
        </article>
      ) : null}
      {profileTasks.length > 0 ? (
        <article className="profile-task-panel" aria-label="profile task queue">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">画像任务 / Profile tasks</Typography.Text>
            <Tag color="processing">{`${profileTasks.length} tasks`}</Tag>
            <select
              aria-label="Profile task source filter"
              className="profile-task-source-filter"
              value={profileTaskSourceFilter}
              onChange={handleProfileTaskSourceFilterChange}
            >
              <option value="">All sources</option>
              <option value="api">api</option>
              <option value="run_auto">run_auto</option>
              <option value="target_manual">target_manual</option>
              <option value="threshold_auto">threshold_auto</option>
            </select>
          </div>
          <div className="profile-task-list">
            {profileTasks.map((task) => (
              <div className="profile-task-row" key={task.id} aria-label={`profile task ${task.id}`}>
                <div>
                  <strong>{task.profileType || 'profile'}</strong>
                  <small>{task.runId || task.targetName || task.id}</small>
                </div>
                <Tag color={profileTaskStatusColor(task.status)}>{task.status}</Tag>
                <span>{`attempts ${task.attempts ?? 0}/${task.maxAttempts ?? 3}`}</span>
                <span>{task.agentId || '-'}</span>
                <span>{`${task.profileSeconds || 0}s`}</span>
                <span>{`source ${task.source || 'api'}`}</span>
                <small>
                  {task.status === 'leased' && task.leaseExpiresAt
                    ? `lease expires ${task.leaseExpiresAt}`
                    : task.updatedAt || task.createdAt || 'no timestamp'}
                </small>
                <div className="profile-task-detail">
                  <small>{profileTaskDetailLabel(task)}</small>
                  {task.status === 'failed' ? (
                    <Button
                      size="small"
                      aria-label={`Retry profile task ${task.id}`}
                      loading={retryingProfileTaskID === task.id}
                      onClick={() => retryProfileTask(task.id)}
                    >
                      重试 / Retry
                    </Button>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        </article>
      ) : null}
      <article className="report-score-panel">
        <div className="panel-heading compact">
          <Typography.Text className="section-title">瓶颈雷达 / Bottleneck radar</Typography.Text>
          <Tag color="warning">Review needed</Tag>
        </div>
        <div className="radar-grid">
          {['CPU bound', 'heap growth', 'slow endpoint', 'IO stable'].map((item) => (
            <span key={item}>{item}</span>
          ))}
        </div>
      </article>

      <article
        className="agent-metrics-panel"
        aria-label={selectedReportAgent ? `agent metrics timeline ${selectedReportAgent.id}` : 'agent metrics timeline empty'}
      >
        <div className="panel-heading compact">
          <Typography.Text className="section-title">目标指标时间线 / Target metric timeline</Typography.Text>
          <Tag color={agentMetricsTimeline.length > 0 ? 'processing' : 'default'}>{`${agentMetricsTimeline.length} samples`}</Tag>
        </div>
        {selectedReportAgent && agentMetricsTimeline.length > 0 ? (
          <>
            <div className="agent-metrics-summary">
              <strong>{selectedReportAgent.name}</strong>
              <span>{`CPU max ${formatPercent(cpuMax)}`}</span>
              <span>{`MEM max ${formatPercent(memoryMax)}`}</span>
              <small>{latestMetricsSample?.collectedAt || 'no timestamp'}</small>
            </div>
            <div className="trend-grid">
              <MetricTrendChart
                ariaLabel="agent cpu trend"
                label="CPU"
                samples={agentMetricsTimeline}
                selector={(metrics) => metrics.cpuUsagePercent}
              />
              <MetricTrendChart
                ariaLabel="agent memory trend"
                label="MEM"
                samples={agentMetricsTimeline}
                selector={(metrics) => metrics.memoryUsagePercent}
              />
            </div>
            <div className="agent-process-list">
              {latestTimelineProcess ? (
                <div className="agent-process-row">
                  <strong>{`${latestTimelineProcess.name} ${formatPercent(latestTimelineProcess.cpuUsagePercent)}`}</strong>
                  <span>{`RSS ${formatBytes(latestTimelineProcess.memoryRssBytes)}`}</span>
                  <span>{`fd ${latestTimelineProcess.fdCount}`}</span>
                  <span>{`threads ${latestTimelineProcess.threadCount}`}</span>
                </div>
              ) : (
                <p>暂无进程指标 / No process metrics yet</p>
              )}
            </div>
          </>
        ) : (
          <p>暂无 Agent 指标时间线 / No agent metric timeline yet</p>
        )}
      </article>

      <article className="artifact-panel">
        <Typography.Text className="section-title">画像产物 / Profile artifacts</Typography.Text>
        <div className="artifact-list">
          {profileArtifacts.length === 0
            ? ['cpu.pb.gz', 'heap.pb.gz', 'goroutine.txt', 'flamegraph.svg'].map((artifact) => (
                <span key={artifact}>{artifact}</span>
              ))
            : profileArtifacts.map((artifact) => (
                <div className="artifact-row" key={artifact.id} aria-label={`profile artifact ${artifact.id}`}>
                  <div>
                    <strong>{artifact.fileName || artifact.id}</strong>
                    <small>{artifact.scenarioName || artifact.runId}</small>
                  </div>
                  <span>
                    <small>target</small>
                    <strong>{artifact.targetName || '-'}</strong>
                  </span>
                  <span>
                    <small>type</small>
                    <strong>{artifact.profileType}</strong>
                  </span>
                  <span>
                    <small>size</small>
                    <strong>{formatBytes(artifact.sizeBytes)}</strong>
                  </span>
                  <div className="artifact-actions" aria-label={`profile artifact actions ${artifact.id}`}>
                    <Tag color={artifact.status === 'collected' ? 'success' : 'warning'}>{artifact.status}</Tag>
                    {artifact.status === 'collected' ? (
                      <a
                        className="artifact-download"
                        href={profileArtifactDownloadURL(artifact.id, workspaceScope)}
                        aria-label={`Download profile artifact ${artifact.fileName || artifact.id}`}
                      >
                        <DownloadOutlined />
                        <span>Download</span>
                      </a>
                    ) : (
                      <span className="artifact-download-disabled">No file</span>
                    )}
                  </div>
                </div>
              ))}
        </div>
      </article>

      <article className="summary-panel">
        <Typography.Text className="section-title">瓶颈摘要 / Bottleneck summary</Typography.Text>
        <p>checkout createOrder 在升压后出现 CPU 热点，建议复核锁竞争和序列化路径。</p>
        <p>createOrder becomes CPU-heavy after ramp-up. Review lock contention and serialization path.</p>
      </article>

      <article className="report-table-panel">
        <div className="run-table">
          {page.modules.map((module) => (
            <div className="run-row" key={module.title.en}>
              <strong>{module.meta}</strong>
              <span>{bilingual(module.status)}</span>
              <div className="run-row-status">
                <Tag color="default">{bilingual(module.status)}</Tag>
              </div>
            </div>
          ))}
        </div>
      </article>
    </section>
  );
}
