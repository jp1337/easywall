package web

import (
	"fmt"
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

// testAddr returns the i-th of a large supply of distinct addresses, for
// tests that need to fill the table to burstMaxAddrs.
func testAddr(i int) string {
	return fmt.Sprintf("10.%d.%d.%d", i/65536%256, i/256%256, i%256)
}

func TestEvictionMakesRoomForANewAddressPastTheCeiling(t *testing.T) {
	b := newLoginBurst()
	base := time.Unix(0, 0).UTC()

	for i := 0; i < burstMaxAddrs; i++ {
		b.record(shared.EvLoginFailed, testAddr(i), base)
	}
	if got := len(b.buckets); got != burstMaxAddrs {
		t.Fatalf("table holds %d buckets, want %d", got, burstMaxAddrs)
	}

	// Every existing bucket's window has since closed and none of them ever
	// fired, so every one of them is dead: a genuinely new address must
	// still get a bucket, not be refused forever because the table has once
	// seen burstMaxAddrs strangers.
	later := base.Add(burstWindow + time.Second)
	b.record(shared.EvLoginFailed, "198.51.100.77", later)
	if _, ok := b.buckets["198.51.100.77"]; !ok {
		t.Error("a new address was refused even though every existing bucket had gone dead")
	}
}

func TestEvictionDoesNotDiscardALiveBucket(t *testing.T) {
	b := newLoginBurst()
	base := time.Unix(0, 0).UTC()

	// Fill the table with addresses that will be dead by the time it matters
	// below: each gets one failure and never reaches the threshold.
	for i := 0; i < burstMaxAddrs-1; i++ {
		b.record(shared.EvLoginFailed, testAddr(i), base)
	}

	// A late arrival, one short of the threshold, well after the others'
	// windows opened but with its own window still fully ahead of it.
	const addr = "203.0.113.9"
	afterOthersWindow := base.Add(burstWindow + time.Second)
	for i := 0; i < burstThreshold-1; i++ {
		if n := b.record(shared.EvLoginFailed, addr, afterOthersWindow); n != nil {
			t.Fatalf("fired early, on failure %d", i+1)
		}
	}
	if got := len(b.buckets); got != burstMaxAddrs {
		t.Fatalf("table holds %d buckets, want %d", got, burstMaxAddrs)
	}

	// Still inside addr's own window, but well past every other address's:
	// a new address arriving now is what forces eviction, and it must not
	// be able to touch addr's still-live bucket to make room for itself.
	stillWithinAddrsWindow := afterOthersWindow.Add(burstWindow - time.Second)
	b.record(shared.EvLoginFailed, "198.51.100.200", stillWithinAddrsWindow)

	if n := b.record(shared.EvLoginFailed, addr, stillWithinAddrsWindow); n == nil {
		t.Error("a live bucket was discarded by eviction before it could fire")
	}
}
