import { build } from 'esbuild';
import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const output = resolve(here, '../../third_party/database');
await mkdir(output, { recursive: true });
await build({ entryPoints: [resolve(here, 'index.js')], bundle: true, minify: true, format: 'iife', target: 'chrome110', outfile: resolve(output, 'database-vendor.js'), legalComments: 'eof' });
const tableRoot = resolve(dirname(require.resolve('tabulator-tables')), '..');
await copyFile(resolve(tableRoot, 'css/tabulator.min.css'), resolve(output, 'tabulator.min.css'));
const packages = ['@codemirror/autocomplete', '@codemirror/legacy-modes', '@codemirror/language', '@codemirror/search', 'codemirror', '@codemirror/state', '@codemirror/view', '@codemirror/lang-sql', '@codemirror/theme-one-dark', 'tabulator-tables'];
const licenses = [];
const visited = new Set();
async function collectLicense(name, resolver) {
  let dir = dirname(resolver.resolve(name === '@codemirror/legacy-modes' ? name + '/mode/shell' : name)), json;
  while (true) {
    try { json = JSON.parse(await readFile(resolve(dir, 'package.json'), 'utf8')); if (json.name === name) break; } catch {}
    const parent = dirname(dir); if (parent === dir) throw new Error(`Cannot locate ${name}`); dir = parent;
  }
  const id = `${name}@${json.version}`;
  if (visited.has(id)) return;
  visited.add(id);
  let text;
  for (const file of ['LICENSE', 'LICENSE.md', 'LICENSE.txt']) { try { text = await readFile(resolve(dir, file), 'utf8'); break; } catch {} }
  if (!text) throw new Error(`Missing license: ${name}`);
  licenses.push(`${id}\n${text}`);
  const local = createRequire(resolve(dir, 'package.json'));
  for (const dependency of Object.keys(json.dependencies || {}).sort()) await collectLicense(dependency, local);
}
for (const name of packages) await collectLicense(name, require);
await writeFile(resolve(output, 'LICENSES.txt'), licenses.join('\n\n--------------------\n\n'));
