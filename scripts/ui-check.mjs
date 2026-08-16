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
 */
import { chromium } from 'playwright-core';

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
  await checkForwardingPortIsNotReparsed(p);
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
