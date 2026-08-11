package web

import "testing"

// A stored hash that parses but cannot be used must be refused, not crashed on.
//
// decodeArgon2Hash checked the *shape* of the hash and nothing about the numbers
// in it, and argon2.IDKey does not tolerate degenerate parameters — it panics. A
// truncated hash panicked inside blake2b; p=0 and t=0 panic with their own
// messages. middleware.Recoverer turned each into a 500 and a stack trace.
//
// Measured against the running server with the trailing segment of the hash
// missing, which is what a hash truncated while being pasted in looks like:
//
//	before: POST /login -> HTTP 500, panic: invalid memory address or nil pointer
//	        dereference, golang.org/x/crypto/blake2b.(*digest).Write
//	after:  POST /login -> HTTP 303, and one ERROR line naming the problem
//
// This matters more than a malformed config usually would. docs/security.md
// offers no password recovery beyond "editing web.toml on the host is the only
// way back", so the person who mangles that line is already locked out, and a 500
// with a stack trace is the least useful thing to hand them.
func TestVerifyPasswordRefusesUnusableStoredHashes(t *testing.T) {
	for _, tc := range []struct{ name, hash string }{
		{"truncated: empty key", "$argon2id$v=19$m=65536,t=3,p=4$D1FlbbAkz3iEp7GIiVMHbA$"},
		{"empty salt and key", "$argon2id$v=19$m=65536,t=3,p=4$$"},
		{"empty salt", "$argon2id$v=19$m=65536,t=3,p=4$$aGFzaA"},
		{"parallelism zero", "$argon2id$v=19$m=65536,t=3,p=0$c2FsdA$aGFzaA"},
		{"iterations zero", "$argon2id$v=19$m=65536,t=0,p=4$c2FsdA$aGFzaA"},
		{"memory below the lane minimum", "$argon2id$v=19$m=8,t=3,p=4$c2FsdA$aGFzaA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The decode has to reject it, which is what keeps IDKey from ever
			// seeing a value it will panic on.
			if _, _, _, err := decodeArgon2Hash(tc.hash); err == nil {
				t.Fatalf("decodeArgon2Hash accepted %q; argon2.IDKey panics on it, and the "+
					"login handler turns that into a 500 with a stack trace", tc.hash)
			}
			// And the whole call refuses rather than crashing. Without the fix this
			// line panics — which is the failure this test exists to catch.
			if VerifyPassword("an-arbitrary-password", tc.hash) {
				t.Error("an unusable stored hash accepted a password")
			}
		})
	}
}

// The ordinary cases still behave.
func TestVerifyPasswordStillWorksOnRealHashes(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("the right password was refused")
	}
	if VerifyPassword("wrong", hash) {
		t.Error("the wrong password was accepted")
	}
	// An empty password line is the first-run state, not a usable hash.
	if VerifyPassword("", "") {
		t.Error("an empty stored hash accepted an empty password")
	}
}
