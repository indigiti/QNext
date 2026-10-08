import { Vela } from '@luxalgo/vela';
import { PineWorkerEngine } from '@luxalgo/vela-pinets';

import type { QNextBar, QNextRange } from './qnext-provider';
import type {
  LabChartSnapshot,
  LabIndicatorDescriptor,
} from './intelligence-lab-snapshot';

export interface PineScalarFeature {
  name: string;
  kind: 'number' | 'boolean';
}

export type LabHistoryFetcher = (
  instrumentID: string,
  timeframe: string,
  range: QNextRange,
) => Promise<QNextBar[]>;

const MAX_AUTO_FEATURES = 12;
const CONTRACT_PREFIX = 'QNEXT_FEATURE__';

export function discoverPineScalarFeatures(source: string): PineScalarFeature[] {
  const found = new Map<string, PineScalarFeature>();

  for (const rawLine of source.split(/\r?\n/)) {
    if (/^\s/.test(rawLine)) continue;
    const line = rawLine.trim();
    if (
      !line ||
      line.startsWith('//') ||
      line.includes('input.') ||
      line.includes('box.') ||
      line.includes('line.') ||
      line.includes('label.') ||
      line.includes('array.') ||
      line.includes('table.') ||
      line.includes('color.') ||
      line.includes('"') ||
      line.includes("'")
    ) {
      continue;
    }

    const explicit = line.match(
      /^(?:(?:var|varip)\s+)?(float|int|bool)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=/,
    );
    if (explicit) {
      const type = explicit[1]!;
      const name = explicit[2]!;
      found.set(name, {
        name,
        kind: type === 'bool' ? 'boolean' : 'number',
      });
      if (found.size >= MAX_AUTO_FEATURES) break;
      continue;
    }

    const inferred = line.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
    if (!inferred) continue;
    const name = inferred[1]!;
    const expression = inferred[2]!;
    if (
      /=>/.test(expression) ||
      /\b(?:box|line|label|array|table|color)\b/.test(expression)
    ) {
      continue;
    }

    const looksBoolean =
      /(?:>=|<=|==|!=|>|<)/.test(expression) ||
      /\b(?:and|or|not)\b/.test(expression);
    const looksNumeric =
      /\b(?:open|high|low|close|volume|bar_index|ta\.)\b/.test(expression) ||
      /[+\-*/]/.test(expression) ||
      /\b\d+(?:\.\d+)?\b/.test(expression);

    if (!looksBoolean && !looksNumeric) continue;
    found.set(name, {
      name,
      kind: looksBoolean ? 'boolean' : 'number',
    });
    if (found.size >= MAX_AUTO_FEATURES) break;
  }

  return [...found.values()];
}

export async function augmentPineIntelligenceContracts(
  snapshot: LabChartSnapshot,
  fetchBars: LabHistoryFetcher,
): Promise<LabChartSnapshot> {
  const targets = snapshot.indicators.filter(
    (indicator) =>
      indicator.kind === 'script' &&
      indicator.language === 'pine' &&
      indicator.source &&
      indicator.historical_feature_names.length === 0,
  );
  if (targets.length === 0) return snapshot;

  const range = historyRange(snapshot);
  const bars = await fetchBars(snapshot.instrument_id, snapshot.timeframe, range);
  if (bars.length < 5) return snapshot;

  const output = cloneSnapshot(snapshot);
  for (const target of targets) {
    const candidates = discoverPineScalarFeatures(target.source ?? '');
    if (candidates.length === 0) continue;

    const extracted = await runOffscreenContract(
      target,
      candidates,
      bars,
      snapshot.timeframe,
    );
    mergeExtracted(output, target.instance_id, extracted);
  }
  output.feature_rows.sort((left, right) => left.bar_time_ms - right.bar_time_ms);
  return output;
}

function historyRange(snapshot: LabChartSnapshot): QNextRange {
  if (snapshot.feature_rows.length > 0) {
    const ordered = [...snapshot.feature_rows].sort(
      (left, right) => left.bar_time_ms - right.bar_time_ms,
    );
    return {
      from: ordered[0]!.bar_time_ms,
      to: ordered.at(-1)!.bar_time_ms + 1,
      limit: Math.min(10_000, Math.max(ordered.length + 50, 500)),
    };
  }
  return { limit: 1_500 };
}

async function runOffscreenContract(
  indicator: LabIndicatorDescriptor,
  candidates: PineScalarFeature[],
  bars: QNextBar[],
  timeframe: string,
): Promise<Map<string, Array<{ time: number; value: number }>>> {
  const host = document.createElement('div');
  host.setAttribute('aria-hidden', 'true');
  host.style.cssText = [
    'position:fixed',
    'left:-10000px',
    'top:-10000px',
    'width:4px',
    'height:4px',
    'opacity:0',
    'pointer-events:none',
    'overflow:hidden',
  ].join(';');
  document.body.append(host);

  const chart = new Vela(host, {
    data: bars,
    timeframe,
    live: false,
    theme: 'dark',
  });
  chart.registerEngine('pine', new PineWorkerEngine());

  try {
    const all = await runInstrumented(chart, indicator, candidates, bars);
    if (all.size > 0) return all;

    const recovered = new Map<string, Array<{ time: number; value: number }>>();
    for (const candidate of candidates) {
      const single = await runInstrumented(chart, indicator, [candidate], bars);
      for (const [name, points] of single) recovered.set(name, points);
    }
    return recovered;
  } finally {
    chart.destroy();
    host.remove();
  }
}

async function runInstrumented(
  chart: Vela,
  indicator: LabIndicatorDescriptor,
  candidates: PineScalarFeature[],
  bars: QNextBar[],
): Promise<Map<string, Array<{ time: number; value: number }>>> {
  const source = instrumentPineSource(indicator.source ?? '', candidates);
  const result = await (chart as any).runScript(source, {
    language: 'pine',
    inputs: indicator.inputs,
  });
  if (!result?.ok || !result.run) return new Map();

  const extracted = new Map<string, Array<{ time: number; value: number }>>();
  try {
    for (const candidate of candidates) {
      const title = featurePlotTitle(candidate.name);
      try {
        const raw = await result.run.series(title);
        const points = normalizeHistory(raw, bars);
        if (points.length > 0) extracted.set(candidate.name, points);
      } catch {
      }
    }
  } finally {
    result.remove?.();
  }
  return extracted;
}

export function instrumentPineSource(
  source: string,
  candidates: PineScalarFeature[],
): string {
  const lines = candidates.map((candidate) => {
    const expression = candidate.kind === 'boolean'
      ? candidate.name + ' ? 1.0 : 0.0'
      : 'float(' + candidate.name + ')';
    return 'plot(' + expression + ', "' + featurePlotTitle(candidate.name) + '", display=display.none)';
  });
  return (
    source.trimEnd() +
    '\n\n// QNext Intelligence Lab auto-contract (off-screen only)\n' +
    lines.join('\n') +
    '\n'
  );
}

function featurePlotTitle(name: string): string {
  return CONTRACT_PREFIX + name;
}

function normalizeHistory(
  raw: unknown,
  bars: QNextBar[],
): Array<{ time: number; value: number }> {
  if (!Array.isArray(raw)) return [];
  const points: Array<{ time: number; value: number }> = [];

  if (
    raw.some(
      (value) =>
        value !== null &&
        typeof value === 'object' &&
        !Array.isArray(value),
    )
  ) {
    for (const value of raw) {
      if (!value || typeof value !== 'object' || Array.isArray(value)) continue;
      const record = value as Record<string, unknown>;
      if (record.time == null || record.value == null) continue;
      const time = Number(record.time);
      const number = Number(record.value);
      if (Number.isFinite(time) && Number.isFinite(number)) {
        points.push({ time, value: number });
      }
    }
    return points.sort((left, right) => left.time - right.time);
  }

  const offset = Math.max(0, bars.length - raw.length);
  for (let index = 0; index < raw.length; index += 1) {
    const rawValue = raw[index];
    if (rawValue == null) continue;
    const number = Number(rawValue);
    const bar = bars[offset + index];
    if (!bar || !Number.isFinite(number)) continue;
    points.push({ time: bar.time, value: number });
  }
  return points;
}

function mergeExtracted(
  snapshot: LabChartSnapshot,
  indicatorID: string,
  extracted: Map<string, Array<{ time: number; value: number }>>,
): void {
  if (extracted.size === 0) return;
  const descriptor = snapshot.indicators.find(
    (indicator) => indicator.instance_id === indicatorID,
  );
  if (!descriptor) return;

  const byTime = new Map(
    snapshot.feature_rows.map((row) => [
      row.bar_time_ms,
      { ...row.features },
    ]),
  );
  const namespace = safeFeaturePart(indicatorID);

  for (const [name, points] of extracted) {
    const key =
      'indicator.' + namespace + '.contract.' + safeFeaturePart(name);
    descriptor.historical_feature_names.push(key);
    const latest = points.at(-1);
    if (latest) {
      snapshot.current_features[key] = latest.value;
      descriptor.current_feature_names.push(key);
    }
    for (const point of points) {
      const features = byTime.get(point.time) ?? {};
      features[key] = point.value;
      byTime.set(point.time, features);
    }
  }

  descriptor.historical_feature_names = [
    ...new Set(descriptor.historical_feature_names),
  ].sort();
  descriptor.current_feature_names = [
    ...new Set(descriptor.current_feature_names),
  ].sort();
  snapshot.feature_rows = [...byTime.entries()]
    .sort(([left], [right]) => left - right)
    .map(([barTimeMs, features]) => ({
      bar_time_ms: barTimeMs,
      features: Object.fromEntries(
        Object.entries(features).sort(([left], [right]) =>
          left.localeCompare(right),
        ),
      ),
    }));
  snapshot.current_features = Object.fromEntries(
    Object.entries(snapshot.current_features).sort(([left], [right]) =>
      left.localeCompare(right),
    ),
  );
}

function cloneSnapshot(snapshot: LabChartSnapshot): LabChartSnapshot {
  return {
    ...snapshot,
    indicators: snapshot.indicators.map((indicator) => ({
      ...indicator,
      inputs: { ...indicator.inputs },
      historical_feature_names: [...indicator.historical_feature_names],
      current_feature_names: [...indicator.current_feature_names],
    })),
    feature_rows: snapshot.feature_rows.map((row) => ({
      bar_time_ms: row.bar_time_ms,
      features: { ...row.features },
    })),
    current_features: { ...snapshot.current_features },
  };
}

function safeFeaturePart(value: string): string {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');
  return normalized || 'unnamed';
}
