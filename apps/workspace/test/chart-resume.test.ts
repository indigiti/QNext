import { describe, expect, it, vi } from 'vitest';
import { installChartResumeRecovery, type ResumeEventTarget, type VisibilityEventTarget } from '../src/charts/chart-resume';

class Events implements ResumeEventTarget {
  private readonly callbacks = new Map<string, Set<EventListener>>();
  addEventListener(type: string, listener: EventListener) {
    if (!this.callbacks.has(type)) this.callbacks.set(type, new Set());
    this.callbacks.get(type)!.add(listener);
  }
  removeEventListener(type: string, listener: EventListener) {
    this.callbacks.get(type)?.delete(listener);
  }
  emit(type: string) {
    for (const callback of this.callbacks.get(type) ?? []) callback(new Event(type));
  }
}
class VisibleEvents extends Events implements VisibilityEventTarget {
  visibilityState = 'visible';
}

describe('chart resume recovery', () => {
  it('recovers only after a hidden tab becomes visible and deduplicates focus', () => {
    let now = 10000;
    const doc = new VisibleEvents();
    const win = new Events();
    const recover = vi.fn();
    const dispose = installChartResumeRecovery(recover, {
      documentTarget: doc, windowTarget: win, now: () => now,
    });
    doc.visibilityState = 'hidden';
    doc.emit('visibilitychange');
    now += 60000;
    doc.visibilityState = 'visible';
    doc.emit('visibilitychange');
    win.emit('focus');
    expect(recover).toHaveBeenCalledTimes(1);
    dispose();
    doc.visibilityState = 'hidden';
    doc.emit('visibilitychange');
    now += 60000;
    doc.visibilityState = 'visible';
    doc.emit('visibilitychange');
    expect(recover).toHaveBeenCalledTimes(1);
  });

  it('ignores quick blur/focus but reconciles an actual inactive window', () => {
    let now = 10000;
    const doc = new VisibleEvents();
    const win = new Events();
    const recover = vi.fn();
    const dispose = installChartResumeRecovery(recover, {
      documentTarget: doc, windowTarget: win, now: () => now,
    });
    win.emit('blur');
    now += 100;
    win.emit('focus');
    expect(recover).not.toHaveBeenCalled();
    win.emit('blur');
    now += 4000;
    win.emit('focus');
    expect(recover).toHaveBeenCalledTimes(1);
    dispose();
  });

  it('reconciles when connectivity returns without requiring a tab switch', () => {
    let now = 10000;
    const doc = new VisibleEvents();
    const win = new Events();
    const recover = vi.fn();
    const dispose = installChartResumeRecovery(recover, {
      documentTarget: doc, windowTarget: win, now: () => now,
    });
    win.emit('online');
    win.emit('online');
    expect(recover).toHaveBeenCalledTimes(1);
    now += 4000;
    win.emit('online');
    expect(recover).toHaveBeenCalledTimes(2);
    dispose();
  });
});
