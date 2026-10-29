package web

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// TestARevokedCookieCannotWriteLogoutLines: the id != "" guard kept a stranger
// with no cookie out of the audit log, and let anyone holding a cookie that was
// already logged out back in. That cookie still carries its sid, so replaying
// POST /logout with it wrote EvLogout every time — 301 lines from 300 replays,
// and GET_LOG shows the last 200.
func TestARevokedCookieCannotWriteLogoutLines(t *testing.T) {
	fc := newFakeCore(t)
	seen := make(chan shared.Command, 64)
	fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })
	s := newTestServer(t, fc)
	cookie := makeAuthCookie(t, s)

	const replays = 20
	for i := 0; i <= replays; i++ {
		doRequest(s, "POST", "/logout", nil, cookie)
	}

	got := 0
	timeout := time.After(time.Second)
	for {
		select {
		case <-seen:
			got++
			continue
		case <-timeout:
		}
		break
	}
	if got != 1 {
		t.Errorf("one logout and %d replays of its cookie wrote %d audit lines, want 1", replays, got)
	}
}
