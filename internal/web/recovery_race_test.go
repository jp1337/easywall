package web

import (
	"net/http"
	"net/url"
	"testing"
)

// TestTwoRecoveryCodesAtOnceBothLeaveTheSet: two different codes, two
// half-logins, submitted together.
//
// handleLoginVerifyPOST read the eight hashes, spent ~300 ms of argon2 finding
// its match, and wrote back the other seven — with no lock across the read and
// the write. Both requests read the same eight, so the second write put the
// first request's code back: both were accepted, the file said 7 where 6 were
// left, and one spent code opened the firewall a second time.
func TestTwoRecoveryCodesAtOnceBothLeaveTheSet(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	_, codes := enrol(t, s)
	pending := [2]*http.Cookie{}
	for i := range pending {
		pending[i] = pendingCookieOf(t, doFormRequest(s, "POST", "/login",
			"username=admin&password="+url.QueryEscape(testPassword)))
	}

	recs := serveConcurrently(s, 2, "/login/verify",
		func(i int) string { return "code=" + url.QueryEscape(codes[i]) },
		func(i int) []*http.Cookie { return []*http.Cookie{pending[i]} })

	for i, rec := range recs {
		if !sessionGrantsAccess(t, s, rec.Result()) {
			t.Errorf("recovery code %d was refused; two different valid codes must both work", i)
		}
	}
	if left := len(s.cfg.RecoveryCodes()); left != len(codes)-2 {
		t.Errorf("%d recovery codes stored after two were used, want %d — one write "+
			"undid the other", left, len(codes)-2)
	}
	for i := 0; i < 2; i++ {
		if _, ok := consumeRecoveryCode(codes[i], s.cfg.RecoveryCodes()); ok {
			t.Errorf("recovery code %d still works after it was used", i)
		}
	}
}

// TestOneRecoveryCodeAtOnceOpensOneSession: the same code, twice, together.
// Single use means one session, whichever request wins.
func TestOneRecoveryCodeAtOnceOpensOneSession(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	_, codes := enrol(t, s)
	pending := [2]*http.Cookie{}
	for i := range pending {
		pending[i] = pendingCookieOf(t, doFormRequest(s, "POST", "/login",
			"username=admin&password="+url.QueryEscape(testPassword)))
	}

	recs := serveConcurrently(s, 2, "/login/verify",
		func(int) string { return "code=" + url.QueryEscape(codes[0]) },
		func(i int) []*http.Cookie { return []*http.Cookie{pending[i]} })

	granted := 0
	for _, rec := range recs {
		if sessionGrantsAccess(t, s, rec.Result()) {
			granted++
		}
	}
	if granted != 1 {
		t.Errorf("one recovery code submitted twice at once opened %d sessions, want 1", granted)
	}
	if left := len(s.cfg.RecoveryCodes()); left != len(codes)-1 {
		t.Errorf("%d recovery codes stored, want %d", left, len(codes)-1)
	}
}
