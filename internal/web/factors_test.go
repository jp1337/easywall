package web

import "testing"

// TestTheLastFactorCannotBeRemoved is the guard that makes the mandate real at
// the back door as well as the front.
//
// RequireSecondFactor stops an operator with no factor from using the
// interface. Without this, an operator with one factor switches it off and is
// then an operator with no factor — and the gate they land on is a page they
// can leave by pressing the same button again. Both removal routes go through
// mayRemoveFactor for that reason; a test that covered only handle2FADisable
// would leave the passkey door open.
func TestTheLastFactorCannotBeRemoved(t *testing.T) {
	tests := []struct {
		name    string
		totp    string
		passkey int
		demo    bool
		want    bool
	}{
		{"one TOTP and nothing else", "JBSWY3DPEHPK3PXP", 0, false, false},
		{"one passkey and nothing else", "", 1, false, false},
		{"TOTP and a passkey", "JBSWY3DPEHPK3PXP", 1, false, true},
		{"two passkeys", "", 2, false, true},
		{"nothing at all", "", 0, false, false},
		{"the demo may do as it likes", "JBSWY3DPEHPK3PXP", 0, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFactorTestServer(t, tc.totp, tc.passkey, tc.demo)
			if got := s.mayRemoveFactor(); got != tc.want {
				t.Errorf("mayRemoveFactor() = %v, want %v (totp=%q passkeys=%d demo=%v)",
					got, tc.want, tc.totp, tc.passkey, tc.demo)
			}
		})
	}
}

// TestDisablingTheOnlyFactorIsRefusedOverHTTP proves the predicate is wired to
// the route and not merely present in the package.
func TestDisablingTheOnlyFactorIsRefusedOverHTTP(t *testing.T) {
	s := newFactorTestServer(t, "JBSWY3DPEHPK3PXP", 0, false)

	resp := s.postAuthed(t, "/password/2fa/disable", map[string]string{
		"current_password": testPassword,
	})
	if resp.StatusCode != 303 {
		t.Fatalf("expected a redirect, got %d", resp.StatusCode)
	}
	if got := s.cfg.TOTPSecret(); got == "" {
		t.Fatal("the only second factor was switched off; the mandate has a back door")
	}
}
