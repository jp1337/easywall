package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// savedSession is the cookie a response wrote, ready to send back — the
// browser's half of a redirect, which is all an attacker replaying one has.
func savedSession(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	val := sessionCookie(rec.Result())
	if val == "" {
		t.Fatalf("no %s cookie in the response", SessionName)
	}
	return &http.Cookie{Name: SessionName, Value: val}
}

// expireTheRevocationRecords ages every revocation past its retention, exactly
// as the sweep in revokeSession would once SessionLifetime had passed.
func expireTheRevocationRecords() {
	revokedSessions.mu.Lock()
	for sid := range revokedSessions.at {
		revokedSessions.at[sid] = time.Now().Add(-time.Duration(SessionLifetime+60) * time.Second)
	}
	revokedSessions.mu.Unlock()
	revokeSession(newSessionID()) // triggers the sweep
}

// A logged-out session must not be able to buy itself a fresh signature.
//
// The cookie is signed and self-contained, and the server enforces its age from
// the timestamp inside it. Saving the session re-signs it: whatever the cookie
// carried is written back with the clock reset to now. setFlash saved the
// session the *request* presented, so POST /login with a revoked cookie and a
// deliberately wrong password answered with that same cookie — same `user`,
// same `sid` — aged zero. The revocation record is kept for one SessionLifetime
// and no longer, on the understanding that the cookie is refused on its own age
// after that; the refresh removed exactly that cap. Ten minutes after pressing
// Log out, the refreshed cookie opened the dashboard.
func TestAFlashCannotResurrectALoggedOutSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	const key = "test-session-key-32bytes-padding!"

	// 1. A live session on a machine somebody else also holds the cookie for.
	cookie := makeAuthCookie(t, s)
	if rec := doRequest(s, "GET", "/dashboard", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("setup: the session should start out valid, got %d", rec.Code)
	}

	// 2. The operator presses Log out.
	doRequest(s, "POST", "/logout", nil, cookie)
	if rec := doRequest(s, "GET", "/dashboard", nil, cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("the cookie still worked immediately after logout: %d", rec.Code)
	}

	// 3. One wrong-password login with the revoked cookie attached. The
	//    invalid_credentials flash is written to the session, and the session is
	//    saved — which is where the fresh signature would come from.
	rec := doFormRequest(s, "POST", "/login",
		"username=admin&password=wrong", cookie)
	refreshed := savedSession(t, rec)

	// 4. Ten minutes pass. The original cookie is now past its own lifetime, and
	//    the revocation record has been swept.
	expireTheRevocationRecords()
	aged := backdatedCookie(t, cookie, key, time.Duration(SessionLifetime+60)*time.Second)
	if rec := doRequest(s, "GET", "/dashboard", nil, aged); rec.Code != http.StatusSeeOther {
		t.Fatalf("control: the un-refreshed cookie should be refused on its age, got %d", rec.Code)
	}

	// 5. The refreshed one carries a signature minted after the logout, so
	//    nothing about its age refuses it. Only what is inside it can.
	if rec := doRequest(s, "GET", "/dashboard", nil, refreshed); rec.Code != http.StatusSeeOther {
		t.Errorf("a logged-out session was resurrected: GET /dashboard answered %d "+
			"with a cookie a flash response re-signed after the logout", rec.Code)
	}

	// 6. And the flash it collected still renders, because an anonymous visitor
	//    gets the message too.
	if body := doRequest(s, "GET", "/login", nil, refreshed).Body.String(); !strings.Contains(body, "Invalid username or password") {
		t.Error("the flash set on the stripped session did not render on the login page")
	}
}

// The guard must not throw out the operator it is protecting.
//
// A wrong current password on the change-password form is a flash written to a
// live, signed-in session. That session has no revocation record, so nothing is
// dropped: the message renders and the next request is still signed in.
func TestAFlashOnALiveSessionKeepsTheOperatorSignedIn(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	cookie := makeAuthCookie(t, s)
	rec := doFormRequest(s, "POST", "/password",
		"current_password=nope&new_password=Whatever123!&confirm_password=Whatever123!", cookie)
	assertRedirect(t, rec, "/password")

	after := savedSession(t, rec)
	body := doRequest(s, "GET", "/password", nil, after).Body.String()
	if !strings.Contains(body, "Current password is incorrect") {
		t.Error("the wrong-password flash did not render for a signed-in operator")
	}
	if rec := doRequest(s, "GET", "/dashboard", nil, after); rec.Code != http.StatusOK {
		t.Errorf("a wrong password on the change-password form signed the operator out: %d", rec.Code)
	}
}

// And a visitor who never had a session at all still gets told why the login
// failed.
func TestAnAnonymousFlashStillRenders(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=wrong")
	body := doRequest(s, "GET", "/login", nil, savedSession(t, rec)).Body.String()
	if !strings.Contains(body, "Invalid username or password") {
		t.Error("a failed login with no session did not render its message")
	}
}

// The same trick, one step further along: a cookie that already carries a flash
// gets re-signed by the page that *renders* it, because render clears the flash
// and saves the session to do it. No login attempt is needed — a captured cookie
// caught between a redirect and the page it redirects to carries a flash on its
// own, and a single GET /login was worth a fresh ten minutes.
func TestClearingAFlashCannotResurrectALoggedOutSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	// A live session that has just been told its password was wrong: user, sid
	// and an unread flash, all in the one cookie.
	cookie := makeAuthCookie(t, s)
	withFlash := savedSession(t, doFormRequest(s, "POST", "/password",
		"current_password=nope&new_password=Whatever123!&confirm_password=Whatever123!", cookie))

	doRequest(s, "POST", "/logout", nil, cookie)

	// Rendering the login page consumes the flash — and saves.
	refreshed := savedSession(t, doRequest(s, "GET", "/login", nil, withFlash))

	expireTheRevocationRecords()
	if rec := doRequest(s, "GET", "/dashboard", nil, refreshed); rec.Code != http.StatusSeeOther {
		t.Errorf("a logged-out session was resurrected: GET /dashboard answered %d "+
			"with a cookie render() re-signed while clearing its flash", rec.Code)
	}
}

// The one legitimate request where the guard fires: signing back in from the
// browser that just signed out, with the revoked cookie still in its jar.
//
// handleLoginVerifyPOST's recovery branch saves the session twice — setFlashN
// writes the "7 left" message, then grantSession writes the new user and a new
// sid — and both act on the same session object for the request. Stripping the
// revoked identity in the first save must therefore not survive the second: the
// operator is signed in, and the message they were owed is still there.
func TestSigningBackInAfterALogoutStillWorks(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	_, codes := enrol(t, s)

	old := makeAuthCookie(t, s)
	doRequest(s, "POST", "/logout", nil, old)

	// The same browser, so the revoked cookie rides along with every request.
	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21", old)
	rec := doFormRequest(s, "POST", "/login/verify", "code="+codes[0],
		append(first.Result().Cookies(), old)...)
	assertRedirect(t, rec, "/dashboard")

	granted := savedSession(t, rec)
	if got := doRequest(s, "GET", "/dashboard", nil, granted); got.Code != http.StatusOK {
		t.Fatalf("signing back in after a logout did not produce a working session: %d", got.Code)
	}
	if body := doRequest(s, "GET", "/dashboard", nil, granted).Body.String(); !strings.Contains(body, "Recovery code used") {
		t.Error("the recovery-codes-left message was lost")
	}
}
