export type ThemePreference = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'qnext-theme-preference';
const CHOICES: ThemePreference[] = ['light', 'dark', 'system'];
let sessionPreference: ThemePreference | null = null;
let systemListenerInstalled = false;
let reloadSystemTheme = false;

export function resolveTheme(preference: ThemePreference, systemDark: boolean): 'dark' | 'light' {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}

export function getThemePreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light' || stored === 'system') return stored;
  } catch { /* storage can be disabled */ }
  return sessionPreference ?? 'dark';
}

function prefersDark(): boolean {
  return typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export function currentTheme(): 'dark' | 'light' {
  return resolveTheme(getThemePreference(), prefersDark());
}

export function applyTheme(): void {
  const theme = currentTheme();
  document.documentElement.dataset.qnextTheme = theme;
  document.documentElement.style.colorScheme = theme;
  window.dispatchEvent(new CustomEvent('qnext-theme-change', { detail: { theme } }));
}

export function setThemePreference(preference: ThemePreference): void {
  if (!CHOICES.includes(preference)) return;
  sessionPreference = preference;
  try {
    window.localStorage.setItem(STORAGE_KEY, preference);
  } catch { /* in-memory fallback for private browsing */ }
  applyTheme();
}

export function watchSystemTheme(reloadOnChange = false): void {
  reloadSystemTheme = reloadOnChange;
  if (systemListenerInstalled || typeof window.matchMedia !== 'function') return;
  systemListenerInstalled = true;
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (getThemePreference() !== 'system') return;
    applyTheme();
    if (reloadSystemTheme) window.location.reload();
  });
}

// Inline SVG icons use no icon fonts and never create visible labels.
export const themeIconPaths: Record<ThemePreference, string> = {
  light: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M2 12h2m16 0h2M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42"/>',
  dark: '<path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/>',
  system: '<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8m-4-4v4"/>',
};

function icon(preference: ThemePreference): SVGSVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', '18');
  svg.setAttribute('height', '18');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.8');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = themeIconPaths[preference];
  return svg;
}

let popupCleanup: (() => void) | undefined;

export function closeThemePopup(restoreFocus = false): void {
  if (!popupCleanup) return;
  const cleanup = popupCleanup;
  popupCleanup = undefined;
  cleanup();
  if (restoreFocus) {
    const active = document.getElementById('qnext-theme-trigger');
    active?.focus();
  }
}

function updateTrigger(trigger: HTMLButtonElement): void {
  const preference = getThemePreference();
  trigger.replaceChildren(icon(preference));
  trigger.setAttribute('aria-label', 'Appearance: ' + preference);
  trigger.title = 'Appearance: ' + preference;
}

export function toggleThemePopup(anchor: HTMLElement | null, reloadOnChange = false): void {
  if (!anchor) return;
  const wasOpen = Boolean(popupCleanup);
  closeThemePopup();
  if (wasOpen) return;

  const menu = document.createElement('div');
  menu.id = 'qnext-theme-menu';
  menu.className = 'qnext-theme-menu';
  menu.setAttribute('role', 'group');
  menu.setAttribute('aria-label', 'Appearance preference');
  const buttons: HTMLButtonElement[] = [];

  for (const preference of CHOICES) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'qnext-theme-choice';
    button.dataset.theme = preference;
    button.setAttribute('aria-label', preference === 'system' ? 'Use system appearance' : 'Use ' + preference + ' appearance');
    button.title = button.getAttribute('aria-label') ?? '';
    button.setAttribute('aria-pressed', String(getThemePreference() === preference));
    button.append(icon(preference));
    button.addEventListener('click', () => {
      setThemePreference(preference);
      const trigger = document.querySelector<HTMLButtonElement>('#qnext-theme-trigger');
      if (trigger) updateTrigger(trigger);
      closeThemePopup(true);
      if (reloadOnChange) window.location.reload();
    });
    menu.append(button);
    buttons.push(button);
  }

  document.body.append(menu);
  const anchorBounds = anchor.getBoundingClientRect();
  const popupBounds = menu.getBoundingClientRect();
  // Constrain the transient popover to the visual viewport, never the chart's
  // bottom-right overlays. No persistent fixed control remains on the canvas.
  const left = Math.max(8, Math.min(anchorBounds.right - popupBounds.width, window.innerWidth - popupBounds.width - 8));
  const below = anchorBounds.bottom + 8;
  const top = below + popupBounds.height <= window.innerHeight - 8
    ? below
    : Math.max(8, anchorBounds.top - popupBounds.height - 8);
  menu.style.left = left + 'px';
  menu.style.top = top + 'px';
  anchor.setAttribute('aria-expanded', 'true');
  anchor.setAttribute('aria-controls', menu.id);

  const close = () => {
    menu.remove();
    anchor.setAttribute('aria-expanded', 'false');
    anchor.removeAttribute('aria-controls');
    document.removeEventListener('pointerdown', onPointerDown, true);
    document.removeEventListener('keydown', onKeyDown, true);
    window.removeEventListener('resize', onResize);
  };
  const onPointerDown = (event: PointerEvent) => {
    if (!menu.contains(event.target as Node) && !anchor.contains(event.target as Node)) closeThemePopup();
  };
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      closeThemePopup();
      anchor.focus();
    } else if (event.key === 'Tab') {
      closeThemePopup();
    } else if (['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(event.key)) {
      event.preventDefault();
      const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === 'Home' ? 0
        : event.key === 'End' ? buttons.length - 1
          : (current + (event.key === 'ArrowRight' ? 1 : buttons.length - 1)) % buttons.length;
      buttons[next]?.focus();
    }
  };
  const onResize = () => closeThemePopup();
  popupCleanup = close;
  document.addEventListener('pointerdown', onPointerDown, true);
  document.addEventListener('keydown', onKeyDown, true);
  window.addEventListener('resize', onResize);
  buttons[CHOICES.indexOf(getThemePreference())]?.focus();
}

export function mountThemeControl(options: { container: HTMLElement; reloadOnChange?: boolean }): void {
  const { container, reloadOnChange = false } = options;
  closeThemePopup();
  let trigger = document.querySelector<HTMLButtonElement>('#qnext-theme-trigger');
  if (!trigger) {
    trigger = document.createElement('button');
    trigger.id = 'qnext-theme-trigger';
    trigger.type = 'button';
    trigger.className = 'qnext-theme-trigger';
    trigger.setAttribute('aria-haspopup', 'true');
    trigger.setAttribute('aria-expanded', 'false');
    trigger.addEventListener('click', () => toggleThemePopup(trigger, reloadOnChange));
  }
  updateTrigger(trigger);
  if (trigger.parentElement !== container) container.append(trigger);
  watchSystemTheme(reloadOnChange);
}
