import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';

import { closeThemePopup } from './theme';

export type VelaZoomSpeed = 'slow' | 'balanced' | 'fast';

const STORAGE_KEY = 'qnext-vela-wheel-zoom-v1';
let registered = false;
let selectedSpeed: VelaZoomSpeed = 'balanced';

export function wheelZoomFactor(speed: VelaZoomSpeed): number {
  return speed === 'slow' ? 0.25 : speed === 'balanced' ? 0.5 : 1;
}

/** Preserve sideways/Shift+wheel history panning; adjust only vertical zoom. */
export function shouldScaleWheelZoom(
  deltaX: number,
  deltaY: number,
  shiftKey: boolean,
  speed: VelaZoomSpeed,
): boolean {
  return speed !== 'fast'
    && !shiftKey
    && Number.isFinite(deltaX)
    && Number.isFinite(deltaY)
    && deltaY !== 0
    && Math.abs(deltaY) >= Math.abs(deltaX);
}

/** Also cap unusually large device deltas to avoid a single huge zoom jump. */
export function scaledWheelZoomDelta(deltaY: number, speed: VelaZoomSpeed): number {
  if (speed === 'fast') return deltaY;
  const scaled = deltaY * wheelZoomFactor(speed);
  return Math.max(-120, Math.min(120, scaled));
}

function readSpeed(): VelaZoomSpeed {
  try {
    const value = window.localStorage.getItem(STORAGE_KEY);
    return value === 'slow' || value === 'balanced' || value === 'fast' ? value : 'balanced';
  } catch {
    return 'balanced';
  }
}

function writeSpeed(value: VelaZoomSpeed): void {
  selectedSpeed = value;
  try {
    window.localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // Private/restricted storage: this session still uses the selected speed.
  }
}


type PopupChoice = { value: VelaZoomSpeed; label: string; icon: string };

const POPUP_CHOICES: readonly PopupChoice[] = [
  { value: 'slow', label: 'Slow zoom', icon: '<path d="M5 12h14"/><path d="M7 8v8"/>' },
  { value: 'balanced', label: 'Balanced zoom', icon: '<path d="M4 16a8 8 0 1 1 16 0"/><path d="M12 16l4-6"/><circle cx="12" cy="16" r="1"/>' },
  { value: 'fast', label: 'Fast zoom (Vela default)', icon: '<path d="m13 2-9 12h7l-1 8 10-12h-7l0-8z"/>' },
];

let closeActivePopup: (() => void) | null = null;
let currentPopupAnchor: HTMLElement | null = null;

export function closeVelaZoomPopup(restoreFocus = false): void {
  const cleanup = closeActivePopup;
  closeActivePopup = null;
  cleanup?.();
  if (restoreFocus && currentPopupAnchor?.isConnected) currentPopupAnchor.focus();
  currentPopupAnchor = null;
}

function icon(paths: string): SVGSVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', '22');
  svg.setAttribute('height', '22');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.8');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = paths; // Built-in SVG paths only; no user-provided content.
  return svg;
}

/** Compact transient control, using the same positioning approach as Appearance. */
export function toggleVelaZoomPopup(anchor: HTMLElement | null): void {
  if (!anchor) return;
  const wasOpen = closeActivePopup !== null;
  closeVelaZoomPopup();
  if (wasOpen) return;
  closeThemePopup();

  const popup = document.createElement('div');
  popup.id = 'qnext-zoom-popover';
  popup.className = 'qnext-zoom-popover';
  popup.setAttribute('role', 'dialog');
  popup.setAttribute('aria-label', 'Chart zoom speed');

  const buttons: HTMLButtonElement[] = [];
  for (const choice of POPUP_CHOICES) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'qnext-zoom-choice';
    button.title = choice.label;
    button.setAttribute('aria-label', choice.label);
    button.setAttribute('aria-pressed', String(selectedSpeed === choice.value));
    if (selectedSpeed === choice.value) button.classList.add('is-selected');
    button.append(icon(choice.icon));
    button.addEventListener('click', () => {
      writeSpeed(choice.value);
      closeVelaZoomPopup(true);
    });
    popup.append(button);
    buttons.push(button);
  }

  document.body.append(popup);
  currentPopupAnchor = anchor;
  anchor.setAttribute('aria-expanded', 'true');
  anchor.setAttribute('aria-controls', popup.id);
  anchor.setAttribute('aria-haspopup', 'dialog');

  const position = () => {
    const origin = anchor.getBoundingClientRect();
    const bounds = popup.getBoundingClientRect();
    const left = Math.max(8, Math.min(origin.right - bounds.width, window.innerWidth - bounds.width - 8));
    const below = origin.bottom + 8;
    const top = below + bounds.height <= window.innerHeight - 8
      ? below
      : Math.max(8, origin.top - bounds.height - 8);
    popup.style.left = Math.round(left) + 'px';
    popup.style.top = Math.round(top) + 'px';
  };

  const onPointerDown = (event: PointerEvent) => {
    if (!(event.target instanceof Node)) return;
    if (!popup.contains(event.target) && !anchor.contains(event.target)) closeVelaZoomPopup();
  };
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      closeVelaZoomPopup(true);
    } else if (event.key === 'Tab') {
      closeVelaZoomPopup();
    } else if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
      event.preventDefault();
      const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === 'Home' ? 0
        : event.key === 'End' ? buttons.length - 1
          : (current + (event.key === 'ArrowRight' ? 1 : buttons.length - 1)) % buttons.length;
      buttons[next]?.focus();
    }
  };
  const onResize = () => closeVelaZoomPopup();

  closeActivePopup = () => {
    popup.remove();
    anchor.setAttribute('aria-expanded', 'false');
    anchor.removeAttribute('aria-controls');
    document.removeEventListener('pointerdown', onPointerDown, true);
    document.removeEventListener('keydown', onKeyDown, true);
    window.removeEventListener('resize', onResize);
  };
  document.addEventListener('pointerdown', onPointerDown, true);
  document.addEventListener('keydown', onKeyDown, true);
  window.addEventListener('resize', onResize);
  position();
  buttons[POPUP_CHOICES.findIndex((choice) => choice.value === selectedSpeed)]?.focus();
}

/**
 * Native icon-only Vela action immediately LEFT of the built-in Data Window
 * panel button. It launches a small popup without resizing the chart.
 */
export function registerVelaZoomSettings(): void {
  if (registered) return;
  registered = true;
  selectedSpeed = readSpeed();

  registerIcon(
    'qnext-zoom-settings',
    '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" xmlns="http://www.w3.org/2000/svg"><path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="2.5" fill="currentColor" stroke="none"/><circle cx="16" cy="17" r="2.5" fill="currentColor" stroke="none"/></svg>',
  );

  registerWidgetAction({
    id: 'qnext.zoom.open',
    target: 'topbar',
    label: 'Zoom speed',
    icon: 'qnext-zoom-settings',
    iconOnly: true,
    align: 'right',
    mobile: 'menu',
    run: () => {
      const focused = document.activeElement;
      const app = document.getElementById('app');
      const anchor = focused instanceof HTMLElement && app?.contains(focused)
        ? focused
        : app?.querySelector<HTMLElement>('[aria-label="Zoom speed"]')
          ?? app?.querySelector<HTMLElement>('[title="Zoom speed"]')
          ?? null;
      toggleVelaZoomPopup(anchor);
    },
  });
}

/**
 * Vela 0.8.4 hardcodes wheel sensitivity in InputController. Re-dispatch a
 * single reduced-delta event into its native handler rather than implementing
 * any competing viewport/zoom logic. The original event never reaches Vela.
 * Chart data, visibleRange, animations and anchors remain Vela-owned.
 */
export function installVelaWheelZoomControl(root: HTMLElement): () => void {
  const forwarded = new WeakSet<WheelEvent>();
  const onWheel = (event: WheelEvent): void => {
    if (forwarded.has(event)) return;
    if (!shouldScaleWheelZoom(event.deltaX, event.deltaY, event.shiftKey, selectedSpeed)) return;
    if (!event.cancelable) return;

    const target = event.target;
    // Vela installs its wheel handler on the interactive data canvas only.
    if (!(target instanceof HTMLCanvasElement)) return;
    if (!target.closest('.vela-workspace') || target.style.pointerEvents === 'none') return;

    const scaled = scaledWheelZoomDelta(event.deltaY, selectedSpeed);
    if (scaled === event.deltaY) return;

    const replacement = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      composed: true,
      deltaX: event.deltaX,
      deltaY: scaled,
      deltaZ: event.deltaZ,
      deltaMode: event.deltaMode,
      clientX: event.clientX,
      clientY: event.clientY,
      screenX: event.screenX,
      screenY: event.screenY,
      ctrlKey: event.ctrlKey,
      metaKey: event.metaKey,
      altKey: event.altKey,
      shiftKey: event.shiftKey,
    });
    forwarded.add(replacement);
    event.preventDefault();
    event.stopImmediatePropagation();
    target.dispatchEvent(replacement);
  };
  root.addEventListener('wheel', onWheel, { capture: true, passive: false });
  return () => root.removeEventListener('wheel', onWheel, { capture: true });
}
