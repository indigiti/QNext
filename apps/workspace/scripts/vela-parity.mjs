import { Vela } from '@luxalgo/vela';
import { VelaWorkspace } from '@luxalgo/vela/workspace';

const checks = [
  ['Vela constructor', typeof Vela === 'function'],
  ['VelaWorkspace constructor', typeof VelaWorkspace === 'function'],
  ['workspace getState', typeof VelaWorkspace.prototype.getState === 'function'],
  ['workspace applyState', typeof VelaWorkspace.prototype.applyState === 'function'],
  ['workspace destroy', typeof VelaWorkspace.prototype.destroy === 'function'],
];

const failed = checks.filter(([, ok]) => !ok);
if (failed.length > 0) {
  for (const [name] of failed) {
    console.error(`VELA_PARITY_FAIL: ${name}`);
  }
  process.exit(1);
}

console.log('VELA_RUNTIME_PARITY_PASS @luxalgo/vela@0.7.7');
