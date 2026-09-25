import { OpsAPI, type FeedStatusResponse, type OpsStatus, type RuntimeDiagnostics, type ServiceAction } from './api';
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
      await probeBrowserStream();
    } else {
      toast('Enter the staging admin token to connect to QNext Ops.');
    }
  } catch (error) {
    toast((error as Error).message, true);
  }
}

void bootstrapAdmin();
