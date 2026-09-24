package web

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The two feed methods send the payload interfaces.md names and read the reply
// the core sends — through the real socket transport, not the demo.
func TestCoreClient_UpdateFeedAndGetFeeds(t *testing.T) {
	fc := newFakeCore(t)
	c := NewCoreClient(fc.socketPath)

	fc.SetResponse(shared.CmdUpdateFeed, successResp(shared.UpdateFeedResult{Before: 20, After: 19, Dropped: 1, Changed: true, Loaded: true}))
	res, err := c.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: []string{"192.0.2.0/24"}})
	if err != nil || res != (shared.UpdateFeedResult{Before: 20, After: 19, Dropped: 1, Changed: true, Loaded: true}) {
		t.Fatalf("UpdateFeed = %+v, %v", res, err)
	}
	var sent shared.UpdateFeedPayload
	if cmd := fc.LastCommand(); cmd.Type != shared.CmdUpdateFeed || json.Unmarshal(cmd.Payload, &sent) != nil ||
		sent.ID != "dshield" || len(sent.Entries) != 1 {
		t.Errorf("the core received %s %s", cmd.Type, cmd.Payload)
	}

	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{{ID: "dshield", ContainsAddr: true}}}))
	feeds, err := c.GetFeeds("192.0.2.7")
	if err != nil || len(feeds.Feeds) != 1 || !feeds.Feeds[0].ContainsAddr {
		t.Fatalf("GetFeeds = %+v, %v", feeds, err)
	}
	var asked shared.GetFeedsPayload
	if cmd := fc.LastCommand(); json.Unmarshal(cmd.Payload, &asked) != nil || asked.Addr != "192.0.2.7" {
		t.Errorf("GET_FEEDS carried %s", cmd.Payload)
	}

	// A refusal is the core's reason, not a parse error.
	fc.SetResponse(shared.CmdUpdateFeed, shared.Response{Error: "feed dshield would shrink from 20 to 2 entries"})
	if _, err := c.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield"}); err == nil || !strings.Contains(err.Error(), "shrink") {
		t.Errorf("a refused update returned %v", err)
	}
}

// Plan P12: a payload over the socket limit is not sent, and the caller can
// tell that from every other failure.
func TestCoreClient_UpdateFeedTooLargeIsRecognisable(t *testing.T) {
	fc := newFakeCore(t)
	c := NewCoreClient(fc.socketPath)
	entries := make([]string, shared.MaxMessageBytes/len(`"2001:db8::1",`)+1)
	for i := range entries {
		entries[i] = "2001:db8::1"
	}
	_, err := c.UpdateFeed(shared.UpdateFeedPayload{ID: "own-1", Entries: entries})
	if !errors.Is(err, shared.ErrRequestTooLarge) {
		t.Fatalf("got %v, want ErrRequestTooLarge", err)
	}
	if fc.LastCommand() != nil {
		t.Error("the oversized command reached the core")
	}
}
