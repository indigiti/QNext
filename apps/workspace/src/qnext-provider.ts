export interface QNextProviderOptions {
  apiBase?: string;
  streamUrl?: string;
  fetchImpl?: typeof fetch;
  webSocketFactory?: (url: string) => WebSocketLike;
  reconnectDelayMs?: number;
  pollIntervalMs?: number;
}

export interface QNextRange {
  from?: number;
  to?: number;
  limit?: number;
  session?: string;
}

export interface QNextBar {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume?: number;
}

export interface QNextTransportSnapshot {
  mode: 'idle' | 'connecting' | 'wss' | 'rest_fallback';
  sockets: number;
  streams: number;
  consumers: number;
  polling_streams: number;
}

interface QNextSymbol {
  instrument_id: string;
  ticker: string;
  description: string;
  type: string;
  prefix: string;
  currency: string;
  timezone: string;
  calendar_id: string;
  synthetic: boolean;
}

interface SymbolsPayload {
  symbols: QNextSymbol[];
}

interface BarsPayload {
  bars: Array<QNextBar & {
    final?: boolean;
    revision?: number;
    quality?: string;
    authority_provider?: string;
  }>;
}

interface CalendarPayload {
  calendar_id: string;
  version: string;
  timezone: string;
  session: string;
  windows: Array<[number, number]>;
}

interface StreamMessage {
  op?: string;
  stream_id?: string;
  seq?: number;
  symbol?: string;
  timeframe?: string;
  bar?: QNextBar;
}

interface WebSocketLike {
  readyState: number;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  send(data: string): void;
  close(): void;
}

interface SharedBarSubscription {
  key: string;
  ticker: string;
  instrumentID: string;
  timeframe: string;
  callbacks: Set<(bar: QNextBar) => void>;
  streamID: string;
  lastSeq: number;
  needsSnapshot: boolean;
  healing: boolean;
  polling: boolean;
  pollFailures: number;
  lastPollSignature: string;
  pollTimer?: ReturnType<typeof setTimeout>;
}

const WS_OPEN = 1;

export class QNextProvider {
  private readonly apiBase: string;
  private readonly explicitStreamUrl?: string;
  private readonly fetchImpl: typeof fetch;
  private readonly webSocketFactory: (url: string) => WebSocketLike;
  private readonly reconnectDelayMs: number;
  private readonly pollIntervalMs: number;
  private symbolsPromise?: Promise<QNextSymbol[]>;

  private socket?: WebSocketLike;
  private socketConnected = false;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private readonly subscriptions = new Map<string, SharedBarSubscription>();
  private readonly streamToKey = new Map<string, string>();

  constructor(options: QNextProviderOptions = {}) {
    this.apiBase = (options.apiBase ?? '').replace(/\/+$/, '');
    this.explicitStreamUrl = options.streamUrl;
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
    this.webSocketFactory =
      options.webSocketFactory ??
      ((url: string) => new WebSocket(url) as unknown as WebSocketLike);
    this.reconnectDelayMs = options.reconnectDelayMs ?? 1_000;
    this.pollIntervalMs = options.pollIntervalMs ?? 250;
  }

  async listSymbols() {
    const symbols = await this.loadSymbols();
    const order = new Map(
      [
        'NIFTY',
        'NIFTY-SYN',
        'BANKNIFTY',
        'BANKNIFTY-SYN',
        'MIDCPNIFTY',
        'MIDCPNIFTY-SYN',
        'FINNIFTY',
        'FINNIFTY-SYN',
        'SENSEX',
        'SENSEX-SYN',
        'BANKEX',
        'BANKEX-SYN',
      ].map((ticker, index) => [ticker, index]),
    );

    return [...symbols]
      .sort((a, b) => {
        const aRank = order.get(a.ticker.toUpperCase()) ?? Number.MAX_SAFE_INTEGER;
        const bRank = order.get(b.ticker.toUpperCase()) ?? Number.MAX_SAFE_INTEGER;
        if (aRank !== bRank) {
          return aRank - bRank;
        }
        return a.ticker.localeCompare(b.ticker);
      })
      .map((symbol) => ({
        ticker: symbol.ticker,
        description: symbol.description,
        type: symbol.type,
        prefix: symbol.prefix,
      }));
  }

  async getBars(
    ticker: string,
    timeframe: string,
    range: QNextRange = {},
  ): Promise<QNextBar[]> {
    const instrument = await this.resolveInstrument(ticker);
    const to = range.to ?? Date.now();
    const limit = range.limit ?? 500;
    const from =
      range.from ??
      to - defaultHistoryLookbackMs(timeframe, Math.max(limit, 1));

    const query = new URLSearchParams({
      instrument_id: instrument.instrument_id,
      timeframe,
      from_ms: String(from),
      to_ms: String(to),
    });

    const response = await this.fetchImpl(
      this.endpoint(`/api/v1/bars/?${query.toString()}`),
    );
    if (!response.ok) {
      throw new Error(`QNext bars request failed: HTTP ${response.status}`);
    }

    const payload = (await response.json()) as BarsPayload;
    const bars = normalizeBars(payload.bars ?? []);
    return limit > 0 && bars.length > limit ? bars.slice(-limit) : bars;
  }

  async getCalendar(
    ticker: string,
    range: QNextRange = {},
  ): Promise<Array<[number, number]>> {
    const instrument = await this.resolveInstrument(ticker);
    const to = range.to ?? Date.now();
    const from =
      range.from ??
      to - 32 * 24 * 60 * 60 * 1_000;

    const query = new URLSearchParams({
      instrument_id: instrument.instrument_id,
      from_ms: String(from),
      to_ms: String(to),
      session: normalizeSession(range.session),
    });

    const response = await this.fetchImpl(
      this.endpoint(`/api/v1/calendar/?${query.toString()}`),
    );
    if (!response.ok) {
      throw new Error(`QNext calendar request failed: HTTP ${response.status}`);
    }

    const payload = (await response.json()) as CalendarPayload;
    return [...(payload.windows ?? [])].sort((a, b) => a[0] - b[0]);
  }

  subscribe(
    ticker: string,
    timeframe: string,
    onBar: (bar: QNextBar) => void,
    _options?: { session?: string },
  ): () => void {
    let cancelled = false;
    let attached: SharedBarSubscription | undefined;

    void this.resolveInstrument(ticker)
      .then((instrument) => {
        if (cancelled) {
          return;
        }

        const key = subscriptionKey(instrument.instrument_id, timeframe);
        let subscription = this.subscriptions.get(key);
        if (!subscription) {
          subscription = {
            key,
            ticker,
            instrumentID: instrument.instrument_id,
            timeframe,
            callbacks: new Set(),
            streamID: '',
            lastSeq: 0,
            needsSnapshot: false,
            healing: false,
            polling: false,
            pollFailures: 0,
            lastPollSignature: '',
          };
          this.subscriptions.set(key, subscription);
        }

        subscription.callbacks.add(onBar);
        attached = subscription;

        if (this.socketConnected) {
          if (!subscription.streamID) {
            this.freshSubscribe(subscription);
          }
          return;
        }
        this.connectSharedSocket();
      })
      .catch((error: unknown) => {
        console.error('QNext live subscription failed', error);
      });

    return () => {
      cancelled = true;
      if (!attached) {
        return;
      }

      attached.callbacks.delete(onBar);
      if (attached.callbacks.size > 0) {
        return;
      }

      this.stopPolling(attached);
      if (attached.streamID) {
        this.send({ op: 'unsubscribe', stream_id: attached.streamID });
        this.streamToKey.delete(attached.streamID);
      }
      this.subscriptions.delete(attached.key);
      this.closeSharedSocketIfIdle();
    };
  }

  transportSnapshot(): QNextTransportSnapshot {
    const streams = this.subscriptions.size;
    const consumers = [...this.subscriptions.values()].reduce(
      (total, subscription) => total + subscription.callbacks.size,
      0,
    );
    const pollingStreams = [...this.subscriptions.values()].filter(
      (subscription) => subscription.polling,
    ).length;

    const mode: QNextTransportSnapshot['mode'] =
      streams === 0
        ? 'idle'
        : this.socketConnected
          ? 'wss'
          : this.socket
            ? 'connecting'
            : 'rest_fallback';

    return {
      mode,
      sockets: this.socket ? 1 : 0,
      streams,
      consumers,
      polling_streams: pollingStreams,
    };
  }

  private connectSharedSocket(): void {
    if (this.socket || this.subscriptions.size === 0) {
      return;
    }

    let socket: WebSocketLike;
    try {
      socket = this.webSocketFactory(this.streamURL());
    } catch (error) {
      console.warn('QNext WebSocket unavailable; falling back to REST polling', error);
      this.startPollingAll();
      this.scheduleReconnect();
      return;
    }

    this.socket = socket;
    this.socketConnected = false;

    socket.onopen = () => {
      if (this.socket !== socket) {
        return;
      }
      this.socketConnected = true;
      this.clearReconnectTimer();
      for (const subscription of this.subscriptions.values()) {
        this.stopPolling(subscription);
        if (subscription.needsSnapshot) {
          void this.healAndSubscribe(subscription);
        } else if (subscription.streamID) {
          this.send({
            op: 'resume',
            stream_id: subscription.streamID,
            after_seq: subscription.lastSeq,
          });
        } else {
          this.freshSubscribe(subscription);
        }
      }
    };

    socket.onmessage = (event) => {
      if (this.socket !== socket || typeof event.data !== 'string') {
        return;
      }

      let message: StreamMessage;
      try {
        message = JSON.parse(event.data) as StreamMessage;
      } catch {
        return;
      }
      this.handleStreamMessage(message);
    };

    socket.onerror = () => {
      if (this.socket === socket) {
        socket.close();
      }
    };

    socket.onclose = () => {
      if (this.socket !== socket) {
        return;
      }
      this.socket = undefined;
      this.socketConnected = false;
      if (this.subscriptions.size === 0) {
        return;
      }
      this.startPollingAll();
      this.scheduleReconnect();
    };
  }

  private handleStreamMessage(message: StreamMessage): void {
    switch (message.op) {
      case 'subscribed': {
        let subscription: SharedBarSubscription | undefined;
        if (message.symbol && message.timeframe) {
          subscription = this.subscriptions.get(
            subscriptionKey(message.symbol, message.timeframe),
          );
        }
        if (!subscription && message.stream_id) {
          const key = this.streamToKey.get(message.stream_id);
          if (key) {
            subscription = this.subscriptions.get(key);
          }
        }
        if (!subscription && !message.symbol && !message.timeframe) {
          const unbound = [...this.subscriptions.values()].filter(
            (candidate) => !candidate.streamID,
          );
          if (unbound.length === 1) {
            subscription = unbound[0];
          }
        }
        if (!subscription || !message.stream_id) {
          return;
        }

        if (subscription.streamID && subscription.streamID !== message.stream_id) {
          this.streamToKey.delete(subscription.streamID);
        }
        subscription.streamID = message.stream_id;
        this.streamToKey.set(message.stream_id, subscription.key);
        if (typeof message.seq === 'number') {
          subscription.lastSeq = Math.max(subscription.lastSeq, message.seq);
        }
        subscription.needsSnapshot = false;
        this.stopPolling(subscription);
        return;
      }

      case 'update': {
        if (!message.stream_id || !message.bar || typeof message.seq !== 'number') {
          return;
        }
        const key = this.streamToKey.get(message.stream_id);
        const subscription = key ? this.subscriptions.get(key) : undefined;
        if (!subscription || message.seq <= subscription.lastSeq) {
          return;
        }
        subscription.lastSeq = message.seq;
        this.deliver(subscription, normalizeBar(message.bar));
        return;
      }

      case 'resync_required': {
        if (!message.stream_id) {
          return;
        }
        const key = this.streamToKey.get(message.stream_id);
        const subscription = key ? this.subscriptions.get(key) : undefined;
        if (!subscription) {
          return;
        }
        subscription.needsSnapshot = true;
        void this.healAndSubscribe(subscription);
        return;
      }
    }
  }

  private freshSubscribe(subscription: SharedBarSubscription): void {
    this.send({
      op: 'subscribe',
      channel: 'bars',
      symbol: subscription.instrumentID,
      timeframe: subscription.timeframe,
    });
  }

  private async healAndSubscribe(subscription: SharedBarSubscription): Promise<void> {
    if (subscription.healing || subscription.callbacks.size === 0) {
      return;
    }
    subscription.healing = true;
    subscription.needsSnapshot = true;

    try {
      const snapshot = await this.getBars(subscription.ticker, subscription.timeframe, {
        limit: 500,
      });
      if (!this.subscriptions.has(subscription.key)) {
        return;
      }
      for (const bar of snapshot) {
        this.deliver(subscription, bar);
      }

      if (subscription.streamID) {
        this.streamToKey.delete(subscription.streamID);
      }
      subscription.streamID = '';
      subscription.lastSeq = 0;
      subscription.needsSnapshot = false;
      this.stopPolling(subscription);
      if (this.socketConnected) {
        this.freshSubscribe(subscription);
      }
    } catch (error) {
      if (this.subscriptions.has(subscription.key)) {
        console.error('QNext stream resync snapshot failed', error);
        this.startPolling(subscription);
      }
    } finally {
      subscription.healing = false;
    }
  }

  private deliver(subscription: SharedBarSubscription, bar: QNextBar): void {
    for (const callback of subscription.callbacks) {
      callback(bar);
    }
  }

  private startPollingAll(): void {
    for (const subscription of this.subscriptions.values()) {
      this.startPolling(subscription);
    }
  }

  private startPolling(subscription: SharedBarSubscription): void {
    if (subscription.polling || subscription.callbacks.size === 0) {
      return;
    }
    subscription.polling = true;
    subscription.pollFailures = 0;

    const poll = async () => {
      if (!subscription.polling || !this.subscriptions.has(subscription.key)) {
        return;
      }
      try {
        const to = Date.now();
        const bars = await this.getBars(subscription.ticker, subscription.timeframe, {
          from: to - livePollingLookbackMs(subscription.timeframe, 2),
          to,
          limit: 2,
        });
        subscription.pollFailures = 0;
        const latest = bars.at(-1);
        if (latest) {
          const signature = [
            latest.time,
            latest.open,
            latest.high,
            latest.low,
            latest.close,
            latest.volume ?? '',
          ].join(':');
          if (signature !== subscription.lastPollSignature) {
            subscription.lastPollSignature = signature;
            this.deliver(subscription, latest);
          }
        }
        if (this.socketConnected && subscription.needsSnapshot) {
          void this.healAndSubscribe(subscription);
        }
      } catch (error) {
        subscription.pollFailures += 1;
        if (subscription.pollFailures === 1 || subscription.pollFailures % 5 === 0) {
          console.error('QNext live polling failed', error);
        }
      } finally {
        if (subscription.polling && this.subscriptions.has(subscription.key)) {
          const retryDelay =
            subscription.pollFailures === 0
              ? this.pollIntervalMs
              : Math.min(
                  10_000,
                  Math.max(
                    1_000,
                    this.pollIntervalMs *
                      2 ** Math.min(subscription.pollFailures, 6),
                  ),
                );
          subscription.pollTimer = setTimeout(poll, retryDelay);
        }
      }
    };

    void poll();
  }

  private stopPolling(subscription: SharedBarSubscription): void {
    subscription.polling = false;
    subscription.pollFailures = 0;
    if (subscription.pollTimer !== undefined) {
      clearTimeout(subscription.pollTimer);
      subscription.pollTimer = undefined;
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer !== undefined || this.subscriptions.size === 0) {
      return;
    }
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = undefined;
      this.connectSharedSocket();
    }, this.reconnectDelayMs);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== undefined) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
  }

  private closeSharedSocketIfIdle(): void {
    if (this.subscriptions.size !== 0) {
      return;
    }
    this.clearReconnectTimer();
    const socket = this.socket;
    this.socket = undefined;
    this.socketConnected = false;
    socket?.close();
  }

  private send(message: Record<string, unknown>): void {
    if (this.socketConnected && this.socket?.readyState === WS_OPEN) {
      this.socket.send(JSON.stringify(message));
    }
  }

  private endpoint(path: string): string {
    return `${this.apiBase}${path}`;
  }

  private streamURL(): string {
    if (this.explicitStreamUrl) {
      return this.explicitStreamUrl;
    }

    const fallbackBase =
      typeof globalThis.location !== 'undefined'
        ? globalThis.location.href
        : 'http://127.0.0.1/';
    const url = new URL(this.endpoint('/api/v1/stream'), fallbackBase);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.toString();
  }

  private async loadSymbols(): Promise<QNextSymbol[]> {
    if (!this.symbolsPromise) {
      this.symbolsPromise = this.fetchImpl(this.endpoint('/api/v1/symbols/'))
        .then(async (response) => {
          if (!response.ok) {
            throw new Error(
              `QNext symbols request failed: HTTP ${response.status}`,
            );
          }
          const payload = (await response.json()) as SymbolsPayload;
          return payload.symbols ?? [];
        })
        .catch((error) => {
          this.symbolsPromise = undefined;
          throw error;
        });
    }
    return this.symbolsPromise;
  }

  private async resolveInstrument(ticker: string): Promise<QNextSymbol> {
    const symbols = await this.loadSymbols();
    const requested = ticker.trim().toUpperCase();

    const direct = symbols.find(
      (symbol) => symbol.instrument_id.toUpperCase() === requested,
    );
    if (direct) {
      return direct;
    }

    const separator = requested.indexOf(':');
    const prefix = separator >= 0 ? requested.slice(0, separator) : '';
    const bareTicker = separator >= 0 ? requested.slice(separator + 1) : requested;

    const resolved = symbols.find((symbol) => {
      if (symbol.ticker.toUpperCase() !== bareTicker) {
        return false;
      }
      return !prefix || symbol.prefix.toUpperCase() === prefix;
    });
    if (!resolved) {
      throw new Error(`QNext symbol is not registered: ${ticker}`);
    }
    return resolved;
  }
}

export function normalizeBars(bars: QNextBar[]): QNextBar[] {
  const byTime = new Map<number, QNextBar>();
  for (const raw of bars) {
    const bar = normalizeBar(raw);
    if (
      Number.isFinite(bar.time) &&
      Number.isFinite(bar.open) &&
      Number.isFinite(bar.high) &&
      Number.isFinite(bar.low) &&
      Number.isFinite(bar.close)
    ) {
      byTime.set(bar.time, bar);
    }
  }
  return [...byTime.values()].sort((a, b) => a.time - b.time);
}

function normalizeBar(bar: QNextBar): QNextBar {
  return {
    time: Number(bar.time),
    open: Number(bar.open),
    high: Number(bar.high),
    low: Number(bar.low),
    close: Number(bar.close),
    ...(bar.volume === undefined ? {} : { volume: Number(bar.volume) }),
  };
}

function subscriptionKey(instrumentID: string, timeframe: string): string {
  return `${instrumentID.trim().toUpperCase()}|${timeframe.trim()}`;
}

function defaultHistoryLookbackMs(timeframe: string, limit: number): number {
  const normalized = timeframe.trim();
  const requested = rawHistoryLookbackMs(normalized, limit);
  const day = 86_400_000;
  const unit = normalized.slice(-1);

  // 15s/30s bars are formed locally from the canonical 5s stream. A purely
  // wall-clock lookback can land entirely inside a weekend/holiday and return
  // no seed bars even though valid sub-minute history exists in the store.
  // Keep enough calendar padding to cross normal non-trading gaps while the
  // caller still trims the response to the requested bar count.
  const floor =
    normalized === '15s' || normalized === '30s'
      ? 7 * day
      : 0;

  const cap =
    unit === 's'
      ? 14 * day
      : unit === 'm'
        ? 31 * day
        : unit === 'h'
          ? 90 * day
          : 366 * day;

  return Math.min(Math.max(requested, floor), cap);
}

function livePollingLookbackMs(timeframe: string, limit: number): number {
  // REST fallback runs frequently, so do not apply the multi-day chart-history
  // padding here. Poll only a small recent window for the latest forming bar.
  return rawHistoryLookbackMs(timeframe, limit);
}

function rawHistoryLookbackMs(timeframe: string, limit: number): number {
  return timeframeDurationMs(timeframe) * Math.max(limit, 1) * 8;
}

function timeframeDurationMs(timeframe: string): number {
  const normalized = timeframe.trim();
  const match = normalized.match(/^(\d+)(s|m|h|D|W|M)$/);
  if (!match) {
    throw new Error(`Unsupported QNext timeframe: ${timeframe}`);
  }

  const value = Number(match[1]);
  const unit = match[2];
  const day = 86_400_000;
  const multiplier =
    unit === 's'
      ? 1_000
      : unit === 'm'
        ? 60_000
        : unit === 'h'
          ? 3_600_000
          : unit === 'D'
            ? day
            : unit === 'W'
              ? 7 * day
              : 30 * day;
  return value * multiplier;
}

function normalizeSession(session: string | undefined): 'regular' | 'extended' {
  return session?.toLowerCase() === 'extended' ? 'extended' : 'regular';
}
