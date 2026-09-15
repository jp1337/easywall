package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// The page's whole job: what the operator typed is what the file holds, and
// what the file holds is what the page shows them next time. Both halves in
// one test, because a form that stores correctly and renders an empty field
// back is indistinguishable from one that stored nothing.
func TestSavingTheNotificationSettingsStoresThemAndRendersThemBack(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	// The second-factor gate sends an unenrolled session to /password before a
	// handler inside the protected group ever runs — see RequireSecondFactor.
	enrollFactor(t, s)
	form := url.Values{
		"kind": {"ntfy"}, "url": {"https://ntfy.example/t"},
		"on_rolled_back": {"on"}, "on_panic": {"on"},
	}
	rr := doAuthFormRequest(t, s, "/notify", form.Encode())
	if rr.Code >= 400 {
		t.Fatalf("POST /notify = %d", rr.Code)
	}
	// Through the accessors, not the fields: this server is never Started, so a
	// bare read is safe here and nowhere the next test copied from this one
	// might put it.
	kind, addr := s.cfg.NotifyDestination()
	if kind != "ntfy" || addr != "https://ntfy.example/t" {
		t.Fatalf("not stored: kind=%q url=%q", kind, addr)
	}
	if !s.cfg.NotifyEnabled("rolled_back") || s.cfg.NotifyEnabled("accepted") ||
		!s.cfg.NotifyEnabled("panic") || s.cfg.NotifyEnabled("failed_logins") {
		t.Fatalf("the triggers are not stored as ticked: %+v", s.cfg.WebConfig)
	}
	body := doRequest(s, "GET", "/notify", nil, makeAuthCookie(t, s)).Body.String()
	if !strings.Contains(body, "https://ntfy.example/t") {
		t.Error("the saved URL is not rendered back on the page")
	}
	// The other direction of TestTheDemoNeverBuildsANotifier's assertion. That
	// one pins the banner present in demo mode; without this one an inverted
	// {{if .Demo}} would keep it green while telling every real operator their
	// firewall sends nothing.
	if strings.Contains(body, `data-testid="notify-demo-banner"`) {
		t.Error("the demo callout is rendered on a page that is not the demo")
	}
}

// file:// is not a web endpoint, and the notifier would hand it to an
// http.Client that cannot dial it. Refused before it reaches the file.
func TestAURLThatIsNotHTTPIsRefused(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	form := url.Values{"kind": {"webhook"}, "url": {"file:///etc/passwd"}}
	_ = doAuthFormRequest(t, s, "/notify", form.Encode())
	if _, addr := s.cfg.NotifyDestination(); addr == "file:///etc/passwd" {
		t.Fatal("a non-HTTP scheme was stored")
	}
}

// The test button ignores the four switches on purpose: an operator pressing
// it has asked for exactly one delivery, right now, and a test that stays
// silent because a trigger is off would prove nothing about the endpoint.
func TestTheTestButtonPostsImmediatelyAndIgnoresTheSwitches(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	if err := s.cfg.SaveNotifications("webhook", srv.URL, false, false, false, false); err != nil { // every switch off
		t.Fatal(err)
	}
	s.rebuildNotifier()

	// The button is a submit inside the settings form, so the browser posts the
	// whole form with it — see TestTheTestButtonSendsToTheAddressInTheForm.
	rr := doAuthFormRequest(t, s, "/notify/test",
		url.Values{"kind": {"webhook"}, "url": {srv.URL}}.Encode())
	if rr.Code >= 400 {
		t.Fatalf("POST /notify/test = %d", rr.Code)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatal("the test button did not send — a test that respects the switches cannot prove the endpoint works")
	}
}

// A destination with no address is notifications aimed at nothing: newNotifier
// refuses to build without both halves, so the four triggers would sit ticked
// and silent while the page reported a successful save. "Nothing" is how this
// is switched off.
func TestADestinationWithNoAddressIsRefused(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	form := url.Values{"kind": {"ntfy"}, "url": {""}, "on_panic": {"on"}}
	_ = doAuthFormRequest(t, s, "/notify", form.Encode())
	if kind, _ := s.cfg.NotifyDestination(); kind != "" {
		t.Fatalf("a destination with no address was stored: kind=%q", kind)
	}
	// And refused for this reason. "Nothing was stored" is also true of a
	// refusal by notify_kind_invalid or by save_error, so the negative on its
	// own cannot tell the operator's actual mistake from any other one — the
	// key is what decides which sentence they read.
	rec := doAuthFormHTMX(t, s, "/notify", form.Encode())
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "notify_url_required") {
		t.Errorf("HX-Trigger = %q, want the notify_url_required key: a refusal for "+
			"another reason stores nothing either", trigger)
	}
}

// The test button has four outcomes and only the happy one had a behavioural
// test: the other three were held up by the registration guard below, which
// proves each key is spelled and coloured everywhere it is shown and nothing
// about which branch produces it. A reordering that put the notifier check in
// front of the demo check, or a refusal that stopped refusing, would have been
// green.
//
// The second demo row needs a notifier present, which rebuildNotifier refuses
// to build in demo mode — so it is set directly rather than through it.
//
// HX-Trigger, not a rendered sentence: the key is the stable artefact and the
// copy is edited.
func TestTheTestButtonsThreeRefusalsAndItsSend(t *testing.T) {
	cases := []struct {
		name    string
		demo    bool
		status  int // what the endpoint answers; 0 means the form carries no address
		wantKey string
		wantHit bool
	}{
		// Two demo rows, and the first is the one that carries the ordering:
		// with an empty form, a demo that looked at the address first answers
		// notify_not_configured — true, and the wrong sentence for a visitor who
		// is not being refused for that — and a demo that dropped the check
		// altogether answers it as well. Both mutations die on row A alone.
		//
		// Row B earns its place against a third: `if demo && url == ""`, the
		// plausible way somebody merges the two conditions while "improving"
		// this handler. Row A passes it — no address, so it still refuses — and
		// row B sends to the endpoint. Run, and it goes red on row B only.
		{"the demo says so before it looks at the address at all", true, 0, "notify_demo_no_send", false},
		{"the demo refuses even with an address submitted", true, 200, "notify_demo_no_send", false},
		{"no destination in the form is refused, not sent", false, 0, "notify_not_configured", false},
		{"an endpoint that answers 500 is reported as a failed send", false, 500, "notify_test_failed", true},
		{"a destination that answers 200 sends", false, 200, "notify_test_sent", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			var s *Server
			if tc.demo {
				s = newDemoTestServer(t)
			} else {
				s = newTestServer(t, newFakeCore(t))
			}
			// Without this the gate 303s the POST to /password and every
			// assertion below passes on a page that never ran the handler.
			enrollFactor(t, s)
			// The form the button submits, not a planted s.notify: the handler
			// dials what was typed, and nothing is saved by any of these rows.
			body := ""
			if tc.status != 0 {
				body = url.Values{"kind": {"webhook"}, "url": {srv.URL}}.Encode()
			}

			rec := doAuthFormHTMX(t, s, "/notify/test", body)
			if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, tc.wantKey) {
				t.Errorf("HX-Trigger = %q, want %q", trigger, tc.wantKey)
			}
			if got := atomic.LoadInt32(&hits) > 0; got != tc.wantHit {
				t.Errorf("endpoint hit = %v, want %v", got, tc.wantHit)
			}
		})
	}
}

// Every outcome this page can produce has to be registered in three places, and
// missing any one of them is invisible until an operator reads a message id out
// of a toast.
//
//	clientStringKeys  the translation is inlined into the page for app.js
//	app.js messages   the HTMX path: show() falls back to `{ text: key }`, so an
//	                  unregistered key is printed literally, in every language
//	flashClass        the no-JavaScript path: notify.html carries method="POST"
//	                  as its fallback, and any key outside successKeys/warningKeys
//	                  renders alert-crit — red, with an error icon
//
// Both halves of this were live at once on the Notifications page: every save
// raised a toast reading "notify_saved", and the same save on the fallback path
// rendered that message in red. TestClientStringsCoverWhatAppJSAsksFor checks
// only that app.js asks for nothing unshipped, which is the other direction.
func TestEveryNotifyOutcomeIsRegisteredEverywhereItIsShown(t *testing.T) {
	want := map[string]string{
		"notify_saved":        "alert-ok",
		"notify_kind_invalid": "alert-warn",
		"notify_url_invalid":  "alert-warn",
		"notify_url_required": "alert-warn",
		// The test button's four outcomes, registered the same way.
		"notify_test_sent":      "alert-ok",
		"notify_test_failed":    "alert-warn",
		"notify_not_configured": "alert-warn",
		"notify_demo_no_send":   "alert-warn",
		// The demo's refusal to save them. Shared with /password, but /notify
		// is the first page to raise it over HTMX, where a flash never renders
		// and an unshipped key prints itself into the toast.
		"demo_readonly": "alert-warn",
	}

	shipped := make(map[string]bool, len(clientStringKeys))
	for _, k := range clientStringKeys {
		shipped[k] = true
	}

	// The keys of app.js's `messages` object literal — the map show() looks in.
	root := filepath.Dir(localesDir(t))
	appJS, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	block := regexp.MustCompile(`(?s)const messages = \{(.*?)\n  \};`).FindStringSubmatch(string(appJS))
	if block == nil {
		t.Fatal("app.js has no `const messages = {` block — the regex or app.js changed shape")
	}
	registered := make(map[string]bool)
	for _, m := range regexp.MustCompile(`(?m)^\s{4}([a-z0-9_]+):`).FindAllStringSubmatch(block[1], -1) {
		registered[m[1]] = true
	}
	if len(registered) < 5 {
		t.Fatalf("found %d keys in app.js's messages map; the regex no longer matches how "+
			"they are written, so this test would pass by finding nothing", len(registered))
	}

	flashClass, ok := templateFuncs()["flashClass"].(func(string) string)
	if !ok {
		t.Fatal("templateFuncs has no flashClass(string) string")
	}

	for key, class := range want {
		if !shipped[key] {
			t.Errorf("clientStringKeys does not ship %q, so app.js has no translation for it", key)
		}
		if !registered[key] {
			t.Errorf("app.js's messages map has no %q — its toast would print the id itself", key)
		}
		if got := flashClass(key); got != class {
			t.Errorf("flashClass(%q) = %q, want %q — the no-JavaScript flash is the wrong colour", key, got, class)
		}
	}
}

// TestTheDemoNeverBuildsANotifier is the other half of the two demo-mode
// guards this page carries: TestDemoModeBuildsNoNotifier (notifyrun_test.go)
// proves rebuildNotifier(), TestPasswordDemoRefusesTheTestButton-style tests
// prove the button; this one proves the page tells a visitor why before they
// press anything.
func TestTheDemoNeverBuildsANotifier(t *testing.T) {
	s := newDemoTestServer(t)
	if err := s.cfg.SaveNotifications("webhook", "https://example.invalid/hook", true, true, true, true); err != nil {
		t.Fatal(err)
	}
	s.rebuildNotifier()
	// currentNotifier(), never a bare s.notify read — Task 5 put the field
	// behind notifyMu and -race found the bare version.
	if s.currentNotifier() != nil {
		t.Fatal("demo mode built a notifier; the public demo must make no outbound request")
	}
	// And the page says so rather than looking broken. data-testid, not the
	// English copy: this assertion must not break when the wording is edited.
	// Not a bare `strings.Contains(body, "demo")` either — the topbar's own
	// demo-chip (base.html, present on every authenticated demo page) puts the
	// substring "demo" in the markup via its `demo-chip` class regardless of
	// whether this page explains anything, which is exactly the kind of
	// vacuous assertion the mutation check below catches.
	body := doRequest(s, "GET", "/notify", nil, makeAuthCookie(t, s)).Body.String()
	if !strings.Contains(body, `data-testid="notify-demo-banner"`) {
		t.Error("the demo page does not carry the demo callout")
	}
}

// The button is a type="submit" inside the settings form and the page promises
// it sends "to the address in the field". It read s.currentNotifier() instead,
// which rebuildNotifier only replaces after a successful *save* — so an
// operator who typed a new address and pressed Send a test without saving got
// "Test sent." for a delivery the old endpoint received. They then had every
// reason to believe the new address works.
//
// Two endpoints, because the assertion that matters is not "B was hit" on its
// own — a handler that sent to both would pass that — but that A was not.
func TestTheTestButtonSendsToTheAddressInTheForm(t *testing.T) {
	var savedHits, typedHits int32
	saved := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&savedHits, 1)
	}))
	defer saved.Close()
	typed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&typedHits, 1)
	}))
	defer typed.Close()

	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	if err := s.cfg.SaveNotifications("webhook", saved.URL, true, true, true, true); err != nil {
		t.Fatal(err)
	}
	s.rebuildNotifier()

	rec := doAuthFormHTMX(t, s, "/notify/test",
		url.Values{"kind": {"webhook"}, "url": {typed.URL}}.Encode())
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "notify_test_sent") {
		t.Errorf("HX-Trigger = %q, want notify_test_sent", trigger)
	}
	if n := atomic.LoadInt32(&typedHits); n != 1 {
		t.Errorf("the address in the form was hit %d times, want 1", n)
	}
	if n := atomic.LoadInt32(&savedHits); n != 0 {
		t.Errorf("the saved address was hit %d times; the operator is told the address "+
			"they typed works when it was never dialled", n)
	}
	// A test proves an address; it does not commit one. The stored destination
	// is untouched and so is the live notifier.
	if _, addr := s.cfg.NotifyDestination(); addr != saved.URL {
		t.Errorf("the stored address is now %q — the test button saved it", addr)
	}
	if n := s.currentNotifier(); n == nil || n.url != saved.URL {
		t.Error("the test button rebuilt the live notifier; a test must not change what is stored")
	}
}

// An address the save path refuses is not an address the test button should
// dial, and two different answers for one typo is a worse page. Same
// validNotifyURL, same two keys.
func TestTheTestButtonRefusesWhatTheSaveWouldRefuse(t *testing.T) {
	for _, tc := range []struct{ name, body, wantKey string }{
		{"an unknown kind", "kind=carrier-pigeon&url=https://example.invalid/h", "notify_kind_invalid"},
		{"a scheme that is not a web endpoint", "kind=webhook&url=file:///etc/passwd", "notify_url_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, newFakeCore(t))
			enrollFactor(t, s)
			rec := doAuthFormHTMX(t, s, "/notify/test", tc.body)
			if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, tc.wantKey) {
				t.Errorf("HX-Trigger = %q, want %q", trigger, tc.wantKey)
			}
		})
	}
}
