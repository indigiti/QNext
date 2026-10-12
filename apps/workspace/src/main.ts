import { VelaWorkspace } from '@luxalgo/vela/workspace';
import { PineWorkerEngine } from '@luxalgo/vela-pinets';

import { resolveChartEngine, type QNextChartEngine } from './charts/chart-engine';
import {
  QNextLightweightWorkspace,
  normalizeLightweightLayout,
} from './charts/lightweight-workspace';
import { QNextProvider } from './qnext-provider';
import { QNextIndicatorEngine } from './qnext-indicator-engine';
import { mountIntelligenceLabBridge } from './intelligence-lab-bridge';
import { mountIntelligenceLabShadowBridge } from './intelligence-lab-shadow-bridge';
import { mountCertifiedAdvisory } from './intelligence-certified-advisory';
import './style.css';

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
    };
    __QNEXT_WORKSPACE__?: VelaWorkspace;
    __QNEXT_LIGHTWEIGHT_WORKSPACE__?: QNextLightweightWorkspace;
  }
}

interface WorkspaceSettings {
  chartEngine: QNextChartEngine;
  lightweightLayout: 1 | 2 | 4;
  lightweightSync: boolean;
}

const runtime = window.__QNEXT_CONFIG__ ?? {};
const defaultApiBase = window.location.pathname.replace(/\/+$/, '');

const fallbackTimeframes = [
  '15s', '30s', '1m', '2m', '3m', '5m', '15m', '30m', '1h', '1D',
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

function createProvider(): QNextProvider {
  return new QNextProvider({
    apiBase: runtime.apiBase ?? defaultApiBase,
    streamUrl: runtime.streamUrl,
  });
}

function resolveInitialTimeframe(timeframes: string[]): string {
  return runtime.timeframe && timeframes.includes(runtime.timeframe)
    ? runtime.timeframe
    : timeframes.includes('1m')
      ? '1m'
      : timeframes[0];
}

async function bootstrapVela(timeframes: string[]) {
  const workspace = new VelaWorkspace('#app', {
    layout: false,
    symbol: runtime.symbol ?? 'NSE:NIFTY',
    timeframe: resolveInitialTimeframe(timeframes),
    timeframes,
    live: true,
    theme: 'dark',
    timezone: 'exchange',
    providers: {
      qnext: () => createProvider(),
    },
    engines: {
      qnext: () => new QNextIndicatorEngine(),
      pine: () => new PineWorkerEngine(),
    },
    indicators: loadCustomIndicators,
    persist: 'qnext-workspace-v2',
    topbar: {
      left: ['symbol', 'timeframes', 'style', 'indicators', 'undo-redo'],
    },
  });

  window.__QNEXT_WORKSPACE__ = workspace;
  const labResolverProvider = createProvider();
  mountIntelligenceLabBridge(
    workspace as unknown as Parameters<typeof mountIntelligenceLabBridge>[0],
    (symbol) => labResolverProvider.resolveInstrumentID(symbol),
    (instrumentID, timeframe, range) =>
      labResolverProvider.getBars(instrumentID, timeframe, range),
  );
  mountIntelligenceLabShadowBridge(
    workspace as unknown as Parameters<typeof mountIntelligenceLabShadowBridge>[0],
    (symbol) => labResolverProvider.resolveInstrumentID(symbol),
    (instrumentID, timeframe, range) =>
      labResolverProvider.getCanonicalBars(instrumentID, timeframe, range),
    (instrumentID, timeframe, range) =>
      labResolverProvider.getBars(instrumentID, timeframe, range),
  );
  mountCertifiedAdvisory(
    workspace as unknown as Parameters<typeof mountCertifiedAdvisory>[0],
    (symbol) => labResolverProvider.resolveInstrumentID(symbol),
    runtime.apiBase ?? defaultApiBase,
  );
}

async function bootstrapLightweight(
  timeframes: string[],
  settings: WorkspaceSettings,
) {
  const app = document.querySelector<HTMLElement>('#app');
  if (!app) throw new Error('QNext workspace root #app not found');

  const provider = createProvider();
  const initialTicker = runtime.symbol ?? 'NSE:NIFTY';
  const initialTimeframe = resolveInitialTimeframe(timeframes);

  let symbols: Awaited<ReturnType<QNextProvider['listSymbols']>>;
  try {
    symbols = await provider.listSymbols();
  } catch (error) {
    console.warn('QNext symbol catalog unavailable; using configured symbol', error);
    symbols = [{
      ticker: initialTicker,
      description: initialTicker,
      type: 'index',
      prefix: 'NSE',
    }];
  }

  const query = new URLSearchParams(window.location.search);
  const layout = normalizeLightweightLayout(
    query.get('layout') ?? runtime.lightweightLayout ?? settings.lightweightLayout,
  );
  const querySync = query.get('sync');
  const syncCharts = querySync === null
    ? runtime.lightweightSync ?? settings.lightweightSync
    : querySync !== '0' && querySync !== 'false';

  const workspace = new QNextLightweightWorkspace({
    container: app,
    provider,
    apiBase: runtime.apiBase ?? defaultApiBase,
    symbols,
    timeframes,
    initialTicker,
    initialTimeframe,
    initialLayout: layout,
    syncCharts,
  });

  await workspace.mount();
  window.__QNEXT_LIGHTWEIGHT_WORKSPACE__ = workspace;
}

async function bootstrap() {
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

void bootstrap();
