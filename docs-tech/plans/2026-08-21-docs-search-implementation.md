# A search on the documentation site — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A field at the top of the documentation sidebar that finds a config
key, an environment variable or an audit action in the table row that defines it,
and a task like "open a port" in the section that explains it.

**Architecture:** Pagefind indexes the HTML `jekyll build` already produces, so
every table cell is indexed without a single anchor being added by hand. CI runs
the indexer between the build and the Pages upload; the index is not a committed
artefact because it is derived from `_site`, which is not in the repository. The
sidebar renders an inert container, and ~176 KB of engine and UI arrive only when
somebody touches it.

**Tech Stack:** Jekyll 4.3, Pagefind 1.4 (via `npx`), Tailwind v4 (`web/src/docs.css`),
Go for the static guards, Playwright for the rendered checks.

**Source:** `docs-tech/specs/2026-08-21-docs-search-design.md`

## Global Constraints

- **`docs/` is the published site.** Nothing internal goes in it. `docs-tech/` is
  never published — see `TestTheTechnicalDocsAreNotPublished`.
- **The indexing command is exactly this**, verified to yield `Discovered 1
  language: en` and `Indexed 26 pages`:
  `npx pagefind@1 --site _site --glob "docs/**/*.html" --root-selector "main.content" --force-language en`
  — **superseded during the final review.** `pagefind@1` is a floating range no
  tool watched, and the loader depends on the bundle's JS API as well as on the
  index format, so the version is pinned exactly and Renovate watches the line.
  The flags are unchanged. See `docs-tech/dependencies.md`.
- The indexer runs **after** the "site is not empty" check rather than before
  it, also decided in that review: Pagefind reports an empty site as `Found 0
  files matching` and exits 1, which loses the crafted `no landing page was
  built` diagnostic. It still runs before the Pages upload, which is the part
  that matters, and `TestTheSearchIndexIsBuiltBeforeThePagesUpload` holds that.
- **A generated file is rebuilt and diffed, never assumed.** After touching
  `web/src/docs.css`, run `npm run build:docs-css` and **grep the built
  `docs/assets/css/style.css`** for the rule. Tailwind drops rules silently.
- **Verify UI by rendering it**, at 1600 / 900 / 390 px in **both** themes.
- **The sidebar navigation stays the complete scriptless path.** The search field
  is hidden without JavaScript rather than shown and dead.
- Do not touch `go.mod`'s `toolchain` line or any Go version pin.
- Do not add a Ruby plugin. The Gemfile keeps its four gems.

## How to get a built site with an index, locally

Needed by Tasks 1, 4 and 5. There is no Ruby on the development machine, so the
site builds in a container. The vendored bundle's native gems do not match the
image, so `bundle install` has to run inside it.

```bash
S=/tmp/docs-search && rm -rf $S && mkdir -p $S/{src,site}
rsync -a docs/ $S/src/
chmod -R a+rwX $S/src && chmod 777 $S/site
podman run --rm -v $S/src:/src:ro,Z -v $S/site:/out:Z docker.io/jekyll/jekyll:4 \
  sh -c 'cp -a /src /tmp/s && cd /tmp/s && bundle install --quiet && JEKYLL_ENV=production jekyll build -d /out --quiet'
npx pagefind@1 --site $S/site --glob "docs/**/*.html" \
  --root-selector "main.content" --force-language en
cd $S/site && python3 -m http.server 8098   # then http://127.0.0.1:8098/docs/
```

Container-written files need `podman unshare rm -rf $S` to delete.

---

### Task 1: The redirect stubs stop claiming a different language

**Files:**
- Create: `docs/_layouts/redirect.html`
- Test: `internal/web/docs_lang_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing other tasks rely on. Independent of the search; do it first
  because it is the smallest and it removes a defect the search only exposed.

**Why:** 23 of the site's 50 built pages say `<html lang="en-US">` while the
other 27 say `en`. They are the old-path redirect stubs, and the `en-US` is
hardcoded in the plugin's own template —
`docs/vendor/bundle/ruby/3.4.0/gems/jekyll-redirect-from-0.16.0/lib/jekyll-redirect-from/redirect.html`.
`redirect_page.rb:11` sets `"layout" => "redirect"`, so a layout of that name in
the site wins over the built-in one.

- [ ] **Step 1: Write the failing test**

Create `internal/web/docs_lang_test.go`:

```go
package web

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// One language, stated once. The site said two: `en` on the 27 real pages and
// `en-US` on the 23 old-path redirect stubs, because jekyll-redirect-from
// renders those from its own template and that template hardcodes `en-US`.
//
// It surfaced as a search defect — Pagefind read two languages and built two
// indexes that could not see each other — but it is an accessibility defect on
// its own: a screen reader is told the wrong language on 23 of the site's URLs.
//
// Checked against the layouts rather than the built site, because the Go suite
// does not build Jekyll. A layout named `redirect` has to exist at all, or the
// plugin's built-in one is used again and nothing here would notice.
func TestEveryDocsLayoutDeclaresTheSiteLanguage(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	dir := filepath.Join(root, "docs", "_layouts")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	langRe := regexp.MustCompile(`<html[^>]*\slang="([^"]*)"`)
	seen := map[string]string{} // lang -> first layout that used it
	sawRedirect := false

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
			continue
		}
		if e.Name() == "redirect.html" {
			sawRedirect = true
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- a layout this test enumerated
		if err != nil {
			t.Fatal(err)
		}
		m := langRe.FindSubmatch(raw)
		if m == nil {
			continue // a partial layout with no <html> of its own
		}
		lang := string(m[1])
		if _, ok := seen[lang]; !ok {
			seen[lang] = e.Name()
		}
	}

	if !sawRedirect {
		t.Error("docs/_layouts/redirect.html does not exist, so jekyll-redirect-from " +
			"renders the stubs from its own template, which hardcodes lang=\"en-US\"")
	}
	if len(seen) > 1 {
		t.Errorf("the layouts declare %d different languages: %v — Pagefind reads each "+
			"as its own index, and a search in one cannot see the other", len(seen), seen)
	}
	// Read from _config.yml rather than restated, because two sources for one
	// fact is what produced the split in the first place.
	cfg, err := os.ReadFile(filepath.Join(root, "docs", "_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^lang:\s*(\S+)`).FindSubmatch(cfg)
	if m == nil {
		t.Fatal("docs/_config.yml declares no lang:, so there is nothing for the layouts to agree with")
	}
	want := string(m[1])
	if len(seen) == 1 && seen[want] == "" {
		t.Errorf("the layouts declare %v but docs/_config.yml says lang: %q", seen, want)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run TestEveryDocsLayoutDeclaresTheSiteLanguage -v`

Expected: FAIL — `docs/_layouts/redirect.html does not exist, so
jekyll-redirect-from renders the stubs from its own template…`

- [ ] **Step 3: Write the layout**

Create `docs/_layouts/redirect.html`. This is the plugin's own template with one
character class changed, so the stubs keep every behaviour they had — the
canonical link, the script redirect, the meta refresh, the `noindex`, and the
visible fallback link for a browser that honours none of them.

```html
<!DOCTYPE html>
<html lang="en">
  <meta charset="utf-8">
  <title>Redirecting&hellip;</title>
  <link rel="canonical" href="{{ page.redirect.to }}">
  <script>location="{{ page.redirect.to }}"</script>
  <meta http-equiv="refresh" content="0; url={{ page.redirect.to }}">
  <meta name="robots" content="noindex">
  <h1>Redirecting&hellip;</h1>
  <a href="{{ page.redirect.to }}">Click here if you are not redirected.</a>
</html>
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/web/ -run TestEveryDocsLayoutDeclaresTheSiteLanguage -v`
Expected: PASS

- [ ] **Step 5: Prove it against a built site, not only the layout**

Build the site with the recipe above, then:

```bash
grep -rho '<html[^>]*lang="[^"]*"' $S/site --include=*.html | sort | uniq -c
```

Expected: one line, `50 <html lang="en"`. Before this task it was `27 … "en"`
and `23 … "en-US"`.

Also confirm a stub still redirects — the layout is load-bearing, not cosmetic:

```bash
cat $S/site/configuration/index.html
```

Expected: a canonical link, a `location=` script and a meta refresh, all three
pointing at `/docs/configuration/`.

- [ ] **Step 6: Commit**

```bash
git add docs/_layouts/redirect.html internal/web/docs_lang_test.go
git commit -m "fix(docs): the redirect stubs say the language the rest of the site says"
```

---

### Task 2: The search container, and nothing visible without JavaScript

**Files:**
- Create: `docs/_includes/search.html`
- Modify: `docs/_layouts/default.html` — the theme script, and the sidebar between
  `sidebar-header` and `sidebar-nav`
- Modify: `web/src/docs.css`
- Rebuild: `docs/assets/css/style.css`
- Test: `internal/web/docs_search_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: the element `div#docs-search` in the sidebar, and `html[data-js]`
  set before paint. Task 4 mounts `PagefindUI` into that element and relies on
  both names.

- [ ] **Step 1: Write the failing test**

Create `internal/web/docs_search_test.go`:

```go
package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsLayout returns docs/_layouts/default.html.
func docsLayout(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_layouts", "default.html"))
	if err != nil {
		t.Fatalf("read the docs layout: %v", err)
	}
	return string(raw)
}

// The search is a container the layout renders and a script mounts into. Either
// half can go missing without anything failing: the page still renders, there is
// simply no search, and a stylesheet diff cannot see it.
func TestTheDocsSidebarRendersTheSearchContainer(t *testing.T) {
	layout := docsLayout(t)

	for _, want := range []struct{ needle, why string }{
		{`{% include search.html %}`, "the sidebar must include the search partial"},
		{"data-js", "the layout must announce that a script is running, or the field cannot be hidden when one is not"},
	} {
		if !strings.Contains(layout, want.needle) {
			t.Errorf("docs/_layouts/default.html has no %q — %s", want.needle, want.why)
		}
	}

	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_includes", "search.html"))
	if err != nil {
		t.Fatalf("read the search include: %v", err)
	}
	if !strings.Contains(string(raw), `id="docs-search"`) {
		t.Error(`docs/_includes/search.html has no id="docs-search" — PagefindUI mounts by that id`)
	}
}

// A field that cannot search is worse than no field. Without JavaScript there is
// no index to query and no way to get one, so the control is hidden and the
// sidebar navigation stays the complete scriptless path to every page.
//
// Asserted against the *built* stylesheet, because Tailwind is what decides
// whether the rule survives, and it has dropped one before.
func TestTheSearchFieldIsHiddenWithoutJavaScript(t *testing.T) {
	css := docsStylesheet(t)
	if !strings.Contains(css, "#docs-search{display:none}") &&
		!strings.Contains(css, "#docs-search {display: none}") {
		t.Error("the built docs stylesheet does not hide #docs-search by default; " +
			"without a script the page would show a search field that cannot search")
	}
	if !strings.Contains(css, `[data-js] #docs-search`) {
		t.Error("nothing in the built docs stylesheet reveals #docs-search once data-js is set")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run "TestTheDocsSidebarRendersTheSearchContainer|TestTheSearchFieldIsHiddenWithoutJavaScript" -v`
Expected: FAIL on the missing include, and on the missing CSS rules.

- [ ] **Step 3: Create the include**

Create `docs/_includes/search.html`:

```html
{% comment %}
  The container PagefindUI mounts into. It stays empty in the HTML: the engine
  and the UI are ~176 KB together, and a reference page is read far more often
  than it is searched, so nothing is fetched until somebody touches the field.

  The placeholder input below is what they touch. It is replaced by PagefindUI's
  own input on first focus or keystroke — see the loader in default.html.
{% endcomment %}
<div class="docs-search" id="docs-search">
  <label class="sr-only" for="docs-search-input">Search the documentation</label>
  <input type="search" id="docs-search-input" class="docs-search-input"
         placeholder="Search" autocomplete="off" spellcheck="false">
</div>
```

- [ ] **Step 4: Include it, and set `data-js`**

In `docs/_layouts/default.html`, insert the include immediately after the
`</div>` that closes `sidebar-header` and before `<div class="sidebar-nav">`:

```html
      {% include search.html %}

      <div class="sidebar-nav">
```

Then add one line to the existing theme script, beside the `data-theme` line it
already sets (around line 16). It goes there and not in a script at the end of
`<body>` for the reason the app's own comment gives: a flag set late would let
the field render, be seen, and then change.

```js
      document.documentElement.setAttribute('data-theme', t);
      document.documentElement.setAttribute('data-js', 'on');
```

- [ ] **Step 5: Style it, and hide it without a script**

In `web/src/docs.css`, inside the `@layer components` block, add:

```css
  /* Hidden by default and revealed by the flag the head script sets. A search
     field with no engine behind it is a control that lies about what it can do;
     the sidebar navigation is the complete path without JavaScript. */
  #docs-search { display: none; }
  [data-js] #docs-search { display: block; padding: 0 14px 10px; }

  .docs-search-input {
    width: 100%;
    padding: 7px 10px;
    font: inherit;
    font-size: 13px;
    color: var(--text);
    background: var(--surface-2);
    border: 1px solid var(--control-edge);
    border-radius: 8px;
  }

  .docs-search-input::placeholder { color: var(--text-subtle); }

  .docs-search-input:focus-visible {
    outline: 2px solid var(--accent-dim);
    outline-offset: 1px;
    border-color: var(--accent);
  }
```

- [ ] **Step 6: Rebuild the stylesheet and grep the built file**

```bash
npm run build:docs-css
grep -o '#docs-search{[^}]*}' docs/assets/css/style.css
grep -o '\[data-js\] #docs-search{[^}]*}' docs/assets/css/style.css
```

Expected: both print a rule. If either is empty, Tailwind dropped it — do not
proceed on the assumption that the source is what shipped.

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/web/ -run "TestTheDocsSidebar|TestTheSearchField|TestDocsStylesheet" -v`
Expected: PASS

- [ ] **Step 8: Look at it, with and without JavaScript**

Build the site locally, then with Playwright at 1600 / 900 / 390 px in both
themes: the field sits above the navigation, is not clipped, and the page has no
horizontal overflow. Then load one page with `javaScriptEnabled: false` and
confirm **no** field is visible and every sidebar link still works.

- [ ] **Step 9: Commit**

```bash
git add docs/_includes/search.html docs/_layouts/default.html web/src/docs.css \
        docs/assets/css/style.css internal/web/docs_search_test.go
git commit -m "feat(docs): a search field in the sidebar, hidden when no script runs"
```

---

### Task 3: The index is built in CI and asserted

**Files:**
- Modify: `.github/workflows/docs.yml` — both the `build` and the `deploy` job

**Interfaces:**
- Consumes: `_site` from `bundle exec jekyll build`.
- Produces: `docs/_site/pagefind/**` in the Pages artefact. Task 4's loader
  fetches `/pagefind/pagefind-ui.js` from there.

**Why both jobs:** `deploy` because it is what ships; `build` because a pull
request that breaks indexing has to fail on the pull request. `docs.yml`'s own
header comment records what it cost to have the build only on the way out.

- [ ] **Step 1: Add Node and the index step to the `build` job**

In `.github/workflows/docs.yml`, in the `build` job, after the `ruby/setup-ruby`
step and before `Build docs`:

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "24"
```

Then, immediately after the `Build docs` step:

```yaml
      # Pagefind indexes the HTML Jekyll just wrote, so every table cell is
      # searchable without an anchor being added by hand. Scoped on purpose:
      #   --glob          the 26 documentation pages only. Without it the
      #                   marketing landing page competes with the docs and
      #                   ranked for "argon2id" and "port forwarding", and the
      #                   23 old-path redirect stubs get indexed too.
      #   --root-selector the content column only, so the sidebar's own link
      #                   text is not searchable on all 26 pages at once.
      #   --force-language states the site's language once instead of letting it
      #                   be inferred per file. It was inferred as two.
      - name: Build the search index
        working-directory: docs
        run: |
          npx --yes pagefind@1 --site _site \
            --glob "docs/**/*.html" \
            --root-selector "main.content" \
            --force-language en
```

- [ ] **Step 2: Assert the index, in the `build` job**

Immediately after the index step:

```yaml
      # Pagefind exits 0 having indexed nothing if the glob matches nothing, so
      # the count is the check. Derived from the page count rather than a
      # literal, or adding a documentation page breaks the build.
      - name: The search index covers every documentation page
        working-directory: docs
        run: |
          set -euo pipefail
          test -d _site/pagefind || { echo "::error::no search index was built"; exit 1; }

          langs=$(ls _site/pagefind/*.pf_meta | wc -l)
          [ "$langs" -eq 1 ] || {
            echo "::error::$langs language indexes were built, want 1 — a page disagrees about <html lang>, and a search in one index cannot see the other"
            ls _site/pagefind/*.pf_meta
            exit 1
          }

          pages=$(find _docs -name '*.md' | wc -l)
          indexed=$(find _site/pagefind/fragment -type f | wc -l)
          echo "$indexed pages indexed, $pages documentation pages exist"
          [ "$indexed" -eq "$pages" ] || {
            echo "::error::indexed $indexed pages but docs/_docs holds $pages; the glob has stopped matching what it should"
            exit 1
          }
```

- [ ] **Step 3: Repeat both in the `deploy` job**

Add the same `actions/setup-node@v4` step, the same `Build the search index`
step and the same assertion step to the `deploy` job, in the same positions —
after `setup-ruby`, and after `Build docs` respectively. They must run **before**
`Upload Pages artifact`, or the index is not in what gets published.

Do not factor these into a composite action in this task. The two jobs already
carry a duplicated "The site is not empty" check for the reason the comment
beside it gives, and matching the file's existing shape is worth more here than
removing four lines of repetition.

- [ ] **Step 4: Check the workflow parses and pins no Go version**

```bash
python3 -c "import yaml;d=yaml.safe_load(open('.github/workflows/docs.yml'));print('parses; jobs:', list(d['jobs']))"
go test ./internal/shared/ -run TestGoToolchainIsTheSameEverywhere -v
```

Expected: `parses; jobs: ['build', 'deploy']`, and PASS.

- [ ] **Step 5: Prove the assertions locally before trusting them**

The job cannot run without a pull request, so run its logic by hand against a
built site. Both directions:

```bash
# it passes on a correct index
ls $S/site/pagefind/*.pf_meta | wc -l          # expect 1
find $S/site/pagefind/fragment -type f | wc -l # expect 26
find docs/_docs -name '*.md' | wc -l           # expect 26

# it fails on a broken glob
npx pagefind@1 --site $S/site --glob "nothing/**/*.html" --force-language en
find $S/site/pagefind/fragment -type f | wc -l # expect 0, which the check rejects
```

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci(docs): build the search index and assert it covers every page"
```

---

### Task 4: The engine arrives when somebody searches

**Files:**
- Modify: `docs/_layouts/default.html` — a loader script before `</body>`
- Modify: `web/src/docs.css` — map Pagefind's custom properties onto the tokens
- Rebuild: `docs/assets/css/style.css`

**Interfaces:**
- Consumes: `div#docs-search` and `input#docs-search-input` from Task 2;
  `/pagefind/pagefind-ui.js` and `/pagefind/pagefind-ui.css` from Task 3.
- Produces: a working search. Nothing later depends on its internals.

- [ ] **Step 1: Add the loader**

In `docs/_layouts/default.html`, before `</body>`, after the existing scripts:

```html
  <script>
    // The engine and its UI are ~176 KB together, and a reference page is read
    // far more often than it is searched. So the sidebar ships an ordinary input
    // and this swaps it for the real thing the first time somebody means to use
    // it — on focus, which also covers the click and the Tab.
    //
    // PagefindUI renders its own input, so the placeholder is removed once it
    // has mounted; `autofocus` puts the caret back where the visitor left it,
    // and the character they were mid-way through typing is replayed.
    (function () {
      var host = document.getElementById('docs-search');
      var seed = document.getElementById('docs-search-input');
      if (!host || !seed) return;

      var loading = false;
      function load() {
        if (loading) return;
        loading = true;

        var pending = seed.value;

        var css = document.createElement('link');
        css.rel = 'stylesheet';
        css.href = '{{ "/pagefind/pagefind-ui.css" | relative_url }}';
        document.head.appendChild(css);

        var js = document.createElement('script');
        js.src = '{{ "/pagefind/pagefind-ui.js" | relative_url }}';
        js.onload = function () {
          seed.remove();
          var ui = new window.PagefindUI({
            element: '#docs-search',
            bundlePath: '{{ "/pagefind/" | relative_url }}',
            // The page-level excerpt does not reliably contain the match — for
            // "acceptance.duration" it returned prose about a daemon that
            // refuses to start. The sub-result carries the heading it sits
            // under and an excerpt that does contain the term, which is the
            // whole point of searching a reference.
            showSubResults: true,
            showImages: false,
            excerptLength: 20,
            pageSize: 5,
            // Result links carry ?highlight=<term>, which the block below acts
            // on. Landing on the right section with the term marked is the
            // difference between an answer and a page to read again.
            highlightParam: 'highlight'
          });
          if (pending) ui.triggerSearch(pending);
          var real = host.querySelector('input');
          if (real) real.focus();
        };
        js.onerror = function () {
          seed.disabled = true;
          seed.placeholder = 'Search unavailable';
        };
        document.body.appendChild(js);
      }

      seed.addEventListener('focus', load, { once: true });
    })();

    // The other half of highlightParam, and the only reason a docs page loads
    // any search code without being asked: it runs when the visitor arrived
    // from a result, which is exactly when the term is worth marking. Gated on
    // the parameter, so an ordinary page load still fetches nothing.
    if (new URLSearchParams(location.search).has('highlight')) {
      var hl = document.createElement('script');
      hl.src = '{{ "/pagefind/pagefind-highlight.js" | relative_url }}';
      hl.onload = function () {
        new window.PagefindHighlight({ highlightParam: 'highlight' });
      };
      document.body.appendChild(hl);
    }
  </script>
```

`PagefindUI` accepts `element` and exposes `triggerSearch`; `resetStyles`
defaults to true and is deliberately not passed, because Pagefind's own reset is
what keeps its layout from inheriting the docs typography. All four names were
read out of the shipped `pagefind-ui.js`, not from documentation.

- [ ] **Step 2: Map Pagefind's custom properties onto the docs tokens**

In `web/src/docs.css`, inside `@layer components`, add. Styling goes through
Pagefind's own variables rather than its class names: overriding a third-party
component by selector is how it breaks on the component's next minor release.
All of these exist in the shipped `pagefind-ui.css`; the two image-related ones
are omitted because `showImages` is off.

```css
  /* Pagefind's own theming hooks, pointed at the docs tokens so the search
     follows the theme toggle with no rules of our own. */
  #docs-search {
    --pagefind-ui-scale: 0.8;
    --pagefind-ui-primary: var(--accent);
    --pagefind-ui-text: var(--text);
    --pagefind-ui-background: var(--surface);
    --pagefind-ui-border: var(--border);
    --pagefind-ui-tag: var(--surface-2);
    --pagefind-ui-border-width: 1px;
    --pagefind-ui-border-radius: 8px;
    --pagefind-ui-font: var(--font-sans);
  }
```

- [ ] **Step 3: Rebuild and grep**

```bash
npm run build:docs-css
grep -o '\--pagefind-ui-primary:[^;]*' docs/assets/css/style.css
```

Expected: one match. Empty means Tailwind dropped the block and the search will
render in its own default colours in both themes.

- [ ] **Step 4: Verify the acceptance queries against a real index**

Build the site and index it, serve it, and drive it. These four are the spec's
success criteria 1 and 2, and they are what this whole plan is for:

| Type this | Expect |
|---|---|
| `acceptance.duration` | a sub-result under Configuration or Environment Variables whose excerpt contains the term |
| `EASYWALL_WEB_BIND_ADDR` | Environment Variables |
| `rollback_skipped` | Audit Log |
| `how do I open a port` | the Ports page |

And the two negative criteria: no result points at `/` or at a redirect stub.

- [ ] **Step 5: Render it**

1600 / 900 / 390 px, both themes, with results on screen. Check that the results
list does not overflow the sidebar horizontally, that the excerpt text is
readable in both themes, and that the sub-result headings are distinguishable
from the page titles above them. Re-check with JavaScript disabled: no field.

- [ ] **Step 6: Commit**

```bash
git add docs/_layouts/default.html web/src/docs.css docs/assets/css/style.css
git commit -m "feat(docs): load the search engine on first use, themed to the docs tokens"
```

---

### Task 5: Write down what the guards protect

**Files:**
- Modify: `docs-tech/ci-and-release.md` — the `docs.yml` section
- Modify: `docs-tech/invariants.md` — the interface table

- [ ] **Step 1: Document the workflow change**

In `docs-tech/ci-and-release.md`, add this under the `docs.yml` section:

```markdown
#### The search index

Pagefind runs between `jekyll build` and the Pages upload, in both jobs — in
`deploy` because that is what ships, in `build` because a pull request that
breaks indexing has to fail on the pull request.

It is **not** a committed build output, unlike `web/static/style.css` and the
diagrams. It is derived from `_site`, which is not in the repository, so there is
nothing to diff a commit against; its protection is the assertion beside the
"site is not empty" check instead.

Three flags, each preventing something that was actually observed:

| Flag | Without it |
|---|---|
| `--glob "docs/**/*.html"` | the marketing landing page competes with the documentation — it ranked for `argon2id` and for `port forwarding` — and the 23 old-path redirect stubs get indexed as pages |
| `--root-selector "main.content"` | the topbar's text is indexed on all 26 pages at once. `<nav>` and `<footer>` are already excluded by default, so this is about everything else outside the content column |
| `--force-language en` | the language is inferred per file. It was inferred as two, and a search in one index could not see the other half of the site |
```

- [ ] **Step 2: Add the guards to invariants.md**

Add to the *What the interface promises* table:

```markdown
| `TestEveryDocsLayoutDeclaresTheSiteLanguage` | every docs layout says one language, and a `redirect` layout exists at all | The site said two — `en` on 27 pages, `en-US` on the 23 redirect stubs, because jekyll-redirect-from renders those from its own template. Pagefind read two languages and built two indexes that could not see each other; a screen reader was told the wrong language on 23 URLs |
| `TestTheDocsSidebarRendersTheSearchContainer`, `TestTheSearchFieldIsHiddenWithoutJavaScript` | the search container is in the layout, and the built stylesheet hides it until a script says otherwise | Both halves fail silently: the page renders either way, and what is missing is a control, not a page |
```

- [ ] **Step 3: Run everything**

```bash
go test ./internal/... && make lint
```

Note: `golangci-lint` from Homebrew is built against an older Go than this
repository targets and cannot load its config. Use `$(go env GOPATH)/bin/golangci-lint`.

- [ ] **Step 4: Commit**

```bash
git add docs-tech/ci-and-release.md docs-tech/invariants.md
git commit -m "docs(tech): the search index, and the two guards around it"
```

---

## Notes for the implementer

- **`--root-selector "main.content"` is doing more than trimming.** Pagefind
  excludes `<nav>` and `<footer>` by default, which already drops the sidebar and
  the per-page contents. The root selector is what stops the topbar and anything
  else outside the content column from being indexed on all 26 pages at once.
- **`excerptLength: 20` is words, not characters.**
- **The two indexes were invisible.** Before Task 1, a search from a page whose
  `lang` was `en` silently queried only the `en` index. It worked, returned
  plausible results, and could not see half the URLs. That is the failure mode to
  keep in mind when touching any of the three flags.
- **26 is not a magic number.** `find docs/_docs -name '*.md' | wc -l` is the
  source; the CI check derives it, and every assertion in this plan that mentions
  26 is quoting today's value of that command, not a constant.
- **Do not add weighting yet.** With the index scoped, the ordering was correct
  in every query tried. `data-pagefind-weight` before anybody needs it is a knob
  nobody will understand later — the spec rules it out on purpose.
