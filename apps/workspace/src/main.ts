import { VelaWorkspace } from '@luxalgo/vela/workspace';

import { QNextProvider } from './qnext-provider';
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

const workspace = new VelaWorkspace('#app', {
  layout: false,
  symbol: 'NSE:NIFTY',
  timeframe: '1m',
  timeframes: [
    '1s', '5s', '10s', '15s', '30s', '45s',
    '1m', '2m', '3m', '5m', '10m', '15m', '30m', '45m',
    '1h', '2h', '3h', '4h',
    '1D', '1W',
    '1M', '3M', '6M', '12M',
  ],
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
  persist: 'qnext-workspace-v1',
  topbar: {
    left: ['symbol', 'timeframes', 'style', 'indicators', 'undo-redo'],
  },
});

window.__QNEXT_WORKSPACE__ = workspace;
