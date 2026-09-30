import { describe, expect, it, vi } from 'vitest';

import { QNextProvider } from '../src/qnext-provider';

class ClosedSocket {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  send() {}

  close() {
    this.onclose?.({} as CloseEvent);
  }
}

describe('QNextProvider fallback backpressure', () => {
  it('backs off REST polling after upstream 502 instead of polling every 250ms', async () => {
    let barRequests = 0;
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/qnext/api/v1/symbols/')) {
        return new Response(
          JSON.stringify({
            symbols: [
              {
                instrument_id: 'QNEXT:NIFTY-SYN+',
                ticker: 'NIFTY-SYN+',
                description: 'QNext Nifty Synthetic+ (Shadow)',
                type: 'index',
                prefix: 'QNEXT',
                currency: 'INR',
                timezone: 'Asia/Kolkata',
                calendar_id: 'NSE_EQ',
                synthetic: true,
              },
            ],
          }),
          { status: 200, headers: { 'content-type': 'application/json' } },
        );
      }
      if (target.startsWith('/qnext/api/v1/bars/?')) {
        barRequests += 1;
        return new Response(JSON.stringify({ error: 'market data unavailable' }), {
          status: 502,
          headers: { 'content-type': 'application/json' },
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const socket = new ClosedSocket();
    const provider = new QNextProvider({
      apiBase: '/qnext',
      fetchImpl,
      webSocketFactory: () => socket,
      pollIntervalMs: 5,
      reconnectDelayMs: 60_000,
    });

    const unsubscribe = provider.subscribe('NIFTY-SYN+', '30s', vi.fn());

    await waitFor(() => socket.onclose !== null);
    socket.close();
    await waitFor(() => barRequests === 1);

    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(barRequests).toBe(1);

    unsubscribe();
  });
});

async function waitFor(predicate: () => boolean, timeoutMs = 1_000) {
  const started = Date.now();
  while (!predicate()) {
    if (Date.now() - started > timeoutMs) {
      throw new Error('timed out waiting for condition');
    }
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
}
