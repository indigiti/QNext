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

const token = sessionStorage.getItem('qnext-ops-token') ?? '';
const api = new OpsAPI({
  base: window.__QNEXT_OPS_CONFIG__?.apiBase,
  token,
});

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
        <p class="muted">Chart-visible shadow synthetic. It cannot replace NIFTY-SYN or become trading authority from this console.</p>
      </div>
      <button id="refresh-syn-plus" class="secondary">Refresh SYN+</button>
    </div>
    <div id="syn-plus-summary" class="status-grid"></div>
    <div id="syn-plus-quality" class="feed-provider-grid"></div>
  `;
  feedCard.insertAdjacentElement('afterend', panel);
  panel.querySelector<HTMLButtonElement>('#refresh-syn-plus')?.addEventListener('click', () => {
    void loadSynPlus();
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

function render(response: FeedStatusResponse) {
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

async function loadSynPlus() {
  try {
    render(await api.feedStatus());
  } catch (error) {
    render({ ok: false, error: (error as Error).message });
  }
}

function start() {
  if (!ensurePanel()) return;
  void loadSynPlus();
  document.querySelector<HTMLButtonElement>('#refresh-feed')?.addEventListener('click', () => {
    void loadSynPlus();
  });
  window.setInterval(() => void loadSynPlus(), 5_000);
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', start, { once: true });
} else {
  start();
}
