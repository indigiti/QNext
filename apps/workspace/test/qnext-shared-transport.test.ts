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

describe('QNextProvider shared live transport', () => {
  it('multiplexes multiple logical bar streams over one browser WebSocket', async () => {
    const sockets: FakeSocket[] = [];
    const provider = new QNextProvider({
      fetchImpl: jsonFetch(),
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      reconnectDelayMs: 60_000,
    });

    const onNifty = vi.fn();
    const onSynthetic = vi.fn();
    const unsubscribeNifty = provider.subscribe('NIFTY', '15s', onNifty);
    const unsubscribeSynthetic = provider.subscribe('NIFTY-SYN', '30s', onSynthetic);

    await waitFor(() => sockets.length === 1);
    sockets[0].open();
    await waitFor(() => sockets[0].sent.length === 2);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].sent).toEqual(
      expect.arrayContaining([
        {
          op: 'subscribe',
          channel: 'bars',
          symbol: 'NSE:NIFTY50',
          timeframe: '15s',
        },
        {
          op: 'subscribe',
          channel: 'bars',
          symbol: 'QNEXT:NIFTY-SYN',
          timeframe: '30s',
        },
      ]),
    );
    expect(provider.transportSnapshot()).toEqual({
      mode: 'wss',
      sockets: 1,
      streams: 2,
      consumers: 2,
      polling_streams: 0,
    });

    sockets[0].message({
      op: 'subscribed',
      stream_id: 'bars:NSE:NIFTY50:15s',
      seq: 0,
      symbol: 'NSE:NIFTY50',
      timeframe: '15s',
    });
    sockets[0].message({
      op: 'subscribed',
      stream_id: 'bars:QNEXT:NIFTY-SYN:30s',
      seq: 0,
      symbol: 'QNEXT:NIFTY-SYN',
      timeframe: '30s',
    });
    sockets[0].message({
      op: 'update',
      stream_id: 'bars:NSE:NIFTY50:15s',
      seq: 1,
      bar: { time: 100, open: 10, high: 12, low: 9, close: 11, volume: 5 },
    });
    sockets[0].message({
      op: 'update',
      stream_id: 'bars:QNEXT:NIFTY-SYN:30s',
      seq: 1,
      bar: { time: 200, open: 20, high: 23, low: 19, close: 22, volume: 0 },
    });

    expect(onNifty).toHaveBeenCalledWith({
      time: 100,
      open: 10,
      high: 12,
      low: 9,
      close: 11,
      volume: 5,
    });
    expect(onSynthetic).toHaveBeenCalledWith({
      time: 200,
      open: 20,
      high: 23,
      low: 19,
      close: 22,
      volume: 0,
    });

    unsubscribeNifty();
    unsubscribeSynthetic();
  });

  it('replays missed bars after reconnect instead of discarding them behind the resume acknowledgement', async () => {
    const sockets: FakeSocket[] = [];
    const provider = new QNextProvider({
      fetchImpl: jsonFetch(),
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      reconnectDelayMs: 1,
    });

    const onBar = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '15s', onBar);

    await waitFor(() => sockets.length === 1);
    sockets[0].open();
    await waitFor(() => sockets[0].sent.length === 1);

    const streamID = 'bars:NSE:NIFTY50:15s';
    sockets[0].message({
      op: 'subscribed',
      stream_id: streamID,
      seq: 5,
      symbol: 'NSE:NIFTY50',
      timeframe: '15s',
    });
    sockets[0].message({
      op: 'update',
      stream_id: streamID,
      seq: 6,
      bar: { time: 100, open: 10, high: 12, low: 9, close: 11, volume: 5 },
    });

    sockets[0].close();
    await waitFor(() => sockets.length === 2);
    sockets[1].open();
    await waitFor(() => sockets[1].sent.length === 1);

    expect(sockets[1].sent[0]).toEqual({
      op: 'resume',
      stream_id: streamID,
      after_seq: 6,
    });

    // Market Core acknowledges resume at its current sequence before sending
    // the replay events. The client must preserve after_seq until replay lands.
    sockets[1].message({
      op: 'subscribed',
      stream_id: streamID,
      seq: 9,
    });
    sockets[1].message({
      op: 'update',
      stream_id: streamID,
      seq: 7,
      bar: { time: 200, open: 11, high: 13, low: 10, close: 12, volume: 6 },
    });
    sockets[1].message({
      op: 'update',
      stream_id: streamID,
      seq: 8,
      bar: { time: 300, open: 12, high: 14, low: 11, close: 13, volume: 7 },
    });
    sockets[1].message({
      op: 'update',
      stream_id: streamID,
      seq: 9,
      bar: { time: 400, open: 13, high: 15, low: 12, close: 14, volume: 8 },
    });

    expect(onBar).toHaveBeenCalledTimes(4);
    expect(onBar).toHaveBeenNthCalledWith(2, {
      time: 200,
      open: 11,
      high: 13,
      low: 10,
      close: 12,
      volume: 6,
    });
    expect(onBar).toHaveBeenNthCalledWith(4, {
      time: 400,
      open: 13,
      high: 15,
      low: 12,
      close: 14,
      volume: 8,
    });

    unsubscribe();
  });

  it('reference-counts duplicate chart consumers instead of duplicating server subscriptions', async () => {
    const sockets: FakeSocket[] = [];
    const provider = new QNextProvider({
      fetchImpl: jsonFetch(),
      webSocketFactory: () => {
        const socket = new FakeSocket();
        sockets.push(socket);
        return socket;
      },
      reconnectDelayMs: 60_000,
    });

    const first = vi.fn();
    const second = vi.fn();
    const unsubscribeFirst = provider.subscribe('NIFTY', '15s', first);
    const unsubscribeSecond = provider.subscribe('NIFTY', '15s', second);

    await waitFor(() => sockets.length === 1);
    sockets[0].open();
    await waitFor(() => sockets[0].sent.length === 1);

    expect(sockets[0].sent).toEqual([
      {
        op: 'subscribe',
        channel: 'bars',
        symbol: 'NSE:NIFTY50',
        timeframe: '15s',
      },
    ]);
    expect(provider.transportSnapshot()).toMatchObject({
      mode: 'wss',
      sockets: 1,
      streams: 1,
      consumers: 2,
    });

    sockets[0].message({
      op: 'subscribed',
      stream_id: 'bars:NSE:NIFTY50:15s',
      seq: 0,
      symbol: 'NSE:NIFTY50',
      timeframe: '15s',
    });
    sockets[0].message({
      op: 'update',
      stream_id: 'bars:NSE:NIFTY50:15s',
      seq: 1,
      bar: { time: 100, open: 10, high: 12, low: 9, close: 11, volume: 5 },
    });
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);

    unsubscribeFirst();
    expect(provider.transportSnapshot()).toMatchObject({
      streams: 1,
      consumers: 1,
    });
    expect(sockets[0].sent).toHaveLength(1);

    unsubscribeSecond();
    expect(sockets[0].sent.at(-1)).toEqual({
      op: 'unsubscribe',
      stream_id: 'bars:NSE:NIFTY50:15s',
    });
    expect(provider.transportSnapshot()).toEqual({
      mode: 'idle',
      sockets: 0,
      streams: 0,
      consumers: 0,
      polling_streams: 0,
    });
  });
});

function jsonFetch(): typeof fetch {
  return vi.fn(async (input: RequestInfo | URL) => {
    const target = String(input);
    if (target.startsWith('/api/v1/symbols/')) {
      return jsonResponse(symbolsPayload);
    }
    if (target.startsWith('/api/v1/bars/?')) {
      return jsonResponse({ bars: [] });
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
