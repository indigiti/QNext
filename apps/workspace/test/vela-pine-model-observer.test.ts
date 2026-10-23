import { describe, expect, it, vi } from 'vitest';
import type { ScriptingEngine, ExecutionHandlers, ExecutionRequest } from '@luxalgo/vela/plugin';
import type { IndicatorModel } from '@luxalgo/vela/plugin';
import { VelaSignalTrace } from '../src/charts/vela-signal-trace';
import { wrapPineModelTrace, markerSnapshot } from '../src/charts/vela-pine-model-observer';

const bars = [
  { time: 1000, open: 1, high: 2, low: 0, close: 1 },
  { time: 2000, open: 1, high: 2, low: 0, close: 1 },
];
const bareModel = (): IndicatorModel => ({
  id: 'buy-sell', title: 'Buy Sell Test', overlay: true, paneHint: 'price',
  series: [], fills: [], backgrounds: [], priceLines: [], inputs: [], inputValues: {},
});
const fake = (sendModel: (handlers: ExecutionHandlers) => void): ScriptingEngine => ({
  language: 'pine',
  capabilities: { streaming: true, visibleRange: true, inputs: true, props: true },
  prepare: vi.fn(async () => ({ language: 'pine', inputs: [], meta: { title: 'Test', overlay: true }, reactsToViewport: false, token: null })),
  execute: vi.fn((_req, handlers) => {
    sendModel(handlers);
    return { stop: vi.fn(), update: vi.fn(), setVisibleRange: vi.fn(), notifyBars: vi.fn() };
  }),
});
const req = (): ExecutionRequest => ({
  mode: 'live', market: { symbol: 'NSE:NIFTY', timeframe: '15s' }, bars,
  getBars: () => bars,
  prepared: { language: 'pine', inputs: [], meta: { title: 'Test', overlay: true }, reactsToViewport: false, token: null },
});

describe('Pine model observer', () => {
  it('forwards unchanged model and records a new plotshape marker at the finalized candle', () => {
    const trace = new VelaSignalTrace();
    const input = bareModel();
    input.labels = [{
      id: 'sell-1', paneId: 'price', xloc: 'bar_index', x: 1, y: 1, yloc: 'abovebar',
      style: 'circle', text: 'SELL', color: '#ff0000', size: 'normal', textAlign: 'center', fontFamily: 'default',
    }];
    const handler = vi.fn();
    const engine = wrapPineModelTrace(fake((handlers) => handlers.onModel(input)), trace);
    engine.execute(req(), { onModel: handler });
    expect(handler).toHaveBeenCalledExactlyOnceWith(input);
    expect(trace.snapshot().at(-1)).toMatchObject({
      kind: 'pine-model', initial: true, markerCount: 1,
      latest: [{ source: 'label', id: 'sell-1', time: 2000, text: 'SELL' }],
    });
  });

  it('records marker changes without flooding identical live model updates', () => {
    const trace = new VelaSignalTrace();
    const model = bareModel();
    trace.pineModel('NIFTY', '1m', model.title, model.id, markerSnapshot(model, bars));
    expect(trace.summary().pineModelChanges).toBe(1);
    trace.pineModel('NIFTY', '1m', model.title, model.id, markerSnapshot(model, bars));
    expect(trace.summary().pineModelChanges).toBe(1);
    model.labels = [{
      id: 'buy-1', paneId: 'price', xloc: 'bar_index', x: 0, y: 1, yloc: 'belowbar',
      style: 'triangleup', color: '#00ff00', size: 'normal', textAlign: 'center', fontFamily: 'default',
    }];
    trace.pineModel('NIFTY', '1m', model.title, model.id, markerSnapshot(model, bars));
    const update = trace.snapshot().at(-1);
    expect(update).toMatchObject({
      kind: 'pine-model', initial: false, addedCount: 1, removedCount: 0,
      added: [{ id: 'buy-1', time: 1000, shape: 'triangleup' }],
    });
    trace.pineModel('NIFTY', '1m', model.title, model.id, []);
    expect(trace.snapshot().at(-1)).toMatchObject({ kind: 'pine-model', removedCount: 1 });
  });

  it('respects marker-series display and anchor-relative label positions', () => {
    const model = bareModel();
    model.anchorTime = 2000;
    model.labels = [{
      id: 'anchor', paneId: 'price', xloc: 'bar_index', x: 0, y: 1, yloc: 'belowbar',
      style: 'circle', size: 'normal', textAlign: 'center', fontFamily: 'default',
    }];
    model.series = [{
      id: 's', title: 'Sell', kind: 'markers', paneId: 'price', visible: false,
      markers: [{ time: 2000, position: 'aboveBar', shape: 'arrowDown', color: '#a00' }],
    }];
    expect(markerSnapshot(model, bars)).toMatchObject([
      { id: 'anchor', time: 2000 },
      { source: 'marker-series', time: 2000, visible: false },
    ]);
  });
});
