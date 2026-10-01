import './admin-shell.css';

export {};

type View =
  | 'dashboard'
  | 'market'
  | 'candles'
  | 'synthetic'
  | 'trading'
  | 'runtime'
  | 'releases'
  | 'diagnostics'
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
  trading: {
    title: 'Indicators & Intelligence',
    description: 'Custom indicators and controlled Quant Intelligence lifecycle.',
  },
  runtime: {
    title: 'Runtime',
    description: 'Market Core state and service lifecycle controls.',
  },
  releases: {
    title: 'Releases',
    description: 'Activate a release or roll back to the previous deployment.',
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
    label: 'Market Data',
    items: [
      { view: 'market', label: 'Feed & 5s Health', icon: '◉' },
      { view: 'candles', label: 'Candles & History', icon: '▥' },
    ],
  },
  {
    label: 'Research & Trading',
    items: [
      { view: 'synthetic', label: 'NIFTY SYN+', icon: '◇' },
      { view: 'trading', label: 'Indicators & Intelligence', icon: '⌁' },
    ],
  },
  {
    label: 'System',
    items: [
      { view: 'runtime', label: 'Runtime', icon: '⚡' },
      { view: 'releases', label: 'Releases', icon: '⬆' },
      { view: 'diagnostics', label: 'Diagnostics', icon: '▤' },
      { view: 'settings', label: 'Settings', icon: '⚙' },
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

  setCardView(document.querySelector<HTMLElement>('#feed-status-card'), 'market');
  setCardView(document.querySelector<HTMLElement>('#active-markets-card'), 'market');

  setCardView(document.querySelector<HTMLElement>('#candle-timeframes-card'), 'candles');
  setCardView(document.querySelector<HTMLElement>('#chart-timeframes-card'), 'candles');
  setCardView(document.querySelector<HTMLElement>('#historical-repair-card'), 'candles');

  setCardView(document.querySelector<HTMLElement>('#syn-plus-status-card'), 'synthetic');
  setCardView(document.querySelector<HTMLElement>('#custom-indicators-card'), 'trading');
  setCardView(document.querySelector<HTMLElement>('#quant-intelligence-card'), 'trading');

  setCardView(document.querySelector<HTMLElement>('#workspace-settings-card'), 'settings');
  setCardView(findCard('#secret-form'), 'settings', 'broker-secrets-card');
  setCardView(findCard('#config-editor'), 'settings', 'market-config-card');
  setCardView(document.querySelector<HTMLElement>('#admin-access-card'), 'security');

  document.querySelectorAll<HTMLElement>('main.grid > .card:not([data-ops-view])').forEach((card) => {
    const id = card.id.toLowerCase();
    const text = `${id} ${card.querySelector('.eyebrow')?.textContent ?? ''} ${card.querySelector('h2')?.textContent ?? ''}`.toLowerCase();
    if (text.includes('intelligence') || text.includes('indicator') || text.includes('strategy')) {
      card.dataset.opsView = 'trading';
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
        <button type="button" class="ops-nav-item" data-ops-nav="${item.view}" data-search="${html(`${group.label} ${item.label}`.toLowerCase())}">
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
      <div>
        <strong>QNeXT</strong>
        <span>Operations</span>
      </div>
      <button type="button" id="ops-sidebar-close" class="ops-sidebar-close" aria-label="Close navigation">×</button>
    </div>
    <label class="ops-search" for="ops-nav-search">
      <span aria-hidden="true">⌕</span>
      <input id="ops-nav-search" type="search" autocomplete="off" placeholder="Quick navigation" />
    </label>
    <nav class="ops-nav" aria-label="QNext Admin navigation">
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
  mobileButton.setAttribute('aria-label', 'Open navigation');
  mobileButton.textContent = '☰';
  topbar.prepend(mobileButton);

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

function miniFiveSecondRows(): string {
  const rows = Array.from(document.querySelectorAll<HTMLElement>('.five-second-symbol-row'));
  if (rows.length === 0) {
    return '<div class="ops-empty-state">5s health is warming. The dashboard will populate from persisted current-day history.</div>';
  }

  return rows.map((row) => {
    const symbol = row.dataset.symbol ?? row.querySelector('.five-second-symbol-title strong')?.textContent ?? '—';
    const kpis = Array.from(row.querySelectorAll<HTMLElement>('.five-second-symbol-kpis > span'));
    const health = kpis.find((item) => item.textContent?.trim().startsWith('Health'))?.querySelector('strong')?.textContent ?? '—';
    const missing = kpis.find((item) => item.textContent?.trim().startsWith('Missing'))?.querySelector('strong')?.textContent ?? '0';
    const timeline = row.querySelector<HTMLElement>('.five-second-timeline')?.innerHTML ?? '';
    const status = row.querySelector<HTMLElement>('.five-second-symbol-title .badge')?.textContent?.trim() ?? '—';
    const bad = status === 'CHECK' || Number(missing) > 0;
    return `
      <button type="button" class="ops-health-row" data-dashboard-view="market" aria-label="Open ${html(symbol)} 5 second health">
        <span class="ops-health-symbol"><strong>${html(symbol)}</strong><small class="${bad ? 'is-bad' : ''}">${html(status)}</small></span>
        <span class="ops-health-timeline five-second-timeline">${timeline}</span>
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
  const transport = textFrom('#browser-transport-status', 'Probing…').replace(/^Browser transport:\s*/i, '');

  return `
    <section id="ops-dashboard-card" class="ops-dashboard" data-ops-view="dashboard">
      <div class="ops-dashboard-hero">
        <div>
          <p class="eyebrow">QNeXT Operations</p>
          <h2>System overview</h2>
          <p class="muted">A compact operational view. Open a section from the sidebar for full controls and diagnostics.</p>
        </div>
        <button type="button" class="secondary" data-dashboard-view="market">Open market health</button>
      </div>

      <div class="ops-kpi-grid">
        <button type="button" class="ops-kpi" data-dashboard-view="runtime">
          <span>Market Core</span><strong>${html(runtimeState)}</strong><small>Runtime & services</small>
        </button>
        <button type="button" class="ops-kpi" data-dashboard-view="market">
          <span>Market data</span><strong>${html(feedState)}</strong><small>${html(transport)}</small>
        </button>
        <button type="button" class="ops-kpi" data-dashboard-view="market">
          <span>Canonical 5s</span><strong>${html(fiveSecondState)}</strong><small>Current-day integrity</small>
        </button>
        <button type="button" class="ops-kpi" data-dashboard-view="releases">
          <span>Release</span><strong>${html(release)}</strong><small>Deploy / rollback</small>
        </button>
      </div>

      <div class="ops-dashboard-grid">
        <section class="ops-panel ops-panel-wide">
          <div class="ops-panel-head">
            <div><span class="eyebrow">Current day</span><h3>5s candle health</h3></div>
            <button type="button" class="secondary compact" data-dashboard-view="market">View details</button>
          </div>
          <div class="ops-health-list">${miniFiveSecondRows()}</div>
        </section>

        <section class="ops-panel">
          <div class="ops-panel-head"><div><span class="eyebrow">Quick access</span><h3>Operations</h3></div></div>
          <div class="ops-quick-grid">
            <button type="button" data-dashboard-view="candles"><span>▥</span><strong>Candles</strong><small>Formation & recovery</small></button>
            <button type="button" data-dashboard-view="synthetic"><span>◇</span><strong>SYN+</strong><small>Research synthetic</small></button>
            <button type="button" data-dashboard-view="trading"><span>⌁</span><strong>Intelligence</strong><small>Indicators & models</small></button>
            <button type="button" data-dashboard-view="diagnostics"><span>▤</span><strong>Diagnostics</strong><small>Logs & runtime</small></button>
          </div>
        </section>
      </div>
    </section>
  `;
}

let dashboardRenderQueued = false;

function renderDashboard(): void {
  const dashboard = document.querySelector<HTMLElement>('#ops-dashboard-card');
  if (!dashboard) return;
  const active = document.body.dataset.opsView === 'dashboard';
  const replacement = document.createElement('div');
  replacement.innerHTML = dashboardMarkup().trim();
  const next = replacement.firstElementChild as HTMLElement;
  dashboard.replaceWith(next);
  wireDashboardButtons();
  if (active) applyView('dashboard', false);
}

function queueDashboardRender(): void {
  if (dashboardRenderQueued) return;
  dashboardRenderQueued = true;
  window.setTimeout(() => {
    dashboardRenderQueued = false;
    renderDashboard();
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

function applyView(view: View, updateHash: boolean): void {
  const meta = views[view];
  document.body.dataset.opsView = view;

  document.querySelectorAll<HTMLElement>('main.grid > [data-ops-view]').forEach((section) => {
    section.hidden = section.dataset.opsView !== view;
  });

  document.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
    const active = button.dataset.opsNav === view;
    button.classList.toggle('active', active);
    button.setAttribute('aria-current', active ? 'page' : 'false');
  });

  const heading = document.querySelector<HTMLElement>('.ops-page-heading');
  if (heading) {
    const h1 = heading.querySelector('h1');
    const description = heading.querySelector('.muted');
    if (h1) h1.textContent = meta.title;
    if (description) description.textContent = meta.description;
  }

  document.querySelector('#ops-sidebar')?.classList.remove('open');
  if (updateHash) history.replaceState(null, '', `#${view}`);
  window.scrollTo({ top: 0, behavior: 'auto' });
}

function viewFromHash(): View {
  const hash = window.location.hash.replace(/^#/, '') as View;
  return views[hash] ? hash : 'dashboard';
}

function installNavigation(sidebar: HTMLElement, topbar: HTMLElement, grid: HTMLElement): void {
  sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
    button.addEventListener('click', () => {
      const view = button.dataset.opsNav as View;
      applyView(view, true);
    });
  });

  const search = sidebar.querySelector<HTMLInputElement>('#ops-nav-search');
  search?.addEventListener('input', () => {
    const query = search.value.trim().toLowerCase();
    sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]').forEach((button) => {
      button.hidden = Boolean(query) && !(button.dataset.search ?? '').includes(query);
    });
    sidebar.querySelectorAll<HTMLElement>('.ops-nav-group').forEach((group) => {
      const anyVisible = Array.from(group.querySelectorAll<HTMLButtonElement>('[data-ops-nav]')).some((item) => !item.hidden);
      group.hidden = !anyVisible;
    });
  });
  search?.addEventListener('keydown', (event) => {
    if (event.key !== 'Enter') return;
    const first = Array.from(sidebar.querySelectorAll<HTMLButtonElement>('[data-ops-nav]')).find((button) => !button.hidden);
    first?.click();
  });

  topbar.querySelector<HTMLButtonElement>('#ops-sidebar-toggle')?.addEventListener('click', () => {
    sidebar.classList.toggle('open');
  });
  sidebar.querySelector<HTMLButtonElement>('#ops-sidebar-close')?.addEventListener('click', () => {
    sidebar.classList.remove('open');
  });

  window.addEventListener('hashchange', () => applyView(viewFromHash(), false));
  wireDashboardButtons();
  classifyCards();
  applyView(viewFromHash(), false);

  grid.querySelectorAll<HTMLElement>('.card').forEach((card) => {
    card.classList.add('ops-managed-card');
  });
}

function mutationShouldRefresh(mutation: MutationRecord, grid: HTMLElement): boolean {
  const target = mutation.target instanceof Element
    ? mutation.target
    : mutation.target.parentElement;
  if (target?.closest('#ops-dashboard-card')) return false;

  if (mutation.target === grid) {
    const added = Array.from(mutation.addedNodes).filter((node): node is HTMLElement => node instanceof HTMLElement);
    if (added.length > 0 && added.every((node) => node.id === 'ops-dashboard-card')) return false;
  }
  return true;
}

function installMutationSync(grid: HTMLElement): void {
  let applying = false;
  const observer = new MutationObserver((mutations) => {
    if (applying || !mutations.some((mutation) => mutationShouldRefresh(mutation, grid))) return;
    applying = true;
    classifyCards();
    grid.querySelectorAll<HTMLElement>('.card').forEach((card) => card.classList.add('ops-managed-card'));
    queueDashboardRender();
    applyView((document.body.dataset.opsView as View) || viewFromHash(), false);
    applying = false;
  });

  observer.observe(grid, { childList: true, subtree: true, characterData: true });
}

installShell();
