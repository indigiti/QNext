/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';
import main from '../src/main.ts?raw';
import velaRuntime from '../src/vela-runtime.ts?raw';
import { normalizeLightweightLayout } from '../src/charts/chart-engine';

describe('selected workspace runtime loading', () => {
  it('loads only the selected chart runtime from bootstrap', () => {
    const vela = main.split('async function bootstrapVela(')[1]?.split('async function bootstrapLightweight(')[0] ?? '';
    const lightweight = main.split('async function bootstrapLightweight(')[1]?.split('async function bootstrap()')[0] ?? '';
    expect(vela).toContain("await import('./vela-runtime')");
    expect(vela).not.toContain("import('./charts/lightweight-workspace')");
    expect(lightweight).toContain("import('./charts/lightweight-workspace')");
    expect(lightweight).not.toContain("import('./vela-runtime')");
    const staticImports = main.split('\n').filter(line => /^import /.test(line) && !/^import type /.test(line)).join('\n');
    for (const source of ['@luxalgo/vela', './pine-engine', './charts/lightweight-workspace', './vela-runtime', './indicator-templates\'']) {
      expect(staticImports).not.toContain(source);
    }
    expect(velaRuntime).toContain("from '@luxalgo/vela/workspace'");
    expect(velaRuntime).toContain("from './pine-engine'");
  });

  it('normalizes settings without importing either chart engine', () => {
    for (const [input, expected] of [[2, 2], ['2', 2], [4, 4], ['4', 4], [1, 1], ['1', 1], [undefined, 1], [3, 1]]) {
      expect(normalizeLightweightLayout(input)).toBe(expected);
    }
  });
});
