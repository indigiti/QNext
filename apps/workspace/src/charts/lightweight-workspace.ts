import { QNextProvider, type QNextTransportSnapshot } from '../qnext-provider';
import { catalogTimeframe } from '../symbol-catalog';
import { createSymbolSearch } from '../symbol-search-launcher';
import { mountThemeControl } from '../theme';
import { mountScreenAwakeButton, type ScreenAwakeController } from '../screen-awake';
import {
  QNextLightweightChart,
  type LightweightCrosshairPoint,
  type LightweightVisibleTimeRange,
} from './lightweight-chart';
import {
  QNextPaperMarkerClient,
  type QNextPaperMarkerSnapshot,
} from './paper-markers';

import { normalizeLightweightLayout, type LightweightLayout } from './chart-engine';
export { normalizeLightweightLayout, type LightweightLayout } from './chart-engine';

export function transportModeLabel(mode: QNextTransportSnapshot['mode']): string {
  switch (mode) {
    case 'wss': return 'WSS connected';
    case 'rest_fallback': return 'REST fallback';
    case 'connecting': return 'Connecting';
    default: return 'Idle';
  }
}

export interface LightweightSymbol {
  ticker: string;
  description?: string;
  type?: string;
  prefix?: string;
  instrument_id?: string;
  mode?: string;
}

export interface LightweightPaneConfig {
  ticker: string;
  timeframe: string;
}

export interface LightweightWorkspaceOptions {
  container: HTMLElement;
  provider: QNextProvider;
  screenAwake?: ScreenAwakeController;
  apiBase?: string;
  symbols: LightweightSymbol[];
  timeframes: string[];
  initialTicker: string;
  initialTimeframe: string;
  initialLayout?: LightweightLayout;
  syncCharts?: boolean;
  markerPollIntervalMs?: number;
  onActiveMarketChange?: () => void;
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
      ticker: symbol?.instrument_id ?? symbol?.ticker ?? fallback.ticker,
      timeframe: catalogTimeframe({mode: symbol?.mode ?? 'unknown'}, fallback.timeframe),
    });
  }
  return configs;
}

export function paperSnapshotMatchesChart(
  snapshot: QNextPaperMarkerSnapshot,
  ticker: string,
  timeframe: string,
): boolean {
  if (!snapshot.available || !snapshot.instrumentId || !snapshot.timeframe) return false;
  if (snapshot.timeframe !== timeframe) return false;

  const requested = ticker.trim().toUpperCase();
  const instrument = snapshot.instrumentId.trim().toUpperCase();
  if (requested === instrument) return true;

  const requestedBare = requested.includes(':') ? requested.slice(requested.indexOf(':') + 1) : requested;
  const instrumentBare = instrument.includes(':') ? instrument.slice(instrument.indexOf(':') + 1) : instrument;
  return requestedBare === instrumentBare;
}

export class QNextLightweightWorkspace {
  private readonly container: HTMLElement;
  private readonly provider: QNextProvider;
  private readonly screenAwake?: ScreenAwakeController;
  private disposeAwakeControl?: () => void;
  private readonly markerClient: QNextPaperMarkerClient;
  private readonly markerPollIntervalMs: number;
  private readonly onActiveMarketChange?: () => void;
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
  private markerTimer?: ReturnType<typeof setTimeout>;
  private markerPolling = false;
  private markerSignature = '';
  private transportBadge?: HTMLSpanElement;
  private transportTimer?: ReturnType<typeof setTimeout>;
  private shell?: HTMLElement;
  private grid?: HTMLElement;
  private readonly historyOnly = new Set<string>();
  private symbolSearch?: HTMLButtonElement;
  private disposeSymbolShortcut?: () => void;
  private timeframeSelect?: HTMLSelectElement;
  private layoutSelect?: HTMLSelectElement;
  private syncInput?: HTMLInputElement;

  constructor(options: LightweightWorkspaceOptions) {
    this.container = options.container;
    this.provider = options.provider;
    this.screenAwake = options.screenAwake;
    this.markerClient = new QNextPaperMarkerClient({ apiBase: options.apiBase });
    this.markerPollIntervalMs = options.markerPollIntervalMs ?? 2_000;
    this.onActiveMarketChange = options.onActiveMarketChange;
    // Consume the same real catalog as the search modal, with no parallel selector.
    this.symbols = options.symbols.filter(item => item.type !== 'root');
    for (const item of this.symbols) {
      if (item.mode === 'historical_1m') this.historyOnly.add(item.instrument_id ?? item.ticker);
    }
    this.timeframes = options.timeframes;
    this.fallback = {
      ticker: options.initialTicker,
      timeframe: this.historyOnly.has(options.initialTicker) ? '1m' : options.initialTimeframe,
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
    this.startMarkerPolling();
    this.refreshTransportBadge();
    this.transportTimer = setInterval(() => this.refreshTransportBadge(), 1000);
  }

  destroy(): void {
    this.disposeSymbolShortcut?.();
    this.disposeAwakeControl?.();
    this.disposeAwakeControl = undefined;
    this.stopMarkerPolling();
    if (this.transportTimer) clearInterval(this.transportTimer);
    this.transportTimer = undefined;
    this.unbindSync();
    for (const chart of this.charts) chart.destroy();
    this.charts = [];
    this.paneElements = [];
    this.container.replaceChildren();
    this.shell = undefined;
    this.grid = undefined;
    this.transportBadge = undefined;
  }

  private refreshTransportBadge(): void {
    if (!this.transportBadge) return;
    const state = this.provider.transportSnapshot();
    this.transportBadge.textContent = transportModeLabel(state.mode);
    this.transportBadge.dataset.transport = state.mode;
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

    const symbolSearch = document.createElement('button');
    symbolSearch.type = 'button';
    symbolSearch.className = 'qnext-lightweight-select qnext-symbol-search';
    symbolSearch.setAttribute('aria-label','Active chart symbol search');
    symbolSearch.textContent = 'Symbol Search';
    const search = createSymbolSearch(() => this.provider.listCatalog(), async entry => {
      const pane = this.activePane;
      const config = this.paneConfigs[pane];
      if (!config) throw new Error('No active chart');
      if (entry.mode === 'historical_1m') this.historyOnly.add(entry.instrument_id);
      else this.historyOnly.delete(entry.instrument_id);
      config.ticker = entry.instrument_id;
      config.timeframe = catalogTimeframe(entry,config.timeframe);
      await this.rebuildPane(pane);
      this.updateToolbarFromActivePane();
    });
    symbolSearch.onclick = search.open;
    this.disposeSymbolShortcut = search.dispose;

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
      const isNSEMinute = this.historyOnly.has(config.ticker);
      if (isNSEMinute && timeframeSelect.value !== '1m') {
        timeframeSelect.value = '1m';
        return;
      }
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
    engineBadge.textContent = 'Connecting';
    this.transportBadge = engineBadge;

    toolbar.append(
      brand,
      symbolSearch,
      timeframeSelect,
      layoutSelect,
      syncLabel,
      engineBadge,
    );
    if (this.screenAwake) {
      this.disposeAwakeControl?.();
      this.disposeAwakeControl = mountScreenAwakeButton(toolbar, this.screenAwake);
    }
    mountThemeControl({ container: toolbar });

    const grid = document.createElement('div');
    grid.className = 'qnext-lightweight-grid';

    shell.append(toolbar, grid);
    this.container.append(shell);

    this.shell = shell;
    this.grid = grid;
    this.symbolSearch = symbolSearch;
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
    await this.refreshPaperMarkers(true);
  }

  private async rebuildPane(index: number): Promise<void> {
    this.onActiveMarketChange?.();
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
    await this.refreshPaperMarkers(true);
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

  activeMarket(): { symbol: string; timeframe: string } | undefined {
    const config = this.paneConfigs[this.activePane];
    return config ? { symbol: config.ticker, timeframe: config.timeframe } : undefined;
  }

  private updateToolbarFromActivePane(): void {
    this.onActiveMarketChange?.();
    const config = this.paneConfigs[this.activePane];
    if (!config) return;
    if (this.symbolSearch) this.symbolSearch.textContent = config.ticker + ' ▾';
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

  private startMarkerPolling(): void {
    if (this.markerPolling) return;
    this.markerPolling = true;

    const poll = async () => {
      if (!this.markerPolling) return;
      try {
        await this.refreshPaperMarkers();
      } catch (error) {
        console.warn('QNext paper marker refresh failed', error);
      } finally {
        if (this.markerPolling) {
          this.markerTimer = setTimeout(poll, this.markerPollIntervalMs);
        }
      }
    };

    void poll();
  }

  private stopMarkerPolling(): void {
    this.markerPolling = false;
    if (this.markerTimer !== undefined) {
      clearTimeout(this.markerTimer);
      this.markerTimer = undefined;
    }
  }

  private async refreshPaperMarkers(force = false): Promise<void> {
    const snapshot = await this.markerClient.snapshot();
    const signature = [
      snapshot.available ? '1' : '0',
      snapshot.enabled ? '1' : '0',
      snapshot.instrumentId,
      snapshot.timeframe,
      snapshot.sessionId,
      snapshot.markers.map((marker) => marker.id).join(','),
    ].join('|');

    if (!force && signature === this.markerSignature) return;
    this.markerSignature = signature;

    this.charts.forEach((chart) => {
      chart.setPaperMarkers(
        paperSnapshotMatchesChart(snapshot, chart.ticker, chart.timeframe)
          ? snapshot.markers
          : [],
      );
    });
  }
}
