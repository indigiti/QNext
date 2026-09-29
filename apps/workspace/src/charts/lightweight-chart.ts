import {
  CandlestickSeries,
  ColorType,
  HistogramSeries,
  createChart,
  type IChartApi,
  type ISeriesApi,
  type UTCTimestamp,
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

export class QNextLightweightChart {
  private readonly container: HTMLElement;
  private readonly provider: QNextProvider;
  private readonly ticker: string;
  private readonly timeframe: string;
  private readonly historyLimit: number;
  private chart?: IChartApi;
  private candleSeries?: ISeriesApi<'Candlestick', UTCTimestamp>;
  private volumeSeries?: ISeriesApi<'Histogram', UTCTimestamp>;
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
    this.volumeSeries = volumeSeries;

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

  destroy(): void {
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.resizeObserver?.disconnect();
    this.resizeObserver = undefined;
    this.chart?.remove();
    this.chart = undefined;
    this.candleSeries = undefined;
    this.volumeSeries = undefined;
  }
}
