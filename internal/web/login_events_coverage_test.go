package web

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Three guards off one list, the way four already hang off AllCommandTypes:
// every event has a label, every event is documented, and both locales carry the
// label. An event added to protocol.go and nowhere else renders as a humanised
// snake_case token in no language a translator chose.
func TestEveryLoginEventIsLabelledDocumentedAndTranslated(t *testing.T) {
	docs := repoFile2(t, "docs", "_docs", "features", "audit-log.md")

	// localeIDs (i18n_coverage_test.go) is the package's existing locale reader;
	// it already reports which ids exist per language, which is all this guard
	// needs — a second reader here would be exactly the kind of drift the guard
	// tests exist to prevent.
	locales := map[string]map[string]bool{}
	for _, lang := range []string{"en", "de"} {
		locales[lang] = localeIDs(t, lang)
	}

	for _, ev := range shared.AllLoginEvents {
		key, ok := auditActionLabels[string(ev)]
		if !ok {
			t.Errorf("%s has no entry in auditActionLabels; it renders as a raw identifier", ev)
			continue
		}
		if !strings.Contains(docs, "`"+string(ev)+"`") {
			t.Errorf("features/audit-log.md does not document %s", ev)
		}
		for lang, ids := range locales {
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q, the label for %s", lang, key, ev)
			}
		}
	}
}

// None of the nine gets a colour. The rule is what the firewall is doing, and a
// failed login is read, not signalled. That 2.13 will push a notification on
// login_failed is not a contradiction: a notification is not a colour.
func TestNoLoginEventIsColoured(t *testing.T) {
	for _, ev := range shared.AllLoginEvents {
		if tone, ok := auditActionTones[string(ev)]; ok {
			t.Errorf("%s is coloured %q; colour describes what the firewall is doing, and a "+
				"login does not change that", ev, tone)
		}
	}
}
