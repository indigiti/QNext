import type {
  ScriptingEngine, ExecutionRequest, ExecutionHandlers, ExecutionSession, PreparedScript,
} from '@luxalgo/vela/plugin';
import type { IndicatorModel } from '@luxalgo/vela/plugin';
import type { PineTraceMarker, VelaSignalTrace } from './vela-signal-trace';

/**
 * Diagnostic adapter around either upstream PineEngine or PineWorkerEngine.
 * Installed only when pine_trace=1; never alters models, alerts or session controls.
 */
export function wrapPineModelTrace(engine: ScriptingEngine, trace: VelaSignalTrace): ScriptingEngine {
  return {
    language: engine.language,
    capabilities: engine.capabilities,
    prepare(source: string, id: string): Promise<PreparedScript> {
      return engine.prepare(source, id);
    },
    execute(req: ExecutionRequest, handlers: ExecutionHandlers): ExecutionSession {
      return engine.execute(req, {
        ...handlers,
        onModel(model: IndicatorModel): void {
          try {
            const bars = req.getBars?.() ?? req.bars;
            trace.pineModel(req.market.symbol, req.market.timeframe, model.title, model.id,
              markerSnapshot(model, bars));
          } catch (error) {
            // A passive trace must never stop Pine or lose a Buy/Sell marker.
            console.warn('QNext Pine model trace failed', error);
          }
          handlers.onModel(model);
        },
      });
    },
  };
}

/** Public Vela model geometry; translate index-based plotshape labels with the model anchor. */
export function markerSnapshot(
  model: IndicatorModel,
  bars: ReadonlyArray<{ time: number }>,
): PineTraceMarker[] {
  const anchor = model.anchorTime === undefined ? 0 : bars.findIndex((bar) => bar.time === model.anchorTime);
  const barOffset = Math.max(0, anchor);
  const labelTime = (xloc: string, x: number): number | undefined => {
    if (xloc === 'bar_time') return x;
    const index = barOffset + Math.trunc(x);
    return bars[index]?.time;
  };
  const labels: PineTraceMarker[] = (model.labels ?? []).map((label) => ({
    id: label.id,
    source: 'label',
    x: label.x,
    xloc: label.xloc,
    ...(labelTime(label.xloc, label.x) === undefined ? {} : { time: labelTime(label.xloc, label.x) }),
    shape: label.style,
    ...(label.text === undefined ? {} : { text: label.text.slice(0, 80) }),
    ...(label.color === undefined ? {} : { color: label.color }),
    visible: true,
  }));
  const markers: PineTraceMarker[] = model.series.flatMap((series) =>
    series.kind !== 'markers'
      ? []
      : series.markers.map((marker, idx) => ({
        id: series.id + ':' + marker.time + ':' + idx,
        source: 'marker-series' as const,
        x: marker.time,
        xloc: 'bar_time',
        time: marker.time,
        shape: marker.shape,
        ...(marker.text === undefined ? {} : { text: marker.text.slice(0, 80) }),
        color: marker.color,
        visible: series.visible !== false && series.display?.pane !== false,
      })),
  );
  return [...labels, ...markers];
}
