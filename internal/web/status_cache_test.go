package web

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// carried-forward: every authenticated render pays a GET_STATUS, and
// Enforcing() can block behind a whole apply cycle for up to 30 s — during
// which render swallows the error and the panic banner disappears exactly when
// the core is busiest. With the chip on every page, that stall would eat the
// countdown too. A ~2 s TTL is what makes the chip affordable.
//
// Drives real authenticated renders through the router rather than calling
// statusForRender directly, so the assertion is the one that matters: an
// operator loading several pages in a row must not each cost a round trip to
// the core.
func TestStatusForRender_IsCachedForAboutTwoSeconds(t *testing.T) {
	srv, calls := newTestServerCountingStatusCalls(t)

	// /ports, not /dashboard: the dashboard handler itself calls GetStatus
	// directly for its own tiles, uncached by design (see statusForRender's
	// doc comment) — that call would inflate this count without telling us
	// anything about the cache. /ports asks the core for status only through
	// render's chip.
	for i := 0; i < 5; i++ {
		getAuthenticated(t, srv, "/ports")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("five renders in a row asked the core %d times; the TTL cache is not "+
			"holding, and every page render pays a socket round trip that can block for 30s", got)
	}

	// Expire the cache directly rather than sleeping the TTL away in a unit test.
	srv.statusAt = time.Now().Add(-3 * time.Second)

	getAuthenticated(t, srv, "/ports")
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("statusForRender asked the core %d times after the TTL expired, want 2; "+
			"a cache that never expires shows a closed window as open", got)
	}
}

// statusForRender used to hold statusMu across the GetStatus round trip. That
// serialised every render behind whichever goroutine was mid-request — exactly
// the "every page stalls behind a core that can block for 30s" failure the
// cache exists to remove, reintroduced at a narrower point: SendCommand dials
// its own connection and never serialised renders before the cache existed.
//
// GET_STATUS answers an error, deliberately: statusForRender never caches a
// failure, so every one of the n concurrent renders is a genuine cache miss
// that has to reach the (slow) core, not just the first. A success response
// would let every render after the first take the fast, lock-free cache-hit
// path once the first writes it — which finishes just as quickly whether the
// slow call happened inside the lock or outside it, and the two mutation
// states would be indistinguishable on the clock.
//
// This needs its own fake core, not newTestServerCountingStatusCalls, because
// GET_STATUS has to actually be slow for "concurrent" and "serialised" to
// read differently on the clock.
func TestStatusForRender_DoesNotSerialiseRendersBehindTheLock(t *testing.T) {
	const (
		n       = 6
		perCall = 80 * time.Millisecond
	)
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, errorRespFor("core busy"))
	fc.OnCommand(shared.CmdGetStatus, func(shared.Command) {
		time.Sleep(perCall)
	})
	srv := newTestServer(t, fc)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			getAuthenticated(t, srv, "/ports")
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Serialised, n renders each paying their own round trip in turn cost
	// roughly n*perCall (here ~480ms); concurrent costs roughly one perCall
	// plus scheduling noise. Halfway between the two tells them apart without
	// being flaky on a loaded machine.
	if budget := time.Duration(n) * perCall / 2; elapsed >= budget {
		t.Fatalf("%d concurrent renders against a %v-slow core took %v, want under %v; "+
			"statusForRender is serialising renders behind the cache lock instead of "+
			"releasing it before the socket round trip", n, perCall, elapsed, budget)
	}
}
