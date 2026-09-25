import {
  stableSeriesId,
  type ContextSelect,
  type EngineContextSnapshot,
  type ExecutionHandlers,
  type ExecutionRequest,
  type ExecutionSession,
  type Fill,
  type IndicatorModel,
  type InputSchema,
  type InputValue,
  type LineLikeSeries,
  type OHLCV,
  type PreparedScript,
  type ScriptingEngine,
} from '@luxalgo/vela/plugin';

export interface AdaptiveEmaDefaults {
  priceSource: string;
  emaLength: number;
  lookbackPeriod: number;
  stddevMultiplier: number;
  atrLength: number;
  atrMultiplier: number;
  upColor: string;
  downColor: string;
  colorBars: boolean;
}

export interface QNextIndicatorDefinition {
  schema: 'QNEXT.INDICATOR/1';
  id: string;
  name: string;
  kind: 'adaptive-ema-qalg';
  defaults?: Partial<AdaptiveEmaDefaults>;
}

interface PreparedDefinition {
  definition: QNextIndicatorDefinition;
  instanceId: string;
}

export interface AdaptiveEmaCalculation {
  source: number[];
  ema: number[];
  stddev: Array<number | null>;
  atr: Array<number | null>;
  trend: number[];
}

const DEFAULTS: AdaptiveEmaDefaults = {
  priceSource: 'close',
  emaLength: 20,
  lookbackPeriod: 30,
  stddevMultiplier: 2,
  atrLength: 14,
  atrMultiplier: 1.5,
  upColor: '#00ffaa',
  downColor: '#ff0000',
  colorBars: true,
};

const PRICE_SOURCES = ['close', 'open', 'high', 'low', 'hl2', 'hlc3', 'ohlc4'] as const;

export class QNextIndicatorEngine implements ScriptingEngine {
  readonly language = 'qnext';
  readonly capabilities = {
    streaming: false,
    visibleRange: false,
    inputs: true,
  };

  prepare(source: string, instanceId: string): Promise<PreparedScript> {
    const definition = parseDefinition(source);
    const defaults = resolveDefaults(definition.defaults);
    const inputs = adaptiveInputs(defaults);

    return Promise.resolve({
      language: this.language,
      inputs,
      meta: {
        title: definition.name,
        shorttitle: definition.name,
        overlay: true,
      },
      reactsToViewport: false,
      token: { definition, instanceId } satisfies PreparedDefinition,
    });
  }

  execute(req: ExecutionRequest, handlers: ExecutionHandlers): ExecutionSession {
    const prepared = req.prepared.token as PreparedDefinition;
    const definition = prepared.definition;
    const defaults = resolveDefaults(definition.defaults);
    const schema = adaptiveInputs(defaults);
    const values: Record<string, InputValue> = {
      ...Object.fromEntries(schema.map((input) => [input.key, input.defval])),
      ...req.inputs,
    };

    let stopped = false;
    let snapshot: EngineContextSnapshot | null = null;

    const run = (): void => {
      if (stopped) return;

      try {
        const bars = req.getBars?.() ?? req.bars;
        const settings = settingsFromInputs(values, defaults);
        const result = calculateAdaptiveEma(bars, settings);
        const emaId = stableSeriesId({
          instanceId: prepared.instanceId,
          kind: 'line',
          title: 'EMA',
          ordinal: 0,
        });
        const priceId = stableSeriesId({
          instanceId: prepared.instanceId,
          kind: 'line',
          title: 'Price',
          ordinal: 1,
        });

        const emaSeries: LineLikeSeries = {
          id: emaId,
          title: 'EMA',
          paneId: '',
          kind: 'line',
          points: bars.map((bar, index) => ({
            time: bar.time,
            value: result.ema[index] ?? null,
            color: result.trend[index] > 0 ? settings.upColor : settings.downColor,
          })),
          style: {
            color: settings.downColor,
            width: 3,
            lineStyle: 'solid',
          },
        };

        const priceSeries: LineLikeSeries = {
          id: priceId,
          title: 'Price',
          paneId: '',
          kind: 'line',
          points: bars.map((bar, index) => ({
            time: bar.time,
            value: result.source[index] ?? null,
          })),
          style: {
            color: 'rgba(128,128,128,0)',
            width: 1,
            lineStyle: 'solid',
          },
          display: {
            pane: false,
            priceScale: false,
            legend: false,
            dataWindow: false,
          },
        };

        const fill: Fill = {
          id: stableSeriesId({
            instanceId: prepared.instanceId,
            kind: 'fill',
            title: 'EMA Price Fill',
            ordinal: 0,
          }),
          paneId: '',
          fromSeriesId: emaId,
          toSeriesId: priceId,
          colors: result.trend.map((trend) =>
            trend > 0
              ? colorWithOpacity(settings.upColor, 0.5)
              : colorWithOpacity(settings.downColor, 0.5),
          ),
        };

        const model: IndicatorModel = {
          id: prepared.instanceId,
          title: definition.name,
          shorttitle: definition.name,
          overlay: true,
          paneHint: 'price',
          series: [emaSeries, priceSeries],
          fills: [fill],
          backgrounds: [],
          priceLines: [],
          barColors: settings.colorBars
            ? bars.flatMap((bar, index) => {
                const trend = result.trend[index] ?? 0;
                if (trend === 0) return [];
                return [{
                  time: bar.time,
                  color: trend > 0 ? settings.upColor : settings.downColor,
                }];
              })
            : [],
          inputs: schema,
          inputValues: values,
        };

        snapshot = {
          language: this.language,
          phase: req.mode === 'live' ? 'streaming' : 'idle',
          barIndex: bars.length - 1,
          meta: {
            title: definition.name,
            shorttitle: definition.name,
            overlay: true,
          },
          plots: {
            EMA: emaSeries.points.map((point) => ({
              time: point.time,
              value: point.value,
            })),
          },
          variables: {
            trend: result.trend.at(-1) ?? 0,
            ...values,
          },
          warnings: [],
        };

        handlers.onModel(model);
        if (req.mode === 'static') handlers.onDone?.();
      } catch (error) {
        handlers.onError?.(error instanceof Error ? error : new Error(String(error)));
      }
    };

    run();

    return {
      getContext: (select?: ContextSelect) => {
        if (!snapshot || !select) return Promise.resolve(snapshot);
        const selected = Object.fromEntries(select.map((key) => [key, snapshot![key]]));
        return Promise.resolve(selected as unknown as EngineContextSnapshot);
      },
      stop: () => {
        stopped = true;
      },
      update: (next) => {
        Object.assign(values, next);
        run();
      },
      setVisibleRange: () => {},
      notifyBars: (reason) => {
        if (reason !== 'backfill') run();
      },
    };
  }
}

export function calculateAdaptiveEma(
  bars: readonly OHLCV[],
  settings: AdaptiveEmaDefaults,
): AdaptiveEmaCalculation {
  const source = bars.map((bar) => priceSourceValue(bar, settings.priceSource));
  const ema = emaSeries(source, settings.emaLength);
  const stddev = rollingStdev(ema, settings.lookbackPeriod);
  const atr = atrSeries(bars, settings.atrLength);
  const trend: number[] = [];

  let state = 0;
  for (let index = 0; index < bars.length; index += 1) {
    const emaValue = ema[index];
    const sdValue = stddev[index];
    const atrValue = atr[index];
    const price = source[index];

    if (
      emaValue !== undefined &&
      sdValue !== null &&
      atrValue !== null &&
      price !== undefined
    ) {
      const upperBand = emaValue + sdValue * settings.stddevMultiplier;
      const lowerBand = emaValue - sdValue * settings.stddevMultiplier;
      const atrUpper = emaValue + atrValue * settings.atrMultiplier;
      const atrLower = emaValue - atrValue * settings.atrMultiplier;
      const longCondition = price > upperBand && price > atrUpper;
      const shortCondition = price < lowerBand && price < atrLower;

      if (longCondition && !shortCondition) state = 1;
      if (shortCondition && !longCondition) state = -1;
    }

    trend.push(state);
  }

  return { source, ema, stddev, atr, trend };
}

function parseDefinition(source: string): QNextIndicatorDefinition {
  let decoded: unknown;
  try {
    decoded = JSON.parse(source);
  } catch {
    throw new Error('QNext indicator definition is invalid JSON');
  }

  if (!decoded || typeof decoded !== 'object') {
    throw new Error('QNext indicator definition must be an object');
  }

  const value = decoded as Record<string, unknown>;
  if (value.schema !== 'QNEXT.INDICATOR/1') {
    throw new Error('Unsupported QNext indicator schema');
  }
  if (value.kind !== 'adaptive-ema-qalg') {
    throw new Error(`Unsupported QNext indicator kind: ${String(value.kind ?? '')}`);
  }
  if (typeof value.id !== 'string' || !/^[a-z0-9][a-z0-9-]{0,63}$/.test(value.id)) {
    throw new Error('QNext indicator id is invalid');
  }
  if (typeof value.name !== 'string' || value.name.trim() === '') {
    throw new Error('QNext indicator name is required');
  }

  return value as unknown as QNextIndicatorDefinition;
}

function adaptiveInputs(defaults: AdaptiveEmaDefaults): InputSchema[] {
  return [
    {
      key: 'priceSource',
      title: 'Price Source',
      type: 'source',
      defval: defaults.priceSource,
      options: PRICE_SOURCES,
      group: 'Adaptive EMA Settings',
      tooltip: 'Price source used for the EMA and trend conditions.',
    },
    {
      key: 'emaLength',
      title: 'EMA Length',
      type: 'int',
      defval: defaults.emaLength,
      min: 1,
      max: 1000,
      step: 1,
      group: 'Adaptive EMA Settings',
    },
    {
      key: 'lookbackPeriod',
      title: 'Lookback Period for SD',
      type: 'int',
      defval: defaults.lookbackPeriod,
      min: 2,
      max: 1000,
      step: 1,
      group: 'Adaptive EMA Settings',
    },
    {
      key: 'stddevMultiplier',
      title: 'Standard Deviation Multiplier',
      type: 'float',
      defval: defaults.stddevMultiplier,
      min: 0.1,
      max: 20,
      step: 0.1,
      group: 'Adaptive EMA Settings',
    },
    {
      key: 'atrLength',
      title: 'ATR Length',
      type: 'int',
      defval: defaults.atrLength,
      min: 1,
      max: 1000,
      step: 1,
      group: 'ATR Settings',
    },
    {
      key: 'atrMultiplier',
      title: 'ATR Multiplier',
      type: 'float',
      defval: defaults.atrMultiplier,
      min: 0.1,
      max: 20,
      step: 0.1,
      group: 'ATR Settings',
    },
    {
      key: 'upColor',
      title: 'Up Trend Color',
      type: 'color',
      defval: defaults.upColor,
      group: 'Bar Coloring',
    },
    {
      key: 'downColor',
      title: 'Down Trend Color',
      type: 'color',
      defval: defaults.downColor,
      group: 'Bar Coloring',
    },
    {
      key: 'colorBars',
      title: 'Color Bars?',
      type: 'bool',
      defval: defaults.colorBars,
      group: 'Bar Coloring',
    },
  ];
}

function resolveDefaults(value?: Partial<AdaptiveEmaDefaults>): AdaptiveEmaDefaults {
  return {
    priceSource: PRICE_SOURCES.includes(value?.priceSource as typeof PRICE_SOURCES[number])
      ? value!.priceSource!
      : DEFAULTS.priceSource,
    emaLength: positiveInt(value?.emaLength, DEFAULTS.emaLength, 1, 1000),
    lookbackPeriod: positiveInt(value?.lookbackPeriod, DEFAULTS.lookbackPeriod, 2, 1000),
    stddevMultiplier: finiteNumber(value?.stddevMultiplier, DEFAULTS.stddevMultiplier, 0.1, 20),
    atrLength: positiveInt(value?.atrLength, DEFAULTS.atrLength, 1, 1000),
    atrMultiplier: finiteNumber(value?.atrMultiplier, DEFAULTS.atrMultiplier, 0.1, 20),
    upColor: validColor(value?.upColor) ? value!.upColor! : DEFAULTS.upColor,
    downColor: validColor(value?.downColor) ? value!.downColor! : DEFAULTS.downColor,
    colorBars: typeof value?.colorBars === 'boolean' ? value.colorBars : DEFAULTS.colorBars,
  };
}

function settingsFromInputs(
  values: Record<string, InputValue>,
  defaults: AdaptiveEmaDefaults,
): AdaptiveEmaDefaults {
  return resolveDefaults({
    priceSource: typeof values.priceSource === 'string' ? values.priceSource : defaults.priceSource,
    emaLength: typeof values.emaLength === 'number' ? values.emaLength : defaults.emaLength,
    lookbackPeriod: typeof values.lookbackPeriod === 'number' ? values.lookbackPeriod : defaults.lookbackPeriod,
    stddevMultiplier: typeof values.stddevMultiplier === 'number' ? values.stddevMultiplier : defaults.stddevMultiplier,
    atrLength: typeof values.atrLength === 'number' ? values.atrLength : defaults.atrLength,
    atrMultiplier: typeof values.atrMultiplier === 'number' ? values.atrMultiplier : defaults.atrMultiplier,
    upColor: typeof values.upColor === 'string' ? values.upColor : defaults.upColor,
    downColor: typeof values.downColor === 'string' ? values.downColor : defaults.downColor,
    colorBars: typeof values.colorBars === 'boolean' ? values.colorBars : defaults.colorBars,
  });
}

function priceSourceValue(bar: OHLCV, source: string): number {
  switch (source) {
    case 'open': return bar.open;
    case 'high': return bar.high;
    case 'low': return bar.low;
    case 'hl2': return (bar.high + bar.low) / 2;
    case 'hlc3': return (bar.high + bar.low + bar.close) / 3;
    case 'ohlc4': return (bar.open + bar.high + bar.low + bar.close) / 4;
    default: return bar.close;
  }
}

function emaSeries(values: readonly number[], period: number): number[] {
  if (values.length === 0) return [];
  const alpha = 2 / (period + 1);
  const out: number[] = [];
  let previous = values[0] ?? 0;

  for (let index = 0; index < values.length; index += 1) {
    const value = values[index] ?? previous;
    previous = index === 0 ? value : alpha * value + (1 - alpha) * previous;
    out.push(previous);
  }

  return out;
}

function rollingStdev(values: readonly number[], period: number): Array<number | null> {
  const out: Array<number | null> = [];
  for (let index = 0; index < values.length; index += 1) {
    if (index + 1 < period) {
      out.push(null);
      continue;
    }

    const window = values.slice(index + 1 - period, index + 1);
    const mean = window.reduce((sum, value) => sum + value, 0) / period;
    const variance =
      window.reduce((sum, value) => sum + (value - mean) ** 2, 0) / period;
    out.push(Math.sqrt(variance));
  }
  return out;
}

function atrSeries(bars: readonly OHLCV[], period: number): Array<number | null> {
  const out: Array<number | null> = [];
  let seed = 0;
  let previousAtr: number | null = null;
  let previousClose: number | null = null;

  for (let index = 0; index < bars.length; index += 1) {
    const bar = bars[index]!;
    const trueRange = previousClose === null
      ? bar.high - bar.low
      : Math.max(
          bar.high - bar.low,
          Math.abs(bar.high - previousClose),
          Math.abs(bar.low - previousClose),
        );
    previousClose = bar.close;

    if (index < period) {
      seed += trueRange;
      if (index === period - 1) {
        previousAtr = seed / period;
        out.push(previousAtr);
      } else {
        out.push(null);
      }
      continue;
    }

    previousAtr = ((previousAtr ?? trueRange) * (period - 1) + trueRange) / period;
    out.push(previousAtr);
  }

  return out;
}

function positiveInt(
  value: number | undefined,
  fallback: number,
  min: number,
  max: number,
): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.round(value)));
}

function finiteNumber(
  value: number | undefined,
  fallback: number,
  min: number,
  max: number,
): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

function validColor(value: string | undefined): boolean {
  return typeof value === 'string' && /^#[0-9a-fA-F]{6}$/.test(value);
}

function colorWithOpacity(hex: string, opacity: number): string {
  const red = Number.parseInt(hex.slice(1, 3), 16);
  const green = Number.parseInt(hex.slice(3, 5), 16);
  const blue = Number.parseInt(hex.slice(5, 7), 16);
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`;
}
