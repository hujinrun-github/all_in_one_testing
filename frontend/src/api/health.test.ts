import { afterEach, describe, expect, it, vi } from 'vitest';
import { getHealth } from './health';

describe('getHealth', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads backend health status from the API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getHealth();

    expect(fetchMock).toHaveBeenCalledWith('/api/health');
    expect(result).toEqual({ status: 'ok' });
  });

  it('throws a readable error when the backend returns a non-OK response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getHealth()).rejects.toThrow('Health check failed with HTTP 503');
  });
});
