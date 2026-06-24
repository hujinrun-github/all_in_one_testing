import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, vi } from 'vitest';
import App from './App';

const scenariosApiURL = 'http://127.0.0.1:8080/api/scenarios';
const runsApiURL = 'http://127.0.0.1:8080/api/runs';
const agentsApiURL = 'http://127.0.0.1:8080/api/agents';
const agentMetricsApiURL = 'http://127.0.0.1:8080/api/agent-metrics';
const agentTokensApiURL = 'http://127.0.0.1:8080/api/agent-tokens';
const targetsApiURL = 'http://127.0.0.1:8080/api/targets';
const profileArtifactsApiURL = 'http://127.0.0.1:8080/api/profile-artifacts';
const profileTasksApiURL = 'http://127.0.0.1:8080/api/profile-tasks';
const profileTaskTemplatesApiURL = 'http://127.0.0.1:8080/api/profile-task-templates';
const runReportsApiURL = 'http://127.0.0.1:8080/api/reports/runs';
const runReportsCompareApiURL = 'http://127.0.0.1:8080/api/reports/compare';
const profileArtifactCompareApiURL = 'http://127.0.0.1:8080/api/reports/profile-artifacts/compare';
const processTrendCompareApiURL = 'http://127.0.0.1:8080/api/reports/process-trends/compare';

const defaultProfileTaskTemplates = [
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
    createdAt: '2026-06-12T09:00:00Z',
    updatedAt: '2026-06-12T09:00:00Z',
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
    createdAt: '2026-06-12T09:00:00Z',
    updatedAt: '2026-06-12T09:00:00Z',
  },
];

function filterRecordsByWorkspaceScope(records: unknown[], rawURL: string): unknown[] {
  const queryIndex = rawURL.indexOf('?');
  const searchParams = new URLSearchParams(queryIndex >= 0 ? rawURL.slice(queryIndex + 1) : '');
  const projectId = searchParams.get('projectId');
  const environment = searchParams.get('environment');

  return records.filter((record) => {
    const scopedRecord = record as { projectId?: string; environment?: string };
    if (projectId && scopedRecord.projectId !== projectId) {
      return false;
    }
    if (environment && scopedRecord.environment !== environment) {
      return false;
    }
    return true;
  });
}

function createScenarioFetchMock(
  initialScenarios: unknown[] = [],
  initialRuns: unknown[] = [],
  initialAgents: unknown[] = [],
  initialTargets: unknown[] = [],
  initialProfileArtifacts: unknown[] = [],
  initialAgentMetrics: unknown[] = [],
  profileArtifactList: unknown[] = initialProfileArtifacts,
  profileArtifactComparisonReport: unknown | null = null,
  processTrendComparisonReport: unknown | null = null,
  options: {
    runCreateFailure?: {
      status: number;
      body: Record<string, unknown>;
    };
    profileTaskList?: unknown[];
    profileTaskTemplates?: unknown[];
  } = {},
) {
  const runHistory = [...initialRuns];
  const runLookupCounts = new Map<string, number>();
  const targetList = [...initialTargets];
  const profileTaskList = [...(options.profileTaskList ?? [])];
  const profileTaskTemplates = [...(options.profileTaskTemplates ?? defaultProfileTaskTemplates)];

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method || 'GET').toUpperCase();

    if ((url === scenariosApiURL || url.startsWith(`${scenariosApiURL}?`)) && method === 'GET') {
      return {
        ok: true,
        json: async () => JSON.parse(JSON.stringify(filterRecordsByWorkspaceScope(initialScenarios, url))),
      };
    }

    if (url === scenariosApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      return {
        ok: true,
        json: async () => ({ ...body, id: 'server-scenario-1' }),
      };
    }

    if ((url === runsApiURL || url.startsWith(`${runsApiURL}?`)) && method === 'GET') {
      return {
        ok: true,
        json: async () => JSON.parse(JSON.stringify(filterRecordsByWorkspaceScope(runHistory, url))),
      };
    }

    if (url === runsApiURL && method === 'POST') {
      if (options.runCreateFailure) {
        return {
          ok: false,
          status: options.runCreateFailure.status,
          json: async () => options.runCreateFailure?.body || {},
        };
      }

      const body = JSON.parse(String(init?.body || '{}'));
      const run = {
        id: 'run-from-scenario-1',
        scenarioId: body.scenarioId,
        scenarioName: 'backend-checkout-smoke',
        targetId: body.targetId,
        targetName: body.targetId ? 'checkout-target' : undefined,
        name: 'backend-checkout-smoke',
        status: 'running',
        method: 'GET',
        url: body.targetId ? 'http://checkout.internal:8080/api/health' : 'http://127.0.0.1:8080/api/health',
        totalRequests: body.totalRequests,
        successRequests: 0,
        failedRequests: 0,
        durationMs: 0,
        qps: 0,
        averageLatencyMs: 0,
        p95LatencyMs: 0,
        createdAt: '2026-06-12T08:30:00Z',
      };
      runHistory.unshift(run);
      return {
        ok: true,
        json: async () => run,
      };
    }

    if (url.startsWith(`${runsApiURL}/`) && url.endsWith('/events') && method === 'GET') {
      const runID = url.slice(`${runsApiURL}/`.length, -'/events'.length);
      const run = runHistory.find((candidate) => (candidate as { id?: string }).id === runID) as
        | Record<string, unknown>
        | undefined;
      if (!run) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'run not found' }),
        };
      }
      return {
        ok: true,
        json: async () => [
          {
            id: `${runID}-event-start`,
            runId: runID,
            type: 'run_started',
            status: 'running',
            message: 'Run started',
            successRequests: 0,
            failedRequests: 0,
            totalRequests: Number(run.totalRequests || 0),
            qps: 0,
            p95LatencyMs: 0,
            createdAt: '2026-06-12T08:30:00Z',
          },
          {
            id: `${runID}-event-progress`,
            runId: runID,
            type: 'run_progress',
            status: String(run.status || 'running'),
            message: 'Run progress updated',
            successRequests: Math.floor(Number(run.totalRequests || 0) / 2),
            failedRequests: 0,
            totalRequests: Number(run.totalRequests || 0),
            qps: 240,
            p95LatencyMs: 3,
            createdAt: '2026-06-12T08:30:01Z',
          },
        ],
      };
    }

    if (url.startsWith(`${runsApiURL}/`) && method === 'GET') {
      const runID = url.slice(`${runsApiURL}/`.length);
      const run = runHistory.find((candidate) => (candidate as { id?: string }).id === runID) as
        | Record<string, unknown>
        | undefined;
      if (!run) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'run not found' }),
        };
      }
      const lookupCount = runLookupCounts.get(runID) ?? 0;
      runLookupCounts.set(runID, lookupCount + 1);
      if (run.status === 'running' && lookupCount < 2) {
        run.successRequests = Math.floor(Number(run.totalRequests || 0) / 2);
        run.durationMs = 6.2;
        run.qps = 240;
        run.averageLatencyMs = 2;
        run.p95LatencyMs = 3;
      } else if (run.status === 'running') {
        run.status = 'finished';
        run.successRequests = run.totalRequests;
        run.durationMs = 12.5;
        run.qps = 480;
        run.averageLatencyMs = 2.1;
        run.p95LatencyMs = 3.4;
      }
      return {
        ok: true,
        json: async () => run,
      };
    }

    if (url.startsWith(`${runsApiURL}/`) && url.endsWith('/stop') && method === 'POST') {
      const runID = url.slice(`${runsApiURL}/`.length, -'/stop'.length);
      const run = runHistory.find((candidate) => (candidate as { id?: string }).id === runID) as
        | Record<string, unknown>
        | undefined;
      if (!run) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'run not found' }),
        };
      }
      run.status = 'canceled';
      return {
        ok: true,
        json: async () => run,
      };
    }

    if ((url === agentsApiURL || url.startsWith(`${agentsApiURL}?`)) && method === 'GET') {
      return {
        ok: true,
        json: async () => JSON.parse(JSON.stringify(filterRecordsByWorkspaceScope(initialAgents, url))),
      };
    }

    if (url.startsWith(agentMetricsApiURL) && method === 'GET') {
      return {
        ok: true,
        json: async () => initialAgentMetrics,
      };
    }

    if (url === agentTokensApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      return {
        ok: true,
        json: async () => ({
          id: 'agent-token-1',
          name: body.name,
          projectId: body.projectId,
          environment: body.environment,
          token: 'ait_test_token',
          status: 'active',
          expiresAt: '2026-06-13T09:30:00Z',
          createdAt: '2026-06-12T09:30:00Z',
          updatedAt: '2026-06-12T09:30:00Z',
        }),
      };
    }

    if (url === `${agentTokensApiURL}/agent-token-1/revoke` && method === 'POST') {
      return {
        ok: true,
        json: async () => ({
          id: 'agent-token-1',
          name: 'agent-install-test',
          status: 'revoked',
          createdAt: '2026-06-12T09:30:00Z',
          updatedAt: '2026-06-12T09:35:00Z',
        }),
      };
    }

    if (url === `${agentTokensApiURL}/agent-token-1/rotate` && method === 'POST') {
      return {
        ok: true,
        json: async () => ({
          id: 'agent-token-1',
          name: 'agent-install-test',
          token: 'ait_rotated_token',
          status: 'active',
          expiresAt: '2026-06-13T10:30:00Z',
          rotatedAt: '2026-06-12T10:30:00Z',
          createdAt: '2026-06-12T09:30:00Z',
          updatedAt: '2026-06-12T10:30:00Z',
        }),
      };
    }

    if ((url === targetsApiURL || url.startsWith(`${targetsApiURL}?`)) && method === 'GET') {
      return {
        ok: true,
        json: async () => JSON.parse(JSON.stringify(filterRecordsByWorkspaceScope(targetList, url))),
      };
    }

    if (url === targetsApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      const target = {
        ...body,
        id: 'target-from-api-1',
        createdAt: '2026-06-12T09:40:00Z',
        updatedAt: '2026-06-12T09:40:00Z',
      };
      targetList.unshift(target);
      return {
        ok: true,
        json: async () => target,
      };
    }

    if (url.startsWith(`${targetsApiURL}/`) && url.includes('/health-checks') && method === 'GET') {
      const targetPath = url.slice(`${targetsApiURL}/`.length);
      const targetID = targetPath.slice(0, targetPath.indexOf('/health-checks'));
      const target = targetList.find((candidate) => (candidate as { id?: string }).id === targetID) as
        | Record<string, unknown>
        | undefined;
      if (!target) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'target not found' }),
        };
      }
      return {
        ok: true,
        json: async () => {
          const history = target.healthHistory as unknown[] | undefined;
          if (Array.isArray(history)) {
            return history;
          }
          return target.lastHealthCheck ? [target.lastHealthCheck] : [];
        },
      };
    }

    if (url.startsWith(`${targetsApiURL}/`) && url.endsWith('/health-check') && method === 'POST') {
      const targetID = url.slice(`${targetsApiURL}/`.length, -'/health-check'.length);
      const target = targetList.find((candidate) => (candidate as { id?: string }).id === targetID) as
        | Record<string, unknown>
        | undefined;
      if (!target) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'target not found' }),
        };
      }
      const healthCheck = (target.healthCheck || {}) as Record<string, unknown>;
      const result = {
        targetId: targetID,
        targetName: target.name,
        status: 'healthy',
        url: `${String(target.baseUrl).replace(/\/$/, '')}${String(healthCheck.path || '/health')}`,
        expectedStatus: Number(healthCheck.expectedStatus || 200),
        observedStatus: Number(healthCheck.expectedStatus || 200),
        latencyMs: 12.5,
        checkedAt: '2026-06-12T09:50:00Z',
      };
      target.lastHealthCheck = result;
      target.healthHistory = [result, ...(((target.healthHistory as unknown[] | undefined) || []))];
      return {
        ok: true,
        json: async () => result,
      };
    }

    if (url.startsWith(`${targetsApiURL}/`) && method === 'PUT') {
      const targetID = url.slice(`${targetsApiURL}/`.length);
      const targetIndex = targetList.findIndex((target) => (target as { id?: string }).id === targetID);
      if (targetIndex < 0) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'target not found' }),
        };
      }
      const body = JSON.parse(String(init?.body || '{}'));
      const currentTarget = targetList[targetIndex] as { createdAt?: string };
      const target = {
        ...body,
        id: targetID,
        createdAt: currentTarget.createdAt || '2026-06-12T09:40:00Z',
        updatedAt: '2026-06-12T09:45:00Z',
      };
      targetList.splice(targetIndex, 1, target);
      return {
        ok: true,
        json: async () => target,
      };
    }

    if (url.startsWith(`${targetsApiURL}/`) && method === 'DELETE') {
      const targetID = url.slice(`${targetsApiURL}/`.length);
      const targetIndex = targetList.findIndex((target) => (target as { id?: string }).id === targetID);
      if (targetIndex < 0) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'target not found' }),
        };
      }
      targetList.splice(targetIndex, 1);
      return {
        ok: true,
        status: 204,
        json: async () => ({}),
      };
    }

    if ((url === profileArtifactsApiURL || url.startsWith(`${profileArtifactsApiURL}?`)) && method === 'GET') {
      return {
        ok: true,
        json: async () => profileArtifactList,
      };
    }

    if (url === profileTasksApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      const task = {
        ...body,
        id: 'profile-task-from-target-1',
        status: 'pending',
        attempts: 0,
        maxAttempts: Number(body.maxAttempts || 3),
        createdAt: '2026-06-12T10:40:00Z',
        updatedAt: '2026-06-12T10:40:00Z',
      };
      profileTaskList.unshift(task);
      return {
        ok: true,
        status: 201,
        json: async () => task,
      };
    }

    if (url === profileTaskTemplatesApiURL && method === 'GET') {
      return {
        ok: true,
        json: async () => profileTaskTemplates,
      };
    }

    if (url === profileTaskTemplatesApiURL && method === 'POST') {
      const body = JSON.parse(String(init?.body || '{}'));
      const template = {
        ...body,
        id: body.id || 'template-from-api-1',
        enabled: body.enabled ?? true,
        createdAt: '2026-06-12T10:10:00Z',
        updatedAt: '2026-06-12T10:10:00Z',
      };
      profileTaskTemplates.push(template);
      return {
        ok: true,
        status: 201,
        json: async () => template,
      };
    }

    if (url.startsWith(`${profileTaskTemplatesApiURL}/`) && method === 'PUT') {
      const templateID = decodeURIComponent(url.slice(`${profileTaskTemplatesApiURL}/`.length));
      const templateIndex = profileTaskTemplates.findIndex((template) => (template as { id?: string }).id === templateID);
      if (templateIndex < 0) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'profile task template not found' }),
        };
      }
      const body = JSON.parse(String(init?.body || '{}'));
      const currentTemplate = profileTaskTemplates[templateIndex] as { createdAt?: string };
      const template = {
        ...body,
        id: templateID,
        enabled: body.enabled ?? true,
        createdAt: currentTemplate.createdAt || '2026-06-12T10:10:00Z',
        updatedAt: '2026-06-12T10:20:00Z',
      };
      profileTaskTemplates.splice(templateIndex, 1, template);
      return {
        ok: true,
        json: async () => template,
      };
    }

    if (url.startsWith(`${profileTaskTemplatesApiURL}/`) && method === 'DELETE') {
      const templateID = decodeURIComponent(url.slice(`${profileTaskTemplatesApiURL}/`.length));
      const templateIndex = profileTaskTemplates.findIndex((template) => (template as { id?: string }).id === templateID);
      if (templateIndex < 0) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'profile task template not found' }),
        };
      }
      profileTaskTemplates.splice(templateIndex, 1);
      return {
        ok: true,
        status: 204,
        json: async () => ({}),
      };
    }

    if (url.startsWith(`${profileTasksApiURL}/`) && url.endsWith('/retry') && method === 'POST') {
      const taskID = decodeURIComponent(url.slice(`${profileTasksApiURL}/`.length, -'/retry'.length));
      const taskIndex = profileTaskList.findIndex((task) => (task as { id?: string }).id === taskID);
      if (taskIndex < 0) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'profile task not found' }),
        };
      }
      const currentTask = profileTaskList[taskIndex] as Record<string, unknown>;
      const retriedTask = {
        ...currentTask,
        status: 'pending',
        attempts: 0,
        artifactId: '',
        error: '',
        leasedAt: '',
        completedAt: '',
        updatedAt: '2026-06-12T08:32:00Z',
      };
      profileTaskList.splice(taskIndex, 1, retriedTask);
      return {
        ok: true,
        json: async () => retriedTask,
      };
    }

    if (url.startsWith(profileTasksApiURL) && method === 'GET') {
      return {
        ok: true,
        json: async () => profileTaskList,
      };
    }

    if (url.startsWith(runReportsCompareApiURL) && method === 'GET') {
      const comparedRuns = runHistory.slice(0, 5).map((run) => {
        const candidate = run as Record<string, unknown>;
        const successRequests = Number(candidate.successRequests || 0);
        const failedRequests = Number(candidate.failedRequests || 0);
        const completedRequests = successRequests + failedRequests;
        return {
          id: candidate.id,
          name: candidate.name || candidate.scenarioName,
          scenarioName: candidate.scenarioName,
          targetName: candidate.targetName,
          status: candidate.status,
          totalRequests: Number(candidate.totalRequests || 0),
          successRequests,
          failedRequests,
          successRatePercent: completedRequests > 0 ? (successRequests / completedRequests) * 100 : 0,
          errorRatePercent: completedRequests > 0 ? (failedRequests / completedRequests) * 100 : 0,
          durationMs: Number(candidate.durationMs || 0),
          qps: Number(candidate.qps || 0),
          averageLatencyMs: Number(candidate.averageLatencyMs || 0),
          p95LatencyMs: Number(candidate.p95LatencyMs || 0),
          createdAt: candidate.createdAt,
        };
      });
      const bestQpsRun = comparedRuns.reduce((currentBest, run) => (run.qps > currentBest.qps ? run : currentBest), comparedRuns[0]);
      const fastestP95Run = comparedRuns.reduce(
        (currentBest, run) => (run.p95LatencyMs < currentBest.p95LatencyMs ? run : currentBest),
        comparedRuns[0],
      );
      const highestErrorRun = comparedRuns.reduce(
        (currentWorst, run) => (run.errorRatePercent > currentWorst.errorRatePercent ? run : currentWorst),
        comparedRuns[0],
      );
      return {
        ok: true,
        json: async () => ({
          generatedAt: '2026-06-16T10:30:00Z',
          runs: comparedRuns,
          summary: {
            runCount: comparedRuns.length,
            bestQpsRunId: bestQpsRun?.id || '',
            fastestP95RunId: fastestP95Run?.id || '',
            highestErrorRateRunId: highestErrorRun?.id || '',
          },
        }),
      };
    }

    if (url.startsWith(profileArtifactCompareApiURL) && method === 'GET') {
      return {
        ok: true,
        json: async () =>
          profileArtifactComparisonReport ?? {
            generatedAt: '2026-06-16T10:30:00Z',
            groups: [],
            summary: {
              runCount: runHistory.slice(0, 5).length,
              artifactCount: 0,
              collectedCount: 0,
              failedCount: 0,
              totalSizeBytes: 0,
            },
          },
      };
    }

    if (url.startsWith(processTrendCompareApiURL) && method === 'GET') {
      return {
        ok: true,
        json: async () =>
          processTrendComparisonReport ?? {
            generatedAt: '2026-06-16T10:30:00Z',
            groups: [],
            summary: {
              runCount: runHistory.slice(0, 5).length,
              processCount: 0,
              highestCpuProcessKey: '',
              highestMemoryProcessKey: '',
            },
          },
      };
    }

    if (url.startsWith(`${runReportsApiURL}/`) && method === 'GET') {
      const runID = url.slice(`${runReportsApiURL}/`.length);
      const run = runHistory.find((candidate) => (candidate as { id?: string }).id === runID) as
        | Record<string, unknown>
        | undefined;
      if (!run) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: 'run not found' }),
        };
      }
      const reportArtifacts = initialProfileArtifacts.filter(
        (artifact) => (artifact as { runId?: string }).runId === runID,
      );
      const successRequests = Number(run.successRequests || 0);
      const failedRequests = Number(run.failedRequests || 0);
      const completedRequests = successRequests + failedRequests;
      const slowSamples = Array.isArray((run as { slowSamples?: unknown[] }).slowSamples)
        ? ((run as { slowSamples: unknown[] }).slowSamples)
        : [];
      const errorSamples = Array.isArray((run as { errorSamples?: unknown[] }).errorSamples)
        ? ((run as { errorSamples: unknown[] }).errorSamples)
        : [];
      const targetMetrics = (run as { targetMetrics?: unknown }).targetMetrics ?? null;
      const targetHealthChecks = Array.isArray((run as { targetHealthChecks?: unknown[] }).targetHealthChecks)
        ? ((run as { targetHealthChecks: unknown[] }).targetHealthChecks)
        : [];
      const alerts = Array.isArray((run as { alerts?: unknown[] }).alerts) ? ((run as { alerts: unknown[] }).alerts) : [];
      return {
        ok: true,
        json: async () => ({
          run,
          events: [
            {
              id: `${runID}-event-start`,
              runId: runID,
              type: 'run_started',
              status: 'running',
              message: 'Run started',
              successRequests: 0,
              failedRequests: 0,
              totalRequests: Number(run.totalRequests || 0),
              qps: 0,
              p95LatencyMs: 0,
              createdAt: '2026-06-12T08:30:00Z',
            },
            {
              id: `${runID}-event-finished`,
              runId: runID,
              type: 'run_finished',
              status: String(run.status || 'finished'),
              message: 'Run finished',
              successRequests,
              failedRequests,
              totalRequests: Number(run.totalRequests || 0),
              qps: Number(run.qps || 0),
              p95LatencyMs: Number(run.p95LatencyMs || 0),
              createdAt: '2026-06-12T08:30:01Z',
            },
          ],
          profileArtifacts: reportArtifacts,
          slowSamples,
          errorSamples,
          alerts,
          targetMetrics,
          targetHealthChecks,
          summary: {
            successRatePercent: completedRequests > 0 ? (successRequests / completedRequests) * 100 : 0,
            errorRatePercent: completedRequests > 0 ? (failedRequests / completedRequests) * 100 : 0,
            eventCount: 2,
            profileArtifactCount: reportArtifacts.length,
            slowSampleCount: slowSamples.length,
            errorSampleCount: errorSamples.length,
            alertCount: alerts.length,
          },
        }),
      };
    }

    throw new Error(`Unexpected fetch ${method} ${url}`);
  });
}

function createMockXMLHttpRequest(fetchMock: ReturnType<typeof createScenarioFetchMock>) {
  return class MockXMLHttpRequest {
    private method = 'GET';
    private url = '';
    private requestHeaders: Record<string, string> = {};
    status = 0;
    responseText = '';
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    open(method: string, url: string) {
      this.method = method;
      this.url = url;
    }

    setRequestHeader(key: string, value: string) {
      this.requestHeaders[key] = value;
    }

    async send(body?: XMLHttpRequestBodyInit | null) {
      try {
        const response = await fetchMock(this.url, {
          method: this.method,
          headers: this.requestHeaders,
          body: body ?? undefined,
        });
        this.status = response.status ?? (response.ok ? 200 : 500);
        this.responseText = JSON.stringify(await response.json());
        this.onload?.();
      } catch {
        this.onerror?.();
      }
    }
  };
}

function installJSONPScriptMock(fetchMock: ReturnType<typeof createScenarioFetchMock>, URLParser: typeof URL = URL) {
  const originalAppendChild = document.head.appendChild.bind(document.head);
  return vi.spyOn(document.head, 'appendChild').mockImplementation((node: Node) => {
    if (node instanceof HTMLScriptElement && node.src.includes('callback=')) {
      const scriptURL = new URLParser(node.src);
      const callbackName = scriptURL.searchParams.get('callback') || '';
      scriptURL.searchParams.delete('callback');
      void fetchMock(scriptURL.toString())
        .then(async (response) => {
          const callback = (window as unknown as Record<string, (payload: unknown) => void>)[callbackName];
          callback?.(await response.json());
        })
        .catch(() => {
          node.onerror?.(new Event('error'));
        });
      return node;
    }
    return originalAppendChild(node);
  });
}

describe('App', () => {
  beforeEach(() => {
    window.location.hash = '';
    window.localStorage.clear();
    vi.stubGlobal('fetch', createScenarioFetchMock());
  });

  afterEach(() => {
    vi.restoreAllMocks();
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

  it('groups compact dashboard row status badges inside their rows', () => {
    render(<App />);

    const agentRow = screen.getByText('api-gateway-01').closest('.agent-row');
    expect(agentRow?.querySelector('.agent-row-status')).not.toBeNull();
    expect(within(agentRow as HTMLElement).getByText('健康 / Healthy')).toBeInTheDocument();

    const recentRunRow = screen.getByText('RUN-2407').closest('.run-row');
    expect(recentRunRow?.querySelector('.run-row-status')).not.toBeNull();
    expect(within(recentRunRow as HTMLElement).getByText('通过 / Passed')).toBeInTheDocument();
  });

  it('loads dashboard run and agent metrics from backend APIs', async () => {
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-dashboard-live',
          scenarioName: 'checkout-live-dashboard',
          name: 'checkout-live-dashboard',
          status: 'running',
          method: 'GET',
          url: 'http://checkout.internal:8080/api/health',
          totalRequests: 100,
          successRequests: 80,
          failedRequests: 5,
          durationMs: 2000,
          qps: 42.42,
          averageLatencyMs: 14.5,
          p95LatencyMs: 123.45,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [
        {
          id: 'agent-live-01',
          name: 'agent-live-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.11',
          version: '0.2.0',
          status: 'online',
          labels: { zone: 'shanghai-live' },
          capabilities: ['host_metrics'],
          lastSeenAt: '2026-06-12T08:31:00Z',
          latestMetrics: {
            agentId: 'agent-live-01',
            collectedAt: '2026-06-12T08:31:00Z',
            cpuUsagePercent: 61.2,
            memoryUsagePercent: 44.8,
            diskReadBytesPerSec: 1048576,
            diskWriteBytesPerSec: 524288,
            networkRxBytesPerSec: 4096,
            networkTxBytesPerSec: 8192,
            processes: [],
          },
        },
        {
          id: 'agent-live-02',
          name: 'agent-live-02',
          hostname: 'checkout-host-02',
          ip: '10.0.0.12',
          version: '0.2.0',
          status: 'offline',
          labels: { zone: 'shanghai-live' },
          capabilities: ['host_metrics'],
          lastSeenAt: '2026-06-12T08:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByText('run-dashboard-live')).toBeInTheDocument();
    expect(screen.getByText('42.42')).toBeInTheDocument();
    expect(screen.getByText('123.45 ms')).toBeInTheDocument();
    expect(screen.getByText('5.00%')).toBeInTheDocument();
    expect(screen.getAllByText('1 / 2').length).toBeGreaterThan(0);

    const agentName = await screen.findByText('agent-live-01');
    const agentRow = agentName.closest('.agent-row');
    expect(within(agentRow as HTMLElement).getByText('shanghai-live')).toBeInTheDocument();
    expect(within(agentRow as HTMLElement).getByText('CPU 61.2%')).toBeInTheDocument();
    expect(within(agentRow as HTMLElement).getByText('MEM 44.8%')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(runsApiURL);
    expect(fetchMock).toHaveBeenCalledWith(agentsApiURL);
  });

  it('refreshes dashboard agent health from the backend API', async () => {
    const dashboardAgents = [
      {
        id: 'agent-live-01',
        name: 'agent-live-01',
        hostname: 'checkout-host-01',
        ip: '10.0.0.11',
        version: '0.2.0',
        status: 'online',
        labels: { zone: 'shanghai-live' },
        capabilities: ['host_metrics'],
        lastSeenAt: '2026-06-12T08:31:00Z',
        latestMetrics: {
          agentId: 'agent-live-01',
          collectedAt: '2026-06-12T08:31:00Z',
          cpuUsagePercent: 12.3,
          memoryUsagePercent: 20.5,
          diskReadBytesPerSec: 1048576,
          diskWriteBytesPerSec: 524288,
          networkRxBytesPerSec: 4096,
          networkTxBytesPerSec: 8192,
          processes: [],
        },
      },
    ];
    const fetchMock = createScenarioFetchMock([], [], dashboardAgents);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const agentName = await screen.findByText('agent-live-01');
    const agentRow = agentName.closest('.agent-row');
    expect(within(agentRow as HTMLElement).getByText('CPU 12.3%')).toBeInTheDocument();

    dashboardAgents[0].latestMetrics.cpuUsagePercent = 88.8;
    dashboardAgents[0].latestMetrics.memoryUsagePercent = 77.7;

    await waitFor(() => {
      expect(fetchMock.mock.calls.filter(([url]) => url === agentsApiURL).length).toBeGreaterThan(1);
    });
    await waitFor(() => {
      expect(within(agentRow as HTMLElement).getByText('CPU 88.8%')).toBeInTheDocument();
      expect(within(agentRow as HTMLElement).getByText('MEM 77.7%')).toBeInTheDocument();
    });
  });

  it('stops the active backend run from the dashboard', async () => {
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-dashboard-live',
          scenarioName: 'checkout-live-dashboard',
          name: 'checkout-live-dashboard',
          status: 'running',
          method: 'GET',
          url: 'http://checkout.internal:8080/api/health',
          totalRequests: 100,
          successRequests: 80,
          failedRequests: 5,
          durationMs: 2000,
          qps: 42.42,
          averageLatencyMs: 14.5,
          p95LatencyMs: 123.45,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByText('run-dashboard-live')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Stop/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${runsApiURL}/run-dashboard-live/stop`, { method: 'POST' });
    });
    expect((await screen.findAllByText('canceled')).length).toBeGreaterThan(0);
  });

  it('refreshes active dashboard run progress until completion', async () => {
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-dashboard-live',
          scenarioName: 'checkout-live-dashboard',
          name: 'checkout-live-dashboard',
          status: 'running',
          method: 'GET',
          url: 'http://checkout.internal:8080/api/health',
          totalRequests: 6,
          successRequests: 0,
          failedRequests: 0,
          durationMs: 0,
          qps: 0,
          averageLatencyMs: 0,
          p95LatencyMs: 0,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByText('run-dashboard-live')).toBeInTheDocument();
    expect(screen.getByText(/0 \/ 6 completed/)).toBeInTheDocument();
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${runsApiURL}/run-dashboard-live`);
    });
    await waitFor(() => {
      expect(screen.getByText(/3 \/ 6 completed/)).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText(/6 \/ 6 completed/)).toBeInTheDocument();
      expect(screen.getAllByText('finished').length).toBeGreaterThan(0);
    });
  });

  it('opens the runs page from the dashboard start run action', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: /Start run/ }));

    await waitFor(() => {
      expect(window.location.hash).toBe('#runs');
    });
    expect(screen.getByRole('heading', { name: /Runs/ })).toBeInTheDocument();
  });

  it('opens the runs page from the dashboard recent runs view all action', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: /View all/ }));

    await waitFor(() => {
      expect(window.location.hash).toBe('#runs');
    });
    expect(screen.getByRole('heading', { name: /Runs/ })).toBeInTheDocument();
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

  it('applies workspace scope to scenario loading and new scenario defaults', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-checkout-scope',
        name: 'checkout-scope',
        projectId: 'project-checkout',
        environment: 'staging',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://checkout.internal:8080',
        path: '/ready',
        queryVariants: [{ name: 'default', weight: '100', queryParams: '' }],
        headers: [],
        bodyVariants: [],
        flowSteps: [],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
      {
        id: 'scenario-billing-scope',
        name: 'billing-scope',
        projectId: 'project-billing',
        environment: 'prod',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://billing.internal:8080',
        path: '/ready',
        queryVariants: [{ name: 'default', weight: '100', queryParams: '' }],
        headers: [],
        bodyVariants: [],
        flowSteps: [],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.clear(screen.getByLabelText('Workspace project ID'));
    await user.type(screen.getByLabelText('Workspace project ID'), 'project-checkout');
    await user.clear(screen.getByLabelText('Workspace environment'));
    await user.type(screen.getByLabelText('Workspace environment'), 'staging');
    await user.click(screen.getByRole('button', { name: /Apply scope/ }));

    const checkoutScopeHeaders = expect.objectContaining({
      'X-AIT-Project-ID': 'project-checkout',
      'X-AIT-Environment': 'staging',
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${scenariosApiURL}?projectId=project-checkout&environment=staging`,
        expect.objectContaining({ headers: checkoutScopeHeaders }),
      );
    });
    expect(await screen.findByLabelText('scenario checkout-scope')).toBeInTheDocument();
    expect(screen.queryByLabelText('scenario billing-scope')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByLabelText(/Project ID/)).toHaveValue('project-checkout');
    expect(within(dialog).getByLabelText(/Environment/)).toHaveValue('staging');
    await user.type(within(dialog).getByLabelText(/Scenario name/), 'scope-created-scenario');
    await user.click(within(dialog).getByRole('button', { name: /Create/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        scenariosApiURL,
        expect.objectContaining({
          method: 'POST',
          headers: checkoutScopeHeaders,
        }),
      );
    });
    const createCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === scenariosApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.projectId).toBe('project-checkout');
    expect(postedScenario.environment).toBe('staging');
  });

  it('applies workspace scope to agent and target loading on the targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: { zone: 'shanghai-a' },
          capabilities: ['host_metrics'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-target',
          projectId: 'project-checkout',
          environment: 'staging',
          baseUrl: 'http://checkout.internal:8080',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: { name: 'checkout', cmdlineContains: '' },
          healthCheck: { enabled: false, path: '/health', expectedStatus: 200, timeoutMs: 1000 },
          metricThresholds: {},
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.clear(screen.getByLabelText('Workspace project ID'));
    await user.type(screen.getByLabelText('Workspace project ID'), 'project-checkout');
    await user.clear(screen.getByLabelText('Workspace environment'));
    await user.type(screen.getByLabelText('Workspace environment'), 'staging');
    await user.click(screen.getByRole('button', { name: /Apply scope/ }));

    const checkoutScopeHeaders = expect.objectContaining({
      'X-AIT-Project-ID': 'project-checkout',
      'X-AIT-Environment': 'staging',
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${agentsApiURL}?projectId=project-checkout&environment=staging`,
        expect.objectContaining({ headers: checkoutScopeHeaders }),
      );
    });
    expect(fetchMock).toHaveBeenCalledWith(
      `${targetsApiURL}?projectId=project-checkout&environment=staging`,
      expect.objectContaining({ headers: checkoutScopeHeaders }),
    );
    expect(screen.getByLabelText('Agent token project ID')).toHaveValue('project-checkout');
    expect(screen.getByLabelText('Agent token environment')).toHaveValue('staging');
    expect(await screen.findByLabelText('agent checkout-01')).toBeInTheDocument();
    expect(await screen.findByLabelText('target checkout-target')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Generate token/ }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        agentTokensApiURL,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
            'X-AIT-Project-ID': 'project-checkout',
            'X-AIT-Environment': 'staging',
          }),
        }),
      );
    });
  });

  it('applies workspace scope to run, report, metric, and profile task requests', async () => {
    window.location.hash = '#reports';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-checkout-scope',
          projectId: 'project-checkout',
          environment: 'staging',
          scenarioName: 'checkout-scope',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 4,
          successRequests: 4,
          failedRequests: 0,
          durationMs: 80,
          qps: 50,
          averageLatencyMs: 12,
          p95LatencyMs: 40,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [
        {
          id: 'agent-checkout-scope',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-agent-scope',
          hostname: 'checkout-host-scope',
          ip: '10.0.0.21',
          version: '0.3.0',
          status: 'online',
          labels: {},
          capabilities: ['host_metrics'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [],
      [
        {
          id: 'profile-artifact-checkout-scope',
          runId: 'run-checkout-scope',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'checkout-scope.pprof',
          sizeBytes: 16,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
      [
        {
          agentId: 'agent-checkout-scope',
          collectedAt: '2026-06-12T09:20:00Z',
          cpuUsagePercent: 12,
          memoryUsagePercent: 30,
          diskReadBytesPerSec: 0,
          diskWriteBytesPerSec: 0,
          networkRxBytesPerSec: 0,
          networkTxBytesPerSec: 0,
          processes: [],
        },
      ],
      [],
      undefined,
      undefined,
      {
        profileTaskList: [
          {
            id: 'profile-task-checkout-scope',
            runId: 'run-checkout-scope',
            agentId: 'agent-checkout-scope',
            profileType: 'cpu',
            profileSeconds: 1,
            source: 'run_auto',
            status: 'failed',
            attempts: 1,
            maxAttempts: 1,
            error: 'profile failed',
            createdAt: '2026-06-12T08:30:00Z',
            updatedAt: '2026-06-12T08:30:01Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.clear(screen.getByLabelText('Workspace project ID'));
    await user.type(screen.getByLabelText('Workspace project ID'), 'project-checkout');
    await user.clear(screen.getByLabelText('Workspace environment'));
    await user.type(screen.getByLabelText('Workspace environment'), 'staging');
    await user.click(screen.getByRole('button', { name: /Apply scope/ }));

    const checkoutScopeHeaders = expect.objectContaining({
      'X-AIT-Project-ID': 'project-checkout',
      'X-AIT-Environment': 'staging',
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${runsApiURL}?projectId=project-checkout&environment=staging`,
        expect.objectContaining({ headers: checkoutScopeHeaders }),
      );
    });
    expect(fetchMock).toHaveBeenCalledWith(
      `${profileArtifactsApiURL}?projectId=project-checkout&environment=staging`,
      expect.objectContaining({ headers: checkoutScopeHeaders }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      `${runReportsCompareApiURL}?limit=5&projectId=project-checkout&environment=staging`,
      expect.objectContaining({ headers: checkoutScopeHeaders }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      `${profileArtifactCompareApiURL}?limit=5&projectId=project-checkout&environment=staging`,
      expect.objectContaining({ headers: checkoutScopeHeaders }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      `${agentMetricsApiURL}?agentId=agent-checkout-scope&limit=120&projectId=project-checkout&environment=staging`,
      expect.objectContaining({ headers: checkoutScopeHeaders }),
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${profileTasksApiURL}?runId=run-checkout-scope&limit=20&projectId=project-checkout&environment=staging`,
        expect.objectContaining({ headers: checkoutScopeHeaders }),
      );
    });
  });

  it('adds a request flow step on the scenarios page', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    render(<App />);

    expect(screen.getByText('04 assert status')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Add step/ }));

    expect(screen.getByText('05 custom request')).toBeInTheDocument();
  });

  it('saves the current request flow with created scenarios', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByRole('button', { name: /Add step/ }));
    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'flow-backed-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps).toHaveLength(5);
    expect(postedScenario.flowSteps[4]).toEqual(
      expect.objectContaining({
        name: 'custom request',
        type: 'request',
        protocol: 'HTTP',
      }),
    );

    const createdScenario = await screen.findByLabelText('scenario flow-backed-scenario');
    expect(within(createdScenario).getByText('05 custom request')).toBeInTheDocument();
  });

  it('configures an executable request flow step before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByRole('button', { name: /Add step/ }));
    await user.click(screen.getByLabelText('Enable flow step 5'));
    await user.selectOptions(screen.getByLabelText('Flow step method 5'), 'POST');
    fireEvent.change(screen.getByLabelText('Flow step path 5'), {
      target: { value: '/checkout' },
    });

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'executable-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[4]).toEqual(
      expect.objectContaining({
        enabled: true,
        method: 'POST',
        path: '/checkout',
      }),
    );
  });

  it('configures a variable extractor flow step before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByLabelText('Enable extract flow step 2'));
    await user.clear(screen.getByLabelText('Extractor variable name 2'));
    await user.type(screen.getByLabelText('Extractor variable name 2'), 'session');
    await user.selectOptions(screen.getByLabelText('Extractor source 2'), 'header');
    await user.clear(screen.getByLabelText('Extractor path 2'));
    await user.type(screen.getByLabelText('Extractor path 2'), 'X-Session-Id');

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'extractor-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[1]).toEqual(
      expect.objectContaining({
        enabled: true,
        type: 'extract',
        extractors: [{ name: 'session', source: 'header', path: 'X-Session-Id' }],
      }),
    );
  });

  it('configures a regex variable extractor before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByLabelText('Enable extract flow step 2'));
    await user.clear(screen.getByLabelText('Extractor variable name 2'));
    await user.type(screen.getByLabelText('Extractor variable name 2'), 'order');
    await user.selectOptions(screen.getByLabelText('Extractor source 2'), 'regex');
    await user.clear(screen.getByLabelText('Extractor path 2'));
    fireEvent.change(screen.getByLabelText('Extractor path 2'), {
      target: { value: 'order=(ORD-[0-9]+)' },
    });

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'regex-extractor-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[1]).toEqual(
      expect.objectContaining({
        enabled: true,
        type: 'extract',
        extractors: [{ name: 'order', source: 'regex', path: 'order=(ORD-[0-9]+)' }],
      }),
    );
  });

  it('configures a response assertion flow step before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByLabelText('Enable assertion flow step 4'));
    await user.selectOptions(screen.getByLabelText('Assertion source 4'), 'json');
    await user.clear(screen.getByLabelText('Assertion path 4'));
    await user.type(screen.getByLabelText('Assertion path 4'), 'state');
    await user.selectOptions(screen.getByLabelText('Assertion operator 4'), 'equals');
    await user.clear(screen.getByLabelText('Assertion expected 4'));
    await user.type(screen.getByLabelText('Assertion expected 4'), 'ok');

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'assertion-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[3]).toEqual(
      expect.objectContaining({
        enabled: true,
        type: 'assertion',
        assertions: [{ name: 'assert status', source: 'json', path: 'state', operator: 'equals', expected: 'ok' }],
      }),
    );
  });

  it('configures a regex response assertion before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByLabelText('Enable assertion flow step 4'));
    await user.selectOptions(screen.getByLabelText('Assertion source 4'), 'body');
    await user.selectOptions(screen.getByLabelText('Assertion operator 4'), 'matches');
    await user.clear(screen.getByLabelText('Assertion expected 4'));
    fireEvent.change(screen.getByLabelText('Assertion expected 4'), {
      target: { value: 'state=OK\\s+order=ORD-[0-9]+' },
    });

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'regex-assertion-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[3]).toEqual(
      expect.objectContaining({
        enabled: true,
        type: 'assertion',
        assertions: [
          {
            name: 'assert status',
            source: 'body',
            path: 'state',
            operator: 'matches',
            expected: 'state=OK\\s+order=ORD-[0-9]+',
          },
        ],
      }),
    );
  });

  it('configures a when condition before saving a scenario', async () => {
    window.location.hash = '#scenarios';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByLabelText('Enable flow step 1'));
    await user.click(screen.getByLabelText('Enable when condition 1'));
    await user.selectOptions(screen.getByLabelText('When source 1'), 'variable');
    await user.clear(screen.getByLabelText('When path 1'));
    await user.type(screen.getByLabelText('When path 1'), 'mode');
    await user.selectOptions(screen.getByLabelText('When operator 1'), 'equals');
    await user.clear(screen.getByLabelText('When expected 1'));
    await user.type(screen.getByLabelText('When expected 1'), 'run');

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'when-flow-scenario');
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
    const postedScenario = JSON.parse(String(createCall?.[1]?.body || '{}'));
    expect(postedScenario.flowSteps[0]).toEqual(
      expect.objectContaining({
        enabled: true,
        type: 'request',
        when: { name: 'when condition', source: 'variable', path: 'mode', operator: 'equals', expected: 'run' },
      }),
    );
  });

  it('shows executable and draft flow step details in saved scenarios', async () => {
    window.location.hash = '#scenarios';
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-flow-api',
        name: 'checkout-flow-saved',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://127.0.0.1:8080',
        path: '/api/health',
        queryVariants: [],
        headers: [],
        bodyVariants: [],
        flowSteps: [
          {
            id: 'login',
            name: 'HTTP login',
            type: 'request',
            enabled: true,
            protocol: 'HTTP',
            method: 'GET',
            path: '/login',
          },
          {
            id: 'checkout',
            name: 'HTTP checkout',
            type: 'request',
            enabled: true,
            protocol: 'HTTP',
            method: 'POST',
            path: '/checkout',
          },
          {
            id: 'draft',
            name: 'draft request',
            type: 'request',
            enabled: false,
            protocol: 'HTTP',
            method: 'GET',
            path: '/draft',
          },
        ],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const savedScenario = await screen.findByLabelText('scenario checkout-flow-saved');
    expect(within(savedScenario).getByText('01 GET /login')).toBeInTheDocument();
    expect(within(savedScenario).getByText('02 POST /checkout')).toBeInTheDocument();
    expect(within(savedScenario).getByText('03 GET /draft')).toBeInTheDocument();
    expect(within(savedScenario).getAllByText('执行 / Execute')).toHaveLength(2);
    expect(within(savedScenario).getByText('草稿 / Draft')).toBeInTheDocument();
  });

  it('shows variable extractors in saved scenario flow steps', async () => {
    window.location.hash = '#scenarios';
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-flow-extract',
        name: 'checkout-flow-extract',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://127.0.0.1:8080',
        path: '/api/health',
        queryVariants: [],
        headers: [{ key: 'Authorization', value: 'Bearer ${token}' }],
        bodyVariants: [],
        flowSteps: [
          {
            id: 'login',
            name: 'HTTP login',
            type: 'request',
            enabled: true,
            protocol: 'HTTP',
            method: 'GET',
            path: '/login',
          },
          {
            id: 'extract-token',
            name: 'extract token',
            type: 'extract',
            enabled: true,
            extractors: [{ name: 'token', source: 'json', path: 'token' }],
          },
          {
            id: 'checkout',
            name: 'HTTP checkout',
            type: 'request',
            enabled: true,
            protocol: 'HTTP',
            method: 'GET',
            path: '/checkout/${token}',
          },
        ],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const savedScenario = await screen.findByLabelText('scenario checkout-flow-extract');
    expect(within(savedScenario).getByText('02 extract token')).toBeInTheDocument();
    expect(within(savedScenario).getByText('提取 / Extract')).toBeInTheDocument();
    expect(within(savedScenario).getByText('token <- json token')).toBeInTheDocument();
    expect(within(savedScenario).getByText('03 GET /checkout/${token}')).toBeInTheDocument();
  });

  it('shows when conditions in saved scenario flow steps', async () => {
    window.location.hash = '#scenarios';
    const fetchMock = createScenarioFetchMock([
      {
        id: 'scenario-flow-when',
        name: 'checkout-flow-when',
        protocol: 'HTTP',
        method: 'GET',
        baseUrl: 'http://127.0.0.1:8080',
        path: '/api/health',
        queryVariants: [],
        headers: [],
        bodyVariants: [],
        flowSteps: [
          {
            id: 'checkout',
            name: 'HTTP checkout',
            type: 'request',
            enabled: true,
            protocol: 'HTTP',
            method: 'GET',
            path: '/checkout',
            when: { name: 'mode is run', source: 'variable', path: 'mode', operator: 'equals', expected: 'run' },
          },
        ],
        timeoutMs: '1000',
        retryCount: '0',
        assertion: 'status < 400',
      },
    ]);
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const savedScenario = await screen.findByLabelText('scenario checkout-flow-when');
    expect(within(savedScenario).getByText('when variable mode equals run')).toBeInTheDocument();
  });

  it('loads profile artifacts from the backend API', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [],
      [],
      [
        {
          id: 'profile-artifact-1',
          runId: 'run-from-scenario-1',
          scenarioName: 'backend-checkout-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-from-scenario-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const artifact = await screen.findByLabelText('profile artifact profile-artifact-1');
    expect(within(artifact).getByText('run-from-scenario-1-cpu.pprof')).toBeInTheDocument();
    expect(within(artifact).getByText('backend-checkout-smoke')).toBeInTheDocument();
    expect(within(artifact).getByText('checkout-target')).toBeInTheDocument();
    expect(within(artifact).getByText('cpu')).toBeInTheDocument();
    expect(within(artifact).getByText('collected')).toBeInTheDocument();
    expect(
      within(artifact).getByRole('link', { name: 'Download profile artifact run-from-scenario-1-cpu.pprof' }),
    ).toHaveAttribute('href', `${profileArtifactsApiURL}/profile-artifact-1/download`);
    expect(fetchMock).toHaveBeenCalledWith(profileArtifactsApiURL);
  });

  it('loads the latest run report summary on the reports page', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-1',
          scenarioName: 'backend-checkout-smoke',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 9,
          failedRequests: 1,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 8.5,
          p95LatencyMs: 31.2,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [
        {
          id: 'profile-artifact-report-1',
          runId: 'run-report-1',
          scenarioName: 'backend-checkout-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-report-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const report = await screen.findByLabelText('run report run-report-1');
    expect(within(report).getByText('backend-checkout-smoke')).toBeInTheDocument();
    expect(within(report).getByText('finished')).toBeInTheDocument();
    expect(within(report).getByText('success 90.0%')).toBeInTheDocument();
    expect(within(report).getByText('errors 10.0%')).toBeInTheDocument();
    expect(within(report).getByText('events 2')).toBeInTheDocument();
    expect(within(report).getByText('artifacts 1')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`${runReportsApiURL}/run-report-1`);
  });

  it('renders multi-run comparison on the reports page', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-compare-latest',
          name: 'checkout-optimized',
          scenarioName: 'checkout',
          targetName: 'checkout-prod',
          status: 'finished',
          totalRequests: 20,
          successRequests: 18,
          failedRequests: 2,
          durationMs: 160,
          qps: 90,
          averageLatencyMs: 18,
          p95LatencyMs: 80,
          createdAt: '2026-06-16T10:10:00Z',
        },
        {
          id: 'run-compare-regression',
          name: 'checkout-regression',
          scenarioName: 'checkout',
          targetName: 'checkout-prod',
          status: 'finished',
          totalRequests: 10,
          successRequests: 8,
          failedRequests: 2,
          durationMs: 180,
          qps: 70,
          averageLatencyMs: 24,
          p95LatencyMs: 140,
          createdAt: '2026-06-16T10:05:00Z',
        },
        {
          id: 'run-compare-baseline',
          name: 'checkout-baseline',
          scenarioName: 'checkout',
          targetName: 'checkout-prod',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 200,
          qps: 50,
          averageLatencyMs: 20,
          p95LatencyMs: 100,
          createdAt: '2026-06-16T10:00:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const comparison = await screen.findByLabelText('run comparison report');
    expect(within(comparison).getByText('多 Run 对比 / Run comparison')).toBeInTheDocument();
    expect(within(comparison).getByText('best qps run-compare-latest')).toBeInTheDocument();
    expect(within(comparison).getByText('fastest p95 run-compare-latest')).toBeInTheDocument();
    expect(within(comparison).getByText('highest errors run-compare-regression')).toBeInTheDocument();

    const latest = within(comparison).getByLabelText('run comparison run-compare-latest');
    expect(within(latest).getByText('checkout-optimized')).toBeInTheDocument();
    expect(within(latest).getByText('success 90.0%')).toBeInTheDocument();
    expect(within(latest).getByText('errors 10.0%')).toBeInTheDocument();
    expect(within(latest).getByText('qps 90')).toBeInTheDocument();
    expect(within(latest).getByText('p95 80 ms')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`${runReportsCompareApiURL}?limit=5`);
  });

  it('renders process trend comparison across recent runs on the reports page', async () => {
    window.location.hash = '#reports';
    const processKey = 'target-checkout-01|agent-checkout-01|checkout-worker|checkout-worker --config=/etc/checkout/prod.yaml --port=8080';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-process-latest',
          name: 'checkout-process-after-release',
          scenarioName: 'checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120000,
          qps: 50,
          averageLatencyMs: 20,
          p95LatencyMs: 80,
          createdAt: '2026-06-14T10:05:00Z',
        },
        {
          id: 'run-process-older',
          name: 'checkout-process-baseline',
          scenarioName: 'checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120000,
          qps: 50,
          averageLatencyMs: 20,
          p95LatencyMs: 80,
          createdAt: '2026-06-14T10:00:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      {
        generatedAt: '2026-06-14T10:06:00Z',
        summary: {
          runCount: 2,
          processCount: 1,
          highestCpuProcessKey: processKey,
          highestMemoryProcessKey: processKey,
        },
        groups: [
          {
            key: processKey,
            targetId: 'target-checkout-01',
            targetName: 'checkout-target',
            agentId: 'agent-checkout-01',
            name: 'checkout-worker',
            cmdline: 'checkout-worker --config=/etc/checkout/prod.yaml --port=8080',
            runCount: 2,
            sampleCount: 4,
            cpuMaxPercent: 55,
            memoryRssMaxBytes: 300000000,
            fdMaxCount: 96,
            threadMaxCount: 14,
            latestRunId: 'run-process-latest',
            latestPid: 2222,
            latestCpuMaxPercent: 55,
            latestMemoryRssMaxBytes: 300000000,
            previousRunId: 'run-process-older',
            previousPid: 1111,
            previousCpuMaxPercent: 35,
            previousMemoryRssMaxBytes: 200000000,
            cpuDeltaPercent: 20,
            memoryRssDeltaBytes: 100000000,
            latestLastSeenAt: '2026-06-14T10:05:45Z',
            runs: [
              {
                runId: 'run-process-latest',
                runName: 'checkout-process-after-release',
                targetId: 'target-checkout-01',
                targetName: 'checkout-target',
                agentId: 'agent-checkout-01',
                pid: 2222,
                sampleCount: 2,
                cpuMaxPercent: 55,
                memoryRssMaxBytes: 300000000,
                fdMaxCount: 96,
                threadMaxCount: 14,
                firstSeenAt: '2026-06-14T10:05:15Z',
                lastSeenAt: '2026-06-14T10:05:45Z',
                createdAt: '2026-06-14T10:05:00Z',
              },
              {
                runId: 'run-process-older',
                runName: 'checkout-process-baseline',
                targetId: 'target-checkout-01',
                targetName: 'checkout-target',
                agentId: 'agent-checkout-01',
                pid: 1111,
                sampleCount: 2,
                cpuMaxPercent: 35,
                memoryRssMaxBytes: 200000000,
                fdMaxCount: 70,
                threadMaxCount: 10,
                firstSeenAt: '2026-06-14T10:00:15Z',
                lastSeenAt: '2026-06-14T10:00:45Z',
                createdAt: '2026-06-14T10:00:00Z',
              },
            ],
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const comparison = await screen.findByLabelText('process trend comparison report');
    expect(within(comparison).getByText('跨 Run 进程趋势 / Process trend comparison')).toBeInTheDocument();
    expect(within(comparison).getByText('1 processes')).toBeInTheDocument();
    expect(within(comparison).getByText('runs 2')).toBeInTheDocument();
    expect(within(comparison).getByText('highest cpu checkout-worker')).toBeInTheDocument();

    const process = within(comparison).getByLabelText(`process trend comparison ${processKey}`);
    expect(within(process).getByText('checkout-worker')).toBeInTheDocument();
    expect(within(process).getByText('checkout-worker --config=/etc/checkout/prod.yaml --port=8080')).toBeInTheDocument();
    expect(within(process).getByText('latest run-process-latest pid 2222')).toBeInTheDocument();
    expect(within(process).getByText('previous run-process-older pid 1111')).toBeInTheDocument();
    expect(within(process).getByText('CPU max 55.0%')).toBeInTheDocument();
    expect(within(process).getByText('CPU delta +20.0%')).toBeInTheDocument();
    expect(within(process).getByText('RSS max 286.1 MB')).toBeInTheDocument();
    expect(within(process).getByText('RSS delta +95.4 MB')).toBeInTheDocument();
    expect(within(process).getByText('samples 4')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`${processTrendCompareApiURL}?limit=5`);
  });

  it('renders guardrail alerts from the latest run report', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-guardrail-1',
          scenarioName: 'checkout-guardrail-smoke',
          targetName: 'checkout-target',
          status: 'aborted',
          totalRequests: 20,
          successRequests: 1,
          failedRequests: 1,
          durationMs: 60,
          qps: 33.3,
          averageLatencyMs: 18,
          p95LatencyMs: 45,
          maxErrorRatePercent: 10,
          maxP95LatencyMs: 500,
          createdAt: '2026-06-16T10:10:00Z',
          alerts: [
            {
              id: 'run-report-guardrail-1-guardrail-error-rate',
              severity: 'critical',
              kind: 'guardrail',
              metric: 'error_rate_percent',
              threshold: 10,
              observed: 50,
              message: 'Error rate guardrail exceeded: 50.0% > 10.0%',
              eventId: 'run-report-guardrail-1-event-aborted',
              createdAt: '2026-06-16T10:10:01Z',
            },
          ],
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const report = await screen.findByLabelText('run report run-report-guardrail-1');
    expect(within(report).getByText('alerts 1')).toBeInTheDocument();

    const alertPanel = await screen.findByLabelText('run report alerts run-report-guardrail-1');
    expect(within(alertPanel).getByText('告警摘要 / Alert summary')).toBeInTheDocument();
    expect(within(alertPanel).getByText('1 alerts')).toBeInTheDocument();
    const alert = within(alertPanel).getByLabelText('run report alert run-report-guardrail-1-guardrail-error-rate');
    expect(within(alert).getByText('critical')).toBeInTheDocument();
    expect(within(alert).getByText('error_rate_percent')).toBeInTheDocument();
    expect(within(alert).getByText('observed 50.0%')).toBeInTheDocument();
    expect(within(alert).getByText('threshold 10.0%')).toBeInTheDocument();
    expect(within(alert).getByText('Error rate guardrail exceeded: 50.0% > 10.0%')).toBeInTheDocument();
    expect(within(alert).getByText('event run-report-guardrail-1-event-aborted')).toBeInTheDocument();
  });

  it('renders target metric threshold alerts with metric units from the latest run report', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-target-alert-1',
          scenarioName: 'checkout-host-thresholds',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 20,
          successRequests: 20,
          failedRequests: 0,
          durationMs: 60000,
          qps: 33.3,
          averageLatencyMs: 18,
          p95LatencyMs: 45,
          createdAt: '2026-06-16T10:10:00Z',
          alerts: [
            {
              id: 'run-report-target-alert-1-target-cpu-percent',
              severity: 'warning',
              kind: 'target_metric',
              metric: 'target_cpu_percent',
              threshold: 70,
              observed: 72.5,
              message: 'Target CPU threshold exceeded: 72.5% > 70.0%',
              createdAt: '2026-06-16T10:10:01Z',
            },
            {
              id: 'run-report-target-alert-1-target-disk-read',
              severity: 'warning',
              kind: 'target_metric',
              metric: 'target_disk_read_bytes_per_sec',
              threshold: 4096,
              observed: 8192,
              message: 'Target disk read threshold exceeded: 8.0 KB/s > 4.0 KB/s',
              createdAt: '2026-06-16T10:10:01Z',
            },
          ],
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const alertPanel = await screen.findByLabelText('run report alerts run-report-target-alert-1');
    expect(within(alertPanel).getByText('2 alerts')).toBeInTheDocument();
    const cpuAlert = within(alertPanel).getByLabelText('run report alert run-report-target-alert-1-target-cpu-percent');
    expect(within(cpuAlert).getByText('target_metric')).toBeInTheDocument();
    expect(within(cpuAlert).getByText('target_cpu_percent')).toBeInTheDocument();
    expect(within(cpuAlert).getByText('observed 72.5%')).toBeInTheDocument();
    expect(within(cpuAlert).getByText('threshold 70.0%')).toBeInTheDocument();

    const diskAlert = within(alertPanel).getByLabelText('run report alert run-report-target-alert-1-target-disk-read');
    expect(within(diskAlert).getByText('target_metric')).toBeInTheDocument();
    expect(within(diskAlert).getByText('target_disk_read_bytes_per_sec')).toBeInTheDocument();
    expect(within(diskAlert).getByText('observed 8.0 KB/s')).toBeInTheDocument();
    expect(within(diskAlert).getByText('threshold 4.0 KB/s')).toBeInTheDocument();
  });

  it('renders target metrics from the latest run report time window', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-window-metrics-1',
          scenarioName: 'checkout-windowed-metrics',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120000,
          qps: 50,
          averageLatencyMs: 20,
          p95LatencyMs: 80,
          createdAt: '2026-06-14T10:01:00Z',
          targetMetrics: {
            targetId: 'target-checkout-01',
            targetName: 'checkout-target',
            agentIds: ['agent-checkout-01'],
            from: '2026-06-14T10:01:00Z',
            to: '2026-06-14T10:03:00Z',
            sampleCount: 2,
            cpuMaxPercent: 72.5,
            memoryMaxPercent: 65,
            diskReadMaxBytesPerSec: 8192,
            diskWriteMaxBytesPerSec: 4096,
            networkRxMaxBytesPerSec: 16384,
            networkTxMaxBytesPerSec: 32768,
            processMatch: { name: 'checkout', cmdlineContains: '--config=/etc/checkout/prod.yaml' },
            samples: [
              {
                agentId: 'agent-checkout-01',
                collectedAt: '2026-06-14T10:01:30Z',
                cpuUsagePercent: 72.5,
                memoryUsagePercent: 61.25,
                diskReadBytesPerSec: 2048,
                diskWriteBytesPerSec: 1024,
                networkRxBytesPerSec: 4096,
                networkTxBytesPerSec: 8192,
                processes: [],
              },
              {
                agentId: 'agent-checkout-01',
                collectedAt: '2026-06-14T10:02:00Z',
                cpuUsagePercent: 51,
                memoryUsagePercent: 65,
                diskReadBytesPerSec: 8192,
                diskWriteBytesPerSec: 4096,
                networkRxBytesPerSec: 16384,
                networkTxBytesPerSec: 32768,
                processes: [],
              },
            ],
            latestProcessSnapshot: [
              {
                pid: 2233,
                name: 'checkout-worker',
                cmdline: 'checkout-worker --config=/etc/checkout/staging.yaml --port=8081',
                cpuUsagePercent: 12.5,
                memoryRssBytes: 134217728,
                fdCount: 64,
                threadCount: 8,
              },
              {
                pid: 1234,
                name: 'checkout-worker',
                cmdline: 'checkout-worker --config=/etc/checkout/prod.yaml --port=8080',
                cpuUsagePercent: 34.5,
                memoryRssBytes: 268435456,
                fdCount: 88,
                threadCount: 12,
              },
            ],
            processTrends: [
              {
                agentId: 'agent-checkout-01',
                pid: 1234,
                name: 'checkout-worker',
                cmdline: 'checkout-worker --config=/etc/checkout/prod.yaml --port=8080',
                sampleCount: 2,
                cpuMaxPercent: 55,
                memoryRssMaxBytes: 300000000,
                fdMaxCount: 96,
                threadMaxCount: 14,
                firstSeenAt: '2026-06-14T10:01:30Z',
                lastSeenAt: '2026-06-14T10:02:00Z',
              },
            ],
          },
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const windowMetrics = await screen.findByLabelText('run window target metrics run-report-window-metrics-1');
    expect(within(windowMetrics).getByText('压测窗口指标 / Run window metrics')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('samples 2')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('CPU max 72.5%')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('MEM max 65.0%')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('Disk read max 8.0 KB/s')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('Disk write max 4.0 KB/s')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('Net RX max 16.0 KB/s')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('Net TX max 32.0 KB/s')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('agent-checkout-01')).toBeInTheDocument();
    expect(within(windowMetrics).getByLabelText('run window cpu trend')).toBeInTheDocument();
    expect(within(windowMetrics).getByLabelText('run window memory trend')).toBeInTheDocument();
    expect(within(windowMetrics).getByLabelText('run window disk read trend')).toBeInTheDocument();
    expect(within(windowMetrics).getByLabelText('run window network rx trend')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('cmdline --config=/etc/checkout/prod.yaml')).toBeInTheDocument();
    expect(within(windowMetrics).getByText('checkout-worker 34.5%')).toBeInTheDocument();
    expect(within(windowMetrics).queryByText('checkout-worker 12.5%')).not.toBeInTheDocument();
    expect(within(windowMetrics).getByText('进程趋势 / Process trends')).toBeInTheDocument();
    const processTrend = within(windowMetrics).getByLabelText('process trend agent-checkout-01-1234');
    expect(within(processTrend).getByText('checkout-worker pid 1234')).toBeInTheDocument();
    expect(within(processTrend).getByText('process samples 2')).toBeInTheDocument();
    expect(within(processTrend).getByText('CPU max 55.0%')).toBeInTheDocument();
    expect(within(processTrend).getByText('RSS max 286.1 MB')).toBeInTheDocument();
    expect(within(processTrend).getByText('fd max 96')).toBeInTheDocument();
    expect(within(processTrend).getByText('threads max 14')).toBeInTheDocument();
  });

  it('renders target health checks from the latest run report', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-health-1',
          scenarioName: 'checkout-health-smoke',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 8,
          failedRequests: 2,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 22,
          p95LatencyMs: 96,
          createdAt: '2026-06-12T08:30:00Z',
          targetHealthChecks: [
            {
              targetId: 'target-checkout',
              targetName: 'checkout-target',
              status: 'unhealthy',
              url: 'http://checkout.internal:8080/ready',
              expectedStatus: 204,
              observedStatus: 503,
              latencyMs: 8.2,
              error: 'expected HTTP 204, got HTTP 503',
              checkedAt: '2026-06-12T08:29:55Z',
            },
          ],
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const healthPanel = await screen.findByLabelText('run target health checks run-report-health-1');
    expect(within(healthPanel).getByText('目标健康 / Target health')).toBeInTheDocument();
    expect(within(healthPanel).getByText('1 checks')).toBeInTheDocument();
    const healthRow = within(healthPanel).getByLabelText('run target health check target-checkout-2026-06-12T08:29:55Z');
    expect(within(healthRow).getByText('unhealthy 503')).toBeInTheDocument();
    expect(within(healthRow).getByText('expected 204')).toBeInTheDocument();
    expect(within(healthRow).getByText('2026-06-12T08:29:55Z')).toBeInTheDocument();
    expect(within(healthRow).getByText('expected HTTP 204, got HTTP 503')).toBeInTheDocument();
  });

  it('retries failed profile tasks on reports page', async () => {
    window.location.hash = '#reports';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-profile-task-1',
          scenarioName: 'checkout-profile-task-smoke',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 22,
          p95LatencyMs: 96,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskList: [
          {
            id: 'profile-task-retry-1',
            agentId: 'agent-checkout-01',
            runId: 'run-report-profile-task-1',
            targetName: 'checkout-target',
            pprofBaseUrl: 'http://checkout.internal:6060/debug/pprof',
            profileType: 'heap',
            profileSeconds: 5,
            status: 'failed',
            attempts: 3,
            maxAttempts: 3,
            error: 'pprof endpoint timed out',
            updatedAt: '2026-06-12T08:31:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const taskPanel = await screen.findByLabelText('profile task queue');
    expect(fetchMock).toHaveBeenCalledWith(`${profileTasksApiURL}?runId=run-report-profile-task-1&limit=20`);
    expect(within(taskPanel).getByText('画像任务 / Profile tasks')).toBeInTheDocument();
    expect(within(taskPanel).getByText('1 tasks')).toBeInTheDocument();
    const taskRow = within(taskPanel).getByLabelText('profile task profile-task-retry-1');
    expect(within(taskRow).getByText('heap')).toBeInTheDocument();
    expect(within(taskRow).getByText('failed')).toBeInTheDocument();
    expect(within(taskRow).getByText('attempts 3/3')).toBeInTheDocument();
    expect(within(taskRow).getByText('agent-checkout-01')).toBeInTheDocument();
    expect(within(taskRow).getByText('pprof endpoint timed out')).toBeInTheDocument();

    await user.click(within(taskRow).getByRole('button', { name: 'Retry profile task profile-task-retry-1' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${profileTasksApiURL}/profile-task-retry-1/retry`, { method: 'POST' });
    });
    expect(within(taskRow).getByText('pending')).toBeInTheDocument();
    expect(within(taskRow).getByText('attempts 0/3')).toBeInTheDocument();
    expect(within(taskRow).queryByText('pprof endpoint timed out')).not.toBeInTheDocument();
  });

  it('renders leased profile task lease expiry on reports page', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-profile-task-lease-1',
          scenarioName: 'checkout-profile-task-lease',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          status: 'running',
          totalRequests: 10,
          successRequests: 8,
          failedRequests: 0,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 22,
          p95LatencyMs: 96,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskList: [
          {
            id: 'profile-task-leased-1',
            agentId: 'agent-checkout-01',
            runId: 'run-report-profile-task-lease-1',
            targetName: 'checkout-target',
            pprofBaseUrl: 'http://checkout.internal:6060/debug/pprof',
            profileType: 'cpu',
            profileSeconds: 10,
            status: 'leased',
            attempts: 1,
            maxAttempts: 3,
            source: 'run_auto',
            leasedAt: '2026-06-12T08:31:00Z',
            leaseExpiresAt: '2026-06-12T08:33:00Z',
            updatedAt: '2026-06-12T08:31:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const taskPanel = await screen.findByLabelText('profile task queue');
    const taskRow = within(taskPanel).getByLabelText('profile task profile-task-leased-1');
    expect(within(taskRow).getByText('leased')).toBeInTheDocument();
    expect(within(taskRow).getByText('source run_auto')).toBeInTheDocument();
    expect(within(taskRow).getByText('lease expires 2026-06-12T08:33:00Z')).toBeInTheDocument();
  });

  it('renders command profiler task details on reports page', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-command-profile-task-1',
          scenarioName: 'checkout-command-profile-task',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          name: 'checkout-command-profile-task',
          status: 'finished',
          method: 'GET',
          url: 'http://checkout.internal:8080/api/health',
          totalRequests: 100,
          successRequests: 100,
          failedRequests: 0,
          durationMs: 1000,
          qps: 100,
          averageLatencyMs: 5,
          p95LatencyMs: 8,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskList: [
          {
            id: 'profile-task-command-1',
            agentId: 'agent-checkout-01',
            runId: 'run-report-command-profile-task-1',
            targetName: 'checkout-target',
            profileType: 'perf',
            profileSeconds: 15,
            profileCommand: 'perf',
            profileCommandArgs: ['record', '-F', '99', '-p', '1234', '-g', '-o', '{{output}}', '--', 'sleep', '{{seconds}}'],
            status: 'pending',
            attempts: 0,
            maxAttempts: 3,
            source: 'target_manual',
            createdAt: '2026-06-12T08:31:00Z',
            updatedAt: '2026-06-12T08:31:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const taskPanel = await screen.findByLabelText('profile task queue');
    const taskRow = within(taskPanel).getByLabelText('profile task profile-task-command-1');
    expect(within(taskRow).getByText('perf')).toBeInTheDocument();
    expect(within(taskRow).getByText('perf record -F 99 -p 1234 -g -o {{output}} -- sleep {{seconds}}')).toBeInTheDocument();
  });

  it('filters report profile tasks by source', async () => {
    window.location.hash = '#reports';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-profile-task-source-1',
          scenarioName: 'checkout-profile-task-source',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 22,
          p95LatencyMs: 96,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskList: [
          {
            id: 'profile-task-target-manual-1',
            agentId: 'agent-checkout-01',
            runId: 'run-report-profile-task-source-1',
            targetName: 'checkout-target',
            pprofBaseUrl: 'http://checkout.internal:6060/debug/pprof',
            profileType: 'heap',
            profileSeconds: 30,
            status: 'pending',
            attempts: 0,
            maxAttempts: 3,
            source: 'target_manual',
            updatedAt: '2026-06-12T08:31:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await screen.findByLabelText('profile task queue');
    await user.selectOptions(screen.getByLabelText('Profile task source filter'), 'target_manual');

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${profileTasksApiURL}?runId=run-report-profile-task-source-1&source=target_manual&limit=20`);
    });
  });

  it('filters report profile tasks by threshold auto source', async () => {
    window.location.hash = '#reports';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-profile-task-threshold-source-1',
          scenarioName: 'checkout-profile-task-threshold-source',
          targetId: 'target-checkout',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 120,
          qps: 83.3,
          averageLatencyMs: 22,
          p95LatencyMs: 96,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskList: [
          {
            id: 'profile-task-threshold-auto-1',
            agentId: 'agent-checkout-01',
            runId: 'run-report-profile-task-threshold-source-1',
            targetName: 'checkout-target',
            pprofBaseUrl: 'http://checkout.internal:6060/debug/pprof',
            profileType: 'cpu',
            profileSeconds: 1,
            status: 'pending',
            attempts: 0,
            maxAttempts: 3,
            source: 'threshold_auto',
            updatedAt: '2026-06-12T08:31:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await screen.findByLabelText('profile task queue');
    await user.selectOptions(screen.getByLabelText('Profile task source filter'), 'threshold_auto');

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${profileTasksApiURL}?runId=run-report-profile-task-threshold-source-1&source=threshold_auto&limit=20`);
    });
  });

  it('renders profile artifacts from the latest run report when the artifact list is empty', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-artifact-1',
          scenarioName: 'report-artifact-smoke',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 4,
          successRequests: 4,
          failedRequests: 0,
          durationMs: 80,
          qps: 50,
          averageLatencyMs: 12,
          p95LatencyMs: 40,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [
        {
          id: 'profile-artifact-from-report-1',
          runId: 'run-report-artifact-1',
          scenarioName: 'report-artifact-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-report-artifact-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
      [],
      [],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('run report run-report-artifact-1')).toBeInTheDocument();
    const artifact = await screen.findByLabelText('profile artifact profile-artifact-from-report-1');
    expect(within(artifact).getByText('run-report-artifact-1-cpu.pprof')).toBeInTheDocument();
  });

  it('renders profile artifact comparison groups in reports', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-artifact-latest',
          scenarioName: 'checkout-latest',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 100,
          qps: 100,
          averageLatencyMs: 10,
          p95LatencyMs: 20,
          createdAt: '2026-06-16T11:00:00Z',
        },
        {
          id: 'run-artifact-middle',
          scenarioName: 'checkout-middle',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 10,
          successRequests: 10,
          failedRequests: 0,
          durationMs: 100,
          qps: 90,
          averageLatencyMs: 11,
          p95LatencyMs: 21,
          createdAt: '2026-06-16T10:00:00Z',
        },
      ],
      [],
      [],
      [],
      [],
      [],
      {
        generatedAt: '2026-06-16T11:05:00Z',
        summary: {
          runCount: 2,
          artifactCount: 4,
          collectedCount: 3,
          failedCount: 1,
          totalSizeBytes: 56,
        },
        groups: [
          {
            profileType: 'cpu',
            artifactCount: 2,
            collectedCount: 2,
            failedCount: 0,
            totalSizeBytes: 48,
            latestRunId: 'run-artifact-latest',
            latestArtifactId: 'profile-latest-cpu',
            latestSizeBytes: 32,
            previousSizeBytes: 16,
            sizeDeltaBytes: 16,
            sizeDeltaPercent: 100,
            latestStatus: 'collected',
            previousStatus: 'collected',
            statusChanged: false,
            artifacts: [
              {
                id: 'profile-latest-cpu',
                runId: 'run-artifact-latest',
                scenarioName: 'checkout-latest',
                targetName: 'checkout-target',
                profileType: 'cpu',
                status: 'collected',
                fileName: 'latest-cpu.pprof',
                sizeBytes: 32,
                startedAt: '2026-06-16T11:00:00Z',
                finishedAt: '2026-06-16T11:00:01Z',
              },
              {
                id: 'profile-middle-cpu',
                runId: 'run-artifact-middle',
                scenarioName: 'checkout-middle',
                targetName: 'checkout-target',
                profileType: 'cpu',
                status: 'collected',
                fileName: 'middle-cpu.pprof',
                sizeBytes: 16,
                startedAt: '2026-06-16T10:00:00Z',
                finishedAt: '2026-06-16T10:00:01Z',
              },
            ],
          },
          {
            profileType: 'heap',
            artifactCount: 2,
            collectedCount: 1,
            failedCount: 1,
            totalSizeBytes: 8,
            latestRunId: 'run-artifact-latest',
            latestArtifactId: 'profile-latest-heap',
            latestSizeBytes: 0,
            previousSizeBytes: 8,
            sizeDeltaBytes: -8,
            sizeDeltaPercent: -100,
            latestStatus: 'failed',
            previousStatus: 'collected',
            statusChanged: true,
            artifacts: [
              {
                id: 'profile-latest-heap',
                runId: 'run-artifact-latest',
                scenarioName: 'checkout-latest',
                targetName: 'checkout-target',
                profileType: 'heap',
                status: 'failed',
                fileName: 'latest-heap.pprof',
                sizeBytes: 0,
                error: 'heap endpoint unavailable',
                startedAt: '2026-06-16T11:00:00Z',
                finishedAt: '2026-06-16T11:00:01Z',
              },
              {
                id: 'profile-middle-heap',
                runId: 'run-artifact-middle',
                scenarioName: 'checkout-middle',
                targetName: 'checkout-target',
                profileType: 'heap',
                status: 'collected',
                fileName: 'middle-heap.pprof',
                sizeBytes: 8,
                startedAt: '2026-06-16T10:00:00Z',
                finishedAt: '2026-06-16T10:00:01Z',
              },
            ],
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const comparison = await screen.findByLabelText('profile artifact comparison report');
    expect(within(comparison).getByText('画像对比 / Profile artifact comparison')).toBeInTheDocument();
    expect(within(comparison).getByText('2 runs')).toBeInTheDocument();
    const summary = within(comparison).getByLabelText('profile artifact comparison summary');
    expect(within(summary).getByText('artifacts 4')).toBeInTheDocument();
    expect(within(summary).getByText('collected 3')).toBeInTheDocument();
    expect(within(summary).getByText('failed 1')).toBeInTheDocument();

    const cpuGroup = within(comparison).getByLabelText('profile artifact comparison cpu');
    expect(within(cpuGroup).getByText('cpu')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('2 artifacts')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('size 48 B')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('delta +16 B')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('change +100.0%')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('status collected')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('latest run-artifact-latest')).toBeInTheDocument();
    expect(within(cpuGroup).getByText('latest-cpu.pprof')).toBeInTheDocument();

    const heapGroup = within(comparison).getByLabelText('profile artifact comparison heap');
    expect(within(heapGroup).getByText('heap')).toBeInTheDocument();
    expect(within(heapGroup).getByText('2 artifacts')).toBeInTheDocument();
    expect(within(heapGroup).getByText('size 8 B')).toBeInTheDocument();
    expect(within(heapGroup).getByText('delta -8 B')).toBeInTheDocument();
    expect(within(heapGroup).getByText('change -100.0%')).toBeInTheDocument();
    expect(within(heapGroup).getByText('status collected -> failed')).toBeInTheDocument();
    expect(within(heapGroup).getByText('failed 1')).toBeInTheDocument();
    expect(within(heapGroup).getByText('heap endpoint unavailable')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`${profileArtifactCompareApiURL}?limit=5`);
  });

  it('renders slow and error samples from the latest run report', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-report-samples-1',
          scenarioName: 'sampled-checkout-smoke',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 3,
          successRequests: 2,
          failedRequests: 1,
          durationMs: 120,
          qps: 25,
          averageLatencyMs: 30,
          p95LatencyMs: 248.7,
          createdAt: '2026-06-12T08:30:00Z',
          slowSamples: [
            {
              id: 'slow-sample-1',
              runId: 'run-report-samples-1',
              kind: 'slow',
              method: 'POST',
              url: 'http://checkout.internal/api/orders',
              statusCode: 200,
              success: true,
              latencyMs: 248.7,
              createdAt: '2026-06-12T08:30:01Z',
            },
          ],
          errorSamples: [
            {
              id: 'error-sample-1',
              runId: 'run-report-samples-1',
              kind: 'error',
              method: 'POST',
              url: 'http://checkout.internal/api/orders',
              statusCode: 503,
              success: false,
              latencyMs: 19.3,
              error: 'HTTP 503',
              createdAt: '2026-06-12T08:30:02Z',
            },
          ],
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const samplesPanel = await screen.findByLabelText('request samples run-report-samples-1');
    expect(within(samplesPanel).getByText('请求样本 / Request samples')).toBeInTheDocument();
    expect(within(samplesPanel).getByText('慢请求样本 / Slow samples')).toBeInTheDocument();
    expect(within(samplesPanel).getByText('错误样本 / Error samples')).toBeInTheDocument();

    const slowSample = within(samplesPanel).getByLabelText('slow sample slow-sample-1');
    expect(within(slowSample).getByText('POST')).toBeInTheDocument();
    expect(within(slowSample).getByText('http://checkout.internal/api/orders')).toBeInTheDocument();
    expect(within(slowSample).getByText('248.70 ms')).toBeInTheDocument();

    const errorSample = within(samplesPanel).getByLabelText('error sample error-sample-1');
    expect(within(errorSample).getByText('503')).toBeInTheDocument();
    expect(within(errorSample).getByText('HTTP 503')).toBeInTheDocument();
    expect(within(errorSample).getByText('19.30 ms')).toBeInTheDocument();
  });

  it('loads the latest run report through XMLHttpRequest when fetch is unavailable', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-xhr-report-1',
          scenarioName: 'xhr-checkout-smoke',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 4,
          successRequests: 3,
          failedRequests: 1,
          durationMs: 80,
          qps: 50,
          averageLatencyMs: 12,
          p95LatencyMs: 40,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [
        {
          id: 'profile-artifact-xhr-report-1',
          runId: 'run-xhr-report-1',
          scenarioName: 'xhr-checkout-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-xhr-report-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
    );
    vi.stubGlobal('fetch', undefined);
    vi.stubGlobal('XMLHttpRequest', createMockXMLHttpRequest(fetchMock));

    render(<App />);

    const report = await screen.findByLabelText('run report run-xhr-report-1');
    expect(within(report).getByText('xhr-checkout-smoke')).toBeInTheDocument();
    expect(within(report).getByText('success 75.0%')).toBeInTheDocument();
    expect(within(report).getByText('errors 25.0%')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      `${runReportsApiURL}/run-xhr-report-1`,
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('loads the latest run report through JSONP when fetch and XMLHttpRequest are unavailable', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-jsonp-report-1',
          scenarioName: 'jsonp-checkout-smoke',
          targetName: 'checkout-target',
          status: 'finished',
          totalRequests: 5,
          successRequests: 4,
          failedRequests: 1,
          durationMs: 90,
          qps: 55.5,
          averageLatencyMs: 12,
          p95LatencyMs: 44,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
      [],
      [],
      [
        {
          id: 'profile-artifact-jsonp-report-1',
          runId: 'run-jsonp-report-1',
          scenarioName: 'jsonp-checkout-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-jsonp-report-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
    );
    vi.stubGlobal('fetch', undefined);
    vi.stubGlobal('XMLHttpRequest', undefined);
    const URLParser = URL;
    vi.stubGlobal('URL', undefined);
    const appendScript = installJSONPScriptMock(fetchMock, URLParser);

    render(<App />);

    const report = await screen.findByLabelText('run report run-jsonp-report-1');
    expect(within(report).getByText('jsonp-checkout-smoke')).toBeInTheDocument();
    expect(within(report).getByText('success 80.0%')).toBeInTheDocument();
    expect(within(report).getByText('errors 20.0%')).toBeInTheDocument();
    expect(appendScript).toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledWith(`${runReportsApiURL}/run-jsonp-report-1`);
  });

  it('loads the latest run report from bootstrap data when browser request APIs are unavailable', async () => {
    window.location.hash = '#reports';
    vi.stubGlobal('fetch', undefined);
    vi.stubGlobal('XMLHttpRequest', undefined);
    vi.stubGlobal('URL', undefined);
    vi.stubGlobal('__AIT_LATEST_RUN_REPORT__', {
      run: {
        id: 'run-bootstrap-report-1',
        scenarioName: 'bootstrap-checkout-smoke',
        targetName: 'checkout-target',
        name: 'bootstrap-checkout-smoke',
        status: 'finished',
        method: 'GET',
        url: 'http://127.0.0.1:8080/api/health',
        totalRequests: 8,
        successRequests: 7,
        failedRequests: 1,
        durationMs: 120,
        qps: 66.6,
        averageLatencyMs: 12,
        p95LatencyMs: 35,
        createdAt: '2026-06-12T08:30:00Z',
      },
      events: [],
      profileArtifacts: [
        {
          id: 'profile-artifact-bootstrap-report-1',
          runId: 'run-bootstrap-report-1',
          scenarioName: 'bootstrap-checkout-smoke',
          targetName: 'checkout-target',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'run-bootstrap-report-1-cpu.pprof',
          sizeBytes: 15,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
      summary: {
        successRatePercent: 87.5,
        errorRatePercent: 12.5,
        eventCount: 0,
        profileArtifactCount: 1,
      },
    });

    render(<App />);

    const report = await screen.findByLabelText('run report run-bootstrap-report-1');
    expect(within(report).getByText('bootstrap-checkout-smoke')).toBeInTheDocument();
    expect(within(report).getByText('success 87.5%')).toBeInTheDocument();
    expect(within(report).getByText('errors 12.5%')).toBeInTheDocument();
    expect(await screen.findByLabelText('profile artifact profile-artifact-bootstrap-report-1')).toBeInTheDocument();
  });

  it('keeps profile artifact status and download controls grouped inside each report row', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [],
      [],
      [
        {
          id: 'profile-artifact-long',
          runId: 'run-long-artifact-1',
          scenarioName: 'checkout-profile-with-a-very-long-scenario-name',
          targetName: 'checkout-target-with-a-very-long-name',
          profileType: 'cpu',
          status: 'collected',
          fileName: 'checkout-profile-with-a-very-long-file-name-cpu.pprof',
          sizeBytes: 300000000,
          startedAt: '2026-06-12T08:30:00Z',
          finishedAt: '2026-06-12T08:30:01Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const artifact = await screen.findByLabelText('profile artifact profile-artifact-long');
    const actions = within(artifact).getByLabelText('profile artifact actions profile-artifact-long');
    expect(actions).toHaveClass('artifact-actions');
    expect(within(actions).getByText('collected')).toBeInTheDocument();
    expect(
      within(actions).getByRole('link', {
        name: 'Download profile artifact checkout-profile-with-a-very-long-file-name-cpu.pprof',
      }),
    ).toHaveAttribute('href', `${profileArtifactsApiURL}/profile-artifact-long/download`);
  });

  it('keeps run history status grouped inside each run history row', async () => {
    window.location.hash = '#runs';
    const fetchMock = createScenarioFetchMock(
      [],
      [
        {
          id: 'run-overflow-1',
          scenarioName: 'checkout-history-with-a-long-name',
          targetName: 'checkout-target-with-a-long-name',
          status: 'finished',
          totalRequests: 3,
          successRequests: 3,
          failedRequests: 0,
          durationMs: 120,
          qps: 25.14,
          averageLatencyMs: 12.5,
          p95LatencyMs: 118.04,
          createdAt: '2026-06-12T08:30:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const row = await screen.findByLabelText('run history run-overflow-1');
    const status = within(row).getByLabelText('run history status run-overflow-1');
    expect(status).toHaveClass('run-history-status');
    expect(within(status).getByText('finished')).toBeInTheDocument();
  });

  it('loads agent metric timeline on the reports page', async () => {
    window.location.hash = '#reports';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: { service: 'checkout' },
          capabilities: ['host_metrics', 'process_metrics'],
          lastSeenAt: '2026-06-14T10:02:00Z',
        },
      ],
      [],
      [],
      [
        {
          agentId: 'agent-checkout-01',
          collectedAt: '2026-06-14T10:01:00Z',
          cpuUsagePercent: 42.5,
          memoryUsagePercent: 61.25,
          diskReadBytesPerSec: 1024,
          diskWriteBytesPerSec: 2048,
          networkRxBytesPerSec: 4096,
          networkTxBytesPerSec: 8192,
          processes: [
            {
              pid: 1234,
              name: 'checkout',
              cpuUsagePercent: 34.5,
              memoryRssBytes: 268435456,
              fdCount: 88,
              threadCount: 12,
            },
          ],
        },
        {
          agentId: 'agent-checkout-01',
          collectedAt: '2026-06-14T10:02:00Z',
          cpuUsagePercent: 82,
          memoryUsagePercent: 68.5,
          diskReadBytesPerSec: 2048,
          diskWriteBytesPerSec: 4096,
          networkRxBytesPerSec: 8192,
          networkTxBytesPerSec: 16384,
          processes: [
            {
              pid: 1234,
              name: 'checkout',
              cpuUsagePercent: 55,
              memoryRssBytes: 300000000,
              fdCount: 91,
              threadCount: 14,
            },
          ],
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const timeline = await screen.findByLabelText('agent metrics timeline agent-checkout-01');
    expect(within(timeline).getByText('checkout-01')).toBeInTheDocument();
    expect(within(timeline).getByText('CPU max 82.0%')).toBeInTheDocument();
    expect(within(timeline).getByText('MEM max 68.5%')).toBeInTheDocument();
    expect(within(timeline).getByLabelText('agent cpu trend')).toBeInTheDocument();
    expect(within(timeline).getByLabelText('agent memory trend')).toBeInTheDocument();
    expect(within(timeline).getByText('checkout 55.0%')).toBeInTheDocument();
    expect(within(timeline).getByText('RSS 286.1 MB')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(agentsApiURL);
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining(`${agentMetricsApiURL}?agentId=agent-checkout-01`));
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
    expect(screen.getByRole('option', { name: 'Custom RPC' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    fireEvent.mouseDown(screen.getByLabelText(/Method/));
    expect(screen.getByRole('option', { name: 'POST' })).toBeInTheDocument();
    await user.keyboard('{Escape}');

    fireEvent.change(screen.getByLabelText(/Scenario name/), { target: { value: 'payment-http-smoke' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), { target: { value: 'https://api.example.test' } });
    fireEvent.change(screen.getByLabelText(/Request path/), { target: { value: '/api/payments' } });
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
    fireEvent.change(screen.getByLabelText('Header key 1'), { target: { value: 'Authorization' } });
    fireEvent.change(screen.getByLabelText('Header value 1'), { target: { value: 'Bearer ${token}' } });
    await user.click(screen.getByRole('button', { name: /Add header/ }));
    fireEvent.change(screen.getByLabelText('Header key 2'), { target: { value: 'Content-Type' } });
    fireEvent.change(screen.getByLabelText('Header value 2'), { target: { value: 'application/json' } });
    expect(screen.getByLabelText('Body JSON variant 1')).toBeInTheDocument();
    expect(screen.getByLabelText('Body weight 1')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Body JSON variant 1'), {
      target: { value: '{ "amount": 100, "currency": "CNY" }' },
    });
    fireEvent.change(screen.getByLabelText('Body weight 1'), {
      target: { value: '70' },
    });
    await user.click(screen.getByRole('button', { name: /Add body variant/ }));
    fireEvent.change(screen.getByLabelText('Body JSON variant 2'), {
      target: { value: '{ "amount": 300, "currency": "CNY" }' },
    });
    fireEvent.change(screen.getByLabelText('Body weight 2'), {
      target: { value: '30' },
    });
    fireEvent.change(screen.getByLabelText(/Timeout/), { target: { value: '2500' } });
    fireEvent.change(screen.getByLabelText(/Retry count/), { target: { value: '2' } });
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
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
      [],
      [],
      [
        {
          id: 'target-checkout',
          name: 'checkout-target',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'test',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: { name: 'checkout', cmdlineContains: '' },
        },
      ],
    );
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

  it('creates a scenario with project and environment metadata', async () => {
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    await user.type(screen.getByLabelText(/Scenario name/), 'checkout-staging-smoke');
    await user.clear(screen.getByLabelText(/Project ID/));
    await user.type(screen.getByLabelText(/Project ID/), 'project-checkout');
    await user.clear(screen.getByLabelText(/Environment/));
    await user.type(screen.getByLabelText(/Environment/), 'staging');
    await user.click(screen.getByRole('button', { name: /Create/ }));

    const createCall = await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([url, init]) => String(url) === scenariosApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
      );
      expect(call).toBeDefined();
      return call;
    });
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'checkout-staging-smoke',
        projectId: 'project-checkout',
        environment: 'staging',
      }),
    );

    const createdScenario = await screen.findByLabelText('scenario checkout-staging-smoke');
    expect(within(createdScenario).getByText('project project-checkout')).toBeInTheDocument();
    expect(within(createdScenario).getByText('env staging')).toBeInTheDocument();
  });

  it('creates a custom RPC scenario through the backend API', async () => {
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);
    render(<App />);

    await user.click(screen.getByRole('button', { name: /New scenario/ }));
    fireEvent.change(screen.getByLabelText(/Protocol/), { target: { value: 'CUSTOM_RPC' } });
    fireEvent.change(screen.getByLabelText(/Scenario name/), { target: { value: 'order-custom-rpc' } });
    fireEvent.change(await screen.findByLabelText(/Adapter URL/), { target: { value: 'http://127.0.0.1:9090' } });
    fireEvent.change(screen.getByLabelText(/Adapter path/), { target: { value: '/invoke' } });
    fireEvent.change(screen.getByLabelText(/RPC method/), {
      target: { value: 'checkout.OrderService/CreateOrder' },
    });
    fireEvent.change(screen.getByLabelText('Body JSON variant 1'), {
      target: { value: '{ "sku": "book" }' },
    });
    await user.click(screen.getByRole('button', { name: /Create/ }));

    const createCall = await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([url, init]) => String(url) === scenariosApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
      );
      expect(call).toBeDefined();
      return call;
    });
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'order-custom-rpc',
        protocol: 'CUSTOM_RPC',
        method: 'checkout.OrderService/CreateOrder',
        baseUrl: 'http://127.0.0.1:9090',
        path: '/invoke',
        bodyVariants: [
          expect.objectContaining({
            weight: '100',
            body: '{ "sku": "book" }',
          }),
        ],
      }),
    );
    const createdScenario = await screen.findByLabelText('scenario order-custom-rpc');
    expect(within(createdScenario).getByText('CUSTOM_RPC')).toBeInTheDocument();
    expect(within(createdScenario).getByText('checkout.OrderService/CreateOrder http://127.0.0.1:9090/invoke')).toBeInTheDocument();
  });

  it('starts a load run from a saved scenario', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
      [],
      [],
      [
        {
          id: 'target-checkout',
          name: 'checkout-target',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'test',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: { name: 'checkout', cmdlineContains: '' },
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    expect(await screen.findByLabelText('selected run target checkout-target')).toBeInTheDocument();
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
        targetId: 'target-checkout',
        totalRequests: 6,
        concurrency: 2,
      }),
    );
    const runResult = await screen.findByLabelText('run result run-from-scenario-1');
    expect(within(runResult).getByText('run-from-scenario-1')).toBeInTheDocument();
    expect(within(runResult).getByText('running')).toBeInTheDocument();
    expect(within(runResult).getByText('checkout-target')).toBeInTheDocument();
    expect(within(runResult).getByText('0 / 6')).toBeInTheDocument();

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${runsApiURL}/run-from-scenario-1`);
    });
    await waitFor(() => {
      expect(within(runResult).getByText('running')).toBeInTheDocument();
      expect(within(runResult).getByText('3 / 6')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(within(runResult).getByText('finished')).toBeInTheDocument();
      expect(within(runResult).getByText('6 / 6')).toBeInTheDocument();
    });
  });

  it('shows the target health preflight failure when starting a run', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
      [],
      [],
      [
        {
          id: 'target-checkout',
          name: 'checkout-target',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'test',
          agentIds: ['agent-checkout-01'],
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 200,
            timeoutMs: 1000,
          },
        },
      ],
      [],
      [],
      [],
      null,
      null,
      {
        runCreateFailure: {
          status: 409,
          body: {
            error: 'target health preflight failed: expected HTTP 200, got HTTP 503',
          },
        },
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    expect(await screen.findByLabelText('selected run target checkout-target')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    expect(
      await screen.findByText('target health preflight failed: expected HTTP 200, got HTTP 503'),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('run result run-from-scenario-1')).not.toBeInTheDocument();
  });

  it('shows executable flow steps for the selected run scenario', async () => {
    window.location.hash = '#runs';
    const fetchMock = createScenarioFetchMock(
      [
        {
          id: 'scenario-flow-api',
          name: 'backend-checkout-flow',
          protocol: 'HTTP',
          method: 'GET',
          baseUrl: 'http://127.0.0.1:8080',
          path: '/api/health',
          queryVariants: [],
          headers: [],
          bodyVariants: [],
          flowSteps: [
            {
              id: 'login',
              name: 'HTTP login',
              type: 'request',
              enabled: true,
              protocol: 'HTTP',
              method: 'GET',
              path: '/login',
            },
            {
              id: 'checkout',
              name: 'HTTP checkout',
              type: 'request',
              enabled: true,
              protocol: 'HTTP',
              method: 'POST',
              path: '/checkout',
            },
            {
              id: 'assert-status',
              name: 'assert status',
              type: 'assertion',
              assertion: 'status < 400',
            },
          ],
          timeoutMs: '1000',
          retryCount: '0',
          assertion: 'status < 400',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const selectedScenario = await screen.findByLabelText('selected run scenario backend-checkout-flow');
    expect(within(selectedScenario).getByText('可执行链路 / Executable flow')).toBeInTheDocument();
    expect(within(selectedScenario).getByText('01 GET /login')).toBeInTheDocument();
    expect(within(selectedScenario).getByText('02 POST /checkout')).toBeInTheDocument();
    expect(within(selectedScenario).queryByText(/assert status/)).not.toBeInTheDocument();
  });

  it('stops a running backend run from the runs page', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
      [],
      [],
      [
        {
          id: 'target-checkout',
          name: 'checkout-target',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'test',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: { name: 'checkout', cmdlineContains: '' },
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Start run/ }));
    const runResult = await screen.findByLabelText('run result run-from-scenario-1');
    expect(within(runResult).getByText('running')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Stop/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${runsApiURL}/run-from-scenario-1/stop`,
        expect.objectContaining({ method: 'POST' }),
      );
    });
    expect(within(runResult).getByText('canceled')).toBeInTheDocument();
  });

  it('shows run events from the backend on the runs page', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    const eventLog = await screen.findByLabelText('run events run-from-scenario-1');
    expect(within(eventLog).getByText('事件流 / Event stream')).toBeInTheDocument();
    expect(within(eventLog).getByText('run_started')).toBeInTheDocument();
    expect(within(eventLog).getByText('run_progress')).toBeInTheDocument();
    expect(within(eventLog).getByText('10 / 20')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`${runsApiURL}/run-from-scenario-1/events`);
  });

  it('subscribes to run events with SSE when EventSource is available', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    const eventSources: MockRunEventSource[] = [];
    class MockRunEventSource {
      url: string;
      close = vi.fn();
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

      constructor(url: string) {
        this.url = url;
        eventSources.push(this);
      }

      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
      }

      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        this.listeners.set(type, (this.listeners.get(type) ?? []).filter((candidate) => candidate !== listener));
      }

      emit(type: string, data: Record<string, unknown>) {
        for (const listener of this.listeners.get(type) ?? []) {
          listener({ data: JSON.stringify(data) } as MessageEvent);
        }
      }
    }
    vi.stubGlobal('EventSource', MockRunEventSource);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    await waitFor(() => {
      expect(eventSources).toHaveLength(1);
    });
    expect(eventSources[0].url).toBe(`${runsApiURL}/run-from-scenario-1/events/stream`);
    expect(fetchMock).not.toHaveBeenCalledWith(`${runsApiURL}/run-from-scenario-1/events`);

    eventSources[0].emit('run_started', {
      id: 'run-from-scenario-1-event-start',
      runId: 'run-from-scenario-1',
      type: 'run_started',
      status: 'running',
      message: 'Run started',
      successRequests: 0,
      failedRequests: 0,
      totalRequests: 20,
      qps: 0,
      p95LatencyMs: 0,
      createdAt: '2026-06-12T08:30:00Z',
    });

    const eventLog = await screen.findByLabelText('run events run-from-scenario-1');
    expect(within(eventLog).getByText('run_started')).toBeInTheDocument();
  });

  it('prefers backend scenarios over stale local drafts when starting a run', async () => {
    window.location.hash = '#runs';
    window.localStorage.setItem(
      'all-in-one-testing.scenario-drafts.v1',
      JSON.stringify([
        {
          id: 'stale-local-scenario',
          name: 'stale-local-scenario',
          protocol: 'HTTP',
          method: 'GET',
          baseUrl: 'http://127.0.0.1:9999',
          path: '/stale',
          queryVariants: [],
          headers: [],
          bodyVariants: [],
          flowSteps: [],
          timeoutMs: '1000',
          retryCount: '0',
          assertion: 'status < 400',
        },
      ]),
    );
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
        {
          id: 'backend-scenario-1',
          name: 'backend-health-check',
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
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-health-check')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        runsApiURL,
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"backend-scenario-1"'),
        }),
      );
    });
  });

  it('passes guardrail thresholds when starting a backend run', async () => {
    window.location.hash = '#runs';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('selected run scenario backend-checkout-smoke')).toBeInTheDocument();
    await user.clear(screen.getByLabelText('Max error rate percent'));
    await user.type(screen.getByLabelText('Max error rate percent'), '2.5');
    await user.clear(screen.getByLabelText('Max p95 latency ms'));
    await user.type(screen.getByLabelText('Max p95 latency ms'), '300');
    await user.click(screen.getByRole('button', { name: /Start run/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        runsApiURL,
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"maxErrorRatePercent":2.5'),
        }),
      );
      expect(fetchMock).toHaveBeenCalledWith(
        runsApiURL,
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"maxP95LatencyMs":300'),
        }),
      );
    });
  });

  it('loads run history from the backend API', async () => {
    window.location.hash = '#runs';
    const fetchMock = createScenarioFetchMock(
      [
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
      ],
      [
        {
          id: 'run-history-1',
          scenarioId: 'scenario-from-api',
          scenarioName: 'backend-checkout-smoke',
          name: 'backend-checkout-smoke',
          status: 'finished',
          method: 'GET',
          url: 'http://127.0.0.1:8080/api/health',
          totalRequests: 9,
          successRequests: 9,
          failedRequests: 0,
          durationMs: 16.2,
          qps: 555.56,
          averageLatencyMs: 1.7,
          p95LatencyMs: 2.9,
          createdAt: '2026-06-12T08:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('run history run-history-1')).toBeInTheDocument();
    expect(screen.getByText('run-history-1')).toBeInTheDocument();
    expect(screen.getByText('9 / 9')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(runsApiURL);
  });

  it('loads agents from the backend API', async () => {
    window.location.hash = '#targets';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          latestMetrics: {
            agentId: 'agent-checkout-01',
            collectedAt: '2026-06-12T09:21:00Z',
            cpuUsagePercent: 72.5,
            memoryUsagePercent: 61.25,
            diskReadBytesPerSec: 1048576,
            diskWriteBytesPerSec: 2097152,
            networkRxBytesPerSec: 32768,
            networkTxBytesPerSec: 65536,
            processes: [
              {
                pid: 1234,
                name: 'checkout',
                cpuUsagePercent: 34.5,
                memoryRssBytes: 268435456,
                fdCount: 88,
                threadCount: 12,
              },
            ],
          },
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const agent = await screen.findByLabelText('agent checkout-01');
    expect(within(agent).getByText('checkout-01')).toBeInTheDocument();
    expect(within(agent).getByText('10.0.0.13')).toBeInTheDocument();
    expect(within(agent).getByText('0.2.0')).toBeInTheDocument();
    expect(within(agent).getByText('project-checkout')).toBeInTheDocument();
    expect(within(agent).getByText('staging')).toBeInTheDocument();
    expect(within(agent).getByText('online')).toBeInTheDocument();
    expect(within(agent).getByText('CPU 72.5%')).toBeInTheDocument();
    expect(within(agent).getByText('MEM 61.3%')).toBeInTheDocument();
    expect(within(agent).getByText('checkout 34.5%')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(agentsApiURL);
  });

  it('shows agent upgrade status when a newer release is available', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText,
      },
    });
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: 'v0.3.0',
          latestVersion: 'v0.4.0',
          upgradeAvailable: true,
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const agent = await screen.findByLabelText('agent checkout-01');
    expect(within(agent).getByText('升级可用 / Upgrade available')).toBeInTheDocument();
    expect(within(agent).getByText('latest v0.4.0')).toBeInTheDocument();
    expect(
      within(agent).getByText(
        'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token <agent-token> --agent-id agent-checkout-01 --force-download',
      ),
    ).toBeInTheDocument();

    await user.click(within(agent).getByRole('button', { name: 'Copy upgrade command for agent-checkout-01' }));

    expect(writeText).toHaveBeenCalledWith(
      'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token <agent-token> --agent-id agent-checkout-01 --force-download',
    );
    expect(within(agent).getByText('已复制 / Copied')).toBeInTheDocument();
  });

  it('handles denied clipboard access when copying an agent command', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const writeText = vi.fn().mockRejectedValue(new DOMException('Write permission denied.', 'NotAllowedError'));
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText,
      },
    });
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          projectId: 'project-checkout',
          environment: 'staging',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: 'v0.3.0',
          latestVersion: 'v0.4.0',
          upgradeAvailable: true,
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const agent = await screen.findByLabelText('agent checkout-01');
    await user.click(within(agent).getByRole('button', { name: 'Copy upgrade command for agent-checkout-01' }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token <agent-token> --agent-id agent-checkout-01 --force-download',
      );
    });
    expect(within(agent).getByText('复制失败 / Copy failed')).toBeInTheDocument();
  });

  it('creates an agent token and shows the onboarding commands', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText,
      },
    });
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.clear(screen.getByLabelText('Agent token project ID'));
    await user.type(screen.getByLabelText('Agent token project ID'), 'project-checkout');
    await user.clear(screen.getByLabelText('Agent token environment'));
    await user.type(screen.getByLabelText('Agent token environment'), 'staging');
    await user.click(screen.getByRole('button', { name: /Generate token/ }));

    const tokenPanel = await screen.findByLabelText('agent token agent-token-1');
    expect(within(tokenPanel).getByText('ait_test_token')).toBeInTheDocument();
    expect(within(tokenPanel).getByText('Project project-checkout')).toBeInTheDocument();
    expect(within(tokenPanel).getByText('Env staging')).toBeInTheDocument();
    expect(within(tokenPanel).getByText('Expires 2026-06-13T09:30:00Z')).toBeInTheDocument();
    expect(within(tokenPanel).getByText(/--token ait_test_token/)).toBeInTheDocument();
    expect(
      within(tokenPanel).getByText(
        'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token ait_test_token',
      ),
    ).toBeInTheDocument();
    expect(within(tokenPanel).getByText(/Authorization: Bearer ait_test_token/)).toBeInTheDocument();

    await user.click(within(tokenPanel).getByRole('button', { name: 'Copy install command for agent-token-1' }));

    expect(writeText).toHaveBeenCalledWith(
      'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token ait_test_token',
    );
    expect(fetchMock).toHaveBeenCalledWith(
      agentTokensApiURL,
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"expiresInSeconds":86400'),
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      agentTokensApiURL,
      expect.objectContaining({
        body: expect.stringContaining('"projectId":"project-checkout"'),
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      agentTokensApiURL,
      expect.objectContaining({
        body: expect.stringContaining('"environment":"staging"'),
      }),
    );
  });

  it('revokes a newly generated agent token from the targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.click(screen.getByRole('button', { name: /Generate token/ }));
    const tokenPanel = await screen.findByLabelText('agent token agent-token-1');
    expect(within(tokenPanel).getByText('ait_test_token')).toBeInTheDocument();

    await user.click(within(tokenPanel).getByRole('button', { name: 'Revoke agent token agent-token-1' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${agentTokensApiURL}/agent-token-1/revoke`,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    expect(within(tokenPanel).getByText('revoked')).toBeInTheDocument();
    expect(within(tokenPanel).queryByText('ait_test_token')).not.toBeInTheDocument();
    expect(within(tokenPanel).queryByText(/Authorization: Bearer ait_test_token/)).not.toBeInTheDocument();
  });

  it('rotates a generated agent token and replaces the visible secret', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText,
      },
    });
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    await user.click(screen.getByRole('button', { name: /Generate token/ }));
    const tokenPanel = await screen.findByLabelText('agent token agent-token-1');
    expect(within(tokenPanel).getByText('ait_test_token')).toBeInTheDocument();

    await user.click(within(tokenPanel).getByRole('button', { name: 'Copy install command for agent-token-1' }));
    expect(within(tokenPanel).getByText('已复制 / Copied')).toBeInTheDocument();
    expect(writeText).toHaveBeenCalledWith(
      'curl -fsSL http://127.0.0.1:8080/agent/install.sh | sudo bash -s -- --server http://127.0.0.1:8080 --token ait_test_token',
    );

    await user.click(within(tokenPanel).getByRole('button', { name: 'Rotate agent token agent-token-1' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${agentTokensApiURL}/agent-token-1/rotate`,
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"expiresInSeconds":86400'),
        }),
      );
    });
    expect(within(tokenPanel).getByText('ait_rotated_token')).toBeInTheDocument();
    expect(within(tokenPanel).getByText('Rotated 2026-06-12T10:30:00Z')).toBeInTheDocument();
    expect(within(tokenPanel).getByText('Expires 2026-06-13T10:30:00Z')).toBeInTheDocument();
    expect(within(tokenPanel).queryByText('已复制 / Copied')).not.toBeInTheDocument();
    expect(within(tokenPanel).queryByText('ait_test_token')).not.toBeInTheDocument();
    expect(within(tokenPanel).queryByText(/Authorization: Bearer ait_test_token/)).not.toBeInTheDocument();
  });

  it('creates a target and binds it to a registered agent', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('agent checkout-01')).toBeInTheDocument();
    await user.type(screen.getByLabelText(/Target name/), 'checkout-service');
    await user.clear(screen.getByLabelText(/Target project ID/));
    await user.type(screen.getByLabelText(/Target project ID/), 'project-checkout');
    await user.clear(screen.getByLabelText(/Target environment/));
    await user.type(screen.getByLabelText(/Target environment/), 'staging');
    await user.type(screen.getByLabelText(/Target base URL/), 'http://checkout.internal:8080');
    await user.type(screen.getByLabelText(/Profile endpoint/), 'http://127.0.0.1:6060/debug/pprof');
    await user.type(screen.getByLabelText(/Process name/), 'checkout');
    await user.type(screen.getByLabelText(/Health check path/), '/ready');
    await user.type(screen.getByLabelText(/Expected health status/), '204');
    await user.type(screen.getByLabelText(/Health timeout ms/), '1500');
    await user.type(screen.getByLabelText(/CPU alert threshold/), '70');
    await user.type(screen.getByLabelText(/Memory alert threshold/), '65');
    await user.type(screen.getByLabelText(/Disk read alert threshold/), '4096');
    await user.click(screen.getByRole('button', { name: /Create target/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        targetsApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTargetCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === targetsApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(createTargetCall).toBeDefined();
    expect(JSON.parse(String(createTargetCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'checkout-service',
        projectId: 'project-checkout',
        baseUrl: 'http://checkout.internal:8080',
        environment: 'staging',
        agentIds: ['agent-checkout-01'],
        profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
        processMatch: expect.objectContaining({
          name: 'checkout',
        }),
        healthCheck: expect.objectContaining({
          enabled: true,
          path: '/ready',
          expectedStatus: 204,
          timeoutMs: 1500,
        }),
        metricThresholds: expect.objectContaining({
          cpuMaxPercent: 70,
          memoryMaxPercent: 65,
          diskReadMaxBytesPerSec: 4096,
        }),
      }),
    );

    const target = await screen.findByLabelText('target checkout-service');
    expect(within(target).getByText('checkout-service')).toBeInTheDocument();
    expect(within(target).getByText('http://checkout.internal:8080')).toBeInTheDocument();
    expect(within(target).getByText('project project-checkout')).toBeInTheDocument();
    expect(within(target).getByText('env staging')).toBeInTheDocument();
    expect(within(target).getByText('checkout-01')).toBeInTheDocument();
    expect(within(target).getByText('/ready -> 204')).toBeInTheDocument();
    expect(within(target).getByText('pprof')).toBeInTheDocument();
  });

  it('checks a saved target health endpoint from the targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 204,
            timeoutMs: 1500,
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    expect(within(target).getByText('/ready -> 204')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Check target health checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${targetsApiURL}/target-checkout/health-check`,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    expect(within(target).getAllByText('healthy 204').length).toBeGreaterThan(0);
  });

  it('shows the latest persisted target health result from the target list', async () => {
    window.location.hash = '#targets';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 204,
            timeoutMs: 1500,
          },
          lastHealthCheck: {
            targetId: 'target-checkout',
            targetName: 'checkout-service',
            status: 'healthy',
            source: 'agent_daemon',
            url: 'http://checkout.internal:8080/ready',
            expectedStatus: 204,
            observedStatus: 204,
            latencyMs: 12.5,
            checkedAt: '2026-06-12T09:50:00Z',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:50:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    expect(within(target).getByText('/ready -> 204')).toBeInTheDocument();
    expect(within(target).getAllByText('healthy 204').length).toBeGreaterThan(0);
    expect(within(target).getAllByText('Agent daemon').length).toBeGreaterThan(0);
  });

  it('shows recent target health check history from the backend', async () => {
    window.location.hash = '#targets';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 204,
            timeoutMs: 1500,
          },
          healthHistory: [
            {
              targetId: 'target-checkout',
              targetName: 'checkout-service',
              status: 'healthy',
              source: 'agent_daemon',
              url: 'http://checkout.internal:8080/ready',
              expectedStatus: 204,
              observedStatus: 204,
              latencyMs: 12.5,
              checkedAt: '2026-06-12T09:50:00Z',
            },
            {
              targetId: 'target-checkout',
              targetName: 'checkout-service',
              status: 'unhealthy',
              source: 'control_plane',
              url: 'http://checkout.internal:8080/ready',
              expectedStatus: 204,
              observedStatus: 503,
              latencyMs: 8.2,
              error: 'expected HTTP 204, got HTTP 503',
              checkedAt: '2026-06-12T09:49:00Z',
            },
          ],
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:50:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${targetsApiURL}/target-checkout/health-checks?limit=5`);
    });
    const history = await waitFor(() => within(target).getByLabelText('target health history target-checkout'));
    expect(within(history).getByText('healthy 204')).toBeInTheDocument();
    expect(within(history).getByText('Agent daemon')).toBeInTheDocument();
    expect(within(history).getByText('unhealthy 503')).toBeInTheDocument();
    expect(within(history).getByText('Control plane')).toBeInTheDocument();
    expect(within(history).getByText('2026-06-12T09:49:00Z')).toBeInTheDocument();
  });

  it('groups target profile and health actions inside the target row', async () => {
    window.location.hash = '#targets';
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 204,
            timeoutMs: 1500,
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    const actions = target.querySelector('.target-row-actions') as HTMLElement | null;
    expect(actions).not.toBeNull();
    expect(within(actions as HTMLElement).getByText('pprof')).toBeInTheDocument();
    expect(within(actions as HTMLElement).getByRole('button', { name: 'Check target health checkout-service' })).toBeInTheDocument();
    expect(within(actions as HTMLElement).getByRole('button', { name: 'Edit target checkout-service' })).toBeInTheDocument();
    expect(within(actions as HTMLElement).getByRole('button', { name: 'Delete target checkout-service' })).toBeInTheDocument();
  });

  it('manages profile task templates on targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock();
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const templatesPanel = await screen.findByLabelText('profile task templates');
    await user.type(within(templatesPanel).getByLabelText('Profile template id'), 'async_profiler');
    await user.type(within(templatesPanel).getByLabelText('Profile template name'), 'Async Profiler');
    await user.selectOptions(within(templatesPanel).getByLabelText('Profile template kind'), 'command');
    await user.clear(within(templatesPanel).getByLabelText('Profile template profile types'));
    await user.type(within(templatesPanel).getByLabelText('Profile template profile types'), 'jfr');
    await user.clear(within(templatesPanel).getByLabelText('Profile template profile type'));
    await user.type(within(templatesPanel).getByLabelText('Profile template profile type'), 'jfr');
    await user.type(within(templatesPanel).getByLabelText('Profile template command'), 'asprof');
    await user.clear(within(templatesPanel).getByLabelText('Profile template command args'));
    fireEvent.change(within(templatesPanel).getByLabelText('Profile template command args'), {
      target: { value: '["-d","{{seconds}}","-f","{{output}}","{{pid}}"]' },
    });
    await user.clear(within(templatesPanel).getByLabelText('Profile template output'));
    fireEvent.change(within(templatesPanel).getByLabelText('Profile template output'), {
      target: { value: '/tmp/{{targetNameSlug}}.jfr' },
    });
    await user.clear(within(templatesPanel).getByLabelText('Profile template timeout buffer'));
    await user.type(within(templatesPanel).getByLabelText('Profile template timeout buffer'), '10');
    await user.clear(within(templatesPanel).getByLabelText('Profile template display order'));
    await user.type(within(templatesPanel).getByLabelText('Profile template display order'), '30');
    await user.click(within(templatesPanel).getByRole('button', { name: 'Create profile template' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        profileTaskTemplatesApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTemplateCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === profileTaskTemplatesApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(JSON.parse(String(createTemplateCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        id: 'async_profiler',
        name: 'Async Profiler',
        kind: 'command',
        profileTypes: ['jfr'],
        profileType: 'jfr',
        requiresPid: true,
        profileCommand: 'asprof',
        profileCommandArgs: ['-d', '{{seconds}}', '-f', '{{output}}', '{{pid}}'],
        profileCommandOutput: '/tmp/{{targetNameSlug}}.jfr',
        timeoutBufferSeconds: 10,
        displayOrder: 30,
      }),
    );

    const createdRow = await within(templatesPanel).findByLabelText('profile task template async_profiler');
    await user.click(within(createdRow).getByRole('button', { name: 'Edit profile template async_profiler' }));
    await user.clear(within(templatesPanel).getByLabelText('Profile template name'));
    await user.type(within(templatesPanel).getByLabelText('Profile template name'), 'Async Profiler Wall');
    await user.clear(within(templatesPanel).getByLabelText('Profile template output'));
    fireEvent.change(within(templatesPanel).getByLabelText('Profile template output'), {
      target: { value: '/var/tmp/{{targetNameSlug}}.jfr' },
    });
    await user.click(within(templatesPanel).getByRole('button', { name: 'Update profile template' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${profileTaskTemplatesApiURL}/async_profiler`,
        expect.objectContaining({
          method: 'PUT',
        }),
      );
    });
    const updateTemplateCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === `${profileTaskTemplatesApiURL}/async_profiler` && (init?.method || 'GET').toUpperCase() === 'PUT',
    );
    expect(JSON.parse(String(updateTemplateCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'Async Profiler Wall',
        profileCommandOutput: '/var/tmp/{{targetNameSlug}}.jfr',
      }),
    );
    expect(within(templatesPanel).getByText('Async Profiler Wall')).toBeInTheDocument();

    await user.click(within(templatesPanel).getByRole('button', { name: 'Delete profile template async_profiler' }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${profileTaskTemplatesApiURL}/async_profiler`,
        expect.objectContaining({
          method: 'DELETE',
        }),
      );
    });
    await waitFor(() => {
      expect(within(templatesPanel).queryByLabelText('profile task template async_profiler')).not.toBeInTheDocument();
    });
  });

  it('creates a profile task from a saved target row', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          healthCheck: {
            enabled: true,
            path: '/ready',
            expectedStatus: 204,
            timeoutMs: 1500,
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    await user.click(within(target).getByRole('button', { name: 'Create profile task checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        profileTasksApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTaskCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === profileTasksApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(createTaskCall).toBeDefined();
    expect(JSON.parse(String(createTaskCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        agentId: 'agent-checkout-01',
        targetId: 'target-checkout',
        targetName: 'checkout-service',
        pprofBaseUrl: 'http://127.0.0.1:6060/debug/pprof',
        profileType: 'cpu',
        profileSeconds: 30,
        maxAttempts: 3,
        source: 'target_manual',
      }),
    );
    expect(within(target).getByText('profile task profile-task-from-target-1 queued')).toBeInTheDocument();
  });

  it('creates a command profiler profile task from a whitelisted template', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof', 'command_profiler'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: '',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    await user.selectOptions(within(target).getByLabelText('Profile task template checkout-service'), 'linux_perf');
    await user.type(within(target).getByLabelText('Command profiler PID checkout-service'), '1234');
    await user.clear(within(target).getByLabelText('Profile task seconds checkout-service'));
    await user.type(within(target).getByLabelText('Profile task seconds checkout-service'), '15');
    await user.click(within(target).getByRole('button', { name: 'Create profile task checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        profileTasksApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTaskCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === profileTasksApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    const body = JSON.parse(String(createTaskCall?.[1]?.body));
    expect(body).toEqual(
      expect.objectContaining({
        agentId: 'agent-checkout-01',
        targetId: 'target-checkout',
        targetName: 'checkout-service',
        profileType: 'perf',
        profileSeconds: 15,
        profileCommand: 'perf',
        profileCommandArgs: ['record', '-F', '99', '-p', '1234', '-g', '-o', '{{output}}', '--', 'sleep', '{{seconds}}'],
        profileCommandOutput: '/tmp/checkout-service-perf.data',
        profileCommandTimeoutMs: 30000,
        maxAttempts: 3,
        source: 'target_manual',
      }),
    );
    expect(body.pprofBaseUrl).toBeUndefined();
    expect(within(target).getByText('profile task profile-task-from-target-1 queued')).toBeInTheDocument();
  });

  it('creates a command profiler task from backend profile task template fields', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'command_profiler'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: '',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
      [],
      [],
      [],
      null,
      null,
      {
        profileTaskTemplates: [
          defaultProfileTaskTemplates[0],
          {
            id: 'linux_perf',
            name: 'Managed perf',
            description: 'Managed command template from backend storage.',
            kind: 'command',
            profileTypes: ['perf'],
            profileType: 'perf',
            requiresProfileEndpoint: false,
            requiresPid: true,
            profileCommand: 'profilectl',
            profileCommandArgs: ['capture', '--pid', '{{pid}}', '--out', '{{output}}', '--duration', '{{seconds}}'],
            profileCommandOutput: '/var/tmp/{{targetNameSlug}}.profile',
            timeoutBufferSeconds: 7,
            enabled: true,
            displayOrder: 20,
            createdAt: '2026-06-12T09:00:00Z',
            updatedAt: '2026-06-12T09:00:00Z',
          },
        ],
      },
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout service');
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(profileTaskTemplatesApiURL);
    });
    await user.selectOptions(within(target).getByLabelText('Profile task template checkout service'), 'linux_perf');
    await user.type(within(target).getByLabelText('Command profiler PID checkout service'), '2345');
    await user.clear(within(target).getByLabelText('Profile task seconds checkout service'));
    await user.type(within(target).getByLabelText('Profile task seconds checkout service'), '20');
    await user.click(within(target).getByRole('button', { name: 'Create profile task checkout service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        profileTasksApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTaskCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === profileTasksApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(JSON.parse(String(createTaskCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        profileType: 'perf',
        profileSeconds: 20,
        profileCommand: 'profilectl',
        profileCommandArgs: ['capture', '--pid', '2345', '--out', '{{output}}', '--duration', '{{seconds}}'],
        profileCommandOutput: '/var/tmp/checkout-service.profile',
        profileCommandTimeoutMs: 27000,
      }),
    );
  });

  it('uses the selected profile task type and duration from the target row', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    fireEvent.change(within(target).getByLabelText('Profile task type checkout-service'), { target: { value: 'heap' } });
    await user.clear(within(target).getByLabelText('Profile task seconds checkout-service'));
    await user.type(within(target).getByLabelText('Profile task seconds checkout-service'), '5');
    await user.click(within(target).getByRole('button', { name: 'Create profile task checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        profileTasksApiURL,
        expect.objectContaining({
          method: 'POST',
        }),
      );
    });
    const createTaskCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === profileTasksApiURL && (init?.method || 'GET').toUpperCase() === 'POST',
    );
    expect(JSON.parse(String(createTaskCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        profileType: 'heap',
        profileSeconds: 5,
      }),
    );
  });

  it('rejects invalid profile task duration before creating a target row task', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    await user.clear(within(target).getByLabelText('Profile task seconds checkout-service'));
    await user.type(within(target).getByLabelText('Profile task seconds checkout-service'), '0');
    await user.click(within(target).getByRole('button', { name: 'Create profile task checkout-service' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Profile task seconds must be between 1 and 300.');
    expect(fetchMock).not.toHaveBeenCalledWith(
      profileTasksApiURL,
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });

  it('edits a saved target binding from the targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('target checkout-service')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Edit target checkout-service' }));
    expect(screen.getByLabelText(/Target base URL/)).toHaveValue('http://checkout.internal:8080');

    await user.clear(screen.getByLabelText(/Target base URL/));
    await user.type(screen.getByLabelText(/Target base URL/), 'http://checkout.prod.internal:8080');
    await user.clear(screen.getByLabelText(/Profile endpoint/));
    await user.type(screen.getByLabelText(/Profile endpoint/), 'http://127.0.0.1:7070/debug/pprof');
    await user.clear(screen.getByLabelText(/Process name/));
    await user.type(screen.getByLabelText(/Process name/), 'checkout-prod');
    await user.click(screen.getByRole('button', { name: /Update target/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${targetsApiURL}/target-checkout`,
        expect.objectContaining({
          method: 'PUT',
        }),
      );
    });
    const updateTargetCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === `${targetsApiURL}/target-checkout` && (init?.method || 'GET').toUpperCase() === 'PUT',
    );
    expect(updateTargetCall).toBeDefined();
    expect(JSON.parse(String(updateTargetCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'checkout-service',
        baseUrl: 'http://checkout.prod.internal:8080',
        agentIds: ['agent-checkout-01'],
        profileEndpoint: 'http://127.0.0.1:7070/debug/pprof',
        processMatch: expect.objectContaining({
          name: 'checkout-prod',
        }),
      }),
    );

    const target = await screen.findByLabelText('target checkout-service');
    expect(within(target).getByText('http://checkout.prod.internal:8080')).toBeInTheDocument();
    expect(within(target).getByText('checkout-prod')).toBeInTheDocument();
  });

  it('unbinds an agent from a saved target row', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    const target = await screen.findByLabelText('target checkout-service');
    expect(within(target).getByText('checkout-01')).toBeInTheDocument();

    await user.click(within(target).getByRole('button', { name: 'Unbind agent checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${targetsApiURL}/target-checkout`,
        expect.objectContaining({
          method: 'PUT',
        }),
      );
    });
    const updateTargetCall = fetchMock.mock.calls.find(
      ([url, init]) => String(url) === `${targetsApiURL}/target-checkout` && (init?.method || 'GET').toUpperCase() === 'PUT',
    );
    expect(updateTargetCall).toBeDefined();
    expect(JSON.parse(String(updateTargetCall?.[1]?.body))).toEqual(
      expect.objectContaining({
        name: 'checkout-service',
        agentIds: [],
        profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
      }),
    );
    expect(within(target).queryByText('checkout-01')).not.toBeInTheDocument();
    expect(within(target).queryByRole('button', { name: 'Create profile task checkout-service' })).not.toBeInTheDocument();
  });

  it('deletes a saved target binding from the targets page', async () => {
    window.location.hash = '#targets';
    const user = userEvent.setup();
    const fetchMock = createScenarioFetchMock(
      [],
      [],
      [
        {
          id: 'agent-checkout-01',
          name: 'checkout-01',
          hostname: 'checkout-host-01',
          ip: '10.0.0.13',
          version: '0.2.0',
          status: 'online',
          labels: {
            service: 'checkout',
            zone: 'shanghai-b',
          },
          capabilities: ['host_metrics', 'process_metrics', 'pprof'],
          lastSeenAt: '2026-06-12T09:20:00Z',
        },
      ],
      [
        {
          id: 'target-checkout',
          name: 'checkout-service',
          baseUrl: 'http://checkout.internal:8080',
          environment: 'default',
          agentIds: ['agent-checkout-01'],
          profileEndpoint: 'http://127.0.0.1:6060/debug/pprof',
          processMatch: {
            name: 'checkout',
            cmdlineContains: '--config=/etc/checkout/config.yaml',
          },
          createdAt: '2026-06-12T09:40:00Z',
          updatedAt: '2026-06-12T09:40:00Z',
        },
      ],
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<App />);

    expect(await screen.findByLabelText('target checkout-service')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Delete target checkout-service' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `${targetsApiURL}/target-checkout`,
        expect.objectContaining({
          method: 'DELETE',
        }),
      );
    });
    expect(screen.queryByLabelText('target checkout-service')).not.toBeInTheDocument();
    expect(screen.getByText('暂无 Target / No targets yet')).toBeInTheDocument();
  });
});
