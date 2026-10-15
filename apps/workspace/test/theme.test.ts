import { describe, expect, it } from 'vitest';
import { resolveTheme } from '../src/theme';
describe('QNext theme preference', () => {
  it('resolves explicit preferences independently of OS theme', () => {
    expect(resolveTheme('dark', false)).toBe('dark');
    expect(resolveTheme('light', true)).toBe('light');
  });
  it('uses the OS color scheme in system mode', () => {
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
  });
});
