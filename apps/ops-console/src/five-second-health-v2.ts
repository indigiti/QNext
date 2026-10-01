import './five-second-health.css';

export {};

type HealthState = 'GOOD' | 'CARRY_FORWARD' | 'RECOVERED' | 'DEGRADED' | 'MISSING';

type HealthSegment = {
  start_ms: number;
  end_ms: number;
  state: HealthState;
  buckets: number;
  reason?: string;
};

type SymbolHealth = {
  instrument_id: string;
  symbol: string;
  calendar_id: string;
  session_start_ms?: number;
  session_end_ms?: number;
  elapsed_end_ms?: number;
  expected: number;
  present: number;
  good: number;
  carry_forward: number;
  recovered: number;
  degraded: number;
  missing: number;
  healthy_pct: number;
  completeness_pct: number;
  last_5s_ms?: number;
  status: string;
  timeline: HealthSegment[];
};

type HealthResponse = {
  generated_at_ms: number;
  bucket_ms: number;
  symbols: SymbolHealth[];
};

type CatalogSymbol = {
  instrument_id: string;
  ticker: string;
  description?: string;
  type?: string;
  synthetic?: boolean;
};

type CatalogResponse = { symbols?: CatalogSymbol[] };

type UsageRow = { views: number; last_view_ms: number };
type UsageMap = Record<string, UsageRow>;

const INDIA_TZ = 'Asia/Kolkata';
const HEALTH_REFRESH_MS = 15_000;
const DASHBOARD_MAX = 3;
const USAGE_KEY = 'qnext-admin-5s-health-views-v1';
const LAST_SYMBOL_KEY = 'qnext-admin-5s-health-last-symbol';
const seedPriority = ['NIFTY', 'NIFTY-SYN', 'NIFTY-SYN+'];

let catalog: CatalogSymbol[] = [];
let selectedSymbol = '';
let latestDetail: SymbolHealth | null = null;
let dashboardHealth: SymbolHealth[] = [];
let detailAbort: AbortController | null = null;
let dashboardAbort: AbortController | null = null;
let dashboardObserver: MutationObserver | null = null;

function qnextRoot(): URL {
  return new URL('/qnext/', window.location.origin);
}

function escapeHTML(value: unknown): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function timeLabel(ms?: number): string {
  if (!ms || ms <= 0) return '—';
  return new Intl.DateTimeFormat('en-GB', {
    timeZone: INDIA_TZ,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(ms));
}

function durationLabel(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0s';
  if (ms < 60_000) return `${Math.round(ms / 1000)}s`;
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

function pct(value: number): string {
  if (!Number.isFinite(value)) return '—';
  return `${value.toFixed(value >= 99.95 ? 2 : 1)}%`;
}

function stateLabel(state: HealthState): string {
  return state === 'CARRY_FORWARD' ? 'CARRY' : state;
}

function stateClass(state: HealthState): string {
  return state.toLowerCase().replace('_', '-');
}

function statusBadge(status: string): string {
  const ok = status === 'HEALTHY' || status === 'WAITING' || status === 'CLOSED';
  return `<span class="badge ${ok ? 'good' : 'bad'}">${escapeHTML(status)}</span>`;
}

function isHealthSymbol(item: CatalogSymbol): boolean {
  const ticker = String(item.ticker ?? '').toUpperCase();
  const type = String(item.type ?? '').toLowerCase();
  return item.synthetic === true || type.includes('index') || ticker.includes('SYN');
}

function readUsage(): UsageMap {
  try {
    const value = JSON.parse(localStorage.getItem(USAGE_KEY) ?? '{}') as UsageMap;
    return value && typeof value === 'object' ? value : {};
  } catch {
    return {};
  }
}

function recordView(symbol: string): void {
  const usage = readUsage();
  const previous = usage[symbol] ?? { views: 0, last_view_ms: 0 };
  usage[symbol] = { views: previous.views + 1, last_view_ms: Date.now() };
  try {
    localStorage.setItem(USAGE_KEY, JSON.stringify(usage));
    localStorage.setItem(LAST_SYMBOL_KEY, symbol);
  } catch {
    // Admin health still works when storage is unavailable.
  }
}

function topSymbols(): string[] {
  const usage = readUsage();
  const rank = new Map(seedPriority.map((symbol, index) => [symbol, seedPriority.length - index]));
  return [...catalog]
    .sort((a, b) => {
      const aUsage = usage[a.ticker] ?? { views: 0, last_view_ms: 0 };
      const bUsage = usage[b.ticker] ?? { views: 0, last_view_ms: 0 };
      if (aUsage.views !== bUsage.views) return bUsage.views - aUsage.views;
      if (aUsage.last_view_ms !== bUsage.last_view_ms) return bUsage.last_view_ms - aUsage.last_view_ms;
      const seeded = (rank.get(b.ticker) ?? 0) - (rank.get(a.ticker) ?? 0);
      if (seeded !== 0) return seeded;
      return a.ticker.localeCompare(b.ticker);
    })
    .slice(0, DASHBOARD_MAX)
    .map((item) => item.ticker);
}

async function fetchHealth(symbols: string[], signal?: AbortSignal): Promise<HealthResponse> {
  const url = new URL('api/v1/5s-health/', qnextRoot());
  if (symbols.length === 1) {
    url.searchParams.set('symbol', symbols[0]);
  } else if (symbols.length > 1) {
    url.searchParams.set('symbols', symbols.join(','));
  }
  const response = await fetch(url, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal,
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json() as Promise<HealthResponse>;
}

async function loadCatalog(): Promise<void> {
  const response = await fetch(new URL('api/v1/symbols/', qnextRoot()), {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  });
  if (!response.ok) throw new Error(`symbols HTTP ${response.status}`);
  const payload = await response.json() as CatalogResponse;
  catalog = (payload.symbols ?? []).filter(isHealthSymbol).sort((a, b) => a.ticker.localeCompare(b.ticker));
}

function ensurePanel(): HTMLElement | null {
  const card = document.querySelector<HTMLElement>('#feed-status-card');
  if (!card) return null;

  const existing = card.querySelector<HTMLElement>('#five-second-day-health');
  if (existing) return existing;

  const section = document.createElement('section');
  section.id = 'five-second-day-health';
  section.innerHTML = `
    <div class="line five-second-health-head">
      <div>
        <strong>Current-day 5s candle health</strong>
        <div class="muted">Choose one symbol to inspect. Only the selected timeline is read from persisted 5s history.</div>
      </div>
      <span id="five-second-day-health-state" class="badge">READY</span>
    </div>
    <div class="five-second-health-legend" aria-label="5 second health legend">
      <span><i class="five-second-swatch good"></i>GOOD</span>
      <span><i class="five-second-swatch carry-forward"></i>CARRY</span>
      <span><i class="five-second-swatch recovered"></i>RECOVERED</span>
      <span><i class="five-second-swatch degraded"></i>DEGRADED</span>
      <span><i class="five-second-swatch missing"></i>MISSING</span>
      <span><i class="five-second-swatch future"></i>FUTURE</span>
    </div>
    <div class="qnext-health-browser">
      <aside class="qnext-health-symbols">
        <div class="qnext-health-symbols-head">
          <strong>Symbols</strong>
          <small class="muted"><span id="qnext-health-symbol-count">0</span> available</small>
        </div>
        <input id="qnext-health-search" type="search" autocomplete="off" placeholder="Search symbol" aria-label="Search health symbols" />
        <div id="qnext-health-symbol-list" class="qnext-health-symbol-list"></div>
      </aside>
      <div id="qnext-health-detail" class="qnext-health-detail">
        <div class="qnext-health-detail-empty">Select a symbol to inspect its complete current-day 5s timeline.</div>
      </div>
    </div>
  `;

  const canonical = card.querySelector('#canonical-5s-section');
  const actions = card.querySelector('.feed-actions');
  if (canonical) canonical.insertAdjacentElement('afterend', section);
  else if (actions) card.insertBefore(section, actions);
  else card.append(section);

  section.querySelector<HTMLInputElement>('#qnext-health-search')?.addEventListener('input', renderSymbolList);
  return section;
}

function renderSymbolList(): void {
  const panel = ensurePanel();
  if (!panel) return;
  const list = panel.querySelector<HTMLDivElement>('#qnext-health-symbol-list');
  const search = panel.querySelector<HTMLInputElement>('#qnext-health-search');
  const count = panel.querySelector<HTMLElement>('#qnext-health-symbol-count');
  if (!list || !search) return;

  const query = search.value.trim().toLowerCase();
  const matches = catalog.filter((item) => {
    if (!query) return true;
    return `${item.ticker} ${item.description ?? ''} ${item.instrument_id}`.toLowerCase().includes(query);
  });
  if (count) count.textContent = String(catalog.length);

  list.innerHTML = matches.map((item) => `
    <button type="button" class="qnext-health-symbol${item.ticker === selectedSymbol ? ' active' : ''}" data-health-select="${escapeHTML(item.ticker)}">
      <span><strong>${escapeHTML(item.ticker)}</strong><small>${escapeHTML(item.description || item.instrument_id)}</small></span>
      <small>${item.synthetic ? 'SYN' : 'INDEX'}</small>
    </button>
  `).join('') || '<div class="muted" style="padding:.7rem">No matching symbols.</div>';
}

function timelineSegments(symbol: SymbolHealth): string {
  const start = symbol.session_start_ms ?? 0;
  const end = symbol.session_end_ms ?? 0;
  const total = end - start;
  if (total <= 0) return '<div class="five-second-timeline-empty">No regular session today</div>';

  return symbol.timeline.map((segment) => {
    const left = Math.max(0, Math.min(100, ((segment.start_ms - start) / total) * 100));
    const width = Math.max(0, Math.min(100 - left, ((segment.end_ms - segment.start_ms) / total) * 100));
    const abnormal = segment.state === 'MISSING' || segment.state === 'DEGRADED';
    const title = `${symbol.symbol} · ${stateLabel(segment.state)} · ${timeLabel(segment.start_ms)}–${timeLabel(segment.end_ms)} · ${durationLabel(segment.end_ms - segment.start_ms)} · ${segment.buckets} bucket${segment.buckets === 1 ? '' : 's'}${segment.reason ? ` · ${segment.reason}` : ''}`;
    return `<span class="five-second-timeline-segment ${stateClass(segment.state)}${abnormal ? ' abnormal' : ''}" style="left:${left.toFixed(5)}%;width:${width.toFixed(5)}%" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}"></span>`;
  }).join('');
}

function breakRows(symbol: SymbolHealth): string {
  const events = symbol.timeline.filter((segment) => segment.state !== 'GOOD');
  if (events.length === 0) {
    return '<div class="muted five-second-no-breaks">No carry, recovered, degraded, or missing 5s intervals in the elapsed session.</div>';
  }
  return events.map((segment) => {
    const severity = segment.state === 'MISSING' || segment.state === 'DEGRADED' ? 'break' : 'event';
    return `<div class="five-second-break-row ${severity}">
      <span class="five-second-event-state ${stateClass(segment.state)}">${escapeHTML(stateLabel(segment.state))}</span>
      <strong>${timeLabel(segment.start_ms)} – ${timeLabel(segment.end_ms)}</strong>
      <span>${durationLabel(segment.end_ms - segment.start_ms)}</span>
      <span>${segment.buckets} × 5s</span>
      <span class="muted">${escapeHTML(segment.reason || '—')}</span>
    </div>`;
  }).join('');
}

function detailMarkup(symbol: SymbolHealth): string {
  const start = symbol.session_start_ms ?? 0;
  const end = symbol.session_end_ms ?? 0;
  const elapsed = start > 0 && end > start && symbol.elapsed_end_ms
    ? Math.max(0, Math.min(100, ((symbol.elapsed_end_ms - start) / (end - start)) * 100))
    : 0;
  const breaks = symbol.timeline.filter((segment) => segment.state === 'MISSING' || segment.state === 'DEGRADED');
  const events = symbol.timeline.filter((segment) => segment.state !== 'GOOD');

  return `<article class="five-second-symbol-row" data-symbol="${escapeHTML(symbol.symbol)}">
    <div class="qnext-health-toolbar">
      <div>
        <div class="five-second-symbol-title"><strong>${escapeHTML(symbol.symbol)}</strong>${statusBadge(symbol.status)}</div>
        <p class="muted">${escapeHTML(symbol.instrument_id)} · ${timeLabel(symbol.session_start_ms)}–${timeLabel(symbol.session_end_ms)} IST</p>
      </div>
      <small class="muted">Auto-refresh 15s while this symbol is open</small>
    </div>
    <div class="qnext-health-kpis">
      <div><span>Health</span><strong>${pct(symbol.healthy_pct)}</strong></div>
      <div><span>Complete</span><strong>${pct(symbol.completeness_pct)}</strong></div>
      <div><span>Expected</span><strong>${symbol.expected}</strong></div>
      <div><span>Missing</span><strong>${symbol.missing}</strong></div>
      <div><span>Degraded</span><strong>${symbol.degraded}</strong></div>
      <div><span>Last 5s</span><strong>${timeLabel(symbol.last_5s_ms)}</strong></div>
    </div>
    <div class="five-second-timeline-wrap">
      <div class="five-second-timeline" aria-label="${escapeHTML(symbol.symbol)} current day 5 second candle timeline">
        <div class="five-second-timeline-elapsed" style="width:${elapsed.toFixed(5)}%"></div>
        ${timelineSegments(symbol)}
      </div>
      <div class="five-second-timeline-axis"><span>${timeLabel(symbol.session_start_ms)}</span><span>${timeLabel(symbol.session_end_ms)}</span></div>
    </div>
    <div class="five-second-state-counts">
      <span>GOOD <strong>${symbol.good}</strong></span>
      <span>CARRY <strong>${symbol.carry_forward}</strong></span>
      <span>RECOVERED <strong>${symbol.recovered}</strong></span>
      <span>DEGRADED <strong>${symbol.degraded}</strong></span>
      <span>MISSING <strong>${symbol.missing}</strong></span>
    </div>
    <details class="five-second-break-details" ${breaks.length > 0 ? 'open' : ''}>
      <summary>${breaks.length} break${breaks.length === 1 ? '' : 's'} · ${events.length} non-GOOD segment${events.length === 1 ? '' : 's'}</summary>
      <div class="five-second-break-list">${breakRows(symbol)}</div>
    </details>
  </article>`;
}

function renderDetail(symbol: SymbolHealth): void {
  latestDetail = symbol;
  const panel = ensurePanel();
  const detail = panel?.querySelector<HTMLDivElement>('#qnext-health-detail');
  const state = panel?.querySelector<HTMLSpanElement>('#five-second-day-health-state');
  if (detail) detail.innerHTML = detailMarkup(symbol);
  if (state) {
    const check = symbol.status === 'CHECK';
    state.className = `badge ${check ? 'bad' : 'good'}`;
    state.textContent = symbol.status;
  }
  renderSymbolList();
}

function renderDetailError(message: string): void {
  const panel = ensurePanel();
  const detail = panel?.querySelector<HTMLDivElement>('#qnext-health-detail');
  const state = panel?.querySelector<HTMLSpanElement>('#five-second-day-health-state');
  if (state) {
    state.className = 'badge bad';
    state.textContent = 'UNAVAILABLE';
  }
  if (detail) detail.innerHTML = `<div class="qnext-health-detail-empty"><div><strong>5s health unavailable</strong><div class="muted">${escapeHTML(message)}</div></div></div>`;
}

async function selectSymbol(symbol: string, countView = true): Promise<void> {
  if (!catalog.some((item) => item.ticker === symbol)) return;
  selectedSymbol = symbol;
  if (countView) recordView(symbol);
  renderSymbolList();

  const detail = ensurePanel()?.querySelector<HTMLDivElement>('#qnext-health-detail');
  if (detail) detail.innerHTML = '<div class="qnext-health-loading">Loading selected 5s timeline…</div>';

  detailAbort?.abort();
  detailAbort = new AbortController();
  try {
    const payload = await fetchHealth([symbol], detailAbort.signal);
    const row = payload.symbols[0];
    if (!row) throw new Error('No health row returned for the selected symbol');
    renderDetail(row);
    void refreshDashboardTop();
  } catch (error) {
    if ((error as Error).name === 'AbortError') return;
    renderDetailError((error as Error).message);
  }
}

function dashboardTimeline(symbol: SymbolHealth): string {
  return `<span class="ops-health-timeline five-second-timeline">${timelineSegments(symbol)}</span>`;
}

function renderDashboardTop(): void {
  const list = document.querySelector<HTMLElement>('.ops-health-list');
  if (!list) return;
  list.dataset.qnextHealthV2 = 'true';
  if (dashboardHealth.length === 0) {
    list.innerHTML = '<div class="ops-empty-state">Top 3 5s health rows will appear here after symbol metadata loads.</div>';
    return;
  }
  list.innerHTML = dashboardHealth.slice(0, DASHBOARD_MAX).map((symbol) => {
    const bad = symbol.status === 'CHECK' || symbol.missing > 0 || symbol.degraded > 0;
    return `<button type="button" class="ops-health-row" data-dashboard-view="market" data-health-symbol="${escapeHTML(symbol.symbol)}" aria-label="Open ${escapeHTML(symbol.symbol)} 5 second health">
      <span class="ops-health-symbol"><strong>${escapeHTML(symbol.symbol)}</strong><small class="${bad ? 'is-bad' : ''}">${escapeHTML(symbol.status)}</small></span>
      ${dashboardTimeline(symbol)}
      <span class="ops-health-pct"><strong>${pct(symbol.healthy_pct)}</strong><small>${symbol.missing} missing</small></span>
    </button>`;
  }).join('');
}

async function refreshDashboardTop(): Promise<void> {
  if (catalog.length === 0) return;
  const symbols = topSymbols();
  if (symbols.length === 0) return;
  dashboardAbort?.abort();
  dashboardAbort = new AbortController();
  try {
    const payload = await fetchHealth(symbols, dashboardAbort.signal);
    dashboardHealth = payload.symbols;
    renderDashboardTop();
  } catch (error) {
    if ((error as Error).name !== 'AbortError') console.warn('QNext dashboard 5s health unavailable', error);
  }
}

function installDashboardObserver(): void {
  if (dashboardObserver) return;
  dashboardObserver = new MutationObserver(() => {
    const list = document.querySelector<HTMLElement>('.ops-health-list');
    if (list && list.dataset.qnextHealthV2 !== 'true') renderDashboardTop();
  });
  dashboardObserver.observe(document.body, { childList: true, subtree: true });
}

function preferredInitialSymbol(): string {
  const stored = localStorage.getItem(LAST_SYMBOL_KEY) ?? '';
  if (catalog.some((item) => item.ticker === stored)) return stored;
  for (const seed of seedPriority) {
    if (catalog.some((item) => item.ticker === seed)) return seed;
  }
  return catalog[0]?.ticker ?? '';
}

async function bootstrapHealth(): Promise<void> {
  ensurePanel();
  installDashboardObserver();
  try {
    await loadCatalog();
    renderSymbolList();
    const initial = preferredInitialSymbol();
    if (initial) await selectSymbol(initial, false);
    await refreshDashboardTop();
  } catch (error) {
    renderDetailError((error as Error).message);
  }
}

document.addEventListener('click', (event) => {
  const target = event.target as Element | null;
  const selector = target?.closest<HTMLElement>('[data-health-select]');
  if (selector?.dataset.healthSelect) {
    void selectSymbol(selector.dataset.healthSelect);
    return;
  }
  const dashboard = target?.closest<HTMLElement>('[data-health-symbol]');
  if (dashboard?.dataset.healthSymbol) {
    void selectSymbol(dashboard.dataset.healthSymbol);
  }
});

window.setInterval(() => {
  if (document.visibilityState !== 'visible') return;
  const view = document.body.dataset.opsView;
  if (view === 'market' && selectedSymbol) void selectSymbol(selectedSymbol, false);
  if (view === 'dashboard') void refreshDashboardTop();
}, HEALTH_REFRESH_MS);

void bootstrapHealth();
