import { OpsAPI } from './api';

type ChartEngine = 'auto' | 'vela' | 'lightweight';
type LightweightLayout = 1 | 2 | 4;

interface WorkspaceSettings {
  chart_engine: ChartEngine;
  lightweight_layout: LightweightLayout;
  lightweight_sync: boolean;
}

const defaults: WorkspaceSettings = {
  chart_engine: 'vela',
  lightweight_layout: 1,
  lightweight_sync: true,
};

function opsAPI(): OpsAPI {
  const runtime = (window as Window & {
    __QNEXT_OPS_CONFIG__?: { apiBase?: string };
  }).__QNEXT_OPS_CONFIG__ ?? {};
  return new OpsAPI({
    base: runtime.apiBase,
    token: sessionStorage.getItem('qnext-ops-token') ?? '',
  });
}

function normalizeSettings(config: Record<string, unknown>): WorkspaceSettings {
  const raw =
    typeof config.workspace === 'object' && config.workspace !== null
      ? config.workspace as Record<string, unknown>
      : {};

  const engine = raw.chart_engine;
  const chart_engine: ChartEngine =
    engine === 'auto' || engine === 'lightweight' || engine === 'vela'
      ? engine
      : defaults.chart_engine;

  const layout = raw.lightweight_layout;
  const lightweight_layout: LightweightLayout =
    layout === 2 || layout === 4 ? layout : 1;

  const lightweight_sync =
    typeof raw.lightweight_sync === 'boolean'
      ? raw.lightweight_sync
      : defaults.lightweight_sync;

  return { chart_engine, lightweight_layout, lightweight_sync };
}

function setNote(message: string, error = false): void {
  const note = document.querySelector<HTMLSpanElement>('#workspace-settings-note');
  if (!note) return;
  note.textContent = message;
  note.classList.toggle('bad-text', error);
}

async function loadSettings(): Promise<void> {
  const engine = document.querySelector<HTMLSelectElement>('#workspace-chart-engine');
  const layout = document.querySelector<HTMLSelectElement>('#workspace-lightweight-layout');
  const sync = document.querySelector<HTMLInputElement>('#workspace-lightweight-sync');
  if (!engine || !layout || !sync) return;

  try {
    setNote('Loading…');
    const config = await opsAPI().getConfig();
    const settings = normalizeSettings(config);
    engine.value = settings.chart_engine;
    layout.value = String(settings.lightweight_layout);
    sync.checked = settings.lightweight_sync;
    setNote('Settings loaded. Changes apply to new chart sessions; Market Core restart is not required.');
  } catch (error) {
    setNote(`Unable to load: ${(error as Error).message}`, true);
  }
}

async function saveSettings(): Promise<void> {
  const engine = document.querySelector<HTMLSelectElement>('#workspace-chart-engine');
  const layout = document.querySelector<HTMLSelectElement>('#workspace-lightweight-layout');
  const sync = document.querySelector<HTMLInputElement>('#workspace-lightweight-sync');
  const save = document.querySelector<HTMLButtonElement>('#save-workspace-settings');
  if (!engine || !layout || !sync || !save) return;

  const chartEngine: ChartEngine =
    engine.value === 'auto' || engine.value === 'lightweight'
      ? engine.value
      : 'vela';
  const layoutValue = Number(layout.value);
  const lightweightLayout: LightweightLayout =
    layoutValue === 2 || layoutValue === 4 ? layoutValue : 1;

  save.disabled = true;
  try {
    setNote('Saving…');
    const api = opsAPI();
    const config = await api.getConfig();
    config.workspace = {
      chart_engine: chartEngine,
      lightweight_layout: lightweightLayout,
      lightweight_sync: sync.checked,
    };
    await api.saveConfig(config);
    setNote('Saved. Reload/open QNext charts to use the new defaults.');
  } catch (error) {
    setNote(`Save failed: ${(error as Error).message}`, true);
  } finally {
    save.disabled = false;
  }
}

function mount(): boolean {
  if (document.querySelector('#workspace-settings-card')) return true;
  const grid = document.querySelector<HTMLElement>('main.grid');
  if (!grid) return false;

  const card = document.createElement('section');
  card.id = 'workspace-settings-card';
  card.className = 'card span-3';
  card.innerHTML = `
    <div class="card-head">
      <div>
        <p class="eyebrow">Workspace</p>
        <h2>Chart engine</h2>
        <p class="muted">Choose the default renderer. URL/runtime overrides remain available for diagnostics and parity testing.</p>
      </div>
      <button id="reload-workspace-settings" type="button" class="secondary">Reload</button>
    </div>
    <div class="status-grid">
      <label>
        <span>Default engine</span>
        <select id="workspace-chart-engine">
          <option value="vela">QNext Advanced / Vela</option>
          <option value="lightweight">QNext Lightweight</option>
          <option value="auto">Auto (safe policy)</option>
        </select>
      </label>
      <label>
        <span>Lightweight layout</span>
        <select id="workspace-lightweight-layout">
          <option value="1">1 chart</option>
          <option value="2">2 charts</option>
          <option value="4">4 charts</option>
        </select>
      </label>
      <label class="checkbox-field">
        <span>Crosshair / range sync</span>
        <input id="workspace-lightweight-sync" type="checkbox" checked />
      </label>
    </div>
    <div class="actions">
      <button id="save-workspace-settings" type="button">Save chart settings</button>
      <span id="workspace-settings-note" class="muted">Authenticate to load settings.</span>
    </div>
  `;

  const chartTimeframes = document.querySelector('#chart-timeframes-card');
  if (chartTimeframes?.parentElement === grid) {
    chartTimeframes.insertAdjacentElement('afterend', card);
  } else {
    grid.append(card);
  }

  card.querySelector('#reload-workspace-settings')?.addEventListener('click', () => {
    void loadSettings();
  });
  card.querySelector('#save-workspace-settings')?.addEventListener('click', () => {
    void saveSettings();
  });

  void loadSettings();
  return true;
}

let attempts = 0;
function mountWhenReady(): void {
  if (mount()) return;
  attempts += 1;
  if (attempts < 40) {
    window.setTimeout(mountWhenReady, 50);
  }
}

mountWhenReady();
