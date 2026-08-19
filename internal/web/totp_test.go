package web

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B.
//
// All three algorithms, because the RFC's own vectors cover all three and
// testing them is therefore free — and because this is exactly where a
// hand-written TOTP goes wrong: the RFC uses seeds of *different lengths* per
// algorithm (20 / 32 / 64 bytes), not one shared seed. Feeding the 20-byte seed
// to SHA-256 produces numbers that look plausible and are wrong.
func TestTOTP_RFC6238Vectors(t *testing.T) {
	algs := []struct {
		name    string
		seed    string
		newHash func() hash.Hash
	}{
		{"SHA1", "12345678901234567890", sha1.New},
		{"SHA256", "12345678901234567890123456789012", sha256.New},
		{"SHA512", "1234567890123456789012345678901234567890123456789012345678901234", sha512.New},
	}

	// T, then the eight-digit value for SHA1, SHA256, SHA512.
	vectors := []struct {
		sec                  int64
		sha1, sha256, sha512 string
	}{
		{59, "94287082", "46119246", "90693936"},
		{1111111109, "07081804", "68084774", "25091201"},
		{1111111111, "14050471", "67062674", "99943326"},
		{1234567890, "89005924", "91819424", "93441116"},
		{2000000000, "69279037", "90698825", "38618901"},
		{20000000000, "65353130", "77737706", "47863826"},
	}

	for _, v := range vectors {
		step := stepAt(time.Unix(v.sec, 0).UTC())
		want := []string{v.sha1, v.sha256, v.sha512}
		for i, a := range algs {
			got := hotp([]byte(a.seed), step, 8, a.newHash)
			if got != want[i] {
				t.Errorf("T=%d %s: got %s, want %s", v.sec, a.name, got, want[i])
			}
			// The six-digit form easywall actually uses is the low six digits of
			// the same truncation, and the assertion is derived rather than
			// copied so the two cannot drift.
			if got6, want6 := hotp([]byte(a.seed), step, 6, a.newHash), want[i][2:]; got6 != want6 {
				t.Errorf("T=%d %s six-digit: got %s, want %s", v.sec, a.name, got6, want6)
			}
		}
	}
}

// The step is the counter, and it is what the replay store persists. A wrong
// step arithmetic is invisible in a single code and locks an operator out at a
// boundary.
func TestStepAt_IsThirtySecondsFromTheEpoch(t *testing.T) {
	for _, tc := range []struct {
		sec  int64
		want uint64
	}{{0, 0}, {29, 0}, {30, 1}, {59, 1}, {60, 2}, {1234567890, 41152263}} {
		if got := stepAt(time.Unix(tc.sec, 0).UTC()); got != tc.want {
			t.Errorf("stepAt(%d) = %d, want %d", tc.sec, got, tc.want)
		}
	}
}

// ±1 is accepted at login, ±2 is not. That is the whole clock tolerance an
// operator gets, and widening it silently is how a stolen code stays usable.
func TestMatchTOTP_WindowIsOneStepEitherSide(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0).UTC()
	cur := stepAt(now)

	for _, d := range []int{-1, 0, 1} {
		code := totpAt(secret, uint64(int64(cur)+int64(d)))
		step, offset, ok := matchTOTP(secret, now, code, totpWindowLogin)
		if !ok {
			t.Fatalf("offset %d was refused inside the window", d)
		}
		if offset != d {
			t.Errorf("offset %d reported as %d", d, offset)
		}
		if step != uint64(int64(cur)+int64(d)) {
			t.Errorf("step for offset %d is %d, want %d", d, step, uint64(int64(cur)+int64(d)))
		}
	}
	for _, d := range []int{-2, 2, 5} {
		code := totpAt(secret, uint64(int64(cur)+int64(d)))
		if _, _, ok := matchTOTP(secret, now, code, totpWindowLogin); ok {
			t.Errorf("offset %d was accepted; the login window is ±1", d)
		}
	}
}

// The enrolment window is wider on purpose, and the sign is what turns "wrong
// code" into "this server's clock is four minutes behind".
func TestMatchTOTP_EnrolmentWindowReportsSignAndMagnitude(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0).UTC()
	cur := stepAt(now)

	code := totpAt(secret, uint64(int64(cur)+8)) // the app is 4 minutes ahead of us
	_, offset, ok := matchTOTP(secret, now, code, totpWindowEnrol)
	if !ok {
		t.Fatal("a code 8 steps out was refused at enrolment")
	}
	if offset != 8 {
		t.Errorf("offset reported as %d, want +8 — the sign says which way the clock is wrong", offset)
	}
}

func TestTOTPSecret_RoundTripsAndIsReadable(t *testing.T) {
	s, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := decodeTOTPSecret(s)
	if err != nil {
		t.Fatalf("a secret this package generated does not decode: %v", err)
	}
	if len(raw) != totpSecretBytes {
		t.Errorf("secret is %d bytes, want %d", len(raw), totpSecretBytes)
	}
	if strings.ContainsAny(s, "=") {
		t.Error("the secret carries base32 padding; it is typed in by hand and read off a screen")
	}
	// It is typed in by hand, so spaces and lower case have to survive.
	spaced := strings.ToLower(formatTOTPSecret(s))
	again, err := decodeTOTPSecret(spaced)
	if err != nil {
		t.Fatalf("the grouped, lower-cased form does not decode: %v", err)
	}
	if string(again) != string(raw) {
		t.Error("the grouped form decodes to a different secret")
	}
}

func TestOtpauthURI_NamesSHA1Explicitly(t *testing.T) {
	uri := otpauthURI("admin", "JBSWY3DPEHPK3PXP")
	for _, want := range []string{
		"otpauth://totp/easywall:admin",
		"secret=JBSWY3DPEHPK3PXP",
		"issuer=easywall",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("otpauth URI %q is missing %q", uri, want)
		}
	}
}
