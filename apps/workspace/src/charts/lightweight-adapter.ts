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
  if (bar.volume === undefined || !Number.isFinite(bar.volume) || bar.volume < 0) {
    return undefined;
  }

  return {
    time: toLightweightTime(bar.time),
    value: bar.volume,
  };
}

/** Compare rendered values only. Metadata/revisions must still be retained by the caller. */
export function sameCandlePoint(a: QNextBar, b: QNextBar): boolean {
  return a.time === b.time && a.open === b.open && a.high === b.high &&
    a.low === b.low && a.close === b.close;
}

export function sameVolumePoint(a: QNextBar, b: QNextBar): boolean {
  const left = toVolumeData(a);
  const right = toVolumeData(b);
  if (!left || !right) return left === right;
  return left.time === right.time && left.value === right.value;
}
