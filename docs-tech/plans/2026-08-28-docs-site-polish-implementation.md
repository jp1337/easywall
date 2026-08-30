---
title: "Docs site polish — implementation plan"
date: 2026-08-28
spec: docs-tech/specs/2026-08-28-docs-site-polish.md
branch: docs/site-polish
---

# Docs site polish — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the twenty-one documentation-site findings from the 2026-08-28
user-testing round — six mechanics fixes, four new pages plus two splits and
three consolidations, and a plain-language rewrite of 12,781 prose words —
without changing one claim the documentation makes about the firewall.

**Architecture:** Three phases in one branch, in order, because Phase 2 splits
pages that Phase 3 rewrites and doing it the other way round rewrites them
twice. Phase 1 touches only `web/src/docs.css`, `docs/_layouts/default.html`
and `docs/_includes/search.html` and is proved by a new browser check;
Phase 2 adds pages, a generator and two guard tests; Phase 3 is prose, held to
a measurable sentence-length rule and a manual claim re-read.

**Tech Stack:** Jekyll 4.3 + kramdown/rouge (built through a `ruby:3.4`
container — this machine is immutable and has no Ruby), Pagefind 1.5.2,
Tailwind 4.1 for `docs.css`, Node 24 + `playwright-core` for the browser
checks, Go 1.x for the guard tests.

**Spec:** [`docs-tech/specs/2026-08-28-docs-site-polish.md`](../specs/2026-08-28-docs-site-polish.md)

---

## Corrections to the spec, found while planning

The spec was checked against the code before this plan was written. Four of its
statements do not hold. Each is corrected in the task that touches it; they are
collected here so nobody re-derives them.

| Spec says | What the code says |
|---|---|
| "Delete the **five** `Done in 2.x` sections (~100 lines)" | There are **seven**: 2.11, 2.10, 2.9, 2.8, 2.7, 2.6, 2.5 — `roadmap.md:79–166`, 88 lines. Task 13 deletes all seven |
| `TestEveryDocsPageIsInTheNav`: "26 pages becomes 30"; the search-index count "is picked up with no edit" | **Neither test needs an edit.** `docs_nav_test.go` derives both sides by walking `docs/_docs` and parsing the `nav:` block, and the action derives `pages` from `find _docs -name '*.md'`. Adding a page plus its nav entry passes both automatically. Only `TestEveryPageIsDocumented`'s `documented` map is a literal, and only its `/whitelist` row changes (Task 12) |
| The new guard: "`easywall.wdkro.de` appears in no published file. Exempt: `CHANGELOG.md` and `docs-tech/plans/`" | `CHANGELOG.md:887` names the old host, and Task 9 **publishes `CHANGELOG.md`** as `docs/_docs/changelog.md`. The generated page must be exempt too, or the guard fails the moment the generator runs. Task 8 writes the exemption in; Task 9 asserts it still passes |
| "42 sentences over 40 words" | Not reproducible. A tokenizer that strips front matter, fenced code, table rows, HTML lines, Liquid lines and inline code counts **63 over 40 and 155 over 30**, corpus average 22.7. Task 14 commits that tokenizer as `scripts/prose-check.mjs` so the number has one definition, and Phase 3 is measured against it rather than against 42 |

### The fifth, and the one that changes an approach

**`processResult` cannot drop a result.** The spec's Phase 1 item 3 — "a
`processResult` hook drops a result unless some `<mark>` is at least as long as
the query word it matched" — rests on the option existing. It does exist, and it
cannot do that. In `docs/_site/pagefind/pagefind-ui.js` (Pagefind 1.5.2) the
hook is applied per result **card**, inside the Svelte component, as:

```js
c = async o => { t(1, n = await o.data()), t(1, n = a?.(n) ?? n), ... }
```

`a` is `process_result`. The `?? n` is the whole problem: returning `null` hands
back the original data and the card renders anyway. Worse, the hook runs lazily
per card *after* the result list and the "23 results" message have been built
from the raw result set, so even a hook that could drop one could not correct
the count.

The discriminator in the spec is right and is kept unchanged. What changes is
where it is applied: **over the rendered results**, from a `MutationObserver` on
the panel, hiding the cards that fail and rewriting the count message. Task 4
carries the code. Two further facts, confirmed rather than corrected:

- `processTerm` exists too and is no help — it transforms the query string before the search, and whether Pagefind will truncate a term is not knowable at that point.
- `focusOnSlash` is a PagefindUI 1.5.2 option. It is **not** used: it focuses the mounted input, and the input does not exist until the overlay has been opened once. It is left at its default `false`, or a second `/` handler fires beside ours.
- `README.md` is not built by Jekyll, so it cannot read `site.demo_url`. It takes the new host as a literal (Task 8).

---

## Global Constraints

Every task's requirements implicitly include this section.

- **Branch:** `docs/site-polish`. Do not merge to `main` inside this plan.
- **Building the site needs a container.** There is no `ruby`, `gem`, `bundler`
  or `jekyll` on this machine — Bazzite is immutable. `podman` is at
  `/usr/bin/podman`. Task 1 is a prerequisite for every task after it.
- **Pagefind is pinned at `1.5.2`**, exactly, everywhere. Never `pagefind@1`.
- **A generated file is rebuilt and diffed, never assumed.** After any change to
  `web/src/docs.css`, run `npm run build:docs-css` and then **grep the built
  file** for the rule — Tailwind drops rules silently and the build stays green.
- **Verify UI by rendering it, not by reading CSS.** Every visual change is
  checked at **1600 / 900 / 390** px wide in **both themes** (`data-theme` is
  `easywall-dark`; the light default is the absence of it).
- **Screenshots are not re-taken.** Nothing in this branch changes the
  application interface, so `docs/assets/img/screens/*` still describes what
  ships. This is the one branch where that rule is suspended, and it is
  suspended for that reason only.
- **`docs/` is published; `docs-tech/` never is.** `TestTheTechnicalDocsAreNotPublished`
  enforces it structurally. Nothing this plan creates under `docs-tech/` may
  acquire a copy under `docs/`.
- **Claims are preserved without exception.** Every number, key name, path,
  flag, table and code block survives Phase 3 byte-identical unless a task says
  otherwise. The sentences whose wording is load-bearing are listed in
  `docs-tech/i18n-review.md`.
- **Commit format** matches the branch's existing history: `type(scope): a
  lowercase sentence saying what is now true`. Examples from this branch:
  `fix(web): the version badge fits the version the build actually carries`,
  `docs(site): the sidebar groups its twenty-seven pages into five sections`.
- **New demo host:** `demo.easywall-project.org`, credentials `demo` / `demo`.
  The old host is `easywall.wdkro.de`. `telemetry.wdkro.de` is a different host
  and is never touched.
- **Do not rename Blacklist/Whitelist.** 961 occurrences across 48 files in
  `internal/` alone; that is its own release. The two pages split here under
  their **current** names.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `scripts/docs-build.sh` | **create** — build `docs/_site` and its Pagefind index through `ruby:3.4` in podman | 1 |
| `scripts/docs-check.mjs` | **create** — four browser assertions against a served `_site`; the only proof Phase 1 has | 2–5 |
| `scripts/render-changelog.mjs` | **create** — `CHANGELOG.md` → `docs/_docs/changelog.md`, with `--check` | 9 |
| `scripts/prose-check.mjs` | **create** — the one definition of "a prose sentence" and its length | 14 |
| `package.json` | **modify** — `build:docs`, `check:docs`, `build:changelog`, `check:changelog`, `check:prose` | 1, 2, 9, 14 |
| `docs/_layouts/default.html` | **modify** — scroll spy (`238–287`), search key (`315–382`), `processResult`, copy buttons, footer licence link (`179`) | 2–5, 10 |
| `docs/_includes/search.html` | **modify** — the `kbd` badge reads `/` | 3 |
| `docs/_includes/demo-callout.html` | **create** — one demo banner, reading the host and credentials from `_config.yml` | 8 |
| `web/src/docs.css` | **modify** — copy button, search clear button, two `em` deletions | 5, 6, 7 |
| `docs/assets/css/style.css` | **generated** — committed output of `npm run build:docs-css` | 5, 6, 7 |
| `docs/_config.yml` | **modify** — `demo_url` / `demo_user` / `demo_password`, four new nav entries | 8–12 |
| `docs/_docs/changelog.md` | **generated, committed** — one `<details>` per version | 9 |
| `docs/_docs/license.md` | **create** — Explanation: what GPL-3.0 means to somebody running easywall | 10 |
| `docs/_docs/installation/reverse-proxy.md` | **create** — How-to: put easywall behind nginx | 11 |
| `docs/_docs/features/whitelist.md` | **create** — the split out of `blacklist.md` | 12 |
| `CHANGELOG.md` | **modify** — one headline line per version, 30 of them | 9 |
| `internal/shared/docs_demo_host_test.go` | **create** — the old host appears in no published file | 8 |
| `internal/shared/docs_coverage_test.go` | **modify** — `/whitelist` remaps to `whitelist.md` | 12 |
| `.github/workflows/docs.yml` | **modify** — `codespell` step | 23 |

---

## Phase 0 — the prerequisite

### Task 1: The site builds on this machine

Nothing after this task can be verified without it. Every remaining task
renders the site.

**Files:**
- Create: `scripts/docs-build.sh`
- Modify: `package.json`

**Interfaces:**
- Consumes: nothing.
- Produces: `npm run build:docs` → `docs/_site/` containing the built site and
  `docs/_site/pagefind/` containing the search index. Every later task's
  verification step calls it.

- [ ] **Step 1: Write the build script**

```bash
cat > scripts/docs-build.sh <<'EOF'
#!/usr/bin/env bash
# Builds docs/_site and its search index.
#
# There is no ruby, gem, bundler or jekyll on this machine and there will not
# be — Bazzite's root filesystem is immutable — so the Jekyll half runs in a
# container. Rootless podman maps container root to the invoking user, so what
# lands in docs/_site is owned by you and not by root.
#
# The gem cache is a named volume. Without it every run re-resolves the Gemfile
# from rubygems.org, which is forty seconds of nothing.
#
# JEKYLL_ENV=production matches both jobs in .github/workflows/docs.yml. A
# development build differs — jekyll-seo-tag emits different tags — and a check
# against it proves something about a site nobody ships.
set -euo pipefail
cd "$(dirname "$0")/.."

podman run --rm \
  -v "$PWD/docs:/srv:Z" \
  -v easywall-docs-gems:/usr/local/bundle \
  -w /srv \
  docker.io/library/ruby:3.4 \
  sh -c 'bundle install --quiet && JEKYLL_ENV=production bundle exec jekyll build'

# The exact version .github/actions/build-search-index pins, with the exact same
# three flags. A floating range, or one flag out of step, means the index these
# checks run against is not the index easywall-project.org serves.
npx --yes pagefind@1.5.2 --site docs/_site \
  --glob "docs/**/*.html" \
  --root-selector "main.content" \
  --force-language en
EOF
chmod +x scripts/docs-build.sh
```

- [ ] **Step 2: Add the npm script**

In `package.json`, in `"scripts"`, after `"check:diagrams"`:

```json
    "build:docs": "scripts/docs-build.sh",
```

- [ ] **Step 3: Run it**

Run: `npm run build:docs`

Expected: `bundle install` pulls jekyll 4.3 and the three plugins, Jekyll writes
`docs/_site`, Pagefind reports `Indexed 26 pages`.

- [ ] **Step 4: Verify the output, and that it is yours**

Run:

```bash
find docs/_site -name '*.html' | wc -l          # >= 15
test -f docs/_site/index.html && echo landing ok
test -f docs/_site/sitemap.xml && echo sitemap ok
find docs/_site/pagefind/fragment -type f | wc -l   # 26, one per page in docs/_docs
stat -c '%U' docs/_site/index.html               # your username, not root
```

Expected: all five lines as annotated. If `stat` says `root`, rootless podman is
not in use — stop and say so rather than working around it with `sudo chown`.

- [ ] **Step 5: Commit**

```bash
git add scripts/docs-build.sh package.json
git commit -m "build(docs): the site builds on a machine with no ruby on it"
```

---

## Phase 1 — mechanics

No content changes. `web/src/docs.css`, `docs/_layouts/default.html`,
`docs/_includes/search.html`, and one new check script.

### Task 2: The contents column reaches the bottom of a short page

The reported symptom was a height bug. It is not: `.docs-toc` is
`position: fixed` with `max-height: calc(100vh - 6rem)` and has all the height
it needs. It is the scroll spy — `default.html:285` observes with
`rootMargin: '-72px 0px -70% 0px'`, a band that ends 70 % up the viewport, and
on a short page the last heading never enters it, so the last entries never
become current.

Note against the spec: it calls this "Net deletion". With the
scrolled-to-bottom rule and the rAF throttle it is about the same size. The
behaviour is what was promised, not the line count.

**Files:**
- Create: `scripts/docs-check.mjs`
- Modify: `package.json`, `docs/_layouts/default.html:266–287`

**Interfaces:**
- Consumes: `npm run build:docs` from Task 1.
- Produces: `scripts/docs-check.mjs` exporting nothing, run as `npm run check:docs`.
  Tasks 3, 4 and 5 each add one `check*` function to it and one line to `main()`.
  The shared helpers they rely on: `serve(root) -> {server, port}`,
  `ok(cond, what)` which prints and counts, and a module-level `let failed = 0`.

- [ ] **Step 1: Write the failing check**

`scripts/docs-check.mjs`, complete file:

```js
#!/usr/bin/env node
/**
 * Four assertions about the documentation site, in a real browser.
 *
 * scripts/ui-check.mjs drives the application in demo mode; nothing drove the
 * documentation. Everything Phase 1 of the docs-site-polish branch changed is
 * behaviour — a scroll handler, a key binding, a result filter, a clipboard
 * write — and none of it is visible to a Go test against the stylesheet or to
 * a diff.
 *
 * Four assertions, deliberately. This is not a suite; it is the four things
 * that were broken.
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

async function main() {
  const { server, port } = await serve(ROOT);
  const base = `http://127.0.0.1:${port}`;
  const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const page = await context.newPage();

  try {
    await checkContents(page, base);
  } finally {
    await browser.close();
    server.close();
  }

  console.log(failed ? `\n${failed} failed` : '\nall checks passed');
  process.exit(failed ? 1 : 0);
}

main();
```

Add to `package.json` `"scripts"`:

```json
    "check:docs": "node scripts/docs-check.mjs",
```

- [ ] **Step 2: Run it to see it fail**

Run: `npm run build:docs && npm run check:docs`

Expected: FAIL on `the last entry is current at the bottom of a short page`.
The first assertion passes — the column exists and has three entries. If the
run dies with `Executable doesn't exist`, run `npx playwright-core install
chromium` first; that is setup, not the failure being looked for.

- [ ] **Step 3: Replace the observer with a scroll handler**

In `docs/_layouts/default.html`, replace everything from the comment
`// Scroll spy. rootMargin pins the "active" line near the top of the` down to
and including `heads.forEach(function (h) { obs.observe(h); });` with:

```js
      // Scroll spy. One rAF-throttled scroll handler, not an
      // IntersectionObserver: the observer watched a band that ended 70% up the
      // viewport, so on a short page — three headings and 475 words, which is
      // export-import — the last heading could never enter it and the last
      // entries never became current. Reported as the column not reaching the
      // bottom; it is the right height and always was.
      var current = null;
      function mark(a) {
        if (current === a) return;
        if (current) current.removeAttribute('aria-current');
        current = a;
        if (current) current.setAttribute('aria-current', 'true');
      }

      // Two rules, and the second is the fix: current is the last heading whose
      // top has passed 100px — just under the 72px topbar — and unconditionally
      // the last heading once the document is scrolled to its end.
      function spy() {
        ticking = false;
        var i = 0;
        for (var j = 0; j < heads.length; j++) {
          if (heads[j].getBoundingClientRect().top <= 100) i = j;
        }
        // 2px of slack, not 0: a fractional device pixel ratio leaves scrollY
        // a hair short of the arithmetic bottom, and the last entry would light
        // up on some displays and not others.
        if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2) {
          i = heads.length - 1;
        }
        mark(links[i]);
      }

      var ticking = false;
      function onScroll() {
        if (ticking) return;
        ticking = true;
        requestAnimationFrame(spy);
      }
      window.addEventListener('scroll', onScroll, { passive: true });
      window.addEventListener('resize', onScroll, { passive: true });
      spy();
```

The existing `var current = null;` and `function mark(a) {...}` above the
observer are replaced by the copies here — do not leave two definitions.

- [ ] **Step 4: Run it to see it pass**

Run: `npm run build:docs && npm run check:docs`

Expected: both assertions ok.

- [ ] **Step 5: Verify it by breaking it**

Temporarily delete the four-line `if (window.innerHeight + window.scrollY >= ...)`
block, rebuild, re-run. Expected: FAIL again. Put it back. A test that passes
whatever the implementation does is not a test — 2.12 shipped seven of those.

- [ ] **Step 6: Render it**

At **1600 / 900 / 390** wide, both themes, on `/docs/features/export-import/`
(short) and `/docs/configuration/` (long): scroll top to bottom and watch the
current entry track the heading. At 390 the contents column is not rendered —
confirm no console error from the handler running with `nav` removed.

- [ ] **Step 7: Commit**

```bash
git add scripts/docs-check.mjs package.json docs/_layouts/default.html
git commit -m "fix(docs): the last contents entry is current at the bottom of a short page"
```

---

### Task 3: `/` opens the search

The Ctrl/Cmd branch goes, and with it the `navigator.platform` sniff — `/` is
the same key everywhere, so the macOS correction has nothing left to correct.

**Files:**
- Modify: `scripts/docs-check.mjs`, `docs/_layouts/default.html:315–382`, `docs/_includes/search.html`

**Interfaces:**
- Consumes: `serve`, `ok`, `failed` and `main()` from Task 2.
- Produces: `checkSearchKey(page, base)`, called from `main()` after `checkContents`.

- [ ] **Step 1: Write the failing check**

Add to `scripts/docs-check.mjs`, above `main()`:

```js
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
  await field.waitFor({ state: 'visible', timeout: 20000 });
  await field.pressSequentially('a/b');
  ok(await dialog.evaluate(d => d.open), 'the overlay stays open while / is typed into it');
  ok(await field.inputValue() === 'a/b', '/ reaches the search field');

  await page.keyboard.press('Escape');
}
```

and in `main()`, after `await checkContents(page, base);`:

```js
    await checkSearchKey(page, base);
```

- [ ] **Step 2: Run it to see it fail**

Run: `npm run build:docs && npm run check:docs`

Expected: FAIL on `the badge on the trigger reads /` (it reads `Ctrl K`) and
FAIL on `/ opens the search overlay`.

- [ ] **Step 3: Change the badge**

In `docs/_includes/search.html`, replace the line

```html
    <kbd class="docs-search-key" id="docs-search-key" aria-hidden="true">Ctrl K</kbd>
```

with

```html
    <kbd class="docs-search-key" id="docs-search-key" aria-hidden="true">/</kbd>
```

and in the `{% comment %}` block above it, replace the paragraph beginning
`The trigger carries the shortcut rather than hiding it:` with:

```
  The trigger carries the shortcut rather than hiding it: a key nobody is told
  about is a key nobody presses. It says `/` and stays saying it — one key, the
  same on every keyboard, so nothing has to be corrected at runtime for anybody.
```

- [ ] **Step 4: Change the binding**

In `docs/_layouts/default.html`, delete these three lines:

```js
      var hint    = document.getElementById('docs-search-key');
```

```js
      // The markup says Ctrl because that is right for most readers of a Linux
      // firewall's documentation; this corrects it for the rest. navigator.platform
      // is deprecated and still the only thing every shipping browser answers.
      if (/Mac|iPhone|iPad|iPod/.test(navigator.platform || '')) hint.textContent = '⌘ K';
```

(`hint` is not named in the guard clause below it, so nothing else needs
touching there.) Then replace:

```js
      document.addEventListener('keydown', function (e) {
        if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
          e.preventDefault();
          open();
        }
      });
```

with:

```js
      document.addEventListener('keydown', function (e) {
        if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
        // Or `/` cannot be typed into the search field this very handler opened,
        // and every future field on the page inherits the same problem.
        // isContentEditable covers the case where the editor is a <div>.
        var t = e.target;
        if (t && (t.isContentEditable ||
                  (t.matches && t.matches('input, textarea, select')))) return;
        e.preventDefault();
        open();
      });
```

PagefindUI 1.5.2 has its own `focusOnSlash` option. Leave it at its default
`false`: it focuses an input that does not exist until the overlay has been
opened once, and switching it on adds a second `/` handler beside this one.

- [ ] **Step 5: Run it to see it pass**

Run: `npm run build:docs && npm run check:docs`

Expected: all six assertions ok.

- [ ] **Step 6: Verify it by breaking it**

Temporarily delete the two-line `var t = e.target; if (t && ...) return;` guard,
rebuild, re-run. Expected: FAIL on `/ reaches the search field` — the field
holds `ab`, not `a/b`. Put it back.

- [ ] **Step 7: Render it**

At **1600 / 900 / 390**, both themes: the badge reads `/` and is not clipped in
the 260px sidebar at 1600, and the trigger is still legible in the mobile drawer
at 390.

- [ ] **Step 8: Commit**

```bash
git add scripts/docs-check.mjs docs/_layouts/default.html docs/_includes/search.html
git commit -m "fix(docs): slash opens the search, on every keyboard there is"
```

---

### Task 4: A search for `asdasd` finds nothing

Pagefind truncates a query term until a prefix matches something in the index.
`asdasd` returns 23 results, every one of them matching `as`; `xyzzyq` returns
one, matching the `X` in `iptables -X`; `zzzzzzzz` returns none, because nothing
in the index starts with `z`. Forward prefix matching is the behaviour that is
wanted — `config` must find `configuration` — and it is also the discriminator:
a real prefix match marks a word **at least as long** as what was typed;
truncation always marks a **shorter** one.

**Read the fifth correction at the top of this plan before starting.** The
spec's `processResult` hook cannot do this. It exists, and PagefindUI 1.5.2
applies it per result card as `data = processResult(data) ?? data` — returning
`null` hands back the original and the card renders anyway — and it runs after
the result list and its "23 results" message have already been built. The
discriminator is unchanged; it is applied over the rendered results instead.

**Files:**
- Modify: `scripts/docs-check.mjs`, `docs/_layouts/default.html` (inside the search IIFE, after the `PagefindUI` constructor call)

**Interfaces:**
- Consumes: `serve`, `ok`, `failed`, `main()` from Task 2; `panel` and `dialog`, already in scope inside the search IIFE.
- Produces: `checkSearchResults(page, base)`, called from `main()` after `checkSearchKey`.

- [ ] **Step 1: Write the failing check**

Add to `scripts/docs-check.mjs`, above `main()`:

```js
// Both directions, in one check, because either alone is satisfiable by doing
// nothing: today `asdasd` finds 23 pages, and a filter crude enough to fix that
// takes `config` -> `configuration` with it.
async function checkSearchResults(page, base) {
  console.log('search results');
  await page.goto(base + '/docs/features/ports/');
  await page.keyboard.press('/');

  const field = page.locator('#docs-search-panel input');
  await field.waitFor({ state: 'visible', timeout: 20000 });

  const shown = page.locator('#docs-search-panel .pagefind-ui__result:not([hidden])');

  await field.fill('asdasd');
  await page.waitForTimeout(900);   // PagefindUI debounces at 300ms
  ok(await shown.count() === 0, 'asdasd finds nothing');
  ok(/no results/i.test(await page.locator('#docs-search-panel .pagefind-ui__message').textContent()),
     'and says so');

  await field.fill('config');
  await page.waitForTimeout(900);
  const hrefs = await page.locator('#docs-search-panel .pagefind-ui__result:not([hidden]) a')
                          .evaluateAll(as => as.map(a => a.getAttribute('href')));
  ok(hrefs.some(h => h && h.includes('/docs/configuration/')),
     'config still finds the configuration page');

  await page.keyboard.press('Escape');
}
```

and in `main()`, after `await checkSearchKey(page, base);`:

```js
    await checkSearchResults(page, base);
```

- [ ] **Step 2: Run it to see it fail**

Run: `npm run build:docs && npm run check:docs`

Expected: FAIL on `asdasd finds nothing` (23 results) and FAIL on `and says so`.
The third assertion already passes — that is the half that must survive.

- [ ] **Step 3: Prune the rendered results**

In `docs/_layouts/default.html`, inside the search IIFE, immediately after the
`new window.PagefindUI({ ... });` call and before `focusField();`, insert:

```js
          prune.watch();
```

and add these two functions inside the same IIFE, above `function focusField()`:

```js
      // Pagefind truncates a query term until a prefix matches something in the
      // index: `asdasd` returns 23 results, every one of them matching `as`, and
      // `xyzzyq` returns one, matching the `X` in `iptables -X`. Forward prefix
      // matching is the behaviour that is wanted — `config` has to find
      // `configuration` — and it is also the clean discriminator, because a real
      // prefix match marks a word at least as long as what was typed while
      // truncation always marks a shorter one.
      //
      // This runs over the rendered results, and not through the processResult
      // option, which looks like the hook for exactly this and is not: 1.5.2
      // applies it per card as `data = processResult(data) ?? data`, so
      // returning null hands the original straight back and the card renders
      // anyway — and it runs after the count has been computed. Read in the
      // built bundle, not inferred from the option list.
      var prune = (function () {
        function run() {
          var field = panel.querySelector('.pagefind-ui__search-input');
          var typed = (field && field.value || '').toLowerCase();
          var words = typed.split(/\s+/).filter(Boolean);
          var cards = panel.querySelectorAll('.pagefind-ui__result');
          var kept = 0;

          for (var i = 0; i < cards.length; i++) {
            var marks = cards[i].querySelectorAll('mark');
            var texts = [];
            for (var m = 0; m < marks.length; m++) texts.push(marks[m].textContent.toLowerCase());

            // Every word typed has to have marked, somewhere in this result, a
            // word it is a genuine prefix of. The length test alone is not
            // enough on a two-word query, where one word can match fully and
            // carry a second that was truncated.
            var keep = !words.length || words.every(function (w) {
              return texts.some(function (k) { return k.length >= w.length && k.indexOf(w) === 0; });
            });

            // ponytail: a stemmed query whose marked form is shorter than what
            // was typed — `running` marked as `run` — is dropped by this rule.
            // No such term exists in the corpus today; if one appears, widen to
            // "or the mark is a prefix of the word and at least 3 characters"
            // rather than removing the length test.
            if (cards[i].hidden === keep) cards[i].hidden = !keep;
            if (keep) kept++;
          }

          var msg = panel.querySelector('.pagefind-ui__message');
          if (msg && words.length) {
            var want = kept === 0
              ? 'No results for ' + (field.value || '')
              : kept + (kept === 1 ? ' result for ' : ' results for ') + (field.value || '');
            // Only when it differs, or writing it is itself a mutation and the
            // observer below calls this forever.
            if (msg.textContent !== want) msg.textContent = want;
          }
        }

        return {
          watch: function () {
            new MutationObserver(function () { run(); })
              .observe(panel, { childList: true, subtree: true });
            run();
          }
        };
      })();
```

- [ ] **Step 4: Run it to see it pass**

Run: `npm run build:docs && npm run check:docs`

Expected: all nine assertions ok.

- [ ] **Step 5: Verify it by breaking it, twice**

Both directions, because each is satisfiable by doing nothing:

1. Change `k.length >= w.length` to `k.length >= 0`. Re-run: FAIL on
   `asdasd finds nothing`. Revert.
2. Change it to `k.length >= w.length + 20`. Re-run: FAIL on
   `config still finds the configuration page`. Revert.

- [ ] **Step 6: Check by hand for a runaway observer**

Open the overlay in a browser, type `config`, and watch the console and the CPU
for five seconds. The observer's callback rewrites the message; if the
`msg.textContent !== want` guard were dropped, that write is itself a mutation
and the loop never ends. Confirm it settles.

- [ ] **Step 7: Render it**

At **1600 / 900 / 390**, both themes: `asdasd` shows the empty message and no
card; `config` shows results; the panel does not collapse to zero height when
everything is hidden.

- [ ] **Step 8: Commit**

```bash
git add scripts/docs-check.mjs docs/_layouts/default.html
git commit -m "fix(docs): a search that matches nothing says nothing was found"
```

---

### Task 5: Every code block can be copied

`.highlighter-rouge` is the element carrying the frame — `div.highlight` and
`pre.highlight` inside it are deliberately bare — so that is what the button is
positioned against, not the `<pre>`, which scrolls.

Feedback is the button's own label swapping to **Copied** for two seconds plus
one `aria-live="polite"` region. That is the convention on GitHub and MDN, it
needs no positioning, and a floating toast is announced either badly or not at
all.

**Files:**
- Modify: `scripts/docs-check.mjs`, `docs/_layouts/default.html`, `web/src/docs.css`
- Generated: `docs/assets/css/style.css`

**Interfaces:**
- Consumes: `serve`, `ok`, `failed`, `main()` from Task 2.
- Produces: `checkCopyButton(context, page, base)` — note it takes the browser
  context, for `grantPermissions`. Called from `main()` last.

- [ ] **Step 1: Write the failing check**

Add to `scripts/docs-check.mjs`, above `main()`:

```js
// The clipboard is the only part of this that a screenshot cannot show, and the
// only part that can be silently wrong: a button that says Copied having
// written nothing looks exactly like one that worked.
async function checkCopyButton(context, page, base) {
  console.log('copy buttons');
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: base });
  await page.goto(base + '/docs/installation/debian/');

  const block = page.locator('.content-body .highlighter-rouge').first();
  const want = (await block.locator('pre').innerText()).replace(/\n$/, '');
  const btn = block.locator('.docs-copy');

  ok(await btn.count() === 1, 'a code block carries exactly one copy button');
  if (!(await btn.count())) return;

  await btn.click();
  ok((await btn.textContent()).trim() === 'Copied', 'the button confirms in its own label');
  ok(await page.evaluate(() => navigator.clipboard.readText()) === want,
     'the clipboard holds the code block');
}
```

and in `main()`, after `await checkSearchResults(page, base);`:

```js
    await checkCopyButton(context, page, base);
```

- [ ] **Step 2: Run it to see it fail**

Run: `npm run build:docs && npm run check:docs`

Expected: FAIL on `a code block carries exactly one copy button` (there are none).

- [ ] **Step 3: Add the buttons**

In `docs/_layouts/default.html`, in the first `<script>` block, after the
on-page-contents IIFE and before the diagrams comment, add:

```js
    // A copy button in every code block. .highlighter-rouge is the element
    // carrying the frame — div.highlight and pre.highlight inside it are
    // deliberately bare — so the button is positioned against that and not
    // against the <pre>, which scrolls horizontally and would carry the button
    // off the screen with it.
    //
    // The confirmation is the button's own label for two seconds plus one
    // aria-live region: the convention on GitHub and on MDN. A floating toast
    // needs positioning it cannot get right over a scrolling <pre>, and is
    // announced either badly or not at all.
    (function () {
      var blocks = document.querySelectorAll('.content-body .highlighter-rouge');
      if (!blocks.length || !navigator.clipboard) return;

      // Styled inline rather than with .sr-only: nothing under docs/ uses that
      // class, so the utility is correctly absent from the built stylesheet —
      // see the note in TestDocsStylesheetKeepsLoadBearingRules — and adding a
      // class here to make Tailwind emit it again is the tail wagging the dog.
      var live = document.createElement('div');
      live.setAttribute('aria-live', 'polite');
      live.style.cssText = 'position:absolute;width:1px;height:1px;overflow:hidden;' +
                           'clip:rect(0 0 0 0);white-space:nowrap';
      document.body.appendChild(live);

      [].forEach.call(blocks, function (block) {
        var pre = block.querySelector('pre');
        if (!pre) return;

        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'docs-copy';
        btn.textContent = 'Copy';

        btn.addEventListener('click', function () {
          // innerText, not textContent: it is what the reader sees, with the
          // rouge spans flattened and the line breaks kept. The trailing
          // newline goes; a trailing space inside a config example does not.
          navigator.clipboard.writeText(pre.innerText.replace(/\n$/, '')).then(function () {
            btn.textContent = 'Copied';
            btn.setAttribute('data-copied', '');
            live.textContent = 'Copied to clipboard';
            setTimeout(function () {
              btn.textContent = 'Copy';
              btn.removeAttribute('data-copied');
              live.textContent = '';
            }, 2000);
          }, function () {
            // A clipboard write is refused over plain HTTP and in a few
            // configurations over HTTPS. Say so rather than confirming.
            btn.textContent = 'Ctrl+C';
            setTimeout(function () { btn.textContent = 'Copy'; }, 2000);
          });
        });

        block.appendChild(btn);
      });
    })();
```

- [ ] **Step 4: Style them**

In `web/src/docs.css`, immediately after the inline-code block that begins
`/* ── Inline code ─────` and its rules, add:

```css
/* ── Copy buttons ─────────────────────────────────────────────────────
   Positioned against .highlighter-rouge, which is the element carrying the
   frame. Against the <pre> the button would scroll out of the block with a
   long command. */
.content-body .highlighter-rouge { position: relative; }

.content-body .docs-copy {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 3px 9px;
  font-family: var(--font-sans);
  font-size: 11.5px;
  line-height: 1.6;
  color: var(--text-subtle);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 120ms ease;
}

/* Focus has to reveal it, or it is a tab stop that cannot be seen — worse than
   no tab stop at all. */
.content-body .highlighter-rouge:hover .docs-copy,
.content-body .docs-copy:focus-visible { opacity: 1; }

.content-body .docs-copy:hover { color: var(--text); background: var(--surface-3); }
.content-body .docs-copy[data-copied] { color: var(--accent); }

/* A touch screen never hovers, so on one the button is simply there. */
@media (hover: none) {
  .content-body .docs-copy { opacity: 1; }
}
```

- [ ] **Step 5: Rebuild the stylesheet and grep the built file**

Run:

```bash
npm run build:docs-css
grep -c 'docs-copy' docs/assets/css/style.css          # >= 4
grep -o '\.content-body \.highlighter-rouge{position:relative}' docs/assets/css/style.css
grep -o '@media (hover:none)' docs/assets/css/style.css
```

Expected: the count is at least 4 and both greps print a match. Tailwind drops
rules silently and the build stays green — a green build is not evidence the
rule shipped.

- [ ] **Step 6: Run the check to see it pass**

Run: `npm run build:docs && npm run check:docs`

Expected: all twelve assertions ok.

- [ ] **Step 7: Verify it by breaking it**

Change `pre.innerText.replace(/\n$/, '')` to `'nope'`, rebuild, re-run.
Expected: FAIL on `the clipboard holds the code block`, while
`the button confirms in its own label` still passes — which is the point of
having both. Revert.

- [ ] **Step 8: Render it**

At **1600 / 900 / 390**, both themes, on `/docs/installation/debian/` (short
blocks) and `/docs/installation/demo/` (a 20-line heredoc and two ini files):
the button does not cover the first line's text, it appears on hover and on
keyboard focus, and at 390 it is permanently visible and does not overlap the
code.

- [ ] **Step 9: Commit**

```bash
git add scripts/docs-check.mjs docs/_layouts/default.html web/src/docs.css docs/assets/css/style.css
git commit -m "feat(docs): every code block can be copied, and says when it was"
```

---

### Task 6: The search clear button is the right size

Reported as oversized and misplaced. **Diagnose by rendering. Do not guess a
number.** The existing constraint is recorded at `docs.css:1319–1324` and it is
load-bearing: the button's width and right offset stay Pagefind's, because the
input's `padding-right` is measured from them — a narrower button means the
query runs underneath it. At Pagefind's own 58px it read as a panel stuck to the
field rather than a control inside it, which is why `height` and the colours are
already overridden and the width is not.

**Files:**
- Modify: `web/src/docs.css:1325–1341`
- Generated: `docs/assets/css/style.css`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing. This task adds no assertion — whether a control looks like
  part of its field is not a thing a number in a stylesheet can say, which is
  the same reason the version badge was measured in a browser rather than
  asserted.

- [ ] **Step 1: Measure what is actually there**

Open the overlay, type a query, and in the console:

```js
const f = document.querySelector('#docs-search-panel .pagefind-ui__search-input');
const b = document.querySelector('#docs-search-panel .pagefind-ui__search-clear');
console.log({
  field:  f.getBoundingClientRect(),
  button: b.getBoundingClientRect(),
  padRight: getComputedStyle(f).paddingRight,
  btnRight: getComputedStyle(b).right,
  btnWidth: getComputedStyle(b).width
});
```

Write the six numbers into the commit message. Every later step is judged
against them.

- [ ] **Step 2: Screenshot it, at all three widths, in both themes**

Six screenshots of the overlay with a query typed. Name what is wrong in words
before changing a value — "the button's baseline sits 4px above the field's
text" is a diagnosis, "it looks big" is not.

- [ ] **Step 3: Change one property at a time**

Only within `#docs-search-panel .pagefind-ui__search-clear`, and **never**
`width` or `right`: those two are Pagefind's and the field's `padding-right` is
computed from them. `top`, `height`, `font-size`, `padding` and the colours are
yours.

- [ ] **Step 4: Prove the query does not run underneath it**

Type a query long enough to fill the field —
`acceptance duration trusted_proxies EASYWALL_WEB_TRUSTED_PROXIES` — and confirm
the text stops before the button at all three widths, in both themes. This is
the exact regression the comment in the stylesheet records.

- [ ] **Step 5: Rebuild and grep**

Run:

```bash
npm run build:docs-css
grep -o '\.pagefind-ui__search-clear{[^}]*}' docs/assets/css/style.css
```

Expected: the rule is present and holds the values just set. Confirm by eye that
neither `width` nor `right` appears in it.

- [ ] **Step 6: Update the comment**

The comment above the rule explains a decision. If the numbers changed, the
comment says the new ones — a comment that describes the previous value is worse
than none.

- [ ] **Step 7: Commit**

```bash
git add web/src/docs.css docs/assets/css/style.css
git commit -m "fix(docs): the search clear button reads as a control inside the field"
```

---

### Task 7: Emphasis stops meaning de-emphasis

`em` means emphasis and `docs.css:1525` mutes it. This is not a contrast
failure — `--text-muted` on `--bg` measures **5.99 : 1** in light and
**8.49 : 1** in dark, both past AA. It is a semantic defect, and the fix is the
same either way: two deletions.

`.content-body em` is **not** in `TestDocsStylesheetKeepsLoadBearingRules`, so
nothing asserts it and nothing needs updating there.

**Files:**
- Modify: `web/src/docs.css` (two rules deleted)
- Generated: `docs/assets/css/style.css`

**Interfaces:**
- Consumes: nothing. Produces: nothing.

- [ ] **Step 1: Delete the rule**

In `web/src/docs.css`, delete:

```css
.content-body em {
  color: var(--text-muted);
}
```

- [ ] **Step 2: Delete the exception that existed only to undo it**

In `web/src/docs.css`, delete both the comment and the rule:

```css
/* em is muted body-wide; inside a callout that lands on the tinted ground
   and loses contrast, so keep it at full strength here. */
.content-body blockquote em { color: inherit; }
```

- [ ] **Step 3: Rebuild and grep for their absence**

Run:

```bash
npm run build:docs-css
grep -c 'content-body em{' docs/assets/css/style.css          # 0
grep -c 'blockquote em{' docs/assets/css/style.css            # 0
go test ./internal/web/ -run TestDocsStylesheetKeepsLoadBearingRules
```

Expected: both counts are `0` and the Go test passes — it asserts seven other
rules, and this change must not have taken one of them with it.

- [ ] **Step 4: Render it**

At **1600 / 900 / 390**, both themes. The pages that matter: the footer
(`default.html:179` italicises the tagline), `/docs/security/` (three
multi-line callouts), `/docs/roadmap.md` (two `Amended in` callouts) and
`/docs/features/blacklist/` (italic inside a callout, the case the deleted
exception existed for). Confirm the callouts still read correctly with `em` at
full strength on the tinted ground.

- [ ] **Step 5: Commit**

```bash
git add web/src/docs.css docs/assets/css/style.css
git commit -m "fix(docs): emphasis is emphasised rather than muted"
```

---

## Phase 2 — structure

### Task 8: The demo has one address, in one place

One page uses the new host. Five still point at `easywall.wdkro.de`, a release
after the move was reported as done. `telemetry.wdkro.de` is a different host
and is not touched by anything in this task.

**Files:**
- Create: `docs/_includes/demo-callout.html`, `internal/shared/docs_demo_host_test.go`
- Modify: `docs/_config.yml`, `README.md:29`, `docs/index.md:34`, `docs/index.md:98`, `docs/_docs/index.md:44`, `docs/_docs/installation/demo.md:13`

**Interfaces:**
- Consumes: nothing.
- Produces: `site.demo_url`, `site.demo_user`, `site.demo_password` for every
  Liquid template; `{% include demo-callout.html %}`, used by any page that
  offers the demo. `TestTheOldDemoHostIsNotPublished`, which Task 9 must keep
  green when it publishes `CHANGELOG.md`.

- [ ] **Step 1: Write the failing test**

`internal/shared/docs_demo_host_test.go`:

```go
package shared

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The public demo moved to demo.easywall-project.org, and the move was reported
// as finished. One page had it. Five did not — README.md, both landing-page
// call-to-action buttons, the documentation index table and demo.md — so a
// release later, every route a reader could take to the demo still sent them to
// a host that is not it.
//
// Structural rather than a list of five files, because the list is exactly what
// failed: anything published is checked, and the two files that must keep the
// old host are named here with the reason they keep it.
func TestTheOldDemoHostIsNotPublished(t *testing.T) {
	root := repoRootDir(t)
	const old = "easywall.wdkro.de"

	// Exempt, with the reason. CHANGELOG.md records what 2.4.0 shipped, and
	// rewriting that is a lie about a release. docs/_docs/changelog.md is
	// generated from it by scripts/render-changelog.mjs, so it inherits the
	// same line and the same exemption.
	exempt := map[string]string{
		"CHANGELOG.md":            "records the host 2.4.0 actually announced",
		"docs/_docs/changelog.md": "generated from CHANGELOG.md, which keeps it",
	}

	// docs-tech/ is never published — TestTheTechnicalDocsAreNotPublished holds
	// that structurally — so the specs and plans that record the old host are
	// out of scope by construction rather than by exemption.
	skipDir := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		"docs-tech": true, "bin": true, "_site": true, ".jekyll-cache": true,
	}

	published := map[string]bool{".md": true, ".html": true, ".yml": true,
		".yaml": true, ".json": true, ".toml": true}

	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !published[strings.ToLower(filepath.Ext(path))] || exempt[rel] != "" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), old) {
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	// The guard has to be able to fail. If both exempt files stopped naming the
	// host, this test would pass by checking nothing meaningful and nobody
	// would know the pattern had stopped matching.
	for name := range exempt {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s is exempt from this check and does not exist: %v", name, err)
		}
		if !strings.Contains(string(raw), old) {
			t.Errorf("%s no longer contains %q, so its exemption is dead weight — "+
				"delete the entry rather than leaving a rule nothing exercises", name, old)
		}
	}

	sort.Strings(found)
	for _, f := range found {
		t.Errorf("%s names %s, which is not the demo any more\n"+
			"  Liquid pages read site.demo_url from docs/_config.yml; README.md is not "+
			"built by Jekyll and takes the literal", f, old)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/shared/ -run TestTheOldDemoHostIsNotPublished -v`

Expected: FAIL, naming exactly five files — `README.md`, `docs/index.md`,
`docs/_docs/index.md`, `docs/_docs/installation/demo.md`. (`docs/index.md`
carries it twice and is reported once.)

- [ ] **Step 3: Put the address in the config**

In `docs/_config.yml`, after the `discord:` block:

```yaml
# The public demo. Three values in one place because five files hardcoded the
# host and four of them were still naming the one before this a release after
# the move — see TestTheOldDemoHostIsNotPublished. _includes/demo-callout.html
# and both landing-page buttons read them from here; README.md cannot, because
# GitHub does not run Jekyll over it.
demo_url: https://demo.easywall-project.org
demo_user: demo
demo_password: demo
```

- [ ] **Step 4: Write the callout include**

`docs/_includes/demo-callout.html`:

```liquid
{% comment %}
  The demo banner, wherever the documentation offers the demo. One component, so
  the host and the credentials live in _config.yml instead of in five files —
  four of which were still on the previous host a release after it changed.

  Liquid is expanded before kramdown runs, so the blockquote below is markdown
  by the time the parser sees it and needs no markdown="1".

  The landing page keeps its own buttons. A marketing hero and a documentation
  callout are not one component, and making them one makes both worse.
{% endcomment %}
> **Try it without installing anything.** [{{ site.demo_url | remove: 'https://' }}]({{ site.demo_url }}) — sign in with `{{ site.demo_user }}` / `{{ site.demo_password }}`. Nothing there reaches a real firewall, and the state resets periodically.
```

- [ ] **Step 5: Replace the five hardcoded places**

`README.md:29` — a literal, because GitHub does not run Jekyll:

```html
  <a href="https://demo.easywall-project.org"><strong>Live demo</strong></a> ·
```

`docs/index.md:34`:

```html
    <a href="{{ site.demo_url }}" class="btn btn-soft btn-lg">Live demo ↗</a>
```

`docs/index.md:98`:

```html
    <a href="{{ site.demo_url }}" class="btn btn-primary btn-lg" target="_blank" rel="noopener">Open the demo ↗</a>
```

and the line below it, so the credentials come from one place too:

```html
  <p class="docs-cta-credentials">Sign in with <code>{{ site.demo_user }}</code> / <code>{{ site.demo_password }}</code></p>
```

`docs/_docs/index.md:44`:

```markdown
| Try it without installing anything | [Live demo]({{ site.demo_url }}) · [Demo mode]({% link _docs/installation/demo.md %}) |
```

`docs/_docs/installation/demo.md:13` — the whole line becomes the include:

```liquid
{% include demo-callout.html %}
```

- [ ] **Step 6: Run the test to see it pass**

Run: `go test ./internal/shared/ -run TestTheOldDemoHostIsNotPublished -v`

Expected: PASS.

- [ ] **Step 7: Verify the test by breaking the code**

Put `easywall.wdkro.de` back into `docs/_docs/installation/demo.md`, re-run:
FAIL naming that file. Remove it. Then delete the `CHANGELOG.md` entry from
`exempt`, re-run: FAIL naming `CHANGELOG.md`. Put it back.

- [ ] **Step 8: Render it**

Run `npm run build:docs`. At **1600 / 900 / 390**, both themes: the landing page
hero button and CTA button, `/docs/`'s table row, and `/docs/installation/demo/`
where the include now renders as a callout. Confirm the callout is a blockquote
and not four lines of literal asterisks — that is the kramdown-in-raw-HTML trap,
and it is why the include ends in markdown rather than in a `<div>`.

- [ ] **Step 9: Commit**

```bash
git add docs/_config.yml docs/_includes/demo-callout.html README.md docs/index.md \
        docs/_docs/index.md docs/_docs/installation/demo.md \
        internal/shared/docs_demo_host_test.go
git commit -m "fix(docs): the demo has one address, and every page uses it"
```

---

### Task 9: The changelog is a page

One page, one URL, one nav entry. Each version is a `<details>`; the newest is
`open` and the rest collapse to a one-line summary. `CHANGELOG.md` holds 30
versions and 1,272 lines, so flat it is fifteen screens — and per-version
routing is 30 URLs nobody links to.

`CHANGELOG.md` **stays the single source** at the repository root: GitHub reads
it and release tooling reads it. The page is generated from it and committed,
like `docs/assets/diagrams/` and the built CSS, because the site is built by
Jekyll on a runner that does not run Node.

**Files:**
- Create: `scripts/render-changelog.mjs`, `docs/_docs/changelog.md` (generated)
- Modify: `CHANGELOG.md` (30 headlines), `package.json`, `docs/_config.yml` (nav), `docs/_layouts/default.html` (one line), `web/src/docs.css`
- Generated: `docs/assets/css/style.css`

**Interfaces:**
- Consumes: `TestTheOldDemoHostIsNotPublished` from Task 8 — its
  `docs/_docs/changelog.md` exemption exists for exactly this task, and this is
  where it starts being exercised.
- Produces: `npm run build:changelog` and `npm run check:changelog`;
  `/docs/changelog/`, which Task 13 links to from the roadmap.

- [ ] **Step 1: Write the generator**

`scripts/render-changelog.mjs`:

```js
#!/usr/bin/env node
/**
 * CHANGELOG.md -> docs/_docs/changelog.md
 *
 * CHANGELOG.md stays the single source at the repository root: GitHub reads it
 * and release tooling reads it. This renders it as one page with a <details>
 * per version, the newest open — thirty versions and 1,272 lines is fifteen
 * screens flat, and a URL per version is thirty URLs nobody links to.
 *
 *   npm run build:changelog    write docs/_docs/changelog.md
 *   npm run check:changelog    fail if the committed page is not what this
 *                              would write
 *
 * The output is committed, like docs/assets/diagrams/ and the built CSS: the
 * site is built by Jekyll on a runner that does not run node.
 */
import { readFileSync, writeFileSync } from 'node:fs';

const CHECK = process.argv.includes('--check');
const SRC = new URL('../CHANGELOG.md', import.meta.url);
const OUT = new URL('../docs/_docs/changelog.md', import.meta.url);

// `## [2.13.0] — 2026-08-28` and `## [2.4.2] - 2026-08-09` both occur in the
// file: the separator became an em dash at 2.5.0 and the entries below it were
// never touched. Both are read; neither is rewritten, because reformatting
// thirty historical entries to satisfy one regex is the tail wagging the dog.
const HEADER = /^## \[([^\]]+)\](?:\s*[—-]\s*(\S+))?\s*$/;

function parse(md) {
  const versions = [];
  let cur = null;
  for (const line of md.split('\n')) {
    // The link reference definitions at the foot of the file are machinery,
    // not content, and they follow the last version's body.
    if (/^\[[^\]]+\]:\s*https?:/.test(line)) continue;
    const m = HEADER.exec(line);
    if (m) {
      cur = { version: m[1], date: m[2] || '', body: [] };
      versions.push(cur);
      continue;
    }
    if (cur) cur.body.push(line);
  }
  return versions;
}

// The collapsed row needs one line saying what the release was, and nothing in
// Keep a Changelog's format provides one. It goes into CHANGELOG.md itself
// rather than into a version -> headline map here: one source, nothing to keep
// in step, and GitHub renders the file better for it.
function headline(v) {
  const i = v.body.findIndex(l => /^\*\*.+\*\*$/.test(l.trim()));
  if (i === -1) {
    console.error(
      `CHANGELOG.md: [${v.version}] has no headline.\n\n` +
      `  ## [${v.version}]${v.date ? ' — ' + v.date : ''}\n\n` +
      '  **One sentence saying what the release is.**\n\n' +
      '  ### Added\n\n' +
      '  A bold line of its own, directly under the version header. Without it the\n' +
      '  collapsed row on /docs/changelog/ has nothing to say.');
    process.exit(1);
  }
  return {
    text: v.body[i].trim().replace(/^\*\*/, '').replace(/\*\*$/, '').replace(/\.$/, ''),
    body: v.body.slice(0, i).concat(v.body.slice(i + 1))
  };
}

const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

// <summary> is raw HTML, and kramdown does not parse markdown inside raw HTML —
// the same rule that left a link rendering as literal brackets in a
// <figcaption> at installation/first-run.md:89. Inline code is the one thing a
// headline is likely to want, so it is converted here rather than forbidden.
function summary(v, text) {
  const label = v.version === 'unreleased' ? 'Unreleased' : v.version;
  return `<strong>${esc(label)}</strong>${v.date ? ' · ' + esc(v.date) : ''} — ` +
         esc(text).replace(/`([^`]+)`/g, '<code>$1</code>');
}

function render(versions) {
  const out = [
    '---',
    'layout: default',
    'title: Changelog',
    'description: Every release of easywall, newest first, and what each one was for.',
    '---',
    '',
    '<!-- Generated from CHANGELOG.md by scripts/render-changelog.mjs.',
    '     Do not edit this file. `npm run check:changelog` fails a pull request',
    '     that changes one of the two without the other. -->',
    '',
    '# Changelog',
    '',
    'Every release, newest first. The newest is open; the rest are one line each',
    'until you open them. This page is generated from',
    '[CHANGELOG.md](https://github.com/jp1337/easywall/blob/main/CHANGELOG.md),',
    'which is the file GitHub and the release tooling read.',
    ''
  ];

  versions.forEach((v, i) => {
    const h = headline(v);
    // markdown="1" is kramdown's own attribute and it is what makes the body
    // render at all: without it everything between the tags is raw HTML and
    // the release notes arrive as one paragraph of asterisks and hyphens.
    out.push(`<details${i === 0 ? ' open' : ''} markdown="1">`);
    out.push(`<summary>${summary(v, h.text)}</summary>`);
    out.push('');
    out.push(h.body.join('\n').replace(/^\n+/, '').replace(/\n+$/, ''));
    out.push('');
    out.push('</details>');
    out.push('');
  });

  return out.join('\n');
}

const versions = parse(readFileSync(SRC, 'utf8'));
const want = render(versions);

if (CHECK) {
  let have = '';
  try { have = readFileSync(OUT, 'utf8'); } catch { /* missing counts as stale */ }
  if (have === want) {
    console.log(`docs/_docs/changelog.md is current — ${versions.length} versions`);
    process.exit(0);
  }
  console.error('docs/_docs/changelog.md is not what CHANGELOG.md would produce.\n' +
                '  Run `npm run build:changelog` and commit the result.');
  process.exit(1);
}

writeFileSync(OUT, want);
console.log(`wrote docs/_docs/changelog.md — ${versions.length} versions`);
```

Add to `package.json` `"scripts"`:

```json
    "build:changelog": "node scripts/render-changelog.mjs",
    "check:changelog": "node scripts/render-changelog.mjs --check",
```

- [ ] **Step 2: Run it to see it fail on the missing headlines**

Run: `npm run build:changelog`

Expected: exits 1 with `CHANGELOG.md: [unreleased] has no headline.` and the
template. That failure is the retrofit's own to-do list.

- [ ] **Step 3: Retrofit thirty headlines**

One bold line of its own directly under each version header, with a blank line
either side:

```markdown
## [2.13.0] — 2026-08-28

**Behind a proxy, easywall knows who you are.**

### Added
```

These are **drafts in the maintainer's register, to accept or replace** — not
placeholders. Nine of them are lifted from wording that already exists in
`roadmap.md`, which Task 13 deletes; the rest are drawn from each version's own
first entry.

| Version | Headline |
|---|---|
| unreleased | **The documentation site says what it means, and a search that finds nothing says so.** |
| 2.13.0 | **Behind a proxy, easywall knows who you are.** |
| 2.12.0 | **The configuration comes from outside, and the page says so.** |
| 2.11.0 | **A rule names a service and who may reach it.** |
| 2.10.0 | **What changes is on the screen.** |
| 2.9.0 | **The interface speaks French, and both binaries read their environment.** |
| 2.8.0 | **A stolen password alone no longer opens the firewall.** |
| 2.7.0 | **The firewall survives a reboot.** |
| 2.6.0 | **Proof instead of rule counts, and a documentation site of its own.** |
| 2.5.1 | **The documented Debian install command was a 404.** |
| 2.5.0 | **Every switch on the options page reaches the firewall.** |
| 2.4.2 | **The interface stops drawing boxes inside boxes.** |
| 2.4.1 | **The documentation site has a dark mode.** |
| 2.4.0 | **A public demo, running the whole interface against nothing.** |
| 2.3.0 | **The interface is built from a component library instead of by hand.** |
| 2.2.0 | **The audit log is readable from the interface.** |
| 2.1.0 | **The protection modules are editable from the interface.** |
| 2.0.0 | **easywall is Go, end to end.** |
| 0.3.1 | **A shell flag that broke the installer on older systems.** |
| 0.3.0 | **A port can say what it is for.** |
| 0.2.4 | **The demo page's security headers are checked.** |
| 0.2.3 | **The installation works.** |
| 0.2.2 | **The readme explains the thing it is the readme for.** |
| 0.2.1 | **easywall installs from a Debian package.** |
| 0.2.0 | **The project can be sponsored.** |
| 0.1.0 | **Almost every line is covered by a unit test.** |
| 0.0.4 | **Custom iptables rules can be applied.** |
| 0.0.3 | **A web interface, on Flask.** |
| 0.0.2 | **The Python rewrite takes over from master.** |
| 0.0.1 | **Two parts: one running as root, one not.** |

- [ ] **Step 4: Generate the page**

Run: `npm run build:changelog`

Expected: `wrote docs/_docs/changelog.md — 30 versions`.

- [ ] **Step 5: Keep the collapsed sections out of the on-page contents**

Each `<details>` body carries `### Added` / `### Changed` / `### Fixed`, and the
contents builder collects every `h2[id], h3[id]` in `.content-body`. Left alone
it would build a list of ninety entries reading *Added, Changed, Fixed, Added,
Changed…* — and every one of them pointing inside a collapsed disclosure.

In `docs/_layouts/default.html`, replace:

```js
      var heads = [].slice.call(body.querySelectorAll('h2[id], h3[id]'));
```

with:

```js
      // A heading inside a collapsed <details> is not on the page as far as the
      // reader is concerned, and an entry that scrolls to something invisible is
      // worse than no entry. The changelog is thirty disclosures deep.
      var heads = [].slice.call(body.querySelectorAll('h2[id], h3[id]'))
                    .filter(function (h) { return !h.closest('details'); });
```

The changelog page then has no qualifying headings, falls under the existing
`heads.length < 3` branch, and correctly renders full width with no contents
column.

- [ ] **Step 6: Style the disclosures**

`.nav-group-label` styles the sidebar's `<details>`; nothing styles one in the
content body. In `web/src/docs.css`, after the blockquote rules, add:

```css
/* ── Disclosures in the body ──────────────────────────────────────────
   The changelog is thirty of these. Only the sidebar's groups were styled, so
   in the content body a <summary> arrived with the browser's default marker
   and no hit area worth the name. */
.content-body details {
  margin: 0 0 0.55rem;
  border: 1px solid var(--border-muted);
  border-radius: 6px;
  background: var(--surface-1);
}

.content-body summary {
  padding: 0.65rem 0.9rem;
  font-size: 0.95rem;
  color: var(--text);
  cursor: pointer;
  list-style: none;
}

.content-body summary::-webkit-details-marker { display: none; }

/* The disclosure triangle, drawn rather than inherited, so it turns with the
   state and matches the sidebar's. */
.content-body summary::before {
  content: "▸";
  display: inline-block;
  width: 1em;
  color: var(--text-subtle);
  transition: transform 120ms ease;
}

.content-body details[open] > summary::before { transform: rotate(90deg); }
.content-body summary:hover { background: var(--surface-2); }
.content-body details[open] > summary { border-bottom: 1px solid var(--border-muted); }
.content-body details > :not(summary) { padding: 0 0.9rem; }
.content-body details > :not(summary):first-of-type { padding-top: 0.8rem; }
.content-body details > :last-child { padding-bottom: 0.8rem; }
```

- [ ] **Step 7: Add the nav entry**

In `docs/_config.yml`, in the `Project` group's `children`, after `Contributing`:

```yaml
      - title: Changelog
        path: /docs/changelog/
```

`TestEveryDocsPageIsInTheNav` derives both sides and needs no edit — but it does
now have a page and an entry to reconcile, so run it.

- [ ] **Step 8: Rebuild everything and check it**

Run:

```bash
npm run build:docs-css
grep -o '\.content-body summary::-webkit-details-marker{display:none}' docs/assets/css/style.css
grep -c 'content-body details' docs/assets/css/style.css        # >= 5
npm run check:changelog
go test ./internal/shared/ -run 'TestEveryDocsPageIsInTheNav|TestTheOldDemoHostIsNotPublished'
npm run build:docs && npm run check:docs
```

Expected: the greps match, `check:changelog` says *is current — 30 versions*,
both Go tests pass — `TestTheOldDemoHostIsNotPublished` in particular, which is
where its `docs/_docs/changelog.md` exemption starts earning its place — and the
browser checks still pass with a 30th page in the index.

- [ ] **Step 9: Verify the check by breaking the source**

Change one word in a `CHANGELOG.md` headline **without** regenerating, run
`npm run check:changelog`: FAIL with *not what CHANGELOG.md would produce*. Run
`npm run build:changelog`, re-run: pass. Revert the word and regenerate.

- [ ] **Step 10: Render it**

At **1600 / 900 / 390**, both themes, on `/docs/changelog/`:

- the newest version is open and the other 29 are one line each;
- the body inside a disclosure is **markdown** — headings, bullets and inline code, not literal asterisks. If it is literal, `markdown="1"` did not survive; that is the kramdown raw-HTML rule and not a CSS problem;
- the page is full width with no contents column and no 340px empty gutter;
- a summary line wraps rather than clipping at 390.

- [ ] **Step 11: Commit**

```bash
git add scripts/render-changelog.mjs package.json CHANGELOG.md docs/_docs/changelog.md \
        docs/_config.yml docs/_layouts/default.html web/src/docs.css docs/assets/css/style.css
git commit -m "feat(docs): the changelog is a page, generated from the file that is the changelog"
```

---

### Task 10: What GPL-3.0 means to somebody running easywall

The footer prints `GPL-3.0` as plain text at `default.html:179` and links
nothing. An Explanation page — what the licence permits, and which obligations
begin only on distribution. It links the full text; it does not inline 35 KB.

**Files:**
- Create: `docs/_docs/license.md`
- Modify: `docs/_config.yml` (nav), `docs/_layouts/default.html:179`

**Interfaces:**
- Consumes: `site.demo_url` is not needed here. Nothing.
- Produces: `/docs/license/`, linked from the footer of every page.

- [ ] **Step 1: Write the page**

`docs/_docs/license.md`:

```markdown
---
layout: default
title: License
description: GPL-3.0, and what it asks of you — which is nothing at all until you hand easywall to somebody else.
---

# License

easywall is under the **GNU General Public License, version 3**. The full text
is [in the repository](https://github.com/jp1337/easywall/blob/main/LICENSE) and
at [gnu.org](https://www.gnu.org/licenses/gpl-3.0.html). This page is what it
means in practice; it is not legal advice, and where the two disagree the
licence is what counts.

## Running it

| | |
|---|---|
| On your own machine | Yes. No conditions at all |
| At work, on company hardware | Yes. Commercial use is use |
| On a server other people reach | Yes. GPL-3.0 is not the AGPL — serving something over a network is not distribution |
| Modified, for yourself | Yes. A change you keep is yours and nobody has to see it |
| Paying nothing, telling nobody | Yes |

Nothing on that list creates an obligation. That is the whole of it for almost
everybody who reads this page.

## Handing it to somebody else

Obligations begin at **distribution** — a binary, a package, a container image,
a modified source tree, anything that leaves your hands.

| You give them | You also owe them |
|---|---|
| An unmodified binary or package | The corresponding source, and this licence with it |
| A build with your changes in it | The same, including your changes, under GPL-3.0 |
| A container image with easywall in it | The same. An image is a distribution of what is inside it |

And in every case the copyright notices and the licence text stay where they
are. You may charge for the distribution; you may not charge for the source, or
withhold it.

> **A network service is not distribution.** Running a modified easywall for
> other people to use over HTTP does not oblige you to publish anything. That
> obligation is the AGPL's, and easywall is not under it.

## Contributing

A pull request is contributed under GPL-3.0, the same as the rest. There is no
contributor licence agreement and nothing to sign — see
[Contributing]({{ '/docs/contributing/' | relative_url }}).

## Why this licence

easywall puts a web interface in front of a machine's packet filter. Anyone
running it is trusting the code with the one thing standing between their host
and the network, and a licence that lets a modified copy be handed on with the
modification hidden is the wrong licence for that. GPL-3.0 means the version
somebody hands you is a version you can read.
```

- [ ] **Step 2: Add the nav entry**

In `docs/_config.yml`, in `Project`, after `Changelog`:

```yaml
      - title: License
        path: /docs/license/
```

- [ ] **Step 3: Link it from the footer**

In `docs/_layouts/default.html:179`, replace `GPL-3.0` with a link:

```html
        <p>easywall — <em>Your firewall. Your rules. No surprises.</em> · <a href="{{ '/docs/license/' | relative_url }}">GPL-3.0</a> · <a href="https://github.com/jp1337/easywall" target="_blank" rel="noopener">GitHub</a> · <a href="{{ site.discord }}" target="_blank" rel="noopener">Discord</a></p>
```

- [ ] **Step 4: Check it**

Run:

```bash
go test ./internal/shared/ -run TestEveryDocsPageIsInTheNav
npm run build:docs
```

Expected: the test passes (it derives both sides, so a page without its nav
entry fails here and nowhere else), and Jekyll builds 31 documentation pages.

- [ ] **Step 5: Render it**

At **1600 / 900 / 390**, both themes: `/docs/license/` — three tables, one
callout, and a contents column with four entries. And the footer on any page,
where the link must not run into the `·` beside it.

- [ ] **Step 6: Commit**

```bash
git add docs/_docs/license.md docs/_config.yml docs/_layouts/default.html
git commit -m "docs(site): the footer's GPL-3.0 leads to a page that says what it means"
```

---

### Task 11: Putting easywall behind a reverse proxy is one page

The task — put easywall behind nginx — is currently spread across three page
types answering it in three registers: `configuration.md:241` (Reference),
`security.md:83` (Explanation) and a table row in `environment.md:63`. Three
page types answering one task is why it reads as duplicated.

Both existing sections shrink to a pointer. **The key itself stays documented in
`configuration.md`**, in the table beside every other key — a reference table
with a hole in it is a worse reference.

**Files:**
- Create: `docs/_docs/installation/reverse-proxy.md`
- Modify: `docs/_config.yml` (nav), `docs/_docs/configuration.md:236–265`, `docs/_docs/security.md:83–98`

**Interfaces:**
- Consumes: nothing.
- Produces: `/docs/installation/reverse-proxy/`, linked from `configuration.md`,
  `security.md` and `environment.md`.

- [ ] **Step 1: Write the page**

`docs/_docs/installation/reverse-proxy.md`:

````markdown
---
layout: default
title: Behind a Reverse Proxy
description: Put easywall behind nginx and have it still know which client is talking to it.
---

# Behind a Reverse Proxy

easywall terminates TLS itself, so a proxy in front of it is a choice rather
than a requirement. Make that choice and one thing breaks quietly: every request
now arrives from the proxy, so the audit log records the proxy's address, the
apply screen cannot tell whether *you* are about to be locked out, and the login
limiter's five attempts per ten minutes are shared by everybody behind it.

Listing the proxy in `trusted_proxies` fixes all three. Getting the list wrong
hands address spoofing to anything that can reach the port.

## 1 · Point nginx at easywall

```nginx
server {
    listen 443 ssl;
    server_name firewall.example.com;

    ssl_certificate     /etc/letsencrypt/live/firewall.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/firewall.example.com/privkey.pem;

    location / {
        # easywall serves HTTPS with its own certificate, generated on first
        # start. It is not in your chain and nginx has no reason to verify it.
        proxy_pass https://127.0.0.1:12227;
        proxy_ssl_verify off;

        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 2 · Tell easywall which peer to believe

```toml
trusted_proxies = ["127.0.0.1"]
```

or `EASYWALL_WEB_TRUSTED_PROXIES=127.0.0.1`.

The value is the address **the proxy's connection arrives from**, as easywall
sees it — not the client's, and not the network the proxy is on.

## 3 · Check that it took

Sign in and open the [audit log]({{ '/docs/features/audit-log/' | relative_url }}).
The entry for that sign-in carries your own address, not `127.0.0.1`. If it says
`127.0.0.1`, the header is not being believed: the peer is not on the list, or
nginx is not sending `X-Forwarded-For`.

## In Docker

`trusted_proxies` names the **proxy container's own address**, and the default
bridge is not the one a compose project uses. Ask Docker rather than guessing:

```bash
docker network inspect <network> --format '{% raw %}{{range .Containers}}{{.Name}} {{.IPv4Address}}{{"\n"}}{{end}}{% endraw %}'
```

The default bridge is `172.17.0.0/16`; a compose project gets a network of its
own, typically `172.18.0.0/16` upward. Take the proxy's address out of that
output — a single address, with no mask.

> **A container's address is not stable across a recreate.** Give the proxy a
> fixed address on a user-defined network, or the list stops matching the next
> time the stack comes up and easywall quietly goes back to believing nobody.

## What it costs

Being on this list is total trust in that peer. Two mistakes hand address
spoofing to anyone who can reach the port:

- listing an address that is **not** actually a proxy in front of easywall;
- listing a **network** rather than the proxies themselves — every host in
  `10.0.0.0/8` can then choose the address easywall records, decides lockouts
  on, and rate limits.

List the proxies. Not the subnet they live in, not `0.0.0.0/0`, and never an
address you do not control. This is why the setting is a *list* and not a
boolean: "trust the header" with no way to say whose is
GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp and GHSA-9g5q-2w5x-hmxf, and no
configuration of easywall can express it.

`X-Real-IP` and `True-Client-IP` are never believed, from any peer.

## When it does not work

| Symptom | Cause |
|---|---|
| The audit log records the proxy | The peer is not on `trusted_proxies`, or nginx sends no `X-Forwarded-For` |
| easywall refuses to start | An entry that is neither an address nor a CIDR network. The message names it |
| The lockout warning on Apply is wrong | Same cause: the verdict is computed for the address easywall resolved |
| It worked, then stopped | A container was recreated and took a new address. Fix the address, not the list |

**Next:** [Configuration]({{ '/docs/configuration/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }})
````

- [ ] **Step 2: Add the nav entry**

In `docs/_config.yml`, in `Installation`, after `First Run`:

```yaml
      - title: Behind a Reverse Proxy
        path: /docs/installation/reverse-proxy/
```

- [ ] **Step 3: Shrink the Reference section**

In `docs/_docs/configuration.md`, replace everything from `### Behind a reverse
proxy` down to `address you do not control.` with:

```markdown
### Behind a reverse proxy

```toml
trusted_proxies = ["127.0.0.1", "10.1.0.5"]
```

Each entry is an address or a CIDR network whose `X-Forwarded-For` easywall
believes. Empty by default, which means the TCP peer is authoritative.

**Being on this list is total trust in that peer.** List the proxies
themselves, never the network they live in: every host in `10.0.0.0/8` can then
choose the address easywall records, decides lockouts on, and rate limits.

The whole task — nginx in front, the value to use, the Docker case, and how to
check it took — is [Behind a reverse proxy]({{ '/docs/installation/reverse-proxy/' | relative_url }}).
```

Leave the `trusted_proxies` row in the key table exactly as it is, including its
`#behind-a-reverse-proxy` anchor, which still resolves.

- [ ] **Step 4: Shrink the Explanation section**

In `docs/_docs/security.md`, keep the first paragraph and the table, and replace
the table's third row so it points at the how-to:

```markdown
| What to do about it | list the proxy in `trusted_proxies` — [Behind a reverse proxy]({{ '/docs/installation/reverse-proxy/' | relative_url }}) is the whole task, with nginx and Docker |
```

Everything else in that section stays: it explains *why* the header is not
believed, which is what the page is for.

- [ ] **Step 5: Point the environment table at it**

In `docs/_docs/environment.md`, after the paragraph beginning `A **list**
variable is comma-separated`, add:

```markdown
Setting up the proxy itself is
[Behind a reverse proxy]({{ '/docs/installation/reverse-proxy/' | relative_url }}).
```

- [ ] **Step 6: Check it**

Run:

```bash
go test ./internal/shared/ -run 'TestEveryDocsPageIsInTheNav|TestEveryConfigKeyIsDocumented'
npm run build:docs
```

Expected: both pass. `TestEveryConfigKeyIsDocumented` is the one that matters
here — the `trusted_proxies` row must not have been carried off with the
section around it.

- [ ] **Step 7: Render it**

At **1600 / 900 / 390**, both themes: the new page, and both shortened sections.
Confirm the nginx block and the `docker network inspect` line do not force a
horizontal scroll on the page body at 390 — a code block scrolls inside itself,
the page does not.

Confirm the `{% raw %}` around the Go template braces in the `docker network
inspect` command survived: if Liquid ate them, the command renders with the
braces missing and is silently wrong.

- [ ] **Step 8: Commit**

```bash
git add docs/_docs/installation/reverse-proxy.md docs/_config.yml \
        docs/_docs/configuration.md docs/_docs/security.md docs/_docs/environment.md
git commit -m "docs(site): putting easywall behind a proxy is one page instead of three halves"
```

---

### Task 12: Blacklist and whitelist are two pages

One page describing two lists that do opposite things means a reader looking for
one reads past the other. The split happens **under the current names** — the
Blocklist/Allowlist rename is 961 occurrences across 48 files in `internal/`
alone and is its own release. `jekyll-redirect-from` is already installed, so
when that release moves the URLs the documentation side costs two lines of front
matter.

Neither page restates the packet order. Both point at the `rule-order` diagram
in `architecture.md`, which is already canonical.

**Files:**
- Create: `docs/_docs/features/whitelist.md`
- Modify: `docs/_docs/features/blacklist.md`, `docs/_config.yml` (nav), `internal/shared/docs_coverage_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `/docs/features/whitelist/`. `TestEveryPageIsDocumented`'s
  `documented` map maps `/whitelist` to it.

- [ ] **Step 1: Change the test first**

In `internal/shared/docs_coverage_test.go`, in the `documented` map:

```go
		"/whitelist":    "_docs/features/whitelist.md",
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/shared/ -run TestEveryPageIsDocumented -v`

Expected: FAIL — `/whitelist is documented in docs/_docs/features/whitelist.md,
which does not exist`.

- [ ] **Step 3: Write the whitelist page**

`docs/_docs/features/whitelist.md`:

```markdown
---
layout: default
title: Whitelist
description: Addresses that reach every port, whether it is open or not — and the way back into your own machine.
---

# Whitelist

A list of addresses that are accepted. Not accepted *for the ports you opened* —
accepted for **every port**, open or not. That is what makes it the way back
into a machine whose rules you are about to change, and it is what makes a wide
entry expensive.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/blacklist" ext="png"
     alt="The address list editor: a textarea of addresses with a live entry count, per-line validation, and context cards explaining what the list does and that the order matters." %}
  <figcaption>Every line is validated as you type, and the line number is named when one does not parse.</figcaption>
</figure>

> **A whitelisted source reaches services you never opened.** It does not pass
> the port rules, it skips them. Prefer a single address over a range, and a
> range over a whole network.

## Where it sits

The whitelist is consulted **after** the [blacklist]({{ '/docs/features/blacklist/' | relative_url }})
and **before** the port rules. An address on both lists is dropped. The full
packet order, for all of it at once, is the `rule-order` diagram in
[Architecture]({{ '/docs/architecture/' | relative_url }}).

## Your way back in

Put the address you administer the host from on this list **before** you start
changing port rules.

| | |
|---|---|
| What it survives | a closed SSH port, and every port rule you change |
| What it does **not** survive | the protection modules — they run before the whitelist, so a packet a module drops never reaches it |
| Why that rarely bites | the rate limits are counted per source address, so somebody else's flood cannot spend your budget |
| The one exception | the [bogon filter]({{ '/docs/features/filters/' | relative_url }}) reads this list. It drops private source addresses, so without that it would drop you for administering the host from one — and the entry meant to prevent exactly that could never be reached |

Together with the [acceptance window]({{ '/docs/features/apply/' | relative_url }})
that is two independent ways not to lose access to your own machine.

## Accepted input

One entry per line. Lines starting with `#` are comments; blank lines are
ignored. The counter under the editor counts real entries — comments and blanks
do not inflate it.

| Form | Example |
|---|---|
| IPv4 address | `192.0.2.42` |
| IPv4 network | `198.51.100.0/24` |
| IPv6 address | `2001:db8::1` |
| IPv6 network | `2001:db8::/32` |

## When it does not work

| Symptom | Cause |
|---|---|
| A whitelisted address is still blocked | It is on the blacklist too — that is checked first. Or a protection module dropped it, which happens before the whitelist. The bogon filter is the exception: it honours this list |
| Nothing changed after saving | Saving stages. It goes live on [Apply]({{ '/docs/features/apply/' | relative_url }}) |
| The editor names a line number | That line is not a valid address or CIDR; the message says why |
| You allowed too broad a range | Remove it, save, apply. If it already locked you out, do nothing — the window rolls it back |
```

- [ ] **Step 4: Cut the blacklist page down to the blacklist**

In `docs/_docs/features/blacklist.md`:

- front matter `title:` becomes `Blacklist`, and `description:` becomes
  `A list of addresses that are dropped before anything else is considered.`
- the `# Blacklist & Whitelist` heading becomes `# Blacklist`
- the opening two lines become:

  ```markdown
  A list of addresses that are dropped. It is consulted before every other rule
  that can accept a packet, which is the only thing about it you really need to
  remember.
  ```

- the `## The order` section keeps the `rule-order` include and the **The
  blacklist wins** paragraph and the *Fixed in 2.11* callout, and gains one line
  pointing at the other page:

  ```markdown
  The list consulted immediately after this one is the
  [whitelist]({{ '/docs/features/whitelist/' | relative_url }}), which accepts.
  ```

- the two-column `## What each list does` table loses its Whitelist column and
  becomes:

  ```markdown
  ## What it does

  | | |
  |---|---|
  | Effect | DROP |
  | Evaluated | before the whitelist, and before the port rules |
  | Skips the protection modules | no — those run first |
  | Use it for | a scanner, an abusive network |
  ```

- `## Accepted input` stays verbatim — the two pages take the same input and a
  reader on either one needs it. This is the one deliberate duplication in the
  split; a pointer would cost a page load to learn what a comment looks like.
- the whole `## Your way back in` section **moves** to `whitelist.md` and is
  deleted here
- in `## When it does not work`, the whitelist rows move; the blacklist keeps:

  ```markdown
  | Symptom | Cause |
  |---|---|
  | A blacklisted address still gets through | The connection was already established; the list only affects new ones |
  | An address you also whitelisted is still blocked | The blacklist is checked first, and it wins. Take the entry off the blacklist |
  | Nothing changed after saving | Saving stages. It goes live on [Apply]({{ '/docs/features/apply/' | relative_url }}) |
  | The editor names a line number | That line is not a valid address or CIDR; the message says why |
  ```

- the `whois` snippet and the logging line at the end stay on `blacklist.md`:
  both are about blocking.

- [ ] **Step 5: Update the nav**

In `docs/_config.yml`, in `Rules`, replace the single entry

```yaml
      - title: Blacklist & Whitelist
        path: /docs/features/blacklist/
```

with two, in evaluation order — the order the firewall consults them is the
order a reader should meet them:

```yaml
      - title: Blacklist
        path: /docs/features/blacklist/
      - title: Whitelist
        path: /docs/features/whitelist/
```

- [ ] **Step 6: Run the tests to see them pass**

Run:

```bash
go test ./internal/shared/ -run 'TestEveryPageIsDocumented|TestEveryDocsPageIsInTheNav' -v
npm run build:docs
```

Expected: both pass; Jekyll builds 32 documentation pages.

- [ ] **Step 7: Verify the test by breaking the code**

Rename `docs/_docs/features/whitelist.md` to `.md.bak`, re-run
`TestEveryPageIsDocumented`: FAIL. Rename it back.

- [ ] **Step 8: Check the incoming links**

Run:

```bash
grep -rn "features/blacklist" docs/ README.md --include='*.md' --include='*.html' | grep -v _site
```

Every hit is read and pointed at whichever page now answers it. A link about
*allowing* an address that still lands on the blacklist page is the split having
made things worse.

- [ ] **Step 9: Render it**

At **1600 / 900 / 390**, both themes: both pages, and the two new sidebar
entries under *Rules* — the group is open on both, and the current one is marked.

- [ ] **Step 10: Commit**

```bash
git add docs/_docs/features/whitelist.md docs/_docs/features/blacklist.md \
        docs/_config.yml internal/shared/docs_coverage_test.go
git commit -m "docs(site): the list that drops and the list that accepts are two pages"
```

---

### Task 13: The roadmap stops being a changelog

**Correction to the spec:** it says "the five *Done in 2.x* sections (~100
lines)". There are **seven** — 2.11, 2.10, 2.9, 2.8, 2.7, 2.6 and 2.5,
`roadmap.md:79–166`, 88 lines. All seven go. Their headlines are not lost: nine
of them became changelog summary lines in Task 9, which is why that task comes
first.

Also in this task: the demo page's per-command table is for whoever maintains
the mock, not for whoever is running it.

**Files:**
- Modify: `docs/_docs/roadmap.md`, `docs/_docs/installation/demo.md`, `docs-tech/` (one page gains a section)

**Interfaces:**
- Consumes: `/docs/changelog/` from Task 9.
- Produces: nothing.

- [ ] **Step 1: Delete the seven Done sections**

In `docs/_docs/roadmap.md`, delete from `## Done in 2.11` down to the line before
the final `---` separator — seven sections, `:79–166`. Replace them with:

```markdown
## What already shipped

Twelve releases of it, newest first, each with what it was for:
[Changelog]({{ '/docs/changelog/' | relative_url }}).
```

- [ ] **Step 2: Drop the shipped rows from the future table**

Delete the `**2.12**` and `**2.13**` rows from the version table — both shipped
on 2026-08-28 and a roadmap listing them is the failure the page's own opening
paragraph names.

- [ ] **Step 3: Fix the two references that now dangle**

The opening paragraph says *see* Done in 2.7 *and* Done in 2.8 *below*, and both
are gone. Replace that sentence:

```markdown
**Ordering principle: by exposure.** The two holes an attacker could actually
walk through came first and shipped in 2.7 and 2.8 — see the
[Changelog]({{ '/docs/changelog/' | relative_url }}). What is left helps you
understand what you are doing, then lets you maintain it, then reaches further:
```

In the code block below it, delete the `2.12` and `2.13` lines so the block
matches the table.

Keep both `> **Amended in …**` callouts. They record decisions about the
*future* rows and are still true.

- [ ] **Step 4: Move the mock's command table**

In `docs/_docs/installation/demo.md`, delete the `## What the mock does` table
— the seven-row `Command | In demo` one. The callout beneath it **stays**,
rewritten so it stands without the table above it:

```markdown
> **Custom-rule syntax cannot be checked in demo mode.** There is no `nft`
> binary, so the page reports live validation as unavailable rather than giving
> a verdict it has no basis for. It used to answer "no errors" whatever was
> typed — a false green on the one page where being wrong locks you out.
> Address-list validation runs in the web process and works normally.
```

Add the deleted table to `docs-tech/` under a `## What the demo mock answers`
heading, in whichever page already describes the mock; if none does, it belongs
in `docs-tech/protocol.md`, beside the fifteen commands it answers.

- [ ] **Step 5: Check it**

Run:

```bash
go test ./internal/shared/
npm run build:docs && npm run check:docs
grep -n "Done in 2" docs/_docs/roadmap.md    # no output
```

Expected: everything passes and the grep prints nothing.
`TestTheTechnicalDocsAreNotPublished` is the one to watch — a table moved into
`docs-tech/` must not have arrived under `docs/`.

- [ ] **Step 6: Render it**

At **1600 / 900 / 390**, both themes: `/docs/roadmap/` — the contents column
lost seven entries and must not have dropped below the three-heading threshold
that removes it entirely; and `/docs/installation/demo/`, where the callout now
follows the *Running your own* section directly.

- [ ] **Step 7: Commit**

```bash
git add docs/_docs/roadmap.md docs/_docs/installation/demo.md docs-tech/
git commit -m "docs(site): the roadmap is what is planned, and the changelog is what happened"
```

---

## Phase 3 — prose

12,781 prose words. The corpus is smaller than `_docs/`'s 28,698 total suggests,
because most of it is already tables and code.

**The risk, named.** This is the one part of the branch where the tests cannot
tell you it went wrong. A flattened sentence that still parses and still sounds
true is invisible to CI. Sentence length is mechanically checkable and Task 14
makes it so; **the claim is not**, and the entire mitigation is
`docs-tech/i18n-review.md` plus a code re-read on every sentence that is
rewritten. Task 15 exists because that mitigation, applied once during planning,
already found one.

### Task 14: One definition of a sentence

The spec says 42 prose sentences run over 40 words. That number is not
reproducible: a tokenizer that strips front matter, fenced code, table rows,
raw-HTML lines, Liquid lines and inline code counts **63 over 40 and 155 over
30**, at a corpus average of 22.7. Neither number is wrong — they measure
different things, and that is the problem. This task commits the tokenizer so
there is one.

**Files:**
- Create: `scripts/prose-check.mjs`
- Modify: `package.json`

**Interfaces:**
- Consumes: nothing.
- Produces: `npm run check:prose`, and `node scripts/prose-check.mjs <path...>`
  for one page or one subtree. Tasks 16–22 each run it over their batch, before
  and after. Task 24 wires it into CI, once it passes.

- [ ] **Step 1: Write the script**

`scripts/prose-check.mjs`:

```js
#!/usr/bin/env node
/**
 * Measures the prose in docs/_docs, so that "sentences over 30 words" means the
 * same thing to two people on two days. The spec that produced this branch and
 * the plan that implements it counted 42 and 63 with the same intention and
 * different filters; this file is the tiebreak, not the authority on style.
 *
 *   npm run check:prose                                every page; exits 1 on a breach
 *   node scripts/prose-check.mjs docs/_docs/features   one subtree, listed
 *
 * What is deliberately NOT prose: front matter, fenced code, table rows, lines
 * of raw HTML, Liquid lines, and inline code — which counts as one word. Those
 * carry the numbers, keys, paths and flags the rewrite preserves byte for byte,
 * and measuring them reports a nine-row limits table as one enormous sentence.
 *
 * The two rules are the spec's: no prose sentence over 30 words, average under
 * 18. Neither can see whether a rewritten sentence still claims what the code
 * does. Nothing can; that is what docs-tech/i18n-review.md is for.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const MAX = 30;
const AVG = 18;
const ROOT = new URL('..', import.meta.url).pathname;

function walk(p, out = []) {
  if (statSync(p).isDirectory()) {
    for (const e of readdirSync(p).sort()) walk(join(p, e), out);
  } else if (p.endsWith('.md')) {
    out.push(p);
  }
  return out;
}

// Paragraphs, with the line each one starts on — a rewrite needs somewhere to
// go, and a sentence has no line of its own once it wraps.
function paragraphs(md) {
  const lines = md.split('\n');
  const out = [];
  let buf = [], start = 0, fenced = false, front = false;

  const flush = () => {
    if (buf.length) out.push({ line: start, text: buf.join(' ') });
    buf = [];
  };

  lines.forEach((raw, i) => {
    const line = raw.trim();

    if (i === 0 && line === '---') { front = true; return; }
    if (front) { if (line === '---') front = false; return; }
    if (/^```/.test(line)) { fenced = !fenced; flush(); return; }
    if (fenced) return;

    // A table row, a raw HTML line, a Liquid line, a heading, a horizontal
    // rule: structure, not prose.
    if (!line || /^\|/.test(line) || /^</.test(line) || /^\{%/.test(line) ||
        /^#{1,6}\s/.test(line) || /^-{3,}$/.test(line)) {
      flush();
      return;
    }

    // A list item is prose and is measured, but each item is its own unit —
    // three bullets are not one sixty-word sentence.
    if (/^[-*+]\s|^\d+\.\s/.test(line)) flush();

    if (!buf.length) start = i + 1;
    buf.push(line);
  });

  flush();
  return out;
}

// Inline code is one word whatever it holds: `EASYWALL_WEB_TRUSTED_PROXIES` is
// one thing a reader takes in, not three, and a rule that punished naming a key
// would be a rule against being specific.
const clean = t => t
  .replace(/`[^`]*`/g, 'CODE')
  .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
  .replace(/\{\{[^}]*\}\}/g, 'LINK')
  .replace(/^[-*+]\s+|^\d+\.\s+/, '');

// An abbreviation does not end a sentence. `e.g.` inside a parenthesis is the
// one that would otherwise split every list of examples in two. The trailing
// period is dropped rather than restored: this string is only ever counted and
// printed.
const split = t => t
  .replace(/\b(e\.g|i\.e|etc|vs|Dr|Mr|No)\./gi, '$1')
  .split(/(?<=[.!?])["')\]]*\s+(?=[A-Z(`"'—])/)
  .map(s => s.trim())
  .filter(Boolean);

const targets = process.argv.slice(2).filter(a => !a.startsWith('--'));
const files = (targets.length ? targets : [join(ROOT, 'docs/_docs')])
  .flatMap(t => walk(t));

let allWords = [], breaches = 0;

for (const file of files) {
  const rows = [];
  for (const p of paragraphs(readFileSync(file, 'utf8'))) {
    for (const s of split(clean(p.text))) {
      const n = s.split(/\s+/).filter(Boolean).length;
      allWords.push(n);
      if (n > MAX) rows.push({ line: p.line, n, s });
    }
  }
  if (!rows.length) continue;
  breaches += rows.length;
  console.log(`\n${relative(ROOT, file)}`);
  for (const r of rows) {
    console.log(`  :${r.line}  ${r.n} words  ${r.s.slice(0, 96)}${r.s.length > 96 ? '...' : ''}`);
  }
}

const avg = allWords.length
  ? allWords.reduce((a, b) => a + b, 0) / allWords.length : 0;

console.log(`\n${files.length} pages · ${allWords.length} prose sentences · ` +
            `average ${avg.toFixed(1)} words · ${breaches} over ${MAX}`);

if (breaches || avg >= AVG) {
  if (avg >= AVG) console.error(`average is ${avg.toFixed(1)}, want under ${AVG}`);
  process.exit(1);
}
```

Add to `package.json` `"scripts"`:

```json
    "check:prose": "node scripts/prose-check.mjs",
```

- [ ] **Step 2: Run it and record the baseline**

Run: `npm run check:prose`

Expected: exits 1. Save the summary line — pages, sentences, average, breaches —
into the commit message. It is the number every later batch is measured against,
and it is the first time this repository has had one.

- [ ] **Step 3: Verify it by breaking a page**

Append a deliberately 45-word sentence to `docs/_docs/features/export-import.md`
— today the only page with nothing over 30 — re-run, and confirm the page
appears with the right line number and word count. Remove it.

- [ ] **Step 4: Verify the exclusions are actually excluding**

Run `node scripts/prose-check.mjs docs/_docs/configuration.md` and read the
output. No reported sentence may be a table row, a code line or front matter. If
one is, the filter is wrong and every number after this is wrong with it.

- [ ] **Step 5: Commit**

```bash
git add scripts/prose-check.mjs package.json
git commit -m "test(docs): a prose sentence has one definition and one length"
```

---

### Task 15: The whitelist does not bypass the protection modules

**Found while planning this phase, by doing the thing §6 of the spec calls the
mitigation — reading `docs-tech/i18n-review.md` against the code. It is outside
the spec's stated scope, and it is a wrong claim about the firewall in three
languages, which is exactly the class of defect this whole phase is arranged
around. It is fixed here rather than filed.**

Two strings say a whitelist entry is exempt from the protection modules. It is
not. `internal/core/nftables.go:1136–1144` and `:828–847`: the modules run
**first**, and only the bogon filter carries exceptions for whitelisted
addresses — added because that module's premise is "nothing legitimately has
this source address", and an operator who whitelists `192.168.1.0/24` has said
otherwise about part of it. Every other module still sees the packet before the
whitelist does.

`docs/_docs/features/blacklist.md` has this right today, and Task 12 carried it
into `whitelist.md` unchanged. The interface has it wrong — and wrong in the
dangerous direction, because it promises that the way back into your own machine
is stronger than it is.

**Files:**
- Modify: `locales/en.json:305,310`, `locales/de.json:305,310`, `locales/fr.json:294,299`, `docs-tech/i18n-review.md`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing. It is a correction.

- [ ] **Step 1: Confirm it against the code. Do not take this plan's word**

Read `internal/core/nftables.go:828–847` — the bogon filter's exemption list and
the paragraph explaining why the module was deliberately *not* moved after the
whitelist — then `:1136–1144`, where the SSH chain returns to "blacklist, then
whitelist, then the port rules". Then `internal/core/nftables_bogon_test.go:53`,
which asserts the bogon exception and nothing broader.

Expected conclusion: the protection modules run before the whitelist; the bogon
filter is the single exception.

- [ ] **Step 2: Correct the two strings in all three locales**

`locales/en.json`:

```json
  {"id": "whitelist_section_desc", "translation": "Accepted before the port rules are consulted. The protection modules still run first — only the bogon filter makes an exception for this list."},
```

```json
  {"id": "whitelist_narrow_body", "translation": "An entry here reaches every port, open or not. Prefer a single address over a range, and a range over a whole network."},
```

`locales/de.json`:

```json
  {"id": "whitelist_section_desc", "translation": "Werden angenommen, bevor die Port-Regeln geprüft werden. Die Schutzmodule laufen trotzdem vorher — nur der Bogon-Filter nimmt diese Liste aus."},
```

```json
  {"id": "whitelist_narrow_body", "translation": "Ein Eintrag hier erreicht jeden Port, auch nicht geöffnete. Eine einzelne Adresse ist einem Bereich vorzuziehen, ein Bereich einem ganzen Netz."},
```

`locales/fr.json`:

```json
  {"id": "whitelist_section_desc", "translation": "Acceptées avant que les règles de ports ne soient consultées. Les modules de protection s'exécutent tout de même en premier — seul le filtre bogon fait une exception pour cette liste."},
```

```json
  {"id": "whitelist_narrow_body", "translation": "Une entrée ici atteint tous les ports, même ceux qui ne sont pas ouverts. Préférez une adresse seule à une plage, et une plage à un réseau entier."},
```

`whitelist_subtitle` and `whitelist_wayback_note` are correct as they stand —
both are about the *port rules*, which the whitelist genuinely does skip. Leave
them.

- [ ] **Step 3: Correct the review page**

In `docs-tech/i18n-review.md`, under `## Blacklist and whitelist semantics`,
update the two rows to the new English, and add a line saying what was wrong.
The page's job is to hold the sentences a translator must not get backwards, and
one that was already backwards in English is the strongest argument it has for
existing.

- [ ] **Step 4: Check it**

Run:

```bash
go test ./internal/web/
go test ./internal/shared/
```

Expected: pass, including the `en`/`de` parity guard and the coverage report —
this changes values and not keys, so parity is unaffected, and a failure here
means something else broke.

- [ ] **Step 5: Render it**

The application, not the documentation: the whitelist page in demo mode, in
**both languages**, at **1600 / 900 / 390**. The German string is longer than the
English and the French longer still; confirm none of the three overflows its
card or pushes the layout at 390.

- [ ] **Step 6: Commit**

```bash
git add locales/en.json locales/de.json locales/fr.json docs-tech/i18n-review.md
git commit -m "fix(i18n): the whitelist does not exempt an address from the protection modules"
```

---

## Tasks 16–22: the seven rewrite batches

Seven tasks with the same six steps and different files. The order is the
**reader's path**, not page size, so the voice settles on the pages read most
before it reaches the pages read least.

### The rules, for all seven

| Rule | What it means in practice |
|---|---|
| No prose sentence over 30 words | `node scripts/prose-check.mjs <files>` names them, with line numbers |
| Average under 18 words | Reported by the same run; the corpus starts at 22.7 |
| One claim per sentence | An em-dash aside carrying a second clause becomes its own sentence |
| An enumeration in prose becomes a table or a list | The fallback cases in `recovery.md` are the worst of these |
| Every number, key, path, flag, table and code block survives byte for byte | `git diff` on the batch shows no change inside a fenced block or a table row |
| The **claim** survives | The reason for step 4, and the reason this phase is the risk |

### Preserved without exception

`docs-tech/i18n-review.md` lists the ~30 sentences where a wrong word describes a
different firewall, in five clusters. Each batch below names the clusters it
touches. A sentence in one of them is **re-checked against the code**, not
reread.

### The six steps, for all seven

Substitute the batch's files for `<files>` and its clusters for `<clusters>`.

- [ ] **Step 1: Measure the batch**

Run: `node scripts/prose-check.mjs <files>`

Write the output into a scratch file. It is the task's to-do list: every line is
a sentence that must come under 30 words, and the summary line is the average
that must come under 18.

- [ ] **Step 2: Rewrite, one page at a time**

Apply the six rules above. Nothing inside a fenced block, a table row or front
matter is touched.

- [ ] **Step 3: Re-measure**

Run: `node scripts/prose-check.mjs <files>`

Expected: no lines listed, and the average under 18. If a sentence resists,
it is carrying two claims and wants splitting, not compressing.

- [ ] **Step 4: Re-check every claim you rewrote**

For each rewritten sentence in `<clusters>`: open the code it describes and
confirm the new sentence says what the code does. Not the old sentence — the
code. This is the only step in the plan with no mechanical check behind it, and
the only reason the phase carries a risk section.

- [ ] **Step 5: Prove nothing structural moved**

Run:

```bash
git diff -- <files> | grep -E '^[+-]' | grep -E '^\+.*\||^-.*\|' | head -40
git diff -- <files> | grep -cE '^[+-]\s*```'
```

Expected: the first prints nothing but table rows you deliberately rewrote, and
the second prints `0` — a fenced block opened or closed in this diff means code
moved when only prose should have.

- [ ] **Step 6: Render and commit**

Every page in the batch at **1600 / 900 / 390**, both themes. Then commit with
the message given for that batch.

---

### Task 16: Installation, the first three ways in

**Files:** `docs/_docs/index.md`, `docs/_docs/installation/requirements.md`,
`docs/_docs/installation/debian.md`, `docs/_docs/installation/docker.md`,
`docs/_docs/installation/manual.md`

**Baseline:** 6 sentences over 40 words; `manual.md` holds an 85-word one and
`requirements.md` averages 29.4 across only 8 sentences — the highest average on
the site, on the second page anybody reads.

**Clusters touched:** none. This batch has the fewest claims and the most
readers, which is why it is first.

**Watch for:** `requirements.md:19,30` says arm64 and it is **correct** —
`.goreleaser.yaml:15,26` builds `[amd64, arm64]`. A tester read it as excluding
the Raspberry Pi; it excludes 32-bit Pi OS and the Pi Zero/1/2, and a Pi 3/4/5
on a 64-bit OS is supported. Make that distinction explicit rather than shorter.

**Commit:** `docs(site): the pages that get read first read plainly`

---

### Task 17: Installation, the two that need explaining

**Files:** `docs/_docs/installation/demo.md`,
`docs/_docs/installation/first-run.md`,
`docs/_docs/installation/reverse-proxy.md`

**Baseline:** `first-run.md` holds the site's second-longest sentence at 160
words and 9 over 30. `reverse-proxy.md` is new in Task 11 and is written in the
target register already — measure it and fix anything that is not.

**Clusters touched:** *The second factor and recovery codes*, *Demo mode*.

**Watch for:** `firstrun_choices_desc` — "Staged, not applied. Nothing reaches
the firewall until you review and apply — and an apply undoes itself unless you
confirm it." Three claims in two sentences, all three load-bearing. The prose on
`first-run.md` says the same thing at length; it may get shorter and may not
lose one of the three.

**Commit:** `docs(site): the first-run page says three things instead of one long one`

---

### Task 18: The two pages the operator lives on

**Files:** `docs/_docs/features/dashboard.md`, `docs/_docs/features/apply.md`

**Baseline:** `apply.md` holds a 102-word sentence and averages 26.2.

**Clusters touched:** *What the acceptance window promises* — nine strings, and
the highest-consequence cluster on the list.

**Watch for:** every sentence about the window has to keep saying that the
**previous** rules come back and that doing nothing is what triggers it.
"Rolled back", "restored", "undone" and "reverted" are not interchangeable if
one of them implies the staged edits were lost. `accept_too_late` is explicit
that they are not: *Your edits are still staged.*

**Commit:** `docs(site): the acceptance window is explained in sentences you can hold`

---

### Task 19: The rule pages

**Files:** `docs/_docs/features/ports.md`, `docs/_docs/features/blacklist.md`,
`docs/_docs/features/whitelist.md`, `docs/_docs/features/forwarding.md`,
`docs/_docs/features/custom-rules.md`, `docs/_docs/features/filters.md`,
`docs/_docs/features/docker.md`

**Baseline:** 9 over 40 across seven pages; `blacklist.md` holds a 110-word one
and `custom-rules.md` averages 26.8.

**Clusters touched:** *Rule evaluation order*, *Blacklist and whitelist
semantics*.

**Watch for:** the order is the claim. *Before*, *after* and *first* are not
stylistic choices on these pages — `blacklist_order_body` and
`whitelist_order_note` say the same fact from two directions and both have to
survive. Task 15 has just corrected the protection-module claim in the locales;
`whitelist.md` and `blacklist.md` must agree with the corrected version and with
`internal/core/nftables.go`, not with the old string.

**Commit:** `docs(site): the rule pages say which list wins in one sentence each`

---

### Task 20: The system pages

**Files:** `docs/_docs/features/system-settings.md`,
`docs/_docs/features/two-factor.md`, `docs/_docs/features/audit-log.md`,
`docs/_docs/features/export-import.md`, `docs/_docs/features/recovery.md`

**Baseline:** `recovery.md` holds the site's longest prose sentence at **93
words**, carrying four claims, and 13 over 30. `two-factor.md` has the site's
highest average at 29.2. `export-import.md` is already clean and is in the batch
only so the group commits together.

**Clusters touched:** *Panic mode and recovery*, *The second factor and recovery
codes*.

**Watch for:** two things that are easy to soften and must not be. Ending panic
mode is **console-only, without exception** — the banner carries no button, on
purpose. And there is **no recovery by mail**: a lost password needs shell
access to the server. `recovery.md`'s 93-word sentence is the enumeration the
spec names; it becomes a table.

**Commit:** `docs(site): recovery explains itself in a table instead of a paragraph`

---

### Task 21: How it works

**Files:** `docs/_docs/architecture.md`, `docs/_docs/configuration.md`,
`docs/_docs/environment.md`, `docs/_docs/security.md`

**Baseline:** the largest batch — `configuration.md` alone has 88 prose
sentences, 23 over 30 and a 74-word one; `security.md` has 12 over 30.

**Clusters touched:** all five, in passing. These are the pages the others link
to for the full statement.

**Watch for:** `TestEveryConfigKeyIsDocumented` and
`TestAuditColourTableMatchesTheCode` both read these pages. Run
`go test ./internal/shared/` after this batch and not only at the end — a table
edited while rewriting the paragraph above it is how a key goes missing.

The `trusted_proxies` section shrank in Task 11; the shortened version is
rewritten here like anything else, and must keep saying that listing a network
rather than the proxies hands address spoofing to every host in it.

**Commit:** `docs(site): the reference pages are reference, in sentences`

---

### Task 22: Project

**Files:** `docs/_docs/contributing.md`, `docs/_docs/roadmap.md`,
`docs/_docs/license.md`

**Baseline:** `roadmap.md` averages 29.1 over only 11 prose sentences — the
tightest ratio on the site — and lost seven sections in Task 13, so re-measure
rather than trusting the number above. `license.md` is new in Task 10.

**Clusters touched:** none.

**Watch for:** `roadmap.md`'s two `Amended in` callouts record decisions and
their reasoning. Shorten them; do not lose *why* 3.0 and 3.1 kept their numbers,
which is the only thing in either callout that a reader cannot reconstruct.

**Commit:** `docs(site): the project pages say what they are for`

- [ ] **After Task 22: the whole corpus**

Run: `npm run check:prose`

Expected: **exit 0** — no sentence over 30 words anywhere, average under 18.
This is the first time it passes, and Task 24 wires it into CI on the strength
of it.

---

### Task 23: The captions say something the alt text does not

Seventeen `<figcaption>` lines. The tester's note said captions must describe
the image, for accessibility. **The `alt` text already does, and does it well** —
`ports.md:14` reads *"The port rules page: TCP and UDP tabs, a filter box, a
table of ports with an SSH protection checkbox and description…"*. A screen
reader gets an accurate account. Rewriting the captions to describe the image
would duplicate the alt text and help nobody: what a sighted reader sees is only
the caption, and the caption is commentary. It is a writing problem, not an
accessibility one.

So the captions are rewritten as **self-contained claims in the new register**:
name the thing on screen, then say the non-obvious part, so they read correctly
whether or not the body text came first. They do **not** describe the image.

And one is broken. `installation/first-run.md:89` puts a markdown link inside a
raw `<figcaption>`; kramdown does not parse markdown in raw HTML blocks, so it
renders as literal brackets. Exactly one caption does this.

**Files:** `docs/_docs/features/{filters,forwarding,ports,two-factor,system-settings,apply,audit-log,blacklist,custom-rules,dashboard}.md`, `docs/_docs/features/whitelist.md`, `docs/_docs/installation/first-run.md`

**Interfaces:**
- Consumes: `whitelist.md`'s caption, written in Task 12.
- Produces: nothing.

- [ ] **Step 1: Find them all**

Run:

```bash
grep -rn "figcaption" docs/_docs --include='*.md'
```

Expected: 17 lines before Task 12, 18 after it. Any number other than that means
one was added or lost and the list below is stale.

- [ ] **Step 2: Fix the broken one first**

`docs/_docs/installation/first-run.md:89` — the link goes, because kramdown will
not parse it there and the caption reads fine without it:

```html
  <figcaption>The only time these eight codes are shown. Copy them now; the second-factor page can issue new ones, but it cannot show these again.</figcaption>
```

Do not "fix" it by moving the link into the body text. The caption's job is to
stand alone.

- [ ] **Step 3: Rewrite the rest**

Each caption names what is on screen, then says the part that is not visible in
the screenshot. Under 30 words, one claim, no markdown syntax of any kind — a
`<figcaption>` is raw HTML and everything in it renders literally.

The existing seventeen are already close to this register; several need nothing
but a shortened second clause. Read each against its own page and change what
does not stand alone.

- [ ] **Step 4: Prove no caption contains markdown**

Run:

```bash
grep -rn "figcaption" docs/_docs --include='*.md' | grep -E '\]\(|\*\*|`|\[.*\]'
```

Expected: **no output**. A hit is a caption that will render as literal
punctuation — which is the bug this task exists to fix, reintroduced.

`<code>` inside a caption is fine and is used at
`features/custom-rules.md:16`: it is HTML inside HTML, not markdown.

- [ ] **Step 5: Render every one of them**

`npm run build:docs`, then every page carrying a figure, at **1600 / 900 / 390**,
both themes. Read each caption on screen. This is the only check there is: a
caption is either literal brackets or it is not, and only a browser says which.

- [ ] **Step 6: Commit**

```bash
git add docs/_docs/features docs/_docs/installation/first-run.md
git commit -m "fix(docs): a caption says the part the picture does not, and renders as one"
```

---

### Task 24: CI keeps both of them

`codespell` is a typo-only dictionary — so near-zero false positives on
`nftables`, `conntrack` and `argon2id`, where `hunspell` floods. It is a step in
`docs.yml` or it rots. `check:prose` joins it, now that Task 22 made it pass.

**Files:** Create `.codespellrc`; modify `.github/workflows/docs.yml`

**Interfaces:**
- Consumes: `npm run check:prose` (Task 14, passing since Task 22),
  `npm run check:changelog` (Task 9), `npm run check:docs` (Tasks 2–5).
- Produces: nothing.

- [ ] **Step 1: Install it locally**

Run: `pip install codespell`

It is not on this machine. Do not work around a missing tool.

- [ ] **Step 2: Run it and read every hit**

Run: `codespell docs/ README.md CHANGELOG.md`

Every hit is judged by hand before anything is added to an ignore list. A real
typo is fixed; a false positive earns its place on the list with a reason.

- [ ] **Step 3: Write the configuration**

`.codespellrc`:

```ini
# codespell, not hunspell: a typo-only dictionary, so nftables, conntrack and
# argon2id do not need excusing. The list below is only for words codespell
# itself gets wrong on this corpus.
[codespell]
skip = ./docs/_site,./node_modules,./.git,./docs/Gemfile.lock,./docs/assets/css/style.css,./web/static
# nd  — appears in `2nd`, which codespell reads as a typo of `and`
# ba  — the base32 alphabet in the TOTP examples
ignore-words-list = nd,ba
```

Every entry carries the reason it is there. An ignore list nobody can audit is
an ignore list that grows until the tool is off.

- [ ] **Step 4: Add the steps to the workflow**

In `.github/workflows/docs.yml`, in the `build` job (the pull-request one), after
the *Site builds* checkout and before *Build docs*:

```yaml
      - name: Spelling
        run: pipx run codespell

      - uses: actions/setup-node@v7
        with:
          node-version: "24"

      # Three checks that need no Ruby and no built site: the generated
      # changelog page matches CHANGELOG.md, and the prose is inside the two
      # limits the docs-site-polish branch set. check:docs is not here — it
      # needs a built _site and a browser, and it runs after the build below.
      - name: The changelog page matches CHANGELOG.md
        run: npm run check:changelog

      - name: Prose stays inside its limits
        run: npm run check:prose
```

These go on the **pull-request** job only. The deploy job publishes what a pull
request already proved; a spelling failure on `main` blocks a deploy for a typo
and helps nobody.

- [ ] **Step 5: Verify each step by breaking it**

Three separate breaks, each pushed to the branch or run through `act`, whichever
is available:

1. Misspell a word in a `docs/_docs` page → the Spelling step fails.
2. Edit a `CHANGELOG.md` headline without regenerating → the changelog step fails.
3. Add a 40-word sentence to any page → the prose step fails.

Revert all three. A CI step nobody has watched fail is a CI step that passes for
the wrong reason.

- [ ] **Step 6: Commit**

```bash
git add .codespellrc .github/workflows/docs.yml
git commit -m "ci(docs): a typo and a forty-word sentence both fail the pull request"
```

---

## Closing the branch

- [ ] **Everything, in one run**

```bash
make test lint
go test ./internal/...
npm run build:docs-css && git diff --stat docs/assets/css/style.css
npm run build:changelog && git diff --stat docs/_docs/changelog.md
npm run check:changelog
npm run check:prose
npm run build:docs
npm run check:docs
codespell
```

Expected: all pass, and both `git diff --stat` lines print **nothing** — a
generated file that changes when it is rebuilt was committed stale.

- [ ] **Screenshots are not re-taken**

Confirm, do not assume: `git diff --stat main -- docs/assets/img/screens/`
prints nothing. Nothing in this branch changed the application interface. Task
15 changed two strings *in* it — read them on screen, and if either now wraps
differently in a way a screenshot would show, that screenshot is re-taken and
this line stops applying.

- [ ] **The nav and the pages agree**

```bash
find docs/_docs -name '*.md' | wc -l     # 30
grep -c 'path: /docs/' docs/_config.yml  # 30
```

Both derived, both checked by `TestEveryDocsPageIsInTheNav`, and worth reading
once by eye because the number in the spec was 30 and this is where it is either
true or is not.

- [ ] **Ko-fi**

This branch is not a release, so it is not a release post. It is a **workbench
post**: *"Pagefind returns 23 results for `asdasd`, because it truncates your
query until something matches — and the hook that looks like it exists to fix
that cannot."* That is a post. Offer to draft it; per the maintainer's standing
note, it will otherwise not get written.

---

## Self-review

**Spec coverage.** Every section of
`docs-tech/specs/2026-08-28-docs-site-polish.md` maps to a task:

| Spec | Task |
|---|---|
| §0 the six re-diagnosed notes | 2 (contents), 7 (italics), 8 (demo link), 23 (caption link, captions), 16 (armhf/arm64 wording). The em-dash note has **no target** — see below |
| §0 the search result count | 4 |
| §2 items 1–6 | 2, 3, 4, 5, 6, 7 |
| §2 `scripts/docs-check.mjs`, four assertions | 2, 3, 4, 5 — one assertion each |
| §3 new pages: reverse-proxy, whitelist, license, changelog | 11, 12, 10, 9 |
| §3 the changelog page and its headlines | 9 |
| §3 consolidations: demo link, demo banner, roadmap, demo mode | 8, 8, 13, 13 |
| §3 guard tests | 8 (new host guard), 9 (check:changelog), 12 (`/whitelist` remap). The nav and search-index counts need no edit — corrected at the top |
| §4 targets, order, batching | 14, 16–22 |
| §4 captions | 23 |
| §4 spelling | 24 |
| §4 screenshots not re-taken | *Closing the branch* |
| §5 verification | in every task's own steps |
| §6 the risk | Phase 3 preamble, and step 4 of each batch |

**One spec item has no task, deliberately.** §0's em-dash note: *"No target
exists."* Searching `_docs/`, the landing page, the layouts, the includes and
`locales/en.json` finds zero hyphens used as a dash; the single hit, `l--p` in
`filters.md:197`, is inside a flag example. Either the tester saw it in the
running application or it was a general note. **Ask them which page.** A task
that changes something to answer it would be changing something at random.

**Placeholder scan.** No step says "add error handling", "similar to Task N",
"write tests for the above", or "TBD". Task 9's thirty headlines and Task 23's
captions are the two places where prose is required rather than shown: the
headlines are given in full as drafts to accept or replace, and the captions are
given as a rule plus the one that is broken, because seventeen existing lines
already in the right register need reading and not replacing.

**Type consistency.** `scripts/docs-check.mjs` defines `serve`, `ok`, `failed`
and `main()` in Task 2, and Tasks 3, 4 and 5 add `checkSearchKey(page, base)`,
`checkSearchResults(page, base)` and `checkCopyButton(context, page, base)` —
the last takes the context, for `grantPermissions`, and `main()` is written in
Task 2 with `context` already in scope. `scripts/render-changelog.mjs` exposes
`build:changelog` / `check:changelog`; `scripts/prose-check.mjs` exposes
`check:prose`; `scripts/docs-build.sh` exposes `build:docs`. Task 8's
`TestTheOldDemoHostIsNotPublished` exempts `docs/_docs/changelog.md` before Task
9 creates it, and Task 9 step 8 is where that exemption is first exercised.
