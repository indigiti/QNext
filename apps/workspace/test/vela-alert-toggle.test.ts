import { describe, expect, it, vi } from 'vitest';
import type { ExecutionHandlers, ScriptingEngine } from '@luxalgo/vela/plugin';
import {
  CHART_ALERTS_STORAGE_KEY,
  ChartAlertSwitch,
  withChartAlertSwitch,
} from '../src/vela-alert-toggle';

function memoryStorage(initial: string | null = null) {
  const stored = new Map<string, string>();
  if (initial !== null) stored.set(CHART_ALERTS_STORAGE_KEY, initial);
  return {
    getItem: vi.fn((key: string) => stored.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { stored.set(key, value); }),
  };
}

function fakeEngine() {
  let callbacks: ExecutionHandlers | null = null;
  const prepare = vi.fn(async () => ({
    language: 'pine', inputs: [], meta: { title: 'Buy/Sell Test', overlay: true },
    reactsToViewport: false, token: null,
  }));
  const session = {
    stop: vi.fn(),
    update: vi.fn(),
    setVisibleRange: vi.fn(),
    notifyBars: vi.fn(),
  };
  const execute = vi.fn((_req, handlers: ExecutionHandlers) => {
    callbacks = handlers;
    return session;
  });
  const engine: ScriptingEngine = {
    language: 'pine',
    capabilities: { streaming: true, visibleRange: true, inputs: true },
    prepare,
    execute,
  };
  return {
    engine, prepare, execute, session,
    emitAlert: () => callbacks?.onAlert?.({
      id: 'buy', message: 'Buy condition', title: 'Buy', time: 15000,
      barIndex: 0,
    }),
    emitModel: (model: Parameters<ExecutionHandlers['onModel']>[0]) => callbacks?.onModel(model),
    emitError: (error: Error) => callbacks?.onError?.(error),
  };
}

describe('Vela chart alert On/Off control', () => {
  it('defaults ON, persists choice and restores OFF without turning off script execution', () => {
    const storage = memoryStorage();
    const alerts = new ChartAlertSwitch(storage);
    expect(alerts.enabled).toBe(true);
    expect(alerts.toggle()).toBe(false);
    expect(storage.setItem).toHaveBeenCalledWith(CHART_ALERTS_STORAGE_KEY, 'off');
    expect(new ChartAlertSwitch(storage).enabled).toBe(false);
    alerts.setEnabled(true);
    expect(new ChartAlertSwitch(storage).enabled).toBe(true);
  });

  it('keeps the setting in memory when browser storage is restricted', () => {
    const storage = {
      getItem: () => { throw new Error('blocked'); },
      setItem: () => { throw new Error('blocked'); },
    };
    const alerts = new ChartAlertSwitch(storage);
    expect(alerts.enabled).toBe(true);
    alerts.setEnabled(false);
    expect(alerts.enabled).toBe(false);
    alerts.setEnabled(true);
    expect(alerts.enabled).toBe(true);
  });

  it('suppresses new Pine script alerts at the engine boundary, not the model/marker stream', async () => {
    const fake = fakeEngine();
    const alerts = new ChartAlertSwitch(memoryStorage());
    const guarded = withChartAlertSwitch(fake.engine, alerts);
    const prepared = await guarded.prepare('//@version=6', 'signal');
    const onModel = vi.fn();
    const onAlert = vi.fn();
    const onError = vi.fn();
    const request = {
      prepared, market: { symbol: 'NIFTY-SYN+', timeframe: '15s' },
      bars: [], mode: 'live' as const,
    };
    const session = guarded.execute(request, { onModel, onAlert, onError });
    expect(session).toBe(fake.session);
    expect(fake.execute).toHaveBeenCalledOnce();
    expect(guarded.capabilities.streaming).toBe(true);

    const model = {
      id: 'signal', title: 'Buy/Sell Test', overlay: true, paneHint: 'price' as const,
      series: [], fills: [], backgrounds: [], priceLines: [],
      inputs: [], inputValues: {},
    };
    fake.emitAlert();
    expect(onAlert).toHaveBeenCalledTimes(1);
    alerts.setEnabled(false);
    fake.emitAlert();
    fake.emitModel(model);
    fake.emitError(new Error('engine still reports failures'));
    expect(onAlert).toHaveBeenCalledTimes(1);
    expect(onModel).toHaveBeenCalledExactlyOnceWith(model);
    expect(onError).toHaveBeenCalledOnce();
    alerts.setEnabled(true);
    fake.emitAlert();
    expect(onAlert).toHaveBeenCalledTimes(2);
    expect(session.stop).not.toHaveBeenCalled();
  });

  it('does not apply chart preferences to other engine types or broker/server alerts', () => {
    const fake = fakeEngine();
    const alerts = new ChartAlertSwitch(memoryStorage('off'));
    const engine = withChartAlertSwitch(fake.engine, alerts);
    expect(engine.language).toBe('pine');
    expect(engine.capabilities).toBe(fake.engine.capabilities);
    // The wrapper neither mutates the original engine nor manipulates
    // server-side alert routes, order actions or notification records.
    expect(engine).not.toBe(fake.engine);
    expect(fake.engine.execute).toBe(fake.execute);
  });
});
