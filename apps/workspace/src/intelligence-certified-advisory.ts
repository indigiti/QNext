import {
  captureActiveChartLabContext,
  type LabChartContext,
} from './intelligence-lab-snapshot';

interface WorkspaceLike {
  getState: () => unknown;
  chart: {
    indicators: () => Array<{
      id: string;
      title: string;
      visible: boolean;
      source?: string;
      nativeType?: string;
      inputs?: readonly { key: string; defval?: unknown }[];
      inputValues?: () => Record<string, unknown>;
      context: (select?: string[]) => Promise<unknown>;
    }>;
  };
}

export interface CertifiedAdvisory {
  advisoryId: string;
  experimentId: string;
  instrumentId: string;
  timeframe: string;
  indicatorConfigurationHash: string;
  featureSchemaVersion: string;
  certifiedAtMs: number;
  barTimeMs: number;
  asOfTimeMs: number;
  createdAtMs: number;
  modelAlgorithm: string;
  modelHash: string;
  decision: 'BUY' | 'SELL' | 'NO_TRADE';
  probabilities: {
    BUY: number;
    SELL: number;
    NO_TRADE: number;
  };
  entryPrice: number;
  target1Price?: number;
  target2Price?: number;
  invalidationPrice?: number;
  expectedReturnPct?: number;
  expectedHorizonBars?: number;
  recommendationPolicyId: string;
}

export interface CertifiedAdvisorySnapshot {
  available: boolean;
  advisories: CertifiedAdvisory[];
}

const POLL_MS = 3_000;

export function normalizeCertifiedAdvisorySnapshot(
  value: unknown,
): CertifiedAdvisorySnapshot {
  const payload = isRecord(value) ? value : {};
  const advisories = Array.isArray(payload.advisories)
    ? payload.advisories
        .map(normalizeAdvisory)
        .filter((item): item is CertifiedAdvisory => item !== undefined)
        .sort((left, right) => right.asOfTimeMs - left.asOfTimeMs)
        .slice(0, 64)
    : [];

  return {
    available: payload.available === true,
    advisories,
  };
}

export function advisoryMatchesContext(
  advisory: CertifiedAdvisory,
  context: LabChartContext,
): boolean {
  return (
    advisory.instrumentId === context.instrument_id &&
    advisory.timeframe === context.timeframe &&
    advisory.featureSchemaVersion === context.feature_schema_version &&
    advisory.indicatorConfigurationHash ===
      context.indicator_configuration_hash
  );
}

export function advisoryFreshness(
  advisory: CertifiedAdvisory,
  nowMs = Date.now(),
): { fresh: boolean; ageMs: number; maxAgeMs: number } {
  const step = timeframeMs(advisory.timeframe);
  const maxAgeMs = Math.max(step * 8, 5 * 60_000);
  const ageMs = Math.max(0, nowMs - advisory.asOfTimeMs);
  return {
    fresh: ageMs <= maxAgeMs,
    ageMs,
    maxAgeMs,
  };
}

export function mountCertifiedAdvisory(
  workspace: WorkspaceLike,
  resolveInstrumentID: (symbol: string) => Promise<string>,
  apiBase = '',
): () => void {
  const existing = document.querySelector<HTMLElement>(
    '#qnext-certified-intelligence',
  );
  existing?.remove();

  const root = document.createElement('aside');
  root.id = 'qnext-certified-intelligence';
  root.hidden = true;
  root.setAttribute('aria-live', 'polite');
  root.style.cssText = [
    'position:fixed',
    'right:18px',
    'bottom:78px',
    'z-index:1000',
    'width:min(360px,calc(100vw - 36px))',
    'padding:12px',
    'border:1px solid rgba(80,200,140,.26)',
    'border-radius:12px',
    'background:rgba(11,13,18,.95)',
    'box-shadow:0 10px 32px rgba(0,0,0,.38)',
    'backdrop-filter:blur(10px)',
    'font:12px system-ui',
    'color:#dce5ef',
  ].join(';');
  document.body.append(root);

  let stopped = false;
  let running = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const base = apiBase.replace(/\/+$/, '');

  const poll = async () => {
    if (stopped || running) return;
    running = true;
    try {
      const response = await fetch(
        `${base}/api/v1/intelligence-advisory/`,
        {
          cache: 'no-store',
          headers: { Accept: 'application/json' },
        },
      );
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const snapshot = normalizeCertifiedAdvisorySnapshot(
        await response.json(),
      );
      if (!snapshot.available || snapshot.advisories.length === 0) {
        hide(root);
        return;
      }

      const context = await captureActiveChartLabContext(
        workspace,
        resolveInstrumentID,
      );
      const advisory = snapshot.advisories.find((item) =>
        advisoryMatchesContext(item, context),
      );
      if (!advisory) {
        hide(root);
        return;
      }

      render(root, advisory);
    } catch (error) {
      console.warn('QNext certified advisory unavailable', error);
      hide(root);
    } finally {
      running = false;
      if (!stopped) timer = setTimeout(poll, POLL_MS);
    }
  };

  void poll();

  return () => {
    stopped = true;
    if (timer !== undefined) clearTimeout(timer);
    root.remove();
  };
}

function render(root: HTMLElement, advisory: CertifiedAdvisory): void {
  const freshness = advisoryFreshness(advisory);
  const stale = !freshness.fresh;
  const decisionClass =
    advisory.decision === 'BUY'
      ? '#60d394'
      : advisory.decision === 'SELL'
        ? '#ff7b86'
        : '#c1cad8';

  root.hidden = false;
  root.innerHTML = `
    <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:12px">
      <div>
        <div style="font-size:10px;letter-spacing:.12em;text-transform:uppercase;color:#75d7a7;font-weight:800">Certified Intelligence</div>
        <div style="margin-top:3px;color:#8896a8">Advisory only · no execution</div>
      </div>
      <div style="font-size:10px;color:${stale ? '#f5b761' : '#75d7a7'};font-weight:800">
        ${stale ? 'STALE' : 'LIVE'}
      </div>
    </div>
    <div style="display:flex;align-items:end;justify-content:space-between;gap:12px;margin-top:10px">
      <div style="font-size:24px;font-weight:850;color:${decisionClass}">${escapeHTML(advisory.decision.replace('_', ' '))}</div>
      <div style="text-align:right;color:#8d99aa;font-size:10px">
        ${escapeHTML(advisory.modelAlgorithm)}<br/>
        ${escapeHTML(advisory.modelHash.slice(0, 12))}
      </div>
    </div>
    <div style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:6px;margin-top:10px">
      ${metric('BUY', pct(advisory.probabilities.BUY))}
      ${metric('SELL', pct(advisory.probabilities.SELL))}
      ${metric('NO TRADE', pct(advisory.probabilities.NO_TRADE))}
    </div>
    <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;margin-top:8px">
      ${metric('Entry', price(advisory.entryPrice))}
      ${metric('Invalidation', price(advisory.invalidationPrice))}
      ${metric('Target 1', price(advisory.target1Price))}
      ${metric('Target 2', price(advisory.target2Price))}
      ${metric('Expected', signedPct(advisory.expectedReturnPct))}
      ${metric('Horizon', advisory.expectedHorizonBars === undefined ? '—' : `${advisory.expectedHorizonBars.toFixed(1)} bars`)}
    </div>
    <div style="display:flex;justify-content:space-between;gap:8px;margin-top:9px;padding-top:8px;border-top:1px solid rgba(255,255,255,.07);color:#738094;font-size:10px">
      <span>As of ${escapeHTML(formatTime(advisory.asOfTimeMs))}</span>
      <span>${escapeHTML(advisory.experimentId.slice(0, 18))}</span>
    </div>
  `;
}

function metric(label: string, value: string): string {
  return `
    <div style="padding:6px 7px;border:1px solid rgba(255,255,255,.06);border-radius:7px;background:rgba(255,255,255,.025)">
      <div style="font-size:9px;color:#768396;text-transform:uppercase;letter-spacing:.05em">${escapeHTML(label)}</div>
      <div style="margin-top:2px;font-weight:750;color:#e5ebf3">${escapeHTML(value)}</div>
    </div>
  `;
}

function normalizeAdvisory(value: unknown): CertifiedAdvisory | undefined {
  if (!isRecord(value)) return undefined;
  const decision =
    value.decision === 'BUY' ||
    value.decision === 'SELL' ||
    value.decision === 'NO_TRADE'
      ? value.decision
      : undefined;
  const probabilities = isRecord(value.probabilities)
    ? value.probabilities
    : {};
  const buy = finiteNumber(probabilities.BUY);
  const sell = finiteNumber(probabilities.SELL);
  const noTrade = finiteNumber(probabilities.NO_TRADE);
  const entryPrice = finiteNumber(value.entryPrice);

  const advisoryId = stringValue(value.advisoryId);
  const experimentId = stringValue(value.experimentId);
  const instrumentId = stringValue(value.instrumentId);
  const timeframe = stringValue(value.timeframe);
  const configurationHash = stringValue(value.indicatorConfigurationHash);
  const featureSchemaVersion = stringValue(value.featureSchemaVersion);
  const modelAlgorithm = stringValue(value.modelAlgorithm);
  const modelHash = stringValue(value.modelHash);
  const policyId = stringValue(value.recommendationPolicyId);
  const certifiedAtMs = positiveNumber(value.certifiedAtMs);
  const barTimeMs = positiveNumber(value.barTimeMs);
  const asOfTimeMs = positiveNumber(value.asOfTimeMs);
  const createdAtMs = positiveNumber(value.createdAtMs);

  if (
    !decision ||
    buy === undefined ||
    sell === undefined ||
    noTrade === undefined ||
    entryPrice === undefined ||
    !advisoryId ||
    !experimentId ||
    !instrumentId ||
    !timeframe ||
    !/^[a-f0-9]{64}$/.test(configurationHash) ||
    !featureSchemaVersion ||
    !modelAlgorithm ||
    !/^[a-f0-9]{64}$/.test(modelHash) ||
    !policyId ||
    certifiedAtMs === undefined ||
    barTimeMs === undefined ||
    asOfTimeMs === undefined ||
    createdAtMs === undefined
  ) {
    return undefined;
  }

  const total = buy + sell + noTrade;
  if (Math.abs(total - 1.0) > 0.01) return undefined;

  return {
    advisoryId,
    experimentId,
    instrumentId,
    timeframe,
    indicatorConfigurationHash: configurationHash,
    featureSchemaVersion,
    certifiedAtMs,
    barTimeMs,
    asOfTimeMs,
    createdAtMs,
    modelAlgorithm,
    modelHash,
    decision,
    probabilities: {
      BUY: buy,
      SELL: sell,
      NO_TRADE: noTrade,
    },
    entryPrice,
    ...(finiteNumber(value.target1Price) === undefined
      ? {}
      : { target1Price: finiteNumber(value.target1Price)! }),
    ...(finiteNumber(value.target2Price) === undefined
      ? {}
      : { target2Price: finiteNumber(value.target2Price)! }),
    ...(finiteNumber(value.invalidationPrice) === undefined
      ? {}
      : { invalidationPrice: finiteNumber(value.invalidationPrice)! }),
    ...(finiteNumber(value.expectedReturnPct) === undefined
      ? {}
      : { expectedReturnPct: finiteNumber(value.expectedReturnPct)! }),
    ...(finiteNumber(value.expectedHorizonBars) === undefined
      ? {}
      : { expectedHorizonBars: finiteNumber(value.expectedHorizonBars)! }),
    recommendationPolicyId: policyId,
  };
}

function hide(root: HTMLElement): void {
  root.hidden = true;
  root.replaceChildren();
}

function finiteNumber(value: unknown): number | undefined {
  if (
    value === null ||
    value === undefined ||
    typeof value === 'boolean' ||
    value === ''
  ) {
    return undefined;
  }
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
}

function positiveNumber(value: unknown): number | undefined {
  const number = finiteNumber(value);
  return number !== undefined && number > 0 ? number : undefined;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function price(value: number | undefined): string {
  if (value === undefined) return '—';
  return Math.abs(value) >= 1_000 ? value.toFixed(1) : value.toFixed(2);
}

function pct(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function signedPct(value: number | undefined): string {
  if (value === undefined) return '—';
  return `${value >= 0 ? '+' : ''}${(value * 100).toFixed(2)}%`;
}

function timeframeMs(value: string): number {
  const match = value.trim().match(/^(\d+)(s|m|h|D|W|M)$/);
  if (!match) return 60_000;
  const amount = Number(match[1]);
  const unit = match[2];
  if (unit === 's') return amount * 1_000;
  if (unit === 'm') return amount * 60_000;
  if (unit === 'h') return amount * 3_600_000;
  if (unit === 'D') return amount * 86_400_000;
  if (unit === 'W') return amount * 7 * 86_400_000;
  return amount * 30 * 86_400_000;
}

function formatTime(value: number): string {
  try {
    return new Date(value).toLocaleString([], {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return String(value);
  }
}

function escapeHTML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function isRecord(value: unknown): value is Record<string, any> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
