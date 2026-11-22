import './admin-shell.css';
import './admin-sylva.css';

export {};

type View =
  | 'dashboard'
  | 'market'
  | 'candles'
  | 'synthetic'
  | 'indicators'
  | 'intelligence'
  | 'lab'
  | 'runtime'
  | 'releases'
  | 'diagnostics'
  | 'storage'
  | 'settings'
  | 'security';

type ViewMeta = {
  title: string;
  description: string;
};

const views: Record<View, ViewMeta> = {
  dashboard: {
    title: 'Dashboard',
    description: 'Market data, canonical 5s integrity, runtime and release health at a glance.',
  },
  market: {
    title: 'Market Data',
    description: 'Live feed, browser transport, active markets and current-day canonical 5s health.',
  },
  candles: {
    title: 'Candles & History',
    description: 'Candle formation, chart-visible intervals and historical recovery controls.',
  },
  synthetic: {
    title: 'Synthetic',
    description: 'NIFTY-SYN+ research synthetic and isolated forward paper evaluation.',
  },
  indicators: {
    title: 'Indicators',
    description: 'Custom indicator catalog, Pine v6 scripts and chart feature contracts.',
  },
  intelligence: {
    title: 'Quant Intelligence',
    description: 'Production candidates, controlled training, promotion gates and rollback.',
  },
  lab: {
    title: 'Intelligence Lab',
    description: 'Isolated chart experiments, ML challengers, backtests and certification.',
  },
  runtime: {
    title: 'Runtime',
    description: 'Market Core state and service lifecycle controls.',
  },
  releases: {
    title: 'Releases',
    description: 'Activate a release or roll back to the previous deployment.',
  },
  storage: {
    title: 'Data Storage',
    description: 'Read-only disk usage, file persistence and bounded storage metadata checks.',
  },
  diagnostics: {
    title: 'Diagnostics',
    description: 'Runtime diagnostics and recent Market Core logs.',
  },
  settings: {
    title: 'Settings',
    description: 'Workspace defaults, broker credentials and market configuration.',
  },
  security: {
    title: 'Security',
    description: 'Admin access recovery and security controls.',
  },
};

const navGroups: Array<{ label: string; items: Array<{ view: View; label: string; icon: string }> }> = [
  {
    label: 'Overview',
    items: [
      { view: 'dashboard', label: 'Dashboard', icon: '▦' },
    ],
  },
  {
    label: 'Market data & quality',
    items: [
      { view: 'market', label: 'Live Market & 5s', icon: '◉' },
      { view: 'candles', label: 'Candles & History', icon: '▥' },
      { view: 'synthetic', label: 'Synthetic Research', icon: '◇' },
    ],
  },
  {
    label: 'Strategies & research',
    items: [
      { view: 'indicators', label: 'Indicators & Pine', icon: '⌁' },
      { view: 'lab', label: 'Intelligence Lab', icon: '◈' },
      { view: 'intelligence', label: 'Quant Intelligence', icon: '◫' },
    ],
  },
  {
    label: 'System operations',
    items: [
      { view: 'runtime', label: 'Runtime & services', icon: '⚡' },
      { view: 'storage', label: 'Data Storage', icon: '▤' },
      { view: 'diagnostics', label: 'Logs & diagnostics', icon: '▤' },
      { view: 'releases', label: 'Releases & rollback', icon: '⬆' },
    ],
  },
  {
    label: 'Administration',
    items: [
      { view: 'settings', label: 'Workspace & brokers', icon: '⚙' },
      { view: 'security', label: 'Security', icon: '◆' },
    ],
  },
];

function html(value: unknown): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function findCard(selector: string): HTMLElement | null {
  return document.querySelector(selector)?.closest<HTMLElement>('.card') ?? null;
}

function setCardView(card: HTMLElement | null, view: View, stableID?: string): void {
  if (!card) return;
  if (stableID && !card.id) card.id = stableID;
  card.dataset.opsView = view;
}

function classifyCards(): void {
  setCardView(findCard('#runtime-status'), 'runtime', 'runtime-card');
  setCardView(findCard('#release-status'), 'releases', 'release-card');
  setCardView(document.querySelector<HTMLElement>('#diagnostics-card'), 'diagnostics');
  setCardView(document.querySelector<HTMLElement>('#storage-health-card'), 'storage');

  setCardView(document.querySelector<HTMLElement>('#feed-status-card'), 'market');
  setCardView(document.querySelector<HTMLElement>('#active-markets-card'), 'market');

  setCardView(document.querySelector<HTMLElement>('#candle-timeframes-card'), 'candles');
  setCardView(document.querySelector<HTMLElement>('#chart-timeframes-card'), 'candles');
  setCardView(document.querySelector<HTMLElement>('#historical-repair-card'), 'candles');

  setCardView(document.querySelector<HTMLElement>('#syn-plus-status-card'), 'synthetic');
  setCardView(document.querySelector<HTMLElement>('#custom-indicators-card'), 'indicators');
  setCardView(document.querySelector<HTMLElement>('#quant-intelligence-card'), 'intelligence');
  setCardView(document.querySelector<HTMLElement>('#intelligence-lab-card'), 'lab');

  setCardView(document.querySelector<HTMLElement>('#workspace-settings-card'), 'settings');
  setCardView(findCard('#secret-form'), 'settings', 'broker-secrets-card');
  setCardView(findCard('#config-editor'), 'settings', 'market-config-card');
  setCardView(document.querySelector<HTMLElement>('#admin-access-card'), 'security');

  document.querySelectorAll<HTMLElement>('main.grid > .card:not([data-ops-view])').forEach((card) => {
    const id = card.id.toLowerCase();
    const text = `${id} ${card.querySelector('.eyebrow')?.textContent ?? ''} ${card.querySelector('h2')?.textContent ?? ''}`.toLowerCase();
    if (text.includes('intelligence lab')) {
      card.dataset.opsView = 'lab';
    } else if (text.includes('quant intelligence') || text.includes('model') || text.includes('strategy')) {
      card.dataset.opsView = 'intelligence';
    } else if (text.includes('indicator')) {
      card.dataset.opsView = 'indicators';
    } else if (text.includes('synthetic') || text.includes('syn+')) {
      card.dataset.opsView = 'synthetic';
    } else if (text.includes('diagnostic')) {
      card.dataset.opsView = 'diagnostics';
    } else if (text.includes('security') || text.includes('recovery') || text.includes('admin access')) {
      card.dataset.opsView = 'security';
    } else {
      card.dataset.opsView = 'settings';
    }
  });
}

function navMarkup(): string {
  return navGroups.map((group) => `
    <div class="ops-nav-group">
      <div class="ops-nav-label">${html(group.label)}</div>
      ${group.items.map((item) => `
        <button type="button" class="ops-nav-item" data-ops-nav="${item.view}" data-search="${html(`${group.label} ${item.label}`.toLowerCase())}" title="${html(item.label)}" aria-label="${html(item.label)}">
          <span class="ops-nav-icon" aria-hidden="true">${html(item.icon)}</span>
          <span>${html(item.label)}</span>
        </button>
      `).join('')}
    </div>
  `).join('');
}

function installShell(): void {
  if (document.querySelector('#ops-sidebar')) return;
  const shell = document.querySelector<HTMLElement>('.shell');
  const topbar = document.querySelector<HTMLElement>('.topbar');
  const grid = document.querySelector<HTMLElement>('main.grid');
  if (!shell || !topbar || !grid) {
    window.setTimeout(installShell, 50);
    return;
  }

  classifyCards();
  document.body.classList.add('ops-shell-ready');
  shell.classList.add('ops-shell');
  grid.id = 'ops-content-grid';

  const sidebar = document.createElement('aside');
  sidebar.id = 'ops-sidebar';
  sidebar.className = 'ops-sidebar';
  sidebar.innerHTML = `
    <div class="ops-brand">
      <div class="ops-brand-mark">QN</div>
      <div class="ops-brand-copy">
        <strong>QNeXT</strong>
        <span>Operations workspace</span>
      </div>
      <button type="button" id="ops-dock-toggle" class="ops-dock-toggle" aria-label="Collapse navigation" aria-expanded="true" aria-controls="ops-nav-region" title="Collapse navigation (Ctrl/⌘ B)">‹</button>
      <button type="button" id="ops-sidebar-close" class="ops-sidebar-close" aria-label="Close navigation">×</button>
    </div>
    <label class="ops-search" for="ops-nav-search">
      <span aria-hidden="true">⌕</span>
      <input id="ops-nav-search" type="search" autocomplete="off" placeholder="Quick navigation" />
    </label>
    <p id="ops-nav-search-feedback" class="ops-search-feedback" role="status" aria-live="polite" hidden></p>
    <nav id="ops-nav-region" class="ops-nav" aria-label="QNext Admin navigation">
      ${navMarkup()}
    </nav>
    <div class="ops-sidebar-foot">
      <div class="ops-sidebar-health"><span class="ops-live-dot"></span><span>QNext control plane</span></div>
      <span class="muted">File-backed · session protected</span>
    </div>
  `;

  const main = document.createElement('div');
  main.className = 'ops-main';
  shell.insertBefore(sidebar, shell.firstChild);
  main.append(topbar, grid);
  const toast = shell.querySelector('#toast');
  shell.insertBefore(main, toast ?? null);

  const heading = topbar.firstElementChild as HTMLElement | null;
  heading?.classList.add('ops-page-heading');
  if (heading) {
    const eyebrow = heading.querySelector<HTMLElement>('.eyebrow');
    if (eyebrow) eyebrow.textContent = 'QNeXT Admin';
  }

  const mobileButton = document.createElement('button');
  mobileButton.id = 'ops-sidebar-toggle';
  mobileButton.type = 'button';
  mobileButton.className = 'ops-sidebar-toggle secondary';
  mobileButton.setAttribute('aria-label', 'Toggle navigation');
  mobileButton.setAttribute('aria-controls', 'ops-sidebar');
  mobileButton.setAttribute('aria-expanded', 'false');
  mobileButton.textContent = '☰';
  topbar.prepend(mobileButton);

  const commandButton = document.createElement('button');
  commandButton.type = 'button';
  commandButton.id = 'ops-command-trigger';
  commandButton.className = 'ops-command-trigger secondary';
  commandButton.setAttribute('aria-label', 'Search admin sections');
  commandButton.setAttribute('aria-keyshortcuts', 'Control+K Meta+K');
  commandButton.setAttribute('title', 'Search sections (Ctrl/⌘ K)');
  commandButton.innerHTML = '<span>Search sections</span><kbd>⌘/Ctrl K</kbd>';
  heading?.insertAdjacentElement('afterend', commandButton);

  installDashboard(grid);
  installNavigation(sidebar, topbar, grid);
  installMutationSync(grid);
}

function textFrom(selector: string, fallback = '—'): string {
  const value = document.querySelector<HTMLElement>(selector)?.textContent?.trim();
  return value || fallback;
}

function summaryValue(containerSelector: string, labels: string[]): string {
  const container = document.querySelector<HTMLElement>(containerSelector);
  if (!container) return '—';
  const items = Array.from(container.children) as HTMLElement[];
  for (const item of items) {
    const label = item.querySelector('span')?.textContent?.trim().toLowerCase() ?? '';
    if (labels.some((candidate) => label.includes(candidate.toLowerCase()))) {
      const strong = item.querySelector('strong')?.textContent?.trim();
      const badge = item.querySelector('.badge')?.textContent?.trim();
      return strong || badge || '—';
    }
  }
  return '—';
}

function marketHealthSummary(): { symbols: string; gaps: string; message: string } {
  const rows = Array.from(document.querySelectorAll<HTMLElement>('.five-second-symbol-row'));
  if (rows.length === 0) {
    return { symbols: '—', gaps: '—', message: 'Awaiting market-day telemetry' };
  }
  let gaps = 0;
  let incomplete = 0;
  for (const row of rows) {
    const missingText = Array.from(row.querySelectorAll<HTMLElement>('.five-second-symbol-kpis > span'))
      .find((item) => item.textContent?.trim().startsWith('Missing'))
      ?.querySelector('strong')?.textContent?.trim();
    if (!missingText || !/^\\d[\\d,]*$/.test(missingText)) {
      incomplete++;
      continue;
    }
    gaps += Number(missingText.replaceAll(',', ''));
  }
  return {
    symbols: String(rows.length),
    gaps: incomplete === 0 ? gaps.toLocaleString('en-IN') : '—',
    message: incomplete ? 'Some sources have not reported yet' : gaps > 0
      ? 'Inspect missing 5s candles in Market Data' : 'No missing candles reported by current-day telemetry',
  };
}

function miniFiveSecondRows(): string {
  const rows = Array.from(document.querySelectorAll<HTMLElement>('.five-second-symbol-row'));
  if (rows.length === 0) {
    return '<div class="ops-empty-state">Waiting for current-day candle health. Historical data is not counted as a live feed.</div>';
  }

  return rows.map((row) => {
    const symbol = row.dataset.symbol ?? row.querySelector('.five-second-symbol-title strong')?.textContent ?? 'Unknown';
    const kpis = Array.from(row.querySelectorAll<HTMLElement>('.five-second-symbol-kpis > span'));
    const health = kpis.find((item) => item.textContent?.trim().startsWith('Health'))?.querySelector('strong')?.textContent?.trim() ?? '—';
    const missing = kpis.find((item) => item.textContent?.trim().startsWith('Missing'))?.querySelector('strong')?.textContent?.trim() ?? '—';
    const status = row.querySelector<HTMLElement>('.five-second-symbol-title .badge')?.textContent?.trim() ?? 'WAITING';
    const needsAttention = status.toUpperCase() === 'CHECK' || (Number(missing.replaceAll(',', '')) > 0);
    const percentage = /^\\d+(?:\\.\\d+)?%$/.test(health) ? Math.min(100, Math.max(0, Number(health.slice(0, -1)))) : null;
    const meter = percentage === null
      ? '<span class="ops-health-track ops-health-track-empty" aria-label="Health percentage unavailable"></span>'
      : `<span class="ops-health-track" role="progressbar" aria-label="${html(symbol)} candle health" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${percentage}"><span class="ops-health-fill" style="width: ${percentage}%"></span></span>`;
    return `
      <button type="button" class="ops-health-row" data-dashboard-view="market" aria-label="Open ${html(symbol)} current-day 5 second health">
        <span class="ops-health-symbol"><strong>${html(symbol)}</strong><small class="${needsAttention ? 'is-bad' : ''}">${html(status)}</small></span>
        <span class="ops-health-visual">${meter}<small>Current-day 5s completeness</small></span>
        <span class="ops-health-pct"><strong>${html(health)}</strong><small>${html(missing)} missing</small></span>
      </button>
    `;
  }).join('');
}

function dashboardMarkup(): string {
  const runtimeState = summaryValue('#runtime-status', ['state', 'status', 'service']);
  const feedState = textFrom('#observability-health', summaryValue('#feed-summary', ['status', 'authority']));
  const fiveSecondState = textFrom('#five-second-day-health-state', 'WARMING');
  const release = summaryValue('#release-status', ['active', 'current', 'release']);
  const transport = textFrom('#browser-transport-status', 'Probing…').replace(/^Browser transport:\\s*/i, '');
  const sessionDay = new Intl.DateTimeFormat('en-IN', {
    timeZone: 'Asia/Kolkata', weekday: 'long', day: 'numeric', month: 'short',
  }).format(new Date());
  const market = marketHealthSummary();

  return `
    <section id="ops-dashboard-card" class="ops-dashboard ops-overview-v2" data-ops-view="dashboard">
      <div class="ops-overview-topline">
        <span class="ops-breadcrumb"><span>QNeXT</span><span aria-hidden="true">/</span> Overview</span>
        <span class="ops-session-date">${html(sessionDay)} · IST</span>
      </div>

      <div class="ops-dashboard-hero">
        <div class="ops-hero-copy">
          <span class="ops-overline"><span class="ops-overline-dot" aria-hidden="true"></span> OPERATIONS WORKSPACE</span>
          <h2>Markets, <span>in focus.</span></h2>
          <p class="muted">One workspace for live market quality, SYN+ integrity, research, and infrastructure. Every figure comes from QNeXT telemetry.</p>
        </div>
        <div class="ops-hero-actions">
          <button type="button" class="ops-primary-action" data-dashboard-view="market">View market health <span aria-hidden="true">↗</span></button>
          <button type="button" class="ops-ghost-action" data-dashboard-view="diagnostics">Diagnostics</button>
        </div>
      </div>

      <div class="ops-section-heading">
        <div><p class="eyebrow">At a glance</p><h3>System pulse</h3></div>
        <p>Live status · No simulated values</p>
      </div>

      <div class="ops-kpi-grid">
        <button type="button" class="ops-kpi ops-kpi-blue" data-dashboard-view="runtime">
          <span class="ops-kpi-top"><span>Market Core</span><span class="ops-kpi-symbol" aria-hidden="true">↗</span></span>
          <strong data-ops-stat="runtime">${html(runtimeState)}</strong>
          <small>Runtime & services <span aria-hidden="true">→</span></small>
        </button>
        <button type="button" class="ops-kpi ops-kpi-green" data-dashboard-view="market">
          <span class="ops-kpi-top"><span>Feed authority</span><span class="ops-kpi-symbol" aria-hidden="true">◉</span></span>
          <strong data-ops-stat="feed">${html(feedState)}</strong>
          <small data-ops-stat="transport">${html(transport)}</small>
        </button>
        <button type="button" class="ops-kpi ops-kpi-warm" data-dashboard-view="market">
          <span class="ops-kpi-top"><span>Canonical 5s</span><span class="ops-kpi-symbol" aria-hidden="true">▥</span></span>
          <strong data-ops-stat="five-second">${html(fiveSecondState)}</strong>
          <small>Current-day integrity <span aria-hidden="true">→</span></small>
        </button>
        <button type="button" class="ops-kpi ops-kpi-ink" data-dashboard-view="releases">
          <span class="ops-kpi-top"><span>Installed release</span><span class="ops-kpi-symbol" aria-hidden="true">↗</span></span>
          <strong data-ops-stat="release">${html(release)}</strong>
          <small>Deployment & rollback <span aria-hidden="true">→</span></small>
        </button>
      </div>

      <div class="ops-dashboard-grid">
        <section class="ops-panel ops-market-board" aria-labelledby="ops-market-board-heading">
          <div class="ops-panel-head">
            <div>
              <p class="eyebrow">Market data quality</p>
              <h3 id="ops-market-board-heading">5-second candle health</h3>
              <p class="ops-panel-subtitle">Current-day completeness by configured symbol</p>
            </div>
            <button type="button" class="ops-ghost-action compact" data-dashboard-view="market">Open feed details <span aria-hidden="true">↗</span></button>
          </div>
          <div class="ops-market-board-meta">
            <span><i aria-hidden="true"></i><strong data-ops-stat="tracked">${html(market.symbols)}</strong> symbols reporting</span>
            <span class="ops-market-board-gaps"><strong data-ops-stat="gaps">${html(market.gaps)}</strong> missing 5s candles</span>
          </div>
          <div class="ops-health-list">${miniFiveSecondRows()}</div>
          <p class="ops-panel-footnote" data-ops-stat="market-note">${html(market.message)}</p>
        </section>

        <section class="ops-panel ops-platform-board" aria-labelledby="ops-platform-heading">
          <div class="ops-panel-head">
            <div><p class="eyebrow">Services</p><h3 id="ops-platform-heading">Operational snapshot</h3>
              <p class="ops-panel-subtitle">Current signals from QNeXT's existing services</p>
            </div>
          </div>
          <div class="ops-platform-list">
            <button type="button" data-dashboard-view="runtime"><span class="ops-platform-icon" aria-hidden="true">◈</span><span><strong>Market Core</strong><small>Runtime status</small></span><em data-ops-stat="runtime">${html(runtimeState)}</em></button>
            <button type="button" data-dashboard-view="market"><span class="ops-platform-icon" aria-hidden="true">◉</span><span><strong>Market transport</strong><small>Browser stream</small></span><em data-ops-stat="transport">${html(transport)}</em></button>
            <button type="button" data-dashboard-view="market"><span class="ops-platform-icon" aria-hidden="true">▥</span><span><strong>5s integrity</strong><small>Current-day overview</small></span><em data-ops-stat="five-second">${html(fiveSecondState)}</em></button>
            <button type="button" data-dashboard-view="releases"><span class="ops-platform-icon" aria-hidden="true">↗</span><span><strong>Release</strong><small>Current deployment</small></span><em data-ops-stat="release">${html(release)}</em></button>
          </div>
          <div class="ops-platform-note">Live telemetry can be unavailable outside market hours. Unknown is not treated as healthy.</div>
        </section>
      </div>

      <section class="ops-shortcuts" aria-labelledby="ops-shortcuts-heading">
        <div class="ops-section-heading"><div><p class="eyebrow">Workspace</p><h3 id="ops-shortcuts-heading">Your tools</h3></div><p>Jump into a specialist view</p></div>
        <div class="ops-shortcut-grid">
          <button type="button" data-dashboard-view="candles"><span aria-hidden="true">▥</span><strong>Candles & history</strong><small>Formation and replay</small></button>
          <button type="button" data-dashboard-view="synthetic"><span aria-hidden="true">◇</span><strong>SYN+ research</strong><small>Volume and synthetic quality</small></button>
          <button type="button" data-dashboard-view="indicators"><span aria-hidden="true">⌁</span><strong>Indicators & Pine</strong><small>Catalog and editor</small></button>
          <button type="button" data-dashboard-view="lab"><span aria-hidden="true">◈</span><strong>Intelligence Lab</strong><small>Experiments and tests</small></button>
          <button type="button" data-dashboard-view="storage"><span aria-hidden="true">▤</span><strong>Storage</strong><small>Capacity and retention</small></button>
          <button type="button" data-dashboard-view="diagnostics"><span aria-hidden="true">≡</span><strong>Diagnostics</strong><small>Service events and logs</small></button>
        </div>
      </section>
    </section>
  `;
}

let dashboardRenderQueued = false;
let cachedHealthRows = '';

function renderDashboard(): void {
  const dashboard = document.querySelector<HTMLElement>('#ops-dashboard-card');
  if (!dashboard) return;

  // Keep the dashboard DOM stable: replacing the whole card on live telemetry
  // refresh used to drop focus, break button interactions and move the viewport.
  const stats: Record<string, string> = {
    runtime: summaryValue('#runtime-status', ['state', 'status', 'service']),
    feed: textFrom('#observability-health', summaryValue('#feed-summary', ['status', 'authority'])),
    'five-second': textFrom('#five-second-day-health-state', 'WARMING'),
    transport: textFrom('#browser-transport-status', 'Probing…').replace(/^Browser transport:\s*/i, ''),
    release: summaryValue('#release-status', ['active', 'current', 'release']),
    tracked: marketHealthSummary().symbols,
    gaps: marketHealthSummary().gaps,
    'market-note': marketHealthSummary().message,
  };
  for (const [name, value] of Object.entries(stats)) {
    dashboard.querySelectorAll<HTMLElement>(`[data-ops-stat="${name}"]`).forEach((element) => {
      if (element.textContent !== value) element.textContent = value;
    });
  }

  const list = dashboard.querySelector<HTMLElement>('.ops-health-list');
  // Do not remove a keyboard-focused row while it is being activated.
  if (list && !list.contains(document.activeElement)) {
    const next = miniFiveSecondRows();
    if (cachedHealthRows !== next) {
      cachedHealthRows = next;
      list.innerHTML = next;
    }
  }
  wireDashboardButtons();
}

function queueDashboardRender(): void {
  if (dashboardRenderQueued) return;
  dashboardRenderQueued = true;
  window.setTimeout(() => {
    dashboardRenderQueued = false;
    if (document.body.dataset.opsView === 'dashboard') renderDashboard();
  }, 120);
}

function installDashboard(grid: HTMLElement): void {
  if (document.querySelector('#ops-dashboard-card')) return;
  const wrapper = document.createElement('div');
  wrapper.innerHTML = dashboardMarkup().trim();
  grid.prepend(wrapper.firstElementChild as HTMLElement);
  wireDashboardButtons();
}

function wireDashboardButtons(): void {
  document.querySelectorAll<HTMLButtonElement>('[data-dashboard-view]').forEach((button) => {
    if (button.dataset.opsWired === 'true') return;
    button.dataset.opsWired = 'true';
    button.addEventListener('click', () => {
      const view = button.dataset.dashboardView as View | undefined;
      if (view && views[view]) applyView(view, true);
    });
  });
}

function setMobileNavigation(open: boolean): void {
  const sidebar = document.querySelector<HTMLElement>('#ops-sidebar');
  const toggle = document.querySelector<HTMLButtonElement>('#ops-sidebar-toggle');
  sidebar?.classList.toggle('open', open);
  toggle?.setAttribute('aria-expanded', String(open));
  document.body.classList.toggle('ops-nav-open', open);
}

function applyView(view: View, updateHash: boolean): void {
  const meta = views[view];
  const changed = document.body.dataset.opsView !== view;
  document.body.dataset.opsView = view;

  document.querySelectorAll<HTMLElement>('main.grid > [data-ops-view]').forEach((section) => {
    section.hidden = section.dataset.opsView !== view;
  });

  document.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
    const active = button.dataset.opsNav === view;
    button.classList.toggle('active', active);
    if (active) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  });

  const heading = document.querySelector<HTMLElement>('.ops-page-heading');
  if (heading) {
    const h1 = heading.querySelector('h1');
    const description = heading.querySelector('.muted');
    if (h1) h1.textContent = meta.title;
    if (description) description.textContent = meta.description;
  }

  setMobileNavigation(false);
  // Observe noisy telemetry DOM only while the dashboard is visible.
  // Market / Indicators editors no longer pay a global subtree observer cost.
  setDashboardSummaryMonitoring(view === 'dashboard');
  if (updateHash) history.replaceState(null, '', `#${view}`);
  if (view === 'dashboard') renderDashboard();

  // Only an actual route change or user navigation resets page scroll.
  // Background polling/mutation reconciliation must never scroll the operator.
  if (changed) {
    // A route transition should start at the top. Clicking the current route
    // again, refreshing an existing view or changing the URL must not scroll.
    window.requestAnimationFrame(() => {
      if (document.body.dataset.opsView === view) {
        window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
      }
    });
  }
}

function viewFromHash(): View {
  const hash = window.location.hash.replace(/^#/, '') as View;
  return views[hash] ? hash : 'dashboard';
}

// Navigation preferences are deliberately presentation-only. No admin tokens,
// feed state, or server-backed configuration is persisted in browser storage.
const DOCK_PREF_KEY = 'qnext-ops-dock-v1';

function readDockPreference(): boolean {
  try { return window.sessionStorage.getItem(DOCK_PREF_KEY) === 'collapsed'; }
  catch { return false; }
}

function setDockCollapsed(collapsed: boolean, remember = true): void {
  document.body.dataset.opsDock = collapsed ? 'collapsed' : 'expanded';
  const button = document.querySelector<HTMLButtonElement>('#ops-dock-toggle');
  button?.setAttribute('aria-expanded', String(!collapsed));
  button?.setAttribute('aria-label', collapsed ? 'Expand navigation' : 'Collapse navigation');
  button?.setAttribute('title', collapsed ? 'Expand navigation (Ctrl/⌘ B)' : 'Collapse navigation (Ctrl/⌘ B)');
  if (button) button.textContent = collapsed ? '›' : '‹';
  // Do not leave the dock showing an accidentally filtered subset of icons.
  // Search is a temporary navigation aid, not a persistent route setting.
  if (collapsed) {
    const search = document.querySelector<HTMLInputElement>('#ops-nav-search');
    if (search && search.value) {
      search.value = '';
      search.dispatchEvent(new Event('input', { bubbles: true }));
    }
  }
  if (remember) {
    try { window.sessionStorage.setItem(DOCK_PREF_KEY, collapsed ? 'collapsed' : 'expanded'); }
    catch { /* Navigation remains usable when storage is blocked. */ }
  }
}

function installNavigation(sidebar: HTMLElement, topbar: HTMLElement, grid: HTMLElement): void {
  sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
    button.addEventListener('click', () => {
      const view = button.dataset.opsNav as View;
      applyView(view, true);
    });
  });

  const search = sidebar.querySelector<HTMLInputElement>('#ops-nav-search');
  const mobileToggle = topbar.querySelector<HTMLButtonElement>('#ops-sidebar-toggle');
  const commandButton = topbar.querySelector<HTMLButtonElement>('#ops-command-trigger');
  setDockCollapsed(readDockPreference(), false);
  sidebar.querySelector<HTMLButtonElement>('#ops-dock-toggle')?.addEventListener('click', () => {
    setDockCollapsed(document.body.dataset.opsDock !== 'collapsed');
  });

  const focusSearch = (): void => {
    if (window.matchMedia('(max-width: 900px)').matches) {
      setMobileNavigation(true);
    } else {
      setDockCollapsed(false);
    }
    search?.focus();
    search?.select();
  };
  commandButton?.addEventListener('click', focusSearch);
  search?.addEventListener('input', () => {
    const query = search.value.trim().toLowerCase();
    let visible = 0;
    sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
      button.hidden = Boolean(query) && !(button.dataset.search ?? '').includes(query);
      if (!button.hidden) visible++;
    });
    sidebar.querySelectorAll<HTMLElement>('.ops-nav-group').forEach((group) => {
      const anyVisible = Array.from(group.querySelectorAll<HTMLButtonElement>('[data-ops-nav]')).some((item) => !item.hidden);
      group.hidden = !anyVisible;
    });
    const feedback = sidebar.querySelector<HTMLElement>('#ops-nav-search-feedback');
    if (feedback) {
      feedback.hidden = !query || visible !== 0;
      feedback.textContent = query && visible === 0 ? 'No matching admin sections' : '';
    }
  });
  search?.addEventListener('keydown', (event) => {
    if (event.key !== 'Enter') return;
    const first = Array.from(sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]')).find((button) => !button.hidden);
    first?.click();
  });

  mobileToggle?.addEventListener('click', () => {
    const opening = !sidebar.classList.contains('open');
    setMobileNavigation(opening);
    if (opening) search?.focus();
  });
  sidebar.querySelector<HTMLButtonElement>('#ops-sidebar-close')?.addEventListener('click', () => {
    setMobileNavigation(false);
    mobileToggle?.focus();
  });
  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && !event.altKey && !event.shiftKey) {
      if (event.key.toLowerCase() === 'k') {
        event.preventDefault();
        focusSearch();
        return;
      }
      if (event.key.toLowerCase() === 'b') {
        const element = event.target;
        if (element instanceof HTMLElement
          && element.closest('input, textarea, select, [contenteditable="true"]')) {
          return;
        }
        event.preventDefault();
        if (window.matchMedia('(max-width: 900px)').matches) {
          setMobileNavigation(!sidebar.classList.contains('open'));
        } else {
          setDockCollapsed(document.body.dataset.opsDock !== 'collapsed');
        }
        return;
      }
    }
    if (event.key === 'Escape' && sidebar.classList.contains('open')) {
      setMobileNavigation(false);
      mobileToggle?.focus();
    } else if (event.key === 'Escape' && document.activeElement === search) {
      search?.blur();
      commandButton?.focus();
    }
  });
  document.addEventListener('click', (event) => {
    const target = event.target;
    if (target instanceof Node && sidebar.classList.contains('open')
      && !sidebar.contains(target) && !mobileToggle?.contains(target)) {
      setMobileNavigation(false);
    }
  });

  window.addEventListener('hashchange', () => applyView(viewFromHash(), false));
  wireDashboardButtons();
  classifyCards();
  applyView(viewFromHash(), false);

  grid.querySelectorAll<HTMLElement>('.card').forEach((card) => {
    card.classList.add('ops-managed-card');
  });
}

// A live Market Core panel updates many times per minute. Observing the
// entire grid for those inner mutations and reapplying card visibility is
// unnecessary and can disturb scroll anchoring on long admin forms.
const DASHBOARD_SOURCE_SELECTOR = [
  '#runtime-status',
  '#release-status',
  '#feed-summary',
  '#observability-health',
  '#browser-transport-status',
  '#five-second-day-health-state',
  '.five-second-symbol-row',
].join(', ');

function isDashboardSourceMutation(mutation: MutationRecord): boolean {
  const target = mutation.target instanceof Element
    ? mutation.target
    : mutation.target.parentElement;
  if (!target || target.closest('#ops-dashboard-card')) return false;
  if (target.closest(DASHBOARD_SOURCE_SELECTOR)) return true;
  return Array.from(mutation.addedNodes).some((node) => (
    node instanceof Element && (
      node.matches(DASHBOARD_SOURCE_SELECTOR) ||
      Boolean(node.querySelector(DASHBOARD_SOURCE_SELECTOR))
    )
  ));
}

let dashboardSummaryObserver: MutationObserver | null = null;
let summaryMonitoringActive = false;

function setDashboardSummaryMonitoring(active: boolean): void {
  const grid = document.querySelector<HTMLElement>('#ops-content-grid');
  if (!dashboardSummaryObserver || !grid || active === summaryMonitoringActive) return;
  if (active) {
    dashboardSummaryObserver.observe(grid, { childList: true, subtree: true, characterData: true });
  } else {
    dashboardSummaryObserver.disconnect();
  }
  summaryMonitoringActive = active;
}

function installMutationSync(grid: HTMLElement): void {
  // Only changes to the list of top-level cards require route classification.
  // Inner text updates (telemetry, indicator catalog, editor state) do not.
  const cardObserver = new MutationObserver((mutations) => {
    const addedCard = mutations.some((mutation) =>
      Array.from(mutation.addedNodes).some((node) =>
        node instanceof HTMLElement && node.matches('.card')
      )
    );
    if (!addedCard) return;

    classifyCards();
    const active = (document.body.dataset.opsView as View) || viewFromHash();
    grid.querySelectorAll<HTMLElement>(':scope > .card').forEach((card) => {
      card.classList.add('ops-managed-card');
      const shouldHide = card.dataset.opsView !== active;
      if (card.hidden !== shouldHide) card.hidden = shouldHide;
    });
    if (active === 'dashboard') queueDashboardRender();
  });
  cardObserver.observe(grid, { childList: true });

  // Refresh only dashboard summaries from the specific source panels.
  // Crucially, this observer NEVER changes route visibility or scroll position.
  dashboardSummaryObserver = new MutationObserver((mutations) => {
    if (document.body.dataset.opsView === 'dashboard'
      && mutations.some(isDashboardSourceMutation)) {
      queueDashboardRender();
    }
  });
  setDashboardSummaryMonitoring(document.body.dataset.opsView === 'dashboard');
}

installShell();
