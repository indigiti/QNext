/** The fixed QNext index/SYN universe, shown in user-requested order. */
export const CORE_INDEX_SYMBOLS = [
  'NIFTY',
  'NIFTY-SYN',
  'NIFTY-SYN+',
  'BANKNIFTY',
  'BANKNIFTY-SYN',
  'MIDCPNIFTY',
  'MIDCPNIFTY-SYN',
  'FINNIFTY',
  'FINNIFTY-SYN',
  'SENSEX',
  'SENSEX-SYN',
  'BANKEX',
  'BANKEX-SYN',
] as const;

export const CORE_GROUP_ID = 'QNEXT-CORE';
export const OKX_GOLD_GROUP_ID = 'QNEXT-OKX-GOLD';
export const OKX_GOLD_GROUP_LABEL = 'Gold & Commodities · OKX';
export const NSE_MINUTE_GROUP_ID = 'QNEXT-NSE-1M';
export const CORE_GROUP_LABEL = 'Indices & SYN / SYN+';
export const NSE_MINUTE_GROUP_LABEL = 'Other NSE symbols · 1m history';

const coreOrder = new Map<string, number>(
  CORE_INDEX_SYMBOLS.map((ticker, index) => [ticker, index]),
);

export type SymbolSection = 'core' | 'nse_1m' | 'okx_gold';
export type SymbolWithSection = {
  ticker: string;
  instrument_id: string;
  description: string;
  type: string;
  prefix: string;
  currency: string;
  timezone: string;
  calendar_id: string;
  synthetic: boolean;
};

/**
 * Treat only non-synthetic NSE equities/additional NSE indices as the new
 * history-only universe; all pre-existing exchange indices and SYN variants
 * remain in Core. This does not expand broker subscription or storage scope.
 */
export function symbolSection(symbol: Pick<SymbolWithSection, 'ticker' | 'prefix' | 'type' | 'synthetic'>): SymbolSection {
  if (symbol.prefix.trim().toUpperCase() === 'OKX') return 'okx_gold';
  const ticker = symbol.ticker.trim().toUpperCase();
  if (coreOrder.has(ticker)) return 'core';
  const type = symbol.type.trim().toLowerCase();
  if (symbol.prefix.toUpperCase() === 'NSE' && !symbol.synthetic
    && (type === 'equity' || type === 'stock' || type === 'index')) {
    return 'nse_1m';
  }
  return 'core';
}

export function orderVisibleSymbols<T extends SymbolWithSection>(symbols: readonly T[]): T[] {
  return [...symbols].sort((a, b) => {
    const aSection = symbolSection(a);
    const bSection = symbolSection(b);
    if (aSection !== bSection) {
      const rank = { core: 0, nse_1m: 1, okx_gold: 2 };
      return rank[aSection] - rank[bSection];
    }
    if (aSection === 'core') {
      const aRank = coreOrder.get(a.ticker.toUpperCase()) ?? Number.MAX_SAFE_INTEGER;
      const bRank = coreOrder.get(b.ticker.toUpperCase()) ?? Number.MAX_SAFE_INTEGER;
      if (aRank !== bRank) return aRank - bRank;
    }
    return a.ticker.localeCompare(b.ticker)
      || a.prefix.localeCompare(b.prefix)
      || a.instrument_id.localeCompare(b.instrument_id);
  });
}

export function sectionOfTicker(ticker: string, symbols: readonly Pick<SymbolWithSection, 'ticker' | 'prefix' | 'type' | 'synthetic'>[]): SymbolSection {
  const canonical = ticker.split(':').pop()?.toUpperCase() ?? ticker.toUpperCase();
  const found = symbols.find((symbol) => symbol.ticker.toUpperCase() === canonical);
  return found ? symbolSection(found) : 'core';
}
