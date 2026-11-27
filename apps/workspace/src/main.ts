import { catalogTimeframe, resolveCatalogInstrument } from './symbol-catalog';
import { createSymbolSearch } from './symbol-search-launcher';

import { normalizeLightweightLayout, resolveChartEngine, type QNextChartEngine } from './charts/chart-engine';
import type { QNextLightweightWorkspace } from './charts/lightweight-workspace';
import type { VelaWorkspace as VelaWorkspaceInstance } from '@luxalgo/vela/workspace';
import type { VelaSignalTrace as VelaSignalTraceInstance } from './charts/vela-signal-trace';
import { QNextProvider } from './qnext-provider';
import { BrowserTabQuotes, installBrowserTab } from './browser-tab';
import { qnextBrowserLatencySnapshot } from './live-latency';
import './style.css';
import { applyTheme, currentTheme } from './theme';
import { createScreenAwakeController } from './screen-awake';
import './indicator-templates.css';
import './theme.css';

declare global {
  interface Window {
    __QNEXT_CONFIG__?: {
      apiBase?: string;
      streamUrl?: string;
      chartEngine?: QNextChartEngine;
      symbol?: string;
      timeframe?: string;
      lightweightLayout?: 1 | 2 | 4;
      lightweightSync?: boolean;
      /** Diagnostic only; worker is the production default. */
      pineEngine?: 'worker' | 'main';
      /** Enables local-only Pine signal tracing; off by default. */
      pineTrace?: boolean;
    };
    __QNEXT_WORKSPACE__?: VelaWorkspaceInstance;
    __QNEXT_LIGHTWEIGHT_WORKSPACE__?: QNextLightweightWorkspace;
    __QNEXT_PINE_TRACE__?: VelaSignalTraceInstance;
  }
}

interface WorkspaceSettings {
  chartEngine: QNextChartEngine;
  lightweightLayout: 1 | 2 | 4;
  lightweightSync: boolean;
}

const screenAwake = createScreenAwakeController();
screenAwake.start();

const runtime = window.__QNEXT_CONFIG__ ?? {};
const defaultApiBase = window.location.pathname.replace(/\/+$/, '');

const fallbackTimeframes = [
  '5s', '15s', '30s', '1m', '2m', '3m', '5m', '15m', '30m', '1h', '1D',
];

const fallbackWorkspaceSettings: WorkspaceSettings = {
  chartEngine: 'vela',
  lightweightLayout: 1,
  lightweightSync: true,
};

async function loadEnabledTimeframes(): Promise<string[]> {
  const apiBase = (runtime.apiBase ?? defaultApiBase).replace(/\/+$/, '');
  try {
    const response = await fetch(`${apiBase}/api/v1/timeframes/`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    const payload = (await response.json()) as { timeframes?: unknown };
    if (!Array.isArray(payload.timeframes)) {
      throw new Error('invalid timeframe payload');
    }
    const enabled = payload.timeframes.filter(
      (value): value is string => typeof value === 'string' && value.trim() !== '',
    );
    if (enabled.length === 0 || !enabled.includes('1m')) {
      throw new Error('enabled timeframe list is empty or missing canonical 1m');
    }
    return enabled;
  } catch (error) {
    console.warn('QNext enabled timeframes unavailable; using defaults', error);
    return [...fallbackTimeframes];
  }
}

async function loadWorkspaceSettings(): Promise<WorkspaceSettings> {
  const apiBase = (runtime.apiBase ?? defaultApiBase).replace(/\/+$/, '');
  try {
    const response = await fetch(`${apiBase}/api/v1/workspace-settings/`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const payload = (await response.json()) as Partial<WorkspaceSettings>;
    const chartEngine =
      payload.chartEngine === 'lightweight' ||
      payload.chartEngine === 'auto' ||
      payload.chartEngine === 'vela'
        ? payload.chartEngine
        : fallbackWorkspaceSettings.chartEngine;
    const lightweightLayout = normalizeLightweightLayout(payload.lightweightLayout);
    const lightweightSync =
      typeof payload.lightweightSync === 'boolean'
        ? payload.lightweightSync
        : fallbackWorkspaceSettings.lightweightSync;
    return { chartEngine, lightweightLayout, lightweightSync };
  } catch (error) {
    console.warn('QNext workspace settings unavailable; using defaults', error);
    return { ...fallbackWorkspaceSettings };
  }
}

async function loadCustomIndicators() {
  const apiBase = (runtime.apiBase ?? defaultApiBase).replace(/\/+$/, '');
  try {
    const response = await fetch(`${apiBase}/api/v1/indicators/`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return await response.json() as {
      indicators: Array<{
        name: string;
        script: string;
        language: string;
        enabled: boolean;
        category: string;
      }>;
    };
  } catch (error) {
    console.warn('QNext custom indicators unavailable', error);
    return { indicators: [] };
  }
}

function createProvider(trace?: VelaSignalTraceInstance, quotes?: BrowserTabQuotes): QNextProvider {
  const provider: QNextProvider = new QNextProvider({
    apiBase: runtime.apiBase ?? defaultApiBase,
    streamUrl: runtime.streamUrl,
    onDeliveredBar: (ticker, timeframe, bar) => {
      trace?.providerBar(ticker, timeframe, bar);
      quotes?.observe(ticker, timeframe, bar, true, () => provider.tabFeed(ticker, timeframe));
    },
    onHistoryBar: (ticker, timeframe, bar) => {
      quotes?.observe(ticker, timeframe, bar, false, () => provider.tabFeed(ticker, timeframe));
    },
  });
  return provider;
}

function resolveInitialTimeframe(timeframes: string[]): string {
  return runtime.timeframe && timeframes.includes(runtime.timeframe)
    ? runtime.timeframe
    : timeframes.includes('1m')
      ? '1m'
      : timeframes[0];
}

async function bootstrapVela(timeframes: string[]) {
  // Vela, its scripting engine and integrations belong to this runtime only.
  const {
    registerIcon, registerWidgetAction, VelaWorkspace,
    createQNextPineEngine, resolveQNextPineMode, withConfirmedMtfRecompute,
    VELA_INITIAL_HISTORY, installVelaPanHistory, VelaSignalTrace, installVelaSignalTrace,
    QNextIndicatorEngine, mountIntelligenceLabShadowBridge, registerVelaAppearanceAction,
    registerVelaScreenAwakeAction, installVelaWheelZoomControl, registerVelaZoomSettings,
    registerIndicatorTemplates, registerVelaChartAlertAction, syncVelaChartAlertAction,
    velaChartAlerts, withChartAlertSwitch,
  } = await import('./vela-runtime');
  const pineTrace = runtime.pineTrace === true || new URLSearchParams(window.location.search).get('pine_trace') === '1'
    ? new VelaSignalTrace()
    : undefined;
  if (pineTrace) window.__QNEXT_PINE_TRACE__ = pineTrace;
  registerVelaAppearanceAction();
  registerVelaZoomSettings();
  registerVelaScreenAwakeAction(screenAwake);
  registerVelaChartAlertAction(velaChartAlerts);
  const disposeWheelZoom = installVelaWheelZoomControl(document.getElementById('app')!);
  window.addEventListener('pagehide', disposeWheelZoom, { once: true });
  const quotes = new BrowserTabQuotes();
  let workspace: VelaWorkspaceInstance;
  registerIndicatorTemplates(() => workspace);
  const catalogProvider = createProvider();
  const search = createSymbolSearch(() => catalogProvider.listCatalog(), async entry => {
    const chart = workspace.chart;
    await chart.setMarket({symbol:entry.instrument_id,timeframe:catalogTimeframe(entry,chart.market.timeframe ?? '1m')});
  });
  registerIcon('qnext-symbol-search','<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/></svg>');
  registerWidgetAction({id:'qnext.symbol.search',target:'topbar',label:'Symbol Search',icon:'qnext-symbol-search',iconOnly:true,align:'left',order:0,run:search.open});
  window.addEventListener('pagehide',search.dispose,{once:true});
  workspace = new VelaWorkspace('#app', {
    layout: false,
    symbol: runtime.symbol ?? 'NSE:NIFTY',
    timeframe: resolveInitialTimeframe(timeframes),
    bars: VELA_INITIAL_HISTORY,
    timeframes,
    live: true,
    theme: currentTheme(),
    timezone: 'exchange',
    providers: {
      qnext: () => createProvider(pineTrace, quotes),
    },
    engines: {
      qnext: () => withChartAlertSwitch(new QNextIndicatorEngine(), velaChartAlerts),
      pine: () => withChartAlertSwitch(
        withConfirmedMtfRecompute(createQNextPineEngine(resolveQNextPineMode(runtime.pineEngine), pineTrace)),
        velaChartAlerts,
      ),
    },
    indicators: loadCustomIndicators,
    persist: 'qnext-workspace-v2',
    topbar: {
      left: ['qnext.symbol.search', 'symbol', 'timeframes', 'style', 'indicators', 'qnext.indicator.templates', 'undo-redo'],
      // Zoom action sits immediately before the Data Window / Object Tree buttons.
      right: ['actions', 'alerts', 'qnext.alerts.toggle', 'qnext.screen-awake.toggle', 'qnext.zoom.open', 'panels', 'screenshot', 'fullscreen'],
    },
  });

  window.__QNEXT_WORKSPACE__ = workspace;
  const tab = installBrowserTab(quotes, () => workspace.chart.market);
  // Selection changes are immediate; regular updates are limited to 1 Hz.
  const tabCharts = new Map<typeof workspace.chart, () => void>();
  const syncTabCharts = () => {
    const charts = new Set(workspace.cells().map(cell => cell.chart));
    for (const [chart, off] of tabCharts) if (!charts.has(chart)) { off(); tabCharts.delete(chart); }
    for (const chart of charts) if (!tabCharts.has(chart)) tabCharts.set(chart, chart.on('market:changed', tab.selectionChanged));
    tab.selectionChanged();
  };
  syncTabCharts();
  const offTabCreated = workspace.on('cell:created', syncTabCharts);
  const offTabDestroyed = workspace.on('cell:destroyed', syncTabCharts);
  window.addEventListener('pagehide', event => {
    if (event.persisted) return;
    offTabCreated(); offTabDestroyed();
    for (const off of tabCharts.values()) off();
    tabCharts.clear(); tab.dispose();
  });
  syncVelaChartAlertAction();
  installVelaPanHistory(workspace);
  if (pineTrace) installVelaSignalTrace(workspace, pineTrace);
  const labResolverProvider = catalogProvider;
  // Keep Shadow-Live collection for Admin/Intelligence Lab, but do not mount
  // the floating public chart status or the manual Send to Lab widget.
  mountIntelligenceLabShadowBridge(
    workspace as unknown as Parameters<typeof mountIntelligenceLabShadowBridge>[0],
    (symbol) => labResolverProvider.resolveInstrumentID(symbol),
    (instrumentID, timeframe, range) =>
      labResolverProvider.getCanonicalBars(instrumentID, timeframe, range),
    (instrumentID, timeframe, range) =>
      labResolverProvider.getBars(instrumentID, timeframe, range),
  );
}

async function bootstrapLightweight(
  timeframes: string[],
  settings: WorkspaceSettings,
) {
  const app = document.querySelector<HTMLElement>('#app');
  if (!app) throw new Error('QNext workspace root #app not found');

  const quotes = new BrowserTabQuotes();
  const provider = createProvider(undefined, quotes);
  let initialTicker = runtime.symbol ?? 'NSE:NIFTY';
  const initialTimeframe = resolveInitialTimeframe(timeframes);

  const loadCatalog = async (): Promise<Awaited<ReturnType<QNextProvider['listCatalog']>>> => {
    let symbols: Awaited<ReturnType<QNextProvider['listCatalog']>> = [];
    try {
      symbols = await provider.listCatalog();
      initialTicker = resolveCatalogInstrument(symbols,initialTicker).instrument_id;
    } catch (error) {
      console.warn('QNext symbol catalog unavailable; using configured symbol', error);
      // Keep the configured chart request, never fabricate a directory entry.
    }
    return symbols;
  };
  const [{ QNextLightweightWorkspace }, symbols] = await Promise.all([
    import('./charts/lightweight-workspace'),
    loadCatalog(),
  ]);

  const query = new URLSearchParams(window.location.search);
  const layout = normalizeLightweightLayout(
    query.get('layout') ?? runtime.lightweightLayout ?? settings.lightweightLayout,
  );
  const querySync = query.get('sync');
  const syncCharts = querySync === null
    ? runtime.lightweightSync ?? settings.lightweightSync
    : querySync !== '0' && querySync !== 'false';

  let workspace: QNextLightweightWorkspace;
  const tab = installBrowserTab(quotes, () => workspace?.activeMarket());
  workspace = new QNextLightweightWorkspace({
    container: app,
    provider,
    screenAwake,
    apiBase: runtime.apiBase ?? defaultApiBase,
    symbols,
    timeframes,
    initialTicker,
    initialTimeframe,
    initialLayout: layout,
    syncCharts,
    onActiveMarketChange: tab.selectionChanged,
  });
  window.addEventListener('pagehide', event => { if (!event.persisted) tab.dispose(); });

  await workspace.mount();
  window.__QNEXT_LIGHTWEIGHT_WORKSPACE__ = workspace;
}

async function bootstrap() {
  // Browser-local DevTools diagnostic. Never mixes server/client clocks.
  (window as Window & { __QNEXT_LATENCY__?: typeof qnextBrowserLatencySnapshot }).__QNEXT_LATENCY__ =
    qnextBrowserLatencySnapshot;
  const [timeframes, settings] = await Promise.all([
    loadEnabledTimeframes(),
    loadWorkspaceSettings(),
  ]);
  const engine = resolveChartEngine(runtime.chartEngine ?? settings.chartEngine);

  if (engine === 'lightweight') {
    await bootstrapLightweight(timeframes, settings);
    return;
  }

  await bootstrapVela(timeframes);
}

applyTheme();
void bootstrap();
