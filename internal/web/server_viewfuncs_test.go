package web

import (
	"strings"
	"testing"
	"time"
)

// identityT stands in for the per-request T. Returning the message id makes the
// mapping visible: the assertion is that an action resolves to its own key, not
// that a particular English wording survives, which is now the locale's job.
func identityT(id string, _ ...interface{}) string { return id }

func TestActionLabel(t *testing.T) {
	cases := map[string]string{
		// Exactly the identifiers internal/core writes.
		"apply_started":    "audit_apply_started",
		"apply_accepted":   "audit_apply_accepted",
		"apply_rolledback": "audit_apply_rolledback",
		"apply_failed":     "audit_apply_failed",
		"rules_saved":      "audit_rules_saved",
		"rules_imported":   "audit_rules_imported",
		"options_saved":    "audit_options_saved",
		"settings_saved":   "audit_settings_saved",
		"system_saved":     "audit_system_saved",
		// An action the map does not know must still render as language, so a
		// new core action is readable before this table catches up. There is no
		// translation to reach for, so it is humanised rather than localized.
		"future_thing": "Future thing",
		"":             "",
	}
	for in, want := range cases {
		if got := actionLabel(identityT, in); got != want {
			t.Errorf("actionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

// The label an operator sees comes from the locale, so every action the core can
// write needs an entry in every locale file. A missing one makes T fall back to
// the raw message id and the audit log prints "audit_apply_failed".
func TestActionLabel_EveryActionIsTranslated(t *testing.T) {
	for _, lang := range []string{"en", "de"} {
		ids := localeIDs(t, lang)
		for action, key := range auditActionLabels {
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q (for action %q)", lang, key, action)
			}
		}
	}
}

func TestActionTone(t *testing.T) {
	// Only the apply_* actions describe what the firewall is doing, so only
	// they carry colour. Saving stages a change and stays neutral.
	cases := map[string]string{
		"apply_accepted":   "ok",
		"apply_started":    "warn",
		"apply_rolledback": "crit",
		"apply_failed":     "crit",
		"rules_saved":      "",
		"rules_imported":   "",
		"options_saved":    "",
		"settings_saved":   "",
		"system_saved":     "",
		"unknown":          "",
	}
	for in, want := range cases {
		if got := actionTone(in); got != want {
			t.Errorf("actionTone(%q) = %q, want %q", in, got, want)
		}
	}
}

// The stylesheet used to key its colours on "rules_applied" and
// "rules_rolled_back", names only the demo client produced, so no real audit
// entry was ever tinted. Guard against a tone table that drifts back to
// identifiers the core does not write.
func TestActionTone_UsesRealCoreIdentifiers(t *testing.T) {
	for _, stale := range []string{"rules_applied", "rules_rolled_back"} {
		if tone := actionTone(stale); tone != "" {
			t.Errorf("%q is not written by internal/core but carries tone %q", stale, tone)
		}
	}
	if actionTone("apply_rolledback") == "" {
		t.Error("apply_rolledback must be tinted — it is the most consequential audit entry")
	}
}

func TestShortTime(t *testing.T) {
	now := time.Now()

	// Same day renders as a clock time.
	today := now.Format(time.RFC3339)
	if got := shortTime(today); got != now.Format("15:04:05") {
		t.Errorf("shortTime(today) = %q, want %q", got, now.Format("15:04:05"))
	}

	// Earlier in the same year gains a day and month but no year.
	earlier := now.AddDate(0, 0, -40)
	if got := shortTime(earlier.Format(time.RFC3339)); got != earlier.Format("2 Jan 15:04") {
		// Crossing a year boundary is a legitimate outcome 40 days back, so only
		// fail when the year genuinely matches.
		if earlier.Year() == now.Year() {
			t.Errorf("shortTime(-40d) = %q, want %q", got, earlier.Format("2 Jan 15:04"))
		}
	}

	// A different year spells the year out.
	old := now.AddDate(-3, 0, 0)
	if got := shortTime(old.Format(time.RFC3339)); got != old.Format("2 Jan 2006 15:04") {
		t.Errorf("shortTime(-3y) = %q, want %q", got, old.Format("2 Jan 2006 15:04"))
	}

	// Anything unparseable is shown untouched rather than swallowed.
	for _, junk := range []string{"", "never", "2026-08-03"} {
		if got := shortTime(junk); got != junk {
			t.Errorf("shortTime(%q) = %q, want it returned unchanged", junk, got)
		}
	}
}

// These three assertions are about what zone the process runs in, so their
// expectations depend on TZ at the time the test binary runs. Go's test result
// cache does not key on TZ: `go test -run ShortTime` a second time under a
// different TZ will happily return the previous run's cached PASS without
// re-executing anything. Verifying this by hand across zones needs
// `-count=1`, or the second and later runs prove nothing.
//
// A stored stamp is rendered in the zone the web process runs in, not in the
// zone the string happens to carry.
//
// This test used to assert the opposite — that a +09:00 stamp kept its wall
// time, "not a converted one" — and the reasoning behind that was sound for an
// input that never occurs. The core writes UTC and only UTC (core/rules.go:319,
// core/firewall.go:286), so "preserve the offset" meant "always display UTC",
// while journalctl next to it showed local time. Two hours apart in Berlin, on
// every row, with a comment in shortTime explaining why it was the operator's
// own time.
//
// The full stored value is still in the title attribute in log.html and
// dashboard.html, so nothing is lost by converting the visible one.
func TestShortTime_RendersInTheLocalZone(t *testing.T) {
	// A fixed instant, expressed in a zone that is neither UTC nor the test
	// machine's, so a function that failed to convert would be visibly wrong
	// wherever this runs.
	stamp := "2020-06-15T23:30:00+09:00"
	instant, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := instant.Local().Format("2 Jan 2006 15:04")
	if got := shortTime(stamp); got != want {
		t.Errorf("shortTime(%q) = %q, want %q — the same instant in this host's zone",
			stamp, got, want)
	}
}

// The case that produced the bug report: what the core actually stores.
//
// The instant is a couple of seconds in the past rather than "now" outright,
// so parsing and formatting between the two Now() calls can't put it on the
// wrong side of a boundary — but it is deliberately not offset by anything
// round like 90 minutes. Any offset large enough to be memorable is also large
// enough to land within that many minutes of local midnight somewhere in the
// world the test runs, and then this test crosses a local day boundary and
// fails for a reason that has nothing to do with shortTime. What is left is a
// window of a few seconds around "now", which is honest: it can only fail in
// the nanosecond slice of a run that straddles local midnight, and no
// constant offset removes that, only widens it. Don't "fix" this back to a
// round number.
func TestShortTime_ConvertsTheUTCTheCoreWrites(t *testing.T) {
	instant := time.Now().Add(-2 * time.Second)
	stored := instant.UTC().Format(time.RFC3339) // exactly what rules.go:319 writes

	want := instant.Local().Format("15:04:05")
	if got := shortTime(stored); got != want {
		t.Errorf("shortTime(%q) = %q, want %q — an operator reads their own clock",
			stored, got, want)
	}
}

// "Today" is decided in the viewer's zone too. A stamp from 23:30 UTC is
// tomorrow in Berlin, and rendering it as a bare clock time under yesterday's
// date would be worse than the bug being fixed.
//
// This only checks that "now" reads as today, which holds for almost any
// implementation and does not pin the defect this task exists to fix — see
// TestShortTime_UTCEveningCrossesTheLocalDayInBerlin below for the
// deterministic case that does.
func TestShortTime_TodayIsDecidedInTheLocalZone(t *testing.T) {
	now := time.Now()
	stored := now.UTC().Format(time.RFC3339)

	if got, want := shortTime(stored), now.Local().Format("15:04:05"); got != want {
		t.Errorf("shortTime(now) = %q, want %q", got, want)
	}
}

// The actual defect, pinned without depending on the clock or the machine's
// zone: a UTC evening stamp whose local calendar day is already the next one.
// 2020-06-15T23:30:00Z is 2020-06-16 01:30 in Berlin — a different day. 2020 is
// also a different year from whenever this test runs, so shortTime always
// takes its "2 Jan 2006 15:04" branch here regardless of the current date —
// the expected string is fully determined and asserted as a literal, not
// computed at runtime.
//
// Getting a fixed, non-host zone requires mutating the package-level
// time.Local, which time.Time.Local() and time.Now() both read implicitly —
// there is no other way to make "this host's local zone" deterministic without
// threading a zone through shortTime's signature, which the brief does not
// ask for. The deferred restore is not optional: leaving time.Local pointed at
// Berlin would silently change what "local" means for every other test in
// this package that runs afterward, including the ones above that depend on
// the real host zone.
func TestShortTime_UTCEveningCrossesTheLocalDayInBerlin(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	old := time.Local
	time.Local = berlin
	defer func() { time.Local = old }()

	const stamp = "2020-06-15T23:30:00Z"
	const want = "16 Jun 2020 01:30" // one day later, local, than the UTC date in the stamp
	if got := shortTime(stamp); got != want {
		t.Errorf("shortTime(%q) = %q, want %q — 23:30 UTC is already the 16th in Berlin",
			stamp, got, want)
	}
}

func TestTemplateFuncs_CountEntries(t *testing.T) {
	funcs := templateFuncs()
	countEntries := funcs["countEntries"].(func([]string) int)

	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"nil", nil, 0},
		{"addresses only", []string{"10.0.0.1", "10.0.0.2"}, 2},
		{"comments and blanks are not entries",
			[]string{"# scanner ranges", "", "192.0.2.42", "  ", "198.51.100.0/24", "#trailing"}, 2},
		{"indented comment still a comment", []string{"   # note", "10.0.0.1"}, 1},
		{"whitespace-only", []string{" ", "\t"}, 0},
	}
	for _, c := range cases {
		if got := countEntries(c.lines); got != c.want {
			t.Errorf("%s: countEntries = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTemplateFuncs_Plural(t *testing.T) {
	funcs := templateFuncs()
	plural := funcs["plural"].(func(int, string, string) string)

	for _, c := range []struct {
		n    int
		want string
	}{{0, "entries"}, {1, "entry"}, {2, "entries"}, {12, "entries"}} {
		if got := plural(c.n, "entry", "entries"); got != c.want {
			t.Errorf("plural(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// countEntries backs the server-rendered counter and initListCounter() in
// app.js backs the live one. They must agree, or the number changes on reload.
func TestCountEntries_MatchesClientSideRule(t *testing.T) {
	funcs := templateFuncs()
	countEntries := funcs["countEntries"].(func([]string) int)

	lines := []string{"# comment", "", "10.0.0.1", "  ", "2001:db8::/32", "\t# indented"}
	// The JS filters trimmed lines that are neither empty nor '#'-prefixed.
	want := 0
	for _, l := range lines {
		trimmed := trimSpaceJS(l)
		if trimmed != "" && !hasPrefixJS(trimmed, "#") {
			want++
		}
	}
	if want != 2 {
		t.Fatalf("fixture drifted: expected 2 real entries, the JS rule computed %d", want)
	}
	if got := countEntries(lines); got != want {
		t.Errorf("countEntries = %d, JS rule yields %d — the counters would disagree", got, want)
	}
}

func trimSpaceJS(s string) string {
	start, end := 0, len(s)
	for start < end && isSpaceJS(s[start]) {
		start++
	}
	for end > start && isSpaceJS(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpaceJS(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

func hasPrefixJS(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func TestRichText(t *testing.T) {
	cases := []struct {
		name, text string
		args       []string
		want       string
	}{
		{"plain text passes through", "Nothing to mark up.", nil, "Nothing to mark up."},
		{"backticks become code", "A single port is `443`.", nil,
			"A single port is <code>443</code>."},
		{"asterisks become emphasis", "Evaluated *before* the whitelist.", nil,
			"Evaluated <em>before</em> the whitelist."},
		{"one link", "Staged until you {}.", []string{"/apply", "apply rules"},
			`Staged until you <a class="link" href="/apply">apply rules</a>.`},
		{"two links, in order", "Use the {} or a {}.",
			[]string{"/whitelist", "whitelist", "/custom", "custom rule"},
			`Use the <a class="link" href="/whitelist">whitelist</a> or a ` +
				`<a class="link" href="/custom">custom rule</a>.`},
		// A translator may put the link first where English has it last. That is
		// the whole reason the slot exists, so it has to work in any position.
		{"link first", "{} is evaluated first.", []string{"/blacklist", "blacklist"},
			`<a class="link" href="/blacklist">blacklist</a> is evaluated first.`},
		{"markup and link together", "Add `443` under {}.", []string{"/ports", "port rules"},
			`Add <code>443</code> under <a class="link" href="/ports">port rules</a>.`},
		// A typo in a locale file must not blank the panel it sits in.
		{"unterminated backtick keeps its text", "A port is `443", nil, "A port is `443"},
		{"unterminated asterisk keeps its text", "Evaluated *before", nil, "Evaluated *before"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := richText(c.text, c.args...)
			if err != nil {
				t.Fatalf("richText(%q): %v", c.text, err)
			}
			if string(got) != c.want {
				t.Errorf("richText(%q)\n got: %s\nwant: %s", c.text, got, c.want)
			}
		})
	}
}

// Locale text is data, and an operator can put anything in a locale file. It is
// interpolated into HTML, so escaping is the only thing between a translation
// and script injection.
func TestRichText_EscapesEverything(t *testing.T) {
	got, err := richText("Danger: <script>alert(1)</script> and `<b>x</b>` {}",
		`" onmouseover="alert(2)`, "<em>label</em>")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"<script>", "<b>x</b>", `onmouseover="alert`, "<em>label"} {
		if strings.Contains(string(got), forbidden) {
			t.Errorf("unescaped %q survived into output: %s", forbidden, got)
		}
	}
	if !strings.Contains(string(got), "&lt;script&gt;") {
		t.Errorf("expected the script tag escaped, got: %s", got)
	}
}

// A count mismatch is a template bug, and a silently dropped link would ship a
// sentence with a dangling "{}" in it.
func TestRichText_RejectsMismatchedSlots(t *testing.T) {
	if _, err := richText("One slot {}."); err == nil {
		t.Error("expected an error when a slot has no link")
	}
	if _, err := richText("No slots.", "/apply", "apply"); err == nil {
		t.Error("expected an error when a link has no slot")
	}
	if _, err := richText("One slot {}.", "/apply"); err == nil {
		t.Error("expected an error for an unpaired href")
	}
}
