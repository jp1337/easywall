package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// pendingCookieOf picks the pending cookie out of a response, because the
// attacker these tests model keeps that one and nothing else.
func pendingCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == pendingCookieName {
			return c
		}
	}
	t.Fatal("no pending cookie in the response")
	return nil
}

// TestPending_AFrozenCookieDoesNotBuyMoreAttempts is the test the budget did not
// have.
//
// Every test that claimed to hold pendingMaxAttempts ended its round with
// `if c := rec.Result().Cookies(); len(c) > 0 { cookies = c }` — it took the
// cookie the server handed back and presented it next time. An attacker does
// not do that. They pay for one password round, keep the one cookie it issued,
// and post it again, and again: the reviewer measured 200 guesses out of one
// round where 3 were claimed, 52,836/s in process, ~9.5M inside the 180-second
// window against the 3 acceptable codes out of 10^6 that totpWindowLogin
// allows.
//
// So this loop never updates its cookie, and it runs well past
// pendingMaxAttempts. From the third attempt on, every answer must be /login —
// the state is spent, whatever cookie is presented.
func TestPending_AFrozenCookieDoesNotBuyMoreAttempts(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	frozen := pendingCookieOf(t, login)

	const replays = 12
	for i := 1; i <= replays; i++ {
		rec := doFormRequest(s, "POST", "/login/verify", "code=000000", frozen)
		loc := rec.Header().Get("Location")
		want := "/login/verify"
		if i >= pendingMaxAttempts {
			want = "/login"
		}
		if loc != want {
			t.Fatalf("replay %d of the same frozen cookie answered %q, want %q — "+
				"the attempt budget is being read out of the request, so one password "+
				"round buys unlimited code guesses", i, loc, want)
		}
	}
}

// TestPending_AFrozenCookieDoesNotBuyMorePasskeyAttempts is the same replay
// against the other door. The two share one budget by design, so a fix that
// only held on /login/verify would leave the release's own new route as the way
// around it.
func TestPending_AFrozenCookieDoesNotBuyMorePasskeyAttempts(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	frozen := pendingCookieOf(t, login)

	for i := 1; i <= 12; i++ {
		rec := doFormRequest(s, "POST", "/login/passkey/finish",
			"credential=not-a-real-assertion", frozen)
		loc := rec.Header().Get("Location")
		want := "/login/verify"
		if i >= pendingMaxAttempts {
			want = "/login"
		}
		if loc != want {
			t.Fatalf("passkey replay %d of the same frozen cookie answered %q, want %q",
				i, loc, want)
		}
	}
}

// TestPending_TheTwoDoorsShareOneFrozenBudget: two wrong codes and then a
// passkey assertion, all on one unchanging cookie. The third attempt is the
// third whichever door it walks through.
func TestPending_TheTwoDoorsShareOneFrozenBudget(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	frozen := pendingCookieOf(t, login)

	for i := 1; i <= 2; i++ {
		rec := doFormRequest(s, "POST", "/login/verify", "code=000000", frozen)
		if loc := rec.Header().Get("Location"); loc != "/login/verify" {
			t.Fatalf("code attempt %d answered %q, want /login/verify", i, loc)
		}
	}
	rec := doFormRequest(s, "POST", "/login/passkey/finish",
		"credential=not-a-real-assertion", frozen)
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("the passkey door answered %q after two code attempts, want /login — "+
			"the two doors are not sharing one budget", loc)
	}
}

// A recovery code is the way back when the phone is gone, and it goes through
// the same route the budget guards. Two wrong codes first, so it is submitted
// against a state that has already been charged.
func TestPending_ARecoveryCodeStillWorksAgainstAChargedState(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.SaveRecoveryCodes(hashes); err != nil {
		t.Fatal(err)
	}

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	pending := pendingCookieOf(t, login)

	for i := 1; i <= pendingMaxAttempts-1; i++ {
		if loc := doFormRequest(s, "POST", "/login/verify", "code=000000", pending).
			Header().Get("Location"); loc != "/login/verify" {
			t.Fatalf("wrong code %d answered %q, want /login/verify", i, loc)
		}
	}

	rec := doFormRequest(s, "POST", "/login/verify", "code="+plain[0], pending)
	assertRedirect(t, rec, "/dashboard")
}

// A restart empties the map, and the direction that has to be right is which way
// an unknown id falls: a fresh budget for whoever holds the cookie, never a
// refusal. 2.7 decided that a restore which cannot be carried out must not
// become a lockout, and totpReplay makes the same call; an attempt counter that
// survives nothing but locks people out on restart would be the worse half of
// both.
func TestPending_ARestartIsAFreshBudgetNotALockout(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	pending := pendingCookieOf(t, login)

	for i := 1; i <= pendingMaxAttempts-1; i++ {
		doFormRequest(s, "POST", "/login/verify", "code=000000", pending)
	}

	// The restart. Nothing else about the operator's world changes: same cookie,
	// same credentials, same three-minute window.
	pendingAttempts.mu.Lock()
	pendingAttempts.at = make(map[string]pendingCounter)
	pendingAttempts.mu.Unlock()

	if loc := doFormRequest(s, "POST", "/login/verify", "code=000000", pending).
		Header().Get("Location"); loc != "/login/verify" {
		t.Errorf("after a restart the pending state answered %q, want /login/verify — "+
			"an empty map is failing closed and a restart is a lockout", loc)
	}
}

// The sweep, on its own. Entries are held for pendingLifetime and no longer, so
// the map is the failed logins of the last three minutes — the answer to
// newPendingStore's objection that a server-side table is memory a stranger
// occupies.
func TestPending_AttemptEntriesAreSweptAtTheLifetime(t *testing.T) {
	stale := newSessionID()
	pendingAttempts.mu.Lock()
	pendingAttempts.at[stale] = pendingCounter{
		n:     pendingMaxAttempts,
		begun: time.Now().Add(-time.Duration(pendingLifetime+1) * time.Second),
	}
	pendingAttempts.mu.Unlock()

	if n := pendingAttemptCount(stale); n != 0 {
		t.Errorf("an entry older than the lifetime still reads %d attempts, want 0", n)
	}

	// A write sweeps it out of the map entirely, not merely past a read guard.
	recordPendingAttempt(newSessionID())

	pendingAttempts.mu.Lock()
	_, still := pendingAttempts.at[stale]
	pendingAttempts.mu.Unlock()
	if still {
		t.Error("an expired entry survived a write; the map only ever grows")
	}
}

// An id-less cookie is refused rather than granted an uncounted state. That is
// the rolling-upgrade case: a pending cookie signed by the previous binary
// carries no id, and accepting it would reopen the hole for three minutes.
func TestPending_ACookieWithNoIDIsRefused(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	sess, _ := s.pending.Get(req, pendingCookieName)
	sess.Values[pendingUserKey] = "admin"
	sess.Values[pendingCredKey] = "x"
	sess.Values[pendingIssuedKey] = time.Now().Unix()
	if err := sess.Save(req, rec); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest("GET", "/login/verify", nil)
	req2.AddCookie(rec.Result().Cookies()[0])
	if _, ok := s.readPending(req2); ok {
		t.Error("a pending cookie with no id was accepted; its attempts are counted nowhere")
	}
}

// TestPending_ACorrectCodeIsRefusedOnceTheBudgetIsSpent is the half the redirect
// tests above cannot see, and it is the one that decides where the budget is
// enforced.
//
// Those tests submit wrong codes, so the answer is a redirect either way — and
// a build that only checked the count *after* verifying the guess passes all of
// them while still verifying every guess. That is not a budget, it is a
// scoreboard: an attacker replaying a frozen cookie would have their four
// hundredth code checked against the secret and be let in if it matched.
// Verified by mutation: deleting `p.Attempts = pendingAttemptCount(p.ID)` from
// readPending leaves every other test in this file green and only this one red.
//
// So the guess here is genuine. It must not be looked at.
func TestPending_ACorrectCodeIsRefusedOnceTheBudgetIsSpent(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	secret, _ := enrol(t, s)

	login := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	frozen := pendingCookieOf(t, login)

	for i := 1; i <= pendingMaxAttempts; i++ {
		doFormRequest(s, "POST", "/login/verify", "code=000000", frozen)
	}

	rec := doFormRequest(s, "POST", "/login/verify", "code="+currentCode(t, secret), frozen)
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("a valid code on a spent state answered %q, want /login", loc)
	}
	if c := sessionCookie(rec.Result()); c != "" {
		t.Error("a valid code on a spent state granted a session; the budget is counted " +
			"but the guess is still verified, so a frozen cookie is still a brute force")
	}
}
