import { describe, expect, it } from 'vitest';

import { resolveChartEngine } from '../src/charts/chart-engine';

describe('chart engine selection', () => {
  it('keeps Vela as the default and for auto mode', () => {
    expect(resolveChartEngine(undefined, '')).toBe('vela');
    expect(resolveChartEngine('auto', '')).toBe('vela');
  });

  it('allows Lightweight Charts through runtime configuration', () => {
    expect(resolveChartEngine('lightweight', '')).toBe('lightweight');
  });

  it('allows a query override for safe side-by-side testing', () => {
    expect(resolveChartEngine('vela', '?chartEngine=lightweight')).toBe('lightweight');
    expect(resolveChartEngine('lightweight', '?chartEngine=vela')).toBe('vela');
  });
});
