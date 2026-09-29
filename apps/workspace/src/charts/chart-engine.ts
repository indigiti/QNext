export type QNextChartEngine = 'vela' | 'lightweight' | 'auto';

export function resolveChartEngine(
  configured: QNextChartEngine | undefined,
  search = window.location.search,
): Exclude<QNextChartEngine, 'auto'> {
  const requested = new URLSearchParams(search).get('chartEngine');
  if (requested === 'lightweight' || requested === 'vela') {
    return requested;
  }

  if (configured === 'lightweight') {
    return 'lightweight';
  }

  // Keep Vela as the safe default until Lightweight reaches feature parity.
  return 'vela';
}
