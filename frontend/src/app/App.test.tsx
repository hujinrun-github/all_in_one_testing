import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, vi } from 'vitest';
import App from './App';

const scenariosApiURL = 'http://127.0.0.1:8080/api/scenarios';
const runsApiURL = 'http://127.0.0.1:8080/api/runs';

function createScenarioFetchMock(initialScenarios: unknown[] = []) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method || 'GET').toUpperCase();

    if (url === scenariosApiURL && method === 'GET') {
      return {
        ok: true,
        json: async () => initialScenarios,
      };
    }

    if (url === scenariosApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      return {
        ok: true,
        json: async () => ({ ...body, id: 'server-scenario-1' }),
      };
    }

    if (url === runsApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      return {
        ok: true,
        json: async () => ({
          id: 'run-from-scenario-1',
          scenarioId: body.scenarioId,
          scenarioName: 'backend-checkout-smoke',
          name: 'backend-checkout-smoke',
          status: 'finished',
          method: 'GET',
          url: 'http://127.0.0.1:8080/api/health',
          totalRequests: body.totalRequests,
          successRequests: body.totalRequests,
          failedRequests: 0,
          durationMs: 12.5,
          qps: 480,
          averageLatencyMs: 2.1,
          p95LatencyMs: 3.4,
        }),
      };
    }

    throw new Error(`Unexpected fetch ${method} ${url}`);
  });
}

describe('App', () => {
  beforeEach(() => {
    window.location.hash = '';
    window.localStorage.clear();
    vi.stubGlobal('fetch', createScenarioFetchMock());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the project workspace navigation', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: '一体化压测平台 / All-in-One Testing' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: '项目工作区 / Project workspace' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '仪表盘 / Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '目标与 Agent / Targets & Agents' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '场景 / Scenarios' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '压测运行 / Runs' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '报告 / Reports' })).toBeInTheDocument();
  });

  it('shows dashboard overview content on the first screen', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: '仪表盘 / Dashboard' })).toBeInTheDocument();
    expect(screen.getByText('运行态势 / Run status')).toBeInTheDocument();
    expect(screen.getByText('实时压测 / Live run')).toBeInTheDocument();
    expect(screen.getByText('Agent 健康 / Agent health')).toBeInTheDocument();
    expect(screen.getByText('画像观察 / Profile watch')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '启动压测 / Start run' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '停止 / Stop' })).toBeInTheDocument();
  });

  it('switches workspace content when sidebar links are clicked', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('link', { name: '目标与 Agent / Targets & Agents' }));
    expect(screen.getByRole('heading', { name: '目标与 Agent / Targets & Agents' })).toBeInTheDocument();
    expect(screen.getByText('Agent 接入 / Agent onboarding')).toBeInTheDocument();
    expect(screen.getByText('主机状态采集 / Host status collection')).toBeInTheDocument();
    expect(window.location.hash).toBe('#targets');

    await user.click(screen.getByRole('link', { name: '报告 / Reports' }));
    expect(screen.getByRole('heading', { name: '报告 / Reports' })).toBeInTheDocument();
    expect(screen.getByText('画像产物 / Profile artifacts')).toBeInTheDocument();
    expect(screen.getByText('瓶颈摘要 / Bottleneck summary')).toBeInTheDocument();
    expect(window.location.hash).toBe('#reports');
  });

  it('renders distinct layouts for scenarios and runs pages', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('link', { name: '场景 / Scenarios' }));
    expect(screen.getByText('调用链编排 / Request flow builder')).toBeInTheDocument();
    expect(screen.getByText('协议适配器 / Protocol adapters')).toBeInTheDocument();
    expect(screen.queryByText('运行控制台 / Run control')).not.toBeInTheDocument();

    await user.click(screen.getByRole('link', { name: '压测运行 / Runs' }));
    expect(screen.getByText('运行控制台 / Run control')).toBeInTheDocument();
    expect(screen.getByText('阶段时间线 / Stage timeline')).toBeInTheDocument();
    expect(screen.queryByText('调用链编排 / Request flow builder')).not.toBeInTheDocument();
  });

  it('creates a scenario from the topbar action', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: /New scenario/ }));

    expect(window.location.hash).toBe('#scenarios');
    expect(screen.getByRole('dialog', { name: /Create scenario/ })).toBeInTheDocument();
    expect(screen.getByLabelText(/Protocol/)).toHaveAttribute('role', 'combobox');
    expect(screen.getByLabelText(/Method/)).toHaveAttribute('role', 'combobox');
    fireEvent.mouseDown(screen.getByLabelText(/Protocol/));
    expect(screen.getByRole('option', { name: 'gRPC' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    fireEvent.mouseDown(screen.getByLabelText(/Method/));
    expect(screen.getByRole('option', { name: 'POST' })).toBeInTheDocument();
    await user.keyboard('{Escape}');

    await user.type(screen.getByLabelText(/Scenario name/), 'payment-http-smoke');
    await user.clear(screen.getByLabelText(/Base URL/));
    await user.type(screen.getByLabelText(/Base URL/), 'https://api.example.test');
    await user.clear(screen.getByLabelText(/Request path/));
    await user.type(screen.getByLabelText(/Request path/), '/api/payments');
    expect(screen.getByLabelText('Query params variant 1')).toBeInTheDocument();
    expect(screen.getByLabelText('Query weight 1')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Query params variant 1'), {
      target: { value: 'tenant=shanghai&debug=true' },
    });
    fireEvent.change(screen.getByLabelText('Query weight 1'), {
      target: { value: '80' },
    });
    await user.click(screen.getByRole('button', { name: /Add query variant/ }));
    fireEvent.change(screen.getByLabelText('Query params variant 2'), {
      target: { value: 'tenant=beijing&debug=false' },
    });
    fireEvent.change(screen.getByLabelText('Query weight 2'), {
      target: { value: '20' },
    });
    expect(screen.getByLabelText('Header key 1')).toBeInTheDocument();
    expect(screen.getByLabelText('Header value 1')).toBeInTheDocument();
    await user.clear(screen.getByLabelText('Header key 1'));
    await user.type(screen.getByLabelText('Header key 1'), 'Authorization');
    await user.clear(screen.getByLabelText('Header value 1'));
    await user.type(screen.getByLabelText('Header value 1'), 'Bearer ${token}');
    await user.click(screen.getByRole('button', { name: /Add header/ }));
    await user.type(screen.getByLabelText('Header key 2'), 'Content-Type');
    await user.type(screen.getByLabelText('Header value 2'), 'application/json');
    expect(screen.getByLabelText('Body JSON variant 1')).toBeInTheDocument();
    expect(screen.getByLabelText('Body weight 1')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Body JSON variant 1'), {
      target: { value: '{ "amount": 100, "currency": "CNY" }' },
    });
    await user.clear(screen.getByLabelText('Body weight 1'));
    await user.type(screen.getByLabelText('Body weight 1'), '70');
    await user.click(screen.getByRole('button', { name: /Add body variant/ }));
    fireEvent.change(screen.getByLabelText('Body JSON variant 2'), {
      target: { value: '{ "amount": 300, "currency": "CNY" }' },
    });
    await user.clear(screen.getByLabelText('Body weight 2'));
    await user.type(screen.getByLabelText('Body weight 2'), '30');
    await user.clear(screen.getByLabelText(/Timeout/));
    await user.type(screen.getByLabelText(/Timeout/), '2500');
    await user.clear(screen.getByLabelText(/Retry count/));
    await user.type(screen.getByLabelText(/Retry count/), '2');
    await user.click(screen.getByRole('button', { name: /Create/ }));

    expect(screen.queryByRole('dialog', { name: /Create scenario/ })).not.toBeInTheDocument();
    const createdScenario = screen.getByLabelText('scenario payment-http-smoke');
    expect(within(createdScenario).getByText('payment-http-smoke')).toBeInTheDocument();
    expect(within(createdScenario).getByText('HTTP')).toBeInTheDocument();
    expect(within(createdScenario).getByText('GET https://api.example.test/api/payments')).toBeInTheDocument();
    expect(within(createdScenario).getByText('tenant=shanghai&debug=true')).toBeInTheDocument();
    expect(within(createdScenario).getByText('query weight 80')).toBeInTheDocument();
    expect(within(createdScenario).getByText('tenant=beijing&debug=false')).toBeInTheDocument();
    expect(within(createdScenario).getByText('query weight 20')).toBeInTheDocument();
    expect(within(createdScenario).getByText('timeout 2500 ms')).toBeInTheDocument();
    expect(within(createdScenario).getByText('retry 2')).toBeInTheDocument();
    expect(within(createdScenario).getByText(/Authorization: Bearer/)).toBeInTheDocument();
    expect(within(createdScenario).getByText(/Content-Type: application\/json/)).toBeInTheDocument();
    expect(within(createdScenario).getByText('{ "amount": 100, "currency": "CNY" }')).toBeInTheDocument();
    expect(within(createdScenario).getByText('body weight 70')).toBeInTheDocument();
    expect(within(createdScenario).getByText('{ "amount": 300, "currency": "CNY" }')).toBeInTheDocument();
    expect(within(createdScenario).getByText('body weight 30')).toBeInTheDocument();
  });

  it('persists created scenarios across page reloads', async () => {
    const user = userEvent.setup();
    const { unmount } = render(<App />);

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'persisted-health-smoke');
    await user.click(screen.getByRole('button', { name: /Create/ }));

    expect(screen.getByLabelText('scenario persisted-health-smoke')).toBeInTheDocument();

    unmount();
    render(<App />);

    expect(screen.getByLabelText('scenario persisted-health-smoke')).toBeInTheDocument();
  });

  it('loads scenarios from the backend API', async () => {
    window.location.hash = '#scenarios';
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-from-api',
        name: 'backend-checkout-smoke',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://127.0.0.1:8080',
        path: '/api/health',
        queryVariants: [],
        headers: [],
        bodyVariants: [],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('scenario backend-checkout-smoke')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(scenariosApiURL);
  });

  it('creates scenarios through the backend API', async () => {
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'server-created-smoke');
    await user.click(screen.getByRole('button', { name: /Create/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        scenariosApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });

    const createCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === scenariosApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(createCall).toBeDefined();
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'server-created-smoke',
        protocol: 'HTTP',
      }),
    );
    expect(await screen.findByLabelText('scenario server-created-smoke')).toBeInTheDocument();
  });

  it('starts a load run from a saved scenario', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-from-api',
        name: 'backend-checkout-smoke',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://127.0.0.1:8080',
        path: '/api/health',
        queryVariants: [],
        headers: [],
        bodyVariants: [],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    await user.clear(screen.getByLabelText('Total requests'));
    await user.type(screen.getByLabelText('Total requests'), '6');
    await user.clear(screen.getByLabelText('Concurrency'));
    await user.type(screen.getByLabelText('Concurrency'), '2');
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        runsApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });

    const runCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === runsApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(runCall).toBeDefined();
    expect(JSON.parse(String(runCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        scenarioId: 'scenario-from-api',
        totalRequests: 6,
        concurrency: 2,
      }),
    );
    expect(await screen.findByText('run-from-scenario-1')).toBeInTheDocument();
    expect(screen.getByText('finished')).toBeInTheDocument();
    expect(screen.getByText('6 / 6')).toBeInTheDocument();
  });
});
