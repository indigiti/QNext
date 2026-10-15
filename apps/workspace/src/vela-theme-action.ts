import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';

import { toggleThemePopup, watchSystemTheme } from './theme';

let registered = false;

/**
 * Native Vela topbar action. Unlike a fixed overlay, this participates in
 * Vela's existing toolbar and mobile overflow, without touching market data.
 */
export function registerVelaAppearanceAction(): void {
  if (registered) return;
  registered = true;

  registerIcon(
    'qnext-appearance-palette',
    '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 0 18z"/></svg>',
  );

  registerWidgetAction({
    id: 'qnext.appearance.open',
    target: 'topbar',
    label: 'Appearance',
    icon: 'qnext-appearance-palette',
    iconOnly: true,
    align: 'right',
    // Right-aligned actions use Vela's built-in mobile overflow.
    order: 1000,
    run: () => {
      const focused = document.activeElement;
      const app = document.getElementById('app');
      const anchor = focused instanceof HTMLElement && app?.contains(focused)
        ? focused
        : app?.querySelector<HTMLElement>('[aria-label="Appearance"]')
          ?? app?.querySelector<HTMLElement>('[title="Appearance"]')
          ?? null;
      toggleThemePopup(anchor, true);
    },
  });
  watchSystemTheme(true);
}
