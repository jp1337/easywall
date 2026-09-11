package shared

import (
	"testing"
	"time"
)

// TestGetHealthKeepsTheShortDeadlineForItsConsumer holds the 5 s deadline.
//
// It was called TestCommandTimeoutKeepsGetHealthShort and it argued from a
// false premise: "two netlink reads and one file read — nothing that queues
// behind the nft mutex the way IMPORT_RULES and VALIDATE_CUSTOM do". Both
// reads do queue behind it. NftablesManager.Enforcing (nftables.go:347) and
// RuleCounters (:416) each take m.mu, and Apply holds it across an nft
// subprocess for up to NftTimeout.
//
// The deadline is still right, for a different reason: Dockerfile's
// HEALTHCHECK is --timeout=5s, so the only documented consumer gives up at
// five seconds regardless, and a longer deadline would buy nothing there while
// making every other caller wait. What the old comment got right is the drift
// it catches — a five-second poll that silently became thirty-five would be
// invisible to every caller.
//
// What this test does NOT assert is that the call cannot block. It can. See
// CmdGetHealth's own comment for the honest scope and what closing it needs.
func TestGetHealthKeepsTheShortDeadlineForItsConsumer(t *testing.T) {
	if got, want := CommandTimeout(CmdGetHealth), 5*time.Second; got != want {
		t.Errorf("CommandTimeout(CmdGetHealth) = %s, want %s", got, want)
	}
}
