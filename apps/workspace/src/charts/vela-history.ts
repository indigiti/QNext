import { installChartResumeRecovery } from './chart-resume';
import type { VelaWorkspace } from '@luxalgo/vela/workspace';
import { nextVelaHistoryDepth, VELA_INITIAL_HISTORY } from './history-pagination';

// Vela 0.7.7 can expand history in place with setMarket({ bars, visibleRange }).
// It owns its indicator/drawing history; never inject bars or replace the
// chart itself. Only load more after the user pans toward the oldest candle.
export function shouldFetchOlderVela(
  range: { from: number; to: number },
  previousFrom: number | undefined,
  oldest: number | null,
): boolean {
  if (oldest === null || previousFrom === undefined) return false;
  if (!Number.isFinite(range.from) || !Number.isFinite(range.to)) return false;
  const width = range.to - range.from;
  if (width <= 0 || range.from >= previousFrom - 1) return false;
  return range.from <= oldest + width * 0.65;
}

export function installVelaPanHistory(workspace: VelaWorkspace): () => void {
  const attachments = new Map<object, Array<() => void>>();
  const timers = new Map<object, ReturnType<typeof setTimeout>>();
  const recovering = new Set<object>();

  function synchronize(): void {
    const liveCharts = new Set(workspace.cells().map((cell) => cell.chart));
    for (const [chart, unbind] of attachments) {
      if (liveCharts.has(chart as typeof workspace.chart)) continue;
      unbind.forEach((dispose) => dispose());
      const timer = timers.get(chart);
      if (timer) clearTimeout(timer);
      timers.delete(chart);
      attachments.delete(chart);
    }

    for (const cell of workspace.cells()) {
      const chart = cell.chart;
      if (attachments.has(chart)) continue;

      let oldest: number | null = null;
      let lastFrom: number | undefined;
      let depth = chart.market.bars ?? VELA_INITIAL_HISTORY;
      let lastLoaded = 0;
      let beforeLoad = 0;
      let inFlight = false;
      let exhausted = false;

      const disposers = [
        chart.on('history:complete', (event) => {
          if (Number.isFinite(event.oldestTime)) oldest = event.oldestTime;
          else oldest = null;
          if (inFlight && event.barsLoaded <= beforeLoad) exhausted = true;
          if (event.reason === 'genesis') exhausted = true;
          if (event.reason === 'aborted') {
            inFlight = false;
            exhausted = true; // no repeated HTTP retries while dragging
          }
          lastLoaded = event.barsLoaded;
          inFlight = false;
        }),
        chart.on('market:changed', () => {
          oldest = null;
          lastFrom = undefined;
          depth = chart.market.bars ?? VELA_INITIAL_HISTORY;
          lastLoaded = 0;
          inFlight = false;
          exhausted = false;
          const timer = timers.get(chart);
          if (timer) clearTimeout(timer);
          timers.delete(chart);
        }),
        chart.on('viewport:changed', (range) => {
          const previous = lastFrom;
          lastFrom = range.from;
          if (inFlight || exhausted ||
              !shouldFetchOlderVela(range, previous, oldest)) return;
          const timer = timers.get(chart);
          if (timer) clearTimeout(timer);
          timers.set(chart, setTimeout(() => {
            timers.delete(chart);
            if (inFlight || exhausted) return;
            const target = nextVelaHistoryDepth(depth);
            if (target <= depth) {
              exhausted = true;
              return;
            }
            const savedRange = chart.getVisibleRange();
            beforeLoad = lastLoaded;
            depth = target;
            inFlight = true;
            void chart.setMarket({
              bars: target,
              ...(savedRange ? { visibleRange: savedRange } : {}),
            }).catch((error) => {
              inFlight = false;
              exhausted = true;
              console.warn('QNext Vela historical pan load failed', error);
            });
          }, 140));
        }),
      ];
      attachments.set(chart, disposers);
    }
  }

  synchronize();
  const unbindResume = installChartResumeRecovery(() => {
    for (const cell of workspace.cells()) {
      const chart = cell.chart;
      if (recovering.has(chart)) continue;
      recovering.add(chart);
      const savedRange = chart.getVisibleRange();
      void chart.setMarket({
        bars: chart.market.bars ?? VELA_INITIAL_HISTORY,
        ...(savedRange ? { visibleRange: savedRange } : {}),
      }).catch((error) => {
        console.warn('QNext Vela activation refresh failed', error);
      }).finally(() => recovering.delete(chart));
    }
  });
  const created = workspace.on('cell:created', synchronize);
  const destroyed = workspace.on('cell:destroyed', synchronize);
  return () => {
    created();
    destroyed();
    unbindResume();
    for (const unbind of attachments.values()) unbind.forEach((fn) => fn());
    for (const timer of timers.values()) clearTimeout(timer);
    attachments.clear();
    timers.clear();
  };
}
