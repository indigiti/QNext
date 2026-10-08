import { describe, expect, it } from 'vitest';

import {
  discoverPineScalarFeatures,
  instrumentPineSource,
} from '../src/pine-intelligence-contract';

describe('Pine Intelligence Lab auto-contract', () => {
  it('discovers safe top-level scalar state without selecting drawings or inputs', () => {
    const source = [
      '//@version=6',
      'indicator("Order Block", overlay=true)',
      'pivotLength = input.int(5, "Pivot")',
      'var int structureTrend = 0',
      'var float lastSwingHigh = na',
      'var float lastSwingLow = na',
      'bool bullishChoch = structureTrend == -1 and close > lastSwingHigh',
      'bearishChoch = structureTrend == 1 and close < lastSwingLow',
      'array<float> bullTops = array.new_float()',
      'var box activeBox = na',
      'if bullishChoch',
      '    float localTop = high',
    ].join('\n');

    expect(discoverPineScalarFeatures(source)).toEqual([
      { name: 'structureTrend', kind: 'number' },
      { name: 'lastSwingHigh', kind: 'number' },
      { name: 'lastSwingLow', kind: 'number' },
      { name: 'bullishChoch', kind: 'boolean' },
      { name: 'bearishChoch', kind: 'boolean' },
    ]);
  });

  it('adds hidden plots only to the Lab clone', () => {
    const source = '//@version=6\nindicator("Demo")\nvar int trend = 0';
    const instrumented = instrumentPineSource(source, [
      { name: 'trend', kind: 'number' },
      { name: 'breakout', kind: 'boolean' },
    ]);

    expect(instrumented).toContain(
      'plot(float(trend), "QNEXT_FEATURE__trend", display=display.none)',
    );
    expect(instrumented).toContain(
      'plot(breakout ? 1.0 : 0.0, "QNEXT_FEATURE__breakout", display=display.none)',
    );
    expect(source).not.toContain('QNEXT_FEATURE__');
  });
});
