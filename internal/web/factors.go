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
// enrolled, because losing a phone should not lose the account. passkeyCount
// is a function rather than a *Config method because the passkey store does
// not exist yet; a later task points it at that store's own count and nothing
// here changes.
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
	return s.factorCount() > 1
}
