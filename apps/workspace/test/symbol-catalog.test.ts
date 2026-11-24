import { describe,it,expect } from 'vitest';
import {normalizeCatalog,catalogFacets,filterCatalog,resolveCatalogInstrument,catalogTimeframe,catalogPage,CATALOG_PAGE_SIZE,type CatalogInstrument} from '../src/symbol-catalog';
const base={description:'Example',type:'equity',currency:'INR',timezone:'Asia/Kolkata',calendar_id:'NSE_EQ',synthetic:false};
const items:CatalogInstrument[]=[
 {...base,instrument_id:'NSE:ABC',ticker:'ABC',prefix:'NSE',data_mode:'historical_1m'},
 {...base,instrument_id:'BSE:ABC',ticker:'ABC',prefix:'BSE'},
 {...base,instrument_id:'NEW:ABC',ticker:'ABC',prefix:'NEW',type:'crypto',market_category:'Crypto',product_type:'spot'},
 {...base,instrument_id:'OKX:XAU-USDT-SWAP',ticker:'XAU-USDT-SWAP',prefix:'OKX',type:'futures',aliases:['gold']},
];
describe('multi-exchange catalog',()=>{
 it('discovers new exchanges and independent product filters',()=>{
  const c=normalizeCatalog(items);
  expect(catalogFacets(c,'exchange')).toEqual(['BSE','NEW','NSE','OKX']);
  expect(filterCatalog(c,{exchange:'NEW',category:'Crypto',product:'spot'}).map(e=>e.instrument_id)).toEqual(['NEW:ABC']);
  expect(filterCatalog(c,{query:'gold perpetual'})[0].instrument_id).toBe('OKX:XAU-USDT-SWAP');
 });
 it('rejects ambiguous bare symbols and resolves canonical ids',()=>{
  expect(()=>resolveCatalogInstrument(items,'ABC')).toThrow('ambiguous');
  expect(resolveCatalogInstrument(items,'BSE:ABC').prefix).toBe('BSE');
 });
 it('keeps minute history distinct and removes duplicate IDs',()=>{
  const c=normalizeCatalog([...items,items[0]]);
  expect(c).toHaveLength(4);
  expect(catalogTimeframe(c.find(e=>e.instrument_id==='NSE:ABC')!,'15s')).toBe('1m');
 });
});

describe('large multi-exchange catalog',()=>{
 it('keeps 50,000 instruments searchable with fixed-size result pages',()=>{
  const start=performance.now();
  const entries=normalizeCatalog(Array.from({length:50000},(_,i)=>({...base,instrument_id:'EX'+(i%5)+':S'+i,ticker:'S'+i,prefix:'EX'+(i%5)})));
  expect(entries).toHaveLength(50000);
  expect(catalogFacets(entries,'exchange')).toHaveLength(5);
  const results=filterCatalog(entries,{exchange:'EX3'});
  expect(results).toHaveLength(10000);
  expect(catalogPage(results,0).items).toHaveLength(CATALOG_PAGE_SIZE);
  expect(catalogPage(results,124).items).toHaveLength(CATALOG_PAGE_SIZE);
  expect(catalogPage(results,999).page).toBe(124);
  expect(catalogPage([],100).items).toHaveLength(0);
  console.info('50k catalog normalize/filter/pagination ms:',Math.round(performance.now()-start));
 });
});
