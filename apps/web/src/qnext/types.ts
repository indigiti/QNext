export type DataQuality =
  | "GOOD"
  | "RECOVERED"
  | "PARTIAL"
  | "STALE"
  | "DEGRADED"
  | "INVALID"
  | string;

export interface QNextBar {
  instrumentId: string;
  timeframe: string;
  openTimeMs: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  final: boolean;
  revision: number;
  quality: DataQuality;
}

export interface Subscription {
  symbol: string;
  timeframe: string;
}

export interface StreamCursor {
  streamId: string;
  lastSeq: number;
}

export interface ProviderStatus {
  phase: "idle" | "syncing" | "live" | "resyncing" | "closed" | "error";
  cursor?: StreamCursor;
  reason?: string;
}

export type ProviderEvent =
  | { type: "bars"; bars: QNextBar[] }
  | { type: "status"; status: ProviderStatus };
