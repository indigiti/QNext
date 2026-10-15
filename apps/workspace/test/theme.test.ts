import { describe, expect, it } from 'vitest';
import { resolveTheme, themeIconPaths } from '../src/theme';
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

describe('icon-only appearance controls', () => {
  it('supplies distinct sun, moon and monitor vector icons without visible labels', () => {
    expect(Object.keys(themeIconPaths).sort()).toEqual(['dark', 'light', 'system']);
    expect(themeIconPaths.light).toContain('<circle');
    expect(themeIconPaths.dark).toContain('<path');
    expect(themeIconPaths.system).toContain('<rect');
    for (const svg of Object.values(themeIconPaths)) {
      expect(svg).not.toContain('<text');
      expect(svg).not.toContain('<script');
    }
  });
});
