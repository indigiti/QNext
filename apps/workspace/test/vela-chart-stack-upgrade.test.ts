/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';
import chartManifest from '../package.json?raw';
import workspaceSource from '../src/main.ts?raw';
import providerSource from '../src/qnext-provider.ts?raw';
import pineEngineSource from '../src/pine-engine.ts?raw';
import velaHistorySource from '../src/charts/vela-history.ts?raw';

const dependencies = (JSON.parse(chartManifest) as { dependencies: Record<string, string> }).dependencies;

describe('Vela 0.10.14 / chart stack upgrade safety', () => {
  it('pins the tested latest chart stack without unexpected PineTS engine jumps', () => {
    expect(dependencies['@luxalgo/vela']).toBe('0.10.14');
    expect(dependencies['@luxalgo/vela-pinets']).toBe('0.3.4');
    expect(dependencies['lightweight-charts']).toBe('5.3.0');
    expect(dependencies.pinets).toBe('0.13.1');
  });

  it('adds Vela native fullscreen without removing existing QNext toolbar controls', () => {
    expect(workspaceSource).toContain(
      "'actions', 'alerts', 'qnext.alerts.toggle', 'qnext.screen-awake.toggle', 'qnext.zoom.open', 'panels', 'screenshot', 'fullscreen'",
    );
    expect(workspaceSource).toContain("'symbol', 'timeframes', 'style', 'indicators', 'qnext.indicator.templates', 'undo-redo'");
    expect(workspaceSource).toContain('installVelaPanHistory(workspace)');
    expect(workspaceSource).toContain('persist: \'qnext-workspace-v2\'');
  });

  it('retains streaming Pine Worker, confirmed MTF reruns and signal instrumentation', () => {
    expect(workspaceSource).toContain('withConfirmedMtfRecompute(createQNextPineEngine(');
    expect(workspaceSource).toContain('installVelaSignalTrace(workspace, pineTrace)');
    expect(pineEngineSource).toContain('PineWorkerEngine');
    expect(pineEngineSource).toContain('withConfirmedMtfRecompute');
  });

  it('does not replace history on every tab focus or discard candle revision semantics', () => {
    expect(velaHistorySource).toContain('void chart.setMarket({ bars: target })');
    expect(velaHistorySource).toContain("chart.on('history:complete'");
    expect(velaHistorySource).toContain("chart.on('market:changed'");
    expect(providerSource).toContain('revision');
    expect(providerSource).toContain('timeframe');
  });

  it('keeps QNext two-section symbol picker and separate canonical data authority', () => {
    expect(workspaceSource).toContain('new VelaWorkspace');
    expect(workspaceSource).toContain('new QNextLightweightWorkspace');
    expect(providerSource).toContain('orderVisibleSymbols');
    expect(providerSource).toContain('CORE_GROUP_ID');
    expect(providerSource).toContain('NSE_MINUTE_GROUP_ID');
  });
});
