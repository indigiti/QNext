import { QNextProvider } from '../qnext-provider';
import {
  QNextLightweightChart,
  type LightweightCrosshairPoint,
  type LightweightVisibleTimeRange,
} from './lightweight-chart';

export type LightweightLayout = 1 | 2 | 4;

export interface LightweightSymbol {
  ticker: string;
  description?: string;
  type?: string;
  prefix?: string;
}

export interface LightweightPaneConfig {
  ticker: string;
  timeframe: string;
}

export interface LightweightWorkspaceOptions {
  container: HTMLElement;
  provider: QNextProvider;
  symbols: LightweightSymbol[];
  timeframes: string[];
  initialTicker: string;
  initialTimeframe: string;
  initialLayout?: LightweightLayout;
  syncCharts?: boolean;
}

export function normalizeLightweightLayout(value: unknown): LightweightLayout {
  return value === 2 || value === '2' ? 2 : value === 4 || value === '4' ? 4 : 1;
}

export function buildLightweightPaneConfigs(
  layout: LightweightLayout,
  current: LightweightPaneConfig[],
  symbols: LightweightSymbol[],
  fallback: LightweightPaneConfig,
): LightweightPaneConfig[] {
  const configs = current.slice(0, layout).map((config) => ({ ...config }));
  while (configs.length < layout) {
    const index = configs.length;
    const symbol = symbols[index % Math.max(symbols.length, 1)];
    configs.push({
      ticker: symbol?.ticker ?? fallback.ticker,
      timeframe: fallback.timeframe,
    });
  }
  return configs;
}

export class QNextLightweightWorkspace {
  private readonly container: HTMLElement;
  private readonly provider: QNextProvider;
  private readonly symbols: LightweightSymbol[];
  private readonly timeframes: string[];
  private readonly fallback: LightweightPaneConfig;
  private layout: LightweightLayout;
  private syncCharts: boolean;
  private activePane = 0;
  private paneConfigs: LightweightPaneConfig[];
  private charts: QNextLightweightChart[] = [];
  private paneElements: HTMLElement[] = [];
  private syncDisposers: Array<() => void> = [];
  private shell?: HTMLElement;
  private grid?: HTMLElement;
  private symbolSelect?: HTMLSelectElement;
  private timeframeSelect?: HTMLSelectElement;
  private layoutSelect?: HTMLSelectElement;
  private syncInput?: HTMLInputElement;

  constructor(options: LightweightWorkspaceOptions) {
    this.container = options.container;
    this.provider = options.provider;
    this.symbols = options.symbols.length > 0
      ? options.symbols
      : [{ ticker: options.initialTicker }];
    this.timeframes = options.timeframes;
    this.fallback = {
      ticker: options.initialTicker,
      timeframe: options.initialTimeframe,
    };
    this.layout = options.initialLayout ?? 1;
    this.syncCharts = options.syncCharts ?? true;
    this.paneConfigs = buildLightweightPaneConfigs(
      this.layout,
      [this.fallback],
      this.symbols,
      this.fallback,
    );
  }

  async mount(): Promise<void> {
    this.renderShell();
    await this.rebuildAllCharts();
  }

  destroy(): void {
    this.unbindSync();
    for (const chart of this.charts) chart.destroy();
    this.charts = [];
    this.paneElements = [];
    this.container.replaceChildren();
    this.shell = undefined;
    this.grid = undefined;
  }

  private renderShell(): void {
    this.container.replaceChildren();
    this.container.dataset.chartEngine = 'lightweight';

    const shell = document.createElement('div');
    shell.className = 'qnext-lightweight-shell';

    const toolbar = document.createElement('div');
    toolbar.className = 'qnext-lightweight-toolbar';

    const brand = document.createElement('div');
    brand.className = 'qnext-lightweight-brand';
    brand.textContent = 'QNext Lightweight';

    const symbolSelect = document.createElement('select');
    symbolSelect.className = 'qnext-lightweight-select qnext-symbol-select';
    symbolSelect.setAttribute('aria-label', 'Active chart symbol');
    for (const symbol of this.symbols) {
      const option = document.createElement('option');
      option.value = symbol.ticker;
      option.textContent = symbol.description
        ? `${symbol.ticker} — ${symbol.description}`
        : symbol.ticker;
      symbolSelect.append(option);
    }
    symbolSelect.addEventListener('change', () => {
      const config = this.paneConfigs[this.activePane];
      if (!config || config.ticker === symbolSelect.value) return;
      config.ticker = symbolSelect.value;
      void this.rebuildPane(this.activePane);
    });

    const timeframeSelect = document.createElement('select');
    timeframeSelect.className = 'qnext-lightweight-select qnext-timeframe-select';
    timeframeSelect.setAttribute('aria-label', 'Active chart timeframe');
    for (const timeframe of this.timeframes) {
      const option = document.createElement('option');
      option.value = timeframe;
      option.textContent = timeframe;
      timeframeSelect.append(option);
    }
    timeframeSelect.addEventListener('change', () => {
      const config = this.paneConfigs[this.activePane];
      if (!config || config.timeframe === timeframeSelect.value) return;
      config.timeframe = timeframeSelect.value;
      void this.rebuildPane(this.activePane);
    });

    const layoutSelect = document.createElement('select');
    layoutSelect.className = 'qnext-lightweight-select qnext-layout-select';
    layoutSelect.setAttribute('aria-label', 'Chart layout');
    for (const value of [1, 2, 4] as const) {
      const option = document.createElement('option');
      option.value = String(value);
      option.textContent = value === 1 ? '1 chart' : `${value} charts`;
      layoutSelect.append(option);
    }
    layoutSelect.value = String(this.layout);
    layoutSelect.addEventListener('change', () => {
      const nextLayout = normalizeLightweightLayout(layoutSelect.value);
      if (nextLayout === this.layout) return;
      this.layout = nextLayout;
      this.activePane = Math.min(this.activePane, nextLayout - 1);
      this.paneConfigs = buildLightweightPaneConfigs(
        nextLayout,
        this.paneConfigs,
        this.symbols,
        this.fallback,
      );
      void this.rebuildAllCharts();
    });

    const syncLabel = document.createElement('label');
    syncLabel.className = 'qnext-lightweight-sync';
    const syncInput = document.createElement('input');
    syncInput.type = 'checkbox';
    syncInput.checked = this.syncCharts;
    syncInput.setAttribute('aria-label', 'Synchronize charts');
    const syncText = document.createElement('span');
    syncText.textContent = 'Sync';
    syncLabel.append(syncInput, syncText);
    syncInput.addEventListener('change', () => {
      this.syncCharts = syncInput.checked;
      this.bindSync();
    });

    const engineBadge = document.createElement('span');
    engineBadge.className = 'qnext-lightweight-engine-badge';
    engineBadge.textContent = 'LIVE · WSS';

    toolbar.append(
      brand,
      symbolSelect,
      timeframeSelect,
      layoutSelect,
      syncLabel,
      engineBadge,
    );

    const grid = document.createElement('div');
    grid.className = 'qnext-lightweight-grid';

    shell.append(toolbar, grid);
    this.container.append(shell);

    this.shell = shell;
    this.grid = grid;
    this.symbolSelect = symbolSelect;
    this.timeframeSelect = timeframeSelect;
    this.layoutSelect = layoutSelect;
    this.syncInput = syncInput;
    this.updateToolbarFromActivePane();
  }

  private async rebuildAllCharts(): Promise<void> {
    this.unbindSync();
    for (const chart of this.charts) chart.destroy();
    this.charts = [];
    this.paneElements = [];

    const grid = this.grid;
    if (!grid) return;
    grid.replaceChildren();
    grid.dataset.layout = String(this.layout);

    for (let index = 0; index < this.paneConfigs.length; index += 1) {
      const pane = this.createPaneElement(index);
      grid.append(pane.wrapper);
      this.paneElements.push(pane.wrapper);

      const config = this.paneConfigs[index];
      const chart = new QNextLightweightChart({
        container: pane.host,
        provider: this.provider,
        ticker: config.ticker,
        timeframe: config.timeframe,
      });
      this.charts.push(chart);
    }

    await Promise.all(this.charts.map((chart) => chart.mount()));
    this.activatePane(this.activePane);
    this.bindSync();
  }

  private async rebuildPane(index: number): Promise<void> {
    const grid = this.grid;
    const oldChart = this.charts[index];
    const oldPane = this.paneElements[index];
    const config = this.paneConfigs[index];
    if (!grid || !oldChart || !oldPane || !config) return;

    this.unbindSync();
    oldChart.destroy();

    const pane = this.createPaneElement(index);
    oldPane.replaceWith(pane.wrapper);
    this.paneElements[index] = pane.wrapper;

    const chart = new QNextLightweightChart({
      container: pane.host,
      provider: this.provider,
      ticker: config.ticker,
      timeframe: config.timeframe,
    });
    this.charts[index] = chart;
    await chart.mount();
    this.activatePane(index);
    this.bindSync();
  }

  private createPaneElement(index: number): { wrapper: HTMLElement; host: HTMLElement } {
    const config = this.paneConfigs[index];
    const wrapper = document.createElement('section');
    wrapper.className = 'qnext-lightweight-pane';
    wrapper.dataset.paneIndex = String(index);
    wrapper.addEventListener('pointerdown', () => this.activatePane(index));

    const header = document.createElement('div');
    header.className = 'qnext-lightweight-pane-header';

    const title = document.createElement('span');
    title.className = 'qnext-lightweight-pane-title';
    title.textContent = `${config.ticker} · ${config.timeframe}`;

    const status = document.createElement('span');
    status.className = 'qnext-lightweight-pane-status';
    status.textContent = `Chart ${index + 1}`;

    header.append(title, status);

    const host = document.createElement('div');
    host.className = 'qnext-lightweight-chart';

    wrapper.append(header, host);
    return { wrapper, host };
  }

  private activatePane(index: number): void {
    if (index < 0 || index >= this.paneConfigs.length) return;
    this.activePane = index;
    this.paneElements.forEach((pane, paneIndex) => {
      pane.classList.toggle('is-active', paneIndex === index);
    });
    this.updateToolbarFromActivePane();
  }

  private updateToolbarFromActivePane(): void {
    const config = this.paneConfigs[this.activePane];
    if (!config) return;
    if (this.symbolSelect) this.symbolSelect.value = config.ticker;
    if (this.timeframeSelect) this.timeframeSelect.value = config.timeframe;
    if (this.layoutSelect) this.layoutSelect.value = String(this.layout);
    if (this.syncInput) this.syncInput.checked = this.syncCharts;
  }

  private bindSync(): void {
    this.unbindSync();
    if (!this.syncCharts || this.charts.length < 2) return;

    let syncingCrosshair = false;
    let syncingRange = false;

    this.charts.forEach((source, sourceIndex) => {
      this.syncDisposers.push(
        source.subscribeCrosshair((point: LightweightCrosshairPoint | null) => {
          if (syncingCrosshair) return;
          syncingCrosshair = true;
          try {
            this.charts.forEach((target, targetIndex) => {
              if (targetIndex !== sourceIndex) target.setCrosshair(point);
            });
          } finally {
            syncingCrosshair = false;
          }
        }),
      );

      this.syncDisposers.push(
        source.subscribeVisibleTimeRange((range: LightweightVisibleTimeRange | null) => {
          if (syncingRange || !range) return;
          syncingRange = true;
          try {
            this.charts.forEach((target, targetIndex) => {
              if (targetIndex !== sourceIndex) target.setVisibleTimeRange(range);
            });
          } finally {
            syncingRange = false;
          }
        }),
      );
    });
  }

  private unbindSync(): void {
    for (const dispose of this.syncDisposers) dispose();
    this.syncDisposers = [];
    for (const chart of this.charts) chart.setCrosshair(null);
  }
}
