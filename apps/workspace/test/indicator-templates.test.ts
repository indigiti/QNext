import { describe, expect, it, vi } from 'vitest';
import type { WorkspaceState } from '@luxalgo/vela/workspace';
import {
  INDICATOR_TEMPLATES_STORAGE_KEY,
  applyIndicatorTemplateToState,
  captureLiveIndicatorSnapshot,
  unavailableTemplateIndicators,
  readIndicatorTemplates,
  sanitizeIndicatorTemplates,
  writeIndicatorTemplates,
  type IndicatorTemplate,
} from '../src/indicator-templates';

const template: IndicatorTemplate = {
  id: 'trend-setup',
  name: 'Trend setup',
  updatedAt: 1_800_000_000_000,
  favorite: true,
  indicators: {
    manifest: [
      {
        name: 'EMA-VWAP-ADX',
        inputs: { emaLength: 34, showVwap: false },
        props: { precision: 2 },
      },
    ],
    natives: [{ type: 'rsi', inputs: { length: 21 } }],
  },
};

const state: WorkspaceState = {
  version: 1,
  layout: '1',
  activeCellId: 'c1',
  charts: [
    {
      id: 'c1',
      symbol: 'NSE:NIFTY',
      timeframe: '15s',
      indicators: {
        manifest: ['Old script'],
        natives: ['sma'],
      },
    },
    {
      id: 'c2',
      symbol: 'NSE:BANKNIFTY',
      timeframe: '1m',
      indicators: {
        manifest: ['Other script'],
        natives: ['volume'],
      },
    },
  ],
};

function memoryStorage(initial?: string) {
  const values = new Map<string, string>();
  if (initial !== undefined) values.set(INDICATOR_TEMPLATES_STORAGE_KEY, initial);
  return {
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { values.set(key, value); }),
  };
}

describe('QNeXT indicator templates', () => {
  it('round-trips custom input and declaration values through versioned storage', () => {
    const storage = memoryStorage();
    expect(writeIndicatorTemplates(storage, [template])).toBe(true);
    expect(readIndicatorTemplates(storage)).toEqual([template]);
    expect(storage.setItem).toHaveBeenCalledWith(
      INDICATOR_TEMPLATES_STORAGE_KEY,
      expect.stringContaining('"emaLength":34'),
    );
  });

  it('drops malformed templates and rejects unsafe or non-primitive input values', () => {
    const result = sanitizeIndicatorTemplates({
      version: 1,
      templates: [
        template,
        { ...template, id: 'bad-inputs', indicators: { manifest: [{ name: 'Broken', inputs: { length: {} } }], natives: [] } },
        { ...template, id: 'bad-name', name: '   ' },
      ],
    });
    expect(result).toEqual([template]);
  });

  it('returns an empty list when local storage is corrupt or unavailable', () => {
    expect(readIndicatorTemplates({
      getItem: () => '{not-json',
    })).toEqual([]);
    expect(readIndicatorTemplates({
      getItem: () => { throw new Error('storage blocked'); },
    })).toEqual([]);
  });

  it('applies only the indicator snapshot to the active chart and preserves its market and drawings', () => {
    const next = applyIndicatorTemplateToState(state, template);
    expect(next.charts[0]?.symbol).toBe('NSE:NIFTY');
    expect(next.charts[0]?.timeframe).toBe('15s');
    expect(next.charts[0]?.drawings).toBeUndefined();
    expect(next.charts[0]?.indicators).toEqual(template.indicators);
    expect(next.charts[1]).toEqual(state.charts[1]);
    expect(state.charts[0]?.indicators?.manifest).toEqual(['Old script']);
  });

  it('can apply the same indicator template to every chart without changing their markets', () => {
    const next = applyIndicatorTemplateToState(state, template, 'all');
    expect(next.charts.map((chart) => chart.indicators)).toEqual([
      template.indicators,
      template.indicators,
    ]);
    expect(next.charts.map((chart) => [chart.symbol, chart.timeframe])).toEqual([
      ['NSE:NIFTY', '15s'],
      ['NSE:BANKNIFTY', '1m'],
    ]);
  });

  it('captures complete values, repeated script instances, false, zero and hidden state', () => {
    const handle = (values: Record<string, unknown>, visible = true) => ({
      inputValues: () => values, propValues: () => ({ precision: 2 }), visible,
    });
    const cell = {
      instances: [
        { entry: { name: 'EMA' }, handle: handle({ length: 9, show: false, offset: 0 }) },
        { entry: { name: 'EMA' }, handle: handle({ length: 34 }, false) },
      ],
      chart: { indicators: () => [{ ...handle({ length: 21 }), nativeType: 'rsi' }] },
    } as unknown as Parameters<typeof captureLiveIndicatorSnapshot>[0];
    expect(captureLiveIndicatorSnapshot(cell, { manifest: ['EMA', 'EMA'], natives: ['rsi'] })).toEqual({
      manifest: [
        { name: 'EMA', inputs: { length: 9, show: false, offset: 0 }, props: { precision: 2 } },
        { name: 'EMA', inputs: { length: 34 }, props: { precision: 2 }, hidden: true },
      ],
      natives: [{ type: 'rsi', inputs: { length: 21 } }],
    });
    expect(() => captureLiveIndicatorSnapshot(cell, { manifest: ['EMA'], natives: ['rsi'] })).toThrow();
  });

  it('blocks unavailable studies before replacing the current set', () => {
    expect(unavailableTemplateIndicators(template.indicators, [
      { name: 'EMA-VWAP-ADX' }, { name: 'RSI', native: true, nativeType: 'rsi' },
    ])).toEqual([]);
    expect(unavailableTemplateIndicators(template.indicators, [])).toEqual(['EMA-VWAP-ADX', 'rsi']);
  });

  it('rejects invalid and over-limit writes rather than silently deleting templates', () => {
    const storage = memoryStorage();
    expect(writeIndicatorTemplates(storage, [{ ...template, name: '' }])).toBe(false);
    expect(writeIndicatorTemplates(storage, Array.from({ length: 101 }, (_, i) => ({ ...template, id: String(i) })))).toBe(false);
    expect(storage.setItem).not.toHaveBeenCalled();
  });

  it('does not write when browser storage rejects the update', () => {
    expect(writeIndicatorTemplates({
      setItem: () => { throw new Error('quota exceeded'); },
    }, [template])).toBe(false);
  });
});
