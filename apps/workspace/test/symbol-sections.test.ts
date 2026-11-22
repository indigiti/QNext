import { describe, expect, it } from 'vitest';
import {
  CORE_INDEX_SYMBOLS, CORE_GROUP_ID, NSE_MINUTE_GROUP_ID, orderVisibleSymbols,
  symbolSection,
} from '../src/symbol-sections';
import { readFileSync } from 'node:fs';

const lightweight = readFileSync(new URL('../src/charts/lightweight-workspace.ts', import.meta.url), 'utf8');
const admin = readFileSync(new URL('../../ops-console/src/main.ts', import.meta.url), 'utf8');
const shell = readFileSync(new URL('../../ops-console/src/admin-shell.ts', import.meta.url), 'utf8');

describe('core index and additional NSE 1m separation', () => {
  it('retains exactly the 13 explicitly requested core tickers in stable order', () => {
    expect([...CORE_INDEX_SYMBOLS]).toEqual([
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
    ]);
    expect(new Set(CORE_INDEX_SYMBOLS).size).toBe(13);
  });

  it('classifies only supplementary non-synthetic NSE equities/indices as 1m', () => {
    expect(symbolSection({ ticker: 'RELIANCE', prefix: 'NSE', type: 'equity', synthetic: false }))
      .toBe('nse_1m');
    expect(symbolSection({ ticker: 'NIFTY100', prefix: 'NSE', type: 'index', synthetic: false }))
      .toBe('nse_1m');
    expect(symbolSection({ ticker: 'NIFTY-SYN+', prefix: 'QNEXT', type: 'index', synthetic: true }))
      .toBe('core');
    expect(symbolSection({ ticker: 'BANKEX', prefix: 'BSE', type: 'index', synthetic: false }))
      .toBe('core');
    expect(symbolSection({ ticker: 'TCS', prefix: 'BSE', type: 'equity', synthetic: false }))
      .toBe('core');
    expect(symbolSection({ ticker: 'CUSTOM-SYN', prefix: 'QNEXT', type: 'index', synthetic: true }))
      .toBe('core');
    expect(symbolSection({ ticker: 'MIDCPNIFTY', prefix: 'NSE', type: 'index', synthetic: false }))
      .toBe('core');
  });

  it('sorts real catalog only; never fabricates missing core symbols', () => {
    const items = [
      { ticker: 'RELIANCE', instrument_id: 'NSE:RELIANCE', description: '', prefix: 'NSE', type: 'equity', currency: 'INR', timezone: 'Asia/Kolkata', calendar_id: 'NSE_EQ', synthetic: false },
      { ticker: 'BANKNIFTY', instrument_id: 'NSE:BANKNIFTY', description: '', prefix: 'NSE', type: 'index', currency: 'INR', timezone: 'Asia/Kolkata', calendar_id: 'NSE_EQ', synthetic: false },
      { ticker: 'NIFTY', instrument_id: 'NSE:NIFTY50', description: '', prefix: 'NSE', type: 'index', currency: 'INR', timezone: 'Asia/Kolkata', calendar_id: 'NSE_EQ', synthetic: false },
    ];
    const sorted = orderVisibleSymbols(items);
    expect(sorted.map((item) => item.ticker)).toEqual(['NIFTY', 'BANKNIFTY', 'RELIANCE']);
    expect(sorted).toHaveLength(items.length);
    expect(items[0].ticker).toBe('RELIANCE');
  });

  it('keeps lightweight picker true optgroups and 1m-only selection without inventing fake symbols', () => {
    expect(lightweight).toContain("const tradable = options.symbols.filter((item) => item.type !== 'root')");
    expect(lightweight).toContain("document.createElement('optgroup')");
    expect(lightweight).toContain('container.label = group.label');
    expect(lightweight).toContain('config.timeframe = \'1m\'');
    expect(lightweight).toContain("symbol.group === NSE_MINUTE_GROUP_ID");
    expect(lightweight).toContain('timeframeSelect.value = \'1m\'');
    expect(lightweight).toContain('group.id');
    expect(CORE_GROUP_ID).not.toBe(NSE_MINUTE_GROUP_ID);
  });

  it('keeps Ops console sections aligned with picker order and actual feed status', () => {
    const match = admin.match(/const requestedCoreSymbolOrder = \[([\s\S]*?)\] as const;/);
    expect(match).not.toBeNull();
    const labels = [...(match?.[1] ?? '').matchAll(/'([^']+)'/g)].map((entry) => entry[1]);
    expect(labels).toEqual([...CORE_INDEX_SYMBOLS]);
    expect(admin).toContain('id="symbol-universes-card"');
    expect(admin).toContain('id="qnext-core-universe-list"');
    expect(admin).toContain('id="qnext-nse-minute-universe-list"');
    expect(admin).toContain('feed?.nse_1m');
    expect(admin).toContain('renderNSEMinuteUniverse(body)');
    expect(admin).toContain('Collector disabled or unavailable');
    expect(shell).toContain("setCardView(document.querySelector<HTMLElement>('#symbol-universes-card'), 'market')");
  });
});
