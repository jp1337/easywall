package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The reason not to add a field to the existing session is the bug middleware.go
// documents above sessionUser: two places answering "signed in" differently
// produced a redirect loop that ended in ERR_TOO_MANY_REDIRECTS. A cookie that
// is not a session cookie cannot answer that question wrongly — and this test is
// what holds that.
func TestPending_AuthenticatesNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login/verify", nil)
	if err := s.writePending(rec, req, pendingLogin{User: "admin", CredFP: "x", IssuedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("writePending set no cookie")
	}

	got := doRequest(s, "GET", "/dashboard", nil, cookies[0])
	assertRedirect(t, got, "/login")
}

// Path=/login, so it is not sent anywhere else. A cookie that reaches /ports is
// a cookie in a proxy log that had no reason to see it.
func TestPending_IsScopedToTheLoginPath(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	_ = s.writePending(rec, req, pendingLogin{User: "admin", CredFP: "x", IssuedAt: time.Now().Unix()})

	c := rec.Result().Cookies()[0]
	if c.Name != pendingCookieName {
		t.Errorf("the pending cookie is called %q, want %q — it must not be the session cookie", c.Name, pendingCookieName)
	}
	if c.Path != "/login" {
		t.Errorf("pending cookie Path is %q, want /login", c.Path)
	}
	if !c.HttpOnly {
		t.Error("the pending cookie is readable from JavaScript")
	}
	if c.MaxAge != pendingLifetime {
		t.Errorf("pending cookie Max-Age is %d, want %d", c.MaxAge, pendingLifetime)
	}
}

// store.MaxAge, never Options.MaxAge. auth.go's comment on newSessionStore
// records what the other one cost: assigning a fresh Options struct leaves the
// codec's own thirty-day max age in place, which is how a logged-out cookie
// stayed valid to the server for a month. The browser-facing Max-Age is checked
// above; this is the half the *server* enforces.
//
// This is read rather than exercised, and both halves of that are deliberate.
//
// It cannot be proved by waiting: securecookie compares whole Unix seconds with
// `t1 < t2-maxAge` (securecookie.go:339), so a 180-second max age takes 181
// seconds to observe. And it cannot be proved by shortening the max age first,
// because calling store.MaxAge(1) in the test sets the codec *regardless of how
// newPendingStore built it* — the test would then pass against the very bug it
// exists to catch. securecookie carries a timeFunc field for exactly this kind
// of test, but exports no way to set it.
//
// So the field is read directly. reflect can read an unexported field through
// Int(), which does not carry the restriction Interface() does, and reading the
// concrete value behind the Codec interface avoids importing securecookie and
// promoting it out of the indirect block in go.mod. If an upgrade renames the
// field this fails loudly — which is the right moment to re-check the invariant,
// rather than the quiet moment to lose it.
func TestPending_TheServerEnforcesTheShortLifetimeToo(t *testing.T) {
	store := newPendingStore("test-session-key-32bytes-padding!")

	if store.Options.MaxAge != pendingLifetime {
		t.Errorf("pending store Options.MaxAge is %d, want %d",
			store.Options.MaxAge, pendingLifetime)
	}
	if len(store.Codecs) == 0 {
		t.Fatal("the pending store has no codecs, so nothing verifies its cookies")
	}

	v := reflect.ValueOf(store.Codecs[0])
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	f := v.FieldByName("maxAge")
	if !f.IsValid() || f.Kind() != reflect.Int64 {
		t.Fatalf("securecookie's codec no longer has an int64 maxAge field (kind %v); "+
			"this guard needs rewriting, and the invariant behind it — that the codec's "+
			"max age is set and not only Options — needs checking by hand first", f.Kind())
	}
	if got := f.Int(); got != int64(pendingLifetime) {
		t.Errorf("the codec enforces a max age of %d seconds, want %d. The store was "+
			"built with Options.MaxAge rather than store.MaxAge, so the server will keep "+
			"accepting a pending cookie long after the browser has dropped it — the same "+
			"bug newSessionStore's comment describes", got, pendingLifetime)
	}
}

func TestPending_AForgedCounterIsInvalid(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	_ = s.writePending(rec, req, pendingLogin{User: "admin", CredFP: "x", Attempts: 2, IssuedAt: time.Now().Unix()})
	c := rec.Result().Cookies()[0]

	// Flip a byte in the middle of the signed value.
	b := []byte(c.Value)
	b[len(b)/2] ^= 0x01
	tampered := &http.Cookie{Name: c.Name, Value: string(b), Path: c.Path}

	req2 := httptest.NewRequest("GET", "/login/verify", nil)
	req2.AddCookie(tampered)
	if _, ok := s.readPending(req2); ok {
		t.Error("a tampered pending cookie was accepted; the attempt counter is forgeable")
	}
}

func TestPending_ExpiresAfterItsLifetime(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	old := time.Now().Add(-time.Duration(pendingLifetime+1) * time.Second).Unix()
	_ = s.writePending(rec, req, pendingLogin{User: "admin", CredFP: "x", IssuedAt: old})

	req2 := httptest.NewRequest("GET", "/login/verify", nil)
	req2.AddCookie(rec.Result().Cookies()[0])
	if _, ok := s.readPending(req2); ok {
		t.Errorf("a pending state issued %ds ago was accepted; the lifetime is %ds",
			pendingLifetime+1, pendingLifetime)
	}
}

func TestPending_RoundTrips(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	want := pendingLogin{User: "admin", CredFP: "abc123", Attempts: 2, IssuedAt: time.Now().Unix()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	if err := s.writePending(rec, req, want); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest("GET", "/login/verify", nil)
	req2.AddCookie(rec.Result().Cookies()[0])
	got, ok := s.readPending(req2)
	if !ok {
		t.Fatal("a pending state this server wrote does not read back")
	}
	if got.User != want.User || got.CredFP != want.CredFP || got.Attempts != want.Attempts {
		t.Errorf("read back %+v, want %+v", got, want)
	}
}

func TestPending_ClearingItEndsIt(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	s.clearPending(rec, req)

	c := rec.Result().Cookies()
	if len(c) == 0 {
		t.Fatal("clearPending set no cookie at all; the browser is never told to drop it")
	}
	if c[0].MaxAge >= 0 {
		t.Errorf("clearPending left Max-Age at %d; it must be negative to delete", c[0].MaxAge)
	}
	if !strings.Contains(rec.Result().Header.Get("Set-Cookie"), pendingCookieName) {
		t.Error("clearPending did not name the pending cookie")
	}
}
