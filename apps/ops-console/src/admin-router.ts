import './admin-polish.css';
import './admin-route-fixes.css';

export {};

type OpsView =
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
  | 'settings'
  | 'security';

const routeByView: Record<OpsView, string> = {
  dashboard: 'dashboard',
  market: 'market-data',
  candles: 'candles-history',
  synthetic: 'synthetic',
  indicators: 'indicators',
  intelligence: 'intelligence',
  lab: 'intelligence-lab',
  runtime: 'runtime',
  releases: 'releases',
  diagnostics: 'diagnostics',
  settings: 'settings',
  security: 'security',
};

const viewByRoute = new Map<string, OpsView>(
  Object.entries(routeByView).map(([view, route]) => [route, view as OpsView]),
);
viewByRoute.set('indicators-intelligence', 'intelligence');

const ADMIN_BASE = '/qnext/admin/';
const QNEXT_API_PREFIX = '/qnext/api/v1/';
const BROKEN_ADMIN_PUBLIC_API_PREFIX = '/qnext/admin/api/v1/';

function normalizedRoute(pathname = window.location.pathname): string {
  if (!pathname.startsWith(ADMIN_BASE)) return '';
  return pathname.slice(ADMIN_BASE.length).replace(/^\/+|\/+$/g, '');
}

function viewFromLocation(): OpsView {
  const route = normalizedRoute();
  const fromPath = viewByRoute.get(route);
  if (fromPath) return fromPath;

  const legacy = window.location.hash.replace(/^#/, '') as OpsView;
  if (legacy && routeByView[legacy]) return legacy;
  return 'dashboard';
}

function prettyURL(view: OpsView): string {
  const route = routeByView[view];
  return `${ADMIN_BASE}${route}/${window.location.search}`;
}

// admin-shell.ts still owns the actual view switch. Prime its legacy hash input
// before it mounts so a direct pretty-route request never paints Dashboard first.
// The hash is removed again as soon as the shell is ready.
function primeShellRouteFromPath(): void {
  const route = normalizedRoute();
  const view = viewByRoute.get(route);
  if (!view) return;
  const expected = `#${view}`;
  if (window.location.hash === expected) return;
  history.replaceState(history.state, '', `${window.location.pathname}${window.location.search}${expected}`);
}

primeShellRouteFromPath();

function installFetchSafetyNet(): void {
  const tagged = window as Window & { __qnextPrettyRouteFetchInstalled?: boolean };
  if (tagged.__qnextPrettyRouteFetchInstalled) return;
  tagged.__qnextPrettyRouteFetchInstalled = true;

  const nativeFetch = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    try {
      const raw = input instanceof Request ? input.url : input.toString();
      const url = new URL(raw, window.location.origin);
      if (url.origin === window.location.origin && url.pathname.startsWith(BROKEN_ADMIN_PUBLIC_API_PREFIX)) {
        url.pathname = QNEXT_API_PREFIX + url.pathname.slice(BROKEN_ADMIN_PUBLIC_API_PREFIX.length);
        if (input instanceof Request) {
          input = new Request(url.toString(), input);
        } else {
          input = url;
        }
      }
    } catch {
      // Native fetch will surface malformed input consistently.
    }
    return nativeFetch(input, init);
  };
}

installFetchSafetyNet();

let suppressNavigationCapture = false;

function activateView(view: OpsView): void {
  const selector = `[data-ops-nav="${view}"]`;
  const button = document.querySelector<HTMLButtonElement>(selector);
  if (!button) return;

  suppressNavigationCapture = true;
  try {
    if (document.body.dataset.opsView !== view) button.click();
    history.replaceState({ qnextOpsView: view }, '', prettyURL(view));
  } finally {
    suppressNavigationCapture = false;
  }
}

function installPrettyNavigation(): void {
  const sidebar = document.querySelector('#ops-sidebar');
  if (!sidebar) {
    window.setTimeout(installPrettyNavigation, 25);
    return;
  }

  activateView(viewFromLocation());

  document.addEventListener('click', (event) => {
    if (suppressNavigationCapture) return;
    const target = event.target as Element | null;
    const control = target?.closest<HTMLElement>('[data-ops-nav], [data-dashboard-view]');
    if (!control) return;

    const raw = control.dataset.opsNav ?? control.dataset.dashboardView;
    if (!raw || !(raw in routeByView)) return;
    const view = raw as OpsView;
    const prior = `${window.location.pathname}${window.location.search}`;

    queueMicrotask(() => {
      if (suppressNavigationCapture) return;
      const next = prettyURL(view);
      const nextWithoutOrigin = new URL(next, window.location.origin);
      if (`${nextWithoutOrigin.pathname}${nextWithoutOrigin.search}` === prior) {
        history.replaceState({ qnextOpsView: view }, '', next);
        return;
      }
      history.replaceState(history.state, '', prior);
      history.pushState({ qnextOpsView: view }, '', next);
    });
  });

  window.addEventListener('popstate', () => {
    activateView(viewFromLocation());
  });
}

function revealAdmin(): void {
  document.documentElement.classList.remove('ops-booting');
  document.documentElement.classList.add('ops-ready');
}

window.addEventListener('qnext:admin-modules-ready', () => {
  installPrettyNavigation();
  requestAnimationFrame(revealAdmin);
}, { once: true });

window.setTimeout(() => {
  if (!document.documentElement.classList.contains('ops-ready')) {
    installPrettyNavigation();
    revealAdmin();
  }
}, 2500);
