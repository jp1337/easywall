package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// shortenNotifyClocks makes the poll loop and the status cache fast enough to
// test. Both are vars for exactly this; see their comments.
func shortenNotifyClocks(t *testing.T) {
	t.Helper()
	oldTick, oldTTL := notifyTick, statusTTL
	notifyTick, statusTTL = 10*time.Millisecond, time.Millisecond
	t.Cleanup(func() { notifyTick, statusTTL = oldTick, oldTTL })
}

// startNotifier runs the loop for the duration of the test and waits for it to
// exit before the test returns.
//
// The wait is the point: runNotifier reads notifyTick and statusTTL, which
// shortenNotifyClocks writes back at cleanup, and closing the stop channel only
// asks the goroutine to leave. Registered after shortenNotifyClocks so this
// cleanup runs first — without it -race reports a write-after-read on both
// globals, which is the test leaking a goroutine rather than a defect in the
// code under test.
func startNotifier(t *testing.T, s *Server) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runNotifier(stop)
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})
}

func TestTheNotifierPollsStatusAndPostsARollback(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	defer srv.Close()

	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		LastApply: "t1", Acceptance: shared.AcceptanceIdle}))
	s := newTestServer(t, fc)
	if err := s.cfg.SaveNotifications("webhook", srv.URL, true, true, true, true); err != nil {
		t.Fatal(err)
	}
	s.rebuildNotifier()
	shortenNotifyClocks(t)
	startNotifier(t, s)

	// The first poll seeds and must announce nothing.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if len(bodies) != 0 {
		mu.Unlock()
		t.Fatal("the first observation announced something; a restart must not page anyone")
	}
	mu.Unlock()

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		LastApply: "t2", Acceptance: shared.AcceptanceRolledBack, AcceptanceReason: "timeout"}))
	waitUntil(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return len(bodies) == 1 })

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(bodies[0], "rolled_back") || !strings.Contains(bodies[0], "timeout") {
		t.Errorf("body = %s", bodies[0])
	}
}

func TestASwitchedOffTriggerSendsNothing(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		LastApply: "t1", Acceptance: shared.AcceptanceIdle}))
	s := newTestServer(t, fc)
	// Configured and reachable, but this trigger is off.
	if err := s.cfg.SaveNotifications("webhook", srv.URL, false, false, false, false); err != nil {
		t.Fatal(err)
	}
	s.rebuildNotifier()
	shortenNotifyClocks(t)
	startNotifier(t, s)

	time.Sleep(50 * time.Millisecond)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		LastApply: "t2", Acceptance: shared.AcceptanceRolledBack}))
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("a switched-off trigger posted %d times", n)
	}
}

// Demo mode is the one configuration that ignores the configuration: the public
// demo is a page anyone on the internet can open, and it makes no outbound
// request whatever the settings say.
func TestDemoModeBuildsNoNotifier(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if err := s.cfg.SaveNotifications("webhook", "https://example.invalid/hook", true, true, true, true); err != nil {
		t.Fatal(err)
	}
	s.cfg.DemoMode = true
	s.rebuildNotifier()
	if s.currentNotifier() != nil {
		t.Fatal("demo mode built a notifier; the demo makes no outbound request")
	}
}

// The burst counter must see the address the request came from even when the
// audit record keeps none — the demo still has to tell one attacker from a
// thousand visitors. Ordering inside Record, not a second address field.
func TestTheBurstHookSeesTheAddressTheDemoDoesNotRecord(t *testing.T) {
	a := newAuditEvents(NewDemoClient(), true)
	var seen string
	a.onBurst = func(_ shared.LoginEvent, addr string) { seen = addr }

	a.Record(shared.EvLoginFailed, "203.0.113.7", 0, false)

	if seen != "203.0.113.7" {
		t.Errorf("the hook saw %q; it must see the real address", seen)
	}
	if p := <-a.ch; p.Addr != "" {
		t.Errorf("the demo queued the address %q; it must record none", p.Addr)
	}
}

// Five failures from one address, through the tap NewServer installs, arrive at
// the webhook as one failed_logins notification.
func TestFailedLoginsCrossingTheThresholdPost(t *testing.T) {
	var hits int32
	var mu sync.Mutex
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if err := s.cfg.SaveNotifications("webhook", srv.URL, false, false, false, true); err != nil {
		t.Fatal(err)
	}
	s.rebuildNotifier()
	s.notifyBurst = newLoginBurst()
	s.events.onBurst = s.notifyLoginFailure

	for i := 0; i < burstThreshold; i++ {
		s.events.Record(shared.EvLoginFailed, "203.0.113.9", 0, false)
	}

	waitUntil(t, 2*time.Second, func() bool { return atomic.LoadInt32(&hits) == 1 })
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(body, "failed_logins") || !strings.Contains(body, "203.0.113.9") {
		t.Errorf("body = %s", body)
	}
}

// A settings save rewrites the six switches and replaces the live notifier while
// the poll loop is reading both. Without notifyMu that is a data race on a
// pointer, and without Config's lock around the switches it is one on the
// fields; -race says so for either.
func TestRebuildingTheNotifierUnderLoadIsRaceFree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if err := s.cfg.SaveNotifications("webhook", srv.URL, true, true, true, true); err != nil {
		t.Fatal(err)
	}

	// The writer sets the pace; the reader runs until it is finished, so the two
	// are genuinely overlapping rather than one finishing before the other starts.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < 50; i++ {
			if err := s.cfg.SaveNotifications("webhook", srv.URL, i%2 == 0, true, true, true); err != nil {
				t.Error(err)
				return
			}
			s.rebuildNotifier()
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			// Both halves of what a second request would touch: the destination
			// rebuildNotifier reads, and the switch dispatch reads.
			s.rebuildNotifier()
			s.dispatchNotification(Notification{Event: "rolled_back", Time: time.Now()})
		}
	}()
	wg.Wait()
}
