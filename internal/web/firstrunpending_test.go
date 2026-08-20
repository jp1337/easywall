package web

import (
	"testing"
	"time"
)

func TestFirstRunPending_RoundTripsAndIsolates(t *testing.T) {
	a := newFirstRunPendingID()
	b := newFirstRunPendingID()
	if a == "" || b == "" || a == b {
		t.Fatalf("ids are not distinct and non-empty: %q %q", a, b)
	}

	firstRunPendingStore(a, pendingFirstRun{
		Answers:      firstRunData{Username: "admin", SSHPort: "2222"},
		PasswordHash: "$argon2id$hash",
		Secret:       "JBSWY3DPEHPK3PXP",
		Issued:       time.Now(),
	})
	t.Cleanup(func() { firstRunPendingClear(a) })

	got, ok := firstRunPendingLookup(a)
	if !ok {
		t.Fatal("a stored entry does not read back")
	}
	if got.Answers.SSHPort != "2222" {
		t.Errorf("the wizard's other answers were lost: %+v", got.Answers)
	}
	if got.PasswordHash != "$argon2id$hash" || got.Secret != "JBSWY3DPEHPK3PXP" {
		t.Error("the hash or the secret did not survive")
	}

	if _, ok := firstRunPendingLookup(b); ok {
		t.Error("one wizard's id reads another's entry")
	}
}

func TestFirstRunPending_ExpiresAndClears(t *testing.T) {
	id := newFirstRunPendingID()
	firstRunPendingStore(id, pendingFirstRun{
		PasswordHash: "$argon2id$hash",
		Issued:       time.Now().Add(-firstRunPendingLifetime - time.Second),
	})
	t.Cleanup(func() { firstRunPendingClear(id) })

	if _, ok := firstRunPendingLookup(id); ok {
		t.Errorf("an entry issued more than %s ago was accepted", firstRunPendingLifetime)
	}

	fresh := newFirstRunPendingID()
	firstRunPendingStore(fresh, pendingFirstRun{PasswordHash: "x", Issued: time.Now()})
	firstRunPendingClear(fresh)
	if _, ok := firstRunPendingLookup(fresh); ok {
		t.Error("a cleared entry still reads back")
	}
}

// Ten minutes has to measure inactivity on the setup step, not total time
// since the QR code first appeared — an operator who is still working through
// "install an authenticator app, scan, mistype twice, read the server time"
// must not have the clock run out from under them while every render of the
// step is itself an activity signal.
func TestFirstRunPending_RefreshExtendsAnActiveEntry(t *testing.T) {
	id := newFirstRunPendingID()
	firstRunPendingStore(id, pendingFirstRun{
		PasswordHash: "$argon2id$hash",
		Issued:       time.Now().Add(-firstRunPendingLifetime + time.Minute),
	})
	t.Cleanup(func() { firstRunPendingClear(id) })

	firstRunPendingRefresh(id)

	p, ok := firstRunPendingLookup(id)
	if !ok {
		t.Fatal("the refreshed entry no longer reads back")
	}
	if time.Since(p.Issued) > time.Second {
		t.Errorf("Issued was not reset to now: %s ago", time.Since(p.Issued))
	}
}

// The refresh must not revive an entry that had already timed out before the
// request offering to refresh it arrived — otherwise a request that only
// exists because the entry was found expired (see firstRunExpired) could
// itself hand that same entry a fresh ten minutes.
func TestFirstRunPending_RefreshCannotReviveAnAlreadyExpiredEntry(t *testing.T) {
	id := newFirstRunPendingID()
	staleIssued := time.Now().Add(-firstRunPendingLifetime - time.Minute)
	firstRunPendingStore(id, pendingFirstRun{
		PasswordHash: "$argon2id$hash",
		Issued:       staleIssued,
	})
	t.Cleanup(func() { firstRunPendingClear(id) })

	firstRunPendingRefresh(id)

	if _, ok := firstRunPendingLookup(id); ok {
		t.Error("an entry already expired before the refresh call was revived by it")
	}
}

func TestFirstRunPending_AnEmptyIDNeverMatches(t *testing.T) {
	firstRunPendingStore("", pendingFirstRun{PasswordHash: "x", Issued: time.Now()})
	if _, ok := firstRunPendingLookup(""); ok {
		t.Error("an empty id resolved to an entry; a request carrying no id would " +
			"then inherit somebody else's half-finished setup")
	}
}
