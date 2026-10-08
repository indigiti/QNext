import { describe, expect, it } from 'vitest';

import {
  advisoryFreshness,
  advisoryMatchesContext,
  normalizeCertifiedAdvisorySnapshot,
} from '../src/intelligence-certified-advisory';

const raw = {
  available: true,
  advisories: [
    {
      advisoryId: 'adv-1',
      experimentId: 'lab-1',
      instrumentId: 'NSE:NIFTY50',
      timeframe: '1m',
      indicatorConfigurationHash: 'a'.repeat(64),
      featureSchemaVersion: 'qnext-chart-indicators-v2',
      certifiedAtMs: 1_800_000_000_000,
      barTimeMs: 1_800_000_060_000,
      asOfTimeMs: 1_800_000_120_000,
      createdAtMs: 1_800_000_121_000,
      modelAlgorithm: 'qnext-ridge-direction-v1',
      modelHash: 'b'.repeat(64),
      decision: 'BUY',
      probabilities: {
        BUY: 0.71,
        SELL: 0.18,
        NO_TRADE: 0.11,
      },
      entryPrice: 100,
      target1Price: 101,
      target2Price: 102,
      invalidationPrice: 99,
      expectedReturnPct: 0.012,
      expectedHorizonBars: 3,
      recommendationPolicyId: 'rec-1',
    },
  ],
};

describe('certified intelligence advisory', () => {
  it('normalizes sanitized certified advisory payloads', () => {
    const snapshot = normalizeCertifiedAdvisorySnapshot(raw);
    expect(snapshot.available).toBe(true);
    expect(snapshot.advisories).toHaveLength(1);
    expect(snapshot.advisories[0]).toMatchObject({
      decision: 'BUY',
      entryPrice: 100,
      target1Price: 101,
      invalidationPrice: 99,
    });
    expect(snapshot.advisories[0]?.probabilities.BUY).toBeCloseTo(0.71);
  });

  it('rejects malformed probabilities and preserves null optional levels', () => {
    const malformed = normalizeCertifiedAdvisorySnapshot({
      available: true,
      advisories: [
        {
          ...raw.advisories[0],
          probabilities: { BUY: 0.9, SELL: 0.9, NO_TRADE: 0.9 },
        },
        {
          ...raw.advisories[0],
          advisoryId: 'adv-2',
          target1Price: null,
          target2Price: null,
          invalidationPrice: null,
          expectedReturnPct: null,
          expectedHorizonBars: null,
        },
      ],
    });

    expect(malformed.advisories).toHaveLength(1);
    expect(malformed.advisories[0]?.advisoryId).toBe('adv-2');
    expect(malformed.advisories[0]?.target1Price).toBeUndefined();
    expect(malformed.advisories[0]?.expectedReturnPct).toBeUndefined();
  });

  it('requires exact certified chart context', () => {
    const advisory = normalizeCertifiedAdvisorySnapshot(raw).advisories[0]!;
    const context = {
      feature_schema_version: 'qnext-chart-indicators-v2' as const,
      instrument_id: 'NSE:NIFTY50',
      timeframe: '1m',
      indicator_configuration_hash: 'a'.repeat(64),
      indicator_ids: ['ema-1'],
    };

    expect(advisoryMatchesContext(advisory, context)).toBe(true);
    expect(
      advisoryMatchesContext(advisory, {
        ...context,
        indicator_configuration_hash: 'c'.repeat(64),
      }),
    ).toBe(false);
    expect(
      advisoryMatchesContext(advisory, {
        ...context,
        timeframe: '5m',
      }),
    ).toBe(false);
  });

  it('marks advisories stale after the bounded timeframe window', () => {
    const advisory = normalizeCertifiedAdvisorySnapshot(raw).advisories[0]!;
    expect(
      advisoryFreshness(advisory, advisory.asOfTimeMs + 60_000).fresh,
    ).toBe(true);
    expect(
      advisoryFreshness(advisory, advisory.asOfTimeMs + 10 * 60_000).fresh,
    ).toBe(false);
  });
});
