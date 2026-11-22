import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';

const shell = readFileSync(new URL('../src/admin-shell.ts', import.meta.url), 'utf8');
const styling = readFileSync(new URL('../src/admin-sylva-v3.css', import.meta.url), 'utf8');
const bootstrap = readFileSync(new URL('../src/admin-bootstrap.ts', import.meta.url), 'utf8');
const theme = readFileSync(new URL('../src/theme.ts', import.meta.url), 'utf8');
const prior = readFileSync(new URL('../src/admin-sylva-v2.css', import.meta.url), 'utf8');

describe('QNext sidebar and dashboard screenshot fidelity without copied CRM code', () => {
  it('renders three real QNext areas, preserving the thirteen original pages', () => {
    expect(shell).toContain('function areaMarkup(): string');
    expect(shell).toContain("view: 'market', label: 'Market Data'");
    expect(shell).toContain("view: 'indicators', label: 'Research'");
    expect(shell).toContain("view: 'runtime', label: 'Operations'");
    expect(shell).toContain('data-ops-area=');
    expect(shell).toContain('data-ops-nav=');
    expect(shell).toContain('aria-pressed="false"');
    expect(shell).toContain("button.setAttribute('aria-pressed', String(selected))");
    expect(shell).not.toContain('Gld Heat');
  });

  it('uses one authenticated appearance control in the sidebar footer', () => {
    expect(shell).toContain('id="ops-theme-host"');
    expect(shell).toContain('ops-session-profile');
    expect(shell).toContain('Session protected');
    expect(bootstrap).toContain("document.querySelector<HTMLElement>('#ops-theme-host')");
    expect(bootstrap).toContain('mountThemeControl({ container: appearanceHost })');
    expect(theme).toContain('setThemePreference');
    expect(theme).toContain('currentTheme()');
  });

  it('creates a rounded floating and independently scrolling desktop navigation', () => {
    expect(shell).toContain("import './admin-sylva-v3.css'");
    expect(styling).toContain('height: calc(100dvh - 24px)');
    expect(styling).toContain('margin: 12px 0 12px 12px');
    expect(styling).toContain('border-radius: 24px');
    expect(styling).toContain('overflow-y: auto');
    expect(styling).toContain('min-height: 0');
    expect(styling).toContain('body.ops-shell-ready[data-ops-dock="collapsed"]');
    expect(styling).toContain(':root[data-qnext-theme="light"]');
  });

  it('keeps search accessible when a narrow viewport hides its text', () => {
    expect(shell).toContain('aria-label\', \'Search admin sections');
    expect(shell).toContain('<svg viewBox="0 0 24 24" width="17" height="17"');
    expect(styling).toContain('.ops-command-trigger svg');
    expect(styling).toContain('@media (max-width: 1100px)');
    expect(styling).toContain('body.ops-shell-ready .ops-command-trigger > span,');
    expect(styling).not.toContain('display: none !important; /* hide search button */');
  });

  it('offers an accessible mobile drawer with click-outside dismissal and focus containment', () => {
    expect(shell).toContain("scrim.id = 'ops-nav-scrim'");
    expect(shell).toContain("scrim.setAttribute('aria-label', 'Close mobile navigation')");
    expect(shell).toContain('sidebar.inert = mobile && !visible');
    expect(shell).toContain('main.inert = visible');
    expect(shell).toContain('scrim.hidden = !visible');
    expect(shell).toContain("event.key === 'Tab'");
    expect(shell).toContain("event.shiftKey ? last : first");
    expect(shell).toContain("mobileMedia.addEventListener('change', () => setMobileNavigation(false))");
    expect(styling).toContain('body.ops-nav-open::before { content: none !important; }');
    expect(styling).toContain('body.ops-shell-ready .ops-nav-scrim:not([hidden])');
    expect(styling).toContain('@media (max-width: 900px)');
  });

  it('preserves genuine API-derived data and does not add heavy card filters', () => {
    expect(shell).toContain("textFrom('#observability-health'");
    expect(shell).toContain("summaryValue('#runtime-status'");
    expect(shell).toContain('marketHealthSummary()');
    expect(prior).toContain('.ops-health-track');
    expect(styling).not.toContain('@import');
    expect(styling).not.toContain('url(http');
    expect(styling).not.toMatch(/\.ops-(?:kpi|panel|health-row)[^{}]*\{[^}]*backdrop-filter/s);
    expect(styling).toContain('@media (prefers-reduced-motion: reduce)');
  });
});
