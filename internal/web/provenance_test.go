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

// provenanceFor must keep Env and Stored apart: swapping them would have the
// marker quote the operator's own stored value as "the environment says", and
// nothing short of checking both fields against their actual sources catches
// that — Overridden alone is true either way the two are assigned.
func TestProvenanceForKeepsEnvAndStoredApart(t *testing.T) {
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if err := s.cfg.SaveTelemetry(false); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}

	got := s.provenanceFor("telemetry")
	if got == nil {
		t.Fatal("provenanceFor = nil, want a marker with the variable set")
	}
	if got.Variable != "EASYWALL_WEB_TELEMETRY" {
		t.Errorf("Variable = %q, want the variable's name", got.Variable)
	}
	if got.Env != "true" {
		t.Errorf("Env = %q, want the environment's value %q", got.Env, "true")
	}
	if got.Stored != "false" {
		t.Errorf("Stored = %q, want the operator's stored value %q", got.Stored, "false")
	}
	if !got.Overridden {
		t.Error("Overridden = false, want true: the stored value disagrees with the environment")
	}
}

// The rendered /system page for the three states the marker distinguishes: no
// variable set (nothing drawn), a variable agreeing with the stored value (the
// plain form, no button), and a variable a stored value is beating (the
// conflict form and the reset button). Exercising the real page — base.html's
// {{define "provenance"}} through system.html's {{with .TelemetryProv}} —
// rather than calling provenanceFor in isolation, because the marker's logic
// living in provenance.go and the template's choice of branch in base.html are
// two different places a mutation can hide, and only the second is caught by
// executing the template.
func TestSystemPage_ProvenanceMarkerRendersEachState(t *testing.T) {
	t.Run("no_variable_set", func(t *testing.T) {
		fc := newFakeCore(t)
		s := newTestServer(t, fc)

		rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
		body := rec.Body.String()
		if strings.Contains(body, `class="provenance"`) {
			t.Errorf("the marker was drawn with no environment variable set:\n%s", body)
		}
	})

	t.Run("agrees_with_stored", func(t *testing.T) {
		t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		if err := s.cfg.SaveTelemetry(true); err != nil {
			t.Fatalf("SaveTelemetry: %v", err)
		}

		rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
		body := rec.Body.String()
		if !strings.Contains(body, `class="provenance"`) {
			t.Fatalf("the marker was not drawn with the variable set:\n%s", body)
		}
		if strings.Contains(body, `name="reset"`) {
			t.Error("a reset button was drawn where the stored value agrees with the environment")
		}
		if strings.Contains(body, "provenance-conflict") {
			t.Error("the conflict form was drawn where there is no conflict")
		}
	})

	t.Run("stored_value_overrides", func(t *testing.T) {
		t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		if err := s.cfg.SaveTelemetry(false); err != nil {
			t.Fatalf("SaveTelemetry: %v", err)
		}

		rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
		body := rec.Body.String()
		if !strings.Contains(body, "provenance-conflict") {
			t.Fatalf("the conflict form was not drawn for a stored value that disagrees:\n%s", body)
		}
		if !strings.Contains(body, `name="reset"`) {
			t.Error("no reset button was drawn for a conflicting stored value")
		}
		// Pins the message to the environment's own value. Swapping Env and
		// Stored in provenanceFor leaves Overridden (and so which branch draws)
		// unchanged, but would quote the stored "false" here instead of the
		// environment's "true".
		if !strings.Contains(body, "EASYWALL_WEB_TELEMETRY = true") {
			t.Errorf("the conflict text does not quote the environment's own value:\n%s", body)
		}
	})
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
