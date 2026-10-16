import {
  CandlestickSeries,
  ColorType,
  HistogramSeries,
  createChart,
  createSeriesMarkers,
  type IChartApi,
  type ISeriesApi,
  type Time,
} from 'lightweight-charts';

import { installChartResumeRecovery } from './chart-resume';
import { QNextProvider, type QNextBar } from '../qnext-provider';
import { toCandlestickData, toVolumeData } from './lightweight-adapter';
import { loadOlderHistoryPage, mergeHistoricalBars, MAX_CHART_HISTORY, trimLiveChartBars } from './history-pagination';
import {
  toLightweightPaperMarker,
  type QNextPaperMarker,
} from './paper-markers';

export interface LightweightChartOptions {
  container: HTMLElement;
  provider: QNextProvider;
  ticker: string;
  timeframe: string;
  historyLimit?: number;
}

export interface LightweightCrosshairPoint {
  time: Time;
  price: number;
}

export interface LightweightVisibleTimeRange {
  from: Time;
  to: Time;
}

export class QNextLightweightChart {
  private readonly container: HTMLElement;
  private readonly provider: QNextProvider;
  readonly ticker: string;
  readonly timeframe: string;
  private readonly historyLimit: number;
  private chart?: IChartApi;
  private candleSeries?: ISeriesApi<'Candlestick'>;
  private volumeSeries?: ISeriesApi<'Histogram'>;
  private bars: QNextBar[] = [];
  private historyExhausted = false;
  private historyInFlight = false;
  private lastLogicalFrom?: number;
  private historyDebounce?: ReturnType<typeof setTimeout>;
  private unbindHistory?: () => void;
  private disposed = false;
  private markerSetter?: (markers: QNextPaperMarker[]) => void;
  private unsubscribe?: () => void;
  private resizeObserver?: ResizeObserver;
  private themeListener?: () => void;
  private unbindResume?: () => void;
  private resumeInFlight = false;

  constructor(options: LightweightChartOptions) {
    this.container = options.container;
    this.provider = options.provider;
    this.ticker = options.ticker;
    this.timeframe = options.timeframe;
    this.historyLimit = options.historyLimit ?? 1_000;
  }

  async mount(): Promise<void> {
    if (this.chart) return;

    const palette = chartPalette();
    const chart = createChart(this.container, {
      width: this.container.clientWidth,
      height: this.container.clientHeight,
      layout: {
        background: { type: ColorType.Solid, color: palette.background },
        textColor: palette.text,
      },
      grid: {
        vertLines: { color: palette.grid },
        horzLines: { color: palette.grid },
      },
      rightPriceScale: {
        borderColor: palette.border,
      },
      timeScale: {
        borderColor: palette.border,
        timeVisible: true,
        secondsVisible: true,
      },
      crosshair: {
        mode: 0,
      },
    });

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#26a69a',
      downColor: '#ef5350',
      borderVisible: false,
      wickUpColor: '#26a69a',
      wickDownColor: '#ef5350',
    });
    const markerPrimitive = createSeriesMarkers(candleSeries, []);

    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.82, bottom: 0 },
    });

    this.themeListener = () => {
      const next = chartPalette();
      chart.applyOptions({
        layout: { background: { type: ColorType.Solid, color: next.background }, textColor: next.text },
        grid: { vertLines: { color: next.grid }, horzLines: { color: next.grid } },
        rightPriceScale: { borderColor: next.border },
        timeScale: { borderColor: next.border },
      });
    };
    window.addEventListener('qnext-theme-change', this.themeListener);
    this.chart = chart;
    this.candleSeries = candleSeries;
    this.volumeSeries = volumeSeries;
    this.markerSetter = (markers) => {
      markerPrimitive.setMarkers(markers.map(toLightweightPaperMarker));
    };

    const bars = await this.provider.getBars(this.ticker, this.timeframe, {
      limit: this.historyLimit,
    });
    if (this.disposed) return;
    this.bars = bars;
    candleSeries.setData(bars.map(toCandlestickData));
    volumeSeries.setData(bars.map(toVolumeData).filter((bar) => bar !== undefined));
    chart.timeScale().fitContent();
    const scale = chart.timeScale();
    const onPan: Parameters<typeof scale.subscribeVisibleLogicalRangeChange>[0] = (range) => {
      if (!range || this.disposed || !this.bars.length) return;
      const previous = this.lastLogicalFrom;
      this.lastLogicalFrom = range.from;
      // Initial fitContent does not fetch. Only leftward user navigation does.
      if (previous === undefined || range.from >= previous - 1 || range.from > 150) return;
      if (this.historyDebounce) clearTimeout(this.historyDebounce);
      this.historyDebounce = setTimeout(() => void this.fetchEarlier(), 120);
    };
    scale.subscribeVisibleLogicalRangeChange(onPan);
    this.unbindHistory = () => scale.unsubscribeVisibleLogicalRangeChange(onPan);

    this.unsubscribe = this.provider.subscribe(
      this.ticker,
      this.timeframe,
      (bar: QNextBar) => {
        const last = this.bars[this.bars.length - 1];
        if (last && bar.time < last.time) return;
        const correctingCurrent = Boolean(last && bar.time === last.time);
        if (correctingCurrent) this.bars[this.bars.length - 1] = bar;
        else this.bars.push(bar);
        const exceeded = this.bars.length > MAX_CHART_HISTORY;
        if (exceeded) {
          this.bars = trimLiveChartBars(this.bars);
          this.historyExhausted = true;
          candleSeries.setData(this.bars.map(toCandlestickData));
          volumeSeries.setData(this.bars.map(toVolumeData).filter((point) => point !== undefined));
        } else {
          candleSeries.update(toCandlestickData(bar));
          if (volumeIsKnown(bar)) {
            const volumePoint = toVolumeData(bar);
            if (volumePoint) volumeSeries.update(volumePoint);
          } else if (correctingCurrent && last?.volume !== undefined) {
            // Missing is NOT zero. Remove a stale histogram point instead of fabricating volume.
            volumeSeries.setData(this.bars.map(toVolumeData).filter((point) => point !== undefined));
          }
        }
      },
    );

    this.unbindResume = installChartResumeRecovery(() => void this.reconcileOnResume());

    this.resizeObserver = new ResizeObserver(() => {
      chart.applyOptions({
        width: this.container.clientWidth,
        height: this.container.clientHeight,
      });
    });
    this.resizeObserver.observe(this.container);
  }

  private async reconcileOnResume(): Promise<void> {
    if (this.disposed || this.resumeInFlight || !this.bars.length) return;
    this.resumeInFlight = true;
    try {
      const previousLast = this.bars[this.bars.length - 1].time;
      const now = Date.now();
      // Only recover recent chart history. Preserve the loaded historical viewport.
      const from = this.bars[0].time;
      const fresh = await this.provider.getBars(this.ticker, this.timeframe, {
        from,
        to: now,
        limit: 5000,
      });
      if (this.disposed || !fresh.length) return;
      // Take the snapshot after awaiting HTTP so concurrent live updates survive.
      const liveByTime = new Map(this.bars.map((bar) => [bar.time, bar]));
      // Closed canonical bars are authoritative; retain a potentially fresher
      // forming live candle when both sources cover the current bucket.
      for (const bar of fresh) {
        if (bar.time < previousLast || !liveByTime.has(bar.time)) {
          liveByTime.set(bar.time, bar);
        }
      }
      const merged = trimLiveChartBars([...liveByTime.values()].sort((a, b) => a.time - b.time));
      const scale = this.chart?.timeScale();
      const visible = scale?.getVisibleRange();
      this.bars = merged;
      this.candleSeries?.setData(merged.map(toCandlestickData));
      this.volumeSeries?.setData(merged.map(toVolumeData).filter((bar) => bar !== undefined));
      if (visible && scale) {
        try { scale.setVisibleRange(visible); } catch { /* viewport outside retained history */ }
      }
    } catch (error) {
      if (!this.disposed) console.warn('QNext activation history reconciliation failed', error);
    } finally {
      this.resumeInFlight = false;
    }
  }

  private async fetchEarlier(): Promise<void> {
    if (this.disposed || this.historyExhausted || this.historyInFlight || !this.bars.length) return;
    if (this.bars.length >= MAX_CHART_HISTORY) {
      this.historyExhausted = true;
      this.container.dataset.qnextHistory = 'depth-limit';
      this.container.title = 'Loaded the maximum 12,000 chart candles';
      return;
    }
    const firstTime = this.bars[0].time;
    this.historyInFlight = true;
    this.container.dataset.qnextHistory = 'loading';
    try {
      const page = await loadOlderHistoryPage(
        firstTime, this.timeframe,
        (from, to, limit) => this.provider.getBars(this.ticker, this.timeframe, { from, to, limit }),
      );
      if (this.disposed) return;
      if (page.exhausted) {
        this.historyExhausted = true;
        this.container.dataset.qnextHistory = 'unavailable';
        this.container.title = 'No older candles found in the configured history search window';
        return;
      }
      const previousFirst = this.bars[0].time;
      const combined = mergeHistoricalBars(this.bars, page.bars);
      const merged = combined.slice(-MAX_CHART_HISTORY);
      const prepended = merged.filter((bar) => bar.time < previousFirst).length;
      if (!prepended) {
        this.historyExhausted = true;
        this.container.dataset.qnextHistory = 'unavailable';
        return;
      }
      const chart = this.chart;
      const scale = chart?.timeScale();
      const visible = scale?.getVisibleLogicalRange();
      this.bars = merged;
      this.candleSeries?.setData(merged.map(toCandlestickData));
      this.volumeSeries?.setData(merged.map(toVolumeData).filter((bar) => bar !== undefined));
      if (visible && scale) {
        // Logical coordinates shift when earlier bars are prepended.
        scale.setVisibleLogicalRange({
          from: visible.from + prepended,
          to: visible.to + prepended,
        });
      }
      if (merged.length >= MAX_CHART_HISTORY) {
        this.historyExhausted = true;
        this.container.dataset.qnextHistory = 'depth-limit';
        this.container.title = 'Loaded the maximum 12,000 chart candles';
      } else {
        this.container.dataset.qnextHistory = 'loaded';
      }
    } catch (error) {
      if (!this.disposed) {
        this.container.dataset.qnextHistory = 'retryable-error';
        console.warn('QNext historical pan fetch failed', error);
      }
    } finally {
      this.historyInFlight = false;
    }
  }

  setPaperMarkers(markers: QNextPaperMarker[]): void {
    this.markerSetter?.(markers);
  }

  subscribeCrosshair(
    listener: (point: LightweightCrosshairPoint | null) => void,
  ): () => void {
    const chart = this.chart;
    const series = this.candleSeries;
    if (!chart || !series) return () => undefined;

    const handler: Parameters<IChartApi['subscribeCrosshairMove']>[0] = (param) => {
      if (!param.time) {
        listener(null);
        return;
      }
      const data = param.seriesData.get(series);
      if (!data || !('close' in data) || typeof data.close !== 'number') {
        listener(null);
        return;
      }
      listener({ time: param.time, price: data.close });
    };

    chart.subscribeCrosshairMove(handler);
    return () => chart.unsubscribeCrosshairMove(handler);
  }

  setCrosshair(point: LightweightCrosshairPoint | null): void {
    const chart = this.chart;
    const series = this.candleSeries;
    if (!chart || !series) return;
    if (!point) {
      chart.clearCrosshairPosition();
      return;
    }
    chart.setCrosshairPosition(point.price, point.time, series);
  }

  subscribeVisibleTimeRange(
    listener: (range: LightweightVisibleTimeRange | null) => void,
  ): () => void {
    const chart = this.chart;
    if (!chart) return () => undefined;
    const timeScale = chart.timeScale();
    const handler: Parameters<typeof timeScale.subscribeVisibleTimeRangeChange>[0] =
      (range) => listener(range);
    timeScale.subscribeVisibleTimeRangeChange(handler);
    return () => timeScale.unsubscribeVisibleTimeRangeChange(handler);
  }

  setVisibleTimeRange(range: LightweightVisibleTimeRange | null): void {
    const chart = this.chart;
    if (!chart || !range) return;
    chart.timeScale().setVisibleRange(range);
  }

  destroy(): void {
    this.disposed = true;
    this.unbindResume?.();
    this.unbindResume = undefined;
    if (this.historyDebounce) clearTimeout(this.historyDebounce);
    this.historyDebounce = undefined;
    this.unbindHistory?.();
    this.unbindHistory = undefined;
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.resizeObserver?.disconnect();
    if (this.themeListener) window.removeEventListener('qnext-theme-change', this.themeListener);
    this.themeListener = undefined;
    this.resizeObserver = undefined;
    this.markerSetter?.([]);
    this.markerSetter = undefined;
    this.chart?.remove();
    this.chart = undefined;
    this.candleSeries = undefined;
    this.volumeSeries = undefined;
    this.bars = [];
  }
}

function chartPalette(): { background: string; text: string; grid: string; border: string } {
  return document.documentElement.dataset.qnextTheme === 'light'
    ? { background: '#ffffff', text: '#26364c', grid: '#e4e9f0', border: '#c3cedc' }
    : { background: '#0b0d12', text: '#d5d9e0', grid: '#1a1f2b', border: '#2a3140' };
}

function volumeIsKnown(bar: QNextBar): boolean {
  return bar.volume !== undefined && Number.isFinite(bar.volume) && bar.volume >= 0;
}
