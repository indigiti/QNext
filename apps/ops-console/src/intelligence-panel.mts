type MetricSet = {
  samples?: number;
  accuracy?: number;
  coverage?: number;
  strategy_return?: number;
  average_strategy_return?: number;
  max_drawdown?: number;
  trades?: number;
};

type Candidate = {
  candidate_id: string;
  model_name: string;
  model_version: string;
  lifecycle_state: string;
  algorithm: string;
  dataset_hash: string;
  model_hash: string;
  created_at_ms: number;
  ridge: number;
  flat_threshold: number;
  train_samples: number;
  validation_samples: number;
  test_samples: number;
  validation_metrics: MetricSet;
  test_metrics: MetricSet;
  champion_test_metrics: MetricSet;
  promotion_gate: { passed: boolean; reasons?: string[] };
};

type ProductionPointer = {
  candidate_id: string;
  model_name: string;
  model_version: string;
  model_hash: string;
  dataset_hash: string;
  promoted_at_ms: number;
  approved_by: string;
  previous_candidate_id?: string;
};

type Operation = {
  state?: 'IDLE' | 'QUEUED' | 'SUCCESS' | 'FAILED';
  request_id?: string | null;
  action?: string | null;
  requested_at_ms?: number;
  completed_at_ms?: number;
  error?: string;
};

type IntelligenceStatus = {
  production: ProductionPointer | null;
  candidates: Candidate[];
  history?: Array<Record<string, unknown>>;
  data?: { features?: number; predictions?: number; outcomes?: number };
  operation?: Operation;
};

type QueueResponse = { queued: boolean; requestId: string; action: string };

declare global {
  interface Window {
    __QNEXT_OPS_CONFIG__?: { apiBase?: string };
  }
}

const base = (window.__QNEXT_OPS_CONFIG__?.apiBase ?? '/qnext/admin/api/index.php').replace(/\/$/, '');
const html = (value: unknown) => String(value ?? '')
  .replaceAll('&', '&amp;')
  .replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;')
  .replaceAll('"', '&quot;')
  .replaceAll("'", '&#039;');

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

function pct(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) ? `${(value * 100).toFixed(2)}%` : '—';
}

function num(value?: number, digits = 5): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(digits) : '—';
}

function date(ms?: number): string {
  return ms && ms > 0 ? new Date(ms).toLocaleString() : '—';
}

function badge(ok: boolean, label: string): string {
  return `<span class="badge ${ok ? 'good' : 'bad'}">${html(label)}</span>`;
}

function metricRows(candidate: Candidate): string {
  const current = candidate.test_metrics ?? {};
  const champion = candidate.champion_test_metrics ?? {};
  return `
    <div class="qi-metric"><span>Test samples</span><strong>${current.samples ?? candidate.test_samples ?? 0}</strong></div>
    <div class="qi-metric"><span>Accuracy</span><strong>${pct(current.accuracy)} <small>vs ${pct(champion.accuracy)}</small></strong></div>
    <div class="qi-metric"><span>Coverage</span><strong>${pct(current.coverage)} <small>vs ${pct(champion.coverage)}</small></strong></div>
    <div class="qi-metric"><span>Strategy return</span><strong>${num(current.strategy_return)} <small>vs ${num(champion.strategy_return)}</small></strong></div>
    <div class="qi-metric"><span>Avg return</span><strong>${num(current.average_strategy_return)} <small>vs ${num(champion.average_strategy_return)}</small></strong></div>
    <div class="qi-metric"><span>Max drawdown</span><strong>${num(current.max_drawdown)} <small>vs ${num(champion.max_drawdown)}</small></strong></div>
  `;
}

function candidateCard(candidate: Candidate, productionId: string | null): string {
  const passed = Boolean(candidate.promotion_gate?.passed);
  const isProduction = candidate.candidate_id === productionId;
  const reasons = candidate.promotion_gate?.reasons ?? [];
  return `
    <article class="feed-provider-card qi-candidate" data-candidate="${html(candidate.candidate_id)}">
      <div class="line">
        <div><strong>${html(candidate.model_name)} v${html(candidate.model_version)}</strong><div class="muted">${html(candidate.candidate_id)}</div></div>
        <div>${isProduction ? badge(true, 'PRODUCTION') : badge(passed, passed ? 'GATE PASS' : 'GATE FAIL')}</div>
      </div>
      <div class="qi-grid">${metricRows(candidate)}</div>
      <div class="actions">
        <button class="secondary qi-diff" type="button">View Diff</button>
        <button class="qi-promote" type="button" ${passed && !isProduction ? '' : 'disabled'}>Promote</button>
      </div>
      <div class="qi-diff-body" hidden>
        <div class="status-grid">
          <div><span>Algorithm</span><strong>${html(candidate.algorithm)}</strong></div>
          <div><span>Ridge</span><strong>${html(candidate.ridge)}</strong></div>
          <div><span>Flat threshold</span><strong>${html(candidate.flat_threshold)}</strong></div>
          <div><span>Train / val / test</span><strong>${candidate.train_samples} / ${candidate.validation_samples} / ${candidate.test_samples}</strong></div>
          <div><span>Created</span><strong>${html(date(candidate.created_at_ms))}</strong></div>
          <div><span>Model hash</span><strong class="qi-hash">${html(candidate.model_hash)}</strong></div>
          <div><span>Dataset hash</span><strong class="qi-hash">${html(candidate.dataset_hash)}</strong></div>
        </div>
        ${reasons.length ? `<p class="muted">Gate reasons: ${reasons.map(html).join(' · ')}</p>` : '<p class="muted">All promotion gates passed.</p>'}
      </div>
    </article>
  `;
}

function operationLabel(operation?: Operation): string {
  const state = operation?.state ?? 'IDLE';
  if (state === 'QUEUED') return `${operation?.action ?? 'action'} queued for supervisor`;
  if (state === 'FAILED') return `${operation?.action ?? 'action'} failed: ${operation?.error ?? 'unknown error'}`;
  if (state === 'SUCCESS') return `${operation?.action ?? 'action'} completed ${date(operation?.completed_at_ms)}`;
  return 'Ready';
}

function render(status: IntelligenceStatus): void {
  const production = document.querySelector<HTMLDivElement>('#qi-production')!;
  const candidates = document.querySelector<HTMLDivElement>('#qi-candidates')!;
  const history = document.querySelector<HTMLDivElement>('#qi-history')!;
  const rollbackSelect = document.querySelector<HTMLSelectElement>('#qi-rollback-target')!;
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  const pointer = status.production;
  const data = status.data ?? {};

  production.innerHTML = pointer ? `
    <div><span>Production</span>${badge(true, 'ACTIVE')}<strong>${html(pointer.candidate_id)}</strong></div>
    <div><span>Model</span><strong>${html(pointer.model_name)} v${html(pointer.model_version)}</strong></div>
    <div><span>Promoted</span><strong>${html(date(pointer.promoted_at_ms))}</strong></div>
    <div><span>Approved by</span><strong>${html(pointer.approved_by)}</strong></div>
    <div><span>Features</span><strong>${data.features ?? 0}</strong></div>
    <div><span>Predictions</span><strong>${data.predictions ?? 0}</strong></div>
    <div><span>Outcomes</span><strong>${data.outcomes ?? 0}</strong></div>
  ` : `
    <div><span>Production</span>${badge(false, 'NONE')}</div>
    <div><span>State</span><strong>DRAFT / research only</strong></div>
    <div><span>Features</span><strong>${data.features ?? 0}</strong></div>
    <div><span>Predictions</span><strong>${data.predictions ?? 0}</strong></div>
    <div><span>Outcomes</span><strong>${data.outcomes ?? 0}</strong></div>
  `;

  state.textContent = operationLabel(status.operation);
  const busy = status.operation?.state === 'QUEUED';
  document.querySelector<HTMLButtonElement>('#qi-train')!.disabled = busy;
  document.querySelector<HTMLButtonElement>('#qi-rollback')!.disabled = busy;

  const ordered = [...(status.candidates ?? [])].sort((a, b) => (b.created_at_ms ?? 0) - (a.created_at_ms ?? 0));
  candidates.innerHTML = ordered.length
    ? ordered.map((item) => candidateCard(item, pointer?.candidate_id ?? null)).join('')
    : '<p class="muted">No trained candidates yet. QNext needs complete feature + prediction + realized outcome records before training.</p>';

  rollbackSelect.innerHTML = '<option value="">Previous production candidate</option>' + ordered
    .filter((item) => item.promotion_gate?.passed && item.candidate_id !== pointer?.candidate_id)
    .map((item) => `<option value="${html(item.candidate_id)}">${html(item.model_name)} v${html(item.model_version)} — ${html(item.candidate_id)}</option>`)
    .join('');

  const events = [...(status.history ?? [])].reverse();
  history.innerHTML = events.length ? events.map((event) => `
    <div class="line qi-history-row">
      <span>${html(event.action)} → ${html(event.candidate_id)}</span>
      <strong>${html(date(Number(event.event_at_ms ?? 0)))}</strong>
      <small>${html(event.approved_by)}${event.note ? ` · ${html(event.note)}` : ''}</small>
    </div>
  `).join('') : '<p class="muted">No promotion or rollback events yet.</p>';

  candidates.querySelectorAll<HTMLButtonElement>('.qi-diff').forEach((button) => {
    button.addEventListener('click', () => {
      const card = button.closest<HTMLElement>('.qi-candidate')!;
      const body = card.querySelector<HTMLElement>('.qi-diff-body')!;
      body.hidden = !body.hidden;
      button.textContent = body.hidden ? 'View Diff' : 'Hide Diff';
    });
  });
  candidates.querySelectorAll<HTMLButtonElement>('.qi-promote').forEach((button) => {
    if (busy) button.disabled = true;
    button.addEventListener('click', () => void promote(button.closest<HTMLElement>('.qi-candidate')!.dataset.candidate ?? '', ordered));
  });
}

async function load(): Promise<void> {
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  try {
    render(await request<IntelligenceStatus>('/intelligence'));
  } catch (error) {
    state.textContent = `Unavailable: ${(error as Error).message}`;
  }
}

async function queueAction(path: string, body: Record<string, unknown>, verb: string): Promise<void> {
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  try {
    const queued = await request<QueueResponse>(path, { method: 'POST', body: JSON.stringify(body) });
    state.textContent = `${verb} queued (${queued.requestId}). Supervisor will apply it on its next run.`;
    await load();
  } catch (error) {
    state.textContent = `${verb} failed: ${(error as Error).message}`;
  }
}

async function train(): Promise<void> {
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  state.textContent = 'Queueing Analyze → Optimize → Holdout Backtest…';
  await queueAction('/intelligence/train', { minSamples: 60, minTestSamples: 12 }, 'Training');
}

async function promote(candidateId: string, candidates: Candidate[]): Promise<void> {
  const candidate = candidates.find((item) => item.candidate_id === candidateId);
  const approvedBy = document.querySelector<HTMLInputElement>('#qi-approved-by')!.value.trim();
  const note = document.querySelector<HTMLInputElement>('#qi-note')!.value.trim();
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  if (!candidate || !approvedBy) {
    state.textContent = 'Promotion requires a selected candidate and approver name.';
    return;
  }
  if (!candidate.promotion_gate?.passed) {
    state.textContent = 'Candidate cannot be promoted because its certification gates failed.';
    return;
  }
  await queueAction('/intelligence/promote', {
    candidateId,
    expectedModelHash: candidate.model_hash,
    approvedBy,
    note,
  }, 'Promotion');
}

async function rollback(): Promise<void> {
  const approvedBy = document.querySelector<HTMLInputElement>('#qi-approved-by')!.value.trim();
  const note = document.querySelector<HTMLInputElement>('#qi-note')!.value.trim();
  const candidateId = document.querySelector<HTMLSelectElement>('#qi-rollback-target')!.value;
  const state = document.querySelector<HTMLSpanElement>('#qi-state')!;
  if (!approvedBy) {
    state.textContent = 'Rollback requires an approver name.';
    return;
  }
  await queueAction('/intelligence/rollback', { candidateId: candidateId || null, approvedBy, note }, 'Rollback');
}

function mount(): void {
  const grid = document.querySelector<HTMLElement>('main.grid');
  if (!grid || document.querySelector('#quant-intelligence-card')) return;
  const section = document.createElement('section');
  section.className = 'card span-3';
  section.id = 'quant-intelligence-card';
  section.innerHTML = `
    <div class="card-head">
      <div>
        <p class="eyebrow">Quant Intelligence</p>
        <h2>Controlled self-learning</h2>
        <p class="muted">Analyze → Optimize → Holdout Backtest → Compare → DRAFT → explicit Promote. QNext never self-promotes.</p>
      </div>
      <div class="actions">
        <button id="qi-train" type="button">Train candidate</button>
        <button id="qi-refresh" type="button" class="secondary">Refresh</button>
      </div>
    </div>
    <div id="qi-production" class="status-grid"></div>
    <div class="line"><span class="muted">Runtime</span><span id="qi-state" class="muted">Waiting for admin authentication…</span></div>
    <div class="qi-approval">
      <input id="qi-approved-by" placeholder="Approver name / ID" autocomplete="off" />
      <input id="qi-note" placeholder="Approval / rollback note (optional)" autocomplete="off" />
      <select id="qi-rollback-target"><option value="">Previous production candidate</option></select>
      <button id="qi-rollback" class="secondary" type="button">Rollback</button>
    </div>
    <h3>Candidates</h3>
    <div id="qi-candidates" class="feed-provider-grid"></div>
    <h3>Promotion history</h3>
    <div id="qi-history" class="stack"></div>
  `;

  const insertBefore = grid.querySelector<HTMLElement>('#custom-indicators-card')?.nextElementSibling;
  if (insertBefore) grid.insertBefore(section, insertBefore); else grid.append(section);

  const style = document.createElement('style');
  style.textContent = `
    #quant-intelligence-card .qi-approval{display:grid;grid-template-columns:1fr 2fr 1.2fr auto;gap:.65rem;margin:1rem 0;align-items:center}
    #quant-intelligence-card .qi-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:.5rem;margin:.75rem 0}
    #quant-intelligence-card .qi-metric{display:flex;flex-direction:column;gap:.2rem}.qi-metric small{color:var(--muted,#8b98a5);font-weight:400}
    #quant-intelligence-card .qi-hash{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.72rem;word-break:break-all}
    #quant-intelligence-card .qi-diff-body{margin-top:.75rem;padding-top:.75rem;border-top:1px solid rgba(255,255,255,.08)}
    #quant-intelligence-card .qi-history-row{display:grid;grid-template-columns:1fr auto;gap:.2rem .8rem}.qi-history-row small{grid-column:1/-1;color:var(--muted,#8b98a5)}
    @media(max-width:900px){#quant-intelligence-card .qi-approval,#quant-intelligence-card .qi-grid{grid-template-columns:1fr}}
  `;
  document.head.append(style);

  document.querySelector('#qi-refresh')!.addEventListener('click', () => void load());
  document.querySelector('#qi-train')!.addEventListener('click', () => void train());
  document.querySelector('#qi-rollback')!.addEventListener('click', () => void rollback());
  void load();
  window.setInterval(() => void load(), 5000);
}

mount();
