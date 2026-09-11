package web

import (
	"sync"
	"time"
)

// spentChallenges remembers the WebAuthn challenges a Finish call has already
// consumed, so the same one cannot be presented twice.
//
// Clearing the challenge cookie is an instruction to a browser and nothing
// else. A party holding its own copy of that cookie and of the response it
// belongs to simply sends both again, and go-webauthn is not a backstop for
// that: its UpdateCounter (webauthn/authenticator.go) deliberately exempts an
// authenticator reporting authDataCount == 0 && SignCount == 0, which is what
// iCloud Keychain and most platform passkeys report on *every* assertion — so a
// replayed response verifies with no CloneWarning at all and
// handleLoginPasskeyFinish's clone branch never fires. The server keeping its
// own record of the spent challenge is the only thing that closes it.
//
// In memory and never on disk, on the pattern of pendingSecrets in
// handler_2fa.go: one account, a handful of entries, and no stranger reaches
// it. An empty map after a restart therefore means every challenge is
// unknown-and-unspent, which is the direction this has to fail — the same trade
// newTOTPReplay makes for an unreadable file. Nothing is lost by it: a
// challenge cannot be presented past the lifetime of the cookie carrying it, at
// most five minutes, and anything in flight across a restart is already dead.
var spentChallenges = struct {
	mu sync.Mutex
	at map[string]time.Time
}{at: make(map[string]time.Time)}

// spendChallenge records a challenge as used and reports whether it was fresh.
//
// ttl is how long the record is kept, and the caller passes its own ceremony's
// lifetime: past that the challenge cookie is no longer accepted at all, so
// remembering it any longer would only grow the map. The sweep rides on the
// insert, the way pendingSecretStore's does — nothing here warrants a
// goroutine.
func spendChallenge(challenge string, ttl time.Duration) bool {
	if challenge == "" {
		return false
	}
	now := time.Now()
	spentChallenges.mu.Lock()
	defer spentChallenges.mu.Unlock()
	for c, expires := range spentChallenges.at {
		if now.After(expires) {
			delete(spentChallenges.at, c)
		}
	}
	if _, spent := spentChallenges.at[challenge]; spent {
		return false
	}
	spentChallenges.at[challenge] = now.Add(ttl)
	return true
}
