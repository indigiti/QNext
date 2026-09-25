import { describe, expect, it, vi } from 'vitest';

import {
  QNextIndicatorEngine,
  calculateAdaptiveEma,
  type AdaptiveEmaDefaults,
} from '../src/qnext-indicator-engine';

const settings: AdaptiveEmaDefaults = {
  priceSource: 'close',
  emaLength: 2,
  lookbackPeriod: 2,
  stddevMultiplier: 0.1,
  atrLength: 2,
  atrMultiplier: 0.1,
  upColor: '#00ffaa',
  downColor: '#ff0000',
  colorBars: true,
};

describe('QNextIndicatorEngine', () => {
  it('calculates persistent long/short trend state from EMA, SD and ATR filters', () => {
    const bars = [10, 10, 20, 30, 5].map((close, index) => ({
      time: index + 1,
      open: close,
      high: close + 1,
      low: close - 1,
      close,
    }));

    const result = calculateAdaptiveEma(bars, settings);

    expect(result.ema).toHaveLength(bars.length);
    expect(result.stddev[0]).toBeNull();
    expect(result.atr[0]).toBeNull();
    expect(result.trend).toContain(1);
    expect(result.trend.at(-1)).toBe(-1);
  });

  it('prepares the managed QNext definition and emits an overlay model with fill and bar colors', async () => {
    const engine = new QNextIndicatorEngine();
    const source = JSON.stringify({
      schema: 'QNEXT.INDICATOR/1',
      id: 'adaptive-ema-qalg',
      name: 'Adaptive EMA [QALG]',
      kind: 'adaptive-ema-qalg',
      defaults: settings,
    });
    const prepared = await engine.prepare(source, 'instance-1');

    expect(prepared.meta).toMatchObject({
      title: 'Adaptive EMA [QALG]',
      overlay: true,
    });
    expect(prepared.inputs.some((input) => input.key === 'atrMultiplier')).toBe(true);

    const handlers = {
      onModel: vi.fn(),
      onDone: vi.fn(),
    };
    const bars = [10, 10, 20, 30, 5].map((close, index) => ({
      time: index + 1,
      open: close,
      high: close + 1,
      low: close - 1,
      close,
    }));

    const session = engine.execute({
      prepared,
      market: { symbol: 'NIFTY', timeframe: '1m' },
      bars,
      getBars: () => bars,
      mode: 'static',
    }, handlers);

    expect(handlers.onModel).toHaveBeenCalledTimes(1);
    const model = handlers.onModel.mock.calls[0]![0];
    expect(model.title).toBe('Adaptive EMA [QALG]');
    expect(model.series).toHaveLength(2);
    expect(model.fills).toHaveLength(1);
    expect(model.barColors.length).toBeGreaterThan(0);

    session.stop();
  });
});
