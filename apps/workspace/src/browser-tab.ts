import type { QNextBar } from './qnext-provider';

export type TabFeed = 'live' | 'fallback' | 'connecting' | 'disconnected' | 'idle';
export interface TabMarket { symbol?: string; timeframe?: string }
type Quote = { bar: Pick<QNextBar, 'time' | 'close' | 'revision'>; direction: number; live: boolean; feed: () => TabFeed };
const key = (symbol: string, timeframe: string) => symbol.trim().toUpperCase() + '\\0' + timeframe;
const formatter = new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 10 });
const colors = { up: '#16a34a', down: '#dc2626', neutral: '#64748b', warning: '#d97706' };

export class BrowserTabQuotes {
  private quotes = new Map<string, Quote>();
  observe(symbol: string, timeframe: string, bar: QNextBar, live: boolean, feed: () => TabFeed): void {
    if (!Number.isFinite(bar.close) || !Number.isFinite(bar.time)) return;
    const id = key(symbol, timeframe);
    const previous = this.quotes.get(id);
    if (previous && (bar.time < previous.bar.time ||
        (bar.time === previous.bar.time && ((bar.revision ?? 0) < (previous.bar.revision ?? 0) ||
         (!live && previous.live))))) return;
    const direction = !previous ? 0 : bar.close === previous.bar.close ? previous.direction :
      Math.sign(bar.close - previous.bar.close);
    this.quotes.delete(id);
    this.quotes.set(id, { bar: { time: bar.time, close: bar.close, revision: bar.revision }, direction, live, feed });
    if (this.quotes.size > 64) this.quotes.delete(this.quotes.keys().next().value!);
  }

  presentation(market: TabMarket | undefined, online = true): { title: string; color: string } {
    if (!market?.symbol) return { title: 'QNeXT', color: colors.neutral };
    const quote = market.timeframe ? this.quotes.get(key(market.symbol, market.timeframe)) : undefined;
    // Keep the exchange-qualified identity internally; strip only the default NSE prefix for display.
    const symbol = market.symbol.replace(/^NSE:/i, '');
    const feed = !online ? 'disconnected' : quote?.feed() ?? 'idle';
    const warning = feed === 'disconnected' || feed === 'connecting' || feed === 'fallback';
    const suffix = feed === 'disconnected' ? ' · Disconnected' : feed === 'connecting' ? ' · Connecting' :
      feed === 'fallback' ? ' · REST' : '';
    const direction = quote?.direction ?? 0;
    const arrow = direction > 0 ? ' ▲' : direction < 0 ? ' ▼' : '';
    return {
      title: symbol + (quote ? ' ' + formatter.format(quote.bar.close) + arrow : '') + suffix + ' - QNeXT',
      color: warning ? colors.warning : feed === 'idle' ? colors.neutral :
        direction > 0 ? colors.up : direction < 0 ? colors.down : colors.neutral,
    };
  }
}

/** One browser-local renderer. No market-data subscription, fetch, or chart redraw. */
export function installBrowserTab(quotes: BrowserTabQuotes, activeMarket: () => TabMarket | undefined) {
  const originalTitle = document.title;
  const icon = document.createElement('link');
  icon.rel = 'icon';
  icon.type = 'image/svg+xml';
  document.head.append(icon);
  let lastColor = '';
  let disposed = false;
  let selected = '';
  const refresh = (selectionOnly = false) => {
    if (disposed) return;
    let market: TabMarket | undefined;
    try { market = activeMarket(); } catch { return; }
    const next = key(market?.symbol ?? '', market?.timeframe ?? '');
    if (selectionOnly && next === selected) return;
    selected = next;
    const view = quotes.presentation(market, navigator.onLine);
    if (document.title !== view.title) document.title = view.title;
    if (lastColor !== view.color) {
      lastColor = view.color;
      const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="6" fill="' +
        view.color + '"/><path d="M8 23V11m8 12V7m8 15V10" stroke="white" stroke-width="1.5"/><path d="M8 15v5m8-9v7m8-4v5" stroke="white" stroke-width="4"/></svg>';
      icon.href = 'data:image/svg+xml,' + encodeURIComponent(svg);
    }
  };
  const selectionChanged = () => refresh(true);
  const tick = () => refresh();
  const timer = setInterval(tick, 1000);
  window.addEventListener('online', tick);
  window.addEventListener('offline', tick);
  document.addEventListener('visibilitychange', tick);
  refresh();
  return { selectionChanged, dispose: () => {
    if (disposed) return;
    disposed = true;
    clearInterval(timer);
    window.removeEventListener('online', tick);
    window.removeEventListener('offline', tick);
    document.removeEventListener('visibilitychange', tick);
    icon.remove();
    document.title = originalTitle;
  } };
}
