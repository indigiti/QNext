import {
  OpsAPI,
  type FeedStatusResponse,
  type HistoricalRepairResult,
  type HistoricalRepairStatusResponse,
  type OpsStatus,
  type RuntimeDiagnostics,
  type ServiceAction,
  type CustomIndicator,
} from './api';
import './style.css';

declare global {
  interface Window {
    __QNEXT_OPS_CONFIG__?: {
      apiBase?: string;
    };
  }
}

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) {
  throw new Error('QNext Ops Console root not found');
}

const runtime = window.__QNEXT_OPS_CONFIG__ ?? {};
let token = sessionStorage.getItem('qnext-ops-token') ?? '';
let api = new OpsAPI({ base: runtime.apiBase, token });
let setupInitialized: boolean | null = null;

root.innerHTML = `
  <div class="shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">QNext</p>
        <h1>Operations Console</h1>
        <p class="muted">Restricted deployment and runtime control plane</p>
      </div>
      <div class="auth">
        <label for="token">Staging admin token</label>
        <input id="token" type="password" autocomplete="off" placeholder="Enter token" />
        <button id="save-token" class="secondary">Use token</button>
      </div>
    </header>

    <main class="grid">
      <section class="card span-2">
        <div class="card-head">
          <div>
            <p class="eyebrow">Runtime</p>
            <h2>Market Core</h2>
          </div>
          <button id="refresh">Refresh</button>
        </div>
        <div id="runtime-status" class="status-grid"></div>
        <div id="cron-setup" class="cron-setup" hidden>
          <strong>Cloudways Cron Supervisor</strong>
          <p class="muted">Add this once in Cloudways → Cron Job Management → Advanced. QNext will then manage Start / Stop / Restart without PHP process functions.</p>
          <code id="cron-command"></code>
          <button id="copy-cron" class="secondary">Copy cron entry</button>
        </div>
        <div class="actions">
          <button data-service="start">Start</button>
          <button data-service="restart">Restart</button>
          <button data-service="stop" class="danger">Stop</button>
          <button id="smoke" class="secondary">Run smoke test</button>
        </div>
      </section>

      <section class="card">
        <p class="eyebrow">Release</p>
        <h2>Deploy / rollback</h2>
        <div id="release-status" class="stack"></div>
        <div class="actions vertical">
          <select id="release-select"></select>
          <button id="activate">Activate selected release</button>
          <button id="rollback" class="secondary">Rollback previous</button>
        </div>
      </section>

      <section class="card span-3" id="diagnostics-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Diagnostics</p>
            <h2>Market Core runtime</h2>
          </div>
          <button id="refresh-diagnostics" class="secondary">Refresh diagnostics</button>
        </div>
        <div id="diagnostics-meta" class="status-grid diagnostics-meta"></div>
        <div class="diagnostics-log-wrap">
          <div class="line">
            <span class="muted">Recent Market Core log</span>
            <button id="copy-diagnostics" class="secondary compact">Copy</button>
          </div>
          <pre id="diagnostics-log">No runtime log available yet.</pre>
        </div>
      </section>

      <section class="card span-3" id="feed-status-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Market feed</p>
            <h2>Live feed status</h2>
          </div>
          <button id="refresh-feed" class="secondary">Refresh feed</button>
        </div>
        <div id="feed-summary" class="status-grid feed-summary"></div>
        <div id="feed-providers" class="feed-provider-grid"></div>
        <div class="actions feed-actions">
          <button id="probe-stream" class="secondary">Probe WSS</button>
          <span id="browser-transport-status" class="muted">Browser transport: probing...</span>
        </div>
        <div class="actions feed-actions">
          <button id="verify-dhan" class="secondary">Verify Dhan standby</button>
          <span id="dhan-standby-result" class="muted">Configure Dhan credentials, restart Market Core, then verify standby readiness.</span>
        </div>
      </section>

      <section class="card span-3" id="active-markets-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Subscriptions</p>
            <h2>Active markets</h2>
            <p class="muted">Enable only the index + synthetic pairs you need. Disabled pairs create no live index subscription, option basket, or synthetic calculation.</p>
          </div>
          <button id="reload-active-markets" class="secondary">Reload</button>
        </div>
        <div id="active-markets-grid" class="market-toggle-grid"></div>
        <div class="actions">
          <button id="nifty-only" class="secondary">NIFTY only</button>
          <button id="enable-all-markets" class="secondary">Enable all</button>
          <button id="save-active-markets">Save & restart</button>
          <span id="active-markets-note" class="muted">At least one market must remain active.</span>
        </div>
      </section>

      <section class="card span-3" id="candle-timeframes-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Candles</p>
            <h2>Enabled timeframes</h2>
            <p class="muted">15s and 30s are tick-built. 1m is the protected canonical source. Higher intraday/day intervals are rolled up from 1m; disabled intervals are not continuously built or stored.</p>
          </div>
          <button id="reload-candle-timeframes" class="secondary">Reload</button>
        </div>
        <div id="candle-timeframes-grid" class="market-toggle-grid"></div>
        <div class="actions">
          <button id="candle-defaults" class="secondary">Use defaults</button>
          <button id="save-candle-timeframes">Save & restart</button>
          <span class="muted">1m is locked ON for automatic Upstox recovery and local rollups.</span>
        </div>
      </section>

      <section class="card span-3" id="chart-timeframes-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Chart Display</p>
            <h2>Enabled timeframes</h2>
            <p class="muted">Controls only the chart timeframe menu. A chart timeframe can be shown only when its Candle Formation timeframe is enabled.</p>
          </div>
          <button id="reload-chart-timeframes" class="secondary">Reload</button>
        </div>
        <div id="chart-timeframes-grid" class="market-toggle-grid"></div>
        <div class="actions">
          <button id="chart-defaults" class="secondary">Use defaults</button>
          <button id="save-chart-timeframes">Save & restart</button>
          <span class="muted">Hidden chart intervals remain available for later display if Candle Formation is enabled.</span>
        </div>
      </section>

      <section class="card span-3" id="historical-repair-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">History</p>
            <h2>Historical recovery</h2>
            <p class="muted">Repairs active cash-index history. 1m is canonical for minute/hour/day rollups; weekly/monthly data uses Upstox historical V3. Seconds remain live-only because the provider does not expose historical second candles.</p>
          </div>
          <button id="refresh-history-repair" class="secondary">Refresh status</button>
        </div>
        <div id="historical-repair-status" class="status-grid"></div>
        <div class="actions history-repair-actions">
          <button data-history-days="3" class="secondary">Repair 3 days</button>
          <button data-history-days="7" class="secondary">Repair 7 days</button>
          <button data-history-days="15" class="secondary">Repair 15 days</button>
          <button data-history-days="30">Repair 30 days</button>
        </div>
        <p class="muted">Startup automatically reconciles 3 days. Manual repair scope is the currently active markets. Missing bars are inserted; differing bars are appended as higher revisions.</p>
        <div id="historical-repair-result" class="history-repair-result muted">No manual repair run in this session.</div>
      </section>


      <section class="card span-3" id="custom-indicators-card">
        <div class="card-head">
          <div>
            <p class="eyebrow">Charts</p>
            <h2>Custom Indicators</h2>
            <p class="muted">Manage native QNext indicators and full Pine Script v6 source. Pine runs off the main UI thread through the Vela PineWorkerEngine.</p>
          </div>
          <div class="actions indicator-head-actions">
            <button id="add-pine-indicator" type="button">+ Add Pine Script</button>
            <button id="reload-custom-indicators" type="button" class="secondary">Reload</button>
          </div>
        </div>
        <div id="custom-indicator-list" class="custom-indicator-list"></div>
        <form id="custom-indicator-form" class="indicator-form">
          <input type="hidden" name="editingId" />
          <div class="indicator-form-grid">
            <label><span>Name</span><input name="name" placeholder="My Pine Indicator" required /></label>
            <label><span>ID</span><input name="id" placeholder="my-pine-indicator" pattern="[a-z0-9][a-z0-9-]{0,63}" required /></label>
            <label><span>Kind</span><select name="kind" id="custom-indicator-kind">
              <option value="pine-v6">Pine Script v6</option>
              <option value="adaptive-ema-qalg">Adaptive EMA [QALG]</option>
            </select></label>
            <label class="checkbox-field"><span>Enabled</span><input name="enabled" type="checkbox" checked /></label>
          </div>

          <div id="pine-script-settings" class="indicator-kind-panel">
            <div class="line">
              <div>
                <strong>Pine Script v6 source</strong>
                <div class="muted">Paste an indicator() or strategy() script beginning with //@version=6.</div>
              </div>
              <span class="badge good">WEB WORKER</span>
            </div>
            <textarea id="pine-script-editor" name="script" class="pine-code-editor" spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
            <p class="muted pine-license-note">Runtime: @luxalgo/vela-pinets + PineTS (AGPL-3.0-only). QNext stores your source file-backed and publishes enabled scripts under Indicators → QNext.</p>
          </div>

          <div id="native-indicator-settings" class="indicator-kind-panel" hidden>
            <div class="indicator-form-grid">
              <label><span>Price Source</span><select name="priceSource">
                <option value="close">close</option><option value="open">open</option><option value="high">high</option><option value="low">low</option>
                <option value="hl2">hl2</option><option value="hlc3">hlc3</option><option value="ohlc4">ohlc4</option>
              </select></label>
              <label><span>EMA Length</span><input name="emaLength" type="number" min="1" max="1000" value="20" /></label>
              <label><span>SD Lookback</span><input name="lookbackPeriod" type="number" min="2" max="1000" value="30" /></label>
              <label><span>SD Multiplier</span><input name="stddevMultiplier" type="number" min="0.1" max="20" step="0.1" value="2" /></label>
              <label><span>ATR Length</span><input name="atrLength" type="number" min="1" max="1000" value="14" /></label>
              <label><span>ATR Multiplier</span><input name="atrMultiplier" type="number" min="0.1" max="20" step="0.1" value="1.5" /></label>
              <label><span>Up Color</span><input name="upColor" type="color" value="#00ffaa" /></label>
              <label><span>Down Color</span><input name="downColor" type="color" value="#ff0000" /></label>
              <label class="checkbox-field"><span>Color Bars</span><input name="colorBars" type="checkbox" checked /></label>
            </div>
          </div>

          <div class="indicator-form-grid">
            <label><span>Attribution</span><input name="attribution" placeholder="Author / source" /></label>
            <label><span>License</span><input name="license" placeholder="e.g. MPL-2.0" /></label>
            <label><span>License URL</span><input name="licenseUrl" placeholder="https://..." /></label>
          </div>
          <label><span>Description</span><input name="description" placeholder="What this indicator does" /></label>
          <div class="actions">
            <button type="submit" id="save-custom-indicator">Add Pine indicator</button>
            <button type="button" id="cancel-custom-indicator" class="secondary">Reset</button>
          </div>
        </form>
      </section>

      <section class="card">
        <p class="eyebrow">Secrets</p>
        <h2>Broker credentials</h2>
        <p class="muted">Values are write-only. Existing secrets are never returned by the API.</p>
        <form id="secret-form" class="stack">
          <input name="UPSTOX_ACCESS_TOKEN" type="password" placeholder="Upstox access token" />
          <input name="DHAN_CLIENT_ID" type="password" placeholder="Dhan client ID" />
          <input name="DHAN_ACCESS_TOKEN" type="password" placeholder="Dhan access token" />
          <button type="submit">Save provided secrets</button>
        </form>
      </section>

      <section class="card span-2">
        <div class="card-head">
          <div>
            <p class="eyebrow">Configuration</p>
            <h2>Market configuration</h2>
          </div>
          <button id="load-config" class="secondary">Reload</button>
        </div>
        <textarea id="config-editor" spellcheck="false"></textarea>
        <div class="actions">
          <button id="save-config">Validate & save</button>
        </div>
      </section>
    </main>

    <div id="toast" role="status" aria-live="polite"></div>
  </div>
`;

const tokenInput = document.querySelector<HTMLInputElement>('#token')!;
const tokenLabel = document.querySelector<HTMLLabelElement>('label[for="token"]')!;
const tokenButton = document.querySelector<HTMLButtonElement>('#save-token')!;
tokenInput.value = token;

function toast(message: string, error = false) {
  const el = document.querySelector<HTMLDivElement>('#toast')!;
  el.textContent = message;
  el.className = error ? 'show error' : 'show';
  window.setTimeout(() => {
    el.className = '';
  }, 4500);
}

function badge(ok: boolean, label: string) {
  return `<span class="badge ${ok ? 'good' : 'bad'}">${label}</span>`;
}

function renderStatus(status: OpsStatus) {
  const runtimeStatus = document.querySelector<HTMLDivElement>('#runtime-status')!;
  runtimeStatus.innerHTML = `
    <div><span>Service</span><strong>${status.service.state}</strong></div>
    <div><span>/health</span>${badge(status.marketCore.health.ok, status.marketCore.health.ok ? 'PASS' : 'FAIL')}</div>
    <div><span>/ready</span>${badge(status.marketCore.ready.ok, status.marketCore.ready.ok ? 'PASS' : 'FAIL')}</div>
    <div><span>/version</span>${badge(status.marketCore.version.ok, status.marketCore.version.ok ? 'PASS' : 'FAIL')}</div>
    <div><span>Storage</span><strong>${status.storageRoot}</strong></div>
    <div><span>Config</span><strong>${status.configPath}</strong></div>
    <div><span>Control</span>${badge(status.host.controlMode !== 'setup', status.host.controlMode.toUpperCase())}</div>
    <div><span>Cron supervisor</span>${badge(status.host.cronControl, status.host.cronControl ? 'PASS' : (status.host.processControl ? 'N/A' : 'SETUP'))}</div>
    <div><span>Helper</span>${badge(status.host.helperAvailable, status.host.helperAvailable ? 'PASS' : 'FAIL')}</div>
  `;

  const releaseStatus = document.querySelector<HTMLDivElement>('#release-status')!;
  releaseStatus.innerHTML = `
    <div class="line"><span>Current</span><strong>${status.release.current ?? 'none'}</strong></div>
    <div class="line"><span>Mode</span><strong>${status.release.mode ?? 'staged'}</strong></div>
    <div class="line"><span>Available</span><strong>${status.release.available.length}</strong></div>
  `;

  const select = document.querySelector<HTMLSelectElement>('#release-select')!;
  const activate = document.querySelector<HTMLButtonElement>('#activate')!;
  const rollback = document.querySelector<HTMLButtonElement>('#rollback')!;
  const directMode = status.release.mode === 'direct';

  select.innerHTML = status.release.available
    .map((version) => `<option value="${version}" ${version === status.release.current ? 'selected' : ''}>${version}</option>`)
    .join('');
  select.disabled = directMode || status.release.available.length === 0;
  activate.disabled = directMode || status.release.available.length === 0;
  rollback.disabled = directMode;

  const serviceReady =
    (status.host.processControl || status.host.cronControl) && status.host.helperAvailable;
  document.querySelectorAll<HTMLButtonElement>('[data-service]').forEach((button) => {
    button.disabled = !serviceReady;
  });

  const cronSetup = document.querySelector<HTMLDivElement>('#cron-setup')!;
  const cronCommand = document.querySelector<HTMLElement>('#cron-command')!;
  cronSetup.hidden = status.host.processControl || status.host.cronControl;
  cronCommand.textContent = status.host.cronCommand;

  if (!status.host.helperAvailable) {
    toast('QNext runtime helper is missing from the private deployment payload.', true);
  } else if (!status.host.processControl && !status.host.cronControl) {
    toast('One-time setup: add the Cloudways cron supervisor entry shown under Runtime.', true);
  }
}

function ageLabel(atMS?: number) {
  if (!atMS || atMS <= 0) return 'never';
  const age = Math.max(0, Date.now() - atMS);
  if (age < 1000) return '<1s ago';
  if (age < 60_000) return `${Math.floor(age / 1000)}s ago`;
  if (age < 3_600_000) return `${Math.floor(age / 60_000)}m ago`;
  return `${Math.floor(age / 3_600_000)}h ago`;
}

function feedState(atMS?: number) {
  if (!atMS || atMS <= 0) return { ok: false, label: 'WAITING' };
  return Date.now() - atMS <= 30_000
    ? { ok: true, label: 'LIVE' }
    : { ok: false, label: 'STALE' };
}

function formatPrice(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
  return value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function renderFeedStatus(response: FeedStatusResponse) {
  const summary = document.querySelector<HTMLDivElement>('#feed-summary')!;
  const providers = document.querySelector<HTMLDivElement>('#feed-providers')!;

  if (!response.ok || !response.body) {
    summary.innerHTML = `
      <div><span>Status</span>${badge(false, 'UNAVAILABLE')}</div>
      <div><span>Reason</span><strong>${response.error ?? 'Market Core feed status unavailable'}</strong></div>
    `;
    providers.innerHTML = '';
    return;
  }

  const body = response.body;
  const telemetry = body.telemetry ?? { providers: {}, instruments: {} };
  const providerTelemetry = telemetry.providers ?? {};
  const resilienceProviders = body.resilience?.providers ?? {};
  const nifty = telemetry.instruments?.[body.nifty_instrument_id];
  const synthetic = telemetry.instruments?.[body.synthetic_instrument_id];
  const syntheticRuntime = telemetry.synthetic;
  const niftyState = feedState(nifty?.last_event_time_ms);
  const syntheticState = feedState(synthetic?.last_event_time_ms);
  const authority = body.resilience?.active_authorities?.[body.nifty_instrument_id]
    ?? nifty?.provider
    ?? '—';

  summary.innerHTML = `
    <div><span>NIFTY</span>${badge(niftyState.ok, niftyState.label)}<strong class="feed-price">${formatPrice(nifty?.price)}</strong></div>
    <div><span>NIFTY last tick</span><strong>${ageLabel(nifty?.last_event_time_ms)}</strong></div>
    <div><span>Authority</span><strong>${authority}</strong></div>
    <div><span>NIFTY-SYN</span>${badge(syntheticState.ok, syntheticState.label)}<strong class="feed-price">${formatPrice(synthetic?.price)}</strong></div>
    <div><span>Synthetic last tick</span><strong>${ageLabel(synthetic?.last_event_time_ms)}</strong></div>
    <div><span>ATM / legs</span><strong>${syntheticRuntime?.atm ?? '—'} / ${syntheticRuntime?.active_legs ?? '—'}</strong></div>
  `;

  const providerCard = (name: string, configured: boolean) => {
    const accepted = providerTelemetry[name];
    const raw = resilienceProviders[name];
    const lastEvent = Math.max(
      accepted?.last_event_time_ms ?? 0,
      raw?.last_event_time_ms ?? 0,
    );
    const state = configured ? feedState(lastEvent) : { ok: false, label: 'NOT CONFIGURED' };
    const received = raw?.received ?? accepted?.observed ?? 0;
    const errors = raw?.errors ?? 0;
    return `
      <div class="feed-provider-card">
        <div class="line"><strong>${name.toUpperCase()}</strong>${badge(state.ok, state.label)}</div>
        <div class="feed-kv"><span>Last event</span><strong>${configured ? ageLabel(lastEvent) : '—'}</strong></div>
        <div class="feed-kv"><span>Received</span><strong>${received}</strong></div>
        <div class="feed-kv"><span>Errors</span><strong>${errors}</strong></div>
      </div>
    `;
  };

  const gap = body.gap_recovery;
  const gapState = (gap?.failures ?? 0) > 0 && (gap?.failures ?? 0) >= (gap?.successes ?? 0)
    ? { ok: false, label: 'CHECK' }
    : { ok: true, label: (gap?.attempts ?? 0) > 0 ? 'ARMED / USED' : 'ARMED' };
  const gapCard = `
    <div class="feed-provider-card">
      <div class="line"><strong>AUTO GAP RECOVERY</strong>${badge(gapState.ok, gapState.label)}</div>
      <div class="feed-kv"><span>Attempts / success</span><strong>${gap?.attempts ?? 0} / ${gap?.successes ?? 0}</strong></div>
      <div class="feed-kv"><span>Recovered bars</span><strong>${gap?.recovered_bars ?? 0}</strong></div>
      <div class="feed-kv"><span>Exact tick replay</span><strong>${gap?.exact_tick_replay ? 'YES' : 'NO'}</strong></div>
      <div class="feed-kv"><span>Exact history TF</span><strong>${gap?.recovered_timeframes?.join(', ') || '—'}</strong></div>
      <div class="feed-kv"><span>Non-exact TF</span><strong>${gap?.non_exact_timeframes?.join(', ') || '—'}</strong></div>
      ${gap?.last_error ? `<div class="feed-kv"><span>Last error</span><strong>${gap.last_error}</strong></div>` : ''}
    </div>
  `;

  providers.innerHTML =
    providerCard('upstox', body.live_configured) +
    providerCard('dhan', body.resilience_configured) +
    gapCard;
}

async function probeBrowserStream() {
  const status = document.querySelector<HTMLSpanElement>('#browser-transport-status')!;
  status.textContent = 'Browser transport: probing WSS...';

  const url = new URL('../api/v1/stream', window.location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';

  await new Promise<void>((resolve) => {
    let settled = false;
    const socket = new WebSocket(url.toString());
    const finish = (label: string) => {
      if (settled) return;
      settled = true;
      status.textContent = `Browser transport: ${label}`;
      try { socket.close(); } catch {}
      resolve();
    };
    const timer = window.setTimeout(() => finish('REST FALLBACK'), 2500);

    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(String(event.data)) as { op?: string; protocol?: string };
        if (message.op === 'hello' && message.protocol === 'QNEXT.STREAM/1') {
          window.clearTimeout(timer);
          finish('WSS READY');
        }
      } catch {
        // Ignore non-protocol frames during the short probe.
      }
    };
    socket.onerror = () => {
      window.clearTimeout(timer);
      finish('REST FALLBACK');
    };
    socket.onclose = () => {
      if (!settled) {
        window.clearTimeout(timer);
        finish('REST FALLBACK');
      }
    };
  });
}

async function loadFeedStatus() {
  try {
    renderFeedStatus(await api.feedStatus());
  } catch (error) {
    renderFeedStatus({ ok: false, error: (error as Error).message });
  }
}

function renderDiagnostics(diagnostics: RuntimeDiagnostics) {
  const meta = document.querySelector<HTMLDivElement>('#diagnostics-meta')!;
  const heartbeat = diagnostics.cronHeartbeatAgeSeconds === null
    ? 'none'
    : `${diagnostics.cronHeartbeatAgeSeconds}s ago`;

  meta.innerHTML = `
    <div><span>Desired state</span><strong>${diagnostics.desiredState}</strong></div>
    <div><span>PID</span><strong>${diagnostics.pid ?? 'none'}</strong></div>
    <div><span>PID alive</span>${badge(diagnostics.pidAlive, diagnostics.pidAlive ? 'PASS' : 'NO')}</div>
    <div><span>Binary</span>${badge(diagnostics.binaryFound, diagnostics.binaryFound ? 'FOUND' : 'MISSING')}</div>
    <div><span>Cron heartbeat</span><strong>${heartbeat}</strong></div>
    <div><span>Control mode</span><strong>${diagnostics.controlMode.toUpperCase()}</strong></div>
  `;

  const log = document.querySelector<HTMLPreElement>('#diagnostics-log')!;
  log.textContent = diagnostics.logLines.length > 0
    ? diagnostics.logLines.join('\n')
    : 'No runtime log available yet.';

  log.dataset.logPath = diagnostics.logPath;
  log.dataset.binaryPath = diagnostics.binaryPath ?? '';
}

async function loadDiagnostics() {
  try {
    renderDiagnostics(await api.diagnostics());
  } catch (error) {
    const log = document.querySelector<HTMLPreElement>('#diagnostics-log')!;
    log.textContent = `Diagnostics unavailable: ${(error as Error).message}`;
  }
}

async function refresh() {
  try {
    const [status] = await Promise.all([
      api.status(),
      loadDiagnostics(),
      loadFeedStatus(),
      loadHistoricalRepairStatus(),
    ]);
    renderStatus(status);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

const marketLabels: Record<string, string> = {
  NIFTY: 'NIFTY + NIFTY-SYN',
  BANKNIFTY: 'BANKNIFTY + BANKNIFTY-SYN',
  MIDCPNIFTY: 'MIDCPNIFTY + MIDCPNIFTY-SYN',
  FINNIFTY: 'FINNIFTY + FINNIFTY-SYN',
  SENSEX: 'SENSEX + SENSEX-SYN',
  BANKEX: 'BANKEX + BANKEX-SYN',
};

function renderActiveMarkets(available: string[], active: string[]) {
  const grid = document.querySelector<HTMLDivElement>('#active-markets-grid')!;
  const activeSet = new Set(active);
  grid.innerHTML = available.map((symbol) => `
    <label class="market-toggle">
      <input type="checkbox" name="active-market" value="${symbol}" ${activeSet.has(symbol) ? 'checked' : ''} />
      <span>
        <strong>${marketLabels[symbol] ?? symbol}</strong>
        <small>${activeSet.has(symbol) ? 'ACTIVE' : 'DISABLED'}</small>
      </span>
    </label>
  `).join('');

  grid.querySelectorAll<HTMLInputElement>('input[name="active-market"]').forEach((input) => {
    input.addEventListener('change', () => {
      const label = input.closest<HTMLLabelElement>('.market-toggle');
      const small = label?.querySelector('small');
      if (small) small.textContent = input.checked ? 'ACTIVE' : 'DISABLED';
    });
  });
}

async function loadActiveMarkets() {
  try {
    const state = await api.activeMarkets();
    renderActiveMarkets(state.available, state.active);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

function selectedActiveMarkets() {
  return Array.from(
    document.querySelectorAll<HTMLInputElement>('input[name="active-market"]:checked'),
  ).map((input) => input.value);
}

function renderCandleTimeframes(
  available: string[],
  enabled: string[],
  protectedTimeframes: string[],
  defaults: string[],
) {
  const grid = document.querySelector<HTMLDivElement>('#candle-timeframes-grid')!;
  const enabledSet = new Set(enabled);
  const protectedSet = new Set(protectedTimeframes);
  const defaultSet = new Set(defaults);

  grid.dataset.defaults = JSON.stringify(defaults);
  grid.innerHTML = available.map((timeframe) => {
    const locked = protectedSet.has(timeframe);
    const checked = enabledSet.has(timeframe) || locked;
    const detail = locked
      ? 'CORE / PROTECTED'
      : checked
        ? (defaultSet.has(timeframe) ? 'ENABLED / DEFAULT' : 'ENABLED')
        : 'DISABLED';
    return `
      <label class="market-toggle">
        <input
          type="checkbox"
          name="candle-timeframe"
          value="${timeframe}"
          ${checked ? 'checked' : ''}
          ${locked ? 'disabled data-protected="true"' : ''}
        />
        <span>
          <strong>${timeframe}</strong>
          <small>${detail}</small>
        </span>
      </label>
    `;
  }).join('');

  grid.querySelectorAll<HTMLInputElement>('input[name="candle-timeframe"]').forEach((input) => {
    input.addEventListener('change', () => {
      const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
      if (small) small.textContent = input.checked ? 'ENABLED' : 'DISABLED';
    });
  });
}

async function loadCandleTimeframes() {
  try {
    const state = await api.candleTimeframes();
    renderCandleTimeframes(state.available, state.enabled, state.protected, state.defaults);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

function selectedCandleTimeframes() {
  const selected = Array.from(
    document.querySelectorAll<HTMLInputElement>('input[name="candle-timeframe"]:checked'),
  ).map((input) => input.value);
  if (!selected.includes('1m')) selected.push('1m');
  return selected;
}

function renderChartTimeframes(
  available: string[],
  enabled: string[],
  candleEnabled: string[],
  defaults: string[],
) {
  const grid = document.querySelector<HTMLDivElement>('#chart-timeframes-grid')!;
  const enabledSet = new Set(enabled);
  const candleSet = new Set(candleEnabled);
  const defaultSet = new Set(defaults);

  grid.dataset.defaults = JSON.stringify(defaults);
  grid.dataset.candleEnabled = JSON.stringify(candleEnabled);
  grid.innerHTML = available.map((timeframe) => {
    const availableForChart = candleSet.has(timeframe);
    const checked = availableForChart && enabledSet.has(timeframe);
    const detail = !availableForChart
      ? 'CANDLE OFF'
      : checked
        ? (defaultSet.has(timeframe) ? 'VISIBLE / DEFAULT' : 'VISIBLE')
        : 'HIDDEN';
    return `
      <label class="market-toggle">
        <input
          type="checkbox"
          name="chart-timeframe"
          value="${timeframe}"
          ${checked ? 'checked' : ''}
          ${availableForChart ? '' : 'disabled data-candle-off="true"'}
        />
        <span>
          <strong>${timeframe}</strong>
          <small>${detail}</small>
        </span>
      </label>
    `;
  }).join('');

  grid.querySelectorAll<HTMLInputElement>('input[name="chart-timeframe"]').forEach((input) => {
    input.addEventListener('change', () => {
      const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
      if (small) small.textContent = input.checked ? 'VISIBLE' : 'HIDDEN';
    });
  });
}

async function loadChartTimeframes() {
  try {
    const state = await api.chartTimeframes();
    renderChartTimeframes(state.available, state.enabled, state.candleEnabled, state.defaults);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

function selectedChartTimeframes() {
  return Array.from(
    document.querySelectorAll<HTMLInputElement>('input[name="chart-timeframe"]:checked'),
  ).map((input) => input.value);
}

function renderHistoricalRepairStatus(response: HistoricalRepairStatusResponse) {
  const status = document.querySelector<HTMLDivElement>('#historical-repair-status')!;
  if (!response.ok || !response.body) {
    status.innerHTML = `
      <div><span>Status</span>${badge(false, 'UNAVAILABLE')}</div>
      <div><span>Reason</span><strong>${response.error ?? 'History repair status unavailable'}</strong></div>
    `;
    return;
  }

  const body = response.body;
  const last = body.last_result;
  const totals = { scanned: 0, missing: 0, corrected: 0, unchanged: 0 };
  for (const market of last?.markets ?? []) {
    for (const counts of Object.values(market.timeframes ?? {})) {
      totals.scanned += counts.scanned ?? 0;
      totals.missing += counts.missing ?? 0;
      totals.corrected += counts.corrected ?? 0;
      totals.unchanged += counts.unchanged ?? 0;
    }
  }

  status.innerHTML = `
    <div><span>State</span>${badge(!body.last_error, body.running ? 'RUNNING' : 'READY')}</div>
    <div><span>Last window</span><strong>${last?.days ? `${last.days} days` : '—'}</strong></div>
    <div><span>Markets</span><strong>${last?.markets?.length ?? 0}</strong></div>
    <div><span>Scanned</span><strong>${totals.scanned}</strong></div>
    <div><span>Inserted</span><strong>${totals.missing}</strong></div>
    <div><span>Corrected</span><strong>${totals.corrected}</strong></div>
    <div><span>Unchanged</span><strong>${totals.unchanged}</strong></div>
    <div><span>Last error</span><strong>${body.last_error || 'none'}</strong></div>
  `;
}

function renderHistoricalRepairResult(result: HistoricalRepairResult) {
  const target = document.querySelector<HTMLDivElement>('#historical-repair-result')!;
  const order = ['1m', '2m', '3m', '5m', '10m', '15m', '30m', '45m', '1h', '2h', '3h', '4h', '1D', '1W', '1M', '3M', '6M', '12M'];
  const rows = result.markets.map((market) => {
    const summary = order
      .filter((timeframe) => market.timeframes?.[timeframe])
      .map((timeframe) => {
        const counts = market.timeframes[timeframe];
        return `${timeframe}: +${counts.missing}, corrected ${counts.corrected}`;
      })
      .join(' · ');
    return `<div><strong>${market.symbol}</strong> — ${summary || 'no completed bars in window'}</div>`;
  }).join('');
  target.innerHTML = `<div><strong>${result.days}-day repair complete</strong></div>${rows}`;
}

async function loadHistoricalRepairStatus() {
  try {
    renderHistoricalRepairStatus(await api.historicalRepairStatus());
  } catch (error) {
    renderHistoricalRepairStatus({ ok: false, error: (error as Error).message });
  }
}


function customIndicatorFromForm(form: HTMLFormElement): CustomIndicator {
  const data = new FormData(form);
  const numberValue = (key: string) => Number(data.get(key));
  return {
    id: String(data.get('id') ?? '').trim(),
    name: String(data.get('name') ?? '').trim(),
    category: 'QNext',
    kind: 'adaptive-ema-qalg',
    enabled: data.get('enabled') === 'on',
    description: String(data.get('description') ?? '').trim(),
    defaults: {
      priceSource: String(data.get('priceSource') ?? 'close'),
      emaLength: numberValue('emaLength'),
      lookbackPeriod: numberValue('lookbackPeriod'),
      stddevMultiplier: numberValue('stddevMultiplier'),
      atrLength: numberValue('atrLength'),
      atrMultiplier: numberValue('atrMultiplier'),
      upColor: String(data.get('upColor') ?? '#00ffaa'),
      downColor: String(data.get('downColor') ?? '#ff0000'),
      colorBars: data.get('colorBars') === 'on',
    },
    attribution: String(data.get('attribution') ?? '').trim(),
    license: String(data.get('license') ?? '').trim(),
    licenseUrl: String(data.get('licenseUrl') ?? '').trim(),
  };
}

function fillCustomIndicatorForm(indicator?: CustomIndicator) {
  const form = document.querySelector<HTMLFormElement>('#custom-indicator-form')!;
  const set = (name: string, value: string | number) => {
    const input = form.elements.namedItem(name) as HTMLInputElement | HTMLSelectElement | null;
    if (input) input.value = String(value);
  };
  const setChecked = (name: string, checked: boolean) => {
    const input = form.elements.namedItem(name) as HTMLInputElement | null;
    if (input) input.checked = checked;
  };
  const editing = form.elements.namedItem('editingId') as HTMLInputElement;

  editing.value = indicator?.id ?? '';
  set('name', indicator?.name ?? 'Adaptive EMA [QALG]');
  set('id', indicator?.id ?? 'adaptive-ema-qalg');
  set('kind', 'adaptive-ema-qalg');
  set('priceSource', indicator?.defaults.priceSource ?? 'close');
  set('emaLength', indicator?.defaults.emaLength ?? 20);
  set('lookbackPeriod', indicator?.defaults.lookbackPeriod ?? 30);
  set('stddevMultiplier', indicator?.defaults.stddevMultiplier ?? 2);
  set('atrLength', indicator?.defaults.atrLength ?? 14);
  set('atrMultiplier', indicator?.defaults.atrMultiplier ?? 1.5);
  set('upColor', indicator?.defaults.upColor ?? '#00ffaa');
  set('downColor', indicator?.defaults.downColor ?? '#ff0000');
  set('description', indicator?.description ?? 'Adaptive EMA trend overlay using EMA, standard deviation and ATR filters.');
  set('attribution', indicator?.attribution ?? 'QuantAlgo');
  set('license', indicator?.license ?? 'MPL-2.0');
  set('licenseUrl', indicator?.licenseUrl ?? 'https://mozilla.org/MPL/2.0/');
  setChecked('enabled', indicator?.enabled ?? true);
  setChecked('colorBars', indicator?.defaults.colorBars ?? true);

  const idField = form.elements.namedItem('id') as HTMLInputElement;
  idField.disabled = Boolean(indicator);
  document.querySelector<HTMLButtonElement>('#save-custom-indicator')!.textContent =
    indicator ? 'Update indicator' : 'Add indicator';
}

function renderCustomIndicators(indicators: CustomIndicator[], revision: number) {
  const list = document.querySelector<HTMLDivElement>('#custom-indicator-list')!;
  if (indicators.length === 0) {
    list.innerHTML = '<div class="muted">No custom indicators configured.</div>';
    return;
  }
  list.innerHTML = indicators.map((indicator) => `
    <div class="custom-indicator-row" data-indicator-id="${indicator.id}">
      <div>
        <div class="line custom-indicator-title">
          <strong>${indicator.name}</strong>
          ${badge(indicator.enabled, indicator.enabled ? 'ENABLED' : 'DISABLED')}
        </div>
        <div class="muted">${indicator.id} · ${indicator.kind} · rev ${revision}</div>
      </div>
      <div class="actions">
        <button type="button" class="secondary" data-indicator-action="edit" data-indicator-id="${indicator.id}">Edit</button>
        <button type="button" class="secondary" data-indicator-action="toggle" data-indicator-id="${indicator.id}">
          ${indicator.enabled ? 'Disable' : 'Enable'}
        </button>
        <button type="button" class="danger" data-indicator-action="delete" data-indicator-id="${indicator.id}">Delete</button>
      </div>
    </div>
  `).join('');

  list.querySelectorAll<HTMLButtonElement>('[data-indicator-action]').forEach((button) => {
    button.addEventListener('click', async () => {
      const id = button.dataset.indicatorId!;
      const indicator = indicators.find((item) => item.id === id);
      if (!indicator) return;
      const action = button.dataset.indicatorAction;

      if (action === 'edit') {
        fillCustomIndicatorForm(indicator);
        return;
      }

      try {
        if (action === 'toggle') {
          await api.updateCustomIndicator(id, { enabled: !indicator.enabled });
          toast(`${indicator.name} ${indicator.enabled ? 'disabled' : 'enabled'}`);
        } else if (action === 'delete') {
          if (!window.confirm(`Delete ${indicator.name}?`)) return;
          await api.deleteCustomIndicator(id);
          toast(`${indicator.name} deleted`);
          fillCustomIndicatorForm();
        }
        await loadCustomIndicators();
      } catch (error) {
        toast((error as Error).message, true);
      }
    });
  });
}

async function loadCustomIndicators() {
  try {
    const catalog = await api.customIndicators();
    renderCustomIndicators(catalog.indicators, catalog.revision);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

async function loadConfig() {
  try {
    const config = await api.getConfig();
    document.querySelector<HTMLTextAreaElement>('#config-editor')!.value =
      JSON.stringify(config, null, 2);
  } catch (error) {
    toast((error as Error).message, true);
  }
}

tokenButton.addEventListener('click', async () => {
  token = tokenInput.value.trim();
  if (!token) {
    toast('Enter an admin token.', true);
    return;
  }

  try {
    if (setupInitialized === false) {
      await api.initializeAdminToken(token);
      setupInitialized = true;
      tokenLabel.textContent = 'Staging admin token';
      tokenInput.placeholder = 'Enter token';
      tokenButton.textContent = 'Use token';
      toast('QNext admin token initialized');
    }

    sessionStorage.setItem('qnext-ops-token', token);
    api = new OpsAPI({ base: runtime.apiBase, token });
    await refresh();
    await loadConfig();
    await loadActiveMarkets();
    await loadCandleTimeframes();
    await loadChartTimeframes();
    await loadHistoricalRepairStatus();
    await loadCustomIndicators();
  } catch (error) {
    toast((error as Error).message, true);
  }
});


document.querySelector('#reload-custom-indicators')!.addEventListener('click', () => void loadCustomIndicators());
document.querySelector('#cancel-custom-indicator')!.addEventListener('click', () => fillCustomIndicatorForm());
document.querySelector<HTMLFormElement>('#custom-indicator-form')!.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget as HTMLFormElement;
  const editingId = (form.elements.namedItem('editingId') as HTMLInputElement).value;
  const indicator = customIndicatorFromForm(form);
  try {
    if (editingId) {
      await api.updateCustomIndicator(editingId, indicator);
      toast(`${indicator.name} updated`);
    } else {
      await api.createCustomIndicator(indicator);
      toast(`${indicator.name} added`);
    }
    fillCustomIndicatorForm();
    await loadCustomIndicators();
  } catch (error) {
    toast((error as Error).message, true);
  }
});

document.querySelector('#refresh')!.addEventListener('click', () => void refresh());
document.querySelector('#refresh-feed')!.addEventListener('click', () => void loadFeedStatus());
document.querySelector('#probe-stream')!.addEventListener('click', () => void probeBrowserStream());
document.querySelector('#verify-dhan')!.addEventListener('click', async () => {
  const resultEl = document.querySelector<HTMLSpanElement>('#dhan-standby-result')!;
  try {
    const result = await api.verifyDhanStandby();
    const age = typeof result.ageMs === 'number'
      ? (result.ageMs < 1000 ? '<1s' : `${Math.floor(result.ageMs / 1000)}s`)
      : 'n/a';
    resultEl.textContent = result.ok
      ? `PASS — Dhan fresh (${age}), received ${result.received ?? 0}, errors ${result.errors ?? 0}, authority ${result.authority ?? 'unknown'}.`
      : `WAIT — ${result.reason ?? 'Dhan standby not ready'}.`;
    toast(
      result.ok
        ? (result.safeForFailoverDrill ? 'Dhan standby PASS; safe for a controlled failover drill.' : 'Dhan standby is fresh.')
        : (result.reason ?? 'Dhan standby not ready'),
      !result.ok,
    );
    await loadFeedStatus();
  } catch (error) {
    resultEl.textContent = `ERROR — ${(error as Error).message}`;
    toast((error as Error).message, true);
  }
});
document.querySelector('#refresh-diagnostics')!.addEventListener('click', () => void loadDiagnostics());
document.querySelector('#copy-diagnostics')!.addEventListener('click', async () => {
  const log = document.querySelector<HTMLPreElement>('#diagnostics-log')!;
  const metadata = [
    `Log path: ${log.dataset.logPath ?? ''}`,
    `Binary path: ${log.dataset.binaryPath ?? ''}`,
    '',
  ].join('\n');
  try {
    await navigator.clipboard.writeText(metadata + log.textContent);
    toast('Runtime diagnostics copied');
  } catch {
    toast('Copy failed. Select the diagnostic text manually.', true);
  }
});
document.querySelector('#load-config')!.addEventListener('click', () => void loadConfig());
document.querySelector('#reload-active-markets')!.addEventListener('click', () => void loadActiveMarkets());
document.querySelector('#reload-candle-timeframes')!.addEventListener('click', () => void loadCandleTimeframes());
document.querySelector('#reload-chart-timeframes')!.addEventListener('click', () => void loadChartTimeframes());
document.querySelector('#candle-defaults')!.addEventListener('click', () => {
  const grid = document.querySelector<HTMLDivElement>('#candle-timeframes-grid')!;
  const defaults = new Set<string>(JSON.parse(grid.dataset.defaults ?? '[]'));
  defaults.add('1m');
  grid.querySelectorAll<HTMLInputElement>('input[name="candle-timeframe"]').forEach((input) => {
    input.checked = defaults.has(input.value) || input.dataset.protected === 'true';
    const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
    if (small) {
      small.textContent = input.dataset.protected === 'true'
        ? 'CORE / PROTECTED'
        : input.checked
          ? 'ENABLED / DEFAULT'
          : 'DISABLED';
    }
  });
});
document.querySelector('#save-candle-timeframes')!.addEventListener('click', async () => {
  try {
    const saved = await api.saveCandleTimeframes(selectedCandleTimeframes());
    const result = await api.service('restart');
    const queued = result.output.toLowerCase().includes('queued');
    toast(
      `Candle timeframes saved: ${saved.enabled.join(', ')}. Restart ${queued ? 'queued' : 'completed'}.`,
    );
    await loadCandleTimeframes();
    await loadChartTimeframes();
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});
document.querySelector('#chart-defaults')!.addEventListener('click', () => {
  const grid = document.querySelector<HTMLDivElement>('#chart-timeframes-grid')!;
  const defaults = new Set<string>(JSON.parse(grid.dataset.defaults ?? '[]'));
  const candleEnabled = new Set<string>(JSON.parse(grid.dataset.candleEnabled ?? '[]'));
  grid.querySelectorAll<HTMLInputElement>('input[name="chart-timeframe"]').forEach((input) => {
    input.checked = candleEnabled.has(input.value) && defaults.has(input.value);
    const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
    if (small) {
      small.textContent = input.dataset.candleOff === 'true'
        ? 'CANDLE OFF'
        : input.checked
          ? 'VISIBLE / DEFAULT'
          : 'HIDDEN';
    }
  });
});
document.querySelector('#save-chart-timeframes')!.addEventListener('click', async () => {
  const enabled = selectedChartTimeframes();
  if (enabled.length === 0) {
    toast('At least one chart display timeframe must remain enabled.', true);
    return;
  }
  try {
    const saved = await api.saveChartTimeframes(enabled);
    const result = await api.service('restart');
    const queued = result.output.toLowerCase().includes('queued');
    toast(
      `Chart display timeframes saved: ${saved.enabled.join(', ')}. Restart ${queued ? 'queued' : 'completed'}.`,
    );
    await loadChartTimeframes();
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});
document.querySelector('#refresh-history-repair')!.addEventListener('click', () => void loadHistoricalRepairStatus());
document.querySelectorAll<HTMLButtonElement>('[data-history-days]').forEach((button) => {
  button.addEventListener('click', async () => {
    const days = Number(button.dataset.historyDays) as 3 | 7 | 15 | 30;
    document.querySelectorAll<HTMLButtonElement>('[data-history-days]').forEach((item) => {
      item.disabled = true;
    });
    try {
      toast(`Running ${days}-day historical repair...`);
      const result = await api.runHistoricalRepair(days);
      renderHistoricalRepairResult(result);
      await loadHistoricalRepairStatus();
      toast(`${days}-day historical repair completed.`);
    } catch (error) {
      toast((error as Error).message, true);
      await loadHistoricalRepairStatus();
    } finally {
      document.querySelectorAll<HTMLButtonElement>('[data-history-days]').forEach((item) => {
        item.disabled = false;
      });
    }
  });
});
document.querySelector('#nifty-only')!.addEventListener('click', () => {
  document.querySelectorAll<HTMLInputElement>('input[name="active-market"]').forEach((input) => {
    input.checked = input.value === 'NIFTY';
    const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
    if (small) small.textContent = input.checked ? 'ACTIVE' : 'DISABLED';
  });
});
document.querySelector('#enable-all-markets')!.addEventListener('click', () => {
  document.querySelectorAll<HTMLInputElement>('input[name="active-market"]').forEach((input) => {
    input.checked = true;
    const small = input.closest<HTMLLabelElement>('.market-toggle')?.querySelector('small');
    if (small) small.textContent = 'ACTIVE';
  });
});
document.querySelector('#save-active-markets')!.addEventListener('click', async () => {
  const active = selectedActiveMarkets();
  if (active.length === 0) {
    toast('At least one market must remain active.', true);
    return;
  }
  try {
    const saved = await api.saveActiveMarkets(active);
    const result = await api.service('restart');
    const queued = result.output.toLowerCase().includes('queued');
    toast(
      `Active markets saved: ${saved.active.join(', ')}. Restart ${queued ? 'queued' : 'completed'}.`,
    );
    await loadActiveMarkets();
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});
document.querySelector('#copy-cron')!.addEventListener('click', async () => {
  const command = document.querySelector<HTMLElement>('#cron-command')!.textContent?.trim() ?? '';
  if (!command) {
    toast('Cron entry is not available yet.', true);
    return;
  }
  try {
    await navigator.clipboard.writeText(command);
    toast('Cron entry copied');
  } catch {
    toast('Copy failed. Select the cron entry manually.', true);
  }
});

document.querySelectorAll<HTMLButtonElement>('[data-service]').forEach((button) => {
  button.addEventListener('click', async () => {
    try {
      const action = button.dataset.service as ServiceAction;
      const result = await api.service(action);
      const queued = result.output.toLowerCase().includes('queued');
      toast(queued
        ? `Service ${action} queued; the cron supervisor will apply it on its next run.`
        : `Service ${action} completed`);
      await refresh();
    } catch (error) {
      toast((error as Error).message, true);
    }
  });
});

document.querySelector('#smoke')!.addEventListener('click', async () => {
  try {
    const result = await api.smoke();
    toast(result.ok ? 'Smoke test PASS' : 'Smoke test reported failures', !result.ok);
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});

document.querySelector('#activate')!.addEventListener('click', async () => {
  const version = document.querySelector<HTMLSelectElement>('#release-select')!.value;
  if (!version) {
    toast('No release selected', true);
    return;
  }
  try {
    await api.activateRelease(version);
    toast(`Activated release ${version}`);
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});

document.querySelector('#rollback')!.addEventListener('click', async () => {
  try {
    await api.rollback();
    toast('Rollback completed');
    await refresh();
  } catch (error) {
    toast((error as Error).message, true);
  }
});

document.querySelector('#save-config')!.addEventListener('click', async () => {
  try {
    const editor = document.querySelector<HTMLTextAreaElement>('#config-editor')!;
    const parsed = JSON.parse(editor.value) as Record<string, unknown>;
    await api.saveConfig(parsed);
    toast('Configuration saved. Restart Market Core to apply.');
  } catch (error) {
    toast((error as Error).message, true);
  }
});

const secretForm = document.querySelector<HTMLFormElement>('#secret-form')!;
secretForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(secretForm);
  const secrets: Record<string, string> = {};
  for (const [key, value] of form.entries()) {
    const stringValue = String(value).trim();
    if (stringValue) {
      secrets[key] = stringValue;
    }
  }
  if (Object.keys(secrets).length === 0) {
    toast('Enter at least one secret', true);
    return;
  }
  try {
    const result = await api.saveSecrets(secrets);
    const resilienceNote = result.resilienceConfigured
      ? ' Dhan NIFTY standby configuration created.'
      : '';
    toast(`Stored: ${result.stored.join(', ')}.${resilienceNote} Restart Market Core to apply.`);
    secretForm.reset();
  } catch (error) {
    toast((error as Error).message, true);
  }
});

async function bootstrapAdmin() {
  try {
    const setup = await api.setupStatus();
    setupInitialized = setup.initialized;

    if (!setup.initialized) {
      sessionStorage.removeItem('qnext-ops-token');
      token = '';
      tokenInput.value = '';
      tokenLabel.textContent = 'Create first-time admin token';
      tokenInput.placeholder = 'Choose token (minimum 16 characters)';
      tokenButton.textContent = 'Initialize';
      toast('First-time setup: create the QNext admin token.');
      return;
    }

    if (token) {
      await refresh();
      await loadConfig();
      await loadActiveMarkets();
      await loadHistoricalRepairStatus();
      await loadCustomIndicators();
      await probeBrowserStream();
    } else {
      toast('Enter the staging admin token to connect to QNext Ops.');
    }
  } catch (error) {
    toast((error as Error).message, true);
  }
}

void bootstrapAdmin();
