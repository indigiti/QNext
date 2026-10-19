import { registerIcon, registerSidePanel } from '@luxalgo/vela/plugin';

export type VelaZoomSpeed = 'slow' | 'balanced' | 'fast';

const STORAGE_KEY = 'qnext-vela-wheel-zoom-v1';
const OPTIONS: ReadonlyArray<{ value: VelaZoomSpeed; label: string; description: string }> = [
  { value: 'slow', label: 'Slow', description: 'Precision zoom (25% of Vela default)' },
  { value: 'balanced', label: 'Balanced', description: 'Smooth zoom (50% of Vela default)' },
  { value: 'fast', label: 'Fast', description: 'Original Vela wheel sensitivity' },
];

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

/**
 * Register a native Vela panel icon between Data Window (order 10) and
 * Object Tree (order 20). The overlay panel does not resize/shift the viewport.
 */
export function registerVelaZoomSettings(): void {
  if (registered) return;
  registered = true;
  selectedSpeed = readSpeed();

  registerIcon(
    'qnext-zoom-settings',
    '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" xmlns="http://www.w3.org/2000/svg"><path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="2.5" fill="currentColor" stroke="none"/><circle cx="16" cy="17" r="2.5" fill="currentColor" stroke="none"/></svg>',
  );

  registerSidePanel({
    id: 'qnext.zoom-settings',
    title: 'Chart Zoom Settings',
    icon: 'qnext-zoom-settings',
    order: 15,
    width: 286,
    overlay: true,
    mount: (_ctx, body) => {
      body.classList.add('qnext-zoom-panel');

      const introduction = document.createElement('p');
      introduction.className = 'qnext-zoom-intro';
      introduction.textContent = 'Mouse-wheel and vertical trackpad zoom sensitivity';
      body.appendChild(introduction);

      const group = document.createElement('fieldset');
      group.className = 'qnext-zoom-options';
      const legend = document.createElement('legend');
      legend.textContent = 'Zoom speed';
      group.appendChild(legend);

      const inputs: Array<{ input: HTMLInputElement; row: HTMLLabelElement }> = [];
      for (const choice of OPTIONS) {
        const row = document.createElement('label');
        row.className = 'qnext-zoom-option';
        const input = document.createElement('input');
        input.type = 'radio';
        input.name = 'qnext-wheel-zoom-speed';
        input.value = choice.value;
        const content = document.createElement('span');
        const title = document.createElement('strong');
        title.textContent = choice.label;
        const description = document.createElement('small');
        description.textContent = choice.description;
        content.append(title, description);
        row.append(input, content);
        group.appendChild(row);
        inputs.push({ input, row });
        input.addEventListener('change', () => {
          if (!input.checked) return;
          writeSpeed(choice.value);
          updateSelection();
        });
      }

      function updateSelection(): void {
        for (const { input, row } of inputs) {
          input.checked = input.value === selectedSpeed;
          row.classList.toggle('is-selected', input.checked);
        }
      }

      updateSelection();
      body.appendChild(group);

      const note = document.createElement('p');
      note.className = 'qnext-zoom-note';
      note.textContent = 'Applies immediately and is saved in this browser. Horizontal panning, drawings and chart position remain unchanged.';
      body.appendChild(note);

      return { onOpen: updateSelection };
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
