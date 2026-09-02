#!/usr/bin/env node
/**
 * Drives the interface in a real browser and fails the build on what a
 * stylesheet diff and a Go test cannot see.
 *
 * Two kinds of check, and the split matters:
 *
 *   - Regressions. Specific bugs that shipped, each with the input that produced
 *     them. A forwarding port of "1E+04" was stored as port 1 because the editor
 *     re-parsed the field's text with parseInt instead of reading the number the
 *     field holds; the page said "Changes saved.". Nothing in the Go suite can
 *     see that, because the mistake is a disagreement with the browser about what
 *     a number is.
 *   - Health. No console error, no failed request, no horizontal overflow, on
 *     every page in both themes. Cheap, and it is how a CSP violation or a
 *     missing asset gets caught across the whole app at once.
 *
 * Expects easywall-web already running in demo mode with no password set, so the
 * run starts at the first-run wizard and sets up its own account. Usage:
 *
 *   EASYWALL_URL=https://127.0.0.1:12227 node scripts/ui-check.mjs
 *
 * The browser is Playwright's own Chromium — install it with
 * `npx playwright-core install chromium`. Set CHROME_PATH to use a different
 * build instead; that is how it runs against a Chromium already on the machine.
 *
 * A second easywall-web is started by the script itself, from `bin/easywall-web`
 * — see checkVerifyPage — because the second login step is unreachable in the
 * demo, where no secret is ever stored. It reads the password hash the wizard
 * above just wrote out of EASYWALL_CONFIG (default /etc/easywall/web.toml); point
 * that at wherever the demo's web.toml lives if it is not there.
 *
 * `--screenshots` with no page names shoots the whole documented set,
 * including the pages above cannot reach with a single signed-in session —
 * see takeFullScreenshotSet, takeWizardScreenshots and takeVerifyScreenshot,
 * which each spin up their own throwaway easywall-web the same way.
 */
import { chromium } from 'playwright-core';
import { createHmac } from 'node:crypto';
import { mkdtempSync, writeFileSync, rmSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn } from 'node:child_process';
import https from 'node:https';
import { setTimeout as sleep } from 'node:timers/promises';

const BASE = process.env.EASYWALL_URL || 'https://127.0.0.1:12227';
// "admin" matches every release screenshot before this script existed —
// screenshot mode reuses the same account the health/regression checks sign
// in with, so the shots this produces read as the same operator throughout
// docs/assets/img/screens/ rather than a visibly different one on the pages
// this script happened to touch first.
const USER = 'admin';
const PASS = 'ui-check-password-2026';

const PAGES = [
  '/dashboard', '/ports', '/ports?type=udp', '/blacklist', '/whitelist',
  '/forwarding', '/custom', '/options', '/settings', '/system', '/password',
  '/log', '/apply',
];

const failures = [];
const fail = (what, detail) => {
  failures.push(`${what}: ${detail}`);
  console.error(`  FAIL ${what}\n       ${detail}`);
};

/**
 * RFC 6238, in the script, so the browser check needs no npm dependency and no
 * agreement with the Go implementation beyond the standard both are reading.
 *
 * Deliberately a second implementation rather than a call into the server: if
 * the two ever disagree, that is precisely the bug worth failing on.
 */
function totp(secretBase32, at = Date.now()) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const clean = secretBase32.replace(/[\s=-]/g, '').toUpperCase();
  let bits = '';
  for (const ch of clean) bits += alphabet.indexOf(ch).toString(2).padStart(5, '0');
  const bytes = Buffer.from(bits.match(/.{8}/g).map(b => parseInt(b, 2)));

  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / 30)));

  const mac = createHmac('sha1', bytes).update(counter).digest();
  const off = mac[mac.length - 1] & 0x0f;
  const num = mac.readUInt32BE(off) & 0x7fffffff;
  return String(num % 1e6).padStart(6, '0');
}

/**
 * The hash comes out of the config run 1's wizard already wrote for PASS, so
 * this needs no argon2 in JavaScript and no environment variable to carry it.
 * A four-line regex read of the `password = "…"` line the wizard wrote.
 */
function readPasswordHash(configPath) {
  let text;
  try {
    text = readFileSync(configPath, 'utf8');
  } catch {
    return null;
  }
  const m = text.match(/^\s*password\s*=\s*"([^"]*)"/m);
  return m && m[1] ? m[1] : null;
}

/**
 * The floor of the acceptance window, read out of the Go source that defines
 * it rather than typed again here. models.go explains why the bound exists at
 * all — an HTML min="" is a hint to the browser and nothing more, the server
 * took any positive number until it did not — and a literal 10 copied into
 * this file would just as quietly stop matching it the next time it moves.
 */
function readAcceptanceDurationMin() {
  const text = readFileSync('internal/shared/models.go', 'utf8');
  const m = text.match(/AcceptanceDurationMin\s*=\s*(\d+)/);
  if (!m) {
    throw new Error('could not read AcceptanceDurationMin out of internal/shared/models.go');
  }
  return parseInt(m[1], 10);
}
// The shortest window the server will accept, used below so checkAcceptanceWindow
// never waits anywhere near the real default of 120s.
const WINDOW_SECONDS = readAcceptanceDurationMin();

/** Polls an HTTPS URL, ignoring certificate errors, until it answers or times out. */
async function waitForPort(url, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const up = await new Promise(resolve => {
      const req = https.get(url, { rejectUnauthorized: false, timeout: 1000 }, res => {
        res.resume();
        resolve(true);
      });
      req.on('error', () => resolve(false));
      req.on('timeout', () => { req.destroy(); resolve(false); });
    });
    if (up) return;
    if (Date.now() > deadline) throw new Error(`${url} did not come up within ${timeoutMs}ms`);
    await sleep(250);
  }
}

/**
 * Complete the first-run wizard if it is still being served.
 *
 * Presence of the form, not the URL: once an account exists the route is not
 * registered at all, so the request 404s and the URL still says /firstrun. The
 * wizard also writes the credentials into the config file, so a second run
 * against the same config legitimately finds nothing to do.
 */
async function setUpAccount(page) {
  await page.goto(`${BASE}/firstrun`, { waitUntil: 'load' });
  if (!(await page.$('input[name=password_confirm]'))) {
    console.log('  ok   an account already exists; skipping the wizard');
    return;
  }
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', PASS);
  await page.fill('input[name=password_confirm]', PASS);
  await Promise.all([
    page.waitForNavigation(),
    page.click("form[action='/firstrun'] button[type=submit]"),
  ]);
  if (page.url().includes('/firstrun')) {
    const alert = await page.textContent('[role=alert]').catch(() => '(none)');
    throw new Error(`the first-run wizard refused its own valid input: ${alert}`);
  }
}

/**
 * Sign in once. The result is reused by every context below.
 *
 * Once, and not once per context, because /login is rate limited to five
 * attempts per ten minutes per source address — which this script used to spend
 * three of on every run, one for each context it opened. Two runs inside ten
 * minutes therefore hit the limiter on the sixth attempt, and the failure named
 * the wrong thing entirely:
 *
 *   run 1: UI checks passed
 *   run 2: Error: could not sign in: still at https://127.0.0.1:12227/login
 *   server: WARN login rate limit exceeded ip=127.0.0.1
 *
 * CI never saw it — a fresh runner each time — so it was a trap set for whoever
 * ran the checks locally twice while working on something, and it reads as a
 * broken login page. The status code is now part of the message so a 429 can
 * never be mistaken for one again.
 */
async function signIn(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'load' });
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', PASS);
  const [response] = await Promise.all([
    page.waitForResponse(r => r.url().endsWith('/login') && r.request().method() === 'POST'),
    page.click("form[action='/login'] button[type=submit]"),
  ]);
  await page.waitForLoadState('load');
  if (response.status() === 429) {
    throw new Error(
      'the login endpoint answered 429: five attempts per ten minutes per address. ' +
      'This run did not get that far on its own — wait for the window to pass, or restart ' +
      'easywall-web, which holds the buckets in memory.');
  }
  if (page.url().includes('/login')) {
    throw new Error(`could not sign in (POST /login -> ${response.status()}): still at ${page.url()}`);
  }
}

/**
 * The whole enrolment, in a browser, against the demo — which shows the flow and
 * discards the final write. The TOTP maths is exercised end to end: the page's
 * own displayed key becomes a code here, and the server has to accept it.
 */
async function checkEnrolmentFlow(page) {
  await page.goto(`${BASE}/password`, { waitUntil: 'networkidle' });
  await page.fill("form[action='/password/2fa/begin'] input[name=current_password]", PASS);
  await Promise.all([
    page.waitForLoadState('load'),
    page.click("form[action='/password/2fa/begin'] button[type=submit]"),
  ]);

  const qr = await page.getAttribute('.qr-plate img', 'src');
  if (!qr || !qr.startsWith('data:image/png;base64,')) {
    fail('2fa setup', `the QR code is ${qr ? qr.slice(0, 40) : 'absent'}; the CSP allows data: URIs and nothing else`);
    return;
  }

  const key = (await page.textContent('.totp-secret')).trim();
  await page.fill("form[action='/password/2fa/confirm'] input[name=code]", totp(key));
  await Promise.all([
    page.waitForLoadState('load'),
    page.click("form[action='/password/2fa/confirm'] button[type=submit]"),
  ]);

  const codes = await page.locator('.recovery-code').count();
  if (codes !== 8) {
    fail('2fa setup', `${codes} recovery codes on the page after confirm, want 8`);
  }
  const body = await page.textContent('body');
  if (!/demo/i.test(body)) {
    fail('2fa setup', 'the demo confirmed an enrolment without saying nothing was saved');
  } else {
    console.log('  ok   enrolment runs end to end and the demo says it saved nothing');
  }
}

/**
 * The verify page cannot be reached against the demo — no secret is ever stored
 * there — and it is the page a half-locked-out human meets first. So: a second
 * instance with prepared state.
 *
 * socket_path points nowhere on purpose. The login path does not need the core,
 * and the dashboard then shows its CoreErr banner, which is itself worth seeing.
 * This scaffolding is also the only way to take two-factor-verify-{light,dark}
 * at all.
 */
async function checkVerifyPage(browser) {
  const dir = mkdtempSync(join(tmpdir(), 'easywall-ui-'));
  const secret = 'JBSWY3DPEHPK3PXP';
  // The hash comes out of the config run 1's wizard already wrote for PASS, so
  // this needs no argon2 in JavaScript and no environment variable. An
  // env-gated skip would be a check that never runs anywhere but on the machine
  // of whoever set the variable — which is the same as not having the check.
  const hash = readPasswordHash(process.env.EASYWALL_CONFIG || '/etc/easywall/web.toml');
  if (!hash) {
    fail('second step', 'could not read the password hash out of the demo config; ' +
      'set EASYWALL_CONFIG to the web.toml run 1 signed in against');
    return;
  }
  writeFileSync(join(dir, 'web.toml'), [
    `bind_addr = "127.0.0.1:12228"`,
    `socket_path = "${join(dir, 'nowhere.sock')}"`,
    `ssl_dir = "${join(dir, 'ssl')}"`,
    `data_dir = "${dir}"`,
    `language = "en"`,
    `session_key = "ui-check-session-key-32-bytes-long!!"`,
    `username = "${USER}"`,
    `password = "${hash}"`,
    `totp_secret = "${secret}"`,
    `recovery_codes = []`,
    `update_check = false`,
  ].join('\n'));

  const proc = spawn('bin/easywall-web', ['-config', join(dir, 'web.toml')], { stdio: 'inherit' });
  try {
    await waitForPort('https://127.0.0.1:12228/login');

    const ctx = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 1000 } });
    const page = await ctx.newPage();

    await page.goto('https://127.0.0.1:12228/login', { waitUntil: 'load' });
    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await Promise.all([page.waitForLoadState('load'), page.click("form[action='/login'] button[type=submit]")]);

    if (!page.url().includes('/login/verify')) {
      fail('second step', `the password step landed on ${page.url()}, want /login/verify`);
      return;
    }

    await page.fill('input[name=code]', totp(secret));
    await Promise.all([page.waitForLoadState('load'), page.click("form[action='/login/verify'] button[type=submit]")]);
    if (!page.url().includes('/dashboard')) {
      fail('second step', `a correct code landed on ${page.url()}, want /dashboard`);
    } else {
      console.log('  ok   password → verify → dashboard');
    }

    // Three wrong codes and back to /login, with nothing saying which factor
    // failed. Ending the session here means dropping the cookie, not asking for
    // /logout: that route is refused with 405 on GET by design — see
    // checkSignOutEndsTheSession — so a goto there leaves the browser signed in,
    // and the /login GET below would redirect it straight past the form to
    // /dashboard instead of presenting one to fill.
    await ctx.clearCookies();
    await page.goto('https://127.0.0.1:12228/login', { waitUntil: 'load' });
    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await Promise.all([page.waitForLoadState('load'), page.click("form[action='/login'] button[type=submit]")]);
    for (let i = 0; i < 3; i++) {
      await page.fill('input[name=code]', '000000');
      await Promise.all([page.waitForLoadState('load'), page.click("form[action='/login/verify'] button[type=submit]")]);
    }
    if (!page.url().endsWith('/login')) {
      fail('second step', `three wrong codes left the browser at ${page.url()}, want /login`);
    } else {
      console.log('  ok   three wrong codes end the attempt');
    }

    await ctx.close();
  } finally {
    proc.kill();
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * A forwarding port must be stored as the number the field holds.
 *
 * "1E+04" is what a spreadsheet writes for 10000. The number input accepts it as
 * valid, and parseInt("1E+04", 10) is 1 — so the rule that got saved was port 1,
 * privileged and not the one anyone asked for, reported as success.
 */
async function checkForwardingPortIsNotReparsed(page) {
  await page.goto(`${BASE}/forwarding`, { waitUntil: 'networkidle' });
  const before = JSON.parse((await page.inputValue('#fwd-json')) || '[]');

  await page.click('#add-fwd-btn');
  const rows = await page.$$('#fwd-tbody tr[data-idx]');
  const row = rows[rows.length - 1];
  await (await row.$('.f-src')).click();
  await page.keyboard.type('1E+04');
  await (await row.$('.f-dst')).click();
  await page.keyboard.type('9999');

  const payload = JSON.parse((await page.inputValue('#fwd-json')) || '[]');
  const added = payload.slice(before.length);
  if (added.length !== 1) {
    fail('forwarding payload', `expected 1 new row in the payload, got ${added.length}`);
    return;
  }
  if (added[0].source_port !== 10000) {
    fail('forwarding port re-parsed',
      `typed "1E+04" (ten thousand); the payload carries source_port ${added[0].source_port}. ` +
      'The editor is re-parsing the field text instead of reading valueAsNumber.');
    return;
  }

  // And it survives the round trip, because the payload being right is only half
  // of it — the server has to accept and store the same number.
  await Promise.all([
    page.waitForNavigation(),
    page.click("#fwd-form button[type=submit]"),
  ]);
  await page.goto(`${BASE}/forwarding`, { waitUntil: 'networkidle' });
  const stored = JSON.parse((await page.inputValue('#fwd-json')) || '[]');
  // Stated as a positive and a negative, so the check does not depend on how many
  // rules were already there: the rule asked for must exist, and the rule the bug
  // produced must not.
  const wanted = stored.some(r => r.source_port === 10000 && r.dest_port === 9999);
  const truncated = stored.some(r => r.source_port === 1 && r.dest_port === 9999);
  if (truncated) {
    fail('forwarding round trip',
      'a rule was stored forwarding privileged port 1 — the "1E+04" was truncated to its first digit');
  } else if (!wanted) {
    fail('forwarding round trip',
      `no stored rule forwards 10000 -> 9999; stored: ${JSON.stringify(stored)}`);
  } else {
    console.log('  ok   a forwarding port of "1E+04" is stored as 10000');
  }
}

/**
 * All four cells of a forwarding row share one top edge.
 *
 * `display: flex` on a <td> stops the element being a table cell: the browser
 * wraps it in an anonymous cell, its box leaves the row's height, and the row
 * separator under that column sits ~10px below the others. Nothing in the Go
 * suite or a stylesheet diff can see it — only a laid-out page can — and it is
 * invisible below the 720px reflow breakpoint, where every td is a block.
 */
async function checkForwardingRowEdgesLineUp(page) {
  await page.goto(`${BASE}/forwarding`, { waitUntil: 'networkidle' });
  const measure = (sel) => page.evaluate((sel) => {
    const row = document.querySelector(sel);
    if (!row) return null;
    return [...row.querySelectorAll('td')].map(td => td.getBoundingClientRect().top);
  }, sel);

  const seededTops = await measure('#fwd-tbody tr');
  if (!seededTops) {
    fail('forwarding row', 'no rows in #fwd-tbody — the demo seed changed');
    return;
  }
  const seededSpread = Math.max(...seededTops) - Math.min(...seededTops);
  if (seededSpread > 1) {
    fail('forwarding row', `the four cells start ${seededSpread.toFixed(1)}px apart; ` +
      'a display:flex <td> is not a table cell and leaves the row height');
  } else {
    console.log('  ok   forwarding row cells share one top edge');
  }

  // app.js builds the row a browser adds independently of forwarding.html —
  // it drifted once (cell-flow survived a template rename to <div class="flow">)
  // and nothing but a rendered, added row would have shown it.
  await page.click('#add-fwd-btn');
  const addedTops = await measure('#fwd-tbody tr:last-child');
  if (!addedTops) {
    fail('forwarding row', 'clicking #add-fwd-btn did not add a row to #fwd-tbody');
    return;
  }
  const addedSpread = Math.max(...addedTops) - Math.min(...addedTops);
  if (addedSpread > 1) {
    fail('forwarding row', `the added row's cells start ${addedSpread.toFixed(1)}px apart; ` +
      'app.js\'s fwdRowHTML has drifted from forwarding.html\'s markup');
  } else {
    console.log('  ok   added forwarding row cells share one top edge');
  }
}

/**
 * The catalogue appends rows into fields that already work without it, and the
 * sources field is one comma-separated input. Both were regressions waiting to
 * happen in a different way: a picker that writes into a field the form does not
 * read produces a page that looks right and saves nothing.
 */
async function checkPortsCatalogue(page) {
  await page.goto(`${BASE}/ports?type=tcp`, { waitUntil: 'networkidle' });
  await page.click('#catalogue-btn');
  await page.click('.catalogue-item[data-service="pihole"]');

  const rows = await page.$$eval('#rules-tbody tr[data-idx]', trs =>
    trs.map(tr => ({
      port: tr.querySelector('.f-port')?.value,
      sources: tr.querySelector('.f-sources')?.value,
      service: tr.dataset.service,
    })));
  const added = rows.filter(r => r.service === 'pihole');
  if (added.length !== 2) {
    fail('ports catalogue', `picking Pi-hole added ${added.length} TCP rows, expected 2 (80, 53)`);
    return;
  }
  if (!added[0].sources.includes('fc00::/7')) {
    fail('ports catalogue', `the private suggestion did not reach the field: "${added[0].sources}"`);
  }

  const payload = await page.$eval('#rules-json', el => el.value);
  if (!payload.includes('"sources"') || !payload.includes('"service":"pihole"')) {
    fail('ports catalogue', `the form payload dropped the new fields: ${payload.slice(0, 300)}`);
  }
}

/**
 * app.js's ruleRowHTML carries a comment requiring it to stay identical to the
 * server-rendered row in ports.html — same column order, same classes, same
 * data-label values, same chip. Nothing checked that beyond eyeballing it, so
 * the two are free to drift the next time either one is touched alone.
 *
 * Pick a catalogue row (browser-built, via ruleRowHTML) and read its shape;
 * save and reload (server-rendered, via ports.html) and read the same row's
 * shape; they must be the same markup, not just the same values.
 */
async function checkPortsRowAgreesWithServer(page) {
  await page.goto(`${BASE}/ports?type=tcp`, { waitUntil: 'networkidle' });
  await page.click('#catalogue-btn');
  await page.click('.catalogue-item[data-service="pihole"]');

  const shape = () => page.$eval('#rules-tbody tr[data-service="pihole"]', tr => ({
    trClasses: tr.className,
    cells: [...tr.querySelectorAll('td')].map(td => ({
      classes: td.className,
      label: td.getAttribute('data-label'),
      chip: td.querySelector('.chip')?.textContent.trim() ?? null,
    })),
  }));

  let clientShape;
  try {
    clientShape = await shape();
  } catch {
    fail('ports row agreement', 'picking Pi-hole did not add a tr[data-service="pihole"] row to compare');
    return;
  }

  await Promise.all([
    page.waitForNavigation(),
    page.click("#ports-form button[type=submit]"),
  ]);
  await page.goto(`${BASE}/ports?type=tcp`, { waitUntil: 'networkidle' });

  let serverShape;
  try {
    serverShape = await shape();
  } catch {
    fail('ports row agreement', 'the saved catalogue row was not there after reload — could not compare');
    return;
  }

  const before = JSON.stringify(clientShape);
  const after = JSON.stringify(serverShape);
  if (before !== after) {
    fail('ports row agreement',
      `the browser-built row (ruleRowHTML) and the server-rendered row (ports.html) disagree.\n` +
      `  client: ${before}\n  server: ${after}`);
  } else {
    console.log('  ok   the catalogue-built row matches the server-rendered row after save and reload');
  }
}

/**
 * The apply screen actually draws the preview, and the verdict names an address.
 *
 * The demo seeds a configuration drift, so /apply always has something to show.
 * A Go test can assert the handler built the data; only a browser can say the
 * page rendered it, that the mono column lines up, and that nothing scrolls
 * sideways at 390px with a long custom rule in the diff.
 */
async function checkApplyPreview(page) {
  await page.goto(`${BASE}/apply`, { waitUntil: 'networkidle' });

  if (await page.locator('.diff-row').count() === 0) {
    fail('apply preview', 'no .diff-row on /apply — the demo seeds a drift, so the page should list it');
    return;
  }
  const verdict = page.locator('.verdict-addr');
  if (await verdict.count() === 0) {
    fail('apply preview', 'no verdict line — the page does not say where the request came from');
  } else if (!/->|→/.test(await verdict.first().innerText())) {
    fail('apply preview', `the verdict line does not name an address and a port: ${await verdict.first().innerText()}`);
  } else {
    console.log('  ok   /apply shows the diff and names the connection it is about');
  }
}

/**
 * Adds one port rule via #add-rule-btn — the same control
 * checkForwardingPortIsNotReparsed already drives for the forwarding table,
 * since ports.html's row editor works the same way: click to append a row,
 * then fill its fields.
 *
 * Skips the click if the port is already staged. This function saves what it
 * adds, and the demo server this runs against keeps its state across runs
 * (see democlient.go's own comment on that) — so a rerun must not pile a
 * second, identical row onto whatever the previous run already left behind.
 */
async function addPortRule(page, port, description = '') {
  const already = await page.$$eval('#rules-tbody tr[data-idx] .f-port',
    (els, port) => els.some(el => el.value === port), port);
  if (already) return;

  await page.click('#add-rule-btn');
  const rows = await page.$$('#rules-tbody tr[data-idx]');
  const row = rows[rows.length - 1];
  await (await row.$('.f-port')).fill(port);
  if (description) await (await row.$('.f-desc')).fill(description);
}

/**
 * Drive a whole acceptance window: apply, read the countdown falling, roll
 * back, and assert the rules came back.
 *
 * The demo runs a real timer, so this is a real window. A fixture here would
 * prove nothing about the one screen that carries the product's thesis.
 */
async function checkAcceptanceWindow(page) {
  let ok = true;
  const failHere = (detail) => { fail('acceptance window', detail); ok = false; };

  // A short window, set through the interface, so the run does not wait two
  // minutes. WINDOW_SECONDS is the floor read out of models.go above this
  // function, not a number typed here that a future change to the floor could
  // silently drift under.
  await page.goto(`${BASE}/system`, { waitUntil: 'networkidle' });
  await page.fill('input[name=acceptance_duration]', String(WINDOW_SECONDS));
  // This form posts through htmx (hx-swap="none"), so the click never
  // navigates anywhere — waitForNavigation here would just hang until its own
  // timeout. Waiting for the response the click itself starts is the same fix
  // signIn uses for /login, for the same reason: a load-state wait can resolve
  // against a page that was already there before the request finished.
  const [saved] = await Promise.all([
    page.waitForResponse(r => r.url().endsWith('/system') && r.request().method() === 'POST'),
    page.click("form[action='/system'] button[type=submit]"),
  ]);
  if (!saved.ok()) {
    fail('acceptance window', `saving a ${WINDOW_SECONDS}s window on /system answered ${saved.status()}`);
    return;
  }

  // Stage something nameable, then apply. Everything up to the click on
  // #start-btn happens before the window opens, so none of it counts against
  // how long the window actually runs.
  //
  // 9443, not 8443: the demo seeds 8443 into both Current and Staged from the
  // start (see democlient.go's seed()), so staging it again is not a change —
  // DiffRules would show nothing and the assertion below would pass for the
  // wrong reason. 9443 is not in the seed on either side.
  await page.goto(`${BASE}/ports`, { waitUntil: 'networkidle' });
  await addPortRule(page, '9443');
  await Promise.all([
    page.waitForNavigation(),
    page.click("#ports-form button[type=submit]"),
  ]);

  await page.goto(`${BASE}/apply`, { waitUntil: 'networkidle' });
  await Promise.all([page.waitForNavigation(), page.click('#start-btn')]);

  const countdown = page.locator('#apply-countdown');
  if (!(await countdown.count())) {
    fail('acceptance window', 'no countdown on /apply with a window open');
    return;
  }

  const first = await countdown.innerText();
  if (!/^\d{2}:\d{2}$/.test(first)) {
    failHere(`the countdown reads ${JSON.stringify(first)}, want mm:ss`);
  }

  // It has to actually fall. A rendered "00:10" that never changes is the
  // defect this release exists to remove, with better typography. This is
  // the one wait in this file that measures the passage of time on purpose,
  // rather than standing in for a condition — and the window is only
  // WINDOW_SECONDS long, so everything from here on is written to spend as
  // little of it as possible.
  await page.waitForTimeout(2500);
  const second = await countdown.innerText();
  if (second === first) {
    failHere(`the countdown did not move in 2.5s: ${first} -> ${second}`);
  }

  // The live diff names what is live — no reload needed, this is still the
  // page #start-btn's own navigation rendered.
  if (!(await page.locator('.diff-row', { hasText: '9443' }).count())) {
    failHere('the open window does not name the port it made live');
  }

  // The chip follows the operator off this page. `load` rather than the usual
  // `networkidle`: every second spent waiting here is one the rollback below
  // has less of the window left to beat.
  await page.goto(`${BASE}/ports`, { waitUntil: 'load' });
  if (!(await page.locator('#apply-chip').count())) {
    failHere('the countdown does not follow the operator to /ports');
  }

  // Roll back, and check the window actually closed.
  await page.goto(`${BASE}/apply`, { waitUntil: 'load' });
  await Promise.all([page.waitForNavigation(), page.click('#rollback-btn')]);

  // A rollback restores what is live, not what is staged: handlePortsGET
  // always reads Staged, and the rollback only touches Current/Backup. So the
  // port just added is expected to still be on /ports — asserting it is gone
  // would fail the check on the one thing this release deliberately kept.
  // What must be true instead is that the apply page offers a start again,
  // not a confirm.
  if (await page.locator('#confirm-btn').count()) {
    failHere('the window is still open after a rollback');
  }
  if (!(await page.locator('#start-btn').count())) {
    failHere('the apply page offers no way forward after a rollback');
  }

  if (ok) {
    console.log('  ok   a whole acceptance window: it runs, it is visible everywhere, it rolls back');
  }
}

/**
 * Signing out has to end the session by pressing the control the operator sees.
 *
 * The Go suite proves the *route* is right: GET /logout answers 405, a
 * cross-origin POST answers 403, a same-origin POST answers 303. What it cannot
 * see is which of those the sidebar actually sends. Sign out was an
 * `<a href="/logout">` until the route moved to POST, and a template that goes
 * back to an anchor — or a form that loses its method — leaves every Go test
 * green while the button quietly answers 405 and the operator stays signed in.
 *
 * So this drives the control rather than the URL: click it, then ask for a page
 * behind the login and see where it lands.
 */
/**
 * The version badge fits the version strings a build of this repository can
 * carry.
 *
 * `.brand-version` was capped at 9ch, which fits "2.13.0" and nothing longer.
 * The .deb sets the version from the changelog and gets six characters; the
 * Dockerfile and the Makefile pass `git describe`, so every container image
 * showed "v2.13…" instead of its own version. "2.13.10" clips just the same,
 * which would have reached every installation at the first patch release past
 * .9 — the one shape of this bug nobody would have read as a packaging quirk.
 *
 * Measured in the browser rather than asserted against the stylesheet: what
 * matters is whether the text fits the box, and no number in a CSS file says
 * that. The `title` attribute carries the full string either way, but a tooltip
 * is not a version number on the screen.
 */
async function checkVersionBadgeFitsTheVersion(page) {
  await page.goto(`${BASE}/dashboard`, { waitUntil: 'load' });
  const bad = await page.evaluate(() => {
    const el = document.querySelector('.brand-version');
    if (!el) return ['(the sidebar has no .brand-version badge at all)'];
    const brand = el.parentElement;
    const original = el.textContent;
    const clipped = [];
    // The changelog version, `git describe` with and without a tag suffix, and
    // a two-digit patch — every shape the packaging here can hand the binary.
    for (const v of ['2.13.0', 'v2.13.0', '2.13.10', 'v2.13.10', 'v2.13.0-rc1', 'v2.13.0-dirty']) {
      el.textContent = v;
      if (el.scrollWidth > el.clientWidth) clipped.push(v);
      // A badge that fits by pushing the product name out of the row is not a
      // fix; the cap has to leave the brand line intact as well.
      else if (brand.scrollWidth > brand.clientWidth) clipped.push(`${v} (overflows the brand row)`);
    }
    el.textContent = original;
    return clipped;
  });
  if (bad.length) {
    throw new Error(`the version badge cannot show: ${bad.join(', ')}`);
  }
  console.log('  ok   the version badge fits every version string a build can carry');
}

async function checkSignOutEndsTheSession(page) {
  await page.goto(`${BASE}/dashboard`, { waitUntil: 'networkidle' });

  const anchors = await page.locator('.nav-footer a').count();
  if (anchors > 0) {
    fail('sign out', `.nav-footer holds ${anchors} link(s); the sign-out control must be a form submit, ` +
      'because GET /logout is refused with 405 and an anchor would sign nobody out');
  }

  const button = page.locator('.nav-footer .logout-btn');
  if (await button.count() === 0) {
    fail('sign out', 'no .logout-btn in the sidebar footer — nothing to sign out with');
    return;
  }
  await button.click();
  await page.waitForLoadState('networkidle');

  await page.goto(`${BASE}/dashboard`, { waitUntil: 'networkidle' });
  if (!page.url().includes('/login')) {
    fail('sign out', `after pressing sign out, /dashboard still answered as ${page.url()} — the session survived it`);
  } else {
    console.log('  ok   sign out ends the session');
  }
}

/**
 * The switcher has to work two ways at once: a real <select> that a running
 * script submits on change, and a submit button that stays in the markup and
 * visible for an operator with no JavaScript — the one screen where this
 * control matters most is the one nothing has run on yet. Neither Go nor a
 * stylesheet diff can see whether the button actually disappears once script
 * has run, or actually works once it hasn't; only a browser can.
 */
/**
 * Picks a language from the select and waits for the POST it is supposed to
 * trigger. Returns whether that POST actually happened.
 *
 * The wait is armed *before* the option is picked, and it waits for the request
 * rather than for a load state. `selectOption` then `waitForLoadState` looks
 * equivalent and is not: the page is already loaded when the second call runs,
 * so if the change handler has not yet started its navigation, the wait
 * resolves against the load that already happened and the cookie is read
 * before the POST has been made. That passes on a fast machine and fails on a
 * CI runner, which is exactly what it did — a green check locally and one red
 * job with "cookie is unset".
 */
async function switchTo(page, code) {
  const posted = page
    .waitForRequest(r => r.url().endsWith('/language') && r.method() === 'POST', { timeout: 5000 })
    .then(() => true, () => false);
  await page.locator('#lang-select').selectOption({ value: code });
  const ok = await posted;
  if (ok) await page.waitForLoadState('load');
  return ok;
}

async function checkLanguageSwitch(ctx) {
  const page = await ctx.newPage();
  await page.goto(`${BASE}/dashboard`, { waitUntil: 'networkidle' });

  const select = page.locator('#lang-select');
  if (await select.count() === 0) {
    fail('language switch', 'no #lang-select in the sidebar — the chip buttons were not replaced');
  }
  if (await page.locator('.lang-option').count() > 0) {
    fail('language switch', '.lang-option chip markup is still rendered alongside the select');
  }
  const submit = page.locator('.lang-submit');
  if (await submit.count() === 0) {
    fail('language switch', 'no .lang-submit in the markup — required for the no-JS path');
  } else if (await submit.isVisible()) {
    fail('language switch', '.lang-submit is visible with JavaScript running; data-js should have hidden it');
  }

  if (await switchTo(page, 'de')) {
    const lang = (await ctx.cookies()).find(c => c.name === 'easywall_lang');
    if (lang?.value !== 'de') {
      fail('language switch', `the POST went through but the cookie is ${lang?.value ?? 'unset'}`);
    } else {
      console.log('  ok   the select submits itself on change with JavaScript running');
    }
  } else {
    fail('language switch', 'selecting "de" never posted /language — the change handler ' +
      'did not fire, or the value was already "de" so there was no change to react to');
  }
  // Leave English behind for whatever runs after this.
  await switchTo(page, 'en');
  const session = await ctx.storageState();
  await page.close();

  // Without JavaScript the select cannot submit itself, so the button in the
  // markup is the only way to change the language — and it must be visible.
  const noJsCtx = await ctx.browser().newContext({
    ignoreHTTPSErrors: true,
    javaScriptEnabled: false,
    storageState: session,
  });
  const noJsPage = await noJsCtx.newPage();
  await noJsPage.goto(`${BASE}/dashboard`);
  const noJsSubmit = noJsPage.locator('.lang-submit');
  if (await noJsSubmit.count() === 0 || !(await noJsSubmit.isVisible())) {
    fail('language switch (no JS)', '.lang-submit is not visible without JavaScript — ' +
      'an operator who cannot read the interface would have no way to change it');
  } else {
    await noJsPage.locator('#lang-select').selectOption({ value: 'de' });
    // click() auto-waits for the navigation a submit button starts, so this one
    // was never racy the way the change handler above was.
    await noJsSubmit.click();
    await noJsPage.waitForLoadState('load');
    const lang = (await noJsCtx.cookies()).find(c => c.name === 'easywall_lang');
    if (lang?.value !== 'de') {
      fail('language switch (no JS)', 'the submit button did not change the language cookie');
    } else {
      console.log('  ok   the submit button works with JavaScript disabled');
    }
  }
  await noJsCtx.close();
}

/**
 * Every input in a rules table (.table-reflow — ports, forwarding, log) must
 * show its own value. The page-wide overflow check above cannot see this: a
 * column that squeezes its input to below the value's content width doesn't
 * make the *page* scroll, table-layout: auto just clips the field in place.
 * That is exactly how task 6's Sources column shipped — 190px showing a
 * 307px default, fc00::/7 entirely invisible, and the page overflow check
 * green throughout because nothing scrolled.
 */
async function checkNoInputClipsItsOwnValue(page, where, path) {
  // The demo's own seed carries no Sources value long enough to prove
  // anything either way. The catalogue's Pi-hole entry does — same fc00::/7
  // default the demo config's two saved rows carry — so add it here,
  // unsaved, purely to put that value on the page for this one check. The
  // next page.goto in the caller's loop discards it; nothing is written to
  // the account's rules, so checkPortsCatalogue's row count downstream is
  // unaffected.
  if (path.startsWith('/ports')) {
    const btn = page.locator('#catalogue-btn');
    if (await btn.count() && await btn.isVisible()) {
      await btn.click();
      const item = page.locator('.catalogue-item[data-service="pihole"]');
      if (await item.count()) await item.click();
    }
  }

  const clipped = await page.evaluate(() => {
    const inputs = document.querySelectorAll('.table-reflow input');
    return [...inputs]
      .filter(el => el.scrollWidth > el.clientWidth)
      .map(el => ({
        label: el.getAttribute('aria-label') || el.name || el.className,
        value: el.value,
        clientWidth: el.clientWidth,
        scrollWidth: el.scrollWidth,
      }));
  });
  for (const c of clipped) {
    fail(`input overflow [${where}]`, `${path} — "${c.label}" value "${c.value}" needs ` +
      `${c.scrollWidth}px, only ${c.clientWidth}px visible`);
  }
}

/** Every page renders without complaint, and without scrolling sideways. */
async function checkPageHealth(ctx, theme, width) {
  const page = await ctx.newPage();
  const seen = [];
  page.on('console', m => { if (m.type() === 'error') seen.push(`console: ${m.text()}`); });
  page.on('pageerror', e => seen.push(`pageerror: ${e.message}`));
  page.on('requestfailed', r => seen.push(`requestfailed: ${r.url()}`));

  const where = `${theme} ${width}px`;
  for (const path of PAGES) {
    seen.length = 0;
    await page.goto(BASE + path, { waitUntil: 'networkidle' });
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    if (overflow > 0) {
      fail(`horizontal overflow [${where}]`, `${path} scrolls ${overflow}px sideways`);
    }
    await checkNoInputClipsItsOwnValue(page, where, path);
    for (const problem of seen) {
      fail(`page problem [${where}]`, `${path} — ${problem}`);
    }
  }
  console.log(`  ok   ${PAGES.length} pages clean at ${where}`);
  await page.close();
}

const launch = process.env.CHROME_PATH
  ? { executablePath: process.env.CHROME_PATH }
  : {};

// The three widths CLAUDE.md names: a desktop, a narrow window, a phone. Only
// 1600 was ever driven here, so "both themes, three widths" was a rule the
// repository stated and checked a third of — and a mobile layout that scrolls
// sideways is invisible at 1600.
//
// 1440 and 1200 added after the Sources column defect shipped invisible to
// this suite: both sit in .page-grid's two-column band (aside present), where
// a shared table can be squeezed well below what 1600 or 900 exercise. 1440
// is an ordinary maximised laptop window and 1200 sat at the worst point of
// the band that clipped — 10 of 30 fields — while every width already listed
// here stayed clean throughout.
const WIDTHS = [1600, 1440, 1200, 900, 390];

/** The full health/regression suite — everything that isn't screenshotting. */
async function runChecks(browser, session) {
  const langCtx = await browser.newContext({ ignoreHTTPSErrors: true, storageState: session });
  await checkLanguageSwitch(langCtx);
  await langCtx.close();

  for (const theme of ['dark', 'light']) {
    for (const width of WIDTHS) {
      const ctx = await browser.newContext({
        ignoreHTTPSErrors: true,
        viewport: { width, height: 1000 },
        storageState: session,
      });
      // After storageState, so the theme this pass is checking wins over
      // whatever the signed-in context happened to leave behind.
      await ctx.addInitScript(t => localStorage.setItem('theme', `easywall-${t}`), theme);
      await checkPageHealth(ctx, theme, width);
      await ctx.close();
    }
  }

  const ctx = await browser.newContext({
    ignoreHTTPSErrors: true,
    viewport: { width: 1600, height: 1000 },
    storageState: session,
  });
  const p = await ctx.newPage();
  await checkForwardingRowEdgesLineUp(p);
  await checkForwardingPortIsNotReparsed(p);
  await checkPortsCatalogue(p);
  await checkPortsRowAgreesWithServer(p);
  await checkApplyPreview(p);
  await checkAcceptanceWindow(p);
  await checkEnrolmentFlow(p);
  await checkVersionBadgeFitsTheVersion(p);
  await checkVerifyPage(browser);
  // Last, and deliberately: signing out revokes the session id every context
  // above is sharing, so anything after it would be driving a signed-out browser.
  await checkSignOutEndsTheSession(p);
  await ctx.close();
}

/**
 * --screenshots [page ...]: capture docs/assets/img/screens/<page>-<theme>.png
 * for each page, in both themes, instead of running the suite above. Pages are
 * path segments ("ports", or "/ports") off a short default list when none are
 * given. Reuses setUpAccount/signIn below — there is no second sign-in path
 * to maintain.
 */
const screenshotFlagIdx = process.argv.indexOf('--screenshots');
const screenshotMode = screenshotFlagIdx !== -1;
const screenshotArgs = screenshotMode
  ? process.argv.slice(screenshotFlagIdx + 1).map(a => a.startsWith('/') ? a : `/${a}`)
  : [];
// The pages docs/assets/img/screens/ actually ships figures for — grep
// `base="/assets/img/screens/` across docs/_docs to regenerate this list.
// Deliberately narrower than PAGES: /whitelist and /system have no figure of
// their own (filters.md reuses the blacklist/options shots), and shooting
// them here would add files nothing links to.
const DEFAULT_SCREENSHOT_PAGES = [
  '/dashboard', '/ports', '/blacklist', '/forwarding', '/custom',
  '/options', '/settings', '/password', '/log', '/apply',
];
// The remaining names in that directory are not URL paths at all — the
// first-run wizard (with its optional 2FA step), the login page a signed-out
// visitor meets, and the two-factor setup/verify screens. Each needs its own
// flow, several need an account that does not exist yet, and login/verify
// need to be reached signed OUT — so they are captured by the functions
// below rather than by visiting a path with an authenticated session.

/**
 * Fills in the one thing the seeded demo TCP tab does not otherwise show: a
 * rule with sources filled in, and a catalogue-added rule with its service
 * chip. Saved server-side, so it survives into every theme's own context.
 */
async function seedPortsScreenshot(page) {
  await page.goto(`${BASE}/ports?type=tcp`, { waitUntil: 'networkidle' });
  await page.locator('#rules-tbody tr[data-idx] .f-sources').first().fill('203.0.113.10, 203.0.113.11');
  if (await page.locator('#rules-tbody tr[data-service="pihole"]').count() === 0) {
    await page.click('#catalogue-btn');
    await page.click('.catalogue-item[data-service="pihole"]');
  }
  await Promise.all([
    page.waitForNavigation(),
    page.click("#ports-form button[type=submit]"),
  ]);
}

// The shape every published screenshot is taken in.
//
// 1600 rather than the 1440 this used from 2.11 to 2.13: `.page-grid` drops its
// 320px context column below 1570px, so at 1440 every screenshot in docs/ showed
// the collapsed single-column fallback — the aside cards stacked under the table
// instead of beside it, on ports, blacklist, forwarding, custom and options
// alike. 1440 is still exercised, by WIDTHS above, where squeezing the layout is
// the whole point. A screenshot is documentation, and documents the layout the
// design is actually about.
const SHOT_VIEWPORT = { width: 1600, height: 900 };

/**
 * Screenshot one page into docs/assets/img/screens/<name>-<theme>.png.
 *
 * The viewport is grown to the document's height first rather than passing
 * `fullPage`. `.sidebar` is `position: fixed` with `min-height: 100vh`, and a
 * fullPage capture leaves a fixed element pinned to the viewport it was laid out
 * in: on every page taller than 900px the sidebar stopped mid-image, with the
 * language switch, the theme toggle and Logout floating in the middle of it and
 * nothing below. Twenty-two of the thirty-four shipped files carried it — every
 * page with a sidebar that ran past 900px. Growing
 * the window instead renders the page the way a reader with a window that tall
 * would see it — which is also the right answer for the sticky save bar.
 */
async function shoot(page, name, theme) {
  const out = `docs/assets/img/screens/${name}-${theme}.png`;
  const height = await page.evaluate(() => document.documentElement.scrollHeight);
  if (height > SHOT_VIEWPORT.height) {
    await page.setViewportSize({ width: SHOT_VIEWPORT.width, height });
    // The reflow is synchronous, but a sticky element's resolved position and
    // any transition on it are not.
    await page.waitForTimeout(200);
  }
  await page.screenshot({ path: out });
  await page.setViewportSize(SHOT_VIEWPORT);
  console.log(`  wrote ${out}`);
}

/** A themed, 1600x900@1.5x context — every screenshot in the set uses this shape. */
async function screenshotContext(browser, theme, extra = {}) {
  const ctx = await browser.newContext({
    ignoreHTTPSErrors: true, viewport: { ...SHOT_VIEWPORT },
    deviceScaleFactor: 1.5, ...extra,
  });
  await ctx.addInitScript(t => localStorage.setItem('theme', `easywall-${t}`), theme);
  return ctx;
}

async function takeScreenshots(browser, session, pages) {
  console.log(`Screenshotting ${pages.join(', ')} in both themes`);
  const prep = await browser.newContext({
    ignoreHTTPSErrors: true, viewport: { ...SHOT_VIEWPORT },
    deviceScaleFactor: 1.5, storageState: session,
  });
  if (pages.includes('/ports')) await seedPortsScreenshot(await prep.newPage());
  await prep.close();

  for (const theme of ['light', 'dark']) {
    const ctx = await screenshotContext(browser, theme, { storageState: session });
    const page = await ctx.newPage();
    for (const path of pages) {
      await page.goto(BASE + path, { waitUntil: 'networkidle' });
      const name = path.replace(/^\//, '').replace(/\?.*$/, '');
      await shoot(page, name, theme);
    }
    await ctx.close();
  }
}

/**
 * The signed-out login page, in one theme. No storageState, so this is always
 * the form itself — never a redirect to /dashboard — and a GET costs nothing
 * against the login rate limiter, which only counts the POST.
 */
async function takeLoginScreenshot(browser, theme) {
  const ctx = await screenshotContext(browser, theme);
  const page = await ctx.newPage();
  await page.goto(`${BASE}/login`, { waitUntil: 'load' });
  await shoot(page, 'login', theme);
  await ctx.close();
}

/**
 * Enabling 2FA from /password. Demo mode runs the whole flow and then
 * discards the write (see handler_2fa.go's IsDemo branch) — correct there,
 * but the page it renders says "not saved" and greys itself out, unlike
 * every real enrolment. So this gets its own throwaway, non-demo instance,
 * the same shape as takeVerifyScreenshot: the wizard creates a plain
 * account (no 2FA yet), then /password's enrolment happens for real and
 * actually lands on "enabled".
 */
async function takeEnrolmentScreenshots(browser, theme) {
  const dir = mkdtempSync(join(tmpdir(), 'easywall-ui-2fa-'));
  const port = 12232;
  writeFileSync(join(dir, 'web.toml'), [
    `bind_addr = "127.0.0.1:${port}"`,
    `socket_path = "${join(dir, 'nowhere.sock')}"`,
    `ssl_dir = "${join(dir, 'ssl')}"`,
    `data_dir = "${dir}"`,
    `language = "en"`,
    `session_key = "ui-check-2fa-session-key-32bytes"`,
    `update_check = false`,
  ].join('\n'));

  const proc = spawn('bin/easywall-web', ['-config', join(dir, 'web.toml')], { stdio: 'inherit' });
  try {
    const base = `https://127.0.0.1:${port}`;
    await waitForPort(`${base}/firstrun`);

    const ctx = await screenshotContext(browser, theme);
    const page = await ctx.newPage();

    await page.goto(`${base}/firstrun`, { waitUntil: 'load' });
    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await page.fill('input[name=password_confirm]', PASS);
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/firstrun'] button[type=submit]"),
    ]);

    await page.goto(`${base}/login`, { waitUntil: 'load' });
    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/login'] button[type=submit]"),
    ]);

    await page.goto(`${base}/password`, { waitUntil: 'networkidle' });
    await page.fill("form[action='/password/2fa/begin'] input[name=current_password]", PASS);
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/password/2fa/begin'] button[type=submit]"),
    ]);
    if (!(await page.$('.totp-secret'))) {
      throw new Error('2FA setup did not show the QR/secret step');
    }
    await shoot(page, 'two-factor-setup', theme);

    const key = (await page.textContent('.totp-secret')).trim();
    await page.fill("form[action='/password/2fa/confirm'] input[name=code]", totp(key));
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/password/2fa/confirm'] button[type=submit]"),
    ]);
    if (!(await page.$('.recovery-code'))) {
      throw new Error('2FA confirm did not show recovery codes');
    }
    await shoot(page, 'two-factor-codes', theme);

    await ctx.close();
  } finally {
    proc.kill();
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * The first-run wizard with its 2FA step, and the account it creates is never
 * used again. Nothing else in this file can reach these three screens: the
 * long-lived instance the rest of the set is shot against already has an
 * account by the time this runs, and /firstrun redirects to /login the
 * moment one exists. So each theme gets its own throwaway instance, spun up
 * and torn down entirely inside this function.
 */
async function takeWizardScreenshots(browser, theme) {
  const dir = mkdtempSync(join(tmpdir(), 'easywall-ui-wizard-'));
  const port = 12229;
  writeFileSync(join(dir, 'web.toml'), [
    `bind_addr = "127.0.0.1:${port}"`,
    `socket_path = "${join(dir, 'nowhere.sock')}"`,
    `ssl_dir = "${join(dir, 'ssl')}"`,
    `data_dir = "${dir}"`,
    `language = "en"`,
    `demo_mode = true`,
    `session_key = "ui-check-wizard-session-key-32byte"`,
    `update_check = false`,
  ].join('\n'));

  const proc = spawn('bin/easywall-web', ['-config', join(dir, 'web.toml')], { stdio: 'inherit' });
  try {
    const base = `https://127.0.0.1:${port}`;
    await waitForPort(`${base}/firstrun`);

    const ctx = await screenshotContext(browser, theme);
    const page = await ctx.newPage();

    await page.goto(`${base}/firstrun`, { waitUntil: 'load' });
    await shoot(page, 'firstrun', theme);

    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await page.fill('input[name=password_confirm]', PASS);
    await page.check('input[name=want_totp]');
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/firstrun'] button[type=submit]"),
    ]);
    if (!(await page.$('.totp-secret'))) {
      throw new Error('firstrun did not reach the TOTP setup step with want_totp checked');
    }
    await shoot(page, 'firstrun-2fa', theme);

    const key = (await page.textContent('.totp-secret')).trim();
    await page.fill("form[action='/firstrun/confirm'] input[name=code]", totp(key));
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/firstrun/confirm'] button[type=submit]"),
    ]);
    if (!(await page.$('.recovery-code'))) {
      throw new Error('firstrun/confirm did not show recovery codes');
    }
    await shoot(page, 'firstrun-codes', theme);

    await ctx.close();
  } finally {
    proc.kill();
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * The second step of a 2FA login. Reachable only against an account that
 * already has a secret — the demo never stores one (see checkVerifyPage,
 * which this mirrors) — so a fresh instance is seeded with the *shot*
 * account's own password hash, read out of EASYWALL_CONFIG exactly as
 * checkVerifyPage does, plus a secret this function controls.
 *
 * Returns false, naming the page, if it cannot get the hash — rather than
 * leaving two-factor-verify-*.png stale and silently wrong about why.
 */
async function takeVerifyScreenshot(browser, theme) {
  const configPath = process.env.EASYWALL_CONFIG || '/etc/easywall/web.toml';
  const hash = readPasswordHash(configPath);
  if (!hash) {
    console.log(`  skip two-factor-verify-${theme}.png: could not read the password hash ` +
      `out of ${configPath} — set EASYWALL_CONFIG to the file the shot account signed in against`);
    return false;
  }

  const dir = mkdtempSync(join(tmpdir(), 'easywall-ui-verify-'));
  const port = 12230;
  const secret = 'JBSWY3DPEHPK3PXP';
  writeFileSync(join(dir, 'web.toml'), [
    `bind_addr = "127.0.0.1:${port}"`,
    `socket_path = "${join(dir, 'nowhere.sock')}"`,
    `ssl_dir = "${join(dir, 'ssl')}"`,
    `data_dir = "${dir}"`,
    `language = "en"`,
    `session_key = "ui-check-verify-session-key-32byte"`,
    `username = "${USER}"`,
    `password = "${hash}"`,
    `totp_secret = "${secret}"`,
    `recovery_codes = []`,
    `update_check = false`,
  ].join('\n'));

  const proc = spawn('bin/easywall-web', ['-config', join(dir, 'web.toml')], { stdio: 'inherit' });
  try {
    const base = `https://127.0.0.1:${port}`;
    await waitForPort(`${base}/login`);

    const ctx = await screenshotContext(browser, theme);
    const page = await ctx.newPage();
    await page.goto(`${base}/login`, { waitUntil: 'load' });
    await page.fill('input[name=username]', USER);
    await page.fill('input[name=password]', PASS);
    await Promise.all([
      page.waitForLoadState('load'),
      page.click("form[action='/login'] button[type=submit]"),
    ]);
    if (!page.url().includes('/login/verify')) {
      throw new Error(`the password step landed on ${page.url()}, want /login/verify`);
    }
    await shoot(page, 'two-factor-verify', theme);
    await ctx.close();
    return true;
  } finally {
    proc.kill();
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * The whole documented set: DEFAULT_SCREENSHOT_PAGES plus every screen that
 * is not a plain authenticated GET. Account and dataset: whatever `admin`
 * and the demo seed's DEMO_MODE instance at BASE hold — see this script's
 * USER/PASS and internal/web/democlient.go's seed().
 */
async function takeFullScreenshotSet(browser, session) {
  await takeScreenshots(browser, session, DEFAULT_SCREENSHOT_PAGES);
  const notCaptured = [];
  for (const theme of ['light', 'dark']) {
    await takeLoginScreenshot(browser, theme);
    await takeEnrolmentScreenshots(browser, theme);
    await takeWizardScreenshots(browser, theme);
    if (!(await takeVerifyScreenshot(browser, theme))) {
      notCaptured.push(`two-factor-verify-${theme}.png`);
    }
  }
  if (notCaptured.length) {
    console.log(`\nNot re-taken (see the "skip" lines above for why): ${notCaptured.join(', ')}`);
  }
}

const browser = await chromium.launch(launch);
try {
  console.log(`Driving ${BASE}`);
  const setup = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await setup.newPage();
  await setUpAccount(page);
  await signIn(page);
  // One sign-in for the whole run — see signIn for what doing it per context
  // cost. Every context below starts already authenticated.
  const session = await setup.storageState();
  await setup.close();

  if (screenshotMode) {
    if (screenshotArgs.length) {
      await takeScreenshots(browser, session, screenshotArgs);
    } else {
      await takeFullScreenshotSet(browser, session);
    }
  } else {
    await runChecks(browser, session);
  }
} finally {
  await browser.close();
}

if (screenshotMode) {
  console.log('\nScreenshots written');
} else if (failures.length) {
  console.error(`\n${failures.length} UI check(s) failed`);
  process.exit(1);
} else {
  console.log('\nUI checks passed');
}
