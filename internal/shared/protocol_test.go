package shared

import (
	"encoding/json"
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

// UPDATE_FEED takes the nft mutex to replace a live set and can queue behind an
// apply for NftTimeout; GET_FEEDS renders a page and must not hang it.
func TestFeedCommandsHaveTheirDeadlineClasses(t *testing.T) {
	if got, want := CommandTimeout(CmdUpdateFeed), NftTimeout+defaultCommandTimeout; got != want {
		t.Errorf("CommandTimeout(CmdUpdateFeed) = %s, want %s", got, want)
	}
	if got, want := CommandTimeout(CmdGetFeeds), defaultCommandTimeout; got != want {
		t.Errorf("CommandTimeout(CmdGetFeeds) = %s, want %s", got, want)
	}
}

// Why the limit is 8 MiB (plan P12): one UPDATE_FEED of FeedMaxEntries
// entries fits at the longest spelling any entry can have. Measured: 100 000
// of the longest IPv6 addresses are 4 200 059 bytes bare and 4 600 059 as /128
// prefixes — both over 4 MiB, which is what the limit was first set to — and
// the /128 case leaves 3 788 549 bytes of the 8 388 608. The longest IPv4
// spelling, a /31, is 2 100 059.
func TestMaxMessageBytesCarriesAFullFeed(t *testing.T) {
	for _, tc := range []struct {
		entry string
		want  int
	}{
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", 4200059},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", 4600059},
		{"255.255.255.254/31", 2100059},
	} {
		entries := make([]string, FeedMaxEntries)
		for i := range entries {
			entries[i] = tc.entry
		}
		payload, err := json.Marshal(UpdateFeedPayload{ID: OwnFeedID(MaxOwnFeeds), Entries: entries})
		if err != nil {
			t.Fatal(err)
		}
		cmd, _ := json.Marshal(Command{Type: CmdUpdateFeed, Payload: payload})
		if len(cmd) != tc.want {
			t.Errorf("%d entries like %s are %d bytes; measured %d — re-measure the margin", FeedMaxEntries, tc.entry, len(cmd), tc.want)
		}
		if len(cmd) > MaxMessageBytes {
			t.Errorf("%d entries like %s are %d bytes; MaxMessageBytes is %d", FeedMaxEntries, tc.entry, len(cmd), MaxMessageBytes)
		}
	}
}
