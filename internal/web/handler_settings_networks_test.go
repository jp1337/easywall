package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The Network page's own validator has to refuse exactly what the core refuses —
// no more and no less. It used to be validateIPListEntries, the blacklist's,
// which accepts a bare address; the core stores these with net.ParseCIDR and
// does not. So the page accepted 192.168.1.5, the core refused it, and the
// operator was shown "Failed to save changes. Check core connection." Measured
// through the real socket:
//
//	web: save settings error error="core error: docker custom network \"192.168.1.5\": not a CIDR network"
func TestTheNetworkEditorRefusesExactlyWhatTheCoreRefuses(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{"plain networks", "172.17.0.0/16\n10.8.0.0/24", true},
		{"a blank line between two groups", "172.17.0.0/16\n\n10.8.0.0/24", true},
		{"a comment, as every other list editor allows", "10.8.0.0/24\n# wireguard peers", true},
		{"an IPv6 network", "2001:db8::/32", true},
		{"a bare address", "192.168.1.5", false},
		{"a hyphen where the slash belongs", "10.9.0.0-24", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pageErrs := validateCIDRListEntries(tc.raw)
			coreErr := shared.ValidateNetworkList("routing network", parseIPList(tc.raw))

			if got := len(pageErrs) == 0; got != tc.valid {
				t.Errorf("the page accepts=%v, want %v (%v)", got, tc.valid, pageErrs)
			}
			if got := coreErr == nil; got != tc.valid {
				t.Errorf("the core accepts=%v, want %v (%v)", got, tc.valid, coreErr)
			}
			// The point of the test: the two halves must never disagree, because
			// the operator only ever sees the generic message when they do.
			if (len(pageErrs) == 0) != (coreErr == nil) {
				t.Errorf("page and core disagree: page=%v core=%v", pageErrs, coreErr)
			}
		})
	}
}

// The demo is the product to everyone who has not installed it. It stored every
// network it was handed, so the public demo reported "Network settings saved."
// for input a real installation refuses.
func TestTheDemoRefusesANetworkTheCoreWouldRefuse(t *testing.T) {
	d := NewDemoClient()

	settings := func(networks ...string) shared.NetworkSettings {
		return shared.NetworkSettings{
			IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
			Docker:  shared.DockerConfig{Enabled: true, CustomNetworks: networks},
			Routing: shared.RoutingConfig{Mode: shared.RoutingClosed},
		}
	}

	if err := d.SaveSettings(settings("192.168.1.5")); err == nil {
		t.Error("the demo accepted a bare address; a real installation refuses it")
	}
	// And it still accepts what the core accepts.
	if err := d.SaveSettings(settings("172.17.0.0/16", "# bridge")); err != nil {
		t.Errorf("the demo refused what the core accepts: %v", err)
	}
}

// localeTranslation returns one message's text from a locale file.
func localeTranslation(t *testing.T, lang, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(localesDir(t), lang+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		ID          string `json:"id"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == id {
			return e.Translation
		}
	}
	t.Fatalf("locales/%s.json has no %q", lang, id)
	return ""
}

// app.js escapes every string it puts on screen, so a message reaching the
// browser through clientStringKeys cannot carry `code` or *emphasis* — the
// markers would be shown to the operator literally. The templates have
// TestMarkupStringsAreRenderedThroughRichText for the same mistake on the server
// side; this is the half that had nothing.
func TestClientStringsCarryNoMarkupAppJSCannotRender(t *testing.T) {
	markupRe := regexp.MustCompile("`[^`]+`|\\*[^*]+\\*")
	for _, lang := range []string{"en", "de"} {
		for _, key := range clientStringKeys {
			text := localeTranslation(t, lang, key)
			if markupRe.MatchString(text) {
				t.Errorf("locales/%s.json: %q is inlined for app.js, which escapes it — "+
					"the markers would be shown literally: %q", lang, key, text)
			}
		}
	}
}
