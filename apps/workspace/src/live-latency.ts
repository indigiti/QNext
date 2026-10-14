// Low-overhead monotonic browser timing: WSS message receipt -> JSON parse ->
// provider normalization -> all synchronous chart consumers. This does NOT
// include network transit or the actual screen paint.
export interface BrowserDispatchLatency {
  samples: number;
  p50_us: number;
  p95_us: number;
  p99_us: number;
}

export class SampledBrowserLatency {
  private calls = 0;
  private counts = new Uint32Array(26);

  shouldSample(): boolean {
    this.calls += 1;
    return this.calls % 16 === 0;
  }

  observe(durationMs: number): void {
    if (!Number.isFinite(durationMs) || durationMs < 0) return;
    const us = Math.max(1, Math.ceil(durationMs * 1_000));
    const bucket = Math.min(this.counts.length - 1, Math.ceil(Math.log2(us)));
    this.counts[bucket] += 1;
  }

  snapshot(): BrowserDispatchLatency {
    let samples = 0;
    for (const count of this.counts) samples += count;
    if (samples === 0) return { samples: 0, p50_us: 0, p95_us: 0, p99_us: 0 };
    const percentile = (percentage: number): number => {
      const threshold = Math.ceil(samples * percentage / 100);
      let seen = 0;
      for (let bucket = 0; bucket < this.counts.length; bucket += 1) {
        seen += this.counts[bucket];
        if (seen >= threshold) return 2 ** bucket;
      }
      return 2 ** (this.counts.length - 1);
    };
    return {
      samples,
      p50_us: percentile(50),
      p95_us: percentile(95),
      p99_us: percentile(99),
    };
  }
}

// Aggregated across the QNext provider instances in one browser tab.
export const liveWSSDispatchLatency = new SampledBrowserLatency();
export function qnextBrowserLatencySnapshot(): BrowserDispatchLatency {
  return liveWSSDispatchLatency.snapshot();
}
