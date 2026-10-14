import { OpsAPI, type BackendPerformanceReport } from './api';

const backendReportAPI = new OpsAPI();
let backendReportSnapshot: BackendPerformanceReport | null = null;
let backendReportSampling = false;

function renderBackendReport(report: BackendPerformanceReport): void {
  backendReportSnapshot = report;
  const output = document.querySelector<HTMLElement>('#backend-performance-output');
  const download = document.querySelector<HTMLButtonElement>('#backend-performance-download');
  if (!output) return;
  if (download) download.disabled = report.status !== 'ready';
  if (report.status !== 'ready') {
    const elapsed = Math.round((report.window?.duration_ms ?? 0) / 1000);
    output.textContent = report.status === 'stale'
      ? 'Previous report is stale. Collecting a fresh 60-second baseline…'
      : `Collecting backend samples: ${elapsed}/60 seconds · ${report.window?.collected_samples ?? 0} samples. Keep this admin page open.`;
    return;
  }
  const broker = report.broker ?? {};
  const rows = Object.entries(report.latencies ?? {}).map(([name, value]) =>
    `${name}: p95 ${value.latest_p95_us ?? '—'} µs · p99 ${value.latest_p99_us ?? '—'} µs · new samples ${value.new_samples ?? '—'}`,
  );
  const warnings = report.warnings?.length ? `\nWarnings: ${report.warnings.join('; ')}` : '';
  output.textContent = [
    `Release: ${report.release?.commit?.slice(0, 12) ?? 'unknown'} · ${Math.round((report.window?.duration_ms ?? 0) / 1000)}s window`,
    `Subscriptions: ${broker.subscribers ?? '—'} · WSS connections: ${broker.websocket_connections ?? '—'}`,
    `Provider observations: +${report.provider_observations_delta ?? '—'} · Slow disconnects: +${report.counter_deltas?.slow_disconnects_delta ?? '—'}`,
    ...rows,
  ].join('\n') + warnings;
}

async function captureBackendReport(): Promise<void> {
  if (!document.querySelector('#backend-performance-panel') || backendReportSampling) return;
  backendReportSampling = true;
  try {
    renderBackendReport(await backendReportAPI.captureBackendPerformanceReport());
  } catch (error) {
    const output = document.querySelector<HTMLElement>('#backend-performance-output');
    if (output) output.textContent = `Backend report unavailable: ${(error as Error).message}`;
  } finally {
    backendReportSampling = false;
  }
}

function downloadBackendReport(): void {
  if (!backendReportSnapshot || backendReportSnapshot.status !== 'ready') return;
  const blob = new Blob([JSON.stringify(backendReportSnapshot, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'qnext-backend-performance-report.json';
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}


type InstrumentTelemetry = {
  provider?: string;
  last_event_time_ms?: number;
  last_trade_time_ms?: number;
  last_received_time_ms?: number;
  last_processed_time_ms?: number;
};

type BrokerStream = {
  stream_id?: string;
  instrument_id?: string;
  timeframe?: string;
  seq?: number;
  subscribers?: number;
  replay_events?: number;
  replay_slots?: number;
  last_published_at_ms?: number;
  last_bar_open_time_ms?: number;
  last_bar_final?: boolean;
  coalesced_forming?: number;
  dropped_forming?: number;
  slow_disconnects?: number;
};

type DemandTarget = {
  instrument_id?: string;
  refs?: number;
  active?: boolean;
  idle_until_ms?: number;
  last_change_ms?: number;
  last_error?: string;
};

type HistoryRuntime = {
  cache?: {
    day?: string;
    streams?: number;
    bars?: number;
    hits?: number;
    misses?: number;
    disk_loads?: number;
    last_disk_load_at_ms?: number;
    last_disk_load_duration_ms?: number;
  };
  persistence?: {
    queued?: number;
    capacity?: number;
    pending?: number;
    enqueued?: number;
    writes?: number;
    errors?: number;
    last_queued_at_ms?: number;
    last_flush_at_ms?: number;
    flush_lag_ms?: number;
    last_error?: string;
  };
};

type FeedPayload = {
  nifty_instrument_id?: string;
  synthetic_instrument_id?: string;
  telemetry?: {
    instruments?: Record<string, InstrumentTelemetry>;
    runtime?: {
      capture?: {
        enqueued?: number;
        dropped?: number;
        errors?: number;
        queued?: number;
        capacity?: number;
      };
      broker?: {
        streams?: number;
        active_streams?: number;
        idle_streams?: number;
        subscribers?: number;
        websocket_connections?: number;
        replay_slots?: number;
        replay_bytes_estimated?: number;
        compacted_streams?: number;
        reclaimed_replay_slots?: number;
        evicted_empty_streams?: number;
        publish_latency?: { samples?: number; p50_us?: number; p95_us?: number; p99_us?: number };
        ws_write_latency?: { samples?: number; p50_us?: number; p95_us?: number; p99_us?: number };
        tick_handle_latency?: { samples?: number; p50_us?: number; p95_us?: number; p99_us?: number };
        publish_to_ws_write_latency?: { samples?: number; p50_us?: number; p95_us?: number; p99_us?: number };
        replay_events?: number;
        coalesced_forming?: number;
        dropped_forming?: number;
        slow_disconnects?: number;
        details?: BrokerStream[];
      };
      demand?: {
        idle_ttl_ms?: number;
        targets?: number;
        active_targets?: number;
        total_refs?: number;
        details?: DemandTarget[];
      };
      history?: HistoryRuntime;
    };
  };
};

type ProxyStatus = {
  failures?: number;
  last_status?: number | null;
  last_at_ms?: number | null;
  last_path?: string | null;
};

function ageMS(at?: number | null): number | null {
  if (!at || at <= 0) return null;
  return Math.max(0, Date.now() - at);
}

function duration(ms: number | null): string {
  if (ms === null || !Number.isFinite(ms)) return '—';
  if (ms < 1_000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1_000).toFixed(ms < 10_000 ? 1 : 0)} s`;
  return `${(ms / 60_000).toFixed(1)} m`;
}

function stateBadge(ok: boolean, label: string): string {
  return `<span class="badge ${ok ? 'good' : 'bad'}">${label}</span>`;
}

function qnextRoot(): URL {
  return new URL('../', window.location.href);
}

async function fetchJSON<T>(path: string): Promise<T> {
  const response = await fetch(new URL(path, qnextRoot()), {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function ensurePanel(): HTMLDivElement | null {
  const card = document.querySelector<HTMLElement>('#feed-status-card');
  if (!card) return null;

  let wrap = document.querySelector<HTMLDivElement>('#live-observability');
  if (wrap) return wrap;

  const actions = card.querySelector('.feed-actions');
  const section = document.createElement('div');
  section.innerHTML = `
    <div class="line" style="margin-top: 14px; margin-bottom: 8px;">
      <div>
        <strong>Live telemetry</strong>
        <div class="muted">Auto-refresh 5s · read-only</div>
      </div>
      <span id="observability-health" class="badge good">LIVE</span>
    </div>
    <div id="live-observability" class="feed-provider-grid"></div>
    <div id="backend-performance-panel" class="feed-provider-card" style="margin-top: 14px;">
      <div class="line"><strong>Backend Performance Report</strong></div>
      <p class="muted">Admin-only · 10s samples · 60s rolling baseline · read-only Market Core</p>
      <pre id="backend-performance-output" style="white-space: pre-wrap; overflow-wrap: anywhere;">Collecting backend telemetry…</pre>
      <div class="actions">
        <button id="backend-performance-generate" class="secondary" type="button">Generate report</button>
        <button id="backend-performance-download" class="secondary" type="button" disabled>Download JSON</button>
      </div>
    </div>
  `;
  if (actions) {
    card.insertBefore(section, actions);
  } else {
    card.appendChild(section);
  }
  section.querySelector<HTMLButtonElement>('#backend-performance-generate')
    ?.addEventListener('click', () => void captureBackendReport());
  section.querySelector<HTMLButtonElement>('#backend-performance-download')
    ?.addEventListener('click', downloadBackendReport);
  void captureBackendReport();
  return section.querySelector<HTMLDivElement>('#live-observability');
}

function streamFor(details: BrokerStream[], instrument: string, timeframe: string): BrokerStream | undefined {
  return details.find((stream) =>
    stream.instrument_id === instrument && stream.timeframe === timeframe,
  );
}

function renderPrimarySyntheticDemandState(feed: FeedPayload, demandDetails: DemandTarget[]): void {
  const instrumentID = feed.synthetic_instrument_id;
  if (!instrumentID) return;
  const target = demandDetails.find((item) => item.instrument_id === instrumentID);
  if (!target || target.active) return;

  const summary = document.querySelectorAll<HTMLElement>('#feed-summary > div');
  const syntheticCard = summary.item(3);
  const syntheticLastTick = summary.item(4);
  const badge = syntheticCard?.querySelector<HTMLSpanElement>('.badge');
  if (badge) {
    badge.className = 'badge good';
    badge.textContent = 'IDLE';
  }
  const lastTickValue = syntheticLastTick?.querySelector<HTMLElement>('strong');
  if (lastTickValue) {
    lastTickValue.textContent = 'on demand';
  }
}

function render(feed: FeedPayload, proxy: ProxyStatus): void {
  const panel = ensurePanel();
  if (!panel) return;

  const telemetry = feed.telemetry ?? {};
  const runtime = telemetry.runtime ?? {};
  const capture = runtime.capture;
  const broker = runtime.broker;
  const demand = runtime.demand;
  const history = runtime.history;
  const historyCache = history?.cache;
  const persistence = history?.persistence;
  const details = broker?.details ?? [];
  const demandDetails = demand?.details ?? [];
  const niftyID = feed.nifty_instrument_id ?? 'NSE:NIFTY50';
  const nifty = telemetry.instruments?.[niftyID];
  const marketAge = ageMS(nifty?.last_event_time_ms);
  const tradeAge = ageMS(nifty?.last_trade_time_ms);
  const processingLag =
    nifty?.last_processed_time_ms && nifty?.last_event_time_ms
      ? Math.max(0, nifty.last_processed_time_ms - nifty.last_event_time_ms)
      : null;
  const bar15 = streamFor(details, niftyID, '15s');
  const bar30 = streamFor(details, niftyID, '30s');
  const bar15Age = ageMS(bar15?.last_published_at_ms);
  const bar30Age = ageMS(bar30?.last_published_at_ms);
  const captureHealthy = !capture || ((capture.dropped ?? 0) === 0 && (capture.errors ?? 0) === 0);
  const proxyHealthy = (proxy.failures ?? 0) === 0 || (ageMS(proxy.last_at_ms) ?? Number.MAX_SAFE_INTEGER) > 60_000;
  const marketHealthy = marketAge !== null && marketAge <= 30_000;
  const demandHealthy = !demandDetails.some((target) => Boolean(target.last_error));
  const persistenceHealthy = !persistence || ((persistence.errors ?? 0) === 0 && !persistence.last_error);
  const activeDemand = demandDetails.filter((target) => target.active);
  const activeIDs = activeDemand
    .map((target) => target.instrument_id)
    .filter((instrumentID): instrumentID is string => Boolean(instrumentID))
    .join(', ');

  const health = document.querySelector<HTMLSpanElement>('#observability-health');
  if (health) {
    const ok = marketHealthy && captureHealthy && proxyHealthy && demandHealthy && persistenceHealthy;
    health.className = `badge ${ok ? 'good' : 'bad'}`;
    health.textContent = ok ? 'HEALTHY' : 'CHECK';
  }

  panel.innerHTML = `
    <div class="feed-provider-card">
      <div class="line"><strong>MARKET CLOCK</strong>${stateBadge(marketHealthy, marketHealthy ? 'LIVE' : 'STALE')}</div>
      <div class="feed-kv"><span>Market age</span><strong>${duration(marketAge)}</strong></div>
      <div class="feed-kv"><span>Trade age</span><strong>${duration(tradeAge)}</strong></div>
      <div class="feed-kv"><span>Processing lag</span><strong>${duration(processingLag)}</strong></div>
      <div class="feed-kv"><span>Provider</span><strong>${nifty?.provider ?? '—'}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>LIVE BROKER</strong>${stateBadge((broker?.subscribers ?? 0) > 0, (broker?.subscribers ?? 0) > 0 ? 'ACTIVE' : 'IDLE')}</div>
      <div class="feed-kv"><span>Logical streams</span><strong>${broker?.streams ?? 0}</strong></div>
      <div class="feed-kv"><span>Active / idle streams</span><strong>${broker?.active_streams ?? '—'} / ${broker?.idle_streams ?? '—'}</strong></div>
      <div class="feed-kv"><span>Physical WSS connections</span><strong>${broker?.websocket_connections ?? '—'}</strong></div>
      <div class="feed-kv"><span>Subscribers</span><strong>${broker?.subscribers ?? 0}</strong></div>
      <div class="feed-kv"><span>Replay events / slots</span><strong>${broker?.replay_events ?? 0} / ${broker?.replay_slots ?? '—'}</strong></div>
      <div class="feed-kv"><span>Replay slot memory (shallow)</span><strong>${broker?.replay_bytes_estimated == null ? '—' : `${(broker.replay_bytes_estimated / 1024).toFixed(1)} KiB`}</strong></div>
      <div class="feed-kv"><span>Compacted streams / reclaimed slots</span><strong>${broker?.compacted_streams ?? '—'} / ${broker?.reclaimed_replay_slots ?? '—'}</strong></div>
      <div class="feed-kv"><span>Empty streams removed</span><strong>${broker?.evicted_empty_streams ?? '—'}</strong></div>
      <div class="feed-kv"><span>Broker publish p50 / p95 / p99</span><strong>${broker?.publish_latency?.samples ? `${broker.publish_latency.p50_us} / ${broker.publish_latency.p95_us} / ${broker.publish_latency.p99_us} µs` : 'warming'}</strong></div>
      <div class="feed-kv"><span>WSS write p50 / p95 / p99</span><strong>${broker?.ws_write_latency?.samples ? `${broker.ws_write_latency.p50_us} / ${broker.ws_write_latency.p95_us} / ${broker.ws_write_latency.p99_us} µs` : 'warming'}</strong></div>
      <div class="feed-kv"><span>Latency sample counts (publish / WSS)</span><strong>${broker?.publish_latency?.samples ?? 0} / ${broker?.ws_write_latency?.samples ?? 0}</strong></div>
      <div class="feed-kv"><span>Tick processing p50 / p95 / p99</span><strong>${broker?.tick_handle_latency?.samples ? `${broker.tick_handle_latency.p50_us} / ${broker.tick_handle_latency.p95_us} / ${broker.tick_handle_latency.p99_us} µs` : 'warming'}</strong></div>
      <div class="feed-kv"><span>Live publish → WSS write p50 / p95 / p99</span><strong>${broker?.publish_to_ws_write_latency?.samples ? `${broker.publish_to_ws_write_latency.p50_us} / ${broker.publish_to_ws_write_latency.p95_us} / ${broker.publish_to_ws_write_latency.p99_us} µs` : 'warming'}</strong></div>
      <div class="feed-kv"><span>Trace samples (tick / WSS delivery)</span><strong>${broker?.tick_handle_latency?.samples ?? 0} / ${broker?.publish_to_ws_write_latency?.samples ?? 0}</strong></div>
      <div class="feed-kv"><span>Active stream subscriptions</span><strong>${details.filter((stream) => (stream.subscribers ?? 0) > 0).map((stream) => `${stream.instrument_id} ${stream.timeframe}: ${stream.subscribers}`).join(', ') || 'none'}</strong></div>
      <div class="feed-kv"><span>Coalesced forming</span><strong>${broker?.coalesced_forming ?? 0}</strong></div>
      <div class="feed-kv"><span>Dropped forming</span><strong>${broker?.dropped_forming ?? 0}</strong></div>
      <div class="feed-kv"><span>Slow disconnects</span><strong>${broker?.slow_disconnects ?? 0}</strong></div>
      <div class="feed-kv"><span>NIFTY 15s bar age</span><strong>${duration(bar15Age)}</strong></div>
      <div class="feed-kv"><span>NIFTY 30s bar age</span><strong>${duration(bar30Age)}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>HISTORY PATH</strong>${stateBadge(persistenceHealthy, persistenceHealthy ? 'HEALTHY' : 'CHECK')}</div>
      <div class="feed-kv"><span>Cache day</span><strong>${historyCache?.day ?? 'warming'}</strong></div>
      <div class="feed-kv"><span>Cached streams / bars</span><strong>${historyCache?.streams ?? 0} / ${historyCache?.bars ?? 0}</strong></div>
      <div class="feed-kv"><span>Cache hits / misses</span><strong>${historyCache?.hits ?? 0} / ${historyCache?.misses ?? 0}</strong></div>
      <div class="feed-kv"><span>JSONL disk loads</span><strong>${historyCache?.disk_loads ?? 0}</strong></div>
      <div class="feed-kv"><span>Last disk load</span><strong>${duration(historyCache?.last_disk_load_duration_ms ?? null)}</strong></div>
      <div class="feed-kv"><span>Persist queue</span><strong>${persistence ? `${persistence.queued ?? 0} / ${persistence.capacity ?? 0}` : 'warming'}</strong></div>
      <div class="feed-kv"><span>Pending finals</span><strong>${persistence?.pending ?? 0}</strong></div>
      <div class="feed-kv"><span>Persist writes / errors</span><strong>${persistence?.writes ?? 0} / ${persistence?.errors ?? 0}</strong></div>
      <div class="feed-kv"><span>Flush lag</span><strong>${duration(persistence?.flush_lag_ms ?? null)}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>LIVE DEMAND</strong>${stateBadge(demandHealthy, demandHealthy ? 'HEALTHY' : 'CHECK')}</div>
      <div class="feed-kv"><span>Synthetic targets</span><strong>${demand?.targets ?? 0}</strong></div>
      <div class="feed-kv"><span>Active synthetics</span><strong>${demand?.active_targets ?? 0}</strong></div>
      <div class="feed-kv"><span>Consumer refs</span><strong>${demand?.total_refs ?? 0}</strong></div>
      <div class="feed-kv"><span>Idle grace</span><strong>${duration(demand?.idle_ttl_ms ?? null)}</strong></div>
      <div class="feed-kv"><span>Active IDs</span><strong>${activeIDs || 'none'}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>RAW CAPTURE</strong>${stateBadge(captureHealthy, capture ? (captureHealthy ? 'HEALTHY' : 'CHECK') : 'OFF')}</div>
      <div class="feed-kv"><span>Queue</span><strong>${capture ? `${capture.queued ?? 0} / ${capture.capacity ?? 0}` : 'disabled'}</strong></div>
      <div class="feed-kv"><span>Enqueued</span><strong>${capture?.enqueued ?? 0}</strong></div>
      <div class="feed-kv"><span>Dropped</span><strong>${capture?.dropped ?? 0}</strong></div>
      <div class="feed-kv"><span>Write errors</span><strong>${capture?.errors ?? 0}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>REST PROXY</strong>${stateBadge(proxyHealthy, proxyHealthy ? 'HEALTHY' : 'RECENT 5XX')}</div>
      <div class="feed-kv"><span>5xx / 502 failures</span><strong>${proxy.failures ?? 0}</strong></div>
      <div class="feed-kv"><span>Last status</span><strong>${proxy.last_status ?? '—'}</strong></div>
      <div class="feed-kv"><span>Last failure</span><strong>${duration(ageMS(proxy.last_at_ms))}</strong></div>
      <div class="feed-kv"><span>Last path</span><strong>${proxy.last_path ?? '—'}</strong></div>
    </div>
  `;

  renderPrimarySyntheticDemandState(feed, demandDetails);
}

async function refreshObservability(): Promise<void> {
  const panel = ensurePanel();
  if (!panel) {
    window.setTimeout(() => void refreshObservability(), 250);
    return;
  }

  try {
    const [feed, proxy] = await Promise.all([
      fetchJSON<FeedPayload>('api/v1/feed-status/'),
      fetchJSON<ProxyStatus>('api/v1/proxy-status/'),
    ]);
    render(feed, proxy);
  } catch (error) {
    const health = document.querySelector<HTMLSpanElement>('#observability-health');
    if (health) {
      health.className = 'badge bad';
      health.textContent = 'UNAVAILABLE';
    }
    panel.innerHTML = `<div class="feed-provider-card"><strong>Telemetry unavailable</strong><div class="muted">${(error as Error).message}</div></div>`;
  }
}

void refreshObservability();
window.setInterval(() => void refreshObservability(), 5_000);
// Private Ops API computes and stores the rolling baseline server-side.
window.setInterval(() => void captureBackendReport(), 10_000);
