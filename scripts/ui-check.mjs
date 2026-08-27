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
const USER = 'ui-check';
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
const WIDTHS = [1600, 900, 390];

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
  await checkApplyPreview(p);
  await checkEnrolmentFlow(p);
  await checkVerifyPage(browser);
  // Last, and deliberately: signing out revokes the session id every context
  // above is sharing, so anything after it would be driving a signed-out browser.
  await checkSignOutEndsTheSession(p);
  await ctx.close();
} finally {
  await browser.close();
}

if (failures.length) {
  console.error(`\n${failures.length} UI check(s) failed`);
  process.exit(1);
}
console.log('\nUI checks passed');
