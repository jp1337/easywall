package web

import (
	"testing"
	"time"
)

func TestActionLabel(t *testing.T) {
	cases := map[string]string{
		// Exactly the identifiers internal/core writes.
		"apply_started":    "Apply started",
		"apply_accepted":   "Rules applied",
		"apply_rolledback": "Rules rolled back",
		"apply_failed":     "Apply failed",
		"rules_saved":      "Rules saved",
		"rules_imported":   "Rules imported",
		"options_saved":    "Options saved",
		"settings_saved":   "Settings saved",
		"system_saved":     "System settings saved",
		// An action the map does not know must still render as language, so a
		// new core action is readable before this table catches up.
		"future_thing": "Future thing",
		"":             "",
	}
	for in, want := range cases {
		if got := actionLabel(in); got != want {
			t.Errorf("actionLabel(%q) = %q, want %q", in, got, want)
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

// The offset stored in the timestamp is the host's own local time and must not
// be converted away — an operator correlating this against syslog reads it in
// that frame.
func TestShortTime_PreservesStoredOffset(t *testing.T) {
	// A fixed instant, expressed in a zone deliberately far from the test host's.
	const stamp = "2020-06-15T23:30:00+09:00"
	if got := shortTime(stamp); got != "15 Jun 2020 23:30" {
		t.Errorf("shortTime(%q) = %q, want %q — the +09:00 wall time, not a converted one",
			stamp, got, "15 Jun 2020 23:30")
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
