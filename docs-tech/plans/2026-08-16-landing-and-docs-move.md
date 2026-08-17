# Landing Page, Docs under /docs, Brand Entity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `easywall-project.org` into a marketing landing page at `/` with all documentation moved under `/docs/**`, old URLs redirecting, and the brand registered as one entity.

**Architecture:** The single Jekyll site stays one repo + one GitHub Pages deployment + one `CNAME` (`easywall-project.org`). Documentation content moves into a Jekyll collection `_docs` whose `permalink: /docs/:path/` yields the `/docs/**` URLs; the old top-level files become `jekyll-redirect-from` stubs. A new `index.md` at the source root is the landing page. The layout gains a `WebSite`/`Organization` JSON-LD block with `sameAs`.

**Tech Stack:** Jekyll 4.3, `jekyll-seo-tag` (present), `jekyll-sitemap` (present), `jekyll-redirect-from` (added). Ruby 4.0 in CI (`.github/workflows/docs.yml`), building with `bundle exec jekyll build` from the `docs/` directory.

**Spec:** `docs-tech/specs/2026-08-16-landing-and-docs-move.md` — the plan argues from the spec; executors read both.

## Global Constraints

- URL of the landing page is exactly `https://easywall-project.org/` — never prefixed with `/docs`.
- All documentation URLs live under `/docs/**`; the old URLs must not 404 — every old URL serves a `jekyll-redirect-from` stub (`redirect_to:`).
- The repo convention: `docs/` is the whole Jekyll source, `docs-tech/` is never published (guard: `TestTheTechnicalDocsAreNotPublished`). Nothing in this plan touches `docs-tech/` **except** this plan and its spec.
- `baseurl` stays `""` in `docs/_config.yml`. All internal links already go through `relative_url`; nothing builds a by-hand absolute path.
- The existing single-`<title>` invariant and the `og:image` front-matter default must keep holding after every task.
- `jekyll-sitemap` must not list the redirect stubs — each stub carries `sitemap: false` in front matter.
- Exact values inherited from the spec: new pages are `easywall — nftables firewall with a web interface`, the product one-liner is *"…cannot lock you out"*, the 120-second self-revert is the hook.
- Verification runs build the site in a container and grep the output; the canonical build command (run from `docs/`):
  `podman run --rm -v "$PWD":/app:Z -v ewdocs-bundle:/gems -e BUNDLE_PATH=/gems -e BUNDLE_APP_CONFIG=/gems/bundlecfg -w /app docker.io/library/ruby:4.0 bash -lc 'bundle install --quiet && bundle exec jekyll build'`
  (`ewdocs-bundle` is a Podman volume that already holds the gems; `bundle install` is a no-op when they are present.)

---

### Task 1: Jekyll groundwork — collection config, redirect plugin, proof the technique

**Files:**
- Modify: `docs/_config.yml`
- Modify: `docs/Gemfile`
- Move: `docs/architecture.md` → `docs/_docs/architecture.md`
- Create (stub): `docs/architecture.md`

**Interfaces:**
- Consumes: the current single-site layout (collection must coexist with the existing `defaults` block and the `site.nav` table).
- Produces: the proof that (a) a file `docs/_docs/<path>.md` renders at `/docs/<path>/`, and (b) a page with `redirect_to` renders a redirect page; every later task builds on this layout.

- [ ] **Step 1: Add the redirect plugin to the Gemfile**

Add to `docs/Gemfile`:
```ruby
gem "jekyll-redirect-from", "~> 0.16"
```

- [ ] **Step 2: Configure the collection and the plugin**

In `docs/_config.yml`:
- Add `jekyll-redirect-from` to the existing `plugins:` list (after `jekyll-sitemap`).
- Add a new `collections:` block (top level, after `plugins:`):
```yaml
collections:
  docs:
    output: true
    permalink: /docs/:path/
```
Do not remove or reorder anything else in `_config.yml`.

- [ ] **Step 3: Move the canary page into the collection**

```bash
git mv docs/architecture.md docs/_docs/architecture.md
```
Keep its front matter (`layout: default`, `title`, `description`) exactly as it now reads. Do not change the body.

- [ ] **Step 4: Leave a redirect stub at the old path**

Replace the previous content of `docs/architecture.md` with exactly:
```markdown
---
title: Architecture
redirect_to: /docs/architecture/
sitemap: false
---
```
Nothing else in the file.

- [ ] **Step 5: Build and prove the technique**

Run the canonical container build from `docs/`.
Expected:
```text
Generating...
done in ~0.5 seconds
```
Then assert:
```bash
test -f _site/docs/architecture/index.html && echo OK-new
test -f _site/architecture/index.html && echo OK-old
grep -q "http-equiv=\"refresh\"" _site/architecture/index.html && echo REDIRECT
grep -q 'rel="canonical" href="https://easywall-project.org/docs/architecture/"' _site/architecture/index.html && echo CANONICAL
grep -q 'easywall-project.org/architecture/' _site/sitemap.xml && echo "STUB_IN_SITE" || echo "no old-URL in sitemap"
grep -q 'easywall-project.org/docs/architecture/' _site/sitemap.xml && echo "docs url in sitemap"
```
Expected: `docs/architecture/index.html` clamped, old `architecture/` is a refresh+canonical redirect page, the sitemap lists the new URL and does **not** list the old one, `index.html` (the future landing) still builds.

- [ ] **Step 6: Commit**

```bash
git add docs/_config.yml docs/Gemfile docs/architecture.md docs/_docs/architecture.md
git commit -m "docs: collection layout + redirect proof with architecture"
```

---

### Task 2 — Move the remaining content into `_docs/` and stub the old URLs

**Files:**
- Move + stub (old → new → stub):
  - `docs/configuration.md` → `docs/_docs/configuration.md` (+ stub at `docs/configuration.md`)
  - `docs/security.md` → `docs/_docs/security.md` (+ stub)
  - `docs/roadmap.md` → `docs/_docs/roadmap.md` (+ stub)
  - `docs/contributing.md` → `docs/_docs/contributing.md` (+ stub)
  - `docs/installation/{requirements,debian,docker,manual,demo,first-run}.md` → `docs/_docs/installation/…` (+ stubs)
  - `docs/features/{dashboard,apply,ports,blacklist,forwarding,custom-rules,filters,docker,export-import,audit-log,system-settings}.md` → `docs/_docs/features/…` (+ stubs)
- Create: `docs/_docs/index.md` — the `/docs/` overview page

**Interfaces:**
- Consumes: the collection `permalink: /docs/:path/` proven in Task 1; the front matter each moved file already carries.
- Produces: every doc URL at `/docs/**`; a stub page at every old URL; `_docs/index.md` at exactly `/docs/`.
- Redirect map (old URL → new URL), state for each stub:
  `/configuration/` → `/docs/configuration/` · `/security/` → `/docs/security/` · `/roadmap/` → `/docs/roadmap/` · `/contributing/` → `/docs/contributing/` · `/installation/<name>/` → `/docs/installation/<name>/` · `/features/<name>/` → `/docs/features/<name>/`

- [ ] **Step 1: Move each content page and stub its old path**

For **each** file listed above, run:
```bash
git mv docs/<old> docs/_docs/<new>   # preserving subfolders
```
then replace the moved file's former location with a stub:
```markdown
---
title: <Title from the old front matter>
redirect_to: /docs/<new>/
sitemap: false
---
```
Use the exact redirect map above for `/docs/…` paths. Keep every moved file's front matter and body byte-identical except for the stub rewrites.

- [ ] **Step 2: Create the `/docs` overview page**

Create `docs/_docs/index.md`:
```markdown
---
layout: default
title: easywall — how it works and where things are
description: The easywall documentation — install, configure, and understand the nftables firewall that cannot lock you out.
permalink: /docs/
---

The old home page's exposition now lives here: the idea, what the product is
made of, and the rule order are the orientation you need before or after
installing.

## The idea

A firewall you edit over the network can lock you out of the machine you are editing it on. easywall makes that recoverable by default.

{% include themed-figure.html base="/assets/diagrams/apply-flow" ext="svg"
   alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available." %}

Editing changes nothing. Applying changes everything — for 120 seconds. If the new rules cut your connection you cannot click Confirm, and *not* confirming is what brings the old rules back.

## What it is made of

| | |
|---|---|
| **Two processes** | the web interface runs unprivileged and has no path to the kernel |
| **netlink, not a shell** | rules are Go structs, so there is no command line to inject into |
| **Three rule sets** | Staged, Current, Backup — editing and enforcing are separate |
| **Audit log** | every change records what moved and when, in one JSON object per line |
| **Coexists with Docker** | easywall owns `table inet easywall` and touches nothing else |
| **English and German** | switchable in the interface, including before you sign in |

[How it works →]({% link _docs/architecture.md %})

## The order rules are evaluated

The one thing worth knowing before you write a rule. A whitelisted address reaches every port; a blacklisted one is dropped before the whitelist is ever consulted.

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, the IPv6 mode, established connections and ICMP, protection modules, Docker bridge networks, blacklist, whitelist, open ports, custom rules, chain policy drops." %}

## Where to start

| You want to | Go to |
|---|---|
| Install on Debian or Ubuntu — amd64 or arm64 | [.deb package]({% link _docs/installation/debian.md %}) |
| Run it in a container | [Docker]({% link _docs/installation/docker.md %}) |
| Build from source | [Manual install]({% link _docs/installation/manual.md %}) |
| Try it without installing anything | [Live demo](https://easywall.wdkro.de) · [Demo mode]({% link _docs/installation/demo.md %}) |
| Know what the setup page is asking | [First run]({% link _docs/installation/first-run.md %}) |
| Put rules into the kernel, safely | [Applying rules]({% link _docs/features/apply.md %}) |
| Understand the design | [Architecture]({% link _docs/architecture.md %}) · [Security]({% link _docs/security.md %}) |
| See what is planned | [Roadmap]({% link _docs/roadmap.md %}) |
| Contribute or ask | [Contributing]({% link _docs/contributing.md %}) · [Discord]({{ site.discord }}) |
```
> Note: `{% link %}` resolves at build time inside the collection. The overview
> reuses the pre-move content text verbatim; fix any obvious transcription typo
> before committing.

- [ ] **Step 3: Build and prove every old URL redirects**

Run the canonical container build from `docs/`. Then assert:
```bash
for p in configuration security roadmap contributing \
         installation/debian installation/docker installation/manual \
         installation/requirements installation/demo installation/first-run \
         features/dashboard features/apply features/ports features/blacklist \
         features/forwarding features/custom-rules features/filters \
         features/docker features/export-import features/audit-log features/system-settings; do
  test -f "_site/docs/$p/index.html" || echo "MISSING new $p"
  test -f "_site/$p/index.html" || echo "MISSING old $p"
done
grep -l "http-equiv=\"refresh\"" _site/architecture/index.html _site/configuration/index.html >/dev/null
test -f _site/docs/index.html && echo "overview OK"
```
Expected: no MISSING lines, the overview exists at `/docs`, and the old stubs redirect. Also check the sitemap lists `/docs/` and `/docs/…` URLs and none of the old top-level prefixes (`/installation/…`, `/features/…`, `/architecture/…`, `/configuration/…`, `/security/…`, `/roadmap/…`, `/contributing/…`).

- [ ] **Step 4: Commit**

```bash
git add -A docs/
git commit -m "docs: move every doc under /docs, stub the old URLs"
```

---

## Task 3 — The landing page

**Files:**
- Rewrite: `docs/index.md` — the landing at `/`

**Interfaces:**
- Consumes: the design-system classes already in `docs/assets/css/style.css` (used by the current hero markup), the `themed-figure` include, the `{{ site.version }}` / `{{ site.discord }}` config values.
- Produces: the marketing page at `/`, its internal links pointing to `/docs/**`, and the landing that receives the brand schema in Task 5.

- [ ] **Step 1: Write the landing page**

Replace the entire body of `docs/index.md` with:
```markdown
---
layout: default
title: easywall — nftables firewall with a web interface
description: Linux firewall management with a web interface. Go, nftables via netlink, two-process privilege isolation, and an apply that undoes itself if you get it wrong.
seo: software
---

<div class="docs-hero">
  <div class="docs-hero-glow"></div>

  <div class="docs-hero-pill">
    <span class="docs-hero-pill-dot"></span>
    <span>A firewall for your server that<em> cannot lock you out.</em></span>
  </div>

  <h1 class="docs-hero-title">
    Your firewall.<br>
    Your rules.<br>
    <span class="docs-hero-accent">No surprises.</span>
  </h1>

  <p class="docs-hero-desc">
    easywall is a web interface for the Linux nftables firewall. Every apply
    reverts itself after 120 seconds unless you confirm it — so editing your
    firewall over the network can never cut you off. Runs on a home server, a
    Raspberry Pi, or a rented VPS.
  </p>

  <div class="docs-hero-buttons">
    <a href="{{ '/docs/installation/debian/' | relative_url }}" class="btn btn-primary btn-lg">Install</a>
    <a href="https://easywall.wdkro.de" class="btn btn-soft btn-lg">Live demo ↗</a>
    <a href="{{ '/docs/' | relative_url }}" class="btn btn-ghost btn-lg">Documentation</a>
  </div>

  <div class="docs-hero-meta">
    <span><strong>Go 1.26</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>GPL-3.0</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>nftables</strong> via netlink</span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>Argon2id</strong> auth</span>
  </div>
</div>

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/dashboard" ext="png"
     alt="The easywall dashboard: firewall status with acceptance state, pending changes and last apply; tiles counting TCP and UDP ports, blacklist, whitelist, custom rules and forwarding; recent-activity list." %}
  <figcaption>The dashboard answers one question first: what is this firewall enforcing right now?</figcaption>
</figure>

<section class="docs-landstrip">
  <div class="docs-cardgrid">
    <div class="docs-card">
      <h2>Debian / Ubuntu</h2>
      <p class="mono">sudo dpkg -i easywall_amd64.deb</p>
      <p>amd64 and arm64, one package, a systemd unit and HTTPS built in.</p>
      <a href="{{ '/docs/installation/debian/' | relative_url }}">Install →</a>
    </div>
    <div class="docs-card">
      <h2>Docker</h2>
      <p class="mono">docker compose up -d</p>
      <p>Lives in its own nftables table and touches nothing else on the host.</p>
      <a href="{{ '/docs/installation/docker/' | relative_url }}">Run it →</a>
    </div>
    <div class="docs-card">
      <h2>From source</h2>
      <p class="mono">make build && sudo make install</p>
      <p>Go 1.26, no runtime dependencies beyond the kernel.</p>
      <a href="{{ '/docs/installation/manual/' | relative_url }}">Build it →</a>
    </div>
  </div>
</section>

## A firewall you keep your hands on

Everything lives on your own box: ports TCP/UDP, blacklist and whitelist, port
forwarding, custom nftables rules, protection modules, a theft-proof audit log.
No cloud, no account, no telemetry you did not agree to.

* Ports — single or range, with per-rule SSH brute-force routing
* Blacklist & whitelist — IPv4, IPv6, CIDR, evaluated before any port rule
* Port forwarding — NAT redirects with protocol selection
* Custom rules — raw nftables, syntax-checked before it is ever applied
* Docker coexistence — owns `table inet easywall`, touches nothing else
* English & German, light & dark

[What it can do →]({{ '/docs/features/' | relative_url }})

<section class="docs-cta" markdown="0">
  <div class="docs-cta-glow"></div>
  <h2>Try it before you install it</h2>
  <p>A full interface running against an in-memory mock. Nothing reaches a real firewall.</p>
  <div class="docs-cta-buttons">
    <a href="https://easywall.wdkro.de" class="btn btn-primary btn-lg" target="_blank" rel="noopener">Open the demo ↗</a>
    <a href="{{ '/docs/installation/requirements/' | relative_url }}" class="btn btn-soft btn-lg">Requirements</a>
  </div>
  <p class="docs-cta-credentials">Sign in with <code>demo</code> / <code>demo</code></p>
</section>

## Built as open source, for 2026

Go, GPL-3.0, nftables via netlink. The apply can never lock you out. Take a
look at the [architecture]({{ '/docs/architecture/' | relative_url }}) and the
[security model]({{ '/docs/security/' | relative_url }}). Find us on
[Discord]({{ site.discord }}) and [GitHub](https://github.com/jp1337/easywall).
```
> The copy above is the agreed landing. Fix any obvious transcription typing
> error when you paste; do not restructure the sections — the review gates
> check the landing's a map (install cards, features, demo CTA, foot-trails 
> all point into `/docs/**`).

- [ ] **Step 2: Build and assert the landing**

Run the canonical build from `docs/`. Then assert:
```bash
test "$(grep -o '<title>[^<]*</title>' _site/index.html | wc -l)" -eq 1
test -f _site/index.html
grep -q 'href="/docs/installation/debian/"' _site/index.html
grep -q 'href="/docs/"' _site/index.html
grep -q 'href="/docs/features/"' _site/index.html
```
Expected: one title, the landing exists at `/`, and its CTA links target `/docs/**` paths (the `relative_url` filter produces exactly these with `baseurl: ""`).

- [ ] **Step 3: Commit**

```bash
git add docs/index.md
git commit -m "docs: landing page at the root, install-first for self-hosters"
```

---

## Task 4 — Rewrite the navigation and internal links to /docs/**

**Files:**
- Modify: `docs/_config.yml` (`nav` table)
- Modify: every file under `docs/_docs/` whose links point at old URL paths
  (`/installation/`, `/features/`, `/architecture/`, `/configuration/`, `/security/`, `/roadmap/`, `/contributing/`)

**Interfaces:**
- Consumes: the moved set from Task 2 (all pages at `/docs/**`), the landing from Task 3 (its own links already fixed).
- Produces: a site whose internal hyperlinks all resolve — the final link-walk in Task 6 must find zero dead links.

- [ ] **Step 1: Rewrite the nav table**

Replace the entire `nav:` block in `docs/_config.yml` with:
```yaml
nav:
  - title: Home
    path: /
  - title: Docs
    group: true
    children:
      - title: Overview
        path: /docs/
      - title: Installation
        group: true
        children:
          - title: Requirements
            path: /docs/installation/requirements/
          - title: Debian / Ubuntu
            path: /docs/installation/debian/
          - title: Docker
            path: /docs/installation/docker/
          - title: Manual (Source)
            path: /docs/installation/manual/
          - title: Demo Mode
            path: /docs/installation/demo/
          - title: First Run
            path: /docs/installation/first-run/
      - title: Architecture
        path: /docs/architecture/
      - title: Configuration
        path: /docs/configuration/
      - title: Security
        path: /docs/security/
      - title: Features
        group: true
        children:
          - title: Dashboard
            path: /docs/features/dashboard/
          - title: Applying Rules
            path: /docs/features/apply/
          - title: Ports
            path: /docs/features/ports/
          - title: Blacklist & Whitelist
            path: /docs/features/blacklist/
          - title: Port Forwarding
            path: /docs/features/forwarding/
          - title: Custom Rules
            path: /docs/features/custom-rules/
          - title: Firewall Filters
            path: /docs/features/filters/
          - title: Docker Coexistence
            path: /docs/features/docker/
          - title: Export & Import
            path: /docs/features/export-import/
          - title: Audit Log
            path: /docs/features/audit-log/
          - title: System & Network Settings
            path: /docs/features/system-settings/
      - title: Roadmap
        path: /docs/roadmap/
      - title: Contributing
        path: /docs/contributing/
```
> Note: the nav block does not support nested `group: true` under a child in the current layout; if the renderer does not accept the nested group, flatten the Installation and Features children to flat links under the Docs group (paths unchanged) — the paths are the invariant, the grouping is not.

- [ ] **Step 2: Rewrite old URL prefixes in documentation text**

In every file under `docs/_docs/`, replace each internal URL/path that starts with
`/installation/` → `/docs/installation/`
`/features/` → `/docs/features/`
`/architecture/` (in links) → `/docs/architecture/`
`/configuration/` → `/docs/configuration/`
`/security/` → `/docs/security/`
`/roadmap/` → `/docs/roadmap/`
`/contributing/` → `/docs/contributing/`
wherever it appears inside `{{ '…' | relative_url }}`, `{% link '…' %}`, or a bare link target. Do not touch `/assets/…` references (they stay at the top level).

Run:
```bash
rg -l '(/installation/|/features/|/architecture/|/configuration/|/security/|/roadmap/|/contributing/)' docs/_docs --glob '*.md'
```
and edit each listed file accordingly.

- [ ] **Step 3: Build and assert the internal links all resolve**

The final assert is a walk. Run the canonical build, then for each internal `href="/…/"` found in `_site`:
```bash
cd _site
for h in $(grep -rhoE 'href="/[^"#]*"' . | sed -E 's/href="([^"]*)"/\1/' | sort -u); do
  test -f ".$h/index.html" || test -f ".$h" || echo "DEAD: $h"
done
cd ..
```
Expected: no `DEAD:` lines. (External `https://…` hrefs are skipped by the `href="/…"` filter.)

- [ ] **Step 4: Commit**

```bash
git add docs/_config.yml docs/_docs
git commit -m "docs: navigation and internal links point at the /docs tree"
```

---

## Task 5 — Brand entity: sameAs Organization/WebSite schema on the landing

**Files:**
- Modify: `docs/_layouts/default.html` (add a `page.seo == "software"` block)

**Interfaces:**
- Consumes: the existing `SoftwareApplication` block in `default.html` (keep it verbatim), the `{{ site.* }}` values in `_config.yml`.
- Produces: the landing page carries both schemas; docs pages carry neither.

- [ ] **Step 1: Add the second JSON-LD block**

In `docs/_layouts/default.html`, inside the existing `{%- if page.seo == "software" -%}` guard, after the `SoftwareApplication` `</script>`, add:
```html
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@type": "WebSite",
    "name": "easywall",
    "url": "{{ site.url }}/",
    "publisher": {
      "@type": "Organization",
      "name": "easywall",
      "url": "{{ site.url }}/",
      "logo": "{{ '/assets/img/og-image.png' | absolute_url }}",
      "sameAs": [
        "https://github.com/jp1337/easywall",
        "{{ site.discord }}",
        "https://ko-fi.com/jp1337"
      ]
    }
  }
  </script>
```

- [ ] **Step 2: Build and assert both schemas live only on the landing**

Run the canonical build. Then (type-presence gate; do NOT count total `ld+json` blocks — jekyll-seo-tag emits its own WebSite JSON-LD on every page):
```bash
grep -q '"@type": "WebSite"' _site/index.html
grep -q '"@type": "SoftwareApplication"' _site/index.html
grep -q 'ko-fi.com/jp1337' _site/index.html
test "$(grep -c '"@type": "SoftwareApplication"' _site/docs/architecture/index.html)" -eq 0
```
Expected: the brand's SoftwareApplication + WebSite(+ sameAs) appear on the landing and on no doc page.

- [ ] **Step 3: Commit**

```bash
git add docs/_layouts/default.html
git commit -m "docs: register the brand as an entity on the landing page"
```

---

## Task 6 — Full-site verification, sitemap, and standards

**Files:**
- Verify-only (no source change unless a check fails)
- Test commands build the same container

- [ ] **Step 1: Run the complete assertion suite**

Canonical build, then:
```bash
set -e
# every old URL still redirects
for p in architecture configuration security roadmap contributing \
         installation/debian installation/docker installation/manual \
         installation/requirements installation/demo installation/first-run \
         features/dashboard features/apply features/ports features/blacklist \
         features/forwarding features/custom-rules features/filters \
         features/docker features/export-import features/audit-log features/system-settings; do
  test -f "_site/$p/index.html" || { echo "no redirect stub for $p"; exit 1; }
done
# landing is at /, never under /docs
test -f _site/index.html
# every page has exactly one <title>
bad=0
for f in $(find _site -name '*.html'); do
  [ "$(grep -o '<title>' "$f" | wc -l)" = 1 ] || { echo "TITLES != 1: $f"; bad=1; }
done
[ "$bad" = 0 ]
# sitemap: doc URLs in, stub URLs out
grep -q 'https://easywall-project.org/docs/' _site/sitemap.xml
if grep -qE 'https://easywall-project.org/(installation|features|architecture|configuration|security|roadmap|contributing)/' _site/sitemap.xml; then
  echo "stub URL leaked into sitemap"; exit 1
fi
# robots.txt + og-image invariant
grep -q 'Sitemap: https://easywall-project.org/sitemap.xml' _site/robots.txt
grep -q 'property="og:image"' _site/index.html
```
Expected: exit 0. If any assertion fails, fix the underlying source in the same task (it is a verification round trip, not a new feature) and re-run the build until green.

- [ ] **Step 2: Commit any fixes made in Step 1**

```bash
git add -A docs/
git commit -m "docs: final-move verification — zero dead links, one title, clean sitemap"
```
(If nothing changed, `git commit` with `--allow-empty` is not needed — leave well alone and the STEP is satisfied by the passing run.)

- [ ] **Step 3: Report the results**

Report: the assert suite above verbatim with its output, plus `git log --oneline -8`.

---

## Scope guard (do not do these)

- No DNS, no second Pages site, no subdomain.
- No new workflows, no Go changes, no `web/` changes.
- No new docs pages beyond `/docs/` overview and the landing.
- No telemetry or analytics code.