import type { VelaWorkspace } from '@luxalgo/vela/workspace';

export interface TraceBar {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume?: number;
  revision?: number;
}

export type TraceEvent =
  | { seq: number; at: number; kind: 'provider-bar' | 'vela-bar'; symbol: string; timeframe: string; bar: TraceBar; olderThanNewest: boolean }
  | { seq: number; at: number; kind: 'pine-run'; cell: string; indicator: string; cause: string; barTime: number; forming: boolean }
  | { seq: number; at: number; kind: 'indicator-error'; symbol: string; timeframe: string; indicatorId: string; message: string };

/**
 * Local-only, opt-in, bounded diagnostic for comparing provider → Vela → Pine.
 * No polling, historical reload, network export, or changes to indicator outputs.
 */
export class VelaSignalTrace {
  private readonly history: TraceEvent[] = [];
  private readonly newestProviderTime = new Map<string, number>();
  private readonly newestVelaTime = new Map<string, number>();
  private nextSeq = 0;
  private maxEvents = 300;

  providerBar(symbol: string, timeframe: string, bar: TraceBar): void {
    this.bar('provider-bar', symbol, timeframe, bar, this.newestProviderTime);
  }

  velaBar(symbol: string, timeframe: string, bar: TraceBar): void {
    this.bar('vela-bar', symbol, timeframe, bar, this.newestVelaTime);
  }

  pineRun(cell: string, indicator: string, cause: string, barTime: number, forming: boolean): void {
    this.push({ kind: 'pine-run', cell, indicator, cause, barTime, forming });
  }

  indicatorError(symbol: string, timeframe: string, indicatorId: string, error: Error): void {
    this.push({ kind: 'indicator-error', symbol, timeframe, indicatorId, message: error.message });
  }

  snapshot(): TraceEvent[] {
    return this.history.map((event) => structuredClone(event));
  }

  summary(): { total: number; providerBars: number; velaBars: number; olderProviderBars: number; pineRuns: number; indicatorErrors: number } {
    return {
      total: this.history.length,
      providerBars: this.history.filter((v) => v.kind === 'provider-bar').length,
      velaBars: this.history.filter((v) => v.kind === 'vela-bar').length,
      olderProviderBars: this.history.filter((v) => v.kind === 'provider-bar' && v.olderThanNewest).length,
      pineRuns: this.history.filter((v) => v.kind === 'pine-run').length,
      indicatorErrors: this.history.filter((v) => v.kind === 'indicator-error').length,
    };
  }

  private bar(kind: 'provider-bar' | 'vela-bar', symbol: string, timeframe: string, bar: TraceBar, newest: Map<string, number>): void {
    if (!Number.isFinite(bar.time)) return;
    const key = symbol.trim().toUpperCase() + '|' + timeframe.trim();
    const latest = newest.get(key);
    const olderThanNewest = latest !== undefined && bar.time < latest;
    if (latest === undefined || bar.time > latest) newest.set(key, bar.time);
    this.push({
      kind, symbol, timeframe, olderThanNewest,
      bar: { time: bar.time, open: bar.open, high: bar.high, low: bar.low, close: bar.close,
        ...(bar.volume === undefined ? {} : { volume: bar.volume }),
        ...(bar.revision === undefined ? {} : { revision: bar.revision }) },
    });
  }

  private push(record: { kind: 'provider-bar' | 'vela-bar'; symbol: string; timeframe: string; bar: TraceBar; olderThanNewest: boolean } |
    { kind: 'pine-run'; cell: string; indicator: string; cause: string; barTime: number; forming: boolean } |
    { kind: 'indicator-error'; symbol: string; timeframe: string; indicatorId: string; message: string }): void {
    this.history.push({ ...record, seq: ++this.nextSeq, at: Date.now() } as TraceEvent);
    if (this.history.length > this.maxEvents) this.history.splice(0, this.history.length - this.maxEvents);
  }
}

/** Only attaches listeners when the local diagnostic is explicitly enabled. */
export function installVelaSignalTrace(workspace: VelaWorkspace, trace: VelaSignalTrace): () => void {
  const charts = new Map<object, Array<() => void>>();
  const synchronize = () => {
    const active = new Set(workspace.cells().map((cell) => cell.chart));
    for (const [chart, dispose] of charts) {
      if (active.has(chart as typeof workspace.chart)) continue;
      for (const fn of dispose) fn();
      charts.delete(chart);
    }
    for (const cell of workspace.cells()) {
      const chart = cell.chart;
      if (charts.has(chart)) continue;
      charts.set(chart, [
        chart.on('bar', (bar) => trace.velaBar(chart.market.symbol, chart.market.timeframe, bar)),
        chart.on('indicator:error', ({ id, error }) => trace.indicatorError(chart.market.symbol, chart.market.timeframe, id, error)),
      ]);
    }
  };
  synchronize();
  const offCreated = workspace.on('cell:created', synchronize);
  const offDestroyed = workspace.on('cell:destroyed', synchronize);
  const offRun = workspace.on('script:run', (run) => trace.pineRun(run.cell, run.title, run.cause, run.time, run.forming));
  return () => {
    offCreated();
    offDestroyed();
    offRun();
    for (const off of charts.values()) for (const fn of off) fn();
    charts.clear();
  };
}
