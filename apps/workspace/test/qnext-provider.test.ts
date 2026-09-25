import { describe, expect, it, vi } from 'vitest';

import { QNextProvider } from '../src/qnext-provider';

const symbolsPayload = {
  symbols: [
    {
      instrument_id: 'NSE:NIFTY50',
      ticker: 'NIFTY',
      description: 'Nifty 50',
      type: 'index',
      prefix: 'NSE',
      currency: 'INR',
      timezone: 'Asia/Kolkata',
      calendar_id: 'NSE_EQ',
      synthetic: false,
    },
    {
      instrument_id: 'QNEXT:NIFTY-SYN',
      ticker: 'NIFTY-SYN',
      description: 'QNext Nifty Synthetic',
      type: 'index',
      prefix: 'QNEXT',
      currency: 'INR',
      timezone: 'Asia/Kolkata',
      calendar_id: 'NSE_EQ',
      synthetic: true,
    },
  ],
};

describe('QNextProvider', () => {
  it('exposes canonical QNext symbols to Vela', async () => {
    const provider = new QNextProvider({
      fetchImpl: jsonFetch({
        '/api/v1/symbols/': symbolsPayload,
      }),
    });

    await expect(provider.listSymbols()).resolves.toEqual([
      {
        ticker: 'NIFTY',
        description: 'Nifty 50',
        type: 'index',
        prefix: 'NSE',
      },
      {
        ticker: 'NIFTY-SYN',
        description: 'QNext Nifty Synthetic',
        type: 'index',
        prefix: 'QNEXT',
      },
    ]);
  });

  it('orders index and synthetic pairs for the workspace picker', async () => {
    const tickers = [
      'BANKEX-SYN',
      'SENSEX',
      'MIDCPNIFTY-SYN',
      'BANKNIFTY',
      'NIFTY-SYN',
      'FINNIFTY',
      'BANKEX',
      'NIFTY',
      'SENSEX-SYN',
      'MIDCPNIFTY',
      'FINNIFTY-SYN',
      'BANKNIFTY-SYN',
    ];
    const provider = new QNextProvider({
      fetchImpl: jsonFetch({
        '/api/v1/symbols/': {
          symbols: tickers.map((ticker) => ({
            instrument_id: `TEST:${ticker}`,
            ticker,
            description: ticker,
            type: 'index',
            prefix: ticker.endsWith('-SYN') ? 'QNEXT' : 'NSE',
            currency: 'INR',
            timezone: 'Asia/Kolkata',
            calendar_id: 'NSE_EQ',
            synthetic: ticker.endsWith('-SYN'),
          })),
        },
      }),
    });

    const symbols = await provider.listSymbols();
    expect(symbols.map((symbol) => symbol.ticker)).toEqual([
      'NIFTY',
      'NIFTY-SYN',
      'BANKNIFTY',
      'BANKNIFTY-SYN',
      'MIDCPNIFTY',
      'MIDCPNIFTY-SYN',
      'FINNIFTY',
      'FINNIFTY-SYN',
      'SENSEX',
      'SENSEX-SYN',
      'BANKEX',
      'BANKEX-SYN',
    ]);
  });

  it('prefixes public API requests with the configured QNext base path', async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      calls.push(target);
      if (target.startsWith('/qnext/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({
      apiBase: '/qnext/',
      fetchImpl,
    });

    await provider.listSymbols();
    expect(calls).toEqual(['/qnext/api/v1/symbols/']);
  });

  it('normalizes, de-duplicates and orders history bars', async () => {
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      if (target.startsWith('/api/v1/bars/?')) {
        expect(target).toContain('instrument_id=NSE%3ANIFTY50');
        expect(target).toContain('timeframe=1m');
        return jsonResponse({
          bars: [
            { time: 3, open: 30, high: 31, low: 29, close: 30.5, volume: 3 },
            { time: 1, open: 10, high: 11, low: 9, close: 10.5, volume: 1 },
            { time: 3, open: 30, high: 32, low: 29, close: 31, volume: 4 },
            { time: 2, open: 20, high: 21, low: 19, close: 20.5, volume: 2 },
          ],
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({ fetchImpl });
    const bars = await provider.getBars('NIFTY', '1m', {
      from: 0,
      to: 10_000,
      limit: 2,
    });

    expect(bars).toEqual([
      { time: 2, open: 20, high: 21, low: 19, close: 20.5, volume: 2 },
      { time: 3, open: 30, high: 32, low: 29, close: 31, volume: 4 },
    ]);
  });

  it('passes the resolved NSE calendar windows to Vela', async () => {
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      if (target.startsWith('/api/v1/calendar/?')) {
        expect(target).toContain('instrument_id=NSE%3ANIFTY50');
        expect(target).toContain('session=regular');
        return jsonResponse({
          calendar_id: 'NSE_EQ',
          version: 'nse-equities-2026-v1',
          timezone: 'Asia/Kolkata',
          session: 'regular',
          windows: [
            [200, 300],
            [100, 150],
          ],
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({ fetchImpl });
    await expect(
      provider.getCalendar('NSE:NIFTY', {
        from: 0,
        to: 1_000,
        session: 'regular',
      }),
    ).resolves.toEqual([
      [100, 150],
      [200, 300],
    ]);
  });

  it('subscribes to canonical live bars over the QNext stream', async () => {
    const sockets: FakeSocket[] = [];
    const provider = new QNextProvider({
      fetchImpl: jsonFetch({
        '/api/v1/symbols/': symbolsPayload,
      }),
      webSocketFactory: (url) => {
        expect(url).toContain('/api/v1/stream');
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      reconnectDelayMs: 60_000,
    });

    const onBar = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '1m', onBar);

    await waitFor(() => sockets.length === 1);
    const socket = sockets[0];
    socket.open();

    expect(socket.sent[0]).toEqual({
      op: 'subscribe',
      channel: 'bars',
      symbol: 'NSE:NIFTY50',
      timeframe: '1m',
    });

    socket.message({
      op: 'subscribed',
      stream_id: 'bars:NSE:NIFTY50:1m',
      seq: 0,
    });
    socket.message({
      op: 'update',
      stream_id: 'bars:NSE:NIFTY50:1m',
      seq: 1,
      bar: {
        time: 100,
        open: 10,
        high: 12,
        low: 9,
        close: 11,
        volume: 20,
      },
    });

    expect(onBar).toHaveBeenCalledWith({
      time: 100,
      open: 10,
      high: 12,
      low: 9,
      close: 11,
      volume: 20,
    });

    unsubscribe();
    expect(socket.sent.at(-1)).toEqual({
      op: 'unsubscribe',
      stream_id: 'bars:NSE:NIFTY50:1m',
    });
  });

  it('falls back to REST polling when the public WebSocket cannot open', async () => {
    const sockets: FakeSocket[] = [];
    let barsRequests = 0;
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/qnext/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      if (target.startsWith('/qnext/api/v1/bars/?')) {
        barsRequests += 1;
        return jsonResponse({
          bars: [
            {
              time: 1_000,
              open: 23000,
              high: 23010,
              low: 22995,
              close: 23005 + barsRequests,
              volume: 0,
            },
          ],
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({
      apiBase: '/qnext',
      fetchImpl,
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      pollIntervalMs: 5,
      reconnectDelayMs: 60_000,
    });

    const onBar = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '15s', onBar);

    await waitFor(() => sockets.length === 1);
    sockets[0].close();

    await waitFor(() => barsRequests >= 1 && onBar.mock.calls.length >= 1);
    expect(onBar.mock.calls[0][0]).toMatchObject({
      time: 1_000,
      open: 23000,
      high: 23010,
      low: 22995,
    });

    unsubscribe();
    const requestsAfterUnsubscribe = barsRequests;
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(barsRequests).toBe(requestsAfterUnsubscribe);
  });

  it('promotes from REST polling back to WSS when a later retry opens', async () => {
    const sockets: FakeSocket[] = [];
    let barsRequests = 0;
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/qnext/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      if (target.startsWith('/qnext/api/v1/bars/?')) {
        barsRequests += 1;
        return jsonResponse({
          bars: [
            {
              time: 1_000,
              open: 23000,
              high: 23010,
              low: 22995,
              close: 23005 + barsRequests,
              volume: 0,
            },
          ],
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({
      apiBase: '/qnext',
      fetchImpl,
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      pollIntervalMs: 5,
      reconnectDelayMs: 5,
    });

    const onBar = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '15s', onBar);

    await waitFor(() => sockets.length === 1);
    sockets[0].close();
    await waitFor(() => barsRequests >= 1 && sockets.length >= 2);

    const beforeOpen = barsRequests;
    sockets[1].open();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(barsRequests).toBe(beforeOpen);

    unsubscribe();
  });

  it('heals RESYNC_REQUIRED with a REST snapshot before fresh live subscribe', async () => {
    const sockets: FakeSocket[] = [];
    let barsRequests = 0;
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const target = String(input);
      if (target.startsWith('/api/v1/symbols/')) {
        return jsonResponse(symbolsPayload);
      }
      if (target.startsWith('/api/v1/bars/?')) {
        barsRequests += 1;
        return jsonResponse({
          bars: [
            { time: 100, open: 10, high: 12, low: 9, close: 11, volume: 20 },
            { time: 200, open: 11, high: 13, low: 10, close: 12, volume: 25 },
          ],
        });
      }
      return new Response(null, { status: 404 });
    }) as typeof fetch;

    const provider = new QNextProvider({
      fetchImpl,
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      reconnectDelayMs: 60_000,
    });
    const onBar = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '1m', onBar);

    await waitFor(() => sockets.length === 1);
    const socket = sockets[0];
    socket.open();
    socket.message({
      op: 'subscribed',
      stream_id: 'bars:NSE:NIFTY50:1m',
      seq: 7,
    });
    socket.message({
      op: 'resync_required',
      stream_id: 'bars:NSE:NIFTY50:1m',
    });

    await waitFor(() => barsRequests === 1 && socket.sent.length >= 2);
    expect(onBar).toHaveBeenNthCalledWith(1, {
      time: 100,
      open: 10,
      high: 12,
      low: 9,
      close: 11,
      volume: 20,
    });
    expect(onBar).toHaveBeenNthCalledWith(2, {
      time: 200,
      open: 11,
      high: 13,
      low: 10,
      close: 12,
      volume: 25,
    });
    expect(socket.sent.at(-1)).toEqual({
      op: 'subscribe',
      channel: 'bars',
      symbol: 'NSE:NIFTY50',
      timeframe: '1m',
    });

    unsubscribe();
  });

});

function jsonFetch(routes: Record<string, unknown>): typeof fetch {
  return vi.fn(async (input: RequestInfo | URL) => {
    const target = String(input);
    for (const [prefix, payload] of Object.entries(routes)) {
      if (target.startsWith(prefix)) {
        return jsonResponse(payload);
      }
    }
    return new Response(null, { status: 404 });
  }) as typeof fetch;
}

function jsonResponse(payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

async function waitFor(predicate: () => boolean): Promise<void> {
  const deadline = Date.now() + 1_000;
  while (!predicate()) {
    if (Date.now() > deadline) {
      throw new Error('condition was not met');
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

class FakeSocket {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  sent: Array<Record<string, unknown>> = [];

  send(data: string): void {
    this.sent.push(JSON.parse(data) as Record<string, unknown>);
  }

  close(): void {
    this.readyState = 3;
    this.onclose?.({} as CloseEvent);
  }

  open(): void {
    this.readyState = 1;
    this.onopen?.({} as Event);
  }

  message(message: Record<string, unknown>): void {
    this.onmessage?.({ data: JSON.stringify(message) } as MessageEvent);
  }
}
