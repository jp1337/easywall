---
title: "Docs site polish — what user testing found"
date: 2026-08-28
status: approved, not yet planned
---

# Docs site polish — what user testing found

A round of user testing on easywall-project.org produced twenty-six notes. This
spec covers the twenty-one that belong to the documentation site; five are
recorded here as out of scope, with the release each one belongs to.

Agreed with the maintainer on 2026-08-28. Every note was checked against the
code before it entered this spec, and six of them turned out to describe
something other than what they said. Those corrections are kept, because a plan
built on the note rather than on the finding would fix the wrong thing.

Branch: `docs/site-polish`.

## 0 · What the notes actually were

Six notes changed meaning once the code was read.

| Note | Finding |
|---|---|
| armhf unsupported, so no Raspberry Pi with Pi-hole | The docs are **correct**. `.goreleaser.yaml:15,26` builds `[amd64, arm64]`; `requirements.md:19,30` already says arm64 only. A Pi 3/4/5 on a 64-bit OS is supported; 32-bit Pi OS and the Pi Zero/1/2 are not. Adding armhf is a build decision, not a docs fix — out of scope |
| The demo link is now correct | One page uses the new host (`features/system-settings.md:51`). **Five** still point at `easywall.wdkro.de`: `README.md:29`, `docs/index.md:34`, `docs/index.md:98`, `_docs/index.md:44`, `_docs/installation/demo.md:13`. `telemetry.wdkro.de` is a different host and is not touched |
| The contents column does not reach the bottom on small topics | Not a height bug. `.docs-toc` is `position: fixed` with `max-height: calc(100vh - 6rem)`. It is the scroll spy: `default.html:285` observes with `rootMargin: '-72px 0px -70% 0px'`, and on a short page the last heading can never enter a band that ends 70 % up the viewport, so the last entries never become current |
| Links in captions do not work | Exactly one: `installation/first-run.md:89`. A markdown link inside a raw `<figcaption>`; kramdown does not parse markdown in raw HTML blocks, so it renders as literal brackets |
| Captions must describe the image — accessibility | The `alt` text **already does**, well. `ports.md:14` reads "The port rules page: TCP and UDP tabs, a filter box, a table of ports with an SSH protection checkbox and description…". A screen reader gets an accurate account. What a sighted tester sees is only the caption, which is commentary. Rewriting captions to describe the image would duplicate the alt text and help nobody. It is a writing problem, not an accessibility one |
| Em dashes instead of hyphens | **No target exists.** Searched `_docs/`, the landing page, the layouts, the includes and `locales/en.json`: zero hyphens used as a dash. The single hit, `l--p` in `filters.md:197`, is inside a flag example. Either the tester saw this in the running application or it was a general note. Ask them which page |

Italic text was reported as hard to read. Measured, it is not a contrast
failure: `--text-muted` on `--bg` is **5.99 : 1** in light and **8.49 : 1** in
dark, both past AA. It is a semantic defect — `em` means emphasis and
`docs.css:1525` de-emphasises it. The fix is the same either way.

### The search result count, diagnosed

`asdasd` returning 23 results is a real bug with an exact cause. **Pagefind
truncates a query term until a prefix matches something in the index.**
Reproduced against the built index:

| query | results | what actually matched |
|---|---|---|
| `asdasd` | 23 | `as` |
| `asd` | 23 | `as` |
| `xyzzyq` | 1 | `X`, from `iptables -X` |
| `q` | 14 | `QR` |
| `zzzzzzzz` | 0 | nothing starts with `z` |

This gives a clean discriminator, because forward prefix matching is the
behaviour that is wanted: `config` → `configuration` marks a **longer** word
than was typed, while truncation always marks a **shorter** one. So: keep a
result only if some marked word is at least as long as the query word it
matched. `config` survives; `asdasd` does not.

## 1 · Scope

Three phases in one branch, in this order. The order is a dependency, not
tidiness: splitting `blacklist.md` after rewriting it means rewriting it twice.

```
Phase 1  mechanics   ── no content changes, verified in a browser
   │
Phase 2  structure   ── new pages, splits, consolidations, guard tests
   │
Phase 3  prose       ── 12,781 words, verified against the code
```

### Out of scope, and where each note goes

| Note | Belongs to |
|---|---|
| Blocklist / Allowlist instead of Blacklist / Whitelist | Its own release. **961 occurrences**, 48 files in `internal/` alone: protocol commands, the `/whitelist` route, `handler_whitelist.go`, `web/templates/whitelist.html`, config keys, stored rule files, three locales, audit-log detail strings. A breaking change that needs a migration and redirects |
| armhf builds | A build decision for a future release |
| A package registry instead of a `.deb` download | Infrastructure: an apt repository, a signing key, hosting |
| Impressum | Needs the maintainer's legal name and address |
| Datenschutzerklärung (GDPR) | Same controller details as the Impressum, so it is blocked on the same thing |

The two list pages **are** split in this branch, under their current names.
The rename release moves the URLs; `jekyll-redirect-from` is already installed,
so the documentation side of that rename costs two lines of front matter.

## 2 · Phase 1 — mechanics

Files: `web/src/docs.css`, `docs/_layouts/default.html`,
`docs/_includes/search.html`. No content changes.

| # | Change | Where |
|---|---|---|
| 1 | Replace the `IntersectionObserver` with one rAF-throttled scroll handler: current = the last heading whose top is ≤ 100 px, and the last heading unconditionally once the document is scrolled to its end. Net deletion | `default.html:238–287` |
| 2 | `/` opens the search. The Ctrl/Cmd branch goes, and with it the `navigator.platform` sniff — `/` is the same key everywhere, so the macOS correction has nothing to correct. Badge reads `/`. Guarded against firing while focus is in a field, or `/` cannot be typed into the search box itself | `default.html:315–382`, `search.html:33` |
| 3 | A `processResult` hook drops a result unless some `<mark>` is at least as long as the query word it matched. The current query is read from `panel.querySelector('input').value`. `processResult` and `processTerm` both exist in PagefindUI 1.5.2 — verified in the built bundle | `default.html:333–352` |
| 4 | A copy button in each `.highlighter-rouge` — that is the element carrying the frame; `div.highlight` and `pre.highlight` are deliberately bare. `navigator.clipboard.writeText`. Feedback is the button's own label swapping to **Copied** for 2 s plus one `aria-live="polite"` region, not a floating box: it is the convention on GitHub and MDN, needs no positioning, and a toast is announced badly or not at all | `default.html`, `docs.css` |
| 5 | The search clear button reads as oversized and misplaced. **Diagnose by rendering, do not guess a number.** The existing constraint is recorded at `docs.css:1325`: its width and right offset are Pagefind's, because the input's `padding-right` is measured from them, so a narrower button makes the query run underneath it | `docs.css:1325–1341` |
| 6 | Delete `.content-body em { color: var(--text-muted) }` and the `blockquote em { color: inherit }` exception that exists only to undo it. Two deletions. `.content-body em` is **not** in `TestDocsStylesheetKeepsLoadBearingRules`, so nothing asserts it | `docs.css:1525`, `:1584` |

### New: `scripts/docs-check.mjs`

Items 1–4 are behaviour. No Go test against the stylesheet and no diff can
prove them, and `scripts/ui-check.mjs` drives the application in demo mode, not
the documentation site. So one new script on the `playwright-core` already in
`devDependencies`, serving a built `_site`, asserting four things:

1. the last contents entry is current at the bottom of a short page
2. `/` opens the overlay, and typing `/` inside the field does not reopen it
3. the copy button writes to the clipboard and confirms
4. `asdasd` yields no results, and `config` still finds `configuration.md`

Four assertions, not a suite. `npm run check:docs`.

## 3 · Phase 2 — structure

### New pages

| Page | Diátaxis type | Content |
|---|---|---|
| `installation/reverse-proxy.md` | How-to | The task — put easywall behind nginx — currently split across `configuration.md:241` (reference), `security.md:83` (explanation) and a table row in `environment.md:63`. Three page types answering one task is why it reads as duplicated. Both existing sections shrink to a pointer; the key itself stays documented in `configuration.md` beside every other key. Carries the Docker example: `trusted_proxies` names the **proxy's own address**, not the subnet, with `docker network inspect` shown — the default bridge is `172.17.0.0/16` but a compose project gets its own network |
| `features/whitelist.md` | How-to | The split. `blacklist.md` keeps blacklist-only content. Neither page restates the packet order; both point at the `rule-order` diagram in `architecture.md`, which is already canonical |
| `license.md` | Explanation | What GPL-3.0 means to somebody running easywall: run it, modify it, and which obligations begin only on distribution. Links the full text; does not inline 34 KB. The footer's plain-text `GPL-3.0` at `default.html:179` becomes a link to it |
| `changelog.md` | Reference | Generated — see below |

### The changelog page

One page. Each version is a `<details>`; the newest is `open`, the rest collapse
to a one-line summary. One URL, one nav entry, and no per-version routing —
`CHANGELOG.md` holds 30 versions and 1,260 lines, so a flat page is 15 screens.

`CHANGELOG.md` **stays the single source** at the repository root: GitHub reads
it, release tooling reads it. `scripts/render-changelog.mjs` generates
`docs/_docs/changelog.md`, committed like `docs/assets/diagrams/` and the built
CSS, with a `--check` mode wired to `npm run check:changelog` so a pull request
that edits the changelog without regenerating fails.

The collapsed rows need a headline per version, and none exists — `## [2.13.0] —
2026-08-28` is followed straight by `### Added`. Rather than a version → headline
map inside the generator, the headline goes into `CHANGELOG.md` itself:

```markdown
## [2.13.0] — 2026-08-28

**Behind a proxy, easywall knows who you are.**

### Added
```

One source, no second file to keep in step, and GitHub renders it better too.
Thirty retrofits — and the wording for the 2.x ones already exists, in the
roadmap's "Done in 2.x" sections that this same phase deletes.

### Consolidations

| Item | Change |
|---|---|
| Demo link and credentials | `demo_url`, `demo_user`, `demo_password` in `_config.yml`, beside `site.discord` and `site.version`, which already work this way. The five hardcoded places updated |
| Demo banner | One `_includes/demo-callout.html`, reading the URL and credentials from the config, used everywhere the demo is offered in the documentation. The landing page keeps its buttons — a marketing hero and a docs callout are not one component, and forcing them to be makes both worse |
| Roadmap | Delete the five "Done in 2.x" sections (~100 lines) and the shipped 2.12 and 2.13 rows from the future table. Add a link to `/docs/changelog/`. The headlines are not lost: they become the changelog's summary lines |
| Demo mode | The `What the mock does` command table (`demo.md:48`) moves to `docs-tech/`. The callout beneath it **stays**, rewritten: with no `nft` binary, custom-rule validation reports itself unavailable, and it once answered "no errors" to anything typed. Somebody running the demo needs that; the per-command table is for whoever maintains the mock |

### Guard tests

| Test | Change |
|---|---|
| `TestEveryDocsPageIsInTheNav` | four new nav entries; 26 pages becomes 30 |
| `TestEveryPageIsDocumented` | `/whitelist` remaps from `blacklist.md` to `whitelist.md` |
| `.github/actions/build-search-index` | `pages=$(find _docs -name '*.md' \| wc -l)` is already derived, so 30 is picked up with no edit |
| **new** | `easywall.wdkro.de` appears in no published file. Exempt: `CHANGELOG.md` and `docs-tech/plans/`, which record what shipped and must keep the old host |
| **new** | `npm run check:changelog` — the generated page matches `CHANGELOG.md` |

## 4 · Phase 3 — prose

A full plain-language rewrite. The corpus is smaller than the raw count
suggests, because most of `_docs/` is already in the terse form:

| | |
|---|---|
| total words in `_docs/` | 28,698 |
| **prose words** — tables, code blocks, HTML and front matter removed | **12,781** |
| sentences over 40 words | **42** |
| longest prose sentence | **93** words, `features/recovery.md` |
| average sentence | 19–30 words per page |

### Targets

| Rule | Today |
|---|---|
| no prose sentence over 30 words | 42 are over 40; the worst is 93 |
| average under 18 words | 19–30 per page |
| one claim per sentence — an em-dash aside carrying a second clause becomes a sentence | the 93-word sentence carries four |
| an enumeration in prose becomes a table or a list | several, including the fallback cases in `recovery.md` |

### Preserved without exception

Every number, key name, path, flag, table and code block — and the **claim**.
`docs-tech/i18n-review.md` already lists the ~30 sentences where a wrong word
describes a different firewall: which list is consulted first, what the
acceptance window undertakes, what panic mode does not end. Each is re-checked
against the code rather than reread. The tables are covered by existing tests:
`TestAuditColourTableMatchesTheCode`, `TestEveryConfigKeyIsDocumented`.

### Order and batching

Reader path, not page size, so the voice settles on the pages read most:
`index` → `requirements` → `debian` / `docker` / `manual` → `demo` →
`first-run` → `dashboard` / `apply` → the rule pages → the system pages →
`configuration` / `environment` / `security` / `architecture` → `contributing` /
`roadmap` / `license`. Seven commits, one per nav group.

### Captions

The 17 `<figcaption>` lines are rewritten as self-contained claims in the new
register — they name the thing on screen, then say the non-obvious part, so they
read correctly whether or not the body text came first. They do **not** describe
the image; the alt text does that already and does it well. The broken markdown
link at `first-run.md:89` is removed.

### Spelling

`hunspell` is installed on the maintainer's machine but floods technical prose
with false positives. `codespell` — a typo-only dictionary, so near-zero false
positives on `nftables`, `conntrack`, `argon2id` — is added as a step in
`docs.yml` with a short ignore list. Automated, or it rots. Not yet installed
locally; `pip install codespell`.

### Screenshots are not re-taken

Nothing in this branch changes the interface, so `docs/assets/img/screens/*`
still describes what ships. Stated because the rule is otherwise unconditional.

## 5 · Verification

| Phase | How |
|---|---|
| 1 | `npm run build:docs-css`, then **grep the built file** — Tailwind drops rules silently and the build stays green. Rendered at 1600 / 900 / 390 in both themes. `npm run check:docs` |
| 2 | `go test ./internal/...` for the nav, coverage and demo-host guards. `npm run check:changelog`. Every new page rendered in both themes |
| 3 | The 42 rewritten long sentences re-read against the code they describe, and the `i18n-review.md` list checked line by line. `codespell` clean |

**Building the site locally needs a container.** There is no `ruby`, `gem`,
`bundler` or `jekyll` on this machine — Bazzite is immutable. `podman` is
present, so the build runs through a `ruby:3.4` image. This is a prerequisite
for the whole branch: every item above is verified by rendering.

## 6 · The risk, named

Phase 3 is the one part of this branch where the tests cannot tell you it went
wrong. A flattened sentence that still parses and still sounds true is invisible
to CI. The `i18n-review.md` list plus a code re-read on each of the 42 rewritten
sentences is the entire mitigation, and none of it is automatic.
