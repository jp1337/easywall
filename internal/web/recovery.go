package web

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	// recoveryAlphabet is Crockford base32: the digits and the letters, less
	// I, L, O and U. Thirty-two symbols, so five bits each and no modulo bias
	// when a random byte is reduced.
	recoveryAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	// recoveryGroupLen and the two groups: ten symbols, fifty bits. A starting
	// value, revisable upward — the requirement is that the shape is
	// unambiguous against six digits and that the alphabet excludes what people
	// mistype.
	recoveryGroupLen = 5
	recoveryCodeLen  = 2 * recoveryGroupLen

	// recoveryCodeCount is how many are issued at once, and how many the card
	// counts down from.
	recoveryCodeCount = 8
)

// newRecoveryCodes returns the codes to show the operator once and the argon2
// hashes to store beside the TOTP secret.
//
// Hashed with the same function as the password, and that is deliberate rather
// than convenient: a recovery code is a password with a short life, it is the
// only thing standing between a lost phone and a locked-out firewall, and
// web.toml is a file an operator can be persuaded to paste into an issue.
func newRecoveryCodes() (plain []string, hashes []string, err error) {
	plain = make([]string, 0, recoveryCodeCount)
	hashes = make([]string, 0, recoveryCodeCount)
	seen := make(map[string]bool, recoveryCodeCount)

	for len(plain) < recoveryCodeCount {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		if seen[code] {
			continue // 2^-50 per pair; the loop is cheaper than reasoning about it
		}
		seen[code] = true

		h, err := HashPassword(code)
		if err != nil {
			return nil, nil, fmt.Errorf("hash recovery code: %w", err)
		}
		plain = append(plain, code)
		hashes = append(hashes, h)
	}
	return plain, hashes, nil
}

// newRecoveryCode returns one code in its canonical XXXXX-XXXXX form.
func newRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate recovery code: %w", err)
	}
	var b strings.Builder
	for i, v := range raw {
		if i == recoveryGroupLen {
			b.WriteByte('-')
		}
		// 256 % 32 == 0, so the reduction is uniform.
		b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
	}
	return b.String(), nil
}

// recoveryConfusions are Crockford's own substitutions. The alphabet excludes
// these four so a code cannot contain them; somebody reading one off a printout
// still types them, and refusing that is a lockout with a working code in the
// operator's hand.
var recoveryConfusions = strings.NewReplacer(
	"I", "1", "L", "1", "O", "0", "U", "V",
	" ", "", "-", "",
)

// normaliseRecoveryCode reduces whatever was typed to the canonical form the
// stored hashes were taken over.
func normaliseRecoveryCode(s string) string {
	clean := recoveryConfusions.Replace(strings.ToUpper(strings.TrimSpace(s)))
	if len(clean) != recoveryCodeLen {
		return clean
	}
	return clean[:recoveryGroupLen] + "-" + clean[recoveryGroupLen:]
}

// isTOTPShape reports whether the submitted value is six digits.
func isTOTPShape(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != totpDigits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isRecoveryShape reports whether the submitted value normalises to a recovery
// code. One input field on the verify page takes either shape, and the shape is
// what decides which check runs — so these two must never both be true.
func isRecoveryShape(s string) bool {
	n := normaliseRecoveryCode(s)
	if len(n) != recoveryCodeLen+1 || n[recoveryGroupLen] != '-' {
		return false
	}
	for i, r := range n {
		if i == recoveryGroupLen {
			continue
		}
		if !strings.ContainsRune(recoveryAlphabet, r) {
			return false
		}
	}
	return true
}

// consumeRecoveryCode checks the submitted value against every stored hash and
// returns the set with the matching one removed.
//
// All eight are checked with no early exit on a match — the same stance
// handler_login.go writes out for not short-circuiting argon2 against the name
// comparison. Eight argon2 verifications is roughly 300 ms, which is why the
// route that calls this is bounded by the pending state's three attempts rather
// than left open.
func consumeRecoveryCode(code string, hashes []string) (remaining []string, ok bool) {
	if !isRecoveryShape(code) {
		return hashes, false
	}
	want := normaliseRecoveryCode(code)

	matched := -1
	for i, h := range hashes {
		if VerifyPassword(want, h) && matched < 0 {
			matched = i
		}
	}
	if matched < 0 {
		return hashes, false
	}

	remaining = make([]string, 0, len(hashes)-1)
	remaining = append(remaining, hashes[:matched]...)
	remaining = append(remaining, hashes[matched+1:]...)
	return remaining, true
}
