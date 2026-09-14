package web

import (
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestFiveFailuresFromOneAddressNotifyOnceThenGoQuiet(t *testing.T) {
	b := newLoginBurst()
	base := time.Unix(0, 0).UTC()
	var fired int
	for i := 0; i < 12; i++ {
		if n := b.record(shared.EvLoginFailed, "203.0.113.5", base.Add(time.Duration(i)*time.Second)); n != nil {
			fired++
			if !strings.Contains(n.Detail, "203.0.113.5") {
				t.Errorf("the address is not in the detail: %q", n.Detail)
			}
		}
	}
	if fired != 1 {
		t.Fatalf("twelve failures fired %d notifications, want exactly 1", fired)
	}
	// Still inside the quiet period.
	if n := b.record(shared.EvLoginFailed, "203.0.113.5", base.Add(10*time.Minute)); n != nil {
		t.Error("the quiet period did not hold")
	}
	// Past it: a fresh burst of the same size notifies again.
	for i := 0; i < burstThreshold-1; i++ {
		if n := b.record(shared.EvLoginFailed, "203.0.113.5", base.Add(20*time.Minute)); n != nil {
			t.Fatalf("fired early, on failure %d of the new burst", i+1)
		}
	}
	if n := b.record(shared.EvLoginFailed, "203.0.113.5", base.Add(20*time.Minute)); n == nil {
		t.Error("after the quiet period the same number of failures must notify again")
	}
}

func TestTwoAddressesAreCountedApart(t *testing.T) {
	b := newLoginBurst()
	base := time.Unix(0, 0).UTC()
	for i := 0; i < 4; i++ {
		b.record(shared.EvLoginFailed, "203.0.113.5", base)
		b.record(shared.EvLoginFailed, "198.51.100.9", base)
	}
	// Four each: neither has crossed five, so nothing has fired.
	if n := b.record(shared.EvLoginFailed, "203.0.113.5", base); n == nil {
		t.Error("the fifth failure from one address must fire")
	}
}

func TestASuccessfulLoginIsNotCounted(t *testing.T) {
	b := newLoginBurst()
	base := time.Unix(0, 0).UTC()
	for i := 0; i < 20; i++ {
		if n := b.record(shared.EvLoginOK, "203.0.113.5", base); n != nil {
			t.Fatal("a successful login produced a failed-login notification")
		}
	}
}
