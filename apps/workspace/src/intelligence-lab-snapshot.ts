export const LAB_CHART_SNAPSHOT_SCHEMA = 'QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1';
export const LAB_FEATURE_SCHEMA_VERSION = 'qnext-chart-indicators-v2';

export interface LabIndicatorDescriptor {
  instance_id: string;
  title: string;
  kind: 'script' | 'native';
  language?: string;
  native_type?: string;
  source?: string;
  source_hash: string;
  inputs: Record<string, string | number | boolean>;
  configuration_hash: string;
  historical_feature_names: string[];
  current_feature_names: string[];
}

export interface LabFeatureRow {
  bar_time_ms: number;
  features: Record<string, number>;
}

export interface LabChartSnapshot {
  schema: typeof LAB_CHART_SNAPSHOT_SCHEMA;
  feature_schema_version: typeof LAB_FEATURE_SCHEMA_VERSION;
  created_at_ms: number;
  instrument_id: string;
  timeframe: string;
  indicator_configuration_hash: string;
  indicators: LabIndicatorDescriptor[];
  feature_rows: LabFeatureRow[];
  current_features: Record<string, number>;
}

interface IndicatorInputSchema {
  key: string;
  defval?: unknown;
}

interface IndicatorHandleLike {
  id: string;
  title: string;
  visible: boolean;
  source?: string;
  nativeType?: string;
  inputs?: readonly IndicatorInputSchema[];
  inputValues?: () => Record<string, unknown>;
  context: (select?: string[]) => Promise<unknown>;
}

interface ChartLike {
  indicators: () => IndicatorHandleLike[];
}

interface WorkspaceLike {
  getState: () => unknown;
  chart: ChartLike;
}

type ContextLike = {
  language?: unknown;
  meta?: { language?: unknown };
  plots?: Record<string, unknown>;
  variables?: Record<string, unknown>;
};

export interface LabChartContext {
  feature_schema_version: typeof LAB_FEATURE_SCHEMA_VERSION;
  instrument_id: string;
  timeframe: string;
  indicator_configuration_hash: string;
  indicator_ids: string[];
}

export async function captureActiveChartLabContext(
  workspace: WorkspaceLike,
  resolveInstrumentID?: (symbol: string) => Promise<string>,
): Promise<LabChartContext> {
  const market = activeMarketFromState(workspace.getState());
  const instrumentID = resolveInstrumentID
    ? await resolveInstrumentID(market.symbol)
    : market.symbol;
  const handles = workspace.chart.indicators().filter((handle) => handle.visible);
  if (handles.length === 0) {
    throw new Error('No enabled indicators are present on the active chart');
  }

  const descriptors: Array<{
    instance_id: string;
    title: string;
    kind: 'script' | 'native';
    language: string;
    native_type: string;
    source_hash: string;
    inputs: Record<string, string | number | boolean>;
    configuration_hash: string;
  }> = [];

  for (const handle of handles) {
    const context = await handle.context(['language', 'meta']) as ContextLike | null;
    const source = typeof handle.source === 'string' ? handle.source : undefined;
    const nativeType = typeof handle.nativeType === 'string' ? handle.nativeType : undefined;
    const sourceMaterial = source ?? `native:${nativeType ?? handle.title}`;
    const sourceHash = await sha256Hex(sourceMaterial);
    const inputs = resolveInputValues(handle, persistedInputDeltas(market.chartState, handle));
    const descriptorMaterial = {
      instance_id: handle.id,
      title: handle.title,
      kind: source ? 'script' as const : 'native' as const,
      language: stringValue(context?.language) ?? stringValue(context?.meta?.language) ?? '',
      native_type: nativeType ?? '',
      source_hash: sourceHash,
      inputs,
    };
    descriptors.push({
      ...descriptorMaterial,
      configuration_hash: await sha256Hex(canonicalJson(descriptorMaterial)),
    });
  }

  descriptors.sort((left, right) => left.instance_id.localeCompare(right.instance_id));
  const indicatorConfigurationHash = await sha256Hex(canonicalJson({
    instrument_id: instrumentID,
    timeframe: market.timeframe,
    indicators: descriptors,
  }));

  return {
    feature_schema_version: LAB_FEATURE_SCHEMA_VERSION,
    instrument_id: instrumentID,
    timeframe: market.timeframe,
    indicator_configuration_hash: indicatorConfigurationHash,
    indicator_ids: descriptors.map((descriptor) => descriptor.instance_id),
  };
}

export async function captureActiveChartLabSnapshot(
  workspace: WorkspaceLike,
  createdAtMs = Date.now(),
  resolveInstrumentID?: (symbol: string) => Promise<string>,
): Promise<LabChartSnapshot> {
  const market = activeMarketFromState(workspace.getState());
  const instrumentID = resolveInstrumentID
    ? await resolveInstrumentID(market.symbol)
    : market.symbol;
  const handles = workspace.chart.indicators().filter((handle) => handle.visible);
  if (handles.length === 0) {
    throw new Error('No enabled indicators are present on the active chart');
  }

  const descriptors: LabIndicatorDescriptor[] = [];
  const rows = new Map<number, Record<string, number>>();
  const currentFeatures: Record<string, number> = {};

  for (const handle of handles) {
    const context = await handle.context(['language', 'meta', 'plots', 'variables']) as ContextLike | null;
    const source = typeof handle.source === 'string' ? handle.source : undefined;
    const nativeType = typeof handle.nativeType === 'string' ? handle.nativeType : undefined;
    const sourceMaterial = source ?? `native:${nativeType ?? handle.title}`;
    const sourceHash = await sha256Hex(sourceMaterial);
    const inputs = resolveInputValues(handle, persistedInputDeltas(market.chartState, handle));

    const descriptorMaterial = {
      instance_id: handle.id,
      title: handle.title,
      kind: source ? 'script' : 'native',
      language: stringValue(context?.language) ?? stringValue(context?.meta?.language),
      native_type: nativeType,
      source_hash: sourceHash,
      inputs,
    };
    const configurationHash = await sha256Hex(canonicalJson(descriptorMaterial));
    const contributed = collectContextFeatures(
      handle.id,
      context,
      rows,
      currentFeatures,
    );
    descriptors.push({
      instance_id: handle.id,
      title: handle.title,
      kind: source ? 'script' : 'native',
      ...(descriptorMaterial.language ? { language: descriptorMaterial.language } : {}),
      ...(nativeType ? { native_type: nativeType } : {}),
      ...(source ? { source } : {}),
      source_hash: sourceHash,
      inputs,
      configuration_hash: configurationHash,
      historical_feature_names: contributed.historical,
      current_feature_names: contributed.current,
    });
  }

  descriptors.sort((a, b) => a.instance_id.localeCompare(b.instance_id));
  const configurationHash = await sha256Hex(canonicalJson({
    instrument_id: instrumentID,
    timeframe: market.timeframe,
    indicators: descriptors.map((descriptor) => ({
      instance_id: descriptor.instance_id,
      title: descriptor.title,
      kind: descriptor.kind,
      language: descriptor.language ?? '',
      native_type: descriptor.native_type ?? '',
      source_hash: descriptor.source_hash,
      inputs: descriptor.inputs,
      configuration_hash: descriptor.configuration_hash,
    })),
  }));

  return {
    schema: LAB_CHART_SNAPSHOT_SCHEMA,
    feature_schema_version: LAB_FEATURE_SCHEMA_VERSION,
    created_at_ms: createdAtMs,
    instrument_id: instrumentID,
    timeframe: market.timeframe,
    indicator_configuration_hash: configurationHash,
    indicators: descriptors,
    feature_rows: [...rows.entries()]
      .sort(([left], [right]) => left - right)
      .map(([barTimeMs, features]) => ({
        bar_time_ms: barTimeMs,
        features: sortNumberRecord(features),
      })),
    current_features: sortNumberRecord(currentFeatures),
  };
}

function activeMarketFromState(state: unknown): { symbol: string; timeframe: string; chartState: Record<string, any> } {
  if (!isRecord(state) || !Array.isArray(state.charts) || state.charts.length === 0) {
    throw new Error('Vela workspace state does not contain a chart');
  }

  const activeId = typeof state.activeCellId === 'string' ? state.activeCellId : '';
  const chart = state.charts.find(
    (candidate) => isRecord(candidate) && activeId && candidate.id === activeId,
  ) ?? state.charts[0];

  if (!isRecord(chart)) {
    throw new Error('Vela active chart state is invalid');
  }
  const symbol = typeof chart.symbol === 'string' ? chart.symbol.trim() : '';
  const timeframe = typeof chart.timeframe === 'string' ? chart.timeframe.trim() : '';
  if (!symbol || !timeframe) {
    throw new Error('Vela active chart is missing symbol or timeframe');
  }
  return { symbol, timeframe, chartState: chart };
}

function persistedInputDeltas(
  chartState: Record<string, any>,
  handle: IndicatorHandleLike,
): Record<string, unknown> {
  const indicators = isRecord(chartState.indicators) ? chartState.indicators : null;
  if (!indicators) return {};

  const manifest = Array.isArray(indicators.manifest) ? indicators.manifest : [];
  const natives = Array.isArray(indicators.natives) ? indicators.natives : [];
  const ledgers = handle.source !== undefined
    ? [
        { entries: manifest, identity: 'name' },
        { entries: natives, identity: 'type' },
      ]
    : [
        { entries: natives, identity: 'type' },
        { entries: manifest, identity: 'name' },
      ];

  for (const ledger of ledgers) {
    for (const entry of ledger.entries) {
      if (typeof entry === 'string') {
        if (entry === handle.title) return {};
        continue;
      }
      if (!isRecord(entry)) continue;
      if (entry[ledger.identity] !== handle.title) continue;
      return isRecord(entry.inputs) ? entry.inputs : {};
    }
  }
  return {};
}

function resolveInputValues(
  handle: IndicatorHandleLike,
  persisted: Record<string, unknown> = {},
): Record<string, string | number | boolean> {
  let values: Record<string, unknown> = {};
  try {
    const provided = handle.inputValues?.();
    if (provided && typeof provided === 'object') values = provided;
  } catch {
    values = {};
  }

  const resolved: Record<string, string | number | boolean> = {};
  for (const input of handle.inputs ?? []) {
    const value = input.key in values
      ? values[input.key]
      : input.key in persisted
        ? persisted[input.key]
        : input.defval;
    if (
      typeof value === 'string' ||
      typeof value === 'boolean' ||
      (typeof value === 'number' && Number.isFinite(value))
    ) {
      resolved[input.key] = value;
    }
  }
  return Object.fromEntries(
    Object.entries(resolved).sort(([left], [right]) => left.localeCompare(right)),
  );
}

function collectContextFeatures(
  indicatorId: string,
  context: ContextLike | null,
  rows: Map<number, Record<string, number>>,
  currentFeatures: Record<string, number>,
): { historical: string[]; current: string[] } {
  if (!context) return { historical: [], current: [] };
  const namespace = safeFeaturePart(indicatorId);
  const historical = new Set<string>();
  const current = new Set<string>();

  for (const [plotName, raw] of Object.entries(context.plots ?? {})) {
    const key = `indicator.${namespace}.plot.${safeFeaturePart(plotName)}`;
    const points = plotPoints(raw);
    for (const point of points) {
      const row = rows.get(point.time) ?? {};
      row[key] = point.value;
      rows.set(point.time, row);
      historical.add(key);
    }
    const latest = points.at(-1);
    if (latest) {
      currentFeatures[key] = latest.value;
      current.add(key);
    }
  }

  for (const [name, raw] of Object.entries(context.variables ?? {})) {
    const key = `indicator.${namespace}.var.${safeFeaturePart(name)}`;
    const points = plotPoints(raw);
    if (points.length > 0) {
      for (const point of points) {
        const row = rows.get(point.time) ?? {};
        row[key] = point.value;
        rows.set(point.time, row);
        historical.add(key);
      }
      const latest = points.at(-1);
      if (latest) {
        currentFeatures[key] = latest.value;
        current.add(key);
      }
      continue;
    }

    const value = numericValue(raw);
    if (value === undefined) continue;
    currentFeatures[key] = value;
    current.add(key);
  }

  return {
    historical: [...historical].sort(),
    current: [...current].sort(),
  };
}

function plotPoints(value: unknown): Array<{ time: number; value: number }> {
  if (!Array.isArray(value)) return [];
  const points: Array<{ time: number; value: number }> = [];
  for (const item of value) {
    if (!isRecord(item)) continue;
    const time = numericValue(item.time);
    const pointValue = numericValue(item.value);
    if (
      time === undefined ||
      pointValue === undefined ||
      !Number.isInteger(time) ||
      time <= 0
    ) {
      continue;
    }
    points.push({ time, value: pointValue });
  }
  return points.sort((left, right) => left.time - right.time);
}

function numericValue(value: unknown): number | undefined {
  if (typeof value === 'boolean') return value ? 1 : 0;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  return undefined;
}

function safeFeaturePart(value: string): string {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');
  return normalized || 'unnamed';
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function sortNumberRecord(value: Record<string, number>): Record<string, number> {
  return Object.fromEntries(
    Object.entries(value).sort(([left], [right]) => left.localeCompare(right)),
  );
}

function canonicalJson(value: unknown): string {
  return JSON.stringify(sortCanonical(value));
}

function sortCanonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortCanonical);
  if (!isRecord(value)) return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, sortCanonical(value[key])]),
  );
}

async function sha256Hex(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value);
  const digest = await globalThis.crypto.subtle.digest('SHA-256', bytes);
  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('');
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
