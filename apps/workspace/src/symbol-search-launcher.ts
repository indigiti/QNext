import type { CatalogEntry } from './symbol-catalog';
/** Shared lazy loader; no chart or Vela dependency. */
export function createSymbolSearch(load:()=>Promise<readonly CatalogEntry[]>,choose:(entry:CatalogEntry)=>void|Promise<void>):{open:()=>void;dispose:()=>void}{
 const open=()=>{void import('./symbol-browser').then(m=>m.openSymbolBrowser(load,choose)).catch(error=>console.error('Symbol search failed',error));};
 const listener=(event:KeyboardEvent)=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='k'){event.preventDefault();open();}};
 window.addEventListener('keydown',listener);
 return {open,dispose:()=>window.removeEventListener('keydown',listener)};
}
