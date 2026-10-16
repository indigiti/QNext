// Browser timers and WebSocket delivery can be throttled while a tab is hidden.
// Reconcile from Market Core when the user returns instead of guessing candles.
export interface ResumeEventTarget {
  addEventListener(type: string, listener: EventListener): void;
  removeEventListener(type: string, listener: EventListener): void;
}

export interface VisibilityEventTarget extends ResumeEventTarget {
  readonly visibilityState: string;
}

export interface ChartResumeOptions {
  documentTarget?: VisibilityEventTarget;
  windowTarget?: ResumeEventTarget;
  now?: () => number;
  minInactiveMs?: number;
}

export function installChartResumeRecovery(
  recover: () => void,
  options: ChartResumeOptions = {},
): () => void {
  const doc = options.documentTarget ?? document;
  const win = options.windowTarget ?? window;
  const now = options.now ?? Date.now;
  const minInactiveMs = options.minInactiveMs ?? 2_000;
  let inactiveSince: number | null = doc.visibilityState === 'hidden' ? now() : null;
  let lastRecoveredAt = -Infinity;

  const markInactive = () => {
    if (inactiveSince === null) inactiveSince = now();
  };
  const restore = () => {
    if (doc.visibilityState === 'hidden') return;
    const since = inactiveSince;
    inactiveSince = null;
    const current = now();
    if (since === null || current - since < minInactiveMs) return;
    if (current - lastRecoveredAt < minInactiveMs) return;
    lastRecoveredAt = current;
    recover();
  };
  const visibilityChanged = () => {
    if (doc.visibilityState === 'hidden') markInactive();
    else restore();
  };
  const online = () => {
    if (doc.visibilityState === 'hidden') {
      markInactive();
      return;
    }
    if (now() - lastRecoveredAt < minInactiveMs) return;
    lastRecoveredAt = now();
    inactiveSince = null;
    recover();
  };

  doc.addEventListener('visibilitychange', visibilityChanged);
  win.addEventListener('blur', markInactive);
  win.addEventListener('focus', restore);
  win.addEventListener('online', online);
  return () => {
    doc.removeEventListener('visibilitychange', visibilityChanged);
    win.removeEventListener('blur', markInactive);
    win.removeEventListener('focus', restore);
    win.removeEventListener('online', online);
  };
}
