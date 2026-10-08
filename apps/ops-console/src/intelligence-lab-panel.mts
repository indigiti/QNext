type LabOperation = {
  state?: 'IDLE' | 'QUEUED' | 'SUCCESS' | 'FAILED';
  action?: string;
  request_id?: string;
  completed_at_ms?: number;
  error?: string;
};

type LabEvaluation = {
  gate_passed?: boolean;
  metrics?: Record<string, number>;
  reasons?: string[];
};

type LabSelection = {
  selected_algorithm?: string;
  ml_family?: string;
  ml_gate_passed?: boolean;
  ml_error?: string;
  missing_indicators?: string[];
};

type LabExperiment = {
  experiment_id: string;
  name: string;
  instrument_id: string;
  timeframe: string;
  indicator_ids: string[];
  created_at_ms: number;
  lifecycle_state: string;
  backtest?: LabEvaluation | null;
  selection?: LabSelection | null;
  candidates?: Array<Record<string, unknown>>;
};

type LabMLRuntime = {
  state?: 'READY' | 'NEEDS_BOOTSTRAP' | 'UNAVAILABLE' | 'FAILED' | 'UNKNOWN';
  message?: string;
  python?: string;
  wheelhouse_fingerprint?: string;
  updated_at_ms?: number;
};

type LabStatus = {
  experiments: LabExperiment[];
  operation?: LabOperation;
  ml_runtime?: LabMLRuntime;
};

type QueueResponse = { queued: boolean; requestId: string; action: string };

const PENDING_KEY = 'qnext-intelligence-lab-pending-v1';

declare global {
  interface Window {
    __QNEXT_OPS_CONFIG__?: { apiBase?: string };
  }
}

const base = (window.__QNEXT_OPS_CONFIG__?.apiBase ?? '/qnext/admin/api/index.php').replace(/\/$/, '');

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body) headers.set('Content-Type', 'application/json');
  const separator = base.includes('?') ? '&' : '?';
  const response = await fetch(base + separator + 'route=' + encodeURIComponent(path), {
    ...init,
    headers,
    credentials: 'same-origin',
  });
  const text = await response.text();
  let payload: unknown = {};
  try { payload = text ? JSON.parse(text) : {}; } catch { throw new Error(`HTTP ${response.status}: invalid JSON`); }
  if (!response.ok) {
    const message = typeof payload === 'object' && payload !== null && 'error' in payload
      ? String((payload as { error?: unknown }).error ?? `HTTP ${response.status}`)
      : `HTTP ${response.status}`;
    throw new Error(message);
  }
  return payload as T;
}

const html = (value: unknown) => String(value ?? '')
  .replaceAll('&', '&amp;')
  .replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;')
  .replaceAll('"', '&quot;')
  .replaceAll("'", '&#039;');

function pct(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) ? `${(value * 100).toFixed(2)}%` : '—';
}

function num(value?: number, digits = 5): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(digits) : '—';
}

function pendingSnapshot(): any | null {
  const raw = localStorage.getItem(PENDING_KEY);
  if (!raw) return null;
  try {
    const value = JSON.parse(raw);
    return value && value.schema === 'QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1' ? value : null;
  } catch {
    return null;
  }
}

function operationLabel(operation?: LabOperation): string {
  const state = operation?.state ?? 'IDLE';
  if (state === 'QUEUED') return `${operation?.action ?? 'action'} queued`;
  if (state === 'FAILED') return `${operation?.action ?? 'action'} failed: ${operation?.error ?? 'unknown error'}`;
  if (state === 'SUCCESS') return `${operation?.action ?? 'action'} completed`;
  return 'Ready';
}

function card(experiment: LabExperiment, busy: boolean): string {
  const metrics = experiment.backtest?.metrics ?? {};
  const canBacktest = experiment.lifecycle_state === 'EXPERIMENT' && !busy;
  return `
    <article class="feed-provider-card qil-card" data-experiment="${html(experiment.experiment_id)}">
      <div class="line">
        <div>
          <strong>${html(experiment.name)}</strong>
          <div class="muted">${html(experiment.instrument_id)} · ${html(experiment.timeframe)}</div>
        </div>
        <span class="badge ${experiment.lifecycle_state === 'BACKTESTED' ? 'good' : 'bad'}">${html(experiment.lifecycle_state)}</span>
      </div>
      <div class="muted qil-indicators">${experiment.indicator_ids.map(html).join(' · ')}</div>
      ${experiment.selection ? `
        <div class="line qil-selection">
          <span>Selected model</span>
          <strong>${html(experiment.selection.selected_algorithm ?? 'ridge')}</strong>
        </div>
        ${experiment.selection.ml_family
          ? `<div class="muted">ML challenger: ${html(experiment.selection.ml_family)} · ${experiment.selection.ml_gate_passed ? 'PASS' : 'NOT SELECTED'}</div>`
          : ''}
        ${experiment.selection.ml_error
          ? `<div class="muted qil-warning">ML fallback: ${html(experiment.selection.ml_error)}</div>`
          : ''}
      ` : ''}
      ${experiment.backtest ? `
        <div class="qil-metrics">
          <div><span>Accuracy</span><strong>${pct(metrics.accuracy)}</strong></div>
          <div><span>Coverage</span><strong>${pct(metrics.coverage)}</strong></div>
          <div><span>Avg return</span><strong>${num(metrics.average_strategy_return)}</strong></div>
          <div><span>Max DD</span><strong>${num(metrics.max_drawdown)}</strong></div>
          <div><span>Examples</span><strong>${metrics.historical_examples ?? '—'}</strong></div>
          <div><span>Features</span><strong>${metrics.feature_count ?? '—'}</strong></div>
          <div><span>Indicator coverage</span><strong>${pct(metrics.indicator_coverage_ratio)}</strong></div>
        </div>
        ${experiment.backtest?.reasons?.length
          ? `<div class="muted qil-warning">${experiment.backtest.reasons.map(html).join(' · ')}</div>`
          : ''}
      ` : '<p class="muted">No Lab backtest yet.</p>'}
      <div class="actions">
        <button class="qil-backtest" type="button" ${canBacktest ? '' : 'disabled'}>Run backtest</button>
      </div>
    </article>
  `;
}

function render(status: LabStatus): void {
  const state = document.querySelector<HTMLSpanElement>('#qil-state')!;
  const pending = document.querySelector<HTMLDivElement>('#qil-pending')!;
  const experiments = document.querySelector<HTMLDivElement>('#qil-experiments')!;
  const snapshot = pendingSnapshot();
  const busy = status.operation?.state === 'QUEUED';

  const runtimeState = status.ml_runtime?.state ?? 'UNKNOWN';
  const runtimeMessage = status.ml_runtime?.message ?? '';
  state.textContent = `${operationLabel(status.operation)} · ML ${runtimeState}${runtimeMessage ? ` · ${runtimeMessage}` : ''}`;
  pending.innerHTML = snapshot ? `
    <div class="line">
      <div>
        <strong>Pending chart snapshot</strong>
        <div class="muted">${html(snapshot.instrument_id)} · ${html(snapshot.timeframe)} · ${snapshot.indicators?.length ?? 0} enabled indicators · ${snapshot.feature_rows?.length ?? 0} feature rows</div>
      </div>
      <button id="qil-import" type="button" ${busy ? 'disabled' : ''}>Import to Lab</button>
    </div>
  ` : '<p class="muted">Use “Send to Lab” on the Vela chart to capture the currently enabled indicators.</p>';

  const ordered = [...(status.experiments ?? [])].sort((a, b) => b.created_at_ms - a.created_at_ms);
  experiments.innerHTML = ordered.length
    ? ordered.map((item) => card(item, busy)).join('')
    : '<p class="muted">No Lab experiments yet.</p>';

  document.querySelector<HTMLButtonElement>('#qil-import')?.addEventListener('click', () => void importPending());
  experiments.querySelectorAll<HTMLButtonElement>('.qil-backtest').forEach((button) => {
    button.addEventListener('click', () => {
      const id = button.closest<HTMLElement>('.qil-card')?.dataset.experiment ?? '';
      if (id) void backtest(id);
    });
  });
}

async function load(): Promise<void> {
  const state = document.querySelector<HTMLSpanElement>('#qil-state')!;
  try {
    render(await request<LabStatus>('/intelligence-lab'));
  } catch (error) {
    state.textContent = `Unavailable: ${(error as Error).message}`;
  }
}

async function importPending(): Promise<void> {
  const snapshot = pendingSnapshot();
  const state = document.querySelector<HTMLSpanElement>('#qil-state')!;
  if (!snapshot) {
    state.textContent = 'No pending chart snapshot.';
    return;
  }
  try {
    const result = await request<QueueResponse>('/intelligence-lab/import', {
      method: 'POST',
      body: JSON.stringify({ snapshot }),
    });
    localStorage.removeItem(PENDING_KEY);
    state.textContent = `Import queued (${result.requestId})`;
    await load();
  } catch (error) {
    state.textContent = `Import failed: ${(error as Error).message}`;
  }
}

async function backtest(experimentId: string): Promise<void> {
  const horizon = Number(document.querySelector<HTMLInputElement>('#qil-horizon')?.value ?? 3);
  const state = document.querySelector<HTMLSpanElement>('#qil-state')!;
  try {
    const result = await request<QueueResponse>('/intelligence-lab/backtest', {
      method: 'POST',
      body: JSON.stringify({
        experimentId,
        horizonBars: Number.isInteger(horizon) ? horizon : 3,
        minSamples: 60,
        minTestSamples: 12,
      }),
    });
    state.textContent = `Backtest queued (${result.requestId})`;
    await load();
  } catch (error) {
    state.textContent = `Backtest failed: ${(error as Error).message}`;
  }
}

function mount(): void {
  const grid = document.querySelector<HTMLElement>('main.grid');
  if (!grid || document.querySelector('#intelligence-lab-card')) return;

  const section = document.createElement('section');
  section.className = 'card span-3';
  section.id = 'intelligence-lab-card';
  section.innerHTML = `
    <div class="card-head">
      <div>
        <p class="eyebrow">Intelligence Lab</p>
        <h2>Chart indicator experiments</h2>
        <p class="muted">Isolated from production. Import enabled chart indicators, backtest on canonical finalized bars, then advance to shadow only after certification gates pass.</p>
      </div>
      <div class="actions">
        <label class="qil-horizon-label">Horizon <input id="qil-horizon" type="number" min="1" max="100" value="3" /></label>
        <button id="qil-refresh" class="secondary" type="button">Refresh</button>
      </div>
    </div>
    <div class="line"><span class="muted">Runtime</span><span id="qil-state" class="muted">Waiting for admin authentication…</span></div>
    <div id="qil-pending" class="qil-pending"></div>
    <h3>Experiments</h3>
    <div id="qil-experiments" class="feed-provider-grid"></div>
  `;

  const quant = grid.querySelector<HTMLElement>('#quant-intelligence-card');
  if (quant?.nextSibling) grid.insertBefore(section, quant.nextSibling);
  else grid.append(section);

  const style = document.createElement('style');
  style.textContent = `
    #intelligence-lab-card .qil-pending{margin:1rem 0;padding:.8rem;border:1px solid rgba(255,255,255,.08);border-radius:.6rem}
    #intelligence-lab-card .qil-indicators{margin:.6rem 0;word-break:break-word}
    #intelligence-lab-card .qil-selection{margin:.45rem 0;padding-top:.45rem;border-top:1px solid rgba(255,255,255,.06)}
    #intelligence-lab-card .qil-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:.5rem;margin:.75rem 0}
    #intelligence-lab-card .qil-metrics div{display:flex;flex-direction:column;gap:.2rem}
    #intelligence-lab-card .qil-horizon-label{display:flex;align-items:center;gap:.4rem;color:var(--muted,#8b98a5)}
    #intelligence-lab-card .qil-warning{margin:.6rem 0;padding:.55rem .65rem;border:1px solid rgba(245,158,11,.25);border-radius:.45rem}
    #intelligence-lab-card #qil-horizon{width:72px}
    @media(max-width:900px){#intelligence-lab-card .qil-metrics{grid-template-columns:1fr 1fr}}
  `;
  document.head.append(style);

  document.querySelector('#qil-refresh')!.addEventListener('click', () => void load());
  void load();
  window.setInterval(() => void load(), 5000);
}

mount();
