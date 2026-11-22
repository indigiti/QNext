import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

// These contracts complement TypeScript compilation and existing admin UX
// tests. Browser-level visual/performance certification remains required.
const shell = readFileSync(new URL('../src/admin-shell.ts', import.meta.url), 'utf8');
const design = readFileSync(new URL('../src/admin-sylva.css', import.meta.url), 'utf8');
const bootstrap = readFileSync(new URL('../src/admin-bootstrap.ts', import.meta.url), 'utf8');
const theme = readFileSync(new URL('../src/theme.css', import.meta.url), 'utf8');

describe('QNeXT frosted-glass admin shell contract', () => {
  it('keeps the existing authenticated, API-backed admin application', () => {
    expect(bootstrap).toContain("if (session.authenticated)");
    expect(bootstrap).toContain("await import('./main')");
    expect(shell).toContain("data-ops-nav");
    expect(shell).toContain('classifyCards();');
    expect(shell).toContain("setCardView(findCard('#runtime-status'), 'runtime'");
  });

  it('provides a dock without removing grouped navigation or accessible labels', () => {
    expect(shell).toContain("import './admin-sylva.css'");
    expect(shell).toContain('id="ops-dock-toggle"');
    expect(shell).toContain('aria-controls="ops-nav-region"');
    expect(shell).toContain('id="ops-nav-region"');
    expect(shell).toContain('aria-label="${html(item.label)}"');
    expect(shell).toContain("event.key.toLowerCase() === 'b'");
    expect(shell).toContain("event.key.toLowerCase() === 'k'");
    expect(shell).toContain('setDockCollapsed(false);');
    expect(shell).toContain('window.sessionStorage');
    expect(shell).toContain("search.dispatchEvent(new Event('input'");
  });

  it('preserves mobile drawer navigation, scroll position, and keyboard focus', () => {
    expect(shell).toContain("window.matchMedia('(max-width: 900px)').matches");
    expect(shell).toContain("setMobileNavigation(true)");
    expect(shell).toContain("event.key === 'Escape'");
    expect(shell).toContain('window.scrollTo');
    expect(shell).toContain('commandButton?.focus()');
    expect(design).toContain('@media (max-width: 900px)');
    expect(design).toContain('@media (prefers-reduced-motion: reduce)');
  });

  it('does not keep the heavy dashboard subtree observer active on other pages', () => {
    expect(shell).toContain("setDashboardSummaryMonitoring(view === 'dashboard')");
    expect(shell).toContain('dashboardSummaryObserver.disconnect()');
    expect(shell).toContain("if (document.body.dataset.opsView === 'dashboard') renderDashboard()");
    expect(shell).toContain('cardObserver.observe(grid, { childList: true })');
    expect(shell).not.toContain('if (changed || updateHash)');
  });

  it('limits blur to chrome and supports existing light/dark token state', () => {
    expect(design).toContain('--ops-sidebar-width: 78px');
    expect(design).toContain('backdrop-filter: blur(14px)');
    expect(design).toContain(":root[data-qnext-theme='light']");
    expect(theme).toContain("data-qnext-theme='light'");
    expect(design).not.toMatch(/\.ops-(?:kpi|panel|health-row)[^{}]*\{[^}]*backdrop-filter/s);
    expect(design).toContain('font-variant-numeric: tabular-nums');
  });
});
