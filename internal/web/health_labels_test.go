package web

import (
	"sort"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Every reason the health check can give has a sentence in every strict locale.
//
// The same guard AllReachReasons hangs off, on the same argument: a closed enum
// exists so a sentence is translated rather than assembled in Go, and a reason
// with no key renders as its own message id — "health_reason_stateful_dead" — on
// the panel whose whole job is to be believed. Seven, not six: six come out of
// computeHealth and core_unreachable is the one the web process originates,
// which is exactly why the list is iterated rather than typed out here.
func TestEveryHealthReasonHasBothLocales(t *testing.T) {
	strict := strictLangSet()
	if len(strict) == 0 {
		t.Fatal("strictLangSet is empty; this guard would inspect nothing")
	}
	if len(shared.AllHealthReasons) == 0 {
		t.Fatal("AllHealthReasons is empty; this guard would inspect nothing")
	}
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, r := range shared.AllHealthReasons {
			key := "health_reason_" + string(r)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q for health reason %q", lang, key, r)
			}
		}
	}
}

// And every state, which the hero renders as the word beside the dot.
//
// The three are written out rather than read from a list because shared has no
// AllHealthStates: HealthState is a three-value enum whose exhaustiveness is
// carried by `easywall-core health`'s exit-code switch, and a fourth value would
// have to pass through that switch's default before it ever reached a page.
func TestEveryHealthStateHasBothLocales(t *testing.T) {
	strict := strictLangSet()
	if len(strict) == 0 {
		t.Fatal("strictLangSet is empty; this guard would inspect nothing")
	}
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, s := range []shared.HealthState{
			shared.HealthOK, shared.HealthDegraded, shared.HealthFail,
		} {
			key := "health_state_" + string(s)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q", lang, key)
			}
		}
	}
}

// And every self-test result the hero's fourth fact can name.
func TestEverySelftestResultHasBothLocales(t *testing.T) {
	strict := strictLangSet()
	if len(strict) == 0 {
		t.Fatal("strictLangSet is empty; this guard would inspect nothing")
	}
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, r := range []shared.SelftestResult{
			shared.SelftestPassed, shared.SelftestFailed, shared.SelftestUnprovable,
		} {
			key := "health_proof_" + string(r)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q", lang, key)
			}
		}
	}
}

// The three actions 2.17 added: labelled, and in both locales, or the audit log
// prints a humanised snake_case token and the filter — which searches the label
// an operator can see — cannot find them at all.
func TestTheNewAuditActionsAreLabelled(t *testing.T) {
	actions := []string{"selftest_passed", "selftest_failed", "health_degraded"}
	for _, action := range actions {
		key, ok := auditActionLabels[action]
		if !ok {
			t.Errorf("%s has no entry in auditActionLabels; it renders as a raw identifier", action)
			continue
		}
		for _, lang := range []string{"en", "de"} {
			if !localeIDs(t, lang)[key] {
				t.Errorf("locales/%s.json has no %q, the label for %s", lang, key, action)
			}
		}
	}
}

// The tone table holds exactly the actions that are firewall states, and the
// list is here rather than derived, because "is this a firewall state" is a
// judgement a person made about each one and not something a program can
// recompute. Adding an entry in server.go without adding it here is the
// deliberate edit this guard exists to force.
//
// selftest_failed is the entry a later reader will want to add. It is not a
// firewall state: the proof disproves a claim about the *rule builder* inside
// its own network namespace, and the firewall on this machine goes on filtering
// exactly as it did a second earlier. The news reaches an operator as
// health_degraded, which is on this list, and as a degraded dashboard. Same
// distinction server.go already draws for the nine login events — a notification
// is not a colour, and neither is a failed proof.
func TestOnlyFirewallStatesCarryATone(t *testing.T) {
	want := map[string]string{
		"apply_accepted":         "ok",
		"apply_started":          "warn",
		"apply_rolledback":       "crit",
		"apply_failed":           "crit",
		"rollback_failed":        "crit",
		"boot_enforced":          "ok",
		"boot_not_configured":    "warn",
		"boot_enforce_failed":    "crit",
		"panic_engaged":          "crit",
		"panic_resumed":          "ok",
		"resume_restore_skipped": "crit",
		"health_degraded":        "warn",
	}

	for action, tone := range auditActionTones {
		expected, ok := want[action]
		if !ok {
			t.Errorf("auditActionTones colours %q %q, and this list says it is not a firewall "+
				"state. Colour means what the firewall is doing (DESIGN.md rule 1) — either it "+
				"belongs here with a reason, or the entry in server.go does not belong there",
				action, tone)
			continue
		}
		if tone != expected {
			t.Errorf("auditActionTones colours %q %q; this list expects %q", action, tone, expected)
		}
	}
	for action, tone := range want {
		if _, ok := auditActionTones[action]; !ok {
			t.Errorf("%q is a firewall state worth %q and auditActionTones has no entry for it; "+
				"it renders neutral grey", action, tone)
		}
	}

	// The three colours DESIGN.md allows, and nothing else. A fourth tone here
	// would be a class the stylesheet does not define, so the entry renders with
	// no colour at all and the build stays green.
	for action, tone := range auditActionTones {
		switch tone {
		case "ok", "warn", "crit":
		default:
			t.Errorf("%q is toned %q; the palette is state-ok, state-warn and state-crit and "+
				"nothing else", action, tone)
		}
	}
}

// healthTone maps the state onto one of those three, and an unknown state onto
// warn rather than ok.
//
// The default branch is not defensive padding. The core and the web process are
// separate binaries that a package upgrade can leave a version apart, so a state
// this build has no case for means a newer core — and rendering that green is
// the same false green the release exists to remove.
func TestHealthTone(t *testing.T) {
	fn, ok := templateFuncs()["healthTone"].(func(shared.HealthState) string)
	if !ok {
		t.Fatal("templateFuncs has no healthTone of that signature")
	}
	cases := map[shared.HealthState]string{
		shared.HealthOK:             "ok",
		shared.HealthDegraded:       "warn",
		shared.HealthFail:           "crit",
		"a-state-from-a-newer-core": "warn",
	}
	for in, want := range cases {
		if got := fn(in); got != want {
			t.Errorf("healthTone(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every class the health rendering asks for is in the *built* stylesheet.
//
// TestEveryTemplateClassIsInTheStylesheet cannot see the dot: its regex is
// `class="([^"{}]*)"`, which skips any attribute holding a template action, and
// the dot is written `class="hero-dot {{healthTone …}}"`. So the one thing that
// would catch a Tailwind rule dropped from the build is this list. A green
// `npm run build:css` is not proof a rule shipped — that has cost this
// repository a release already.
func TestTheHealthClassesAreInTheStylesheet(t *testing.T) {
	defined := styleClasses(t)
	for _, cls := range []string{
		"hero-dot", "hero-note", "hero-fact-sub",
	} {
		if !defined[cls] {
			t.Errorf("class %q is not in web/static/style.css; rebuild it, and if the rebuild "+
				"does not emit the rule, the cause is the source scanning", cls)
		}
	}

	// The dot modifiers are compound selectors (.hero-dot.warn), so the class
	// names on their own prove nothing about the pairing. Assert the pair.
	css := appStylesheet(t)
	for _, sel := range []string{".hero-dot.ok", ".hero-dot.warn", ".hero-dot.crit"} {
		if !strings.Contains(css, sel) {
			t.Errorf("%s is not in the built stylesheet, so that state's dot has no colour", sel)
		}
	}
}

// A locale key the dashboard builds with printf is invisible to
// TestTemplatesOnlyUseTranslatedKeys, whose regex matches `T "literal"` only.
// This is the same check for the three families the health panel composes, and
// it fails on a key that exists in no locale rather than on one that is merely
// missing from a lax language.
func TestTheComposedHealthKeysExistInEnglish(t *testing.T) {
	ids := localeIDs(t, "en")
	var missing []string
	for _, key := range composedHealthKeys() {
		if !ids[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		t.Errorf("locales/en.json has no %q; the dashboard composes that id with printf, so it "+
			"renders the id itself and no guard on literal T calls can see it", key)
	}
}

func composedHealthKeys() []string {
	keys := []string{}
	for _, r := range shared.AllHealthReasons {
		keys = append(keys, "health_reason_"+string(r))
	}
	for _, s := range []shared.HealthState{shared.HealthOK, shared.HealthDegraded, shared.HealthFail} {
		keys = append(keys, "health_state_"+string(s))
	}
	for _, r := range []shared.SelftestResult{
		shared.SelftestPassed, shared.SelftestFailed, shared.SelftestUnprovable,
	} {
		keys = append(keys, "health_proof_"+string(r))
	}
	return keys
}
