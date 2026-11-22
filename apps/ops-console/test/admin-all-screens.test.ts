import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { screenPlans } from '../src/admin-screens';

const shell = readFileSync(new URL('../src/admin-shell.ts', import.meta.url), 'utf8');
const scaffold = readFileSync(new URL('../src/admin-screens.ts', import.meta.url), 'utf8');
const design = readFileSync(new URL('../src/admin-all-screens.css', import.meta.url), 'utf8');
const main = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8');
const bootstrap = readFileSync(new URL('../src/admin-bootstrap.ts', import.meta.url), 'utf8');

const routeNames = [
  'market', 'candles', 'synthetic', 'indicators', 'lab', 'intelligence',
  'runtime', 'storage', 'diagnostics', 'releases', 'settings', 'security',
];

describe('all QNext specialist screens use a real shared workspace structure', () => {
  it('covers every non-dashboard route with unique valid navigation and contextual content', () => {
    expect(screenPlans).toHaveLength(12);
    expect(screenPlans.map((plan) => plan.route).sort()).toEqual([...routeNames].sort());
    expect(new Set(screenPlans.map((plan) => plan.number)).size).toBe(12);
    for (const plan of screenPlans) {
      expect(plan.title.length).toBeGreaterThan(12);
      expect(plan.summary.length).toBeGreaterThan(45);
      expect(plan.scope.length).toBeGreaterThan(8);
      expect(plan.related.length).toBeGreaterThanOrEqual(2);
      for (const item of plan.related) {
        expect(routeNames).toContain(item.route);
        expect(item.route).not.toBe(plan.route);
      }
    }
  });

  it('inserts route scaffolds as siblings instead of remounting the functional forms', () => {
    expect(scaffold).toContain('grid.insertBefore(fragment, grid.querySelector(\':scope > .card\'))');
    expect(scaffold).toContain('card.classList.add(\'ops-screen-card\')');
    expect(scaffold).toContain('card.dataset.opsTone = plan.tone');
    expect(scaffold).toContain('if (nav.dataset.opsSignature === signature) continue');
    expect(scaffold).toContain('nav.replaceChildren()');
    expect(scaffold).toContain('card.querySelector<HTMLElement>');
    expect(scaffold).not.toContain('cloneNode(');
    expect(scaffold).not.toContain('replaceWith(');
    expect(scaffold).not.toContain('window.fetch(');
    expect(scaffold).not.toContain('WebSocket(');
    expect(shell).toContain("import './admin-all-screens.css'");
    expect(shell).toContain("from './admin-screens'");
  });

  it('handles delayed dynamic Market Core and Quant/Lab panels without polling or scroll jumps', () => {
    expect(shell).toContain('installScreenPages(grid, (view) => applyView(view, true))');
    expect(shell).toContain('syncScreenPageSections(grid);');
    expect(shell).toContain('cardObserver.observe(grid, { childList: true })');
    expect(shell).toContain("if (changed) {");
    expect(scaffold).toContain('target.scrollIntoView({ behavior: \'auto\', block: \'start\' })');
    expect(scaffold).toContain('heading.focus({ preventScroll: true })');
    expect(scaffold).toContain('if (!target || target.hidden) return');
  });

  it('retains existing control identity, operations and server-side auth', () => {
    for (const id of [
      'runtime-status', 'release-status', 'storage-health-card', 'diagnostics-card',
      'feed-status-card', 'active-markets-card', 'candle-timeframes-card',
      'chart-timeframes-card', 'historical-repair-card', 'custom-indicators-card',
      'secret-form', 'config-editor',
    ]) {
      expect(main).toContain(id);
    }
    expect(main).toContain('new OpsAPI');
    expect(bootstrap).toContain('if (session.authenticated)');
    expect(scaffold).toContain("card.id !== 'release-card' && card.id !== 'broker-secrets-card'");
    expect(scaffold).toContain('body.append(headline)');
  });

  it('renders every route with designed layout, not an old generic status grid', () => {
    expect(design).toContain('main.grid > .ops-screen-frame');
    expect(design).toContain('main.grid > .card.ops-screen-card');
    expect(design).toContain('.ops-screen-head');
    expect(design).toContain('.ops-screen-toolbar');
    expect(design).toContain('.ops-screen-sections');
    expect(design).toContain('body.ops-shell-ready[data-ops-view="candles"] #chart-timeframes-card');
    expect(design).toContain('body.ops-shell-ready[data-ops-view="settings"] #broker-secrets-card');
    expect(design).toContain('#custom-indicators-card .indicator-workspace');
    expect(design).toContain('#diagnostics-log');
    expect(design).toContain('.storage-table th');
    expect(design).toContain('.market-toggle:has(input:checked)');
    expect(design).toContain('grid-template-columns: repeat(4,minmax(0,1fr))');
    expect(design).toContain('border-radius: 20px');
  });

  it('respects theme, responsive breakpoints, accessibility and editing performance', () => {
    expect(design).toContain(':root[data-qnext-theme="light"]');
    expect(design).toContain('@media (max-width: 1180px)');
    expect(design).toContain('@media (max-width: 900px)');
    expect(design).toContain('@media (max-width: 640px)');
    expect(design).toContain('@media (prefers-reduced-motion: reduce)');
    expect(design).toContain(':focus-visible');
    expect(design).toContain('scroll-margin-top: 100px');
    expect(scaffold).toContain('aria-labelledby');
    expect(scaffold).toContain('aria-label');
    expect(design).not.toContain('@import');
    expect(design).not.toContain('url(http');
    expect(design).not.toMatch(/\.ops-screen-card[^{}]*\{[^}]*backdrop-filter/s);
  });
});
