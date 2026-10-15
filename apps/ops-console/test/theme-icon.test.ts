import { describe, expect, it } from 'vitest';
import { resolveTheme, themeIconPaths } from '../src/theme';

describe('admin icon-only appearance', () => {
  it('retains the existing dark, light and system theme preference semantics', () => {
    expect(resolveTheme('dark', false)).toBe('dark');
    expect(resolveTheme('light', true)).toBe('light');
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
  });

  it('offers sun, moon and monitor icons without visible text', () => {
    expect(Object.keys(themeIconPaths).sort()).toEqual(['dark', 'light', 'system']);
    expect(themeIconPaths.light).toContain('<circle');
    expect(themeIconPaths.dark).toContain('<path');
    expect(themeIconPaths.system).toContain('<rect');
    for (const svg of Object.values(themeIconPaths)) {
      expect(svg).not.toMatch(/<text|<script/i);
    }
  });
});
