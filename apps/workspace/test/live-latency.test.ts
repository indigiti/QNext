import { describe, expect, it } from 'vitest';
import { SampledBrowserLatency } from '../src/live-latency';

describe('SampledBrowserLatency', () => {
  it('samples 1 in 16 messages and reports bounded percentile estimates', () => {
    const h = new SampledBrowserLatency();
    let selected = 0;
    for (let i = 0; i < 160; i += 1) {
      if (h.shouldSample()) {
        selected += 1;
        h.observe(i < 152 ? 0.002 : 2);
      }
    }
    expect(selected).toBe(10);
    expect(h.snapshot()).toEqual({
      samples: 10, p50_us: 2, p95_us: 2048, p99_us: 2048,
    });
  });

  it('ignores invalid durations', () => {
    const h = new SampledBrowserLatency();
    h.observe(-1);
    h.observe(Number.NaN);
    expect(h.snapshot().samples).toBe(0);
  });
});
