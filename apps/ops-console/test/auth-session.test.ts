import { describe, expect, it } from 'vitest';

import { OpsAPI } from '../src/api';

describe('OpsAPI admin session recovery', () => {
  it('exchanges the token for a same-origin session after a stripped-header 403 and retries', async () => {
    const calls: Array<{ path: string; method: string; token: string | null }> = [];
    let sessionEstablished = false;

    const fetcher: typeof fetch = async (input, init) => {
      const url = new URL(String(input), 'https://stage.example.test');
      const path = decodeURIComponent(url.searchParams.get('route') ?? '');
      const headers = new Headers(init?.headers);
      const method = init?.method ?? 'GET';
      calls.push({ path, method, token: headers.get('X-QNext-Ops-Token') });

      if (path === '/session') {
        expect(method).toBe('POST');
        expect(headers.get('X-QNext-Ops-Token')).toBeNull();
        expect(JSON.parse(String(init?.body))).toEqual({ token: 'correct-token' });
        sessionEstablished = true;
        return new Response(JSON.stringify({ authenticated: true }), { status: 200 });
      }

      if (path === '/config') {
        if (!sessionEstablished) {
          return new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 });
        }
        return new Response(JSON.stringify({ timeframes: ['1m'] }), { status: 200 });
      }

      return new Response(JSON.stringify({ error: 'not found' }), { status: 404 });
    };

    const api = new OpsAPI({ token: 'correct-token', fetcher });
    await expect(api.getConfig()).resolves.toEqual({ timeframes: ['1m'] });
    expect(calls).toEqual([
      { path: '/config', method: 'GET', token: 'correct-token' },
      { path: '/session', method: 'POST', token: null },
      { path: '/config', method: 'GET', token: null },
    ]);
  });

  it('locks protected polling after a rejected token until a new token is supplied', async () => {
    let calls = 0;
    const rejectingFetcher: typeof fetch = async () => {
      calls += 1;
      return new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 });
    };

    const rejected = new OpsAPI({ token: 'bad-token', fetcher: rejectingFetcher });
    await expect(rejected.status()).rejects.toThrow('forbidden');
    expect(calls).toBe(2); // protected request + one session exchange

    const unauthenticated = new OpsAPI({ fetcher: rejectingFetcher });
    await expect(unauthenticated.status()).rejects.toThrow('Admin authentication required');
    expect(calls).toBe(2); // no repeated network polling while auth is rejected

    let restoredSession = false;
    const restoredFetcher: typeof fetch = async (input) => {
      const url = new URL(String(input), 'https://stage.example.test');
      const path = decodeURIComponent(url.searchParams.get('route') ?? '');
      if (path === '/session') {
        restoredSession = true;
        return new Response(JSON.stringify({ authenticated: true }), { status: 200 });
      }
      if (path === '/status') {
        if (!restoredSession) {
          return new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 });
        }
        return new Response(JSON.stringify({
          release: { current: 'r1', available: ['r1'] },
          service: { ok: true, state: 'active' },
          marketCore: { health: { ok: true }, ready: { ok: true }, version: { ok: true } },
          storageRoot: '/private/storage',
          configPath: '/private/config/q1-market.json',
          host: {
            processControl: false,
            cronControl: true,
            controlMode: 'cron',
            helperAvailable: true,
            helperPath: '/private/deploy/qnext-ops-user',
            cronCommand: '* * * * * helper',
          },
        }), { status: 200 });
      }
      return new Response(JSON.stringify({ error: 'not found' }), { status: 404 });
    };

    const restored = new OpsAPI({ token: 'new-token', fetcher: restoredFetcher });
    await expect(restored.status()).resolves.toMatchObject({ service: { state: 'active' } });
  });
});
