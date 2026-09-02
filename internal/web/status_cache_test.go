package web

import (
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
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Acceptance: shared.AcceptanceIdle,
	}))
	var calls int32
	fc.OnCommand(shared.CmdGetStatus, func(shared.Command) {
		atomic.AddInt32(&calls, 1)
	})
	srv := newTestServer(t, fc)

	// /ports, not /dashboard: the dashboard handler itself calls GetStatus
	// directly for its own tiles, uncached by design (see statusForRender's
	// doc comment) — that call would inflate this count without telling us
	// anything about the cache. /ports asks the core for status only through
	// render's chip.
	for i := 0; i < 5; i++ {
		getAuthenticated(t, srv, "/ports")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("five renders in a row asked the core %d times; the TTL cache is not "+
			"holding, and every page render pays a socket round trip that can block for 30s", got)
	}

	// Expire the cache directly rather than sleeping the TTL away in a unit test.
	srv.statusAt = time.Now().Add(-3 * time.Second)

	getAuthenticated(t, srv, "/ports")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("statusForRender asked the core %d times after the TTL expired, want 2; "+
			"a cache that never expires shows a closed window as open", got)
	}
}
