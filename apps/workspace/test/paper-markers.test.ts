import { describe, expect, it } from 'vitest';

import { paperSnapshotMatchesChart } from '../src/charts/lightweight-workspace';
import {
  normalizePaperMarkerSnapshot,
  toLightweightPaperMarker,
} from '../src/charts/paper-markers';

describe('paper markers', () => {
  it('normalizes, sorts and maps safe marker payloads', () => {
    const snapshot = normalizePaperMarkerSnapshot({
      available: true,
      enabled: true,
      instrumentId: 'QNEXT:NIFTY-SYN+',
      timeframe: '1m',
      sessionId: 'paper-1',
      markers: [
        {
          id: 'fill-1',
          eventTimeMs: 121_000,
          anchorTimeMs: 121_000,
          kind: 'FILL',
          side: 'BUY',
          price: 101.25,
          label: 'BUY fill',
        },
        {
          id: 'signal-1',
          eventTimeMs: 121_000,
          anchorTimeMs: 61_000,
          kind: 'SIGNAL',
          side: 'BUY',
          price: 100.5,
          label: 'cross up',
        },
        { id: '', kind: 'FILL' },
      ],
    });

    expect(snapshot.markers.map((marker) => marker.id)).toEqual(['signal-1', 'fill-1']);

    const lightweight = toLightweightPaperMarker(snapshot.markers[0]);
    expect(lightweight.time).toBe(61);
    expect(lightweight.position).toBe('belowBar');
    expect(lightweight.shape).toBe('circle');
  });

  it('matches either bare ticker or instrument id but requires the paper timeframe', () => {
    const snapshot = normalizePaperMarkerSnapshot({
      available: true,
      instrumentId: 'QNEXT:NIFTY-SYN+',
      timeframe: '1m',
      markers: [],
    });

    expect(paperSnapshotMatchesChart(snapshot, 'NIFTY-SYN+', '1m')).toBe(true);
    expect(paperSnapshotMatchesChart(snapshot, 'QNEXT:NIFTY-SYN+', '1m')).toBe(true);
    expect(paperSnapshotMatchesChart(snapshot, 'NIFTY-SYN+', '5m')).toBe(false);
    expect(paperSnapshotMatchesChart(snapshot, 'NIFTY-SYN', '1m')).toBe(false);
  });
});
