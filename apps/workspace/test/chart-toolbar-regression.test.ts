/// <reference types="vite/client" />
import templateSource from '../src/indicator-templates.ts?raw';
import workspaceSource from '../src/main.ts?raw';
import shadowSource from '../src/intelligence-lab-shadow-bridge.ts?raw';
import { describe, expect, it } from 'vitest';

// Guard the two workspace integration contracts that caused user-visible regressions.
// Browser pointer tests remain part of staging smoke, because Vitest is DOM-free here.
describe('public chart toolbar integration', () => {
  it('renders Symbol Search as an icon-only action with its accessible label', () => {
    expect(workspaceSource).toContain("label:'Symbol Search',icon:'qnext-symbol-search',iconOnly:true");
  });

  it('keeps the standard Vela timeframe and candle-style entry points', () => {
    expect(workspaceSource).toContain("'symbol', 'timeframes', 'style', 'indicators', 'qnext.indicator.templates', 'undo-redo'");
  });

  it('registers the template action before creating Vela and includes its stylesheet', () => {
    expect(workspaceSource).toContain("import './indicator-templates.css'");
    expect(workspaceSource.indexOf('registerIndicatorTemplates(() => workspace)')).toBeLessThan(workspaceSource.indexOf('new VelaWorkspace('));
  });

  it('renders the Templates action as an icon with its accessible label', () => {
    expect(templateSource).toContain("label: 'Templates',");
    expect(templateSource).toContain("icon: 'qnext-indicator-templates',\n    iconOnly: true,");
  });

  it('places chart alert On/Off next to the native Alerts bell, ahead of Screen Awake', () => {
    expect(workspaceSource).toContain("'alerts', 'qnext.alerts.toggle', 'qnext.screen-awake.toggle', 'qnext.zoom.open'");
  });

  it('does not mount the floating Lab widget, but retains background Shadow processing', () => {
    expect(workspaceSource).not.toContain('mountIntelligenceLabBridge(');
    expect(workspaceSource).toContain('mountIntelligenceLabShadowBridge(');
  });

  it('keeps Shadow-Live authenticated API processing but never creates the retired status widget', () => {
    expect(shadowSource).toContain('intelligence-lab%2Fshadow-targets');
    expect(shadowSource).toContain('intelligence-lab%2Fshadow-observation');
    expect(shadowSource).not.toContain('ensureShadowStatus(');
    expect(shadowSource).not.toContain('qnext-intelligence-lab-bridge');
    expect(shadowSource).not.toContain('setStatus(');
  });
});
