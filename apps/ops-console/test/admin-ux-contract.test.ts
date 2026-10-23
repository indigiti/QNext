import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const shell = readFileSync(new URL('../src/admin-shell.ts', import.meta.url), 'utf8');
const routeStyles = readFileSync(new URL('../src/admin-route-fixes.css', import.meta.url), 'utf8');
const bootstrap = readFileSync(new URL('../src/admin-bootstrap.ts', import.meta.url), 'utf8');

describe('QNeXT admin UX contract', () => {
  it('keeps authentication before mounting the administrative console', () => {
    expect(bootstrap.indexOf("await import('./main')")).toBeGreaterThan(bootstrap.indexOf('adminLoaded = true'));
    expect(bootstrap).toContain('if (session.authenticated)');
  });

  it('does not replace the live overview or scroll during background telemetry refreshes', () => {
    const render = shell.split('function renderDashboard(): void {')[1]?.split('function queueDashboardRender(): void {')[0] ?? '';
    expect(render).not.toContain('replaceWith(');
    expect(render).toContain('data-ops-stat');
    const apply = shell.split('function applyView(view: View, updateHash: boolean): void {')[1]?.split('function viewFromHash(): View {')[0] ?? '';
    expect(apply).toContain('if (changed || updateHash)');
    expect(apply).toContain('window.scrollTo');
    const observer = shell.split('function installMutationSync(grid: HTMLElement): void {')[1] ?? '';
    expect(observer).not.toContain('applyView(');
  });

  it('maintains every administrative view and clear grouped navigation', () => {
    for (const view of ['dashboard', 'market', 'candles', 'synthetic', 'indicators', 'lab', 'intelligence', 'runtime', 'diagnostics', 'releases', 'settings', 'security']) {
      expect(shell).toContain(`view: '${view}'`);
    }
    expect(shell).toContain('Market data & quality');
    expect(shell).toContain('Strategies & research');
    expect(shell).toContain('System operations');
  });

  it('supports search visibility, keyboard focus and both themes', () => {
    expect(routeStyles).toContain('.ops-nav-item[hidden]');
    expect(routeStyles).toContain('button:focus-visible');
    expect(routeStyles).toContain("data-qnext-theme='light'");
    expect(shell).toContain("event.key === 'Escape'");
    expect(shell).toContain('aria-expanded');
  });
});
