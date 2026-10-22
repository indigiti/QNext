import { describe, expect, it, vi } from 'vitest';

import {
  ScreenAwakeController,
  type ScreenAwakeEvent,
  type ScreenAwakeLease,
  type ScreenAwakeRuntime,
} from '../src/screen-awake';

function harness(overrides: Partial<ScreenAwakeRuntime> = {}) {
  const events = new Map<ScreenAwakeEvent, Set<() => void>>();
  let visibility = 'visible';
  let time = 10000;
  let preference: string | null = null;
  const leases: Array<ScreenAwakeLease & { release: ReturnType<typeof vi.fn> }> = [];
  const request = vi.fn(async () => {
    const handlers: Array<() => void> = [];
    const lease = {
      released: false,
      release: vi.fn(async () => {
        lease.released = true;
        handlers.forEach((handler) => handler());
      }),
      addEventListener: (_type: 'release', handler: () => void) => { handlers.push(handler); },
    };
    leases.push(lease);
    return lease;
  });
  const runtime: ScreenAwakeRuntime = {
    defaultEnabled: true,
    visibility: () => visibility,
    now: () => time,
    request,
    on: (event, handler) => {
      const handlers = events.get(event) ?? new Set<() => void>();
      handlers.add(handler);
      events.set(event, handlers);
      return () => handlers.delete(handler);
    },
    readPreference: () => preference,
    savePreference: (value) => { preference = value; },
    ...overrides,
  };
  return {
    controller: new ScreenAwakeController(runtime),
    request,
    leases,
    emit: (event: ScreenAwakeEvent) => events.get(event)?.forEach((handler) => handler()),
    setVisibility: (value: string) => { visibility = value; },
    advance: (ms: number) => { time += ms; },
    preference: () => preference,
    listenerCount: () => [...events.values()].reduce((sum, set) => sum + set.size, 0),
  };
}

const settle = async () => {
  for (let i = 0; i < 6; i += 1) await Promise.resolve();
};

describe('Screen Awake controller', () => {
  it('acquires by default on mobile and releases/reacquires on visibility change', async () => {
    const h = harness();
    h.controller.start();
    await settle();
    expect(h.request).toHaveBeenCalledWith();
    expect(h.controller.status).toBe('active');
    h.setVisibility('hidden');
    h.emit('visibilitychange');
    await settle();
    expect(h.leases[0].release).toHaveBeenCalledTimes(1);
    expect(h.controller.status).toBe('idle');
    h.setVisibility('visible');
    h.emit('visibilitychange');
    await settle();
    expect(h.request).toHaveBeenCalledTimes(2);
    expect(h.controller.status).toBe('active');
  });

  it('respects an off preference and toggles on only by request', async () => {
    const h = harness({ readPreference: () => 'off' });
    h.controller.start();
    await settle();
    expect(h.controller.status).toBe('off');
    expect(h.request).not.toHaveBeenCalled();
    h.controller.toggle();
    await settle();
    expect(h.preference()).toBe('on');
    expect(h.controller.status).toBe('active');
    h.controller.toggle();
    await settle();
    expect(h.preference()).toBe('off');
    expect(h.leases[0].release).toHaveBeenCalledTimes(1);
    expect(h.controller.status).toBe('off');
  });

  it('reports unsupported browsers without requesting a lock', () => {
    const h = harness({ request: undefined });
    h.controller.start();
    expect(h.controller.status).toBe('unsupported');
    h.controller.toggle();
    expect(h.controller.status).toBe('off');
    h.controller.stop();
    expect(h.listenerCount()).toBe(0);
  });

  it('retries after a blocked initial request on the next user gesture', async () => {
    let calls = 0;
    const h = harness({
      request: async () => {
        calls += 1;
        if (calls === 1) throw new Error('user gesture needed');
        return {
          released: false,
          release: async () => undefined,
          addEventListener: () => undefined,
        };
      },
    });
    h.controller.start();
    await settle();
    expect(h.controller.status).toBe('blocked');
    h.emit('pointerdown');
    await settle();
    expect(calls).toBe(2);
    expect(h.controller.status).toBe('active');
  });

  it('ignores an in-flight lock granted after pagehide and stops cleanly', async () => {
    let resolveRequest: ((lease: ScreenAwakeLease) => void) | undefined;
    const h = harness({
      request: () => new Promise<ScreenAwakeLease>((resolve) => { resolveRequest = resolve; }),
    });
    h.controller.start();
    h.emit('pagehide');
    const lease = {
      released: false,
      release: vi.fn(async () => undefined),
      addEventListener: vi.fn(),
    };
    resolveRequest?.(lease);
    await settle();
    expect(lease.release).toHaveBeenCalledTimes(1);
    expect(h.controller.status).toBe('idle');
    h.controller.stop();
    expect(h.listenerCount()).toBe(0);
  });
});
