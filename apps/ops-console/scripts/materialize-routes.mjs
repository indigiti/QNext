import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const routes = [
  'dashboard',
  'market-data',
  'candles-history',
  'synthetic',
  'indicators-intelligence',
  'runtime',
  'releases',
  'diagnostics',
  'settings',
  'security',
];

const dist = resolve(process.cwd(), 'dist');
const indexPath = resolve(dist, 'index.html');
const indexHTML = await readFile(indexPath, 'utf8');

for (const route of routes) {
  const routeDir = resolve(dist, route);
  await mkdir(routeDir, { recursive: true });
  await writeFile(resolve(routeDir, 'index.html'), indexHTML, 'utf8');
}

console.log(`Materialized ${routes.length} QNext Admin pretty routes.`);
