import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// Guard the two workspace integration contracts that caused user-visible regressions.
// Browser pointer tests remain part of staging smoke, because Vitest is DOM-free here.
const workspaceSource = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8');
const shellCSS = readFileSync(new URL('../src/style.css', import.meta.url), 'utf8');

describe('public chart toolbar integration', () => {
  it('keeps the standard Vela timeframe and candle-style entry points', () => {
    expect(workspaceSource).toContain("'symbol', 'timeframes', 'style', 'indicators', 'undo-redo'");
  });

  it('raises Vela popup positioners above the chart instead of changing canvas pointer handling', () => {
    expect(shellCSS).toMatch(/#app \.vela-workspace > \.vela-ui-layer\s*\{\s*z-index:\s*100\s*;/);
    expect(shellCSS).not.toContain('.vela-ws-grid { pointer-events: none');
  });

  it('does not mount the floating Lab widget, but retains background Shadow processing', () => {
    expect(workspaceSource).not.toContain('mountIntelligenceLabBridge(');
    expect(workspaceSource).toContain('mountIntelligenceLabShadowBridge(');
  });
});
