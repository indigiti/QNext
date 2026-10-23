import { describe, expect, it } from 'vitest';
import { VelaSignalTrace } from '../src/charts/vela-signal-trace';

const bar = (time: number, revision = 0) =>
  ({ time, open: 100, high: 102, low: 99, close: 101, volume: 10, revision });

describe('opt-in Vela/Pine signal trace', () => {
  it('identifies a late revision by instrument and timeframe without falsely advancing the newest bar', () => {
    const trace = new VelaSignalTrace();
    trace.providerBar('NSE:NIFTY', '1m', bar(120000));
    trace.velaBar('NSE:NIFTY', '1m', bar(120000));
    trace.providerBar('NSE:NIFTY', '1m', bar(180000));
    trace.velaBar('NSE:NIFTY', '1m', bar(180000));
    trace.providerBar('NSE:NIFTY', '1m', bar(120000, 2));
    const events = trace.snapshot();
    expect(events.at(-1)).toMatchObject({ kind: 'provider-bar', olderThanNewest: true, bar: { time: 120000, revision: 2 } });
    expect(trace.summary()).toMatchObject({ providerBars: 3, velaBars: 2, olderProviderBars: 1 });
    trace.providerBar('NSE:NIFTY', '1m', bar(180000, 3));
    expect(trace.snapshot().at(-1)).toMatchObject({ olderThanNewest: false });
  });

  it('keeps symbol/timeframe order isolated and captures Pine run causes', () => {
    const trace = new VelaSignalTrace();
    trace.providerBar('NSE:NIFTY', '15s', bar(60000));
    trace.providerBar('NSE:NIFTY', '1m', bar(30000));
    trace.providerBar('NSE:BANKNIFTY', '15s', bar(30000));
    trace.pineRun('chart-1', 'Buy Sell Test', 'bar', 60000, true);
    expect(trace.summary()).toMatchObject({ olderProviderBars: 0, pineRuns: 1 });
    expect(trace.snapshot().at(-1)).toMatchObject({ kind: 'pine-run', indicator: 'Buy Sell Test', cause: 'bar' });
  });

  it('keeps only the most recent 300 trace events and snapshots cannot mutate trace', () => {
    const trace = new VelaSignalTrace();
    for (let n = 0; n < 330; n++) trace.providerBar('NSE:NIFTY', '15s', bar(n * 15000));
    expect(trace.snapshot()).toHaveLength(300);
    const s = trace.snapshot();
    expect(s[0]).toMatchObject({ seq: 31 });
    if (s[0].kind === 'provider-bar') s[0].bar.close = -100;
    expect(trace.snapshot()[0]).not.toEqual(s[0]);
  });
});
