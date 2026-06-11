import {
  DashboardOutlined,
  FileTextOutlined,
  NodeIndexOutlined,
  PlayCircleOutlined,
  ProfileOutlined,
} from '@ant-design/icons';
import { Badge, Button, ConfigProvider, Form, Input, Layout, Menu, Modal, Select, Tag, Typography } from 'antd';
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

type ScenarioDraft = {
  id: string;
  name: string;
  protocol: string;
  method: string;
  baseUrl: string;
  path: string;
  queryVariants: QueryVariant[];
  headers: HeaderPair[];
  bodyVariants: BodyVariant[];
  timeoutMs: string;
  retryCount: string;
  assertion: string;
};

type ScenarioFormValues = Omit<ScenarioDraft, 'id'>;

type CreateRunPayload = {
  scenarioId: string;
  totalRequests: number;
  concurrency: number;
  timeoutMs: number;
};

type RunResult = {
  id: string;
  scenarioId?: string;
  scenarioName?: string;
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
};

const defaultScenarioValues: ScenarioFormValues = {
  name: '',
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

const scenarioDraftsStorageKey = 'all-in-one-testing.scenario-drafts.v1';
const scenarioApiBaseURL = (import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');

const protocolOptions = [
  { label: 'HTTP', value: 'HTTP' },
  { label: 'gRPC', value: 'GRPC' },
  { label: 'Dubbo', value: 'DUBBO' },
  { label: 'Thrift', value: 'THRIFT' },
  { label: 'Custom RPC', value: 'CUSTOM_RPC' },
];

const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((method) => ({
  label: method,
  value: method,
}));

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

const agentRows = [
  { name: 'api-gateway-01', zone: 'shanghai-a', cpu: '42%', mem: '61%', io: '18 MB/s', state: { zh: '健康', en: 'Healthy' } },
  { name: 'checkout-02', zone: 'shanghai-b', cpu: '78%', mem: '69%', io: '31 MB/s', state: { zh: '观察', en: 'Watching' } },
  { name: 'search-03', zone: 'shanghai-c', cpu: '35%', mem: '54%', io: '11 MB/s', state: { zh: '健康', en: 'Healthy' } },
];

const profileRows = [
  { label: { zh: 'CPU pprof', en: 'CPU pprof' }, value: { zh: '采样中 60s', en: 'Sampling 60s' } },
  { label: { zh: 'Heap prof', en: 'Heap prof' }, value: { zh: '下次 14:35', en: 'Next 14:35' } },
  { label: { zh: '火焰图', en: 'Flamegraph' }, value: { zh: '等待聚合', en: 'Aggregation queued' } },
];

const recentRuns = [
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
      .map((draft) => ({
        ...draft,
        queryVariants: Array.isArray(draft.queryVariants) ? draft.queryVariants : [],
        headers: Array.isArray(draft.headers) ? draft.headers : [],
        bodyVariants: Array.isArray(draft.bodyVariants) ? draft.bodyVariants : [],
      }));
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

async function loadBackendScenarioDrafts(): Promise<ScenarioDraft[]> {
  const response = await fetch(`${scenarioApiBaseURL}/api/scenarios`);
  if (!response.ok) {
    throw new Error(`Failed to load scenarios with HTTP ${response.status}`);
  }
  return response.json() as Promise<ScenarioDraft[]>;
}

async function createBackendScenarioDraft(scenario: ScenarioDraft): Promise<ScenarioDraft> {
  const response = await fetch(`${scenarioApiBaseURL}/api/scenarios`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(scenario),
  });
  if (!response.ok) {
    throw new Error(`Failed to create scenario with HTTP ${response.status}`);
  }
  return response.json() as Promise<ScenarioDraft>;
}

async function createBackendRun(payload: CreateRunPayload): Promise<RunResult> {
  const response = await fetch(`${scenarioApiBaseURL}/api/runs`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(`Failed to create run with HTTP ${response.status}`);
  }
  return response.json() as Promise<RunResult>;
}

function createDefaultScenarioValues(): ScenarioFormValues {
  return {
    ...defaultScenarioValues,
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

export default function App() {
  const [activePage, setActivePage] = useState<PageKey>(() => pageFromHash(window.location.hash));
  const [isCreateScenarioOpen, setIsCreateScenarioOpen] = useState(false);
  const [scenarioDrafts, setScenarioDrafts] = useState<ScenarioDraft[]>(loadStoredScenarioDrafts);
  const [scenarioForm] = Form.useForm<ScenarioFormValues>();
  const currentPage = workspacePages.find((page) => page.key === activePage) ?? workspacePages[0];

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

    loadBackendScenarioDrafts()
      .then((backendDrafts) => {
        if (!isMounted) {
          return;
        }
        setScenarioDrafts((currentDrafts) => mergeScenarioDrafts(backendDrafts, currentDrafts));
      })
      .catch(() => {
        // Keep the local cache as a fallback when the backend is not reachable.
      });

    return () => {
      isMounted = false;
    };
  }, []);

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

  const openCreateScenario = () => {
    navigateToPage('scenarios');
    scenarioForm.setFieldsValue(createDefaultScenarioValues());
    setIsCreateScenarioOpen(true);
  };

  const closeCreateScenario = () => {
    setIsCreateScenarioOpen(false);
    scenarioForm.resetFields();
  };

  const handleCreateScenario = async (values: ScenarioFormValues) => {
    const nextScenario: ScenarioDraft = {
      id: `scenario-${Date.now()}`,
      name: values.name.trim(),
      protocol: (values.protocol || 'HTTP').trim().toUpperCase(),
      method: (values.method || 'GET').trim().toUpperCase(),
      baseUrl: values.baseUrl.trim(),
      path: normalizePath(values.path),
      queryVariants: normalizeQueryVariants(values.queryVariants),
      headers: normalizeHeaders(values.headers),
      bodyVariants: normalizeBodyVariants(values.bodyVariants),
      timeoutMs: (values.timeoutMs || defaultScenarioValues.timeoutMs).trim(),
      retryCount: (values.retryCount || defaultScenarioValues.retryCount).trim(),
      assertion: (values.assertion || 'status < 400').trim(),
    };

    try {
      const createdScenario = await createBackendScenarioDraft(nextScenario);
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
                <span>8 agents</span>
                <span>3 targets</span>
              </div>
            </section>

            <ActiveWorkspacePage page={currentPage} scenarioDrafts={scenarioDrafts} />
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

          <Form.Item
            label="基础地址 / Base URL"
            name="baseUrl"
            rules={[{ required: true, whitespace: true, message: 'Please enter a base URL' }]}
          >
            <Input placeholder="http://127.0.0.1:8080" />
          </Form.Item>

          <div className="scenario-form-grid">
            <Form.Item label="协议 / Protocol" name="protocol">
              <Select options={protocolOptions} />
            </Form.Item>
            <Form.Item label="方法 / Method" name="method">
              <Select options={methodOptions} />
            </Form.Item>
          </div>

          <Form.Item
            label="请求路径 / Request path"
            name="path"
            rules={[{ required: true, whitespace: true, message: 'Please enter a request path' }]}
          >
            <Input placeholder="/api/health" />
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

function ActiveWorkspacePage({ page, scenarioDrafts }: { page: WorkspacePage; scenarioDrafts: ScenarioDraft[] }) {
  switch (page.key) {
    case 'dashboard':
      return <DashboardPage />;
    case 'targets':
      return <TargetsPage page={page} />;
    case 'scenarios':
      return <ScenariosPage page={page} scenarioDrafts={scenarioDrafts} />;
    case 'runs':
      return <RunsPage page={page} scenarioDrafts={scenarioDrafts} />;
    case 'reports':
      return <ReportsPage page={page} />;
    default:
      return null;
  }
}

function DashboardPage() {
  return (
    <>
      <section className="status-section" aria-label="运行态势 / Run status">
        <div className="section-heading">
          <Typography.Text className="section-title">运行态势 / Run status</Typography.Text>
          <Typography.Text className="section-subtitle">RUN-2407 · checkout-mixed-rpc</Typography.Text>
        </div>
        <div className="metric-grid">
          {statusMetrics.map((metric) => (
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
              <p>checkout-mixed-rpc · fixed QPS · 42m remaining</p>
            </div>
            <Tag color="processing">running</Tag>
          </div>
          <div className="stage-list">
            {runStages.map((stage) => (
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
            <Button type="primary">启动压测 / Start run</Button>
            <Button danger>停止 / Stop</Button>
            <Button>保存模板 / Save template</Button>
          </div>
        </article>

        <article className="agent-panel">
          <div className="panel-heading compact">
            <Typography.Text className="section-title">Agent 健康 / Agent health</Typography.Text>
            <Badge status="success" text="8 / 9" />
          </div>
          <div className="agent-list">
            {agentRows.map((agent) => (
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
                <Tag color={agent.state.en === 'Healthy' ? 'success' : 'warning'}>{bilingual(agent.state)}</Tag>
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
            <Button size="small">查看全部 / View all</Button>
          </div>
          <div className="run-table">
            {recentRuns.map((run) => (
              <div className="run-row" key={run.id}>
                <span>{run.id}</span>
                <strong>{run.scenario}</strong>
                <span>{run.latency}</span>
                <Tag color={run.result.en === 'Passed' ? 'success' : 'warning'}>{bilingual(run.result)}</Tag>
              </div>
            ))}
          </div>
        </article>
      </section>
    </>
  );
}

function TargetsPage({ page }: { page: WorkspacePage }) {
  const [onboarding, hostCollection, binding, profileEndpoints] = page.modules;

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
      </article>

      <article className="target-setup-panel">
        <Typography.Text className="section-title">{bilingual(onboarding.title)}</Typography.Text>
        <p>{bilingual(onboarding.description)}</p>
        <code>curl -fsSL agent/install.sh | sh</code>
      </article>

      <article className="target-collector-panel">
        <Typography.Text className="section-title">{bilingual(hostCollection.title)}</Typography.Text>
        <div className="collector-grid">
          {['CPU', 'MEM', 'DISK IO', 'NET', 'PROC'].map((item) => (
            <span key={item}>{item}</span>
          ))}
        </div>
      </article>

      <article className="target-binding-panel">
        <Typography.Text className="section-title">{bilingual(binding.title)}</Typography.Text>
        <p>{bilingual(profileEndpoints.description)}</p>
        <span className="module-meta">{profileEndpoints.meta}</span>
      </article>
    </section>
  );
}

function ScenariosPage({ page, scenarioDrafts }: { page: WorkspacePage; scenarioDrafts: ScenarioDraft[] }) {
  return (
    <section className="scenario-layout" aria-label={bilingual(page.label)}>
      <article className="flow-builder-panel">
        <div className="panel-heading compact">
          <Typography.Text className="section-title">调用链编排 / Request flow builder</Typography.Text>
          <Button size="small">新增步骤 / Add step</Button>
        </div>
        <div className="flow-lane">
          {[
            '01 HTTP login',
            '02 extract token',
            '03 RPC createOrder',
            '04 assert status',
          ].map((step) => (
            <div className="flow-step" key={step}>
              <span>{step}</span>
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

function RunsPage({ page, scenarioDrafts }: { page: WorkspacePage; scenarioDrafts: ScenarioDraft[] }) {
  const [selectedScenarioId, setSelectedScenarioId] = useState('');
  const [totalRequests, setTotalRequests] = useState('20');
  const [concurrency, setConcurrency] = useState('4');
  const [timeoutMs, setTimeoutMs] = useState('1000');
  const [runResult, setRunResult] = useState<RunResult | null>(null);
  const [runError, setRunError] = useState('');
  const [isRunning, setIsRunning] = useState(false);

  useEffect(() => {
    if (scenarioDrafts.length === 0) {
      setSelectedScenarioId('');
      return;
    }

    setSelectedScenarioId((currentScenarioId) => {
      if (currentScenarioId && scenarioDrafts.some((scenario) => scenario.id === currentScenarioId)) {
        return currentScenarioId;
      }
      return scenarioDrafts[0].id;
    });
  }, [scenarioDrafts]);

  const selectedScenario = scenarioDrafts.find((scenario) => scenario.id === selectedScenarioId);
  const scenarioOptions = scenarioDrafts.map((scenario) => ({
    label: scenario.name,
    value: scenario.id,
  }));

  const startRun = async () => {
    if (!selectedScenario) {
      setRunError('Please create or load a scenario before starting a run.');
      return;
    }

    setIsRunning(true);
    setRunError('');
    try {
      const result = await createBackendRun({
        scenarioId: selectedScenario.id,
        totalRequests: parseRunNumber(totalRequests, 1),
        concurrency: parseRunNumber(concurrency, 1),
        timeoutMs: parseRunNumber(timeoutMs, Number.parseInt(selectedScenario.timeoutMs, 10) || 1000),
      });
      setRunResult(result);
    } catch (error) {
      setRunError(error instanceof Error ? error.message : 'Failed to start run.');
    } finally {
      setIsRunning(false);
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
        </div>

        {selectedScenario ? (
          <div className="selected-run-scenario" aria-label={`selected run scenario ${selectedScenario.name}`}>
            <strong>{selectedScenario.name}</strong>
            <span>{scenarioRequestLine(selectedScenario)}</span>
            <small>{`${selectedScenario.method} · timeout ${selectedScenario.timeoutMs} ms · ${selectedScenario.assertion}`}</small>
          </div>
        ) : (
          <div className="selected-run-scenario empty">暂无可运行场景 / No runnable scenarios</div>
        )}

        <div className="control-actions">
          <Button type="primary" loading={isRunning} disabled={!selectedScenario} onClick={startRun}>
            启动压测 / Start run
          </Button>
          <Button danger>停止 / Stop</Button>
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

function ReportsPage({ page }: { page: WorkspacePage }) {
  return (
    <section className="reports-layout" aria-label={bilingual(page.label)}>
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

      <article className="artifact-panel">
        <Typography.Text className="section-title">画像产物 / Profile artifacts</Typography.Text>
        <div className="artifact-list">
          {['cpu.pb.gz', 'heap.pb.gz', 'goroutine.txt', 'flamegraph.svg'].map((artifact) => (
            <span key={artifact}>{artifact}</span>
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
              <Tag color="default">{bilingual(module.status)}</Tag>
            </div>
          ))}
        </div>
      </article>
    </section>
  );
}
