import { describe, expect, it, vi } from 'vitest';
import type {
  ExecutionRequest, ExecutionSession, PreparedScript, ScriptingEngine,
} from '@luxalgo/vela/plugin';
import { withConfirmedMtfRecompute } from '../src/pine-engine';

function setup(title = 'FVG') {
  const session: ExecutionSession = {
    getContext: vi.fn(async () => null),
    update: vi.fn(),
    setVisibleRange: vi.fn(),
    notifyBars: vi.fn(),
    stop: vi.fn(),
  };
  const execute = vi.fn((_req: ExecutionRequest) => session);
  const prepare = async (): Promise<PreparedScript> => ({
    language: 'pine', inputs: [], meta: { title, overlay: true },
    reactsToViewport: false, token: null,
  });
  const upstream: ScriptingEngine = {
    language: 'pine',
    capabilities: { streaming: true, inputs: true, visibleRange: true },
    prepare, execute,
  };
  const engine = withConfirmedMtfRecompute(upstream);
  const bars: ExecutionRequest['bars'] = [
    { time: 15_000, open: 1, high: 1, low: 1, close: 1 },
  ];
  const request = (prepared: PreparedScript): ExecutionRequest => ({
    prepared, mode: 'live', market: { symbol: 'NIFTY-SYN+', timeframe: '15s' },
    bars, getBars: () => bars, inputs: { gapTf: '2' },
  });
  return { engine, execute, session, bars, request, handlers: { onModel: vi.fn() } };
}

describe('confirmed MTF snapshot policy', () => {
  it('keeps the selected timeframe and reruns on inputs or a new chart bar, not every tick', async () => {
    const { engine, execute, session, bars, request, handlers } = setup();
    const prepared = await engine.prepare('//@version=6\n// @qnext-confirmed-mtf-recompute', 'fvg');
    const active = engine.execute(request(prepared), handlers);
    expect(execute.mock.calls[0]?.[0]).toMatchObject({ mode: 'static', inputs: { gapTf: '2' } });
    active.update({ gapTf: '5' });
    expect(session.update).toHaveBeenCalledWith({ gapTf: '5' });
    active.notifyBars();
    expect(session.notifyBars).not.toHaveBeenCalled();
    bars.push({ ...bars[0]!, time: 30_000 });
    active.notifyBars();
    active.notifyBars();
    expect(session.notifyBars).toHaveBeenCalledTimes(1);
    active.notifyBars('backfill');
    active.notifyBars('complete');
    expect(session.notifyBars).toHaveBeenCalledWith('backfill');
    expect(session.notifyBars).toHaveBeenCalledWith('complete');
    active.stop();
    expect(session.stop).toHaveBeenCalledOnce();
  });

  it('automatically uses snapshots for the legacy Trinity ribbon without changing its inputs', async () => {
    const { engine, execute, session, bars, request, handlers } = setup('Trinity MidPoint ATR Ribbon');
    const prepared = await engine.prepare('//@version=6\nplot(request.security(syminfo.tickerid, "1", close))', 'trinity');
    const req = { ...request(prepared), inputs: { higherTf: '1' } };
    const active = engine.execute(req, handlers);
    expect(execute.mock.calls[0]?.[0]).toMatchObject({ mode: 'static', inputs: { higherTf: '1' } });
    active.update({ higherTf: '2' });
    active.update({ higherTf: '5' });
    expect(session.update).toHaveBeenLastCalledWith({ higherTf: '5' });
    bars[0] = { ...bars[0]!, close: 2 };
    active.notifyBars();
    expect(session.notifyBars).not.toHaveBeenCalled();
    bars.push({ ...bars[0]!, time: 30_000 });
    active.notifyBars();
    expect(session.notifyBars).toHaveBeenCalledTimes(1);
    // A late revision of the previous bar must invalidate the snapshot.
    bars[0] = { ...bars[0]!, high: 3 };
    active.notifyBars();
    expect(session.notifyBars).toHaveBeenCalledTimes(2);
    active.notifyBars();
    expect(session.notifyBars).toHaveBeenCalledTimes(2);
    active.notifyBars('complete');
    expect(session.notifyBars).toHaveBeenLastCalledWith('complete');
    active.stop();
  });

  it('does not switch other MTF indicators or a Trinity title without security calls', async () => {
    for (const [title, source] of [
      ['Other MTF', 'plot(request.security(syminfo.tickerid, "1", close))'],
      ['Trinity MidPoint ATR Ribbon', 'plot(close)'],
    ] as const) {
      const { engine, execute, request, handlers } = setup(title);
      const prepared = await engine.prepare(source, 'unrelated');
      const req = request(prepared);
      engine.execute(req, handlers);
      expect(execute.mock.calls[0]?.[0]).toBe(req);
    }
  });

  it('leaves unrelated live scripts streaming', async () => {
    const { engine, execute, session, request, handlers } = setup();
    const prepared = await engine.prepare('//@version=6', 'ema');
    const req = request(prepared);
    expect(engine.execute(req, handlers)).toBe(session);
    expect(execute.mock.calls[0]?.[0]).toBe(req);
  });
});
