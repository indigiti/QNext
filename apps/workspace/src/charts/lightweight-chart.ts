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

import { QNextProvider, type QNextBar } from '../qnext-provider';
import { toCandlestickData, toVolumeData } from './lightweight-adapter';
import { loadOlderHistoryPage, mergeHistoricalBars } from './history-pagination';
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

  constructor(options: LightweightChartOptions) {
    this.container = options.container;
    this.provider = options.provider;
    this.ticker = options.ticker;
    this.timeframe = options.timeframe;
    this.historyLimit = options.historyLimit ?? 1_000;
  }

  async mount(): Promise<void> {
    if (this.chart) return;

    const chart = createChart(this.container, {
      width: this.container.clientWidth,
      height: this.container.clientHeight,
      layout: {
        background: { type: ColorType.Solid, color: '#0b0d12' },
        textColor: '#d5d9e0',
      },
      grid: {
        vertLines: { color: '#1a1f2b' },
        horzLines: { color: '#1a1f2b' },
      },
      rightPriceScale: {
        borderColor: '#2a3140',
      },
      timeScale: {
        borderColor: '#2a3140',
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
        if (last && bar.time === last.time) this.bars[this.bars.length - 1] = bar;
        else this.bars.push(bar);
        candleSeries.update(toCandlestickData(bar));
        const volume = toVolumeData(bar);
        if (volume) volumeSeries.update(volume);
      },
    );

    this.resizeObserver = new ResizeObserver(() => {
      chart.applyOptions({
        width: this.container.clientWidth,
        height: this.container.clientHeight,
      });
    });
    this.resizeObserver.observe(this.container);
  }

  private async fetchEarlier(): Promise<void> {
    if (this.disposed || this.historyExhausted || this.historyInFlight || !this.bars.length) return;
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
      const merged = mergeHistoricalBars(this.bars, page.bars);
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
      this.container.dataset.qnextHistory = 'loaded';
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
    if (this.historyDebounce) clearTimeout(this.historyDebounce);
    this.historyDebounce = undefined;
    this.unbindHistory?.();
    this.unbindHistory = undefined;
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.resizeObserver?.disconnect();
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
