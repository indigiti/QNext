// Shared structural treatment for every authenticated specialist admin page.
// No telemetry requests, polling, broker operations, or chart elements are
// created by this module. It preserves existing card nodes and listeners.
export type ScreenRoute =
  | 'market' | 'candles' | 'synthetic' | 'indicators' | 'lab'
  | 'intelligence' | 'runtime' | 'storage' | 'diagnostics'
  | 'releases' | 'settings' | 'security';

export type ScreenPlan = {
  route: ScreenRoute;
  number: string;
  group: string;
  title: string;
  summary: string;
  scope: string;
  tone: 'sky' | 'lime' | 'coral' | 'violet';
  related: readonly { route: ScreenRoute; label: string }[];
};

// Route content names genuine QNext functions. No simulated KPIs or
// provider-health claims appear on these static scaffolds.
export const screenPlans: readonly ScreenPlan[] = [
  { route: 'market', number: '01', group: 'Market data / Live',
    title: 'Market data, without guesswork.',
    summary: 'Monitor provider streams, futures-volume authority, browser transport and the indexes currently enabled for live calculations.',
    scope: 'Live feeds & subscriptions', tone: 'sky',
    related: [{ route: 'candles', label: 'Candle formation' }, { route: 'synthetic', label: 'SYN+ quality' }] },
  { route: 'candles', number: '02', group: 'Market data / History',
    title: 'Candles & history.',
    summary: 'Configure canonical formation, chart-visible intervals and source-aware historical recovery from one consistent workspace.',
    scope: 'Formation, display & repair', tone: 'sky',
    related: [{ route: 'market', label: 'Live feed' }, { route: 'storage', label: 'Storage' }] },
  { route: 'synthetic', number: '03', group: 'Market data / Synthetic',
    title: 'Synthetic intelligence.',
    summary: 'Inspect SYN+ snapshots, volume attribution and isolated paper evaluation without confusing synthetic price with traded volume.',
    scope: 'SYN+ research and paper', tone: 'lime',
    related: [{ route: 'market', label: 'Feed authority' }, { route: 'lab', label: 'Research lab' }] },
  { route: 'indicators', number: '04', group: 'Research / Indicators',
    title: 'Build better signals.',
    summary: 'Search your indicator library, edit Pine v6 source and manage native scripts while leaving live chart execution isolated.',
    scope: 'Library & Pine editor', tone: 'lime',
    related: [{ route: 'lab', label: 'Intelligence Lab' }, { route: 'candles', label: 'Timeframes' }] },
  { route: 'lab', number: '05', group: 'Research / Experimentation',
    title: 'Experiment with discipline.',
    summary: 'Run isolated research, inspect challenger results and respect certification boundaries before anything reaches trading charts.',
    scope: 'Research & certification', tone: 'violet',
    related: [{ route: 'intelligence', label: 'Quant lifecycle' }, { route: 'indicators', label: 'Indicators' }] },
  { route: 'intelligence', number: '06', group: 'Research / Quant',
    title: 'Models under control.',
    summary: 'Review data, candidates, evaluation gates and production promotion or rollback with explicit model provenance.',
    scope: 'Candidates & promotion gates', tone: 'violet',
    related: [{ route: 'lab', label: 'Intelligence Lab' }, { route: 'diagnostics', label: 'Diagnostics' }] },
  { route: 'runtime', number: '07', group: 'Operations / Services',
    title: 'Keep Market Core in control.',
    summary: 'Inspect service state and run lifecycle or smoke-test actions. Service changes operate through the existing protected API.',
    scope: 'Privileged service controls', tone: 'coral',
    related: [{ route: 'market', label: 'Feed health' }, { route: 'diagnostics', label: 'Logs' }] },
  { route: 'storage', number: '08', group: 'Operations / Storage',
    title: 'Know where your data lives.',
    summary: 'Review disk usage, raw retention and canonical symbol-day inventory. File health does not prove candle or volume integrity.',
    scope: 'Capacity & retention', tone: 'sky',
    related: [{ route: 'candles', label: 'Candle history' }, { route: 'diagnostics', label: 'Diagnostics' }] },
  { route: 'diagnostics', number: '09', group: 'Operations / Diagnostics',
    title: 'Find the failure path.',
    summary: 'Inspect runtime telemetry and recent logs before deciding whether an issue belongs to a provider, Market Core or the browser.',
    scope: 'Runtime telemetry & logs', tone: 'coral',
    related: [{ route: 'runtime', label: 'Market Core' }, { route: 'market', label: 'Live streams' }] },
  { route: 'releases', number: '10', group: 'Operations / Deployments',
    title: 'Deploy with a way back.',
    summary: 'Inspect the installed release, activate a reviewed package or roll back through existing deployment controls.',
    scope: 'Privileged deployment actions', tone: 'coral',
    related: [{ route: 'runtime', label: 'Runtime' }, { route: 'diagnostics', label: 'Release diagnostics' }] },
  { route: 'settings', number: '11', group: 'Administration / Configuration',
    title: 'Your workspace, configured.',
    summary: 'Control workspace preferences, protected broker credentials and Market Core configuration with distinct, clearly labelled actions.',
    scope: 'Workspace & broker configuration', tone: 'lime',
    related: [{ route: 'security', label: 'Security' }, { route: 'market', label: 'Enabled markets' }] },
  { route: 'security', number: '12', group: 'Administration / Security',
    title: 'Access stays protected.',
    summary: 'Review administrator access and recovery controls. Protected actions remain behind the existing authenticated API.',
    scope: 'Identity & access controls', tone: 'violet',
    related: [{ route: 'settings', label: 'Settings' }, { route: 'diagnostics', label: 'Audit diagnostics' }] },
];

const plansByRoute = new Map(screenPlans.map((plan) => [plan.route, plan]));
const textSafe = (value: string): string => value.replace(/&/g, '&amp;')
  .replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&#039;');

function decorateLegacyCard(card: HTMLElement): void {
  // These two legacy panels lack the header/action structure used elsewhere.
  // Move only their existing headings; forms, handlers and API bindings stay put.
  if (card.id !== 'release-card' && card.id !== 'broker-secrets-card') return;
  if (card.querySelector(':scope > .card-head')) return;
  const headline = card.querySelector<HTMLElement>(':scope > h2');
  if (!headline) return;
  const eyebrow = card.querySelector<HTMLElement>(':scope > p.eyebrow');
  const detail = card.querySelector<HTMLElement>(':scope > p.muted');
  const head = document.createElement('div');
  head.className = 'card-head ops-card-head';
  const body = document.createElement('div');
  head.append(body);
  card.prepend(head);
  if (eyebrow) body.append(eyebrow);
  body.append(headline);
  if (detail) body.append(detail);
}

function createFrame(plan: ScreenPlan, navigate: (route: ScreenRoute) => void): HTMLElement {
  const section = document.createElement('section');
  section.className = 'ops-screen-frame';
  section.id = 'ops-screen-' + plan.route;
  section.dataset.opsView = plan.route;
  section.dataset.opsTone = plan.tone;
  section.hidden = true;
  section.setAttribute('aria-labelledby', section.id + '-title');

  section.innerHTML = `
    <div class="ops-screen-head">
      <div class="ops-screen-headcopy">
        <span class="ops-screen-path">${textSafe(plan.group)}</span>
        <span class="ops-screen-index">${textSafe(plan.number)} / 12</span>
        <h2 id="${section.id}-title" tabindex="-1">${textSafe(plan.title)}</h2>
        <p>${textSafe(plan.summary)}</p>
      </div>
      <div class="ops-screen-aside">
        <span class="ops-screen-aside-label">WORKSPACE SCOPE</span>
        <strong>${textSafe(plan.scope)}</strong>
        <span class="ops-screen-aside-note">QNeXT · authenticated operations</span>
      </div>
    </div>
    <div class="ops-screen-toolbar">
      <nav class="ops-screen-related" aria-label="Related QNeXT workspaces">
        ${plan.related.map((related) => `<button class="ops-related-link" type="button"
           data-ops-related="${related.route}">${textSafe(related.label)}
           <span aria-hidden="true">↗</span></button>`).join('')}
      </nav>
      <nav class="ops-screen-sections" aria-label="${textSafe(plan.route)} page sections" hidden></nav>
    </div>
  `;

  section.addEventListener('click', (event) => {
    if (!(event.target instanceof Element)) return;
    const related = event.target.closest<HTMLButtonElement>('[data-ops-related]');
    if (related) {
      const route = related.dataset.opsRelated as ScreenRoute;
      if (plansByRoute.has(route)) navigate(route);
      return;
    }
    const jump = event.target.closest<HTMLButtonElement>('[data-ops-jump]');
    if (!jump) return;
    const anchor = jump.dataset.opsJump;
    if (!anchor) return;
    const target = document.getElementById(anchor);
    if (!target || target.hidden) return;
    target.scrollIntoView({ behavior: 'auto', block: 'start' });
    const heading = target.querySelector<HTMLElement>('h2, h3');
    if (heading) {
      heading.tabIndex = -1;
      heading.focus({ preventScroll: true });
    }
  });
  return section;
}

export function installScreenPages(grid: HTMLElement, navigate: (route: ScreenRoute) => void): void {
  if (grid.querySelector(':scope > #ops-screen-market')) return;
  const fragment = document.createDocumentFragment();
  for (const plan of screenPlans) fragment.append(createFrame(plan, navigate));
  // Screen frames remain sibling elements to the original .card nodes.
  // Existing admin widgets and late-loaded modules require this invariant.
  grid.insertBefore(fragment, grid.querySelector(':scope > .card'));
}

export function syncScreenPageSections(grid: HTMLElement): void {
  for (const plan of screenPlans) {
    const frame = grid.querySelector<HTMLElement>('#ops-screen-' + plan.route);
    const nav = frame?.querySelector<HTMLElement>('.ops-screen-sections');
    if (!nav) continue;
    const cards = Array.from(grid.querySelectorAll<HTMLElement>(':scope > .card'))
      .filter((card) => card.dataset.opsView === plan.route);
    const summary: Array<{ id: string; title: string }> = [];

    cards.forEach((card, i) => {
      card.classList.add('ops-screen-card');
      card.dataset.opsTone = plan.tone;
      if (!card.id) card.id = 'ops-workspace-' + plan.route + '-' + String(i + 1);
      decorateLegacyCard(card);
      const heading = card.querySelector<HTMLElement>(':scope > .card-head h2, :scope > h2, h2');
      const title = heading?.textContent?.trim() || 'Workspace section ' + String(i + 1);
      summary.push({ id: card.id, title });
    });

    const signature = JSON.stringify(summary);
    if (nav.dataset.opsSignature === signature) continue;
    nav.dataset.opsSignature = signature;
    nav.replaceChildren();
    for (const [i, entry] of summary.entries()) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'ops-section-link';
      button.dataset.opsJump = entry.id;
      button.title = 'Jump to ' + entry.title;
      const count = document.createElement('span');
      count.className = 'ops-section-number';
      count.textContent = String(i + 1).padStart(2, '0');
      const label = document.createElement('span');
      label.textContent = entry.title;
      button.append(count, label);
      nav.append(button);
    }
    nav.hidden = summary.length === 0;
  }
}
