import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';
import type { ScriptingEngine } from '@luxalgo/vela/plugin';

export const CHART_ALERTS_STORAGE_KEY = 'qnext-vela-chart-script-alerts-v1';
export const CHART_ALERTS_ACTION_ID = 'qnext.alerts.toggle';

/**
 * Client-side preference for notifications emitted by QNext's chart
 * scripting engines. On by default, including when storage is unavailable.
 * This is deliberately independent from broker/order/risk alerts.
 */
export class ChartAlertSwitch {
  private on: boolean;

  constructor(private readonly storage?: Pick<Storage, 'getItem' | 'setItem'>) {
    let value: string | null = null;
    try {
      value = storage?.getItem(CHART_ALERTS_STORAGE_KEY) ?? null;
    } catch {
      // Restricted browser storage must never disable indicator execution.
    }
    this.on = value !== 'off';
  }

  get enabled(): boolean { return this.on; }

  setEnabled(value: boolean, persist = true): void {
    this.on = value;
    if (!persist) return;
    try {
      this.storage?.setItem(CHART_ALERTS_STORAGE_KEY, value ? 'on' : 'off');
    } catch {
      // Still honors the choice for the current browser session.
    }
  }

  toggle(): boolean {
    this.setEnabled(!this.on);
    return this.on;
  }
}

/**
 * Guard the native Vela scripting-engine *alert* sink, before its chart
 * emits an alert and before Vela adds a toast/bell entry.
 *
 * The guard reads the current preference for EVERY emission. Toggling
 * while a PineWorkerEngine session is running takes effect immediately.
 * Models, markers, bgcolor/fill, warnings and error handling are untouched.
 */
export function withChartAlertSwitch(engine: ScriptingEngine, control: ChartAlertSwitch): ScriptingEngine {
  return {
    language: engine.language,
    capabilities: engine.capabilities,
    prepare: (source, instanceId) => engine.prepare(source, instanceId),
    execute: (req, handlers) => engine.execute(req, {
      ...handlers,
      onAlert: (alert) => {
        if (control.enabled) handlers.onAlert?.(alert);
      },
    }),
  };
}

function browserStorage(): Pick<Storage, 'getItem' | 'setItem'> | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}

export const velaChartAlerts = new ChartAlertSwitch(browserStorage());

function updateTogglePresentation(control: ChartAlertSwitch): void {
  const app = document.getElementById('app');
  if (app) app.dataset.qnextChartAlerts = control.enabled ? 'on' : 'off';
  // Vela owns the action button. Only adjust its accessibility metadata,
  // never its click handler or the Vela topbar's DOM subtree.
  const label = control.enabled
    ? 'Chart script alerts ON — click to turn OFF'
    : 'Chart script alerts OFF — click to turn ON';
  app?.querySelectorAll<HTMLButtonElement>(`[data-action-id="${CHART_ALERTS_ACTION_ID}"]`).forEach((button) => {
    button.setAttribute('aria-pressed', String(control.enabled));
    button.setAttribute('aria-label', label);
    button.setAttribute('title', label);
  });
}

let actionRegistered = false;

/**
 * An explicit Vela-native icon tool beside the built-in Alerts bell.
 * On mobile, Vela projects it into its native overflow menu. This only
 * mutes NEW script alert notifications in this chart workspace.
 */
export function registerVelaChartAlertAction(control: ChartAlertSwitch = velaChartAlerts): void {
  if (actionRegistered) return;
  actionRegistered = true;

  registerIcon(
    'qnext-alert-bell-control',
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 21h4"/></svg>',
  );

  registerWidgetAction({
    id: CHART_ALERTS_ACTION_ID,
    target: 'topbar',
    label: 'Toggle chart script alerts on/off',
    icon: 'qnext-alert-bell-control',
    iconOnly: true,
    align: 'right',
    mobile: 'menu',
    run: (context) => {
      const enabled = control.toggle();
      updateTogglePresentation(control);
      context.toast(enabled ? 'Chart script alerts ON' : 'Chart script alerts OFF', 'info');
    },
  });

  updateTogglePresentation(control);
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (event) => {
      if (event.key !== CHART_ALERTS_STORAGE_KEY) return;
      control.setEnabled(event.newValue !== 'off', false);
      updateTogglePresentation(control);
    });
  }
}

/** Call once after Vela mounts to reflect the saved preference on its button. */
export function syncVelaChartAlertAction(): void {
  updateTogglePresentation(velaChartAlerts);
}
