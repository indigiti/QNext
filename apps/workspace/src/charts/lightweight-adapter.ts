import type { CandlestickData, HistogramData, UTCTimestamp } from 'lightweight-charts';

import type { QNextBar } from '../qnext-provider';

export function toLightweightTime(milliseconds: number): UTCTimestamp {
  return Math.floor(milliseconds / 1_000) as UTCTimestamp;
}

export function toCandlestickData(bar: QNextBar): CandlestickData<UTCTimestamp> {
  return {
    time: toLightweightTime(bar.time),
    open: bar.open,
    high: bar.high,
    low: bar.low,
    close: bar.close,
  };
}

export function toVolumeData(bar: QNextBar): HistogramData<UTCTimestamp> | undefined {
  if (bar.volume === undefined) {
    return undefined;
  }

  return {
    time: toLightweightTime(bar.time),
    value: bar.volume,
  };
}
