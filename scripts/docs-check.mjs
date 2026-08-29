#!/usr/bin/env node
/**
 * Targeted assertions about the documentation site, in a real browser.
 *
 * scripts/ui-check.mjs drives the application in demo mode; nothing drove the
 * documentation. Everything Phase 1 of the docs-site-polish branch changed is
 * behaviour — a scroll handler, a key binding, a result filter, a clipboard
 * write — and none of it is visible to a Go test against the stylesheet or to
 * a diff.
 *
 * This is not a suite; it is the handful of things that were broken.
 *
 *   npm run build:docs && npm run check:docs
 *
 * The browser is Playwright's own Chromium — install it with
 * `npx playwright-core install chromium`. CHROME_PATH uses a different build,
 * which is how it runs against a Chromium already on the machine.
 */
import { chromium } from 'playwright-core';
import { createServer } from 'node:http';
import { readFile, stat } from 'node:fs/promises';
import { join, extname } from 'node:path';

const ROOT = new URL('../docs/_site/', import.meta.url).pathname;

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css',
  '.js': 'text/javascript',
  '.mjs': 'text/javascript',
  '.json': 'application/json',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
  '.xml': 'application/xml',
  '.wasm': 'application/wasm'
};

// Jekyll writes pretty URLs as directories holding index.html, so a static
// server that does not resolve a directory serves 404 for every page on the
// site.
function serve(root) {
  const server = createServer(async (req, res) => {
    let p = join(root, decodeURIComponent(req.url.split('?')[0]));
    try {
      if ((await stat(p)).isDirectory()) p = join(p, 'index.html');
      const body = await readFile(p);
      res.writeHead(200, { 'content-type': TYPES[extname(p)] ?? 'application/octet-stream' });
      res.end(body);
    } catch {
      res.writeHead(404).end();
    }
  });
  return new Promise(r =>
    server.listen(0, '127.0.0.1', () => r({ server, port: server.address().port })));
}

let failed = 0;
function ok(cond, what) {
  console.log((cond ? '  ok    ' : '  FAIL  ') + what);
  if (!cond) failed++;
}

// export-import is the shortest page that still gets a contents column: 475
// words and exactly three headings, which is the minimum the layout renders one
// for. That is precisely the case the IntersectionObserver could not answer —
// its band ended 70% up the viewport, and on a page this short the last heading
// never reached it.
async function checkContents(page, base) {
  console.log('on-page contents');
  await page.goto(base + '/docs/features/export-import/');
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await page.waitForTimeout(250);

  const entries = page.locator('.docs-toc-item a');
  const n = await entries.count();
  ok(n >= 3, `the contents column has ${n} entries`);
  if (!n) return;
  ok(await entries.nth(n - 1).getAttribute('aria-current') === 'true',
     'the last entry is current at the bottom of a short page');
}

// `/` is the convention every documentation site with a search uses, it is the
// same key on every keyboard, and it needs no platform sniff to print. What it
// does need is a guard: without one, `/` cannot be typed into the search field
// it just opened.
async function checkSearchKey(page, base) {
  console.log('search shortcut');
  await page.goto(base + '/docs/features/ports/');

  ok((await page.locator('#docs-search-key').textContent()).trim() === '/',
     'the badge on the trigger reads /');

  await page.keyboard.press('/');
  const dialog = page.locator('#docs-search-dialog');
  ok(await dialog.evaluate(d => d.open), '/ opens the search overlay');

  const field = page.locator('#docs-search-panel input');
  try {
    await field.waitFor({ state: 'visible', timeout: 20000 });
  } catch {
    // The field never mounted — PagefindUI failed to load, or the overlay
    // never opened. Record it as a failure and skip the assertions that
    // depend on the field existing, instead of letting the rejection kill
    // every check still queued after this one.
    ok(false, '/ reaches the search field');
    return;
  }
  await field.pressSequentially('a/b');
  ok(await dialog.evaluate(d => d.open), 'the overlay stays open while / is typed into it');
  ok(await field.inputValue() === 'a/b', '/ reaches the search field');

  await page.keyboard.press('Escape');
}

async function main() {
  const { server, port } = await serve(ROOT);
  const base = `http://127.0.0.1:${port}`;
  const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const page = await context.newPage();

  try {
    await checkContents(page, base);
    await checkSearchKey(page, base);
  } finally {
    await browser.close();
    server.close();
  }

  console.log(failed ? `\n${failed} failed` : '\nall checks passed');
  process.exit(failed ? 1 : 0);
}

main();
