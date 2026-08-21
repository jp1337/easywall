# The interface speaks French — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A third language ships, and the fourth need not come from us — a
translation may be incomplete, what is missing renders English, and both facts
are visible rather than hidden.

**Architecture:** The per-key fallback already exists: `langCandidates` ends with
the default language and `en`, and go-i18n resolves each message ID down that
list. What is missing is the *accounting*. This adds a status file, guards that
enumerate `locales/*.json` instead of naming two languages, a coverage report, a
reviewed/unreviewed state, and a switcher that survives more than two languages.

**Tech Stack:** Go, go-i18n v2, `html/template`, Tailwind v4, Playwright.

**Source:** `docs-tech/specs/2026-08-16-roadmap-2.7-to-3.0.md`, section
*2.9 — The interface speaks French, and the next language need not come from us*.

## Global Constraints

- **`en` and `de` keep the hard parity rule.** Every other language may have
  gaps; a missing key renders the English string and is **reported**, never
  hidden.
- **The switcher must work without JavaScript.** Already written above
  `{{define "langswitch"}}`: *an operator who cannot read the interface should
  not also need JavaScript to fix that.* This is not negotiable.
- **A language nobody has reviewed says so** — in the switcher and in the report.
- Every visible string goes through the catalogue. A new string means a new key
  in `en.json` **and** `de.json`; `fr.json` may lag.
- UI changes are verified by rendering at **1600 / 900 / 390 px in both themes**,
  never by reading CSS.
- Screenshots under `docs/assets/img/screens/` are re-taken in the same change
  that alters what they show.
- Do not touch `go.mod`'s `toolchain` line or any Go version pin.

---

### Task 1: Every locale file is inventoried, and the guards stop naming two languages

**Files:**
- Create: `locales/status.json`
- Create: `internal/web/locales.go`
- Modify: `internal/web/i18n_coverage_test.go` — lines 70, 86, 103, 156, 346
- Test: `internal/web/locales_test.go` (create)

**Interfaces:**
- Produces:
  - `type LocaleStatus struct { Reviewed bool \`json:"reviewed"\` }`
  - `func LoadLocaleStatus(dir string) (map[string]LocaleStatus, error)`
  - `func LocaleCodes(dir string) ([]string, error)` — sorted base codes from `dir/*.json`, excluding `status.json`
  - `const StrictLocales = "en,de"` is **not** used; the strict set is the exported
    `var StrictLangs = []string{"en", "de"}`

**Why:** `TestLocaleFilesAreAtParity` compares `en` against `de` and nothing else.
`localeIDs(t, lang)` is only ever called with those two. A `fr.json` gets no
parity check, no empty-translation check, no duplicate-ID check. CONTRIBUTING.md
tells a contributor those tests *"will tell you about anything you missed"* —
which is false for every language that is not German.

- [ ] **Step 1: Write the failing test**

Create `internal/web/locales_test.go`:

```go
package web

import (
	"path/filepath"
	"testing"
)

func TestLocaleCodes_FindsEveryLocaleFileAndNotTheStatusFile(t *testing.T) {
	dir := localesDir(t)
	codes, err := LocaleCodes(dir)
	if err != nil {
		t.Fatalf("LocaleCodes: %v", err)
	}
	want := map[string]bool{"en": true, "de": true}
	for _, c := range codes {
		if c == "status" {
			t.Error("status.json was treated as a language")
		}
		delete(want, c)
	}
	if len(want) > 0 {
		t.Errorf("LocaleCodes missed %v", want)
	}
}

// Every language file has a status entry, and every status entry has a file.
// Without this, a language ships with no statement about whether a human ever
// read it — which is the entire point of the state.
func TestEveryLocaleHasAStatusEntry(t *testing.T) {
	dir := localesDir(t)
	codes, err := LocaleCodes(dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		t.Fatalf("LoadLocaleStatus: %v", err)
	}
	for _, c := range codes {
		if _, ok := status[c]; !ok {
			t.Errorf("locales/%s.json has no entry in locales/status.json", c)
		}
		delete(status, c)
	}
	for c := range status {
		t.Errorf("locales/status.json names %q, which has no locale file", c)
	}
}

func TestStrictLanguagesAreReviewed(t *testing.T) {
	status, err := LoadLocaleStatus(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range StrictLangs {
		if !status[l].Reviewed {
			t.Errorf("%s is a strict language and must be marked reviewed", l)
		}
	}
}

var _ = filepath.Join // keep the import if the helpers move
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run "TestLocaleCodes|TestEveryLocaleHasAStatusEntry|TestStrictLanguagesAreReviewed" -v`
Expected: FAIL — `undefined: LocaleCodes`.

- [ ] **Step 3: Write the implementation**

Create `locales/status.json`:

```json
{
  "en": { "reviewed": true },
  "de": { "reviewed": true }
}
```

Create `internal/web/locales.go`:

```go
package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StrictLangs are the languages that must stay at exact parity with each other.
// Everything else may have gaps: the missing key renders English, and the gap is
// reported rather than hidden.
//
// Two and not more on purpose. The roadmap's reasoning: with eight languages an
// exact-parity rule blocks every feature from here on until all eight are
// translated — or the rule becomes a sham, because somebody pastes the English
// text in to make the test green. A French interface displaying English while
// the project claims to support French is worse than no translation.
var StrictLangs = []string{"en", "de"}

// statusFile is not a language.
const statusFile = "status.json"

// LocaleStatus records what is known about a translation beyond its contents.
type LocaleStatus struct {
	// Reviewed is whether a human who speaks the language has read it. A draft
	// says so in the switcher; claiming a language nobody checked is the same
	// class of untruth as a stale screenshot.
	Reviewed bool `json:"reviewed"`
}

// LocaleCodes lists the language codes dir holds a catalogue for, sorted.
func LocaleCodes(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read locales dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == statusFile {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(out)
	return out, nil
}

// LoadLocaleStatus reads locales/status.json.
//
// A missing file is not an error: it yields an empty map, and an absent entry
// counts as *not reviewed*. That is the understating direction — claiming a
// language nobody checked is the failure worth avoiding — and it is also what
// lets the existing tests keep using bundleWith, which writes a catalogue into
// a temp directory that has no status file in it.
func LoadLocaleStatus(dir string) (map[string]LocaleStatus, error) {
	raw, err := os.ReadFile(filepath.Join(dir, statusFile)) // #nosec G304 -- the locales dir this process was configured with
	if errors.Is(err, os.ErrNotExist) {
		return map[string]LocaleStatus{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", statusFile, err)
	}
	var out map[string]LocaleStatus
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", statusFile, err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/web/ -run "TestLocaleCodes|TestEveryLocaleHasAStatusEntry|TestStrictLanguagesAreReviewed" -v`
Expected: PASS.

- [ ] **Step 5: Make the existing guards enumerate**

In `internal/web/i18n_coverage_test.go`, replace every hardcoded language list
with `LocaleCodes(localesDir(t))`:

- **line 70** (`TestTemplatesOnlyUseTranslatedKeys`): build the `langs` map by
  ranging over the codes rather than writing `{"en": ..., "de": ...}`.
- **line 86**: range over the codes.
- **line 103** (`TestLocaleFilesAreAtParity`): keep the *hard* comparison, but
  between the members of `StrictLangs` — read them from that variable, not as
  literals, so the rule and the code that enforces it cannot disagree.
- **line 156** (`TestGermanTranslationsAreNotCopiedEnglish`): rename to
  `TestTranslationsAreNotCopiedEnglish` and run it for every code except `en`.
  A language that is not reviewed is exempt from this one — an in-house draft
  legitimately contains untranslated fragments, and that is what its status
  records. Skip with `t.Logf` naming the language, so the exemption is visible
  in the output rather than silent.
- **line 346** (`TestClientStringsCoverWhatAppJSAsksFor`): range over the codes,
  but only require full coverage for `StrictLangs`; for the others report what
  is missing without failing.

- [ ] **Step 6: Run the whole web suite**

Run: `go test ./internal/web/`
Expected: PASS. Nothing changes for `en`/`de` — the point of this step is that
the tests now *describe* a rule instead of two file names.

- [ ] **Step 7: Commit**

```bash
git add locales/status.json internal/web/locales.go internal/web/locales_test.go \
        internal/web/i18n_coverage_test.go
git commit -m "test(i18n): the guards enumerate the locale files instead of naming two"
```

---

### Task 2: The coverage report

**Files:**
- Create: `internal/web/coverage.go`
- Test: `internal/web/coverage_test.go` (create)

**Interfaces:**
- Consumes: `LocaleCodes`, `LoadLocaleStatus`, `StrictLangs` (Task 1).
- Produces:
  - `type Coverage struct { Lang string; Have, Total int; Missing []string; Reviewed bool }`
  - `func (c Coverage) Percent() int` — integer percent, rounded down, `100` only
    when `Missing` is empty
  - `func LocaleCoverage(dir string) ([]Coverage, error)` — one entry per code,
    sorted by `Lang`, measured against `en.json`'s ID set

- [ ] **Step 1: Write the failing test**

Create `internal/web/coverage_test.go`:

```go
package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocaleCoverage_EnglishIsTheYardstickAndIsComplete(t *testing.T) {
	cov, err := LocaleCoverage(localesDir(t))
	if err != nil {
		t.Fatalf("LocaleCoverage: %v", err)
	}
	if len(cov) == 0 {
		t.Fatal("no coverage reported")
	}
	for _, c := range cov {
		if c.Lang != "en" {
			continue
		}
		if len(c.Missing) != 0 || c.Percent() != 100 {
			t.Errorf("en is the yardstick and must be 100%%, got %d%% missing %v",
				c.Percent(), c.Missing)
		}
	}
}

// Percent must never round a gap up to a full hundred. "100%" is a promise, and
// a language one key short has not kept it.
func TestCoveragePercent_NeverRoundsAGapUpToAHundred(t *testing.T) {
	c := Coverage{Have: 9999, Total: 10000, Missing: []string{"one_key"}}
	if c.Percent() == 100 {
		t.Error("a language one key short reported 100%")
	}
	full := Coverage{Have: 10, Total: 10}
	if full.Percent() != 100 {
		t.Errorf("a complete language reported %d%%", full.Percent())
	}
}

// Pasting the English string must not raise the number.
//
// The fallback renders English for a missing key anyway, so a copied line
// changes nothing a user sees — it only changes what the report claims. The
// roadmap named this exact failure when it argued against exact parity.
func TestCoverage_CopiedEnglishDoesNotCount(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("en.json", `[{"id":"a","translation":"Apply"},{"id":"b","translation":"Cancel"}]`)
	write("xx.json", `[{"id":"a","translation":"Appliquer"},{"id":"b","translation":"Cancel"}]`)
	write("status.json", `{"en":{"reviewed":true},"xx":{"reviewed":false}}`)

	cov, err := LocaleCoverage(dir)
	if err != nil {
		t.Fatalf("LocaleCoverage: %v", err)
	}
	for _, c := range cov {
		if c.Lang != "xx" {
			continue
		}
		if c.Have != 1 {
			t.Errorf("Have = %d, want 1 — \"Cancel\" is the English string verbatim", c.Have)
		}
		if len(c.Missing) != 1 || c.Missing[0] != "b" {
			t.Errorf("Missing = %v, want [b]", c.Missing)
		}
	}
}

func TestLocaleCoverage_ReportsTheReviewedFlag(t *testing.T) {
	cov, err := LocaleCoverage(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cov {
		if c.Lang == "de" && !c.Reviewed {
			t.Error("de is marked unreviewed in the coverage report")
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run "TestLocaleCoverage|TestCoveragePercent" -v`
Expected: FAIL — `undefined: LocaleCoverage`.

- [ ] **Step 3: Write the implementation**

Create `internal/web/coverage.go`:

```go
package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The fallback hides gaps by design: a key missing from fr.json renders the
// English string, and the page looks finished. That is the right behaviour for
// the operator in front of it and the wrong one for everybody else — a gap
// nobody can see is indistinguishable from no gap. This is the accounting that
// makes it visible again.
//
// English is the yardstick because it is the language every other one falls
// back to; a key that is not in en.json cannot be rendered by anybody.

// Coverage is one language measured against English.
type Coverage struct {
	Lang     string
	Have     int
	Total    int
	Missing  []string
	Reviewed bool
}

// Percent is how much of English this language covers.
//
// It never returns 100 while anything is missing. "100%" is a promise, and a
// language one key short of 10,000 has not kept it — integer division would
// have rounded that up and said so.
func (c Coverage) Percent() int {
	if c.Total == 0 {
		return 0
	}
	p := c.Have * 100 / c.Total
	if p >= 100 && len(c.Missing) > 0 {
		return 99
	}
	return p
}

// LocaleCoverage measures every catalogue in dir against en.json.
func LocaleCoverage(dir string) ([]Coverage, error) {
	english, err := localeTranslations(dir, "en")
	if err != nil {
		return nil, err
	}
	codes, err := LocaleCodes(dir)
	if err != nil {
		return nil, err
	}
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		return nil, err
	}

	var out []Coverage
	for _, code := range codes {
		tr, err := localeTranslations(dir, code)
		if err != nil {
			return nil, err
		}
		c := Coverage{Lang: code, Total: len(english), Reviewed: status[code].Reviewed}
		for id, en := range english {
			t, present := tr[id]
			switch {
			case !present:
				c.Missing = append(c.Missing, id)
			case code != "en" && t == en:
				// Byte-identical to English is not a translation. It renders
				// exactly as the fallback would, so counting it would let
				// somebody raise the number by pasting — which is the failure
				// the roadmap named: the rule becoming a sham because the
				// English text was copied in to make a test green.
				//
				// A handful of terms are legitimately the same in both
				// languages ("Docker", "IP"), so this understates coverage by
				// a percent or two. That is the right direction to be wrong
				// in: overstating is what misleads.
				c.Missing = append(c.Missing, id)
			default:
				c.Have++
			}
		}
		sort.Strings(c.Missing)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lang < out[j].Lang })
	return out, nil
}

// localeTranslations reads one catalogue as id -> translation, keeping only the
// entries that actually carry text. An empty translation is not a translation,
// so it does not count towards coverage — otherwise a file of 461 blanks would
// report 100%.
func localeTranslations(dir, code string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, code+".json")) // #nosec G304 -- a code LocaleCodes listed
	if err != nil {
		return nil, fmt.Errorf("read %s.json: %w", code, err)
	}
	var entries []struct {
		ID          string `json:"id"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse %s.json: %w", code, err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Translation != "" {
			out[e.ID] = e.Translation
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/web/ -run "TestLocaleCoverage|TestCoveragePercent" -v`
Expected: PASS.

- [ ] **Step 5: Print the report in the test run**

Add a test named `TestLocaleCoverageReport` that never fails but prints one line
per language via `t.Logf` — `fr  86%  (63 missing, unreviewed)`. Run with `-v` it
is the report; run without, it costs nothing. This is deliberately not a separate
tool: a report you have to remember to run is a report nobody runs.

- [ ] **Step 6: Commit**

```bash
git add internal/web/coverage.go internal/web/coverage_test.go
git commit -m "feat(i18n): a coverage report, because the fallback hides gaps by design"
```

---

### Task 3: The switcher becomes a select that survives eleven languages

**Files:**
- Modify: `web/templates/base.html` — `{{define "langswitch"}}` at line 47, and the
  nonced head script that sets `data-theme` (around line 20)
- Modify: `web/src/app.css` — the `.lang-switch` / `.lang-options` / `.lang-option` rules
- Test: `internal/web/handler_language_test.go` (extend), `scripts/ui-check.mjs` (extend)

**Interfaces:**
- Consumes: `PageData.Langs []languageOption` with fields `Code`, `Label`,
  `Current` — unchanged.
- Produces: no Go API change. The four include sites (`base.html:161`,
  `login.html:62`, `firstrun.html:221`, `login_verify.html:61`) keep calling
  `{{template "langswitch" .}}` and need no edit.

**Why:** two chip buttons in a 240 px sidebar do not hold eleven languages. The
endonyms of the languages on the roadmap come to roughly 720 px, so
`.lang-options` wraps into a four- or five-row block at the foot of a sidebar
that then scrolls — in order to let somebody who cannot read the interface change
its language. A `<select>` also gives the native OS picker at 390 px.

- [ ] **Step 1: Write the failing test**

Create `internal/web/langswitch_test.go`. The assertion is about *markup*, so it
reads the template rather than standing up a server — the same approach
`TestTemplateClassesExistInStylesheet` and `TestMarkupStringsAreRenderedThroughRichText`
already take in this package.

```go
package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// langswitchBlock returns the body of {{define "langswitch"}} from base.html.
func langswitchBlock(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t)) // localesDir walks up to <repo>/locales
	raw, err := os.ReadFile(filepath.Join(root, "web", "templates", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	start := strings.Index(body, `{{define "langswitch"}}`)
	if start < 0 {
		t.Fatal(`base.html has no {{define "langswitch"}}`)
	}
	rest := body[start:]
	end := strings.Index(rest, "{{end}}\n{{end}}")
	if end < 0 {
		t.Fatal("could not find the end of the langswitch block")
	}
	return rest[:end]
}

// The switcher has to work with JavaScript switched off. The reason is written
// above {{define "langswitch"}} and predates this change: an operator who
// cannot read the interface should not also need JavaScript to fix that.
func TestLangSwitch_WorksWithoutJavaScript(t *testing.T) {
	block := langswitchBlock(t)

	for _, want := range []struct{ needle, why string }{
		{"<select", "the switcher must be a select; chip buttons do not hold eleven languages"},
		{`name="lang"`, "the select must post the lang field the handler reads"},
		{`type="submit"`, "a submit button must be in the markup, not added by a script"},
		{"lang-submit", "the submit button needs the class the stylesheet hides when JS is on"},
		{`method="POST"`, "the form writes a cookie, so it must not be a GET"},
	} {
		if !strings.Contains(block, want.needle) {
			t.Errorf("langswitch has no %q — %s", want.needle, want.why)
		}
	}

	// The old chip markup must be gone, or both render and the sidebar grows.
	if strings.Contains(block, "lang-option") && !strings.Contains(block, "lang-options-legacy") {
		t.Error("the chip-button markup is still present alongside the select")
	}
}

// The flag that hides the submit button is set in the head, not in app.js.
// app.js loads at the end of <body>, so the button would render, be seen, and
// then vanish — a visible flicker on every page load.
func TestJSFlagIsSetInTheHeadNotInAppJS(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	base, err := os.ReadFile(filepath.Join(root, "web", "templates", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(base), "data-js") {
		t.Error("base.html never sets data-js; the no-JS submit button would always show")
	}

	appJS, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(appJS), `setAttribute('data-js'`) {
		t.Error("app.js sets data-js; it loads at the end of body, so the button would flicker")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run TestLangSwitch_SubmitsWithoutJavaScript -v`
Expected: FAIL — the markup still renders chip buttons.

- [ ] **Step 3: Rewrite the template**

Replace the body of `{{define "langswitch"}}` in `web/templates/base.html`:

```html
{{if .Langs}}
<form method="POST" action="/language" class="lang-switch">
  <input type="hidden" name="return" value="{{.Path}}">
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3M3.6 9h16.8M3.6 15h16.8"/></svg>
  <label class="sr-only" for="lang-select">{{T "nav_language"}}</label>
  <select id="lang-select" name="lang" class="lang-select">
    {{range .Langs}}
    <option value="{{.Code}}"{{if .Current}} selected{{end}}>{{.Label}}</option>
    {{end}}
  </select>
  <button type="submit" class="lang-submit">{{T "lang_apply"}}</button>
</form>
{{end}}
```

`lang_apply` is a new key — add it to `en.json` and `de.json` in Task 4.

- [ ] **Step 4: Hide the button only when JavaScript is present**

In the **existing nonced head script** in `base.html` — the one that already sets
`data-theme` — add `document.documentElement.setAttribute('data-js', 'on')`.

Not in `app.js`: that file loads at the end of `<body>`, so the button would
render, be seen, and then vanish. In `web/src/app.css`:

```css
[data-js="on"] .lang-submit { display: none; }
```

And in `app.js`, submit the form on `change`:

```js
const langSelect = document.getElementById('lang-select');
if (langSelect) langSelect.addEventListener('change', () => langSelect.form.submit());
```

- [ ] **Step 5: Restyle**

Replace the `.lang-option` chip rules in `web/src/app.css` with `.lang-select`
rules that fit the sidebar's 240 px and match the design system's form controls —
read `DESIGN.md` for the control tokens rather than inventing values. Keep
`.lang-switch`'s existing layout and icon placement.

- [ ] **Step 6: Rebuild the stylesheet and check the rule survived**

```bash
npm run build:css
grep -c "lang-select" web/static/style.css
```
Expected: a non-zero count. Tailwind drops rules silently; a green build is not
proof the rule shipped.

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/web/`
Expected: PASS. `TestTemplateClassesExistInStylesheet` covers the new classes;
if it fails, the stylesheet was not rebuilt or the class name differs.

- [ ] **Step 8: Look at it**

Serve demo mode and view `/login` and the dashboard at **1600, 900 and 390 px in
both themes**, with JavaScript on and off. Confirm: the sidebar does not scroll
because of the switcher; at 390 px the native picker opens; with JavaScript off
the button is visible and the form posts.

- [ ] **Step 9: Commit**

```bash
git add web/templates/base.html web/src/app.css web/static/style.css \
        web/static/app.js internal/web/handler_language_test.go
git commit -m "feat(i18n): the language switcher becomes a select that holds more than two"
```

---

### Task 4: An unreviewed language says so

**Files:**
- Modify: `locales/en.json`, `locales/de.json` — add `lang_apply` and `lang_unreviewed`
- Modify: `internal/web/server.go` — `languageOption` gains `Reviewed bool`; `languageOptions` fills it
- Modify: `web/templates/base.html` — the `<option>` label
- Test: `internal/web/handler_language_test.go` (extend)

**Interfaces:**
- Consumes: `LoadLocaleStatus` (Task 1).
- Produces: `languageOption` gains `Reviewed bool`. `Label` stays the endonym;
  the marker is appended in the template, not baked into `Label`, so the report
  and the switcher cannot disagree about what the language is called.

- [ ] **Step 1: Write the failing test**

```go
// A draft is offered, and says it is a draft. Asserted on the Reviewed field
// rather than on rendered text: the marker's wording is itself a translation
// and will change, but what it reports must not.
func TestLanguageOptions_MarkAnUnreviewedLanguage(t *testing.T) {
	s, dir := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"fr.json": `[{"id":"language_name","translation":"Français"}]`,
		"status.json": `{"en":{"reviewed":true},"fr":{"reviewed":false}}`,
	})
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		t.Fatalf("LoadLocaleStatus: %v", err)
	}
	s.localeStatus = status

	req := httptest.NewRequest("GET", "/dashboard", nil)
	got := map[string]bool{}
	for _, o := range s.languageOptions(req, "en") {
		got[o.Code] = o.Reviewed
	}
	if !got["en"] {
		t.Error("en came back unreviewed")
	}
	if got["fr"] {
		t.Error("fr came back reviewed; status.json says it is not")
	}
}

// An absent status file must not turn every language into a draft-free claim.
// bundleWith writes no status.json, which is exactly the case every other test
// in this file hits.
func TestLanguageOptions_NoStatusFileMeansUnreviewed(t *testing.T) {
	s, dir := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
	})
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		t.Fatalf("a missing status.json must not be an error: %v", err)
	}
	s.localeStatus = status
	req := httptest.NewRequest("GET", "/dashboard", nil)
	for _, o := range s.languageOptions(req, "en") {
		if o.Reviewed {
			t.Errorf("%s reported reviewed with no status file", o.Code)
		}
	}
}
```

Note that `bundleWith` writes whatever files it is given into the temp
directory, so passing `status.json` in the map is enough — it needs no change.
`Server` gains an unexported `localeStatus map[string]LocaleStatus` field, set
from `LoadLocaleStatus` at construction in `NewServer` and by hand in these two
tests.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web/ -run TestLanguageOptions_MarkAnUnreviewedLanguage -v`
Expected: FAIL — `languageOption` has no `Reviewed` field.

- [ ] **Step 3: Implement**

Add `Reviewed bool` to `languageOption` in `internal/web/server.go`. Load the
status once at server construction — beside the bundle — rather than per request,
and have `languageOptions` read it. A language absent from `status.json` counts
as **not** reviewed: the safe direction is to understate.

Add to `locales/en.json`:
- `lang_apply` → `Apply`
- `lang_unreviewed` → `unreviewed`

and to `locales/de.json`:
- `lang_apply` → `Übernehmen`
- `lang_unreviewed` → `ungeprüft`

In `base.html`, render the marker after the endonym:

```html
    <option value="{{.Code}}"{{if .Current}} selected{{end}}>{{.Label}}{{if not .Reviewed}} — {{T "lang_unreviewed"}}{{end}}</option>
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/web/`
Expected: PASS, including `TestLocaleFilesAreAtParity` — both new keys are in
both strict languages.

- [ ] **Step 5: Commit**

```bash
git add locales/en.json locales/de.json internal/web/server.go \
        web/templates/base.html internal/web/handler_language_test.go
git commit -m "feat(i18n): a language nobody has reviewed says so in the switcher"
```

---

### Task 5: The reviewer's list of security-bearing sentences

**Files:**
- Create: `docs-tech/i18n-review.md`
- Test: `internal/web/i18n_review_test.go` (create)

**Why:** a reviewer who starts at key 1 of 461 reaches the sentences that matter
late and tired. The roughly thirty that carry security meaning — evaluation
order, what a block does, what an acceptance window promises, what panic mode
does and does not end — are collected so they are read first.

- [ ] **Step 1: Write the list**

Create `docs-tech/i18n-review.md`. Go through `locales/en.json` and collect the
IDs whose text a mistranslation would make *dangerous*, not merely wrong. Group
them: rule evaluation order, the acceptance window's promise, blacklist vs
whitelist precedence, panic mode and recovery, the second factor and recovery
codes, demo mode. Give each its English text.

State the rule at the top: **a translator may rephrase these, but may not change
what they claim.** An acceptance window that "keeps" changes in one language and
"applies" them in another is a different product.

- [ ] **Step 2: Write the test that keeps the list honest**

Create `internal/web/i18n_review_test.go` with a test that every ID named in
`docs-tech/i18n-review.md` exists in `en.json`. A list naming keys that no longer
exist sends a reviewer looking for text that is not there.

- [ ] **Step 3: Run it**

Run: `go test ./internal/web/ -run TestReviewListNamesRealKeys -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add docs-tech/i18n-review.md internal/web/i18n_review_test.go
git commit -m "docs(tech): the sentences a translation reviewer reads first"
```

---

### Task 6: fr.json

**Files:**
- Create: `locales/fr.json`
- Modify: `locales/status.json`

- [ ] **Step 1: Draft the translation**

Copy `locales/en.json` to `locales/fr.json`, keep every `id` unchanged, and
translate every `translation`.

Three forms must survive, exactly as `CONTRIBUTING.md` describes them:

| In the message | Must stay | Note |
|---|---|---|
| `` `443` `` | backticks | literals: ports, CIDRs, nftables statements — never translated |
| `*before*` | asterisks | emphasis that carries meaning |
| `{}` | one per link | **placed in French word order**, not English |

`language_name` becomes `Français` — the language's own name, not `French`.

Translate the IDs in `docs-tech/i18n-review.md` **first** and with the most care;
they are the sentences where a wrong word changes what the firewall promises.

- [ ] **Step 2: Register it as unreviewed**

In `locales/status.json`:

```json
  "fr": { "reviewed": false }
```

- [ ] **Step 3: Run the suite**

Run: `go test ./internal/web/ -v -run "TestLocale|TestTranslations|TestClientStrings|TestLanguageOptions"`
Expected: PASS. `TestLocaleCoverageReport` prints French's percentage; the
parity rule does not apply to it, and `TestTranslationsAreNotCopiedEnglish`
skips it while it is unreviewed, naming it in the output.

- [ ] **Step 4: Look at it**

Render the interface in French at 1600 and 390 px, both themes. French runs
roughly 15–20 % longer than English: check the sidebar, the buttons, the stat
tiles and the table headers for clipping and for text that wraps into two lines
where English fits one.

- [ ] **Step 5: Commit**

```bash
git add locales/fr.json locales/status.json
git commit -m "feat(i18n): a French interface, shipped marked as unreviewed"
```

---

### Task 7: Everything that says the interface speaks two languages

**Files:**
- Modify: `CLAUDE.md` line 21, `docs/_docs/index.md` line 26,
  `docs/_docs/installation/first-run.md` line 94 (the screenshot `alt` text),
  `CONTRIBUTING.md` lines 79–90, `DESIGN.md` around line 1061
- Modify: `docs/_docs/roadmap.md`, `README.md` — the renumbering the roadmap
  amendment requires
- Test: `internal/web/i18n_docs_test.go` (create)

- [ ] **Step 1: Write the test that stops the claim drifting again**

Create `internal/web/i18n_docs_test.go`:

```go
package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Four documents named the two languages that existed when they were written.
// The endonym is the thing that dates: a page saying "English and German" while
// the switcher offers three is the same defect as a stale screenshot, and it is
// the reason this test derives the list instead of restating it.
//
// It does not check the *wording*. It checks that no document names a language
// the interface does not offer, and that every language the interface offers is
// named where the languages are listed.
func TestNoDocumentNamesALanguageTheInterfaceDoesNotOffer(t *testing.T) {
	root := filepath.Dir(localesDir(t))

	// The endonym is what a page shows a reader, so that is what is searched
	// for. Add a line here when a language is added; the test below is what
	// makes forgetting it fail.
	endonyms := map[string]string{
		"en": "English",
		"de": "Deutsch",
		"fr": "Français",
	}

	offered, err := LocaleCodes(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range offered {
		if _, ok := endonyms[code]; !ok {
			t.Fatalf("locales/%s.json exists but this test has no endonym for it; "+
				"add one so the documents can be checked against it", code)
		}
	}

	for _, rel := range [][]string{
		{"CLAUDE.md"},
		{"docs", "_docs", "index.md"},
		{"docs", "_docs", "installation", "first-run.md"},
		{"CONTRIBUTING.md"},
	} {
		name := filepath.Join(rel...)
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)

		// A document that lists languages at all must list all of them.
		listsAny := false
		for _, code := range offered {
			if strings.Contains(body, endonyms[code]) {
				listsAny = true
			}
		}
		if !listsAny {
			continue // this page does not enumerate languages; nothing to keep in step
		}
		for _, code := range offered {
			if !strings.Contains(body, endonyms[code]) {
				t.Errorf("%s lists languages but never names %s (%s), which the "+
					"interface offers", name, endonyms[code], code)
			}
		}
	}
}
```

**Note for the implementer:** this test will fail until Step 2 is done — that is
the point. `docs/_docs/installation/first-run.md` in particular has the endonyms
inside a screenshot `alt` attribute, so fixing it means fixing the alt text,
which is the sentence that describes the picture Task 8 re-takes.

- [ ] **Step 2: Fix the claims**

- `CLAUDE.md`: *every visible string, English and German at parity* → state the
  rule instead of the list: English and German at parity, further languages by
  fallback with a coverage report.
- `docs/_docs/index.md`: the feature table row.
- `docs/_docs/installation/first-run.md`: the `alt` text says *"a language switch
  between German and English in the footer"* — it is now a select with three.
- `CONTRIBUTING.md` **Adding a Language**: rewrite. Today it promises
  *"`TestLocaleFilesAreAtParity` and `TestTemplatesOnlyUseTranslatedKeys` will
  tell you about anything you missed"*, which was never true for any language
  but German. Say what is actually enforced, that a partial translation is
  welcome, how `status.json` works, and point at `docs-tech/i18n-review.md`.
- `DESIGN.md`: the note about `Deutsch` and `English` naming themselves now
  describes a select.
- `docs/_docs/roadmap.md` and `README.md`: carry the numbering from the roadmap
  amendment; move the delivered rows into *Done*.

- [ ] **Step 3: Run the tests and commit**

```bash
go test ./internal/...
git add CLAUDE.md CONTRIBUTING.md DESIGN.md README.md docs/_docs/index.md \
        docs/_docs/installation/first-run.md docs/_docs/roadmap.md \
        internal/web/i18n_docs_test.go
git commit -m "docs(i18n): what is actually enforced when you add a language"
```

---

### Task 8: The screenshots

**Files:**
- Modify: `docs/assets/img/screens/*.png`

**Why:** the switcher changed shape and a third language appeared. Every
screenshot showing the sidebar or an auth page now shows something the interface
no longer does.

- [ ] **Step 1: Re-take them**

Follow the method recorded in commit `522f27d`: build with
`make build VERSION=<the version being released>` — **not** `git describe`, which
bakes a dirty development hash into the images — then drive the three throwaway
instances at the same viewport with the same fade-in settle wait.

- [ ] **Step 2: Verify against the live DOM**

Check the sidebar chip, the dashboard body line and both auth-page footers read
the release version, and that the language control in every image is the select.

- [ ] **Step 3: Commit**

```bash
git add docs/assets/img/screens/
git commit -m "docs: re-take the screenshots for the new language switcher"
```

---

### Task 9: The release announces itself on Discord

**Files:**
- Modify: `.github/workflows/release.yml` — a new `announce` job after `debian` (line 107)
- Modify: `docs-tech/ci-and-release.md` — the workflow table and the `release.yml` section

**Interfaces:**
- Consumes: the repository secret `DISCORD_WEBHOOK`, already set.
- Produces: nothing other code uses.

**Why:** 2.8.0 shipped and was announced by hand, hours later. An announcement
that depends on somebody remembering is one that is eventually forgotten, and a
release nobody hears about is most of a release wasted.

- [ ] **Step 1: Add the job**

Append to `.github/workflows/release.yml`:

```yaml
  announce:
    name: Announce on Discord
    needs: [goreleaser, debian]
    runs-on: ubuntu-24.04
    # Best-effort on purpose: the release is already published and its assets
    # are already uploaded by the time this runs. A Discord outage must not
    # colour a good release red — it is reported as a warning and nothing else.
    continue-on-error: true
    # The secret is mapped to env at job level deliberately. The `secrets`
    # context is NOT available in a step's `if:` — only `env` is — so
    # `if: ${{ secrets.DISCORD_WEBHOOK != '' }}` would never be true and the
    # job would silently never post, which is the failure mode this whole task
    # exists to remove.
    env:
      WEBHOOK: ${{ secrets.DISCORD_WEBHOOK }}
      TAG: ${{ github.ref_name }}
      REPO: ${{ github.repository }}
    steps:
      - name: Post the release
        # No secret, no post — a fork, or a clone with no webhook configured,
        # skips this rather than failing on an empty URL.
        if: env.WEBHOOK != ''
        run: |
          set -euo pipefail
          # jq builds the JSON so a quote or a backtick in the release notes
          # cannot break out of the payload.
          payload=$(jq -n \
            --arg title "easywall ${TAG#v}" \
            --arg url "https://github.com/${REPO}/releases/tag/${TAG}" \
            '{
               username: "easywall",
               embeds: [{
                 title: $title,
                 url: $url,
                 color: 3776250,
                 description: "A new release is out. The full account is in the release notes; the `.deb` for amd64 and arm64 is on the release page, and the images are on GHCR, Docker Hub and Quay.",
                 footer: { text: "Every apply reverts itself after 120 seconds unless you confirm it." }
               }]
             }')
          code=$(curl -sS -o /tmp/discord.out -w '%{http_code}' \
                 -H 'Content-Type: application/json' \
                 -X POST -d "$payload" "$WEBHOOK")
          echo "Discord responded $code"
          if [ "$code" != "204" ] && [ "$code" != "200" ]; then
            cat /tmp/discord.out
            exit 1
          fi
```

- [ ] **Step 2: Check the workflow parses**

Run: `python3 -c "import yaml,sys;yaml.safe_load(open('.github/workflows/release.yml'));print('parses')"`
Expected: `parses`.

Also confirm `TestWorkflowsDoNotPinAGoVersionOfTheirOwn` — the subtest *no
workflow pins a version of its own* in `TestGoToolchainIsTheSameEverywhere` —
still passes; this job adds no `go-version:`.

Run: `go test ./internal/shared/ -run TestGoToolchainIsTheSameEverywhere -v`

- [ ] **Step 3: Prove the payload before trusting the workflow**

The job cannot be run without cutting a release, so verify the part that can
actually be wrong — the `jq` construction — locally:

```bash
TAG=v2.9.0 REPO=jp1337/easywall
jq -n --arg title "easywall ${TAG#v}" \
      --arg url "https://github.com/${REPO}/releases/tag/${TAG}" \
      '{username:"easywall",embeds:[{title:$title,url:$url,color:3776250}]}'
```
Expected: valid JSON with `easywall 2.9.0` and the correct URL.

- [ ] **Step 4: Document it**

In `docs-tech/ci-and-release.md`: add the job to the `release.yml` diagram and
say what it is and is not. It is best-effort and `continue-on-error`, because the
release is complete before it runs. Record that the Ko-fi post is **not** here
and cannot be: Ko-fi has no writing API, so that post is a browser-assisted step
a person takes, and pretending otherwise in this file would be the kind of
documentation that sends somebody looking for a job that does not exist.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml docs-tech/ci-and-release.md
git commit -m "ci(release): announce the release on Discord"
```

---

### Task 10: Release 2.9.0

Only after **both** this plan and
`docs-tech/plans/2026-08-20-docker-environment-variables-implementation.md` are
complete and `main` is green.

- [ ] **Step 1: Follow the release procedure**

`docs-tech/ci-and-release.md`, *Cutting a release*:

1. `CHANGELOG.md` — move `[Unreleased]` to `## [2.9.0] — <today>`, and add the
   link reference at the end of the section, the convention that file uses.
2. `debian/changelog` — a new `easywall (2.9.0) unstable; urgency=medium` entry.
3. `internal/shared/version.go` and `docs/_config.yml`'s `version:` — both to
   `2.9.0`; `TestDocsVersionMatchesRelease` fails if they disagree.
4. Tag `v2.9.0` and push it.
5. **Read the release run's log rather than the tick**, and download one `.deb`
   to confirm it contains what it should — including `usr/share/easywall/locales/fr.json`.

- [ ] **Step 2: Announce**

Discord, via the webhook in `release.yml` if that work has landed by then;
otherwise by hand. The Ko-fi post is a browser-assisted step and cannot run in
CI — see the note in this repository's release discussion.

---

## Notes for the implementer

- The per-key fallback is **already there**: `langCandidates` ends with the
  default language and `en`, and go-i18n resolves each message ID down that list.
  Nothing in Tasks 1–2 adds fallback; they add the accounting that makes an
  invisible gap visible.
- `internal/web/handler_language_test.go:117` expects exactly
  `{"de": "Deutsch", "en": "English"}` — but **it will not go red when `fr.json`
  lands**, and that is worth knowing before you go looking for the failure.
  `bundleWith` (line 80) writes its own catalogue into `t.TempDir()`, so every
  test in that file is isolated from the real `locales/`. Good design, and it
  means nothing in that file notices a new language at all. The tests that do
  notice are the ones added in Tasks 1 and 2, which read `locales/` directly.
- Because `bundleWith` builds a temp directory with no `status.json` in it,
  `LoadLocaleStatus` must return an empty map rather than an error when the file
  is absent. An absent status therefore means *not reviewed*, which is the
  understating direction and keeps every existing test in that file working
  untouched.
- Packaging needs no change: `debian/rules:37`, `Makefile:61` and `Dockerfile:58`
  all copy `locales/` whole, so a new file ships everywhere by itself. Verified
  against the shipped 2.8.0 `.deb`.
- French is roughly 15–20 % longer than English. Layout problems will be in the
  sidebar and in button labels, not in body copy.
