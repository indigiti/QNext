/// <reference types="vite/client" />
import workspaceSource from '../src/main.ts?raw';
import { describe, expect, it } from 'vitest';

// Guard the two workspace integration contracts that caused user-visible regressions.
// Browser pointer tests remain part of staging smoke, because Vitest is DOM-free here.
describe('public chart toolbar integration', () => {
  it('keeps the standard Vela timeframe and candle-style entry points', () => {
    expect(workspaceSource).toContain("'symbol', 'timeframes', 'style', 'indicators', 'undo-redo'");
  });

  it('does not mount the floating Lab widget, but retains background Shadow processing', () => {
    expect(workspaceSource).not.toContain('mountIntelligenceLabBridge(');
    expect(workspaceSource).toContain('mountIntelligenceLabShadowBridge(');
  });
});
