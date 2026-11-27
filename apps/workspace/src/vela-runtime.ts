// Loaded only for the selected Vela workspace. Keep this module free of
// bootstrap side effects; registration and construction stay in main.ts.
export { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';
export { VelaWorkspace } from '@luxalgo/vela/workspace';
export { createQNextPineEngine, resolveQNextPineMode, withConfirmedMtfRecompute } from './pine-engine';
export { VELA_INITIAL_HISTORY } from './charts/history-pagination';
export { installVelaPanHistory } from './charts/vela-history';
export { VelaSignalTrace, installVelaSignalTrace } from './charts/vela-signal-trace';
export { QNextIndicatorEngine } from './qnext-indicator-engine';
export { mountIntelligenceLabShadowBridge } from './intelligence-lab-shadow-bridge';
export { registerVelaAppearanceAction } from './vela-theme-action';
export { registerVelaScreenAwakeAction } from './vela-screen-awake-action';
export { installVelaWheelZoomControl, registerVelaZoomSettings } from './vela-zoom-settings';
export { registerIndicatorTemplates } from './indicator-templates';
export {
  registerVelaChartAlertAction, syncVelaChartAlertAction,
  velaChartAlerts, withChartAlertSwitch,
} from './vela-alert-toggle';
