import { VelaWorkspace } from '@luxalgo/vela/workspace';

import { QNextProvider } from './qnext-provider';
import { QNextIndicatorEngine } from './qnext-indicator-engine';
import './style.css';

declare global {
  interface Window {
    __QNEXT_CONFIG__?: {
      apiBase?: string;
      streamUrl?: string;
    };
    __QNEXT_WORKSPACE__?: VelaWorkspace;
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

async function bootstrap() {
  const timeframes = await loadEnabledTimeframes();
  const workspace = new VelaWorkspace('#app', {
    layout: false,
    symbol: 'NSE:NIFTY',
    timeframe: timeframes.includes('1m') ? '1m' : timeframes[0],
    timeframes,
    live: true,
    theme: 'dark',
    timezone: 'exchange',
    providers: {
      qnext: () =>
        new QNextProvider({
          apiBase: runtime.apiBase ?? defaultApiBase,
          streamUrl: runtime.streamUrl,
        }),
    },
    engines: {
      qnext: () => new QNextIndicatorEngine(),
    },
    indicators: loadCustomIndicators,
    persist: 'qnext-workspace-v2',
    topbar: {
      left: ['symbol', 'timeframes', 'style', 'indicators', 'undo-redo'],
    },
  });

  window.__QNEXT_WORKSPACE__ = workspace;
}

void bootstrap();
