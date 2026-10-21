import { describe, expect, it, vi } from 'vitest';
import type { QNextBar } from '../src/qnext-provider';
import {
  applyRevisedHistoricalBar, loadOlderHistoryPage, mergeHistoricalBars, historyPageSpanMs,
  nextVelaHistoryDepth, VELA_MAX_HISTORY,
} from '../src/charts/history-pagination';
import { shouldFetchOlderVela } from '../src/charts/vela-history';

const bar = (time: number, close = time): QNextBar => ({
  time, open: close, high: close + 1, low: close - 1, close, volume: 42,
});

describe('automatic pan history', () => {
  it('applies late finalized volume only for a newer known revision', () => {
    const bars = [
      { ...bar(1_000, 100), volume: 200, revision: 0 },
      { ...bar(61_000, 101), volume: 10, revision: 0 },
    ];
    const corrected = { ...bars[0], volume: 350, revision: 1 };
    expect(applyRevisedHistoricalBar(bars, corrected)).toBe(true);
    expect(bars[0].volume).toBe(350);
    expect(bars[1].volume).toBe(10);
    expect(applyRevisedHistoricalBar(bars, { ...corrected, volume: 50, revision: 0 })).toBe(false);
    expect(applyRevisedHistoricalBar(bars, { ...corrected, time: 121_000, revision: 2 })).toBe(false);
    expect(bars).toHaveLength(2);
    expect(bars[0].volume).toBe(350);
  });

  it('prevents duplicate bars and preserves newer live revisions', () => {
    expect(mergeHistoricalBars([bar(200, 999), bar(300)], [bar(100), bar(200, 111)]))
      .toEqual([bar(100), bar(200, 999), bar(300)]);
  });

  it('only requests preceding bounded nonoverlapping pages', async () => {
    const now = 1_800_000_000_000;
    const span = historyPageSpanMs('15s');
    const load = vi.fn(async (from: number, to: number) =>
      to === now ? [] : [bar(to - 15_000), bar(to - 30_000)]);
    const result = await loadOlderHistoryPage(now, '15s', load);
    expect(load).toHaveBeenCalledTimes(2);
    expect(load).toHaveBeenNthCalledWith(1, now - span, now, 500);
    expect(load).toHaveBeenNthCalledWith(2, now - span * 2, now - span, 500);
    expect(result.exhausted).toBe(false);
    expect(result.nextCursor).toBe(now - span - 30_000);
  });

  it('bounds empty-history scans without indefinitely hammering the API', async () => {
    const load = vi.fn(async () => []);
    const now = 1_800_000_000_000;
    const result = await loadOlderHistoryPage(now, '15s', load);
    expect(result.exhausted).toBe(true);
    expect(load).toHaveBeenCalledTimes(3);
    expect(result.nextCursor).toBe(now - 3 * historyPageSpanMs('15s'));
  });

  it('filters out the cursor bar and future candles', async () => {
    const cursor = 1_800_000_000_000;
    const result = await loadOlderHistoryPage(cursor, '15s', async () => [
      bar(cursor - 15_000), bar(cursor), bar(cursor + 15_000),
    ]);
    expect(result.bars.map((value) => value.time)).toEqual([cursor - 15_000]);
  });

  it('only extends Vela after leftward navigation near loaded oldest time', () => {
    const oldest = 1_800_000_000_000;
    const range = { from: oldest + 20_000, to: oldest + 100_000 };
    expect(shouldFetchOlderVela(range, undefined, oldest)).toBe(false);
    expect(shouldFetchOlderVela(range, range.from - 10, oldest)).toBe(false);
    expect(shouldFetchOlderVela(range, range.from + 30_000, oldest)).toBe(true);
    expect(shouldFetchOlderVela(range, range.from + 30_000, null)).toBe(false);
    expect(nextVelaHistoryDepth(VELA_MAX_HISTORY)).toBe(VELA_MAX_HISTORY);
  });
});

import { trimLiveChartBars } from '../src/charts/history-pagination';

describe('live chart bar limit', () => {
  it('keeps only the most recent candles once the cap is reached', () => {
    expect(trimLiveChartBars([1, 2, 3, 4], 2)).toEqual([3, 4]);
    expect(trimLiveChartBars([1, 2], 2)).toEqual([1, 2]);
  });
});
