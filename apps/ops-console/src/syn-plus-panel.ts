import { OpsAPI, type FeedStatusBody, type FeedStatusResponse } from './api';

type SynPlusRuntime = {
  mode?: string;
  authority?: boolean;
  chart_visible?: boolean;
  instrument_id?: string;
  version?: string;
  snapshot_at_ms?: number;
  source_current_ts_ms?: number;
  received_at_ms?: number;
  expiry?: string;
  atm?: number;
  spot?: number;
  value?: number;
  basis_to_spot?: number;
  quality?: string;
  valid_candidates?: number;
  rejected_candidates?: number;
  microprice_legs?: number;
  mid_legs?: number;
  ltp_legs?: number;
  median_absolute_deviation?: number;
  chart_timeframes?: string[];
};

type SynPlusFeedBody = FeedStatusBody & {
  telemetry: FeedStatusBody['telemetry'] & {
    synthetics?: Record<string, SynPlusRuntime>;
  };
};

type PaperMetrics = {
  realized_pnl?: number;
  unrealized_pnl?: number;
  total_pnl?: number;
  avg_entry_price?: number;
  trade_count?: number;
  win_count?: number;
  win_rate?: number;
  max_drawdown?: number;
  max_drawdown_pct?: number;
};

type PaperStatus = {
  schema?: string;
  runtime_version?: string;
  enabled?: boolean;
  broker_execution_enabled?: boolean;
  instrument_id?: string;
  timeframe?: string;
  strategy?: { name?: string; fast?: number; slow?: number };
  initial_capital?: number;
  unit_size?: number;
  session_key?: string;
  paper_session_id?: string;
  started_at_ms?: number;
  last_poll_ms?: number;
  last_error?: string;
  cash?: number;
  position_qty?: number;
  mark_price?: number;
  equity?: number;
  pending_order?: unknown;
  last_bar?: { close_time_ms?: number; quality?: string } | null;
  metrics?: PaperMetrics;
  fills?: unknown[];
  markers?: unknown[];
};

type PaperEnvelope = {
  ok?: boolean;
  status?: number | null;
  error?: string;
  body?: PaperStatus;
};

function opsAPI() {
  return new OpsAPI({
    base: window.__QNEXT_OPS_CONFIG__?.apiBase,
    token: sessionStorage.getItem('qnext-ops-token') ?? '',
  });
}

function escapeHTML(value: unknown) {
  return String(value ?? '—')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function price(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

function percent(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
  return `${(value * 100).toFixed(2)}%`;
}

function signed(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
  return `${value >= 0 ? '+' : ''}${price(value)}`;
}

function ageLabel(atMS?: number) {
  if (!atMS || atMS <= 0) return 'never';
  const age = Math.max(0, Date.now() - atMS);
  if (age < 1000) return '<1s ago';
  if (age < 60_000) return `${Math.floor(age / 1000)}s ago`;
  if (age < 3_600_000) return `${Math.floor(age / 60_000)}m ago`;
  return `${Math.floor(age / 3_600_000)}h ago`;
}

function badge(ok: boolean, label: string) {
  return `<span class="badge ${ok ? 'good' : 'bad'}">${escapeHTML(label)}</span>`;
}

function ensurePanel() {
  let panel = document.querySelector<HTMLElement>('#syn-plus-status-card');
  if (panel) return panel;

  const feedCard = document.querySelector<HTMLElement>('#feed-status-card');
  if (!feedCard) return null;

  panel = document.createElement('section');
  panel.id = 'syn-plus-status-card';
  panel.className = 'card span-3';
  panel.innerHTML = `
    <div class="card-head">
      <div>
        <p class="eyebrow">Research synthetic</p>
        <h2>NIFTY-SYN+</h2>
        <p class="muted">Shadow synthetic with isolated Strategy Paper. Paper execution cannot route broker orders or promote SYN+ to trading authority.</p>
      </div>
      <button id="refresh-syn-plus" class="secondary">Refresh SYN+</button>
    </div>
    <div id="syn-plus-summary" class="status-grid"></div>
    <div id="syn-plus-quality" class="feed-provider-grid"></div>

    <div class="line" style="margin-top: 1.2rem">
      <div>
        <p class="eyebrow">Forward evaluation</p>
        <h2>Strategy Paper</h2>
      </div>
      <div id="syn-plus-paper-mode"></div>
    </div>
    <div id="syn-plus-paper-status" class="status-grid"></div>
    <div id="syn-plus-paper-config" class="feed-provider-grid" style="margin-top: .8rem">
      <div class="feed-provider-card">
        <div class="line"><strong>PAPER CONFIGURATION</strong><span class="muted">changes require OFF</span></div>
        <div class="feed-kv"><span>Timeframe</span><select id="syn-paper-timeframe"><option>15s</option><option>30s</option><option selected>1m</option><option>2m</option><option>3m</option><option>5m</option><option>15m</option><option>30m</option><option>1h</option><option>1D</option></select></div>
        <div class="feed-kv"><span>Initial capital</span><input id="syn-paper-capital" type="number" min="1" step="1000" value="100000" /></div>
        <div class="feed-kv"><span>Unit size</span><input id="syn-paper-unit" type="number" min="0.0001" step="1" value="1" /></div>
      </div>
      <div class="feed-provider-card">
        <div class="line"><strong>STARTER STRATEGY</strong>${badge(true, 'SMA CROSS')}</div>
        <div class="feed-kv"><span>Fast</span><input id="syn-paper-fast" type="number" min="1" step="1" value="2" /></div>
        <div class="feed-kv"><span>Slow</span><input id="syn-paper-slow" type="number" min="2" step="1" value="3" /></div>
        <div class="feed-kv"><span>Execution</span><strong>Q4 NEXT-BAR</strong></div>
      </div>
    </div>
    <div class="actions">
      <button id="syn-paper-enable">Enable Paper</button>
      <button id="syn-paper-disable" class="danger">Disable Paper</button>
      <button id="syn-paper-reset" class="secondary">Reset Session</button>
      <span class="muted">Reset clears the active forward session; prior event logs remain file-backed.</span>
    </div>
  `;
  feedCard.insertAdjacentElement('afterend', panel);
  panel.querySelector<HTMLButtonElement>('#refresh-syn-plus')?.addEventListener('click', () => {
    void loadAll();
  });
  panel.querySelector<HTMLButtonElement>('#syn-paper-enable')?.addEventListener('click', () => {
    void paperControl('enable');
  });
  panel.querySelector<HTMLButtonElement>('#syn-paper-disable')?.addEventListener('click', () => {
    void paperControl('disable');
  });
  panel.querySelector<HTMLButtonElement>('#syn-paper-reset')?.addEventListener('click', () => {
    void paperControl('reset');
  });
  return panel;
}

function findRuntime(body: SynPlusFeedBody) {
  const synthetics = body.telemetry?.synthetics ?? {};
  const exact = synthetics['QNEXT:NIFTY-SYN+'];
  if (exact) return ['QNEXT:NIFTY-SYN+', exact] as const;
  return Object.entries(synthetics).find(([, runtime]) =>
    runtime?.mode === 'SHADOW' && runtime?.chart_visible === true,
  );
}

function renderFeed(response: FeedStatusResponse) {
  const panel = ensurePanel();
  if (!panel) return;
  const summary = panel.querySelector<HTMLDivElement>('#syn-plus-summary')!;
  const quality = panel.querySelector<HTMLDivElement>('#syn-plus-quality')!;

  if (!response.ok || !response.body) {
    summary.innerHTML = `
      <div><span>Status</span>${badge(false, 'UNAVAILABLE')}</div>
      <div><span>Reason</span><strong>${escapeHTML(response.error ?? 'Feed status unavailable')}</strong></div>
    `;
    quality.innerHTML = '';
    return;
  }

  const body = response.body as SynPlusFeedBody;
  const found = findRuntime(body);
  if (!found) {
    summary.innerHTML = `
      <div><span>Mode</span>${badge(true, 'SHADOW')}</div>
      <div><span>Status</span>${badge(false, 'WAITING')}</div>
      <div><span>Authority</span><strong>NO</strong></div>
      <div><span>Chart</span><strong>VISIBLE / AWAITING DATA</strong></div>
    `;
    quality.innerHTML = '';
    return;
  }

  const [instrumentID, runtime] = found;
  const instrument = body.telemetry.instruments?.[instrumentID];
  const lastEvent = Math.max(
    runtime.received_at_ms ?? 0,
    runtime.snapshot_at_ms ?? 0,
    instrument?.last_event_time_ms ?? 0,
  );
  const live = lastEvent > 0 && Date.now() - lastEvent <= 30_000;
  const good = runtime.quality === 'GOOD';

  summary.innerHTML = `
    <div><span>Mode</span>${badge(true, escapeHTML(runtime.mode ?? 'SHADOW'))}</div>
    <div><span>Status</span>${badge(live && good, live ? (runtime.quality ?? 'LIVE') : 'STALE')}</div>
    <div><span>Authority</span><strong>${runtime.authority ? 'YES' : 'NO — SHADOW ONLY'}</strong></div>
    <div><span>Chart</span><strong>${runtime.chart_visible ? 'VISIBLE' : 'HIDDEN'}</strong></div>
    <div><span>Value</span><strong class="feed-price">${price(runtime.value ?? instrument?.price)}</strong></div>
    <div><span>Last update</span><strong>${ageLabel(lastEvent)}</strong></div>
    <div><span>Spot</span><strong>${price(runtime.spot)}</strong></div>
    <div><span>Basis to spot</span><strong>${price(runtime.basis_to_spot)}</strong></div>
    <div><span>ATM</span><strong>${price(runtime.atm)}</strong></div>
    <div><span>Expiry</span><strong>${escapeHTML(runtime.expiry)}</strong></div>
    <div><span>Version</span><strong>${escapeHTML(runtime.version)}</strong></div>
    <div><span>Instrument</span><strong>${escapeHTML(instrumentID)}</strong></div>
  `;

  quality.innerHTML = `
    <div class="feed-provider-card">
      <div class="line"><strong>CANDIDATE QUALITY</strong>${badge(good, runtime.quality ?? 'WAITING')}</div>
      <div class="feed-kv"><span>Valid / rejected</span><strong>${runtime.valid_candidates ?? 0} / ${runtime.rejected_candidates ?? 0}</strong></div>
      <div class="feed-kv"><span>Median abs deviation</span><strong>${price(runtime.median_absolute_deviation)}</strong></div>
      <div class="feed-kv"><span>Snapshot</span><strong>${ageLabel(runtime.snapshot_at_ms)}</strong></div>
    </div>
    <div class="feed-provider-card">
      <div class="line"><strong>FAIR PRICE SOURCES</strong>${badge(true, 'RESEARCH')}</div>
      <div class="feed-kv"><span>Microprice legs</span><strong>${runtime.microprice_legs ?? 0}</strong></div>
      <div class="feed-kv"><span>Mid legs</span><strong>${runtime.mid_legs ?? 0}</strong></div>
      <div class="feed-kv"><span>LTP legs</span><strong>${runtime.ltp_legs ?? 0}</strong></div>
    </div>
    <div class="feed-provider-card">
      <div class="line"><strong>CHART OUTPUT</strong>${badge(runtime.chart_visible === true, runtime.chart_visible ? 'VISIBLE' : 'HIDDEN')}</div>
      <div class="feed-kv"><span>Timeframes</span><strong>${escapeHTML(runtime.chart_timeframes?.join(', ') || '—')}</strong></div>
      <div class="feed-kv"><span>Provider</span><strong>${escapeHTML(instrument?.provider ?? 'qnext-syn-plus-shadow')}</strong></div>
      <div class="feed-kv"><span>Trading authority</span><strong>DISABLED</strong></div>
    </div>
  `;
}

function paperInputs() {
  const panel = ensurePanel();
  if (!panel) return null;
  return {
    timeframe: panel.querySelector<HTMLSelectElement>('#syn-paper-timeframe')!,
    capital: panel.querySelector<HTMLInputElement>('#syn-paper-capital')!,
    unit: panel.querySelector<HTMLInputElement>('#syn-paper-unit')!,
    fast: panel.querySelector<HTMLInputElement>('#syn-paper-fast')!,
    slow: panel.querySelector<HTMLInputElement>('#syn-paper-slow')!,
  };
}

function syncPaperInputs(status: PaperStatus) {
  const inputs = paperInputs();
  if (!inputs) return;
  inputs.timeframe.value = status.timeframe ?? '1m';
  inputs.capital.value = String(status.initial_capital ?? 100000);
  inputs.unit.value = String(status.unit_size ?? 1);
  inputs.fast.value = String(status.strategy?.fast ?? 2);
  inputs.slow.value = String(status.strategy?.slow ?? 3);
  for (const input of Object.values(inputs)) input.disabled = status.enabled === true;
}

function renderPaper(status?: PaperStatus, error?: string) {
  const panel = ensurePanel();
  if (!panel) return;
  const mode = panel.querySelector<HTMLDivElement>('#syn-plus-paper-mode')!;
  const output = panel.querySelector<HTMLDivElement>('#syn-plus-paper-status')!;
  const enable = panel.querySelector<HTMLButtonElement>('#syn-paper-enable')!;
  const disable = panel.querySelector<HTMLButtonElement>('#syn-paper-disable')!;
  const reset = panel.querySelector<HTMLButtonElement>('#syn-paper-reset')!;

  if (!status) {
    mode.innerHTML = badge(false, 'RUNTIME UNAVAILABLE');
    output.innerHTML = `<div><span>Error</span><strong>${escapeHTML(error ?? 'Paper runtime unavailable')}</strong></div>`;
    enable.disabled = true;
    disable.disabled = true;
    reset.disabled = true;
    return;
  }

  const enabled = status.enabled === true;
  const metrics = status.metrics ?? {};
  const lastBarAt = status.last_bar?.close_time_ms ?? 0;
  mode.innerHTML = badge(enabled, enabled ? 'PAPER ON' : 'PAPER OFF');
  output.innerHTML = `
    <div><span>Broker routing</span><strong>${status.broker_execution_enabled ? 'ENABLED' : 'DISABLED'}</strong></div>
    <div><span>Position</span><strong>${price(status.position_qty)}</strong></div>
    <div><span>Avg entry</span><strong>${price(metrics.avg_entry_price)}</strong></div>
    <div><span>Mark</span><strong>${price(status.mark_price)}</strong></div>
    <div><span>Equity</span><strong>${price(status.equity)}</strong></div>
    <div><span>Total P&L</span><strong>${signed(metrics.total_pnl)}</strong></div>
    <div><span>Realized P&L</span><strong>${signed(metrics.realized_pnl)}</strong></div>
    <div><span>Unrealized P&L</span><strong>${signed(metrics.unrealized_pnl)}</strong></div>
    <div><span>Max drawdown</span><strong>${price(metrics.max_drawdown)} / ${percent(metrics.max_drawdown_pct)}</strong></div>
    <div><span>Closed trades</span><strong>${metrics.trade_count ?? 0}</strong></div>
    <div><span>Win rate</span><strong>${percent(metrics.win_rate)}</strong></div>
    <div><span>Signal / fill markers</span><strong>${status.markers?.length ?? 0}</strong></div>
    <div><span>Last final bar</span><strong>${ageLabel(lastBarAt)}</strong></div>
    <div><span>Pending order</span><strong>${status.pending_order ? 'YES — NEXT BAR' : 'NO'}</strong></div>
    <div><span>Session</span><strong>${escapeHTML(status.session_key || '—')}</strong></div>
    <div><span>Runtime error</span><strong>${escapeHTML(status.last_error || 'none')}</strong></div>
  `;
  syncPaperInputs(status);
  enable.disabled = enabled;
  disable.disabled = !enabled;
  reset.disabled = !status.session_key;
}

async function requestPaper(payload?: Record<string, unknown>): Promise<PaperStatus> {
  const base = (window.__QNEXT_OPS_CONFIG__?.apiBase ?? '/qnext/admin/api/index.php').replace(/\/$/, '');
  const separator = base.includes('?') ? '&' : '?';
  const response = await fetch(`${base}${separator}route=${encodeURIComponent('/syn-plus-paper')}`, {
    method: payload ? 'POST' : 'GET',
    headers: {
      Accept: 'application/json',
      ...(payload ? { 'Content-Type': 'application/json' } : {}),
      ...(sessionStorage.getItem('qnext-ops-token')
        ? { 'X-QNext-Ops-Token': sessionStorage.getItem('qnext-ops-token') ?? '' }
        : {}),
    },
    credentials: 'same-origin',
    body: payload ? JSON.stringify(payload) : undefined,
  });
  const decoded = (await response.json()) as PaperEnvelope & PaperStatus;
  if (!response.ok) throw new Error(decoded.error ?? `HTTP ${response.status}`);
  if (typeof decoded.ok === 'boolean') {
    if (!decoded.ok) throw new Error(decoded.error ?? 'Paper runtime unavailable');
    return decoded.body ?? {};
  }
  return decoded;
}

async function loadPaper() {
  try {
    renderPaper(await requestPaper());
  } catch (error) {
    renderPaper(undefined, (error as Error).message);
  }
}

async function paperControl(action: 'enable' | 'disable' | 'reset') {
  try {
    const payload: Record<string, unknown> = { action };
    if (action === 'enable') {
      const inputs = paperInputs();
      if (!inputs) return;
      payload.timeframe = inputs.timeframe.value;
      payload.initial_capital = Number(inputs.capital.value);
      payload.unit_size = Number(inputs.unit.value);
      payload.fast = Number(inputs.fast.value);
      payload.slow = Number(inputs.slow.value);
    }
    renderPaper(await requestPaper(payload));
  } catch (error) {
    renderPaper(undefined, (error as Error).message);
  }
}

async function loadFeed() {
  try {
    renderFeed(await opsAPI().feedStatus());
  } catch (error) {
    renderFeed({ ok: false, error: (error as Error).message });
  }
}

async function loadAll() {
  await Promise.all([loadFeed(), loadPaper()]);
}

function start() {
  if (!ensurePanel()) return;
  void loadAll();
  document.querySelector<HTMLButtonElement>('#refresh-feed')?.addEventListener('click', () => {
    void loadAll();
  });
  window.setInterval(() => void loadAll(), 5_000);
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', start, { once: true });
} else {
  start();
}
