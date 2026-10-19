import { afterEach, describe, expect, it, vi } from 'vitest';
import type { VelaWorkspace } from '@luxalgo/vela/workspace';
import { installVelaPanHistory } from '../src/charts/vela-history';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function workspaceHarness() {
  const listeners = new Map<string, (value: unknown) => void>();
  const chart = {
    market: { bars: 1500 },
    setMarket: vi.fn(async (_change: { bars: number }) => undefined),
    on: vi.fn((event: string, handler: (value: unknown) => void) => {
      listeners.set(event, handler);
      return () => listeners.delete(event);
    }),
  };
  const workspace = {
    chart,
    cells: () => [{ chart }],
    on: vi.fn(() => () => undefined),
  };
  return { workspace: workspace as unknown as VelaWorkspace, chart, listeners };
}

describe('Vela viewport stability integration', () => {
  it('does not register a second tab-focus recovery handler on top of Vela native gap healing', () => {
    const documentMock = {
      visibilityState: 'visible',
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    };
    const windowMock = { addEventListener: vi.fn(), removeEventListener: vi.fn() };
    vi.stubGlobal('document', documentMock);
    vi.stubGlobal('window', windowMock);
    const { workspace, chart } = workspaceHarness();
    const dispose = installVelaPanHistory(workspace);

    expect(documentMock.addEventListener).not.toHaveBeenCalled();
    expect(windowMock.addEventListener).not.toHaveBeenCalled();
    expect(chart.setMarket).not.toHaveBeenCalled();
    dispose();
  });

  it('extends older history without passing a lossy visible-range snapshot', () => {
    vi.useFakeTimers();
    const { workspace, chart, listeners } = workspaceHarness();
    const dispose = installVelaPanHistory(workspace);
    const emit = (event: string, payload: unknown) => {
      const listener = listeners.get(event);
      expect(listener).toBeDefined();
      listener?.(payload);
    };
    emit('history:complete', { oldestTime: 1000, barsLoaded: 1500, reason: 'depth' });
    emit('viewport:changed', { from: 1700, to: 2700 });
    emit('viewport:changed', { from: 1100, to: 2100 });
    vi.advanceTimersByTime(150);

    expect(chart.setMarket).toHaveBeenCalledTimes(1);
    expect(chart.setMarket).toHaveBeenCalledWith({ bars: 3000 });
    dispose();
  });
});
