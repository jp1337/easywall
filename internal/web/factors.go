package web

// How many second factors this account has, and whether one may be taken away.
//
// One file, because the rule is stated once and asked twice — by
// handle2FADisable and by the passkey removal — and a rule written twice is a
// rule whose two copies come to disagree. RequireSecondFactor asks the same
// question in its own words (hasSecondFactor) and gets it from here too, so
// the gate and the guard can never differ about what "has a factor" means.

// factorCount returns the number of enrolled second factors.
//
// TOTP is one or zero; there is a single secret. Passkeys are however many are
// enrolled, because losing a phone should not lose the account. passkeyCount is
// a function rather than a *Config method because the two live in different
// files — the secret in web.toml, the credentials in passkeys.json under
// data_dir — and server.go wires it to the store's own count. That split is
// also why clearing web.toml is no longer the whole way out of a lockout; the
// operator-facing places that say so name passkeys.json too.
//
// Recovery codes are deliberately not counted. They are what is left when both
// factors are gone — a way back in, not a factor. Counting them would let an
// operator satisfy the mandate with eight strings on a printout, which is the
// thing the mandate exists to be better than.
func (s *Server) factorCount() int {
	n := 0
	if s.cfg.TOTPSecret() != "" {
		n++
	}
	n += s.passkeyCount()
	// A passkeys.json that is present and will not read or parse is not an
	// account with no passkeys — it is an account whose passkeys are unknown,
	// and counting that as zero let the password alone sign in to a
	// passkey-only account. One factor, not one per credential: nobody knows
	// how many are in there. Nothing can answer it at the second step, which is
	// exactly why it is safe to count: the TOTP secret and the eight recovery
	// codes live in web.toml and are untouched by whatever happened to this
	// file, so the door this phantom closes is not the door an operator gets
	// back in through. See newPasskeyStore for the rest of the reasoning.
	if s.passkeys != nil && s.passkeys.isCorrupt() {
		n++
	}
	return n
}

// hasSecondFactor reports whether the mandate is satisfied.
func (s *Server) hasSecondFactor() bool { return s.factorCount() > 0 }

// mayRemoveFactor reports whether one factor may be taken away.
//
// The demo is exempt for the same reason it is exempt from the gate: the
// account belongs to nobody, and the point of the demo is that every control
// can be pressed.
func (s *Server) mayRemoveFactor() bool {
	if s.client.IsDemo() {
		return true
	}
	// An unreadable passkeys.json is counted by factorCount, and that count
	// must not license a removal: "TOTP plus a file nobody can read" would
	// otherwise pass as two factors and let the operator switch TOTP off,
	// leaving a real account behind a factor that cannot be presented and only
	// the recovery codes to get in with. Refusing here locks nobody out — the
	// factor it refuses to remove is the working one — and the refusal ends the
	// moment the file is repaired, deleted, or written over by a new enrolment.
	if s.passkeys != nil && s.passkeys.isCorrupt() {
		return false
	}
	return s.factorCount() > 1
}
