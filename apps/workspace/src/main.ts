import { VelaWorkspace } from '@luxalgo/vela/workspace';
import { PineWorkerEngine } from '@luxalgo/vela-pinets';

import { resolveChartEngine, type QNextChartEngine } from './charts/chart-engine';
import { QNextLightweightChart } from './charts/lightweight-chart';
import { QNextProvider } from './qnext-provider';
import { QNextIndicatorEngine } from './qnext-indicator-engine';
import './style.css';

declare global {
  interface Window {
    __QNEXT_CONFIG__?: {
      apiBase?: string;
      streamUrl?: string;
      chartEngine?: QNextChartEngine;
      symbol?: string;
      timeframe?: string;
    };
    __QNEXT_WORKSPACE__?: VelaWorkspace;
    __QNEXT_LIGHTWEIGHT_CHART__?: QNextLightweightChart;
  }
}

const runtime = window.__QNEXT_CONFIG__ ?? {};
const defaultApiBase = window.location.pathname.replace(/\/+$/, '');

const fallbackTimeframes = [
  '15s', '30s', '1m', '2m', '3m', '5m', '15m', '30m', '1h', '1D',
];

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

async function bootstrapVela(timeframes: string[]) {
  const workspace = new VelaWorkspace('#app', {
    layout: false,
    symbol: runtime.symbol ?? 'NSE:NIFTY',
    timeframe:
      runtime.timeframe && timeframes.includes(runtime.timeframe)
        ? runtime.timeframe
        : timeframes.includes('1m')
          ? '1m'
          : timeframes[0],
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
}

async function bootstrapLightweight(timeframes: string[]) {
  const app = document.querySelector<HTMLElement>('#app');
  if (!app) throw new Error('QNext workspace root #app not found');

  app.replaceChildren();
  app.dataset.chartEngine = 'lightweight';

  const shell = document.createElement('div');
  shell.className = 'qnext-lightweight-shell';

  const badge = document.createElement('div');
  badge.className = 'qnext-lightweight-badge';
  badge.textContent = 'QNext Lightweight';

  const chartHost = document.createElement('div');
  chartHost.className = 'qnext-lightweight-chart';

  shell.append(badge, chartHost);
  app.append(shell);

  const timeframe =
    runtime.timeframe && timeframes.includes(runtime.timeframe)
      ? runtime.timeframe
      : timeframes.includes('1m')
        ? '1m'
        : timeframes[0];

  const chart = new QNextLightweightChart({
    container: chartHost,
    provider: createProvider(),
    ticker: runtime.symbol ?? 'NSE:NIFTY',
    timeframe,
  });

  await chart.mount();
  window.__QNEXT_LIGHTWEIGHT_CHART__ = chart;
}

async function bootstrap() {
  const timeframes = await loadEnabledTimeframes();
  const engine = resolveChartEngine(runtime.chartEngine);

  if (engine === 'lightweight') {
    await bootstrapLightweight(timeframes);
    return;
  }

  await bootstrapVela(timeframes);
}

void bootstrap();
