package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- see the note on totpAt; HMAC-SHA1, not a digest
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"strings"
	"time"
)

const (
	// totpPeriod is the RFC 6238 time step, and it is fixed. Every authenticator
	// assumes 30 seconds when the otpauth URI does not say otherwise, and ours
	// does say so.
	totpPeriod = 30 * time.Second

	// totpDigits is what the URI declares and what the field on the verify page
	// is sized for.
	totpDigits = 6

	// totpSecretBytes is 160 bits, the length RFC 4226 recommends and every app
	// handles.
	totpSecretBytes = 20

	// totpWindowLogin is the clock tolerance at sign-in: one step either side,
	// 30 seconds of slack in each direction. Wider means a code stays usable
	// longer after it has been shoulder-surfed.
	totpWindowLogin = 1

	// totpWindowEnrol is deliberately much wider, and it is not a security
	// decision: it is how the enrolment page can say "the code is right, but
	// this server's clock is about four minutes behind" instead of "wrong code".
	// Nothing is stored on a match outside ±1.
	totpWindowEnrol = 10
)

// totpSecretEncoding is base32 without padding: the secret is read off a screen
// and typed into a phone, and "=" is noise nobody can place.
var totpSecretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// hotp is RFC 4226 §5.3: HMAC the counter, take the dynamic truncation, reduce
// to the requested number of digits.
//
// newHash is a parameter and the algorithm is *not* configurable anywhere above
// this function. It is here because RFC 6238's Appendix B vectors cover SHA-1,
// SHA-256 and SHA-512, so testing all three costs nothing — and because a
// hand-written implementation that is only ever exercised against one algorithm
// has one third of the RFC's own proof available to it.
func hotp(secret []byte, counter uint64, digits int, newHash func() hash.Hash) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(newHash, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, truncated%mod)
}

// stepAt returns the RFC 6238 counter for t.
func stepAt(t time.Time) uint64 {
	return uint64(t.Unix()) / uint64(totpPeriod/time.Second)
}

// totpAt returns the code easywall accepts: SHA-1, six digits.
//
// SHA-1 is fixed here and in otpauthURI, and it is not a compromise.
// HMAC's security rests on preimage resistance and the PRF property, not on
// collision resistance, so the SHA-1 collision attacks do not reach HMAC-SHA1;
// NIST SP 800-131A Rev. 1 permits SHA-1 for HMAC and forbids it only for
// signatures and timestamps. The Rev. 3 draft deprecates it through
// 31 December 2030 and disallows it after — a date to know, not a date that is
// now.
//
// Interoperability is the actual constraint, in the opposite direction. A
// substantial share of authenticator apps silently ignore the `algorithm=`
// field of an otpauth URI, and Microsoft Authenticator is effectively SHA-1
// only. The failure mode is the wrong one for this audience: the app shows six
// digits, they do not match, and nothing distinguishes a mistyped key from a
// wrong clock from an unhonoured algorithm parameter. A silent failure while
// enrolling a second factor is a lockout.
//
// #nosec G401 -- HMAC-SHA1 per RFC 6238; see above.
func totpAt(secret []byte, step uint64) string {
	return hotp(secret, step, totpDigits, sha1.New)
}

// matchTOTP scans ±window steps around now and reports which one matched.
//
// The whole window is scanned with no early exit, and the first match wins:
// the same stance handler_login.go already writes out for not short-circuiting
// argon2 against the name comparison.
//
// The returned step is the one that *matched*, not the current one — the replay
// store needs that distinction. Storing the current step instead would burn the
// still-valid step N when a code from N-1 was accepted, locking the operator out
// for thirty seconds immediately after a successful login.
func matchTOTP(secret []byte, now time.Time, code string, window int) (step uint64, offset int, ok bool) {
	cur := int64(stepAt(now))
	for d := -window; d <= window; d++ {
		c := cur + int64(d)
		if c < 0 {
			continue
		}
		candidate := totpAt(secret, uint64(c))
		match := subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1
		if match && !ok {
			step, offset, ok = uint64(c), d, true
		}
	}
	return step, offset, ok
}

// newTOTPSecret returns a fresh base32 secret.
func newTOTPSecret() (string, error) {
	raw := make([]byte, totpSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate TOTP secret: %w", err)
	}
	return totpSecretEncoding.EncodeToString(raw), nil
}

// decodeTOTPSecret accepts the secret in any form a human can produce it:
// grouped, lower-cased, padded.
func decodeTOTPSecret(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "=", "").Replace(s))
	raw, err := totpSecretEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("decode TOTP secret: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("decode TOTP secret: it is empty")
	}
	return raw, nil
}

// formatTOTPSecret groups the secret in fours, because it gets copied by hand.
func formatTOTPSecret(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// otpauthURI builds the string the QR code carries.
//
// The label and the issuer both say easywall so an operator with several
// installations can tell them apart in the app's list. algorithm, digits and
// period are stated explicitly even though all three are the defaults: an app
// that reads them gets the truth, and one that ignores them lands on the same
// values anyway.
func otpauthURI(user, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", "easywall")
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", int(totpPeriod/time.Second)))
	return "otpauth://totp/" + url.PathEscape("easywall:"+user) + "?" + q.Encode()
}
