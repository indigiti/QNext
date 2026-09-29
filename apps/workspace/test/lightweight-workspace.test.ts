import { describe, expect, it } from 'vitest';

import {
  buildLightweightPaneConfigs,
  normalizeLightweightLayout,
} from '../src/charts/lightweight-workspace';

describe('lightweight workspace layout', () => {
  it('accepts supported layouts and falls back to one chart', () => {
    expect(normalizeLightweightLayout(1)).toBe(1);
    expect(normalizeLightweightLayout('2')).toBe(2);
    expect(normalizeLightweightLayout(4)).toBe(4);
    expect(normalizeLightweightLayout('8')).toBe(1);
  });

  it('preserves existing pane choices and fills new panes from the symbol catalog', () => {
    const configs = buildLightweightPaneConfigs(
      4,
      [{ ticker: 'NSE:NIFTY', timeframe: '1m' }],
      [
        { ticker: 'NSE:NIFTY' },
        { ticker: 'NSE:BANKNIFTY' },
        { ticker: 'NSE:FINNIFTY' },
        { ticker: 'NSE:MIDCPNIFTY' },
      ],
      { ticker: 'NSE:NIFTY', timeframe: '1m' },
    );

    expect(configs).toEqual([
      { ticker: 'NSE:NIFTY', timeframe: '1m' },
      { ticker: 'NSE:BANKNIFTY', timeframe: '1m' },
      { ticker: 'NSE:FINNIFTY', timeframe: '1m' },
      { ticker: 'NSE:MIDCPNIFTY', timeframe: '1m' },
    ]);
  });

  it('drops hidden panes when layout shrinks', () => {
    const configs = buildLightweightPaneConfigs(
      1,
      [
        { ticker: 'NSE:NIFTY', timeframe: '1m' },
        { ticker: 'NSE:BANKNIFTY', timeframe: '5m' },
      ],
      [],
      { ticker: 'NSE:NIFTY', timeframe: '1m' },
    );

    expect(configs).toEqual([{ ticker: 'NSE:NIFTY', timeframe: '1m' }]);
  });
});
