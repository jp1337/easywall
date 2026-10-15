// Copies the 4x3 SVG flag of every country easywall knows (shared.CountryCodes)
// from flag-icons (MIT) to web/static/flags/<cc>.svg, with the package's
// licence beside them. The output is committed, like web/static/fonts: the
// interface makes no third-party request, and a release needs no npm.
//
//   npm run build:flags
//
// The directory is written whole: a flag for a code no longer in the list is
// removed. TestEveryCountryHasItsFlag holds the result to the list.
import { readFile, writeFile, readdir, copyFile, rm, mkdir } from 'node:fs/promises';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
const src = path.join(root, 'node_modules/flag-icons');
const out = path.join(root, 'web/static/flags');

const go = await readFile(path.join(root, 'internal/shared/countries.go'), 'utf8');
const block = go.match(/var CountryCodes = \[\]string\{([^}]*)\}/);
if (!block) throw new Error('CountryCodes not found in internal/shared/countries.go');
const codes = [...block[1].matchAll(/"([a-z]{2})"/g)].map((m) => m[1]);

await mkdir(out, { recursive: true });
for (const f of await readdir(out)) await rm(path.join(out, f));
let bytes = 0;
for (const cc of codes) {
  // flag-icons ships them through svgo already; what is left to drop is the
  // indentation between tags and the root id, which nothing here references.
  // No flag holds <text>, so no whitespace between tags is content.
  const svg = (await readFile(path.join(src, 'flags/4x3', `${cc}.svg`), 'utf8'))
    .replace(/>\s+</g, '><').replace(/ id="flag-icons-[a-z]+"/, '').trim() + '\n';
  await writeFile(path.join(out, `${cc}.svg`), svg);
  bytes += Buffer.byteLength(svg);
}
await copyFile(path.join(src, 'LICENSE'), path.join(out, 'LICENSE-flag-icons.txt'));
console.log(`${codes.length} flags, ${(bytes / 1024).toFixed(0)} KiB`);
