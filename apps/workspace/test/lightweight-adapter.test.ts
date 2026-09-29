import { describe, expect, it } from 'vitest';

import {
  toCandlestickData,
  toLightweightTime,
  toVolumeData,
} from '../src/charts/lightweight-adapter';

const bar = {
  time: 1_700_000_000_987,
  open: 100,
  high: 110,
  low: 95,
  close: 105,
  volume: 1_234,
};

describe('lightweight chart adapter', () => {
  it('converts QNext millisecond timestamps to unix seconds', () => {
    expect(toLightweightTime(bar.time)).toBe(1_700_000_000);
  });

  it('maps QNext OHLC bars to candlestick data', () => {
    expect(toCandlestickData(bar)).toEqual({
      time: 1_700_000_000,
      open: 100,
      high: 110,
      low: 95,
      close: 105,
    });
  });

  it('maps volume when available', () => {
    expect(toVolumeData(bar)).toEqual({
      time: 1_700_000_000,
      value: 1_234,
    });
  });

  it('omits volume points when QNext has no volume', () => {
    expect(toVolumeData({ ...bar, volume: undefined })).toBeUndefined();
  });
});
