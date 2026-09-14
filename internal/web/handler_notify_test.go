package web

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
