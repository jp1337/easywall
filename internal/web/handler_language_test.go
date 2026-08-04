package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// safeRedirect handles an attacker-supplied form field that is written straight
// into a Location header, so every rejection here is load-bearing.
func TestSafeRedirect(t *testing.T) {
	cases := map[string]string{
		// Kept, including the query: /ports carries its open tab in ?type=.
		"/ports":                  "/ports",
		"/ports?type=udp":         "/ports?type=udp",
		"/log?q=apply_rolledback": "/log?q=apply_rolledback",
		"/dashboard":              "/dashboard",
		// A fragment never reaches the server; dropping it loses nothing.
		"/ports#top": "/ports",

		// Off-site, and the reason this function exists.
		"//example.com/":       "/dashboard",
		"//example.com":        "/dashboard",
		"https://example.com/": "/dashboard",
		"http://example.com":   "/dashboard",
		"javascript:alert(1)":  "/dashboard",
		"/\\example.com":       "/dashboard",
		// Kept: this is a local path whose query happens to mention a host. The
		// Location header points at this origin, and easywall has no endpoint that
		// would forward it on. Rejecting it would also have to reject ?type=udp.
		"/redirect?to=//evil.com": "/redirect?to=//evil.com",

		// Header splitting.
		"/ports\r\nSet-Cookie: a=b": "/dashboard",
		"/ports\nX-Evil: 1":         "/dashboard",

		// Nothing useful to return to.
		"":          "/dashboard",
		"/":         "/dashboard",
		"dashboard": "/dashboard",
	}
	for in, want := range cases {
		if got := safeRedirect(in); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}

// safeRedirect is only useful if nothing it returns can leave the origin. The
// value is rebuilt from a parsed path and query, so this asserts the property
// rather than a list of prefixes: whatever goes in, what comes out is local.
func TestSafeRedirect_NeverLeavesTheSite(t *testing.T) {
	for _, in := range []string{
		"//evil.example", "https://evil.example", "http:/evil", "\\\\evil.example",
		"/\t//evil.example", "//", "///evil.example", " //evil.example",
		// Percent-encoded slashes: url.Parse decodes these into Path, so a check
		// that only looked at the raw string would wave them through.
		"/%2f%2fevil.example", "/%2F/evil.example", "%2f%2fevil.example",
		// Userinfo and ports, which carry a host past a naive prefix check.
		"//user:pass@evil.example/", "//evil.example:8443/x",
		// Backslash variants browsers have historically treated as slashes.
		"/\\evil.example", "\\/evil.example", "/\\\\evil.example",
		// Traversal and control characters.
		"/..//evil.example", "/\x00//evil.example",
	} {
		got := safeRedirect(in)
		if !strings.HasPrefix(got, "/") || strings.HasPrefix(got, "//") {
			t.Errorf("safeRedirect(%q) = %q, which is not a local path", in, got)
		}
		if strings.ContainsAny(got, ":\\\r\n") {
			t.Errorf("safeRedirect(%q) = %q, which still carries a dangerous character", in, got)
		}
	}
}

func bundleWith(t *testing.T, files map[string]string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &Config{}
	cfg.Language = "en"
	return &Server{bundle: NewBundle(dir), cfg: cfg}, dir
}

func TestAvailableLangs(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})
	got := AvailableLangs(s.bundle)
	if len(got) != 2 || got[0] != "de" || got[1] != "en" {
		t.Errorf("AvailableLangs = %v, want [de en]", got)
	}
}

// Each language is offered under its own name, whatever the current interface
// language is: a German speaker looks for "Deutsch", not for "German".
func TestLanguageOptions_UsesEndonyms(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})
	req := httptest.NewRequest("GET", "/dashboard", nil)
	opts := s.languageOptions(req, "de")

	if len(opts) != 2 {
		t.Fatalf("got %d options, want 2: %+v", len(opts), opts)
	}
	want := map[string]string{"de": "Deutsch", "en": "English"}
	current := 0
	for _, o := range opts {
		if want[o.Code] != o.Label {
			t.Errorf("%s is labelled %q, want %q", o.Code, o.Label, want[o.Code])
		}
		if o.Current {
			current++
			if o.Code != "de" {
				t.Errorf("marked %q as current, want de", o.Code)
			}
		}
	}
	if current != 1 {
		t.Errorf("%d options marked current, want exactly 1", current)
	}
}

// A single locale means the switcher can only offer what is already showing, so
// it is not drawn at all.
func TestLanguageOptions_HiddenWithOneLocale(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
	})
	req := httptest.NewRequest("GET", "/dashboard", nil)
	if opts := s.languageOptions(req, "en"); opts != nil {
		t.Errorf("expected no switcher for a single locale, got %+v", opts)
	}
}

// A locale file that never names itself must still be selectable rather than
// rendering an empty button.
func TestLanguageOptions_FallsBackToTheCode(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"fr.json": `[{"id":"nav_ports","translation":"Ports"}]`,
	})
	req := httptest.NewRequest("GET", "/dashboard", nil)
	for _, o := range s.languageOptions(req, "en") {
		if o.Code == "fr" && o.Label != "FR" {
			t.Errorf("unnamed locale is labelled %q, want %q", o.Label, "FR")
		}
	}
}

// The cookie describes the person; Accept-Language describes the machine.
func TestResolveLang_CookieBeatsHeader(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	req.AddCookie(&http.Cookie{Name: LangCookie, Value: "de"})
	if got := ResolveLang(s.bundle, req, "en"); got != "de" {
		t.Errorf("with a de cookie and an en header, got %q, want de", got)
	}

	// A cookie naming a language that is not installed must not win, or a stale
	// cookie from a removed locale would pin the interface to raw message ids.
	req2 := httptest.NewRequest("GET", "/dashboard", nil)
	req2.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	req2.AddCookie(&http.Cookie{Name: LangCookie, Value: "fr"})
	if got := ResolveLang(s.bundle, req2, "en"); got != "de" {
		t.Errorf("with an uninstalled cookie language, got %q, want de from the header", got)
	}
}

func TestHandleLanguage_SetsCookieAndRedirects(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})

	body := strings.NewReader("lang=de&return=%2Fports%3Ftype%3Dudp")
	req := httptest.NewRequest("POST", "/language", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleLanguage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/ports?type=udp" {
		t.Errorf("Location = %q, want /ports?type=udp", loc)
	}
	var found *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == LangCookie {
			found = c
		}
	}
	if found == nil {
		t.Fatal("no language cookie was set")
	}
	if found.Value != "de" {
		t.Errorf("cookie value = %q, want de", found.Value)
	}
	if !found.HttpOnly || !found.Secure {
		t.Errorf("cookie should be HttpOnly and Secure, got %+v", found)
	}
}

// An unknown language must not reach the cookie: it would be echoed back on every
// later request and pin the interface to untranslated message ids.
func TestHandleLanguage_RejectsUnknownLanguage(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})

	for _, lang := range []string{"xx", "", "de-DE", "../../etc/passwd", "<script>"} {
		req := httptest.NewRequest("POST", "/language",
			strings.NewReader("lang="+lang+"&return=%2Fports"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		s.handleLanguage(rec, req)

		for _, c := range rec.Result().Cookies() {
			if c.Name == LangCookie {
				t.Errorf("lang=%q was stored in a cookie as %q", lang, c.Value)
			}
		}
		if rec.Code != http.StatusSeeOther {
			t.Errorf("lang=%q: status %d, want a redirect", lang, rec.Code)
		}
	}
}

// The rebuilt value has to survive a real Location header, so this asserts the
// end-to-end property: whatever the form field contains, the browser is sent to
// this origin. url.Parse on the result is the same thing a browser resolves.
func TestHandleLanguage_LocationIsAlwaysLocal(t *testing.T) {
	s, _ := bundleWith(t, map[string]string{
		"en.json": `[{"id":"language_name","translation":"English"}]`,
		"de.json": `[{"id":"language_name","translation":"Deutsch"}]`,
	})

	for _, target := range []string{
		"//evil.example/", "https://evil.example", "/\\evil.example",
		"/%2f%2fevil.example", "//user:pass@evil.example/", "/ports\r\nX-Evil: 1",
		"/ports?type=udp", "/log", "", "/",
	} {
		req := httptest.NewRequest("POST", "/language",
			strings.NewReader("lang=de&return="+url.QueryEscape(target)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		s.handleLanguage(rec, req)

		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Errorf("return=%q produced no Location header", target)
			continue
		}
		u, err := url.Parse(loc)
		if err != nil {
			t.Errorf("return=%q produced an unparsable Location %q: %v", target, loc, err)
			continue
		}
		if u.Scheme != "" || u.Host != "" || u.User != nil {
			t.Errorf("return=%q escaped the origin: Location=%q", target, loc)
		}
		if !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") {
			t.Errorf("return=%q produced a non-local Location %q", target, loc)
		}
	}
}

// isLocalPath is the last guard before a Location header, so it is tested on its
// own rather than only through safeRedirect.
func TestIsLocalPath(t *testing.T) {
	local := []string{"/dashboard", "/ports?type=udp", "/a", "/./x", "/%2e%2e/x"}
	notLocal := []string{
		"", "/", "//", "//evil.example", "/\\evil.example", "\\\\evil.example",
		"dashboard", "https://evil.example", "http:/evil", " /dashboard",
	}
	for _, s := range local {
		if !isLocalPath(s) {
			t.Errorf("isLocalPath(%q) = false, want true", s)
		}
	}
	for _, s := range notLocal {
		if isLocalPath(s) {
			t.Errorf("isLocalPath(%q) = true, want false", s)
		}
	}
}
