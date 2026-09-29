import type { SeriesMarker, Time, UTCTimestamp } from 'lightweight-charts';

export type PaperMarkerKind = 'SIGNAL' | 'FILL';
export type PaperMarkerSide = 'BUY' | 'SELL';

export interface QNextPaperMarker {
  id: string;
  eventTimeMs: number;
  anchorTimeMs: number;
  kind: PaperMarkerKind;
  side: PaperMarkerSide;
  price: number;
  label: string;
}

export interface QNextPaperMarkerSnapshot {
  available: boolean;
  enabled: boolean;
  instrumentId: string;
  timeframe: string;
  sessionId: string;
  markers: QNextPaperMarker[];
}

export interface PaperMarkerClientOptions {
  apiBase?: string;
  fetchImpl?: typeof fetch;
}

export class QNextPaperMarkerClient {
  private readonly apiBase: string;
  private readonly fetchImpl: typeof fetch;

  constructor(options: PaperMarkerClientOptions = {}) {
    this.apiBase = (options.apiBase ?? '').replace(/\/+$/, '');
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  async snapshot(): Promise<QNextPaperMarkerSnapshot> {
    const response = await this.fetchImpl(`${this.apiBase}/api/v1/paper-markers/`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      throw new Error(`QNext paper markers request failed: HTTP ${response.status}`);
    }
    return normalizePaperMarkerSnapshot(await response.json());
  }
}

export function normalizePaperMarkerSnapshot(value: unknown): QNextPaperMarkerSnapshot {
  const payload = isRecord(value) ? value : {};
  const rawMarkers = Array.isArray(payload.markers) ? payload.markers : [];
  const markers = rawMarkers
    .map(normalizePaperMarker)
    .filter((marker): marker is QNextPaperMarker => marker !== undefined)
    .sort((a, b) => a.anchorTimeMs - b.anchorTimeMs || a.id.localeCompare(b.id))
    .slice(-100);

  return {
    available: payload.available === true,
    enabled: payload.enabled === true,
    instrumentId: typeof payload.instrumentId === 'string' ? payload.instrumentId : '',
    timeframe: typeof payload.timeframe === 'string' ? payload.timeframe : '',
    sessionId: typeof payload.sessionId === 'string' ? payload.sessionId : '',
    markers,
  };
}

export function toLightweightPaperMarker(
  marker: QNextPaperMarker,
): SeriesMarker<Time> {
  const time = Math.floor(marker.anchorTimeMs / 1_000) as UTCTimestamp;
  const text = `${marker.kind === 'FILL' ? 'FILL' : 'SIG'} ${marker.side} @ ${formatMarkerPrice(marker.price)}`;

  if (marker.kind === 'FILL') {
    return {
      id: marker.id,
      time,
      position: marker.side === 'BUY' ? 'belowBar' : 'aboveBar',
      shape: marker.side === 'BUY' ? 'arrowUp' : 'arrowDown',
      color: marker.side === 'BUY' ? '#26a69a' : '#ef5350',
      text,
      size: 1,
    };
  }

  return {
    id: marker.id,
    time,
    position: marker.side === 'BUY' ? 'belowBar' : 'aboveBar',
    shape: 'circle',
    color: marker.side === 'BUY' ? '#64d8cb' : '#ff817b',
    text,
    size: 0.8,
  };
}

function normalizePaperMarker(value: unknown): QNextPaperMarker | undefined {
  if (!isRecord(value)) return undefined;

  const id = typeof value.id === 'string' ? value.id.trim() : '';
  const kind = value.kind === 'SIGNAL' || value.kind === 'FILL' ? value.kind : undefined;
  const side = value.side === 'BUY' || value.side === 'SELL' ? value.side : undefined;
  const eventTimeMs = Number(value.eventTimeMs);
  const anchorTimeMs = Number(value.anchorTimeMs);
  const price = Number(value.price);
  const label = typeof value.label === 'string' ? value.label : '';

  if (
    !id ||
    !kind ||
    !side ||
    !Number.isFinite(eventTimeMs) ||
    eventTimeMs <= 0 ||
    !Number.isFinite(anchorTimeMs) ||
    anchorTimeMs <= 0 ||
    !Number.isFinite(price)
  ) {
    return undefined;
  }

  return { id, eventTimeMs, anchorTimeMs, kind, side, price, label };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function formatMarkerPrice(value: number): string {
  if (Math.abs(value) >= 1_000) return value.toFixed(1);
  return value.toFixed(2);
}
