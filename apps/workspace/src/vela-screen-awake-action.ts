import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';

import { type ScreenAwakeController, screenAwakeText } from './screen-awake';

let registered = false;

/**
 * Pinned immediately after the built-in Alerts slot by QNext's Vela topbar.
 * Vela moves both right-side actions to its native mobile three-dot sheet.
 */
export function registerVelaScreenAwakeAction(controller: ScreenAwakeController): void {
  if (registered) return;
  registered = true;

  registerIcon(
    'qnext-screen-awake',
    '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" xmlns="http://www.w3.org/2000/svg"><rect x="4" y="3" width="16" height="15" rx="2"/><path d="M9 22h6M12 18v4M9 10l2 2 4-4"/></svg>',
  );

  registerWidgetAction({
    id: 'qnext.screen-awake.toggle',
    target: 'topbar',
    label: 'Screen Awake',
    icon: 'qnext-screen-awake',
    iconOnly: true,
    align: 'right',
    mobile: 'menu',
    run: (ctx) => {
      controller.toggle();
      const status = controller.status;
      ctx.toast(
        status === 'off'
          ? 'Screen Awake off'
          : status === 'unsupported'
            ? 'This browser does not support Screen Awake'
            : 'Screen Awake requested — ' + screenAwakeText(status).replace('Screen Awake: ', ''),
      );
    },
  });
}
