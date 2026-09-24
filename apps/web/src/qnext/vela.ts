import type { QNextProvider } from "./QNextProvider.js";
import type { ProviderStatus, QNextBar } from "./types.js";

export interface VelaChartPort {
  replaceBars(bars: readonly QNextBar[]): void;
  setConnectionStatus(status: ProviderStatus): void;
}

/**
 * Thin presentation binding. The Vela side receives canonical browser state;
 * it never owns market normalization, authority, candle construction, or replay.
 */
export function bindQNextProviderToVela(provider: QNextProvider, chart: VelaChartPort): () => void {
  return provider.subscribe((event) => {
    if (event.type === "bars") chart.replaceBars(event.bars);
    if (event.type === "status") chart.setConnectionStatus(event.status);
  });
}
