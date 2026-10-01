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

const INDIA_TZ = 'Asia/Kolkata';
const HEALTH_REFRESH_MS = 15_000;

function qnextRoot(): URL {
  return new URL('../', window.location.href);
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
  if (state === 'CARRY_FORWARD') return 'CARRY';
  return state;
}

function stateClass(state: HealthState): string {
  return state.toLowerCase().replace('_', '-');
}

function statusBadge(status: string): string {
  const ok = status === 'HEALTHY' || status === 'WAITING' || status === 'CLOSED';
  return `<span class="badge ${ok ? 'good' : 'bad'}">${escapeHTML(status)}</span>`;
}

function ensurePanel(): HTMLDivElement | null {
  const card = document.querySelector<HTMLElement>('#feed-status-card');
  if (!card) return null;

  let body = card.querySelector<HTMLDivElement>('#five-second-day-health-body');
  if (body) return body;

  const section = document.createElement('section');
  section.id = 'five-second-day-health';
  section.innerHTML = `
    <div class="line five-second-health-head">
      <div>
        <strong>Current-day 5s candle health</strong>
        <div class="muted">Persisted canonical 5s history · full regular-session timeline · auto-refresh 15s</div>
      </div>
      <span id="five-second-day-health-state" class="badge">CHECKING</span>
    </div>
    <div class="five-second-health-legend" aria-label="5 second health legend">
      <span><i class="five-second-swatch good"></i>GOOD</span>
      <span><i class="five-second-swatch carry-forward"></i>CARRY</span>
      <span><i class="five-second-swatch recovered"></i>RECOVERED</span>
      <span><i class="five-second-swatch degraded"></i>DEGRADED</span>
      <span><i class="five-second-swatch missing"></i>MISSING</span>
      <span><i class="five-second-swatch future"></i>FUTURE</span>
    </div>
    <div id="five-second-day-health-body" class="five-second-health-list"></div>
  `;

  const canonical = card.querySelector('#canonical-5s-section');
  const actions = card.querySelector('.feed-actions');
  if (canonical) {
    canonical.insertAdjacentElement('afterend', section);
  } else if (actions) {
    card.insertBefore(section, actions);
  } else {
    card.append(section);
  }
  return section.querySelector<HTMLDivElement>('#five-second-day-health-body');
}

function timelineSegments(symbol: SymbolHealth): string {
  const start = symbol.session_start_ms ?? 0;
  const end = symbol.session_end_ms ?? 0;
  const total = end - start;
  if (total <= 0) {
    return '<div class="five-second-timeline-empty">No regular session today</div>';
  }

  return symbol.timeline.map((segment) => {
    const left = Math.max(0, Math.min(100, ((segment.start_ms - start) / total) * 100));
    const width = Math.max(0, Math.min(100 - left, ((segment.end_ms - segment.start_ms) / total) * 100));
    const abnormal = segment.state === 'MISSING' || segment.state === 'DEGRADED';
    const title = `${symbol.symbol} · ${stateLabel(segment.state)} · ${timeLabel(segment.start_ms)}–${timeLabel(segment.end_ms)} · ${durationLabel(segment.end_ms - segment.start_ms)} · ${segment.buckets} bucket${segment.buckets === 1 ? '' : 's'}${segment.reason ? ` · ${segment.reason}` : ''}`;
    return `<span
      class="five-second-timeline-segment ${stateClass(segment.state)}${abnormal ? ' abnormal' : ''}"
      style="left:${left.toFixed(5)}%;width:${width.toFixed(5)}%"
      title="${escapeHTML(title)}"
      aria-label="${escapeHTML(title)}"
    ></span>`;
  }).join('');
}

function breakRows(symbol: SymbolHealth): string {
  const events = symbol.timeline.filter((segment) => segment.state !== 'GOOD');
  if (events.length === 0) {
    return '<div class="muted five-second-no-breaks">No carry, recovered, degraded, or missing 5s intervals in the elapsed session.</div>';
  }
  return events.map((segment) => {
    const severity = segment.state === 'MISSING' || segment.state === 'DEGRADED' ? 'break' : 'event';
    return `
      <div class="five-second-break-row ${severity}">
        <span class="five-second-event-state ${stateClass(segment.state)}">${escapeHTML(stateLabel(segment.state))}</span>
        <strong>${timeLabel(segment.start_ms)} – ${timeLabel(segment.end_ms)}</strong>
        <span>${durationLabel(segment.end_ms - segment.start_ms)}</span>
        <span>${segment.buckets} × 5s</span>
        <span class="muted">${escapeHTML(segment.reason || '—')}</span>
      </div>
    `;
  }).join('');
}

function renderSymbol(symbol: SymbolHealth): string {
  const sessionStart = timeLabel(symbol.session_start_ms);
  const sessionEnd = timeLabel(symbol.session_end_ms);
  const elapsed = symbol.elapsed_end_ms && symbol.session_end_ms
    ? Math.max(0, Math.min(100, ((symbol.elapsed_end_ms - (symbol.session_start_ms ?? symbol.elapsed_end_ms)) / (symbol.session_end_ms - (symbol.session_start_ms ?? symbol.session_end_ms))) * 100))
    : 0;
  const breaks = symbol.timeline.filter((segment) => segment.state === 'MISSING' || segment.state === 'DEGRADED');
  const events = symbol.timeline.filter((segment) => segment.state !== 'GOOD');

  return `
    <article class="five-second-symbol-row" data-symbol="${escapeHTML(symbol.symbol)}">
      <div class="five-second-symbol-summary">
        <div class="five-second-symbol-title">
          <strong>${escapeHTML(symbol.symbol)}</strong>
          ${statusBadge(symbol.status)}
        </div>
        <div class="five-second-symbol-kpis">
          <span>Health <strong>${pct(symbol.healthy_pct)}</strong></span>
          <span>Complete <strong>${pct(symbol.completeness_pct)}</strong></span>
          <span>Expected <strong>${symbol.expected}</strong></span>
          <span>Missing <strong class="${symbol.missing > 0 ? 'five-second-bad-text' : ''}">${symbol.missing}</strong></span>
          <span>Degraded <strong class="${symbol.degraded > 0 ? 'five-second-warn-text' : ''}">${symbol.degraded}</strong></span>
          <span>Last 5s <strong>${timeLabel(symbol.last_5s_ms)}</strong></span>
        </div>
      </div>

      <div class="five-second-timeline-wrap">
        <div class="five-second-timeline" aria-label="${escapeHTML(symbol.symbol)} current day 5 second candle timeline">
          <div class="five-second-timeline-elapsed" style="width:${elapsed.toFixed(5)}%"></div>
          ${timelineSegments(symbol)}
        </div>
        <div class="five-second-timeline-axis">
          <span>${sessionStart}</span>
          <span>${sessionEnd}</span>
        </div>
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
    </article>
  `;
}

function render(payload: HealthResponse): void {
  const body = ensurePanel();
  if (!body) return;

  const state = document.querySelector<HTMLSpanElement>('#five-second-day-health-state');
  const check = payload.symbols.some((symbol) => symbol.status === 'CHECK');
  if (state) {
    state.className = `badge ${check ? 'bad' : 'good'}`;
    state.textContent = check ? 'CHECK' : 'HEALTHY';
  }

  if (payload.symbols.length === 0) {
    body.innerHTML = '<div class="feed-provider-card"><strong>No index 5s health rows available.</strong></div>';
    return;
  }

  body.innerHTML = payload.symbols.map(renderSymbol).join('');
}

async function refreshFiveSecondHealth(): Promise<void> {
  const body = ensurePanel();
  if (!body) {
    window.setTimeout(() => void refreshFiveSecondHealth(), 300);
    return;
  }

  try {
    const response = await fetch(new URL('api/v1/5s-health/', qnextRoot()), {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      cache: 'no-store',
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    render(await response.json() as HealthResponse);
  } catch (error) {
    const state = document.querySelector<HTMLSpanElement>('#five-second-day-health-state');
    if (state) {
      state.className = 'badge bad';
      state.textContent = 'UNAVAILABLE';
    }
    body.innerHTML = `<div class="feed-provider-card"><strong>5s day-health unavailable</strong><div class="muted">${escapeHTML((error as Error).message)}</div></div>`;
  }
}

void refreshFiveSecondHealth();
window.setInterval(() => void refreshFiveSecondHealth(), HEALTH_REFRESH_MS);
