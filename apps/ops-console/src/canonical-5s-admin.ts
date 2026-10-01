type IntegrityState = 'GOOD' | 'CARRY_FORWARD' | 'RECOVERED' | 'MISSING' | 'DEGRADED';

type IntegrityEntry = {
  instrument_id?: string;
  open_time_ms?: number;
  close_time_ms?: number;
  state?: IntegrityState;
  reason?: string;
};

type SubminuteTelemetry = {
  enabled?: boolean;
  source_timeframe?: string;
  five_second_finals?: number;
  carry_forward_finals?: number;
  clock_advance_calls?: number;
  finalized_by_interval?: Record<string, number>;
  incomplete_buckets?: Record<string, number>;
  integrity?: {
    counts?: Partial<Record<IntegrityState, number>>;
    latest?: Record<string, IntegrityEntry>;
  };
};

type ProviderSnapshot = {
  last_received_time_ms?: number;
};

type InstrumentSnapshot = {
  provider?: string;
  last_received_time_ms?: number;
  quality?: string;
};

type FeedStatus = {
  nifty_instrument_id?: string;
  synthetic_instrument_id?: string;
  telemetry?: {
    providers?: Record<string, ProviderSnapshot>;
    instruments?: Record<string, InstrumentSnapshot>;
    runtime?: {
      subminute?: SubminuteTelemetry;
    };
  };
};

function qnextRoot(): URL {
  return new URL('../', window.location.href);
}

function ageMS(at?: number): number | null {
  if (!at || at <= 0) return null;
  return Math.max(0, Date.now() - at);
}

function duration(ms: number | null): string {
  if (ms === null || !Number.isFinite(ms)) return '—';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)} s`;
  return `${(ms / 60_000).toFixed(1)} m`;
}

function escapeHTML(value: unknown): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function badge(ok: boolean, label: string): string {
  return `<span class="badge ${ok ? 'good' : 'bad'}">${escapeHTML(label)}</span>`;
}

function stateOK(state?: IntegrityState): boolean {
  return state === 'GOOD' || state === 'CARRY_FORWARD' || state === 'RECOVERED';
}

function carryEligible(feed: FeedStatus, instrumentID: string): boolean {
  const telemetry = feed.telemetry;
  const instrument = telemetry?.instruments?.[instrumentID];
  if (!instrument?.provider) return false;
  const provider = telemetry?.providers?.[instrument.provider];
  const instrumentAge = ageMS(instrument.last_received_time_ms);
  const providerAge = ageMS(provider?.last_received_time_ms);
  const quality = String(instrument.quality ?? '').toLowerCase();
  const blocked = quality === 'invalid' || quality === 'degraded' || quality === 'stale';
  return !blocked
    && instrumentAge !== null
    && instrumentAge <= 30_000
    && providerAge !== null
    && providerAge <= 8_000;
}

function ensureCanonicalFormationRow(): void {
  const card = document.querySelector<HTMLElement>('#candle-timeframes-card');
  if (!card) return;

  const description = card.querySelector<HTMLParagraphElement>('.card-head p.muted');
  if (description) {
    description.textContent = '5s is the locked canonical live source. 15s, 30s and live 1m are derived from complete 5s children; 2m+ roll up from 1m. Upstox 1m remains the historical recovery authority.';
  }

  const note = card.querySelector<HTMLSpanElement>('.actions span.muted');
  if (note) {
    note.textContent = '5s is always ON internally. 1m is also protected because provider 1m is the recovery authority.';
  }

  const grid = card.querySelector<HTMLDivElement>('#candle-timeframes-grid');
  if (!grid || grid.querySelector('[data-canonical-five-second="true"]')) return;

  const locked = document.createElement('label');
  locked.className = 'market-toggle';
  locked.dataset.canonicalFiveSecond = 'true';
  locked.innerHTML = `
    <input type="checkbox" checked disabled aria-label="5s canonical candle source, locked on" />
    <span><strong>5s</strong><small>CANONICAL / LOCKED</small></span>
  `;
  grid.prepend(locked);
}

function patchChartNote(): void {
  const card = document.querySelector<HTMLElement>('#chart-timeframes-card');
  const description = card?.querySelector<HTMLParagraphElement>('.card-head p.muted');
  if (description) {
    description.textContent = 'Controls chart-visible intervals only. Canonical 5s formation remains internal and locked ON; chart-visible intervals must have their Candle Formation interval enabled.';
  }
}

function patchMarketClockLabel(): void {
  document.querySelectorAll<HTMLElement>('#live-observability strong').forEach((element) => {
    if (element.textContent?.trim() === 'MARKET CLOCK') {
      element.textContent = 'MARKET DATA CLOCK';
    }
  });
}

function ensurePanel(): HTMLDivElement | null {
  const card = document.querySelector<HTMLElement>('#feed-status-card');
  if (!card) return null;
  let panel = card.querySelector<HTMLDivElement>('#canonical-5s-telemetry');
  if (panel) return panel;

  const section = document.createElement('div');
  section.id = 'canonical-5s-section';
  section.innerHTML = `
    <div class="line" style="margin-top: 14px; margin-bottom: 8px;">
      <div>
        <strong>Canonical 5s integrity</strong>
        <div class="muted">Independent finalization clock · health-qualified carry · read-only</div>
      </div>
      <span id="canonical-5s-health" class="badge">CHECKING</span>
    </div>
    <div id="canonical-5s-telemetry" class="feed-provider-grid"></div>
  `;

  const actions = card.querySelector('.feed-actions');
  if (actions) card.insertBefore(section, actions);
  else card.append(section);
  return section.querySelector<HTMLDivElement>('#canonical-5s-telemetry');
}

function instrumentCard(feed: FeedStatus, instrumentID: string, label: string, subminute: SubminuteTelemetry): string {
  const entry = subminute.integrity?.latest?.[instrumentID];
  const instrument = feed.telemetry?.instruments?.[instrumentID];
  const provider = instrument?.provider
    ? feed.telemetry?.providers?.[instrument.provider]
    : undefined;
  const instrumentAge = ageMS(instrument?.last_received_time_ms);
  const providerAge = ageMS(provider?.last_received_time_ms);
  const carry = carryEligible(feed, instrumentID);
  const state = entry?.state ?? 'MISSING';

  return `
    <div class="feed-provider-card">
      <div class="line"><strong>${escapeHTML(label)}</strong>${badge(stateOK(state), state)}</div>
      <div class="feed-kv"><span>Instrument</span><strong>${escapeHTML(instrumentID)}</strong></div>
      <div class="feed-kv"><span>Last 5s close age</span><strong>${duration(ageMS(entry?.close_time_ms))}</strong></div>
      <div class="feed-kv"><span>Provider packet age</span><strong>${duration(providerAge)}</strong></div>
      <div class="feed-kv"><span>Instrument data age</span><strong>${duration(instrumentAge)}</strong></div>
      <div class="feed-kv"><span>Carry eligible</span><strong>${carry ? 'YES' : 'NO'}</strong></div>
      <div class="feed-kv"><span>Reason</span><strong>${escapeHTML(entry?.reason || '—')}</strong></div>
    </div>
  `;
}

function render(feed: FeedStatus): void {
  const panel = ensurePanel();
  if (!panel) return;
  const subminute = feed.telemetry?.runtime?.subminute;
  const health = document.querySelector<HTMLSpanElement>('#canonical-5s-health');

  if (!subminute?.enabled) {
    if (health) {
      health.className = 'badge bad';
      health.textContent = 'DISABLED';
    }
    panel.innerHTML = '<div class="feed-provider-card"><strong>Canonical 5s telemetry unavailable</strong><div class="muted">Market Core has not exposed the sub-minute recorder snapshot.</div></div>';
    return;
  }

  const counts = subminute.integrity?.counts ?? {};
  const latest = Object.values(subminute.integrity?.latest ?? {});
  const latestHealthy = latest.length === 0 || latest.every((entry) => stateOK(entry.state));
  const missing = counts.MISSING ?? 0;
  const degraded = counts.DEGRADED ?? 0;
  const ok = latestHealthy && subminute.clock_advance_calls !== 0;

  if (health) {
    health.className = `badge ${ok ? 'good' : 'bad'}`;
    health.textContent = ok ? 'HEALTHY' : 'CHECK';
  }

  const finalized = subminute.finalized_by_interval ?? {};
  const incomplete = subminute.incomplete_buckets ?? {};
  panel.innerHTML = `
    <div class="feed-provider-card">
      <div class="line"><strong>FINALIZATION CLOCK</strong>${badge((subminute.clock_advance_calls ?? 0) > 0, (subminute.clock_advance_calls ?? 0) > 0 ? 'ARMED' : 'WAIT')}</div>
      <div class="feed-kv"><span>Canonical source</span><strong>${escapeHTML(subminute.source_timeframe ?? '5s')}</strong></div>
      <div class="feed-kv"><span>Clock advance calls</span><strong>${subminute.clock_advance_calls ?? 0}</strong></div>
      <div class="feed-kv"><span>5s finals</span><strong>${subminute.five_second_finals ?? 0}</strong></div>
      <div class="feed-kv"><span>Carry-forward finals</span><strong>${subminute.carry_forward_finals ?? 0}</strong></div>
      <div class="feed-kv"><span>15s / 30s / 1m finals</span><strong>${finalized['15s'] ?? 0} / ${finalized['30s'] ?? 0} / ${finalized['1m'] ?? 0}</strong></div>
      <div class="feed-kv"><span>Incomplete 15s / 30s / 1m</span><strong>${incomplete['15s'] ?? 0} / ${incomplete['30s'] ?? 0} / ${incomplete['1m'] ?? 0}</strong></div>
    </div>

    <div class="feed-provider-card">
      <div class="line"><strong>5s INTEGRITY LEDGER</strong>${badge(missing === 0 && degraded === 0, missing === 0 && degraded === 0 ? 'CLEAN' : 'CHECK')}</div>
      <div class="feed-kv"><span>GOOD</span><strong>${counts.GOOD ?? 0}</strong></div>
      <div class="feed-kv"><span>CARRY_FORWARD</span><strong>${counts.CARRY_FORWARD ?? 0}</strong></div>
      <div class="feed-kv"><span>RECOVERED</span><strong>${counts.RECOVERED ?? 0}</strong></div>
      <div class="feed-kv"><span>MISSING</span><strong>${missing}</strong></div>
      <div class="feed-kv"><span>DEGRADED</span><strong>${degraded}</strong></div>
    </div>

    ${feed.nifty_instrument_id ? instrumentCard(feed, feed.nifty_instrument_id, 'NIFTY 5s', subminute) : ''}
    ${feed.synthetic_instrument_id ? instrumentCard(feed, feed.synthetic_instrument_id, 'NIFTY-SYN 5s', subminute) : ''}
  `;
}

async function refresh(): Promise<void> {
  ensureCanonicalFormationRow();
  patchChartNote();
  patchMarketClockLabel();
  try {
    const response = await fetch(new URL('api/v1/feed-status/', qnextRoot()), {
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    render(await response.json() as FeedStatus);
  } catch (error) {
    const panel = ensurePanel();
    const health = document.querySelector<HTMLSpanElement>('#canonical-5s-health');
    if (health) {
      health.className = 'badge bad';
      health.textContent = 'UNAVAILABLE';
    }
    if (panel) {
      panel.innerHTML = `<div class="feed-provider-card"><strong>Canonical 5s telemetry unavailable</strong><div class="muted">${escapeHTML((error as Error).message)}</div></div>`;
    }
  }
}

const candleGrid = document.querySelector('#candle-timeframes-grid');
if (candleGrid) {
  new MutationObserver(() => ensureCanonicalFormationRow()).observe(candleGrid, { childList: true });
}

void refresh();
window.setInterval(() => void refresh(), 5_000);
window.setInterval(patchMarketClockLabel, 1_000);
