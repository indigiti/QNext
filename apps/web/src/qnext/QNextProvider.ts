import { CanonicalBarBook } from "./reconcile.js";
import type { ProviderEvent, ProviderStatus, QNextBar, StreamCursor, Subscription } from "./types.js";

export interface WebSocketLike {
  readyState: number;
  send(data: string): void;
  close(): void;
  addEventListener(type: "open" | "message" | "close" | "error", listener: (event: any) => void): void;
}

export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;
export type WebSocketFactory = (url: string) => WebSocketLike;

export interface QNextProviderOptions {
  restBaseUrl: string;
  streamUrl: string;
  fetch?: FetchLike;
  webSocketFactory?: WebSocketFactory;
}

type Listener = (event: ProviderEvent) => void;

const WS_OPEN = 1;

export class QNextProvider {
  readonly #restBaseUrl: string;
  readonly #streamUrl: string;
  readonly #fetch: FetchLike;
  readonly #socketFactory: WebSocketFactory;
  readonly #book = new CanonicalBarBook();
  readonly #listeners = new Set<Listener>();

  #socket?: WebSocketLike;
  #subscription?: Subscription;
  #cursor?: StreamCursor;
  #status: ProviderStatus = { phase: "idle" };
  #resyncGeneration = 0;

  constructor(options: QNextProviderOptions) {
    this.#restBaseUrl = options.restBaseUrl.replace(/\/$/, "");
    this.#streamUrl = options.streamUrl;
    this.#fetch = options.fetch ?? fetch.bind(globalThis);
    this.#socketFactory = options.webSocketFactory ?? ((url) => new WebSocket(url));
  }

  subscribe(listener: Listener): () => void {
    this.#listeners.add(listener);
    listener({ type: "status", status: this.#status });
    return () => this.#listeners.delete(listener);
  }

  bars(): QNextBar[] {
    return this.#book.snapshot();
  }

  status(): ProviderStatus {
    return this.#status;
  }

  async start(subscription: Subscription): Promise<void> {
    this.#subscription = subscription;
    await this.#syncHistory("syncing");
    this.#openSocket(false);
  }

  reconnect(): void {
    if (!this.#subscription) throw new Error("provider has not been started");
    this.#socket?.close();
    this.#openSocket(Boolean(this.#cursor));
  }

  close(): void {
    this.#socket?.close();
    this.#setStatus({ phase: "closed", cursor: this.#cursor });
  }

  async #syncHistory(phase: "syncing" | "resyncing"): Promise<void> {
    const subscription = this.#subscription;
    if (!subscription) throw new Error("missing subscription");
    const generation = ++this.#resyncGeneration;
    this.#setStatus({ phase, cursor: this.#cursor });

    const url = new URL(`${this.#restBaseUrl}/api/v1/bars`);
    url.searchParams.set("symbol", subscription.symbol);
    url.searchParams.set("timeframe", subscription.timeframe);

    const response = await this.#fetch(url.toString(), { method: "GET" });
    if (!response.ok) throw new Error(`history request failed: ${response.status}`);
    const payload = await response.json() as unknown;
    const bars = parseHistory(payload, subscription);
    if (generation !== this.#resyncGeneration) return;

    this.#book.replaceHistory(bars);
    this.#emit({ type: "bars", bars: this.#book.snapshot() });
  }

  #openSocket(preferResume: boolean): void {
    const subscription = this.#subscription;
    if (!subscription) throw new Error("missing subscription");

    const socket = this.#socketFactory(this.#streamUrl);
    this.#socket = socket;

    socket.addEventListener("open", () => {
      socket.send(JSON.stringify({ op: "hello", protocol: "QNEXT.STREAM/1" }));
      if (preferResume && this.#cursor) {
        socket.send(JSON.stringify({
          op: "resume",
          stream_id: this.#cursor.streamId,
          after_seq: this.#cursor.lastSeq,
        }));
      } else {
        socket.send(JSON.stringify({
          op: "subscribe",
          channel: "bars",
          symbol: subscription.symbol,
          timeframe: subscription.timeframe,
        }));
      }
    });

    socket.addEventListener("message", (event) => {
      void this.#handleMessage(String(event.data)).catch((error: unknown) => {
        this.#setStatus({ phase: "error", cursor: this.#cursor, reason: errorMessage(error) });
      });
    });

    socket.addEventListener("close", () => {
      if (this.#status.phase !== "closed" && this.#status.phase !== "error") {
        this.#setStatus({ phase: "idle", cursor: this.#cursor, reason: "socket_closed" });
      }
    });

    socket.addEventListener("error", () => {
      this.#setStatus({ phase: "error", cursor: this.#cursor, reason: "socket_error" });
    });
  }

  async #handleMessage(raw: string): Promise<void> {
    const message = JSON.parse(raw) as Record<string, unknown>;
    const op = String(message.op ?? "");

    if (op === "subscribed") {
      const streamId = requiredString(message.stream_id, "stream_id");
      this.#cursor = { streamId, lastSeq: 0 };
      this.#setStatus({ phase: "live", cursor: this.#cursor });
      return;
    }

    if (op === "resync_required") {
      await this.#syncHistory("resyncing");
      this.#cursor = undefined;
      if (this.#socket?.readyState === WS_OPEN && this.#subscription) {
        this.#socket.send(JSON.stringify({
          op: "subscribe",
          channel: "bars",
          symbol: this.#subscription.symbol,
          timeframe: this.#subscription.timeframe,
        }));
      }
      return;
    }

    if (message.channel === "bars") {
      await this.#handleBar(message);
      return;
    }

    if (op === "error") {
      throw new Error(`${String(message.code ?? "STREAM_ERROR")}: ${String(message.message ?? "unknown")}`);
    }
  }

  async #handleBar(message: Record<string, unknown>): Promise<void> {
    const subscription = this.#subscription;
    if (!subscription) return;

    const streamId = requiredString(message.stream_id, "stream_id");
    const seq = requiredInteger(message.seq, "seq");
    if (this.#cursor && streamId === this.#cursor.streamId && seq !== this.#cursor.lastSeq + 1) {
      await this.#syncHistory("resyncing");
      this.#cursor = undefined;
      if (this.#socket?.readyState === WS_OPEN) {
        this.#socket.send(JSON.stringify({
          op: "subscribe",
          channel: "bars",
          symbol: subscription.symbol,
          timeframe: subscription.timeframe,
        }));
      }
      return;
    }

    const incoming = parseStreamBar(message, subscription);
    this.#cursor = { streamId, lastSeq: seq };
    if (this.#book.apply(incoming)) {
      this.#emit({ type: "bars", bars: this.#book.snapshot() });
    }
    this.#setStatus({ phase: "live", cursor: this.#cursor });
  }

  #setStatus(status: ProviderStatus): void {
    this.#status = status;
    this.#emit({ type: "status", status });
  }

  #emit(event: ProviderEvent): void {
    for (const listener of this.#listeners) listener(event);
  }
}

function parseHistory(payload: unknown, subscription: Subscription): QNextBar[] {
  const rows = Array.isArray(payload) ? payload : (
    payload && typeof payload === "object" && Array.isArray((payload as any).bars)
      ? (payload as any).bars
      : []
  );
  return rows.map((row: any) => ({
    instrumentId: String(row.instrument_id ?? row.symbol ?? subscription.symbol),
    timeframe: String(row.timeframe ?? subscription.timeframe),
    openTimeMs: Number(row.open_time_ms ?? row.time),
    open: Number(row.open),
    high: Number(row.high),
    low: Number(row.low),
    close: Number(row.close),
    volume: Number(row.volume ?? 0),
    final: Boolean(row.final ?? true),
    revision: Number(row.revision ?? 0),
    quality: String(row.quality ?? "GOOD"),
  }));
}

function parseStreamBar(message: Record<string, unknown>, subscription: Subscription): QNextBar {
  const bar = message.bar as Record<string, unknown>;
  if (!bar || typeof bar !== "object") throw new Error("stream bar payload is missing");
  return {
    instrumentId: String(message.symbol ?? subscription.symbol),
    timeframe: String(message.timeframe ?? subscription.timeframe),
    openTimeMs: Number(bar.time),
    open: Number(bar.open),
    high: Number(bar.high),
    low: Number(bar.low),
    close: Number(bar.close),
    volume: Number(bar.volume ?? 0),
    final: Boolean(bar.final),
    revision: Number(bar.revision ?? 0),
    quality: String(message.quality ?? "GOOD"),
  };
}

function requiredString(value: unknown, field: string): string {
  if (typeof value !== "string" || !value) throw new Error(`missing ${field}`);
  return value;
}

function requiredInteger(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isInteger(value)) throw new Error(`invalid ${field}`);
  return value;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
