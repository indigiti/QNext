import type { QNextBar } from '../qnext-provider';

// Requests are intentionally bounded: one chart fetches at most 500 candles
// from each of three non-overlapping calendar windows per user pan event.
// Weekend and holiday windows are skipped, without assuming missing SYN+
// bars can be reconstructed from an unrelated instrument.
const DAY_MS = 86_400_000;
export const OLDER_HISTORY_CHUNK = 500;
export const MAX_CHART_HISTORY = 12_000;

export function trimLiveChartBars<T>(bars: T[], max = MAX_CHART_HISTORY): T[] {
  return bars.length > max ? bars.slice(-max) : bars;
}
export const MAX_EMPTY_HISTORY_WINDOWS = 3;

export function historyPageSpanMs(timeframe: string): number {
  const match = /^(\d+)(s|m|h|D|W|M)$/.exec(timeframe);
  if (!match) return 7 * DAY_MS;
  const unit = match[2];
  if (unit === 's' || unit === 'm') return 7 * DAY_MS;
  if (unit === 'h') return 28 * DAY_MS;
  if (unit === 'D') return 366 * DAY_MS;
  return 5 * 366 * DAY_MS;
}

export function mergeHistoricalBars(
  existing: readonly QNextBar[],
  earlier: readonly QNextBar[],
): QNextBar[] {
  const byTime = new Map<number, QNextBar>();
  for (const bar of earlier) byTime.set(bar.time, bar);
  // Prefer the existing series for overlaps (it can contain fresher live updates).
  for (const bar of existing) byTime.set(bar.time, bar);
  return [...byTime.values()].sort((a, b) => a.time - b.time);
}

// Apply only explicit newer revisions to an already-visible historical bar.
// Synthetic 1m proxy finalization can follow the first tick of the next
// minute. Never insert an unknown timestamp or let a stale replay overwrite it.
export function applyRevisedHistoricalBar(
  bars: QNextBar[],
  update: QNextBar,
): boolean {
  if (!Number.isFinite(update.time) || !Number.isFinite(update.revision)) return false;
  const index = bars.findIndex((bar) => bar.time === update.time);
  if (index < 0 || (update.revision ?? 0) <= (bars[index].revision ?? 0)) return false;
  bars[index] = update;
  return true;
}

export interface OlderHistoryPage {
  bars: QNextBar[];
  nextCursor: number;
  exhausted: boolean;
}

export async function loadOlderHistoryPage(
  earliest: number,
  timeframe: string,
  load: (from: number, to: number, limit: number) => Promise<QNextBar[]>,
): Promise<OlderHistoryPage> {
  const span = historyPageSpanMs(timeframe);
  let cursor = earliest;
  for (let attempt = 0; attempt < MAX_EMPTY_HISTORY_WINDOWS; attempt += 1) {
    const from = Math.max(0, cursor - span);
    const bars = (await load(from, cursor, OLDER_HISTORY_CHUNK))
      .filter((bar) => Number.isFinite(bar.time) && bar.time >= from && bar.time < cursor);
    if (bars.length) {
      return { bars, nextCursor: Math.min(...bars.map((bar) => bar.time)), exhausted: false };
    }
    if (from === 0) break;
    cursor = from;
  }
  return { bars: [], nextCursor: cursor, exhausted: true };
}

// Vela may perform a depth-only re-load on a viewport edge. Never let one
// user pan request more than a bounded total depth from a chart.
export const VELA_INITIAL_HISTORY = 1500;
export const VELA_HISTORY_STEP = 1500;
export const VELA_MAX_HISTORY = 12000;
export function nextVelaHistoryDepth(current: number): number {
  return Math.min(VELA_MAX_HISTORY, current + VELA_HISTORY_STEP);
}
