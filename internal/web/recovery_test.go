package web

import (
	"regexp"
	"strings"
	"testing"
)

var recoveryShapeRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{5}-[0-9A-HJKMNP-TV-Z]{5}$`)

func TestRecoveryCodes_EightDistinctCodesOfTheRightShape(t *testing.T) {
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != recoveryCodeCount || len(hashes) != recoveryCodeCount {
		t.Fatalf("got %d codes and %d hashes, want %d of each", len(plain), len(hashes), recoveryCodeCount)
	}
	seen := map[string]bool{}
	for _, c := range plain {
		if !recoveryShapeRe.MatchString(c) {
			t.Errorf("%q is not XXXXX-XXXXX over the Crockford alphabet", c)
		}
		if strings.ContainsAny(c, "ILOU") {
			t.Errorf("%q contains a character the alphabet excludes because people mistype it", c)
		}
		if seen[c] {
			t.Errorf("%q was generated twice", c)
		}
		seen[c] = true
	}
	for i, h := range hashes {
		if !VerifyPassword(plain[i], h) {
			t.Errorf("code %d does not verify against its own hash", i)
		}
	}
}

// One field on the verify page takes either, so the two shapes must not overlap.
func TestShapes_DoNotOverlap(t *testing.T) {
	for _, tc := range []struct {
		in             string
		totp, recovery bool
	}{
		{"123456", true, false},
		{"12345", false, false},
		{"1234567", false, false},
		{"ABCDE-FGHJK", false, true},
		{"abcde-fghjk", false, true},
		{"ABCDEFGHJK", false, true},
		{"abcde fghjk", false, true},
		{"", false, false},
		{"ABCDE-FGHJI", false, true}, // I normalises to 1
		{"hello world", false, false},
	} {
		if got := isTOTPShape(tc.in); got != tc.totp {
			t.Errorf("isTOTPShape(%q) = %v, want %v", tc.in, got, tc.totp)
		}
		if got := isRecoveryShape(tc.in); got != tc.recovery {
			t.Errorf("isRecoveryShape(%q) = %v, want %v", tc.in, got, tc.recovery)
		}
	}
}

// Crockford's own substitutions. The alphabet excludes I, L, O and U so that a
// code cannot *contain* them; a human reading one off a printout still types
// them, and refusing that is a lockout with a working code in the operator's
// hand.
func TestNormaliseRecoveryCode_AcceptsWhatPeopleActuallyType(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"abcde-fghjk", "ABCDE-FGHJK"},
		{"ABCDE FGHJK", "ABCDE-FGHJK"},
		{"ABCDEFGHJK", "ABCDE-FGHJK"},
		{" abcde-fghjk ", "ABCDE-FGHJK"},
		{"IBCDE-FGHJK", "1BCDE-FGHJK"},
		{"LBCDE-FGHJK", "1BCDE-FGHJK"},
		{"OBCDE-FGHJK", "0BCDE-FGHJK"},
	} {
		if got := normaliseRecoveryCode(tc.in); got != tc.want {
			t.Errorf("normaliseRecoveryCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestConsumeRecoveryCode_LeavesSeven(t *testing.T) {
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}

	remaining, ok := consumeRecoveryCode(plain[3], hashes)
	if !ok {
		t.Fatal("a code from the set was not accepted")
	}
	if len(remaining) != recoveryCodeCount-1 {
		t.Errorf("consuming one left %d hashes, want %d", len(remaining), recoveryCodeCount-1)
	}

	// The consumed one no longer matches; every other one still does.
	if _, ok := consumeRecoveryCode(plain[3], remaining); ok {
		t.Error("a consumed code still opens the door")
	}
	for i, c := range plain {
		if i == 3 {
			continue
		}
		if _, ok := consumeRecoveryCode(c, remaining); !ok {
			t.Errorf("consuming code 3 invalidated code %d", i)
		}
	}
}

func TestConsumeRecoveryCode_RegeneratingInvalidatesAllEight(t *testing.T) {
	old, oldHashes, _ := newRecoveryCodes()
	_, newHashes, _ := newRecoveryCodes()
	if len(oldHashes) != len(newHashes) {
		t.Fatal("the two sets are not the same size")
	}
	for i, c := range old {
		if _, ok := consumeRecoveryCode(c, newHashes); ok {
			t.Errorf("old code %d still opens the door after regeneration", i)
		}
	}
}

func TestConsumeRecoveryCode_RejectsWhatIsNotACode(t *testing.T) {
	_, hashes, _ := newRecoveryCodes()
	for _, bad := range []string{"", "AAAAA-AAAAA", "123456", "not a code"} {
		if _, ok := consumeRecoveryCode(bad, hashes); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}
