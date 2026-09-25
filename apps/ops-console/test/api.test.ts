import { describe, expect, it } from 'vitest';

import { OpsAPI } from '../src/api';

describe('OpsAPI', () => {
  it('checks setup status without sending a token', async () => {
    const calls: Array<{ url: string; token: string | null }> = [];
    const fetcher: typeof fetch = async (input, init) => {
      const headers = new Headers(init?.headers);
      calls.push({ url: String(input), token: headers.get('X-QNext-Ops-Token') });
      return new Response(JSON.stringify({ initialized: false, bootstrapConfigured: true }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const status = await api.setupStatus();

    expect(status.initialized).toBe(false);
    expect(status.bootstrapConfigured).toBe(true);
    expect(calls).toEqual([
      { url: '/qnext/admin/api/index.php?route=%2Fsetup-status', token: null },
    ]);
  });

  it('requires a separate one-time bootstrap token for first-time admin initialization', async () => {
    const calls: Array<{ url: string; adminToken: string | null; setupToken: string | null; body: string }> = [];
    const fetcher: typeof fetch = async (input, init) => {
      const headers = new Headers(init?.headers);
      calls.push({
        url: String(input),
        adminToken: headers.get('X-QNext-Ops-Token'),
        setupToken: headers.get('X-QNext-Setup-Token'),
        body: String(init?.body ?? ''),
      });
      return new Response(JSON.stringify({ initialized: true }), { status: 201 });
    };

    const api = new OpsAPI({ token: 'must-not-be-used-for-setup', fetcher });
    await api.initializeAdminToken('new-admin-token-0123456789', 'bootstrap-0123456789abcdef');

    expect(calls).toEqual([{
      url: '/qnext/admin/api/index.php?route=%2Fsetup',
      adminToken: null,
      setupToken: 'bootstrap-0123456789abcdef',
      body: JSON.stringify({ token: 'new-admin-token-0123456789' }),
    }]);
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

  it('loads authenticated market feed status', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input) => {
      calls.push(String(input));
      return new Response(JSON.stringify({
        ok: true,
        status: 200,
        body: {
          live_configured: true,
          resilience_configured: false,
          nifty_instrument_id: 'NSE:NIFTY50',
          synthetic_instrument_id: 'QNEXT:NIFTY-SYN',
          telemetry: {
            providers: { upstox: { observed: 10, last_event_time_ms: 1 } },
            instruments: {},
          },
          resilience: { providers: {} },
        },
      }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const feed = await api.feedStatus();

    expect(feed.ok).toBe(true);
    expect(feed.body?.live_configured).toBe(true);
    expect(calls).toEqual(['/qnext/admin/api/index.php?route=%2Ffeed-status']);
  });

  it('verifies Dhan standby readiness', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input, init) => {
      calls.push(`${init?.method ?? 'GET'} ${String(input)}`);
      return new Response(JSON.stringify({
        ok: true,
        fresh: true,
        ageMs: 250,
        received: 12,
        errors: 0,
        authority: 'upstox',
        safeForFailoverDrill: true,
        reason: 'Dhan standby is receiving fresh NIFTY quotes',
      }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const result = await api.verifyDhanStandby();

    expect(result.ok).toBe(true);
    expect(result.safeForFailoverDrill).toBe(true);
    expect(calls).toEqual(['POST /qnext/admin/api/index.php?route=%2Fdhan-standby-check']);
  });

  it('loads and saves active markets', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input, init) => {
      calls.push(`${init?.method ?? 'GET'} ${String(input)}`);
      if (init?.method === 'PUT') {
        return new Response(JSON.stringify({
          saved: true,
          active: ['NIFTY'],
        }), { status: 200 });
      }
      return new Response(JSON.stringify({
        available: ['NIFTY', 'BANKNIFTY'],
        active: ['NIFTY', 'BANKNIFTY'],
      }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const state = await api.activeMarkets();
    expect(state.active).toEqual(['NIFTY', 'BANKNIFTY']);

    const saved = await api.saveActiveMarkets(['NIFTY']);
    expect(saved.active).toEqual(['NIFTY']);
    expect(calls).toEqual([
      'GET /qnext/admin/api/index.php?route=%2Factive-markets',
      'PUT /qnext/admin/api/index.php?route=%2Factive-markets',
    ]);
  });

  it('loads and runs historical repair', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input, init) => {
      calls.push(`${init?.method ?? 'GET'} ${String(input)}`);
      if (init?.method === 'POST') {
        return new Response(JSON.stringify({
          days: 30,
          markets: [],
          started_at_ms: 1,
          completed_at_ms: 2,
        }), { status: 200 });
      }
      return new Response(JSON.stringify({
        ok: true,
        body: { running: false },
      }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    const status = await api.historicalRepairStatus();
    expect(status.body?.running).toBe(false);

    const result = await api.runHistoricalRepair(30);
    expect(result.days).toBe(30);
    expect(calls).toEqual([
      'GET /qnext/admin/api/index.php?route=%2Fhistory-repair',
      'POST /qnext/admin/api/index.php?route=%2Fhistory-repair',
    ]);
  });


  it('manages custom indicators through authenticated CRUD endpoints', async () => {
    const calls: string[] = [];
    const fetcher: typeof fetch = async (input, init) => {
      const method = init?.method ?? 'GET';
      calls.push(`${method} ${String(input)}`);
      if (method === 'GET') {
        return new Response(JSON.stringify({
          schema: 'QNEXT.INDICATORS/1',
          revision: 1,
          kinds: [
            { id: 'pine-v6', label: 'Pine Script v6' },
            { id: 'adaptive-ema-qalg', label: 'Adaptive EMA [QALG]' },
          ],
          indicators: [],
        }), { status: 200 });
      }
      if (method === 'POST') {
        return new Response(JSON.stringify({ saved: true, revision: 2, indicator: { id: 'pine-ema-cross' } }), { status: 201 });
      }
      if (method === 'PUT') {
        return new Response(JSON.stringify({ saved: true, revision: 3, indicator: { id: 'pine-ema-cross', enabled: false } }), { status: 200 });
      }
      return new Response(JSON.stringify({ deleted: true, revision: 4, id: 'pine-ema-cross' }), { status: 200 });
    };

    const api = new OpsAPI({ token: 'secret', fetcher });
    await api.customIndicators();
    const indicator = {
      id: 'pine-ema-cross',
      name: 'Pine EMA Cross',
      category: 'QNext' as const,
      kind: 'pine-v6' as const,
      language: 'pine' as const,
      enabled: true,
      script: '//@version=6\nindicator("Pine EMA Cross", overlay=true)\nplot(ta.ema(close, 9))',
    };
    await api.createCustomIndicator(indicator);
    await api.updateCustomIndicator(indicator.id, { enabled: false });
    await api.deleteCustomIndicator(indicator.id);

    expect(calls).toEqual([
      'GET /qnext/admin/api/index.php?route=%2Fcustom-indicators',
      'POST /qnext/admin/api/index.php?route=%2Fcustom-indicators',
      'PUT /qnext/admin/api/index.php?route=%2Fcustom-indicators%2Fpine-ema-cross',
      'DELETE /qnext/admin/api/index.php?route=%2Fcustom-indicators%2Fpine-ema-cross',
    ]);
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
