import { PineEngine, PineWorkerEngine } from '@luxalgo/vela-pinets';
import type { ExecutionRequest, PreparedScript, ScriptingEngine } from '@luxalgo/vela/plugin';
import type { VelaSignalTrace } from './charts/vela-signal-trace';
import { wrapPineModelTrace } from './charts/vela-pine-model-observer';

export type QNextPineMode = 'worker' | 'main';

// QNext uses the worker in production to keep Pine execution off the UI thread.
// Opt into 'main' in window.__QNEXT_CONFIG__ only for controlled parity diagnosis.
export function resolveQNextPineMode(value: unknown): QNextPineMode {
  return value === 'main' ? 'main' : 'worker';
}

export function createQNextPineEngine(mode: QNextPineMode = 'worker', trace?: VelaSignalTrace): ScriptingEngine {
  const engine = mode === 'main' ? new PineEngine() : new PineWorkerEngine();
  return trace ? wrapPineModelTrace(engine, trace) : engine;
}

/** Opt-in fresh snapshots for confirmed multi-timeframe drawing scripts. */
export function withConfirmedMtfRecompute(engine: ScriptingEngine): ScriptingEngine {
  const optedIn = new WeakSet<PreparedScript>();
  const newestTime = (req: ExecutionRequest): number | undefined => {
    const bars = req.getBars?.() ?? req.bars;
    return bars[bars.length - 1]?.time;
  };

  return {
    language: engine.language,
    capabilities: engine.capabilities,
    async prepare(source, id) {
      const prepared = await engine.prepare(source, id);
      if (/^[ \t]*\/\/[ \t]*@qnext-confirmed-mtf-recompute[ \t]*\r?$/m.test(source)) {
        optedIn.add(prepared);
      }
      return prepared;
    },
    execute(req, handlers) {
      if (req.mode !== 'live' || !optedIn.has(req.prepared)) {
        return engine.execute(req, handlers);
      }
      // Rebuild from the latest candle snapshot rather than mutating a cached
      // secondary Pine context. Input changes still trigger a run immediately.
      const session = engine.execute({ ...req, mode: 'static' }, handlers);
      let lastScheduled = newestTime(req);
      return {
        ...session,
        notifyBars(reason) {
          if (reason === 'backfill') {
            session.notifyBars(reason);
            return;
          }
          const current = newestTime(req);
          if (reason !== 'complete' && current === lastScheduled) return;
          lastScheduled = current;
          session.notifyBars(reason);
        },
      };
    },
  };
}
