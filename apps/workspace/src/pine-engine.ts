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

/**
 * Fresh snapshots for explicitly opted-in MTF scripts and the installed Trinity
 * ribbon (legacy admin-managed copies predate the opt-in marker).
 * This is execution scheduling only: Pine's lookahead/offset semantics stay intact.
 */
export function withConfirmedMtfRecompute(engine: ScriptingEngine): ScriptingEngine {
  const optedIn = new WeakSet<PreparedScript>();
  // A same-timestamp correction to the previous bar or a changed history
  // window must not be swallowed by the ordinary forming-bar tick dedupe.
  // O(1) per tick: do not hash or copy the full history in the UI thread.
  const snapshotKey = (req: ExecutionRequest): string => {
    const bars = req.getBars?.() ?? req.bars;
    const closed = bars[bars.length - 2];
    return JSON.stringify([
      bars.length, bars[0]?.time, bars[bars.length - 1]?.time,
      closed?.time, closed?.open, closed?.high, closed?.low, closed?.close, closed?.volume,
    ]);
  };

  return {
    language: engine.language,
    capabilities: engine.capabilities,
    async prepare(source, id) {
      const prepared = await engine.prepare(source, id);
      const explicit = /^[ \t]*\/\/[ \t]*@qnext-confirmed-mtf-recompute[ \t]*\r?$/m.test(source);
      const legacyTrinity = prepared.meta.title?.trim().toLowerCase() === 'trinity midpoint atr ribbon'
        && /\brequest\s*\.\s*security\s*\(/.test(source);
      if (explicit || legacyTrinity) {
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
      let lastScheduled = snapshotKey(req);
      return {
        ...session,
        notifyBars(reason) {
          if (reason === 'backfill') {
            session.notifyBars(reason);
            return;
          }
          const current = snapshotKey(req);
          if (reason !== 'complete' && current === lastScheduled) return;
          lastScheduled = current;
          session.notifyBars(reason);
        },
      };
    },
  };
}
