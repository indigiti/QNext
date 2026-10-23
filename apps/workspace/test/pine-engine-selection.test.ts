import { describe, expect, it } from 'vitest';
import { PineEngine, PineWorkerEngine } from '@luxalgo/vela-pinets';
import { createQNextPineEngine, resolveQNextPineMode } from '../src/pine-engine';

describe('Pine engine selection', () => {
  it('uses the Web Worker by default; invalid configs cannot switch execution mode', () => {
    expect(resolveQNextPineMode(undefined)).toBe('worker');
    expect(resolveQNextPineMode('pine')).toBe('worker');
    expect(createQNextPineEngine(resolveQNextPineMode(undefined))).toBeInstanceOf(PineWorkerEngine);
  });

  it('allows explicitly selecting the in-process engine for parity diagnosis', () => {
    expect(resolveQNextPineMode('main')).toBe('main');
    expect(createQNextPineEngine('main')).toBeInstanceOf(PineEngine);
  });

  it('keeps the same Pine language and streaming capabilities in either engine', () => {
    const engines = [createQNextPineEngine('worker'), createQNextPineEngine('main')];
    for (const engine of engines) {
      expect(engine.language).toBe('pine');
      expect(engine.capabilities.streaming).toBe(true);
      expect(engine.capabilities.inputs).toBe(true);
    }
  });
});
