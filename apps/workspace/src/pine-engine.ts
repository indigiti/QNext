import { PineEngine, PineWorkerEngine } from '@luxalgo/vela-pinets';

export type QNextPineMode = 'worker' | 'main';

// QNext uses the worker in production to keep Pine execution off the UI thread.
// Opt into 'main' in window.__QNEXT_CONFIG__ only for controlled parity diagnosis.
export function resolveQNextPineMode(value: unknown): QNextPineMode {
  return value === 'main' ? 'main' : 'worker';
}

export function createQNextPineEngine(mode: QNextPineMode = 'worker'): PineEngine | PineWorkerEngine {
  return mode === 'main' ? new PineEngine() : new PineWorkerEngine();
}
