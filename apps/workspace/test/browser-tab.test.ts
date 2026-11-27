import { describe, expect, it } from 'vitest';
import { BrowserTabQuotes } from '../src/browser-tab';
import type { QNextBar } from '../src/qnext-provider';

const bar = (time: number, close: number, revision = 0): QNextBar => ({ time, open: close, high: close, low: close, close, revision });
const market = { symbol: 'NSE:NIFTY-SYN+', timeframe: '5s' };
const live = () => 'live' as const;
describe('browser tab quotes', () => {
  it('seeds history, tracks price direction and retains precision', () => {
    const quotes = new BrowserTabQuotes();
    quotes.observe(market.symbol, market.timeframe, bar(10, 26145.5), false, live);
    expect(quotes.presentation(market).title).toBe('NIFTY-SYN+ 26,145.50 - QNeXT');
    quotes.observe(market.symbol, market.timeframe, bar(10, 26145.625, 1), true, live);
    expect(quotes.presentation(market).title).toBe('NIFTY-SYN+ 26,145.625 ▲ - QNeXT');
    quotes.observe(market.symbol, market.timeframe, bar(10, 26145.625, 2), true, live);
    expect(quotes.presentation(market).title).toContain('▲');
    quotes.observe(market.symbol, market.timeframe, bar(15, 26140), true, live);
    expect(quotes.presentation(market).title).toContain('▼');
  });
  it('ignores old and lower revision corrections and stale history', () => {
    const quotes = new BrowserTabQuotes();
    quotes.observe(market.symbol, market.timeframe, bar(20, 100, 2), true, live);
    quotes.observe(market.symbol, market.timeframe, bar(10, 90, 99), true, live);
    quotes.observe(market.symbol, market.timeframe, bar(20, 80, 1), true, live);
    quotes.observe(market.symbol, market.timeframe, bar(20, 70, 3), false, live);
    expect(quotes.presentation(market).title).toBe('NIFTY-SYN+ 100.00 - QNeXT');
  });
  it('isolates exchanges, symbols and timeframes, and distinguishes connection state', () => {
    const quotes = new BrowserTabQuotes();
    quotes.observe(market.symbol, market.timeframe, bar(20, 100), true, () => 'fallback');
    expect(quotes.presentation(market).title).toContain('REST');
    expect(quotes.presentation(market, false).title).toContain('Disconnected');
    expect(quotes.presentation({ ...market, timeframe: '1m' }).title).toBe('NIFTY-SYN+ - QNeXT');
    expect(quotes.presentation({ ...market, symbol: 'BSE:NIFTY-SYN+' }).title).toBe('BSE:NIFTY-SYN+ - QNeXT');
    expect(quotes.presentation(undefined).title).toBe('QNeXT');
  });
});
