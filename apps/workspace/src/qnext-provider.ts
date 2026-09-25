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

const WS_OPEN = 1;

export class QNextProvider {
  private readonly apiBase: string;
  private readonly explicitStreamUrl?: string;
  private readonly fetchImpl: typeof fetch;
  private readonly webSocketFactory: (url: string) => WebSocketLike;
  private readonly reconnectDelayMs: number;
  private readonly pollIntervalMs: number;
  private symbolsPromise?: Promise<QNextSymbol[]>;

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
    let socket: WebSocketLike | undefined;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    let pollTimer: ReturnType<typeof setTimeout> | undefined;
    let polling = false;
    let streamID = '';
    let lastSeq = 0;
    let resuming = false;
    let needsSnapshot = false;
    let healing = false;
    let socketOpened = false;
    let lastPollSignature = '';

    const send = (message: Record<string, unknown>) => {
      if (socket?.readyState === WS_OPEN) {
        socket.send(JSON.stringify(message));
      }
    };

    const freshSubscribe = (instrumentID: string) => {
      send({
        op: 'subscribe',
        channel: 'bars',
        symbol: instrumentID,
        timeframe,
      });
    };

    const healAndSubscribe = async (instrumentID: string) => {
      if (cancelled || healing) {
        return;
      }
      healing = true;
      needsSnapshot = true;
      streamID = '';
      lastSeq = 0;
      resuming = false;
      try {
        const snapshot = await this.getBars(ticker, timeframe, { limit: 500 });
        if (cancelled) {
          return;
        }
        for (const bar of snapshot) {
          onBar(bar);
        }
        needsSnapshot = false;
        freshSubscribe(instrumentID);
      } catch (error) {
        console.error('QNext stream resync snapshot failed', error);
        socket?.close();
      } finally {
        healing = false;
      }
    };

    const startPolling = () => {
      if (cancelled || polling) {
        return;
      }
      polling = true;

      const poll = async () => {
        if (cancelled || !polling) {
          return;
        }
        try {
          const bars = await this.getBars(ticker, timeframe, { limit: 2 });
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
            if (signature !== lastPollSignature) {
              lastPollSignature = signature;
              onBar(latest);
            }
          }
        } catch (error) {
          console.error('QNext live polling failed', error);
        } finally {
          if (!cancelled && polling) {
            pollTimer = setTimeout(poll, this.pollIntervalMs);
          }
        }
      };

      void poll();
    };

    const stopPolling = () => {
      polling = false;
      if (pollTimer !== undefined) {
        clearTimeout(pollTimer);
        pollTimer = undefined;
      }
    };

    const scheduleReconnect = (instrumentID: string) => {
      if (cancelled || reconnectTimer !== undefined) {
        return;
      }
      reconnectTimer = setTimeout(() => {
        reconnectTimer = undefined;
        connect(instrumentID);
      }, this.reconnectDelayMs);
    };

    const connect = (instrumentID: string) => {
      if (cancelled) {
        return;
      }

      try {
        socket = this.webSocketFactory(this.streamURL());
      } catch (error) {
        console.warn('QNext WebSocket unavailable; falling back to REST polling', error);
        startPolling();
        scheduleReconnect(instrumentID);
        return;
      }
      socketOpened = false;
      socket.onopen = () => {
        socketOpened = true;
        stopPolling();
        if (needsSnapshot) {
          void healAndSubscribe(instrumentID);
          return;
        }
        if (streamID) {
          resuming = true;
          send({
            op: 'resume',
            stream_id: streamID,
            after_seq: lastSeq,
          });
          return;
        }
        resuming = false;
        freshSubscribe(instrumentID);
      };

      socket.onmessage = (event) => {
        if (typeof event.data !== 'string') {
          return;
        }

        let message: StreamMessage;
        try {
          message = JSON.parse(event.data) as StreamMessage;
        } catch {
          return;
        }

        switch (message.op) {
          case 'subscribed':
            if (message.stream_id) {
              streamID = message.stream_id;
            }
            if (!resuming && typeof message.seq === 'number') {
              lastSeq = Math.max(lastSeq, message.seq);
            }
            resuming = false;
            break;

          case 'update':
            if (
              message.bar &&
              typeof message.seq === 'number' &&
              message.seq > lastSeq
            ) {
              lastSeq = message.seq;
              onBar(normalizeBar(message.bar));
            }
            break;

          case 'resync_required':
            needsSnapshot = true;
            void healAndSubscribe(instrumentID);
            break;
        }
      };

      socket.onerror = () => {
        socket?.close();
      };

      socket.onclose = () => {
        startPolling();
        scheduleReconnect(instrumentID);
      };
    };

    void this.resolveInstrument(ticker)
      .then((instrument) => connect(instrument.instrument_id))
      .catch((error: unknown) => {
        console.error('QNext live subscription failed', error);
      });

    return () => {
      cancelled = true;
      if (reconnectTimer !== undefined) {
        clearTimeout(reconnectTimer);
        reconnectTimer = undefined;
      }
      polling = false;
      if (pollTimer !== undefined) {
        clearTimeout(pollTimer);
        pollTimer = undefined;
      }
      if (streamID) {
        send({ op: 'unsubscribe', stream_id: streamID });
      }
      socket?.close();
    };
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

function defaultHistoryLookbackMs(timeframe: string, limit: number): number {
  const duration = timeframeDurationMs(timeframe);
  const requested = duration * limit * 8;
  const day = 86_400_000;
  const unit = timeframe.trim().slice(-1);

  const cap =
    unit === 's'
      ? 3 * day
      : unit === 'm'
        ? 31 * day
        : unit === 'h'
          ? 90 * day
          : 366 * day;

  return Math.min(requested, cap);
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
