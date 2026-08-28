package web

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A control whose key no variable names draws nothing. Returning nil rather than
// a zero struct is what makes {{with}} the whole guard in the template — an
// empty marker is a marker, and it is one an operator asks about.
func TestProvenanceForIsNilWhenNoVariableIsSet(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if got := s.provenanceFor("telemetry"); got != nil {
		t.Errorf("provenanceFor = %+v, want nil with no variable set", got)
	}
}

// Every key an environment variable names is either marked in the interface or
// listed as having no control, with a reason. Derived from the table, so the
// fourteenth variable is a decision somebody has to make rather than a marker
// nobody notices is missing.
//
// The shape docs-tech/invariants.md calls for: the list comes from the code, and
// the exceptions carry their reasons where the next reader will find them.
func TestEveryEnvKeyIsMarkedOrExplicitlyHasNoControl(t *testing.T) {
	for _, v := range shared.WebEnvVars {
		if _, ok := keysWithoutAControl[v.TOMLKey]; ok {
			continue
		}
		if _, ok := keysMarkedInTheInterface[v.TOMLKey]; !ok {
			t.Errorf("%s names %q, and the interface neither marks it nor says why "+
				"it has no control", v.Name, v.TOMLKey)
		}
	}
	for key := range keysWithoutAControl {
		if _, ok := shared.WebEnvVar(key); !ok {
			t.Errorf("keysWithoutAControl names %q, which no variable targets", key)
		}
	}
	for key := range keysMarkedInTheInterface {
		if _, ok := shared.WebEnvVar(key); !ok {
			t.Errorf("keysMarkedInTheInterface names %q, which no variable targets", key)
		}
	}
}

// The template a key is marked in has to actually call the marker.
func TestTheMarkedTemplatesCallTheMarker(t *testing.T) {
	for key, tmpl := range keysMarkedInTheInterface {
		body := repoFile2(t, "web", "templates", tmpl)
		if !strings.Contains(body, `{{template "provenance"`) {
			t.Errorf("%s is said to mark %q, and never calls the provenance block", tmpl, key)
		}
	}
}

// keysMarkedInTheInterface maps a TOML key to the template whose control shows
// where its value came from.
var keysMarkedInTheInterface = map[string]string{
	"telemetry": "system.html",
}

// keysWithoutAControl are the keys the interface has no control for, each with
// the reason. A key here is documented on the environment page and nowhere else,
// which is correct — there is no control that could lie about it.
var keysWithoutAControl = map[string]string{
	"bind_addr":    "the address the interface is being read over; changing it from the interface would end the request that changed it",
	"socket_path":  "where the core is reached, decided by the deployment before either process starts",
	"ssl_dir":      "a path, set before the first certificate exists",
	"data_dir":     "a path, set before the first write",
	"tls.cert":     "a path to a file the operator installs outside easywall",
	"tls.key":      "a path to a file the operator installs outside easywall",
	"demo_mode":    "decides whether there is a firewall behind the interface at all; a control for it inside that interface would be answering its own question",
	"update_check": "no control exists yet; the switch is on the 2.x roadmap and the environment page is where it is documented meanwhile",
	"language":     "the switch in the sidebar writes a cookie for this browser, not a stored value — so there is no stored-versus-environment conflict to draw",
}
