# easywall's published site: a landing page, the docs under /docs, and the brand as an entity

**Date:** 2026-08-16 · **Status:** approved, not yet executed

Not published. This directory sits outside `docs/`, which is the entire Jekyll
source — see `TestTheTechnicalDocsAreNotPublished`.

## The situation

Easy wall is one small nftables firewall among many. The published site
(`easywall-project.org`) is currently the documentation, nothing else: the home
URL `/` carries a documentation page whose first heading is a tagline, and every
other page hangs off it. There is no page that does what a home page is for —
say what the product is, who it is for, and how to run it — in the 90 seconds a
first-time visitor gives it.

The audience the site needs to win is somebody running a Raspberry Pi or a home
server who does not manage firewalls for a living. When that person searches,
they land on the current home page and are met with the *rule order*, the
*process model* and the way *staged, current and backup* relate — helpful once
you have decided, useless before you have. Pi-hole wins that audience with a
home page that says *install it, it just works, your data stays yours*. easywall
needs that same first floor, with the documentation under it.

There is a second, more mechanical reason this is the right moment: the domain
is weeks old. Nothing of value lives at the current URLs yet. Any URL change
done now is cheap; the same change in two years would strand a sitemap full of
backlinks. The unchanged 23 `sitemap.xml` entries would hard-404 and the
submission to Search Console would have described a site that no longer has
those pages.

## Decisions

| Question | Decision |
|---|---|
| Where the landing lives | At `https://easywall-project.org/` — the Jekyll source root. Never under `/docs` |
| Where the docs live | Under `/docs/**` URLs, same one Jekyll site, same deployment, same CNAME. A real subdomain would need a second Pages site and is rejected (below) |
| Subdomain vs path | Path. GitHub Pages serves one custom domain per Pages site; `docs.` would need a second repo, a second build and an extra DNS record. The cost buys nothing the path does not |
| How `/docs/**` URLs are produced | A Jekyll **collection** `_docs` with `permalink: /docs/:path/`, source files in `repo/docs/_docs/…`. Rejects the obvious alternative — nesting content in `repo/docs/docs/…` — because the double folder is a permanent trap, not a temporary one |
| Old URLs | Stub at each old path with `jekyll-redirect-from` (`redirect_to:`). Static GitHub Pages cannot serve a true HTTP 301; the stub ships meta-refresh plus canonical. Accepted: the domain is fresh, and the Search Console sitemap has not been live long enough to matter |
| The rich home-page doc | Motion the content (`rule order`, the "made of" table) into the `/docs/` overview. Nothing is deleted, only it moves |
| The brand as one entity | On the landing page the existing `SoftwareApplication` JSON-LD gains `Organization`+`WebSite` with `sameAs` → GitHub repo, Discord, Ko-fi, live demo. One mark, all its public landing places |
| Content ratio | The docs evolve into the place documentation lives; the landing page never teaches rule models. No new page joins the docs navigation |
| Backlinks | A separate track, done by an editor: submissions to curated lists that link the GitHub repo, not the site. The two changes do not touch each other |
| Spec/plan location | `docs-tech/specs/` and `docs-tech/plans/`, never `docs/` |

## The landing page

Structure of the new `/` (Jekyll source: `docs/index.md`):

* Hero — *"A firewall for your server that cannot lock you out"*; the 120-second
  self-revert as the one-sentence hook; CTA *Install* (`/docs/installation/debian/`)
  and *See it run* (live demo).
* The dashboard screenshot and the apply/revert diagram — the two existing
  visuals, so lookers see the product, not the prose.
* Install strip — Debian/Docker/from-source cards, one command each, reading
  "a few minutes" and emphasizing amd64 + arm64.
* Feature ribbon — ports, blacklist/whitelist, NAT, Docker coexistence, EN/DE,
  light & dark, all links into `/docs/…`.
* A short "who it is for" — a home server, a Pi, a VPS. Privacy and
  self-hosted, GPL-3.0. This mirrors the ko-fi goal copy and the README
  description, so the message is identical in every channel.
* Footer: brand + GitHub · Discord · Ko-fi · the live demo.

The old home page text is not thrown away: its two diagrams and the rule-order
explanation move to a new `/docs/` overview page.

## The docs under /docs

Existing pages and their destinations — nothing is added, nothing is dropped,
only re-homed:

* `docs/architecture.md` → `_docs/architecture.md`
* `docs/configuration.md` → `_docs/configuration.md`
* `docs/security.md` → `_docs/security.md`
* `docs/roadmap.md` → `_docs/roadmap.md`
* `docs/contributing.md` → `_docs/contributing.md`
* `docs/installation/*` → `_docs/installation/*`
* `docs/features/*` → `_docs/features/*`
* new `_docs/index.md` → `/docs/` overview (holds the old home content)

Jekyll configuration addition (in `docs/_config.yml`):

```yaml
collections:
  docs:
    output: true
    permalink: /docs/:path/
```

`baseurl` stays `""`; every internal link already goes through `relative_url`,
so only the URL strings in `_config.yml` (the `nav` table) and in the markdown
change.

## Redirect stubs

For every pre-move URL, one stub page at the old source path (the old top-level
files stay, emptied of content, with only):

```markdown
---
layout: redirect
redirect_to: /docs/…
---
```

Requires the standard GitHub-Pages-supported plugin:

```yaml
# docs/Gemfile
gem "jekyll-redirect-from", "~> 0.16"
```

```yaml
# docs/_config.yml  plugins
- jekyll-redirect-from
```

The sitemap plugin must not list the stubs. `jekyll-sitemap` honors
`sitemap: false` in front matter, so a single config default keeps them out of
`sitemap.xml` (`defaults` scoped to `layout: redirect`), and the plan asserts
the built sitemap contains no non-`/docs/**` content URL.

## Entity schema (landing page)

Replace the current single `SoftwareApplication` block in
`docs/_layouts/default.html` (only emitted when `page.seo == "software"`) with
two scripts: the existing `SoftwareApplication` kept verbatim, plus an
`Organization`/`WebSite` block:

```json
{
  "@context": "https://schema.org",
  "@type": "WebSite",
  "name": "easywall",
  "url": "https://easywall-project.org/",
  "publisher": {
    "@type": "Organization",
    "name": "easywall",
    "url": "https://easywall-project.org/",
    "logo": "https://easywall-project.org/assets/img/og-image.png",
    "sameAs": [
      "https://github.com/jp1337/easywall",
      "https://discord.gg/3zJMvChvUA",
      "https://ko-fi.com/jp1337"
    ]
  }
}
```

## What must be verified

* The collection `permalink: /docs/:path/` really yields `/docs/installation/debian/`
  (assumption to prove first — a spike step at the start of the plan).
* The landing is served at `/`, never prefixed.
* Every old 23 URL serves the redirect stub.
* A link walk of all `relative_url` and bare internal href hrefs finds no dead
  destination after the move.
* `robots.txt`, `sitemap.xml` (now listing `/docs/**` and the landing), the
  single-`<title>` invariant and the `og:image` all survive.
* `/docs/` exists and carries the old content with the diagrams.

## Non-goals

- No subdomain, no second Pages site, no DNS changes.
- No new marketing pages beyond the landing — content additions do not
  happen under `/docs` yet.
- No rebuild of the repo's Go code, no workflow changes.
- No `nft` raw-content planning here; that belongs to the roadmap.