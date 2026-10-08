import { describe, expect, it } from 'vitest';

import {
  captureActiveChartLabSnapshot,
  LAB_CHART_SNAPSHOT_SCHEMA,
} from '../src/intelligence-lab-snapshot';

describe('Intelligence Lab chart snapshot', () => {
  it('captures only visible indicators from the active chart', async () => {
    const workspace = {
      getState: () => ({
        activeCellId: 'chart-1',
        charts: [
          { id: 'chart-1', symbol: 'NSE:NIFTY', timeframe: '1m' },
          { id: 'chart-2', symbol: 'NSE:BANKNIFTY', timeframe: '5m' },
        ],
      }),
      chart: {
        indicators: () => [
          {
            id: 'ema-1',
            title: 'Adaptive EMA',
            visible: true,
            nativeType: 'adaptive-ema-qalg',
            inputs: [{ key: 'emaLength', defval: 20 }],
            inputValues: () => ({ emaLength: 34 }),
            context: async () => ({
              meta: { language: 'qnext' },
              plots: {
                EMA: [
                  { time: 1_800_000_000_000, value: 100 },
                  { time: 1_800_000_060_000, value: 101 },
                ],
              },
              variables: { trend: 1, colorBars: true },
            }),
          },
          {
            id: 'hidden-rsi',
            title: 'RSI',
            visible: false,
            source: '//@version=6\nindicator("RSI")',
            inputs: [],
            inputValues: () => ({}),
            context: async () => ({
              plots: { RSI: [{ time: 1_800_000_060_000, value: 55 }] },
              variables: {},
            }),
          },
        ],
      },
    };

    const snapshot = await captureActiveChartLabSnapshot(workspace, 1_800_000_120_000);

    expect(snapshot.schema).toBe(LAB_CHART_SNAPSHOT_SCHEMA);
    expect(snapshot.instrument_id).toBe('NSE:NIFTY');
    expect(snapshot.timeframe).toBe('1m');
    expect(snapshot.indicators).toHaveLength(1);
    expect(snapshot.indicators[0]?.instance_id).toBe('ema-1');
    expect(snapshot.indicators[0]?.inputs).toEqual({ emaLength: 34 });
    expect(snapshot.feature_rows).toEqual([
      {
        bar_time_ms: 1_800_000_000_000,
        features: { 'indicator.ema_1.plot.ema': 100 },
      },
      {
        bar_time_ms: 1_800_000_060_000,
        features: { 'indicator.ema_1.plot.ema': 101 },
      },
    ]);
    expect(snapshot.current_features).toMatchObject({
      'indicator.ema_1.plot.ema': 101,
      'indicator.ema_1.var.trend': 1,
      'indicator.ema_1.var.colorbars': 1,
    });
    expect(snapshot.indicator_configuration_hash).toMatch(/^[a-f0-9]{64}$/);
  });

  it('rejects a chart with no enabled indicators', async () => {
    const workspace = {
      getState: () => ({
        activeCellId: 'chart-1',
        charts: [{ id: 'chart-1', symbol: 'NSE:NIFTY', timeframe: '1m' }],
      }),
      chart: {
        indicators: () => [
          {
            id: 'hidden',
            title: 'Hidden',
            visible: false,
            context: async () => null,
          },
        ],
      },
    };

    await expect(captureActiveChartLabSnapshot(workspace)).rejects.toThrow(
      'No enabled indicators',
    );
  });
});
