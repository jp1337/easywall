# A search on the documentation site — design

**Goal.** Somebody who wants `acceptance.duration`, `EASYWALL_WEB_BIND_ADDR` or
`rollback_skipped` types it and lands on the row that defines it. Somebody who
wants "how do I open a port" types that and lands on the section that says.

**Why now.** 26 pages and roughly 26,500 words, of which **691 lines are table
rows** — the reference material is in tables, and the sidebar cannot find a
table row. The request arrived twice; the first one was lost, which is its own
argument.

Not published: this file is under `docs-tech/`, which is outside the Jekyll
source. See `TestTheTechnicalDocsAreNotPublished`. A spec written into `docs/`
would appear on easywall-project.org.

## The decision, and what it rests on

**Pagefind 1.4**, indexing the built HTML after `jekyll build`, with its own UI
component styled to `DESIGN.md`, in a field at the top of the sidebar.

Every number below was measured against the real built site, not read from a
README.

| Question | Answer | How it was checked |
|---|---|---|
| Do identifiers match at all? | Yes. `acceptance.duration` → 2 hits, `EASYWALL_WEB_BIND_ADDR` → 1, `rollback_skipped` → 1, `BIND_ADDR` → 3 | `pagefind.search()` in a browser against the built site |
| Is table content reached? | Yes — the index is built from rendered HTML, so a table cell is text like any other. `bogon` returns the filter table's rows | same |
| Can a result point *into* a page? | Yes. Sub-results carry a heading title and its anchor: `bogon` → *What the bogon filter drops* → `/docs/features/filters/#what-the-bogon-filter-drops` | `sub_results` on the first hit |
| How big is it? | Index 80 KB, fragments 212 KB, engine 44.5 KB, UI 117.2 KB JS + 14.1 KB CSS | `du`, `ls` on the emitted bundle |
| How long does indexing take? | 0.058 s for 26 pages | Pagefind's own output |

Rejected: **lunr.js**, because the index would have to be built from the
markdown, and that is where the tables stop being tables — either the reference
content is lost or an extractor has to be written for it. **Algolia DocSearch**,
because it is an external service in a documentation set whose selling point is
that easywall reaches none.

## What the spike found that the design has to answer

Four things, all of them real, none of them guessed.

### 1. The page-level excerpt does not show the match

Searching `acceptance.duration` returned `/docs/configuration/` with the excerpt
*"daemon that refuses to start is a worse outcome than one running a doc"* —
prose from elsewhere on the page. The **sub-result** excerpt for the same hit is
the right text.

**So:** the UI shows sub-results (`showSubResults: true`), not the page excerpt
alone. A result whose snippet has nothing to do with the query is worse than no
snippet, because it reads as a wrong answer rather than a missing one.

### 2. The landing page competes with the documentation

`/` ranked for `argon2id` and for `port forwarding`. It is a marketing page; a
person searching the docs does not want it.

### 3. Ranking is currently accidental

`/docs/installation/demo/` outranked `/docs/environment/` for `BIND_ADDR`,
because the demo page happens to contain the string in a shell block.

**So:** 2 and 3 are both addressed by scoping the index rather than by tuning
weights. `--glob "docs/**/*.html"` and `--root-selector "main.content"` reduce
50 indexed files to the 26 documentation pages and drop the landing page, the
sidebar, the topbar and the per-page table of contents from the indexed text.
Weighting is deliberately *not* part of this design: with the index scoped, the
remaining ordering was correct in every query tried, and a weight added before
anybody needs it is one nobody will understand later.

### 4. `<html lang>` disagrees across the site — 27 pages `en`, 23 `en-US`

Pagefind read that as two languages and built two indexes, so a search from a
page in one would never see the other. The 23 are the old-path redirect stubs
that `jekyll-redirect-from` generates from **its own** built-in layout, which
hardcodes `lang="en-US"`; `docs/_config.yml` says `lang: en` and
`docs/_layouts/default.html` says `<html lang="en">`.

This is a defect independent of search: a screen reader is told a different
language on half the site's URLs. It is repaired here, in the same change, at
the user's request.

**So:** two separate fixes, because they fail differently.

- The `--glob` above already keeps the stubs out of the index, so the split
  cannot reach the search through them. `--force-language en` is kept anyway, as
  the one line that states the site's language to the indexer rather than
  leaving it inferred per file — cheap, and it fails loudly if a page ever
  disagrees.
- A repository-owned `docs/_layouts/redirect.html`, replacing the plugin's
  built-in template, is what actually repairs the defect: it makes the stubs
  themselves say `en`. The search would be correct without it, and a screen
  reader would still be told the wrong language on 23 URLs.

Whether `jekyll-redirect-from` honours a layout of that name in this version is
the first thing the implementation must establish; the plugin documents the hook,
and a built stub whose `lang` is unchanged is how it would fail.

## Architecture

Four pieces, each with one job.

```
jekyll build ──► docs/_site  ──► pagefind --site _site --glob … ──► _site/pagefind/*
                    │                                                     │
                    │                                                     │
              main.content                                        index + fragments
              is the indexed                                      + engine + UI
              region                                                      │
                    ▼                                                     ▼
        sidebar: <div id="docs-search">  ◄── loaded on first interaction ──┘
```

| Piece | Job | Depends on |
|---|---|---|
| `pagefind` CLI in CI | turn built HTML into a static index | `_site` existing |
| `docs/_includes/search.html` | the container and its label, in the sidebar | nothing |
| the loader script | fetch engine + UI on first interaction, hand over focus | the container |
| `web/src/docs.css` | map Pagefind's custom properties onto easywall tokens; `npm run build:docs-css` renders it to the committed `docs/assets/css/style.css` | the existing token set |

### The indexing command

```bash
npx pagefind@1 --site _site \
  --glob "docs/**/*.html" \
  --root-selector "main.content" \
  --force-language en
```

Verified output: `Discovered 1 language: en`, `Indexed 26 pages`. Before
scoping it was 2 languages and 50 pages.

### Where it runs

`docs.yml` already has two jobs that both run `bundle exec jekyll build`: `build`
on a pull request, and `deploy` on a push to main. The index step goes into
**both**, immediately after the build — into `deploy` because that is what ships,
and into `build` because a pull request that breaks indexing must fail on the
pull request. The repository already has Node for Tailwind and the diagrams, so
this adds a tool but not a toolchain.

The index is **not** a committed build output, unlike `web/static/style.css` and
the diagrams. It is derived from `_site`, and `_site` is not in the repository,
so there is nothing to diff a commit against. Its protection is therefore a CI
assertion rather than a rebuild-and-compare — see *Guards*.

### Loading

The engine and UI are ~176 KB together. The documentation site currently ships
almost no JavaScript, and a reference page that is read far more often than it is
searched should not pay for the search on every view.

So the sidebar renders an inert input, and the engine is imported on the first
focus or keystroke, after which `PagefindUI` takes over with `autofocus` so the
keystroke that triggered the load is not lost. The exact hand-over is the one
part of this design that must be confirmed by rendering rather than reasoning:
whether the first character survives the swap is a browser question.

### Without JavaScript

A client-side index cannot work without JavaScript, and there is no server on
GitHub Pages to move it to. The sidebar navigation stays what it has always
been — the complete, working, scriptless way to reach every page — and the search
field is **hidden** unless a script is running, using the same `data-js`
attribute pattern the firewall interface uses for its language switcher.

An input that does nothing is worse than an absent one. This is not the language
switcher, where the no-JS path was non-negotiable because an operator who cannot
read the interface must still be able to change it; nobody is locked out of a
documentation site by the absence of a search box.

## Interface

The field sits at the top of the sidebar, above the navigation. Results replace
the navigation while a query is present, grouped by page with heading-level
sub-results beneath each.

```
┌─ Sidebar ──────────┐
│ ⌕ acceptance.dur   │
├────────────────────┤
│ Configuration      │
│  ‣ Acceptance      │
│    "Seconds befo…" │
│                    │
│ System Settings    │
│  ‣ Choosing a …    │
└────────────────────┘
```

Styling goes through Pagefind's own custom properties rather than overriding its
selectors — `--pagefind-ui-primary`, `--pagefind-ui-background`,
`--pagefind-ui-border`, `--pagefind-ui-text`, `--pagefind-ui-font`,
`--pagefind-ui-tag`, `--pagefind-ui-border-radius`, `--pagefind-ui-border-width`,
`--pagefind-ui-scale`. All eleven exist in the shipped `pagefind-ui.css`; the two
image-related ones are irrelevant because `showImages` is off. Fighting a
third-party stylesheet by selector is how a component breaks on its next minor
release.

`?highlight=` — Pagefind ships `pagefind-highlight.js`, which highlights the
matched term on the page the result leads to. For a design whose stated purpose
is landing on the right *row*, arriving at the right *section* with the term
marked is the difference between an answer and a page to re-read. Included, and
to be confirmed against the real markup.

## Guards

| Guard | Protects | Why it is not obvious |
|---|---|---|
| CI: exactly one language index | the `en`/`en-US` split cannot come back | It failed silently: two indexes built, no warning, and search worked *within* each half |
| CI: indexed page count equals the number of documentation pages | the glob does not quietly stop matching | Pagefind exits 0 having indexed nothing if the glob matches nothing |
| CI: the index directory is non-empty and contains a `pf_meta` | an index that built but produced no searchable data | Same failure mode as the "site is not empty" check next to it, for the same reason |
| Go: the sidebar include references the search container | the field is not dropped from the layout by an edit elsewhere | A missing container is an invisible failure — the page renders, there is simply no search |
| Go: `_config.yml`'s `lang` and `default.html`'s `<html lang>` agree | the inconsistency this change repairs cannot reappear from the config side | Two sources for one fact is what produced the split |

## Out of scope

- **Search weighting and `data-pagefind-weight`.** Scoping the index fixed the
  ordering problems that were actually observed. Not needed yet.
- **Filters and facets** (by page type, by version). 26 pages do not need
  faceting.
- **German-language search.** The documentation is English; the interface is
  translated, the docs are not.
- **Searching `docs-tech/`.** It is not published and must not become
  searchable.
- **A Cmd+K overlay.** Considered and declined in favour of the sidebar field,
  which is close to what Pagefind ships and needs no focus trap, no scroll lock
  and no second accessibility story.

## Success criteria

1. `acceptance.duration`, `EASYWALL_WEB_BIND_ADDR` and `rollback_skipped` each
   return the page that defines them, as the first hit, with a sub-result whose
   excerpt contains the term.
2. "how do I open a port", typed as prose, returns the Ports page.
3. Exactly one language index, and an indexed page count equal to the number
   of documentation pages — 26 today, and derived rather than restated so that
   adding a page does not need this line edited.
4. No result points at the landing page or at a redirect stub.
5. The documentation site loads no search JavaScript until the field is touched.
6. With JavaScript off, no search field is visible and every page is still
   reachable through the sidebar.
7. Rendered and checked at 1600 / 900 / 390 px in both themes, per `CLAUDE.md`.
