import { OpsAPI, type OpsStatus, type RuntimeDiagnostics, type ServiceAction } from './api';
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
    ]);
    renderStatus(status);
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
  } catch (error) {
    toast((error as Error).message, true);
  }
});

document.querySelector('#refresh')!.addEventListener('click', () => void refresh());
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
    toast(`Stored: ${result.stored.join(', ')}. Restart Market Core to apply.`);
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
    } else {
      toast('Enter the staging admin token to connect to QNext Ops.');
    }
  } catch (error) {
    toast((error as Error).message, true);
  }
}

void bootstrapAdmin();
