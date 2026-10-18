import { Vela } from '@luxalgo/vela';
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8'));
const expected = {
  '@luxalgo/vela': '0.8.4',
  '@luxalgo/vela-pinets': '0.2.16',
  pinets: '0.11.0',
};
import { VelaWorkspace } from '@luxalgo/vela/workspace';
import { PineWorkerEngine } from '@luxalgo/vela-pinets';
import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';

const checks = [
  ...Object.entries(expected).map(([name, version]) => [
    `pinned ${name}@${version}`,
    manifest.dependencies?.[name] === version,
  ]),
  ['Vela constructor', typeof Vela === 'function'],
  ['in-place setMarket for bounded history and tab resume', typeof Vela.prototype.setMarket === 'function'],
  ['visible-range restoration for pan history', typeof Vela.prototype.getVisibleRange === 'function'],
  ['VelaWorkspace constructor', typeof VelaWorkspace === 'function'],
  ['workspace getState', typeof VelaWorkspace.prototype.getState === 'function'],
  ['workspace applyState', typeof VelaWorkspace.prototype.applyState === 'function'],
  ['workspace destroy', typeof VelaWorkspace.prototype.destroy === 'function'],
  ['PineWorkerEngine constructor', typeof PineWorkerEngine === 'function'],
  ['Vela icon action API', typeof registerIcon === 'function' && typeof registerWidgetAction === 'function'],
];

const failed = checks.filter(([, ok]) => !ok);
if (failed.length > 0) {
  for (const [name] of failed) {
    console.error(`VELA_PARITY_FAIL: ${name}`);
  }
  process.exit(1);
}

console.log(`VELA_RUNTIME_PARITY_PASS @luxalgo/vela@${expected['@luxalgo/vela']} + @luxalgo/vela-pinets@${expected['@luxalgo/vela-pinets']}`);
