import { describe, expect, it } from 'vitest';

import { OpsAPI } from '../src/api';

describe('OpsAPI', () => {
  it('checks setup status without sending a token', async () => {
    const calls: Array<{ url: string; token: string | null }> = [];
    const fetcher: typeof fetch = async (input, init) => {
      const headers = new Headers(init?.headers);
      calls.push({ url: String(input), token: headers.get('X-QNext-Ops-Token') });
      return new Response(JSON.stringify({ initialized: false }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const status = await api.setupStatus();

    expect(status.initialized).toBe(false);
    expect(calls).toEqual([
      { url: '/qnext/admin/api/index.php?route=%2Fsetup-status', token: null },
    ]);
  });

  it('sends the staging token and parses status', async () => {
    const calls: Array<{ url: string; token: string | null }> = [];
    const fetcher: typeof fetch = async (input, init) => {
      const headers = new Headers(init?.headers);
      calls.push({
        url: String(input),
        token: headers.get('X-QNext-Ops-Token'),
      });
      return new Response(JSON.stringify({
        release: { current: 'r1', available: ['r1'] },
        service: { ok: true, state: 'active' },
        marketCore: {
          health: { ok: true },
          ready: { ok: true },
          version: { ok: true },
        },
        storageRoot: '/private/storage',
        configPath: '/private/config/q1-market.json',
      }), { status: 200 });
    };

    const api = new OpsAPI({
      token: 'secret',
      fetcher,
    });
    const status = await api.status();

    expect(status.release.current).toBe('r1');
    expect(calls).toEqual([{ url: '/qnext/admin/api/index.php?route=%2Fstatus', token: 'secret' }]);
  });

  it('loads authenticated runtime diagnostics', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input) => {
      calls.push(String(input));
      return new Response(JSON.stringify({
        desiredState: 'running',
        pid: 123,
        pidAlive: true,
        pidPath: '/private/run/market-core.pid',
        binaryPath: '/private/bin/qnext-market-core',
        binaryFound: true,
        logPath: '/private/logs/market-core.log',
        logLines: ['market-core started'],
        cronHeartbeatAt: '2026-09-25T07:00:00Z',
        cronHeartbeatAgeSeconds: 10,
        controlMode: 'cron',
        helperPath: '/private/deploy/qnext-ops-user',
      }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const diagnostics = await api.diagnostics();

    expect(diagnostics.pidAlive).toBe(true);
    expect(calls).toEqual(['/qnext/admin/api/index.php?route=%2Fdiagnostics']);
  });

  it('surfaces API errors', async () => {
    const fetcher: typeof fetch = async () =>
      new Response(JSON.stringify({ error: 'denied' }), { status: 403 });
    const api = new OpsAPI({ fetcher });

    await expect(api.status()).rejects.toThrow('denied');
  });
});


  it('reports non-JSON routing failures clearly', async () => {
    const fetcher: typeof fetch = async () =>
      new Response('<!DOCTYPE html><title>Not Found</title>', {
        status: 404,
        headers: { 'Content-Type': 'text/html; charset=UTF-8' },
      });
    const api = new OpsAPI({ fetcher });

    await expect(api.status()).rejects.toThrow('Ops API returned non-JSON HTTP 404');
  });
