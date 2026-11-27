import { describe, expect, it, vi } from 'vitest';
import { QNextProvider, type QNextBar } from '../src/qnext-provider';
import { sameCandlePoint, sameVolumePoint } from '../src/charts/lightweight-adapter';

const symbols = { symbols: [{
  instrument_id: 'NSE:NIFTY50', ticker: 'NIFTY', description: 'Nifty 50',
  type: 'index', prefix: 'NSE', currency: 'INR', timezone: 'Asia/Kolkata',
  calendar_id: 'NSE_EQ', synthetic: false,
}] };
const fixture = { time: 100, open: 10, high: 12, low: 9, close: 11, volume: 5,
  final: true, quality: 'GOOD', revision: 2, volume_source: 'FUTURES_PROXY', volume_status: 'OBSERVED' };
const range = { from: 0, to: 1000, limit: 10 };
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}
function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status });
}
class Socket {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  send() {}
  open() { this.readyState = 1; this.onopen?.({} as Event); }
  close() { this.readyState = 3; this.onclose?.({} as CloseEvent); }
}
describe('history request single flight', () => {
  it('shares an identical pending chart and canonical request, without sharing mutable output or caching completed results', async () => {
    const first = deferred<Response>();
    let requests = 0;
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('/symbols/')) return json(symbols);
      requests++;
      return requests === 1 ? first.promise : json({ bars: [fixture] });
    });
    const provider = new QNextProvider({ fetchImpl });
    const a = provider.getBars('NIFTY', '1m', range);
    const b = provider.getCanonicalBars('NSE:NIFTY50', '1', range);
    await vi.waitFor(() => expect(requests).toBe(1));
    first.resolve(json({ bars: [fixture] }));
    const [chart, canonical] = await Promise.all([a, b]);
    expect(requests).toBe(1);
    expect(canonical[0]).toMatchObject({ final: true, quality: 'GOOD', revision: 2 });
    chart[0].close = 999;
    expect(canonical[0].close).toBe(11);
    expect((await provider.getBars('NIFTY', '1m', range))[0].close).toBe(11);
    expect(requests).toBe(2);
  });

  it('keeps different query ranges separate and clears failed requests', async () => {
    let requests = 0;
    const provider = new QNextProvider({ fetchImpl: vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('/symbols/')) return json(symbols);
      requests++;
      return json({}, 502);
    }) });
    const results = await Promise.allSettled([
      provider.getBars('NIFTY', '1m', range),
      provider.getCanonicalBars('NIFTY', '1m', range),
      provider.getBars('NIFTY', '1m', { ...range, to: 2000 }),
    ]);
    expect(requests).toBe(2);
    expect(results.every(result => result.status === 'rejected')).toBe(true);
    await expect(provider.getBars('NIFTY', '1m', range)).rejects.toThrow('HTTP 502');
    expect(requests).toBe(3);
  });
});

describe('fallback generation isolation', () => {
  it('does not deliver or reschedule an obsolete REST poll across socket recovery and a second disconnect', async () => {
    const pending: Array<ReturnType<typeof deferred<Response>>> = [];
    const sockets: Socket[] = [];
    const provider = new QNextProvider({
      fetchImpl: vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes('/symbols/')) return json(symbols);
        const request = deferred<Response>();
        pending.push(request);
        return request.promise;
      }),
      webSocketFactory: () => { const s = new Socket(); sockets.push(s); return s; },
      reconnectDelayMs: 5,
      pollIntervalMs: 60_000,
    });
    const received = vi.fn();
    const unsubscribe = provider.subscribe('NIFTY', '1m', received);
    try {
      await vi.waitFor(() => expect(sockets).toHaveLength(1));
      sockets[0].open();
      sockets[0].close();
      await vi.waitFor(() => expect(pending).toHaveLength(1));
      await vi.waitFor(() => expect(sockets.length).toBeGreaterThanOrEqual(2));
      sockets[1].open();
      sockets[1].close();
      expect(pending).toHaveLength(1);
      pending[0].resolve(json({ bars: [fixture] }));
      await vi.waitFor(() => expect(pending).toHaveLength(2));
      expect(received).not.toHaveBeenCalled();
      pending[1].resolve(json({ bars: [{ ...fixture, close: 12 }] }));
      await vi.waitFor(() => expect(received).toHaveBeenCalledTimes(1));
      expect(received).toHaveBeenCalledWith(expect.objectContaining({ close: 12 }));
    } finally {
      unsubscribe();
      for (const request of pending) request.resolve(json({ bars: [] }));
    }
  });
});

describe('rendered series comparison', () => {
  it('skips metadata-only and independent series updates without treating missing volume as zero', () => {
    const bar: QNextBar = fixture;
    expect(sameCandlePoint(bar, { ...bar, revision: 3, volume: 8 })).toBe(true);
    expect(sameVolumePoint(bar, { ...bar, close: 12 })).toBe(true);
    expect(sameCandlePoint(bar, { ...bar, close: 12 })).toBe(false);
    expect(sameVolumePoint(bar, { ...bar, volume: 8 })).toBe(false);
    expect(sameVolumePoint({ ...bar, volume: 0 }, { ...bar, volume: undefined })).toBe(false);
    expect(sameVolumePoint({ ...bar, volume: undefined }, { ...bar, volume: NaN })).toBe(true);
  });
});
