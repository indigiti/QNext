import { PineEngine, PineWorkerEngine } from '@luxalgo/vela-pinets';
import type { ScriptingEngine } from '@luxalgo/vela/plugin';
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
