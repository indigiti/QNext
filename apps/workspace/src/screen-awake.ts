/**
 * A browser-local display wake lock. This never modifies chart lifecycle,
 * focus recovery, data transport or candle subscriptions.
 */
export interface ScreenAwakeLease {
  readonly released: boolean;
  release(): Promise<void>;
  addEventListener(type: 'release', listener: () => void): void;
}

export type ScreenAwakeEvent = 'visibilitychange' | 'pagehide' | 'pageshow' | 'pointerdown' | 'keydown';
export type ScreenAwakeStatus = 'off' | 'active' | 'requesting' | 'blocked' | 'unsupported' | 'idle';

export interface ScreenAwakeRuntime {
  visibility(): string;
  on(event: ScreenAwakeEvent, listener: () => void): () => void;
  request?: () => Promise<ScreenAwakeLease>;
  readPreference?: () => string | null;
  savePreference?: (value: 'on' | 'off') => void;
  defaultEnabled: boolean;
  now?: () => number;
}

const STORAGE_KEY = 'qnext-screen-awake-v1';

export class ScreenAwakeController {
  private enabledValue: boolean;
  private started = false;
  private pageActive = true;
  private lease: ScreenAwakeLease | null = null;
  private requesting = false;
  private blocked = false;
  private epoch = 0;
  private lastGestureAttempt = Number.NEGATIVE_INFINITY;
  private readonly disposers: Array<() => void> = [];
  private readonly listeners = new Set<() => void>();

  constructor(private readonly runtime: ScreenAwakeRuntime) {
    let preference: string | null = null;
    try {
      preference = runtime.readPreference?.() ?? null;
    } catch {
      // Private browsing can disable storage; use the device default.
    }
    this.enabledValue = preference === 'on' || (preference !== 'off' && runtime.defaultEnabled);
  }

  get enabled(): boolean { return this.enabledValue; }

  get status(): ScreenAwakeStatus {
    if (!this.enabledValue) return 'off';
    if (!this.runtime.request) return 'unsupported';
    if (this.lease && !this.lease.released) return 'active';
    if (this.requesting) return 'requesting';
    return this.blocked ? 'blocked' : 'idle';
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    listener();
    return () => this.listeners.delete(listener);
  }

  private emit(): void {
    for (const listener of this.listeners) listener();
  }

  start(): void {
    if (this.started) return;
    this.started = true;
    this.disposers.push(
      this.runtime.on('visibilitychange', () => {
        if (this.runtime.visibility() === 'visible') this.acquire(true);
        else this.invalidateAndRelease();
      }),
      this.runtime.on('pagehide', () => {
        this.pageActive = false;
        this.invalidateAndRelease();
      }),
      this.runtime.on('pageshow', () => {
        this.pageActive = true;
        this.acquire(true);
      }),
      this.runtime.on('pointerdown', () => this.onGesture()),
      this.runtime.on('keydown', () => this.onGesture()),
    );
    this.acquire(true);
  }

  stop(): void {
    if (!this.started) return;
    this.started = false;
    for (const dispose of this.disposers.splice(0)) dispose();
    this.invalidateAndRelease();
  }

  toggle(): void { this.setEnabled(!this.enabledValue); }

  setEnabled(value: boolean): void {
    if (this.enabledValue === value) return;
    this.enabledValue = value;
    try {
      this.runtime.savePreference?.(value ? 'on' : 'off');
    } catch {
      // Device/storage restrictions should never prevent a session toggle.
    }
    ++this.epoch;
    this.blocked = false;
    if (!value) this.releaseLease();
    else this.acquire(true); // Called directly from the toolbar tap when enabled.
    this.emit();
  }

  private onGesture(): void {
    if (!this.enabledValue || this.lease || this.requesting) return;
    const now = this.runtime.now?.() ?? Date.now();
    if (now - this.lastGestureAttempt < 2500) return;
    this.lastGestureAttempt = now;
    this.acquire(true);
  }

  private isEligible(): boolean {
    return this.started
      && this.pageActive
      && this.enabledValue
      && this.runtime.visibility() === 'visible';
  }

  private releaseLease(): void {
    const lease = this.lease;
    this.lease = null;
    if (lease && !lease.released) void lease.release().catch(() => undefined);
    this.emit();
  }

  private invalidateAndRelease(): void {
    ++this.epoch;
    this.releaseLease();
  }

  private acquire(_fromInteraction = false): void {
    if (!this.isEligible() || !this.runtime.request || this.requesting || this.lease) return;
    const epoch = this.epoch;
    this.requesting = true;
    this.blocked = false;
    this.emit();

    // The call is synchronous up to request(), preserving user activation on iOS.
    let pending: Promise<ScreenAwakeLease>;
    try {
      pending = this.runtime.request();
    } catch (error) {
      pending = Promise.reject(error);
    }

    void pending.then((lease) => {
      if (epoch !== this.epoch || !this.isEligible()) {
        if (!lease.released) void lease.release().catch(() => undefined);
        return;
      }
      this.lease = lease;
      lease.addEventListener('release', () => {
        if (this.lease !== lease) return;
        this.lease = null;
        this.emit();
        // No immediate retry loop: visibility restoration or a gesture retries.
      });
      this.emit();
    }).catch(() => {
      if (epoch === this.epoch) {
        this.blocked = true;
        this.emit();
      }
    }).finally(() => {
      this.requesting = false;
      this.emit();
      // If a request finished after a hide/reopen or off/on race, retry safely.
      if (epoch !== this.epoch && this.isEligible()) this.acquire(true);
    });
  }
}

export function screenAwakeText(status: ScreenAwakeStatus): string {
  switch (status) {
    case 'active': return 'Screen Awake: On';
    case 'off': return 'Screen Awake: Off';
    case 'requesting': return 'Screen Awake: Activating';
    case 'blocked': return 'Screen Awake: Restricted by device or browser';
    case 'unsupported': return 'Screen Awake: Not supported';
    default: return 'Screen Awake: Waiting for browser';
  }
}

/** Shared entry point for the Vela and Lightweight chart toolbars. */
export function createScreenAwakeController(): ScreenAwakeController {
  const supported = typeof navigator.wakeLock?.request === 'function';
  const isMobile = typeof window.matchMedia === 'function'
    && window.matchMedia('(pointer: coarse) and (max-width: 920px)').matches;

  return new ScreenAwakeController({
    defaultEnabled: isMobile,
    visibility: () => document.visibilityState,
    on: (event, listener) => {
      const target: Document | Window =
        event === 'pagehide' || event === 'pageshow' ? window : document;
      target.addEventListener(event, listener);
      return () => target.removeEventListener(event, listener);
    },
    request: supported ? () => navigator.wakeLock.request('screen') : undefined,
    readPreference: () => window.localStorage.getItem(STORAGE_KEY),
    savePreference: (value) => window.localStorage.setItem(STORAGE_KEY, value),
  });
}

export function mountScreenAwakeButton(
  container: HTMLElement,
  controller: ScreenAwakeController,
): () => void {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'qnext-screen-awake-trigger';
  button.innerHTML = '<svg viewBox="0 0 24 24" width="19" height="19" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="4" y="3" width="16" height="15" rx="2"/><path d="M9 22h6M12 18v4M9 10l2 2 4-4"/></svg>';
  button.addEventListener('click', () => controller.toggle());
  const unsubscribe = controller.subscribe(() => {
    button.dataset.status = controller.status;
    button.setAttribute('aria-pressed', String(controller.enabled));
    button.setAttribute('aria-label', screenAwakeText(controller.status));
    button.title = screenAwakeText(controller.status);
  });
  container.append(button);
  return () => { unsubscribe(); button.remove(); };
}
