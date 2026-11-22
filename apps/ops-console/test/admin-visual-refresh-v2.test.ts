import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const shell = readFileSync(new URL('../src/admin-shell.ts', import.meta.url), 'utf8');
const style = readFileSync(new URL('../src/admin-sylva-v2.css', import.meta.url), 'utf8');
const main = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8');
const router = readFileSync(new URL('../src/admin-router.ts', import.meta.url), 'utf8');

describe('QNeXT screenshot-informed operations dashboard', () => {
  it('keeps real market-data sources and removes mock trend graphics', () => {
    expect(shell).toContain("summaryValue('#runtime-status'");
    expect(shell).toContain("textFrom('#observability-health'");
    expect(shell).toContain("textFrom('#five-second-day-health-state'");
    expect(shell).toContain("summaryValue('#release-status'");
    expect(shell).toContain("document.querySelectorAll<HTMLElement>('.five-second-symbol-row')");
    expect(shell).toContain('marketHealthSummary()');
    expect(shell).toContain('No simulated values');
    expect(shell).not.toContain('Math.random');
    expect(shell).not.toContain('Won this month');
  });

  it('uses an accessible modern overview with truthful unknown states', () => {
    expect(shell).toContain('class="ops-dashboard ops-overview-v2"');
    expect(shell).toContain('class="ops-dashboard-hero"');
    expect(shell).toContain('class="ops-kpi ops-kpi-blue"');
    expect(shell).toContain('class="ops-kpi ops-kpi-green"');
    expect(shell).toContain('class="ops-kpi ops-kpi-warm"');
    expect(shell).toContain('class="ops-kpi ops-kpi-ink"');
    expect(shell).toContain('id="ops-market-board-heading"');
    expect(shell).toContain('aria-valuenow=');
    expect(shell).toContain("gaps: '—'");
    expect(shell).toContain("symbols: '—'");
    expect(shell).toContain('No missing candles reported by current-day telemetry');
  });

  it('preserves the auth-first API architecture and existing router', () => {
    expect(main).toContain('new OpsAPI');
    expect(router).toContain('ADMIN_BASE');
    expect(shell).toContain("data-dashboard-view=\"market\"");
    expect(shell).toContain('wireDashboardButtons();');
    expect(shell).not.toContain('replaceWith(');
    expect(shell).toContain('window.scrollTo');
  });

  it('styles dense specialist pages and coherent navigation with local SVG', () => {
    expect(shell).toContain("import './admin-sylva-v2.css'");
    expect(shell).toContain('navIconPaths: Record<View, string>');
    expect(shell).toContain('navIcon(item.view)');
    expect(style).toContain('font-family: Inter');
    expect(style).toContain('--ops-accent: #c0ed50');
    expect(style).toContain('.ops-health-track');
    expect(style).toContain('.ops-shortcut-grid');
    expect(style).toContain('.storage-table th');
    expect(style).toContain('border-radius: 999px');
    expect(style).not.toContain('@import');
    expect(style).not.toContain('url(http');
  });

  it('remains responsive, readable, and reduced-motion aware', () => {
    expect(style).toContain("@media (max-width: 1370px)");
    expect(style).toContain("@media (max-width: 1120px)");
    expect(style).toContain("@media (max-width: 700px)");
    expect(style).toContain("@media (max-width: 420px)");
    expect(style).toContain("@media (prefers-reduced-motion: reduce)");
    expect(style).toContain("@media (prefers-contrast: more)");
    expect(style).toContain(":root[data-qnext-theme='light']");
    expect(style).toContain('font-variant-numeric: tabular-nums');
    expect(style).not.toMatch(/\.ops-(?:panel|kpi|health-row)[^{}]*\{[^}]*backdrop-filter/s);
  });

  it('updates repeated live status labels without remounting focused controls', () => {
    const render = shell.split('function renderDashboard(): void {')[1]?.split('function queueDashboardRender(): void {')[0] ?? '';
    expect(render).toContain('querySelectorAll<HTMLElement>');
    expect(render).toContain('element.textContent = value');
    expect(render).toContain('list.contains(document.activeElement)');
    expect(render).not.toContain('replaceWith(');
    expect(render).not.toContain('dashboard.innerHTML');
  });
});
