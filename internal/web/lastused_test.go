package web

import (
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// idT returns the message id itself, so these tests assert on which sentence was
// chosen rather than on how it happens to be worded in English today.
func idT(id string, _ ...interface{}) string { return id }

// A rule with no id and a rule that has never been seen are different claims,
// and the interface has to make different statements about them.
//
// "never" on a rule the collector has never been able to key on would be a
// measurement nobody took, printed as a measurement — and it is the one a
// careful operator would act on, because a port nothing has ever reached is
// exactly what this release exists to point at.
func TestARuleWithoutAnIDRendersEmDash(t *testing.T) {
	usage := map[string]shared.RuleUsage{"aaaaaaaaaaaa": {LastSeen: time.Now()}}

	if got := lastUsed(idT, "", usage, true); got != "—" {
		t.Errorf("a rule with no id rendered %q, want an em dash; it has not been observed, "+
			"which is not the same as having been observed to be idle", got)
	}
	if got := lastUsed(idT, "bbbbbbbbbbbb", usage, true); got != "used_never" {
		t.Errorf("a rule with an id and no recorded use rendered %q, want used_never", got)
	}
	// And when the counters could not be read at all, nothing is claimed about
	// any rule.
	if got := lastUsed(idT, "aaaaaaaaaaaa", nil, false); got != "—" {
		t.Errorf("with the usage unavailable, a rule rendered %q, want an em dash", got)
	}
}

func TestLastUsed_PicksTheRightSentence(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"seconds", 20 * time.Second, "used_just_now"},
		{"one minute", 70 * time.Second, "used_minutes_one"},
		{"minutes", 40 * time.Minute, "used_minutes_many"},
		{"one hour", 61 * time.Minute, "used_hours_one"},
		{"hours", 9 * time.Hour, "used_hours_many"},
		{"one day", 26 * time.Hour, "used_days_one"},
		{"days", 60 * 24 * time.Hour, "used_days_many"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := map[string]shared.RuleUsage{"x": {LastSeen: now.Add(-tc.ago)}}
			if got := lastUsed(idT, "x", usage, true); got != tc.want {
				t.Errorf("%v ago rendered %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

// A clock that went backwards — an NTP step, a VM resumed from a snapshot —
// must not print "in 3 hours" in a column headed Last used.
func TestLastUsed_AFutureTimestampReadsAsJustNow(t *testing.T) {
	usage := map[string]shared.RuleUsage{"x": {LastSeen: time.Now().Add(3 * time.Hour)}}
	if got := lastUsed(idT, "x", usage, true); got != "used_just_now" {
		t.Errorf("a timestamp in the future rendered %q, want used_just_now", got)
	}
}

// Every id the function can return has to exist in both strict locales, or the
// column renders a raw message id on some page in some language.
func TestLastUsedKeysAreTranslated(t *testing.T) {
	want := []string{
		"ports_last_used", "used_never", "used_just_now",
		"used_minutes_one", "used_minutes_many",
		"used_hours_one", "used_hours_many",
		"used_days_one", "used_days_many",
		"tile_tcp_unused",
	}
	for _, lang := range StrictLangs {
		ids := localeIDs(t, lang)
		for _, id := range want {
			if !ids[id] {
				t.Errorf("%s.json has no %q", lang, id)
			}
		}
	}
}

// The counted sentences carry the number, or "40 minutes ago" reads as
// "minutes ago" with nothing in front of it.
func TestCountedUsageStringsCarryTheirPlaceholder(t *testing.T) {
	en := localeStrings(t, "en")
	de := localeStrings(t, "de")
	for _, id := range []string{"used_minutes_many", "used_hours_many", "used_days_many", "tile_tcp_unused"} {
		for lang, m := range map[string]map[string]string{"en": en, "de": de} {
			if !strings.Contains(m[id], "{{.N}}") {
				t.Errorf("%s.json %q = %q, which carries no {{.N}}", lang, id, m[id])
			}
		}
	}
}
