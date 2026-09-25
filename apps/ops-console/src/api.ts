export type ServiceAction = 'start' | 'stop' | 'restart';

export interface Probe {
  ok: boolean;
  status?: number;
  error?: string;
  body?: unknown;
}

export interface OpsStatus {
  release: {
    current: string | null;
    available: string[];
    mode?: 'staged' | 'direct';
  };
  service: {
    ok: boolean;
    state: string;
    output?: string;
  };
  marketCore: {
    health: Probe;
    ready: Probe;
    version: Probe;
  };
  storageRoot: string;
  configPath: string;
  host: {
    processControl: boolean;
    cronControl: boolean;
    controlMode: 'direct' | 'cron' | 'setup';
    helperAvailable: boolean;
    helperPath: string;
    cronCommand: string;
  };
}

export interface RuntimeDiagnostics {
  desiredState: 'running' | 'stopped';
  pid: number | null;
  pidAlive: boolean;
  pidPath: string;
  binaryPath: string | null;
  binaryFound: boolean;
  logPath: string;
  logLines: string[];
  cronHeartbeatAt: string | null;
  cronHeartbeatAgeSeconds: number | null;
  controlMode: 'direct' | 'cron' | 'setup';
  helperPath: string;
}

export interface FeedProviderSnapshot {
  observed?: number;
  received?: number;
  accepted?: number;
  errors?: number;
  last_event_time_ms?: number;
  last_received_time_ms?: number;
  last_price?: number;
}

export interface FeedInstrumentSnapshot {
  provider?: string;
  price?: number;
  last_event_time_ms?: number;
  last_received_time_ms?: number;
  quality?: string;
  synthetic_version?: string;
}

export interface FeedStatusBody {
  live_configured: boolean;
  resilience_configured: boolean;
  nifty_instrument_id: string;
  synthetic_instrument_id: string;
  telemetry: {
    providers: Record<string, FeedProviderSnapshot>;
    instruments: Record<string, FeedInstrumentSnapshot>;
    synthetic?: {
      atm?: number;
      expiry?: string;
      generation?: number;
      pending_atm?: number;
      pending_expiry?: string;
      active_legs?: number;
      warm_subscriptions?: number;
    };
  };
  resilience: {
    active_authorities?: Record<string, string>;
    authority_states?: Record<string, string>;
    providers?: Record<string, FeedProviderSnapshot>;
  };
}

export interface FeedStatusResponse {
  ok: boolean;
  status?: number | null;
  error?: string;
  body?: FeedStatusBody;
}

export interface SecretWriteResult {
  stored: string[];
}

export interface APIOptions {
  base?: string;
  token?: string;
  fetcher?: typeof fetch;
}

export class OpsAPI {
  private readonly base: string;
  private readonly token?: string;
  private readonly fetcher: typeof fetch;

  constructor(options: APIOptions = {}) {
    this.base = (options.base ?? '/qnext/admin/api/index.php').replace(/\/$/, '');
    this.token = options.token;
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
  }

  setupStatus(): Promise<{ initialized: boolean }> {
    return this.request<{ initialized: boolean }>('/setup-status', {}, false);
  }

  initializeAdminToken(token: string): Promise<{ initialized: boolean }> {
    return this.request<{ initialized: boolean }>('/setup', {
      method: 'POST',
      body: JSON.stringify({ token }),
    }, false);
  }

  status(): Promise<OpsStatus> {
    return this.request<OpsStatus>('/status');
  }

  getConfig(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('/config');
  }

  diagnostics(): Promise<RuntimeDiagnostics> {
    return this.request<RuntimeDiagnostics>('/diagnostics');
  }

  feedStatus(): Promise<FeedStatusResponse> {
    return this.request<FeedStatusResponse>('/feed-status');
  }

  saveConfig(config: Record<string, unknown>): Promise<{ saved: boolean }> {
    return this.request('/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  }

  saveSecrets(secrets: Record<string, string>): Promise<SecretWriteResult> {
    return this.request('/secrets', {
      method: 'POST',
      body: JSON.stringify(secrets),
    });
  }

  service(action: ServiceAction): Promise<{ ok: boolean; action: string; output: string }> {
    return this.request(`/service/${action}`, { method: 'POST' });
  }

  smoke(): Promise<{ ok: boolean; probes: Record<string, Probe> }> {
    return this.request('/smoke', { method: 'POST' });
  }

  activateRelease(version: string): Promise<{ ok: boolean; version: string; output: string }> {
    return this.request(`/releases/${encodeURIComponent(version)}/activate`, { method: 'POST' });
  }

  rollback(): Promise<{ ok: boolean; output: string }> {
    return this.request('/rollback', { method: 'POST' });
  }

  private async request<T>(
    path: string,
    init: RequestInit = {},
    includeToken = true,
  ): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set('Accept', 'application/json');
    if (init.body) {
      headers.set('Content-Type', 'application/json');
    }
    if (includeToken && this.token) {
      headers.set('X-QNext-Ops-Token', this.token);
    }

    const separator = this.base.includes('?') ? '&' : '?';
    const url = this.base + separator + 'route=' + encodeURIComponent(path);
    const response = await this.fetcher(url, {
      ...init,
      headers,
      credentials: 'same-origin',
    });
    const text = await response.text();

    let payload: unknown = {};
    if (text) {
      try {
        payload = JSON.parse(text);
      } catch {
        const contentType = response.headers.get('Content-Type') ?? 'unknown';
        throw new Error(
          `Ops API returned non-JSON HTTP ${response.status} (${contentType}). Check /qnext/admin/api/index.php routing.`,
        );
      }
    }

    if (!response.ok) {
      const message =
        typeof payload === 'object' &&
        payload !== null &&
        'error' in payload &&
        typeof (payload as { error?: unknown }).error === 'string'
          ? (payload as { error: string }).error
          : `HTTP ${response.status}`;
      throw new Error(message);
    }
    return payload as T;
  }
}
