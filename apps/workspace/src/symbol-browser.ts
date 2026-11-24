import { catalogFacets, filterCatalog, type CatalogEntry } from './symbol-catalog';
import './symbol-browser.css';

const STORAGE = 'qnext-symbol-browser-v1';
let preferences = { favourites: [] as string[], recent: [] as string[] };
try {
  const saved = JSON.parse(localStorage.getItem(STORAGE) ?? '{}');
  for (const key of ['favourites','recent'] as const)
    if (Array.isArray(saved[key])) preferences[key] = saved[key].filter((v: unknown) => typeof v === 'string').slice(0, key === 'recent' ? 30 : 500);
} catch { /* storage optional */ }
function persist(): void { try { localStorage.setItem(STORAGE,JSON.stringify(preferences)); } catch { /* session fallback */ } }
function node<K extends keyof HTMLElementTagNameMap>(tag:K,text?:string):HTMLElementTagNameMap[K] {
  const e=document.createElement(tag); if(text!==undefined)e.textContent=text; return e;
}
function button(label:string):HTMLButtonElement {const b=node('button',label);b.type='button';return b;}
function select(label:string,values:readonly string[]):HTMLSelectElement {
  const s=node('select');s.setAttribute('aria-label',label);
  for(const value of ['',...values]){const o=node('option',value||'All '+label.toLowerCase());o.value=value;s.append(o);}
  return s;
}
let current:HTMLDialogElement|undefined;
const PAGE=80;
/** Mounted on demand. Filtering never opens a market subscription. */
export async function openSymbolBrowser(load:()=>Promise<readonly CatalogEntry[]>,choose:(entry:CatalogEntry)=>void|Promise<void>):Promise<void>{
  if(current?.isConnected){current.querySelector('input')?.focus();return;}
  const d=node('dialog');d.className='qnext-symbol-browser';current=d;
  const heading=node('h2','Symbol Search');heading.id='qnext-symbol-browser-title';d.setAttribute('aria-labelledby',heading.id);
  const close=button('×');close.setAttribute('aria-label','Close symbol search');close.onclick=()=>d.close();
  const header=node('header');header.append(heading,close);
  const filters=node('div');filters.className='qnext-symbol-filters';
  const q=node('input');q.type='search';q.placeholder='Search symbols, company or exchange…';q.setAttribute('aria-label','Search symbols');
  const tabs=node('div');tabs.className='qnext-symbol-tabs';
  const status=node('div','Loading instruments…');status.setAttribute('role','status');
  const results=node('div');results.className='qnext-symbol-results';
  const more=button('Show more');more.hidden=true;
  d.append(header,filters,tabs,status,results,more);filters.append(q);
  const opener=document.activeElement;
  d.addEventListener('close',()=>{d.remove();if(current===d)current=undefined;if(opener instanceof HTMLElement&&opener.isConnected)opener.focus();},{once:true});
  document.body.append(d);d.showModal();q.focus();
  let entries:readonly CatalogEntry[];
  try{entries=await load();}catch{if(d.open)status.textContent='Symbol catalog unavailable. Close and retry.';return;}
  if(!d.open)return;
  const exchange=select('Exchanges',catalogFacets(entries,'exchange'));
  const product=select('Products',catalogFacets(entries,'product'));
  filters.append(exchange,product);
  let category='',limit=PAGE,selecting=false;
  const tabButtons=new Map<string,HTMLButtonElement>();
  for(const cat of ['', 'Favourites','Recent',...catalogFacets(entries,'category')]){
    const b=button(cat||'All');tabButtons.set(cat,b);tabs.append(b);
    b.onclick=()=>{category=cat;reset();};
  }
  function reset():void{limit=PAGE;render();results.scrollTop=0;}
  function render():void{
    for(const [key,b] of tabButtons)b.setAttribute('aria-pressed',String(key===category));
    const ids=category==='Favourites'?new Set(preferences.favourites):category==='Recent'?new Set(preferences.recent):undefined;
    let matches=filterCatalog(entries,{query:q.value,exchange:exchange.value,product:product.value,category:ids?'':category,ids});
    if(category==='Recent'){const order=new Map(preferences.recent.map((id,i)=>[id,i]));matches=matches.sort((a,b)=>(order.get(a.instrument_id)??0)-(order.get(b.instrument_id)??0));}
    status.textContent=matches.length?matches.length+' instruments':'No matching instruments';
    const fragment=document.createDocumentFragment(), favourites=new Set(preferences.favourites);
    for(const entry of matches.slice(0,limit)){
      const row=node('div');row.className='qnext-symbol-row';
      const favourite=favourites.has(entry.instrument_id);
      const star=button(favourite?'★':'☆');star.setAttribute('aria-label',(favourite?'Remove favourite ':'Favourite ')+entry.ticker);star.setAttribute('aria-pressed',String(favourite));
      star.onclick=()=>{preferences.favourites=favourite?preferences.favourites.filter(id=>id!==entry.instrument_id):[entry.instrument_id,...preferences.favourites].slice(0,500);persist();render();};
      const open=button('');open.className='qnext-symbol-open';
      const name=node('strong',entry.ticker),detail=node('span',entry.description);
      const meta=node('span',[entry.exchange,entry.category,entry.product==='unknown'?'':entry.product,entry.mode==='historical_1m'?'1m history':entry.mode==='stream'?'Stream configured':'Status unverified'].filter(Boolean).join(' · '));
      open.append(name,detail,meta);
      open.onclick=async()=>{if(selecting)return;selecting=true;try{await choose(entry);preferences.recent=[entry.instrument_id,...preferences.recent.filter(id=>id!==entry.instrument_id)].slice(0,30);persist();if(d.open)d.close();}catch{if(d.open)status.textContent='Unable to open '+entry.ticker+'. Try again.';}finally{selecting=false;}};
      row.append(star,open);fragment.append(row);
    }
    results.replaceChildren(fragment);more.hidden=matches.length<=limit;
  }
  q.oninput=reset;exchange.onchange=reset;product.onchange=reset;more.onclick=()=>{limit+=PAGE;render();};
  d.addEventListener('keydown',event=>{
    if(event.target!==q&&!(event.target instanceof HTMLElement&&event.target.classList.contains('qnext-symbol-open')))return;
    if(event.key!=='ArrowDown'&&event.key!=='ArrowUp'&&!(event.key==='Enter'&&event.target===q))return;
    const buttons=Array.from(results.querySelectorAll<HTMLButtonElement>('.qnext-symbol-open'));if(!buttons.length)return;
    const index=buttons.indexOf(document.activeElement as HTMLButtonElement);event.preventDefault();
    if(event.key==='Enter')buttons[0].click();else buttons[Math.max(0,Math.min(buttons.length-1,index+(event.key==='ArrowDown'?1:-1)))].focus();
  });
  render();
}
