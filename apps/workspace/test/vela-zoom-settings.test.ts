import { describe, expect, it } from 'vitest';
import { scaledWheelZoomDelta, shouldScaleWheelZoom, wheelZoomFactor } from '../src/vela-zoom-settings';

describe('Vela wheel zoom preferences', () => {
  it('keeps Vela fast mode unchanged', () => {
    expect(wheelZoomFactor('fast')).toBe(1);
    expect(scaledWheelZoomDelta(100, 'fast')).toBe(100);
    expect(shouldScaleWheelZoom(0, 100, false, 'fast')).toBe(false);
  });

  it('reduces regular vertical mouse-wheel zoom', () => {
    expect(wheelZoomFactor('balanced')).toBe(0.5);
    expect(scaledWheelZoomDelta(100, 'balanced')).toBe(50);
    expect(scaledWheelZoomDelta(-100, 'slow')).toBe(-25);
    expect(shouldScaleWheelZoom(0, 100, false, 'balanced')).toBe(true);
  });

  it('caps outsized zoom impulses and preserves sideways/Shift panning', () => {
    expect(scaledWheelZoomDelta(900, 'balanced')).toBe(120);
    expect(scaledWheelZoomDelta(-900, 'slow')).toBe(-120);
    expect(shouldScaleWheelZoom(120, 25, false, 'balanced')).toBe(false);
    expect(shouldScaleWheelZoom(0, 100, true, 'slow')).toBe(false);
    expect(shouldScaleWheelZoom(0, Number.NaN, false, 'slow')).toBe(false);
  });
});
