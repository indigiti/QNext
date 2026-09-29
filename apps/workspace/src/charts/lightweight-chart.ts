import {
  CandlestickSeries,
  ColorType,
  HistogramSeries,
  createChart,
  type IChartApi,
  type ISeriesApi,
  type Time,
} from 'lightweight-charts';

import { QNextProvider, type QNextBar } from '../qnext-provider';
import { toCandlestickData, toVolumeData } from './lightweight-adapter';

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

    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.82, bottom: 0 },
    });

    this.chart = chart;
    this.candleSeries = candleSeries;

    const bars = await this.provider.getBars(this.ticker, this.timeframe, {
      limit: this.historyLimit,
    });
    candleSeries.setData(bars.map(toCandlestickData));
    volumeSeries.setData(bars.map(toVolumeData).filter((bar) => bar !== undefined));
    chart.timeScale().fitContent();

    this.unsubscribe = this.provider.subscribe(
      this.ticker,
      this.timeframe,
      (bar: QNextBar) => {
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
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.resizeObserver?.disconnect();
    this.resizeObserver = undefined;
    this.chart?.remove();
    this.chart = undefined;
    this.candleSeries = undefined;
  }
}
