import {
  captureActiveChartLabSnapshot,
  type LabChartSnapshot,
} from './intelligence-lab-snapshot';
import {
  augmentPineIntelligenceContracts,
  type LabHistoryFetcher,
} from './pine-intelligence-contract';
import type { QNextCanonicalBar, QNextRange } from './qnext-provider';

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

interface ShadowTarget {
  experiment_id: string;
  instrument_id: string;
  timeframe: string;
  indicator_configuration_hash: string;
  feature_schema_version: string;
  horizon_bars: number;
  started_at_ms: number;
}

type CanonicalBarFetcher = (
  instrumentID: string,
  timeframe: string,
  range?: QNextRange,
) => Promise<QNextCanonicalBar[]>;

const TARGET_REFRESH_MS = 15_000;
const SHADOW_POLL_MS = 2_000;
const SHADOW_HISTORY_ROWS = 128;
const STORAGE_KEY = 'qnext-intelligence-lab-shadow-last-v1';

export function mountIntelligenceLabShadowBridge(
  workspace: WorkspaceLike,
  resolveInstrumentID: (symbol: string) => Promise<string>,
  fetchCanonicalBars: CanonicalBarFetcher,
  fetchBars: LabHistoryFetcher,
): () => void {
  let stopped = false;
  let targets: ShadowTarget[] = [];
  let targetRefreshTimer: ReturnType<typeof setTimeout> | undefined;
  let shadowTimer: ReturnType<typeof setTimeout> | undefined;
  let targetRefreshInFlight = false;
  let shadowInFlight = false;
  const lastQueued = loadLastQueued();

  const status = ensureShadowStatus();

  const refreshTargets = async () => {
    if (stopped || targetRefreshInFlight) return;
    targetRefreshInFlight = true;
    try {
      const response = await fetch(
        '/qnext/admin/api/index.php?route=%2Fintelligence-lab%2Fshadow-targets',
        {
          credentials: 'same-origin',
          cache: 'no-store',
          headers: { Accept: 'application/json' },
        },
      );
      if (response.status === 403) {
        targets = [];
        setStatus(status, 'Shadow: Admin sign-in required');
        return;
      }
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const payload = (await response.json()) as { targets?: unknown };
      targets = Array.isArray(payload.targets)
        ? payload.targets.filter(isShadowTarget)
        : [];
      setStatus(
        status,
        targets.length > 0
          ? `Shadow: ${targets.length} active`
          : 'Shadow: idle',
      );
    } catch (error) {
      setStatus(
        status,
        `Shadow: ${error instanceof Error ? error.message : String(error)}`,
      );
    } finally {
      targetRefreshInFlight = false;
      if (!stopped) {
        targetRefreshTimer = setTimeout(refreshTargets, TARGET_REFRESH_MS);
      }
    }
  };

  const tick = async () => {
    if (stopped || shadowInFlight) return;
    shadowInFlight = true;
    try {
      if (targets.length === 0) return;

      const active = activeMarket(workspace.getState());
      if (!active) return;
      const instrumentID = await resolveInstrumentID(active.symbol);
      const matching = targets.filter(
        (target) =>
          target.instrument_id === instrumentID &&
          target.timeframe === active.timeframe,
      );
      if (matching.length === 0) {
        setStatus(status, 'Shadow: active chart has no matching experiment');
        return;
      }

      const stepMs = timeframeMs(active.timeframe);
      const now = Date.now();
      const canonical = await fetchCanonicalBars(
        instrumentID,
        active.timeframe,
        {
          from: now - Math.max(stepMs * 16, 2 * 60 * 60 * 1_000),
          to: now,
          limit: 4,
        },
      );
      const latest = canonical
        .filter(
          (bar) =>
            bar.final &&
            (bar.quality === 'GOOD' || bar.quality === 'RECOVERED'),
        )
        .at(-1);
      if (!latest) {
        setStatus(status, 'Shadow: waiting for finalized bar');
        return;
      }

      const pendingTargets = matching.filter(
        (target) =>
          latest.time >= target.started_at_ms - timeframeMs(active.timeframe) &&
          (lastQueued[target.experiment_id] ?? 0) < latest.time,
      );
      if (pendingTargets.length === 0) {
        setStatus(status, `Shadow: captured ${formatTime(latest.time)}`);
        return;
      }

      let snapshot = await captureActiveChartLabSnapshot(
        workspace,
        Date.now(),
        async () => instrumentID,
      );
      const needsPineContract = snapshot.indicators.some(
        (indicator) =>
          indicator.language === 'pine' &&
          indicator.historical_feature_names.length === 0,
      );
      if (needsPineContract) {
        snapshot = await augmentPineIntelligenceContracts(snapshot, fetchBars);
      }

      const rows = snapshot.feature_rows
        .filter((row) => row.bar_time_ms <= latest.time)
        .slice(-SHADOW_HISTORY_ROWS);
      if (rows.at(-1)?.bar_time_ms !== latest.time) {
        setStatus(status, 'Shadow: waiting for indicator finalization');
        return;
      }

      for (const target of pendingTargets) {
        if (
          snapshot.indicator_configuration_hash !==
            target.indicator_configuration_hash ||
          snapshot.feature_schema_version !== target.feature_schema_version
        ) {
          setStatus(status, 'Shadow: chart indicator context changed');
          continue;
        }

        const response = await fetch(
          '/qnext/admin/api/index.php?route=%2Fintelligence-lab%2Fshadow-observation',
          {
            method: 'POST',
            credentials: 'same-origin',
            headers: {
              Accept: 'application/json',
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              experimentId: target.experiment_id,
              indicatorConfigurationHash:
                snapshot.indicator_configuration_hash,
              featureSchemaVersion: snapshot.feature_schema_version,
              barTimeMs: latest.time,
              createdAtMs: Date.now(),
              featureRows: rows,
              currentFeatures: snapshot.current_features,
            }),
          },
        );

        if (response.status === 403) {
          setStatus(status, 'Shadow: Admin sign-in required');
          return;
        }
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `HTTP ${response.status}`);
        }

        lastQueued[target.experiment_id] = latest.time;
        saveLastQueued(lastQueued);
      }
      setStatus(status, `Shadow: queued ${formatTime(latest.time)}`);
    } catch (error) {
      setStatus(
        status,
        `Shadow: ${error instanceof Error ? error.message : String(error)}`,
      );
    } finally {
      shadowInFlight = false;
      if (!stopped) {
        shadowTimer = setTimeout(tick, SHADOW_POLL_MS);
      }
    }
  };

  void refreshTargets();
  void tick();

  return () => {
    stopped = true;
    if (targetRefreshTimer !== undefined) clearTimeout(targetRefreshTimer);
    if (shadowTimer !== undefined) clearTimeout(shadowTimer);
    status?.remove();
  };
}

function activeMarket(
  state: unknown,
): { symbol: string; timeframe: string } | null {
  if (!isRecord(state) || !Array.isArray(state.charts) || state.charts.length === 0) {
    return null;
  }
  const activeId = typeof state.activeCellId === 'string' ? state.activeCellId : '';
  const chart =
    state.charts.find(
      (candidate) =>
        isRecord(candidate) && activeId !== '' && candidate.id === activeId,
    ) ?? state.charts[0];
  if (!isRecord(chart)) return null;
  const symbol = typeof chart.symbol === 'string' ? chart.symbol.trim() : '';
  const timeframe =
    typeof chart.timeframe === 'string' ? chart.timeframe.trim() : '';
  return symbol && timeframe ? { symbol, timeframe } : null;
}

function ensureShadowStatus(): HTMLSpanElement | null {
  const root = document.querySelector<HTMLDivElement>(
    '#qnext-intelligence-lab-bridge',
  );
  if (!root) return null;
  const existing = root.querySelector<HTMLSpanElement>(
    '#qnext-intelligence-lab-shadow-status',
  );
  if (existing) return existing;
  const status = document.createElement('span');
  status.id = 'qnext-intelligence-lab-shadow-status';
  status.textContent = 'Shadow: idle';
  status.style.cssText =
    'font:11px system-ui;color:#7f8b9d;max-width:220px;white-space:nowrap';
  root.prepend(status);
  return status;
}

function setStatus(status: HTMLSpanElement | null, message: string): void {
  if (status) status.textContent = message;
}

function isShadowTarget(value: unknown): value is ShadowTarget {
  if (!isRecord(value)) return false;
  return (
    typeof value.experiment_id === 'string' &&
    typeof value.instrument_id === 'string' &&
    typeof value.timeframe === 'string' &&
    typeof value.indicator_configuration_hash === 'string' &&
    typeof value.feature_schema_version === 'string' &&
    typeof value.horizon_bars === 'number' &&
    typeof value.started_at_ms === 'number'
  );
}

function isRecord(value: unknown): value is Record<string, any> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function loadLastQueued(): Record<string, number> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const parsed = raw ? JSON.parse(raw) : {};
    if (!isRecord(parsed)) return {};
    return Object.fromEntries(
      Object.entries(parsed)
        .filter(([, value]) => typeof value === 'number' && Number.isFinite(value))
        .map(([key, value]) => [key, Number(value)]),
    );
  } catch {
    return {};
  }
}

function saveLastQueued(value: Record<string, number>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
  } catch {
    // Shadow de-duplication is also enforced server-side.
  }
}

function timeframeMs(value: string): number {
  const match = value.trim().match(/^(\d+)(s|m|h|D)$/);
  if (!match) return 60_000;
  const amount = Number(match[1]);
  const unit = match[2];
  if (unit === 's') return amount * 1_000;
  if (unit === 'm') return amount * 60_000;
  if (unit === 'h') return amount * 3_600_000;
  return amount * 86_400_000;
}

function formatTime(value: number): string {
  try {
    return new Date(value).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return String(value);
  }
}
