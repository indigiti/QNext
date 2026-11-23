import { describe, expect, it } from 'vitest';
import { OKX_GOLD_GROUP_ID, orderVisibleSymbols, symbolSection } from '../src/symbol-sections';

describe('OKX gold isolation', () => {
  it('assigns OKX gold to its own group without changing NSE/BSE categorization', () => {
    expect(symbolSection({ ticker:'XAUUSD-XPERP',prefix:'OKX',type:'futures',synthetic:false })).toBe('okx_gold');
    expect(symbolSection({ ticker:'RELIANCE',prefix:'NSE',type:'equity',synthetic:false })).toBe('nse_1m');
    expect(symbolSection({ ticker:'NIFTY-SYN+',prefix:'QNEXT',type:'index',synthetic:true })).toBe('core');
    expect(OKX_GOLD_GROUP_ID).not.toBe('QNEXT-CORE');
  });
  it('sorts core then NSE 1m then gold',()=>{
    const base = {description:'',currency:'USD',timezone:'UTC',calendar_id:'OKX_CRYPTO_24_7',synthetic:false};
    const symbols = [
      {...base,ticker:'XAUUSD-XPERP',instrument_id:'OKX:XAUUSD-XPERP',prefix:'OKX',type:'futures'},
      {...base,ticker:'RELIANCE',instrument_id:'NSE:RELIANCE',prefix:'NSE',type:'equity'},
      {...base,ticker:'NIFTY',instrument_id:'NSE:NIFTY50',prefix:'NSE',type:'index'},
    ];
    expect(orderVisibleSymbols(symbols).map(x=>x.ticker)).toEqual(['NIFTY','RELIANCE','XAUUSD-XPERP']);
  });
});
