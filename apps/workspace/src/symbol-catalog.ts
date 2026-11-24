import { orderVisibleSymbols, symbolSection, type SymbolWithSection } from './symbol-sections';

export interface CatalogInstrument extends SymbolWithSection {
  market_category?: string;
  product_type?: string;
  data_mode?: string;
  aliases?: string[];
}
export interface CatalogEntry extends CatalogInstrument {
  exchange: string;
  category: string;
  product: string;
  mode: string;
  searchKey: string;
}
const classes: Record<string,string> = { equity:'Stocks',stock:'Stocks',index:'Indices',crypto:'Crypto',forex:'Forex',commodity:'Commodities',futures:'Futures',future:'Futures',option:'Options',bond:'Bonds' };
const products: Record<string,string> = { futures:'futures',future:'futures',option:'options',equity:'spot',stock:'spot',index:'index' };
export function normalizeCatalog(items: readonly CatalogInstrument[]): CatalogEntry[] {
  const seen = new Set<string>();
  return orderVisibleSymbols(items).flatMap(item => {
    const id = item.instrument_id.trim().toUpperCase();
    if (!id || !item.ticker || seen.has(id)) return [];
    seen.add(id);
    const type = item.type.toLowerCase();
    const gold = item.prefix.toUpperCase() === 'OKX' && ['XAUUSD-XPERP','XAU-USDT-SWAP'].includes(item.ticker.toUpperCase());
    const category = item.synthetic ? 'Synthetics' : item.market_category || (gold ? 'Commodities' : classes[type] ?? 'Other');
    const product = item.product_type || (gold ? 'perpetual' : item.synthetic ? 'synthetic' : products[type] ?? 'unknown');
    const mode = item.data_mode || (symbolSection(item) === 'nse_1m' ? 'historical_1m' : 'unknown');
    const exchange = item.prefix.trim().toUpperCase();
    return [{...item,exchange,category,product,mode,searchKey:[item.instrument_id,item.ticker,item.description,exchange,category,product,...(item.aliases ?? [])].join(' ').toLowerCase()}];
  });
}
export function catalogFacets(entries: readonly CatalogEntry[], key: 'exchange'|'category'|'product'): string[] {
  return [...new Set(entries.map(entry=>entry[key]).filter(Boolean))].sort((a,b)=>a.localeCompare(b));
}
export function filterCatalog(entries: readonly CatalogEntry[], f: {query?:string;exchange?:string;category?:string;product?:string;ids?:ReadonlySet<string>}): CatalogEntry[] {
  const terms=(f.query??'').toLowerCase().trim().split(/\s+/).filter(Boolean);
  return entries.filter(e=>(!f.exchange||e.exchange===f.exchange)&&(!f.category||e.category===f.category)&&(!f.product||e.product===f.product)&&(!f.ids||f.ids.has(e.instrument_id))&&terms.every(t=>e.searchKey.includes(t)));
}
export function resolveCatalogInstrument<T extends Pick<CatalogInstrument,'instrument_id'|'ticker'|'prefix'>>(items:readonly T[],ticker:string):T {
  const requested=ticker.trim().toUpperCase();
  const direct=items.find(i=>i.instrument_id.toUpperCase()===requested);
  if(direct)return direct;
  const pos=requested.indexOf(':');
  const prefix=pos>=0?requested.slice(0,pos):'';
  const bare=pos>=0?requested.slice(pos+1):requested;
  const matches=items.filter(i=>i.ticker.toUpperCase()===bare&&(!prefix||i.prefix.toUpperCase()===prefix));
  if(matches.length!==1)throw new Error(matches.length?'QNext symbol is ambiguous: '+ticker:'QNext symbol is not registered: '+ticker);
  return matches[0];
}
export const catalogTimeframe=(entry:Pick<CatalogEntry,'mode'>,current:string):string=>entry.mode==='historical_1m'?'1m':current;
