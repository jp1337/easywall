package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// ── scheduling ────────────────────────────────────────────────────────────

// pinOffset fixes the install's offset: no randomness in a test.
func pinOffset(fs *feedStore, d time.Duration) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.loadLocked().OffsetSeconds = int64(d / time.Second)
}

// Every pull after the first is one whole interval after the last attempt,
// never earlier and never later — hourly, 12 h, daily and own feeds alike. An
// offset-aligned schedule pulled an hourly list anywhere up to two hours
// apart, and a daily own feed at 23 h 59 min, which CrowdSec answers with 429.
func TestTheNextPullIsOneIntervalAfterTheLast(t *testing.T) {
	r := testRunner(t)
	pinOffset(r.store, 17*time.Minute+20*time.Second)
	last := time.Date(2026, 9, 24, 13, 58, 40, 0, time.UTC) // well away from the offset
	for id, interval := range map[string]time.Duration{
		"tor-exits":     time.Hour,
		"spamhaus-drop": 12 * time.Hour,
		"ipsum-3":       24 * time.Hour,
		"own-1":         shared.OwnFeedInterval,
	} {
		r.store.setFetchState(id, feedFetchState{LastAttempt: last})
		if got := nextFeedAttempt(r.store.fetchState(id), interval); !got.Equal(last.Add(interval)) {
			t.Errorf("%s: next pull %v, want exactly %v", id, got, last.Add(interval))
		}
		if r.due(id, interval, last.Add(interval-time.Second)) {
			t.Errorf("%s: due a second early", id)
		}
		if !r.due(id, interval, last.Add(interval)) {
			t.Errorf("%s: not due after one interval", id)
		}
	}
}

// The offset places only the first pull of a feed with no attempt on record:
// at this install's slot after a pass first finds it, then it holds its place.
func TestTheFirstPullWaitsForTheInstallSlot(t *testing.T) {
	r := testRunner(t)
	pinOffset(r.store, 17*time.Minute)
	found := time.Date(2026, 9, 24, 13, 20, 0, 0, time.UTC)
	slot := time.Date(2026, 9, 24, 14, 17, 0, 0, time.UTC)
	if got := firstFeedSlot(found, time.Hour, 17*time.Minute); !got.Equal(slot) {
		t.Fatalf("first slot %v, want %v", got, slot)
	}
	if got := firstFeedSlot(slot, time.Hour, 17*time.Minute); !got.Equal(slot) {
		t.Errorf("found on its slot: %v, want the same slot", got)
	}
	if got := firstFeedSlot(found, 12*time.Hour, 13*time.Hour+17*time.Minute); !got.Equal(time.Date(2026, 9, 25, 1, 17, 0, 0, time.UTC)) {
		t.Errorf("12 h first slot %v", got)
	}
	if r.due("dshield", time.Hour, found) {
		t.Error("due when first found, before its slot")
	}
	// Later passes measure from when it was first found, not from now —
	// otherwise the slot would keep moving ahead of the clock.
	if r.due("dshield", time.Hour, slot.Add(-time.Second)) {
		t.Error("due a second before its slot")
	}
	if !r.due("dshield", time.Hour, slot.Add(40*time.Second)) {
		t.Error("not due at its slot")
	}
}

func TestFeedRetryWaitsAnHourDoublingToTheInterval(t *testing.T) {
	for _, c := range []struct {
		failures int
		reason   string
		interval time.Duration
		want     time.Duration
	}{
		{1, feedErrTimeout, 24 * time.Hour, time.Hour},
		{2, feedErrTimeout, 24 * time.Hour, 2 * time.Hour},
		{4, feedErrTimeout, 24 * time.Hour, 8 * time.Hour},
		{9, feedErrTimeout, 24 * time.Hour, 24 * time.Hour},
		{3, feedErrHTTPStatus, time.Hour, time.Hour},
		{1, feedErrRateLimited, 12 * time.Hour, 12 * time.Hour},
	} {
		st := feedFetchState{Failures: c.failures, LastError: c.reason}
		if got := feedRetryDelay(st, c.interval); got != c.want {
			t.Errorf("%d × %s, interval %v: wait %v, want %v", c.failures, c.reason, c.interval, got, c.want)
		}
	}
	last := time.Now()
	st := feedFetchState{LastAttempt: last, Failures: 2, LastError: feedErrTimeout}
	if got := nextFeedAttempt(st, 24*time.Hour); !got.Equal(last.Add(2 * time.Hour)) {
		t.Errorf("after 2 failures: %v, want last + 2 h", got)
	}
}

// ── one refresh, end to end: httptest list, fake core ─────────────────────

// feedHarness is a list server, a fake core that answers GET_FEEDS and
// UPDATE_FEED, and a runner between them.
type feedHarness struct {
	t       *testing.T
	fc      *fakeCore
	r       *feedRunner
	hits    atomic.Int32
	mu      sync.Mutex
	updates []shared.UpdateFeedPayload
	header  http.Header
}

func newFeedHarness(t *testing.T, stored bool) *feedHarness {
	h := &feedHarness{t: t, fc: newFakeCore(t)}
	h.r = newFeedRunner(NewCoreClient(h.fc.socketPath), newFeedStore(t.TempDir()+"/feed_fetch.json"))
	h.setStored(stored)
	h.fc.SetResponse(shared.CmdUpdateFeed, successResp(shared.UpdateFeedResult{After: 1, Changed: true}))
	h.fc.OnCommand(shared.CmdUpdateFeed, func(c shared.Command) {
		var p shared.UpdateFeedPayload
		_ = json.Unmarshal(c.Payload, &p)
		h.mu.Lock()
		h.updates = append(h.updates, p)
		h.mu.Unlock()
	})
	return h
}

func (h *feedHarness) setStored(stored bool) {
	h.fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "own-1", Stored: stored}, {ID: "spamhaus-drop", Stored: stored},
	}}))
}

// list serves body with an ETag, or 304 to a matching If-None-Match.
func (h *feedHarness) list(body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		h.mu.Lock()
		h.header = r.Header.Clone()
		h.mu.Unlock()
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Thu, 24 Sep 2026 13:15:02 GMT")
		_, _ = w.Write([]byte(body))
	}))
	h.t.Cleanup(srv.Close)
	return srv.URL
}

func (h *feedHarness) lastHeader(name string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.header.Get(name)
}

func (h *feedHarness) sent() []shared.UpdateFeedPayload {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.updates)
}

func plainSource(u string) feedSource {
	return feedSource{urls: []string{u}, format: shared.FeedFormatPlain, interval: time.Hour}
}

func TestARefreshSendsCanonicalEntriesAndKeepsTheValidators(t *testing.T) {
	h := newFeedHarness(t, false)
	h.r.refreshFrom("own-1", plainSource(h.list("192.0.2.1\n192.0.2.1/32\n2001:db8::/128\n# c\nbad\n")), false)

	sent := h.sent()
	if len(sent) != 1 || sent[0].ID != "own-1" || sent[0].NotModified ||
		!slices.Equal(sent[0].Entries, []string{"192.0.2.1", "2001:db8::"}) {
		t.Fatalf("UPDATE_FEED %+v", sent)
	}
	st := h.r.store.fetchState("own-1")
	if st.ETag != `"v1"` || st.LastModified == "" || st.Status != FeedUpdated || st.Failures != 0 ||
		st.Rejected != 1 || !slices.Equal(st.RejectedSample, []string{"bad"}) || st.LastAttempt.IsZero() {
		t.Errorf("state %+v", st)
	}
}

// The core took the list and it had not changed: "unchanged", not "updated".
func TestACopyTheCoreFoundTheSameIsUnchanged(t *testing.T) {
	h := newFeedHarness(t, true)
	h.fc.SetResponse(shared.CmdUpdateFeed, successResp(shared.UpdateFeedResult{Before: 1, After: 1}))
	h.r.store.setFetchState("own-1", feedFetchState{ETag: `"older"`})
	h.r.refreshFrom("own-1", plainSource(h.list("192.0.2.1\n")), false)
	if st := h.r.store.fetchState("own-1"); st.Status != FeedUnchanged || st.ETag != `"v1"` {
		t.Errorf("state %+v, want unchanged with the new validator", st)
	}
}

// The attempt is on disk before the request goes out, so a process that dies
// mid-refresh — a parser panic on a hostile body, say — does not restart and
// fetch again at once, in a loop, against somebody else's server.
func TestTheAttemptIsRecordedBeforeTheRequest(t *testing.T) {
	h := newFeedHarness(t, false)
	path := h.r.store.path
	var onDisk time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onDisk = newFeedStore(path).fetchState("own-1").LastAttempt
		_, _ = w.Write([]byte("192.0.2.1\n"))
	}))
	t.Cleanup(srv.Close)
	h.r.refreshFrom("own-1", plainSource(srv.URL), false)
	if onDisk.IsZero() {
		t.Error("the request went out before the attempt was written")
	}
}

// P8: a 304 moves checked_at in the core and nothing else.
func TestANotModifiedListIsSentAsNotModified(t *testing.T) {
	h := newFeedHarness(t, true)
	u := h.list("192.0.2.1\n")
	h.r.store.setFetchState("own-1", feedFetchState{ETag: `"v1"`, LastModified: "x", Rejected: 3})
	h.r.refreshFrom("own-1", plainSource(u), false)

	sent := h.sent()
	if len(sent) != 1 || !sent[0].NotModified || len(sent[0].Entries) != 0 {
		t.Fatalf("UPDATE_FEED %+v, want one not_modified with no entries", sent)
	}
	if st := h.r.store.fetchState("own-1"); st.Status != FeedUnchanged || st.ETag != `"v1"` || st.Rejected != 3 {
		t.Errorf("state %+v", st)
	}
}

// A feed switched off, pruned by the core and switched on again still has
// its validators here. Sending them would be answered 304 for ever, pointing
// at a copy that no longer exists.
func TestNoValidatorsGoOutWhenTheCoreHasNoCopy(t *testing.T) {
	h := newFeedHarness(t, false)
	u := h.list("192.0.2.1\n")
	h.r.store.setFetchState("own-1", feedFetchState{ETag: `"v1"`, LastModified: "x"})
	h.r.refreshFrom("own-1", plainSource(u), false)

	if inm := h.lastHeader("If-None-Match"); inm != "" {
		t.Errorf("If-None-Match %q sent with no copy in the core", inm)
	}
	if sent := h.sent(); len(sent) != 1 || sent[0].NotModified || len(sent[0].Entries) != 1 {
		t.Errorf("UPDATE_FEED %+v, want the full list", sent)
	}
}

func TestAFailedRefreshKeepsTheOldValidatorsAndCounts(t *testing.T) {
	for _, stored := range []bool{true, false} {
		h := newFeedHarness(t, stored)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)
		h.r.store.setFetchState("own-1", feedFetchState{ETag: `"old"`, Failures: 2})
		h.r.refreshFrom("own-1", plainSource(srv.URL), false)

		st := h.r.store.fetchState("own-1")
		want := FeedFailedNoCopy
		if stored {
			want = FeedFailedCopy
		}
		if st.ETag != `"old"` || st.Failures != 3 || st.LastError != feedErrHTTPStatus || st.HTTPStatus != 503 || st.Status != want {
			t.Errorf("stored=%v: state %+v", stored, st)
		}
		if len(h.sent()) != 0 {
			t.Errorf("stored=%v: a failed fetch reached the core", stored)
		}
	}
}

func TestAListWithNoEntriesIsAFailureAndNeverReachesTheCore(t *testing.T) {
	h := newFeedHarness(t, true)
	h.r.refreshFrom("own-1", plainSource(h.list("Umm... You can only fetch the data every 30 minutes - sorry.\n")), false)
	st := h.r.store.fetchState("own-1")
	if st.LastError != feedErrEmpty || st.Status != FeedFailedCopy || st.Rejected != 1 || len(st.RejectedSample) != 1 || st.ETag != "" {
		t.Errorf("state %+v", st)
	}
	if len(h.sent()) != 0 {
		t.Error("an empty list was sent: the core would have been asked to load nothing")
	}
}

func TestAListOverTheEntryLimitIsTooLarge(t *testing.T) {
	h := newFeedHarness(t, false)
	var b strings.Builder
	for i := 0; i <= shared.FeedMaxEntries; i++ {
		fmt.Fprintf(&b, "10.%d.%d.%d\n", i>>16&255, i>>8&255, i&255)
	}
	h.r.refreshFrom("own-1", plainSource(h.list(b.String())), false)
	if st := h.r.store.fetchState("own-1"); st.LastError != feedErrTooLarge {
		t.Errorf("state %+v, want too_large", st)
	}
	if len(h.sent()) != 0 {
		t.Error("sent anyway")
	}
}

// P7, and the way it is kept: the parsed update waits in memory and is
// offered again — the list is not downloaded twice in five minutes.
func TestAnUpdateRefusedDuringAnApplyIsRetriedWithoutFetchingAgain(t *testing.T) {
	h := newFeedHarness(t, false)
	h.fc.SetResponse(shared.CmdUpdateFeed, errorRespFor(shared.ErrApplyInProgressText))
	u := h.list("192.0.2.1\n")
	now := time.Now()
	h.r.now = func() time.Time { return now }
	h.r.refreshFrom("own-1", plainSource(u), false)

	if st := h.r.store.fetchState("own-1"); st.ETag != "" || st.Failures != 0 || st.LastError != "" {
		t.Fatalf("after the refusal: %+v — the validators wait for the core, and it is not a failure", st)
	}
	if h.r.due("own-1", time.Hour, now.Add(feedApplyRetry-time.Second)) {
		t.Fatal("due before the retry")
	}
	if !h.r.due("own-1", time.Hour, now.Add(feedApplyRetry)) {
		t.Fatal("not due at the retry")
	}

	h.fc.SetResponse(shared.CmdUpdateFeed, successResp(shared.UpdateFeedResult{Changed: true}))
	h.r.refresh("own-1", false)
	if n := h.hits.Load(); n != 1 {
		t.Errorf("the list was fetched %d times, want once", n)
	}
	sent := h.sent()
	if len(sent) != 2 || !slices.Equal(sent[1].Entries, []string{"192.0.2.1"}) {
		t.Fatalf("UPDATE_FEED %+v, want the same entries offered twice", sent)
	}
	if st := h.r.store.fetchState("own-1"); st.ETag != `"v1"` || st.Status != FeedUpdated {
		t.Errorf("after the retry: %+v", st)
	}
}

func TestTheCoresAnswerNamesTheFailure(t *testing.T) {
	h := newFeedHarness(t, true)
	h.fc.SetResponse(shared.CmdUpdateFeed, errorRespFor("feed shrinks from 1000 to 10 entries"))
	u := h.list("192.0.2.1\n")
	h.r.refreshFrom("own-1", plainSource(u), false)
	if st := h.r.store.fetchState("own-1"); st.LastError != feedErrRefused || st.ETag != "" {
		t.Errorf("refused: %+v — want refused, and no validator for a version the core did not take", st)
	}

	// No core at all.
	r := newFeedRunner(NewCoreClient(t.TempDir()+"/none.sock"), newFeedStore(t.TempDir()+"/f.json"))
	r.refreshFrom("own-1", plainSource(u), false)
	if st := r.store.fetchState("own-1"); st.LastError != feedErrCoreUnreachable || st.Status != FeedFailedNoCopy {
		t.Errorf("no core: %+v", st)
	}
}

// Spamhaus publishes v4 and v6 as two files. Both or neither: v6 is 91 of
// ~1800 entries, so a v4-only update would pass the shrink guard and quietly
// stop blocking every v6 network.
func TestATwoFileFeedUpdatesOnlyWhenBothArrive(t *testing.T) {
	h := newFeedHarness(t, true)
	v4 := h.list(`{"cidr":"192.0.2.0/24"}` + "\n" + `{"type":"metadata","records":1}` + "\n")
	v6 := h.list(`{"cidr":"2001:db8::/32"}` + "\n")
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(down.Close)
	src := func(urls ...string) feedSource {
		return feedSource{urls: urls, format: shared.FeedFormatSpamhaus, interval: 12 * time.Hour}
	}
	h.r.store.setFetchState("spamhaus-drop", feedFetchState{ETag: `"v1"`})

	h.r.refreshFrom("spamhaus-drop", src(v4, down.URL), false)
	if len(h.sent()) != 0 {
		t.Fatal("one file of two reached the core")
	}
	if st := h.r.store.fetchState("spamhaus-drop"); st.LastError != feedErrHTTPStatus {
		t.Fatalf("state %+v", st)
	}

	h.r.refreshFrom("spamhaus-drop", src(v4, v6), false)
	sent := h.sent()
	if len(sent) != 1 || !slices.Equal(sent[0].Entries, []string{"192.0.2.0/24", "2001:db8::/32"}) {
		t.Fatalf("UPDATE_FEED %+v, want both files merged", sent)
	}
	if inm := h.lastHeader("If-None-Match"); inm != "" {
		t.Errorf("If-None-Match %q on a two-file feed: a 304 for one file says nothing of the other", inm)
	}
	if st := h.r.store.fetchState("spamhaus-drop"); st.ETag != "" || st.Failures != 0 {
		t.Errorf("state %+v", st)
	}
}

// Ruling X1: the core refuses a list that shrank below 70 % of the live
// copy. The row needs both counts to say so, and how to take the smaller
// list; the next update the core accepts clears them.
func TestAShrunkListRecordsBothCounts(t *testing.T) {
	h := newFeedHarness(t, true)
	h.fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "own-1", Stored: true, Entries: 1000},
	}}))
	h.fc.SetResponse(shared.CmdUpdateFeed, errorRespFor(shared.ErrFeedShrankText))
	u := h.list("192.0.2.1\n192.0.2.2\n")
	h.r.refreshFrom("own-1", plainSource(u), false)

	st := h.r.store.fetchState("own-1")
	if st.LastError != feedErrShrank || st.Status != FeedFailedCopy || st.ShrankFrom != 1000 || st.ShrankTo != 2 || st.ETag != "" {
		t.Fatalf("after the refusal: %+v — want shrank, 1000 → 2, failed with the copy kept, no validator", st)
	}

	h.fc.SetResponse(shared.CmdUpdateFeed, successResp(shared.UpdateFeedResult{Changed: true}))
	h.r.refreshFrom("own-1", plainSource(u), false)
	if st := h.r.store.fetchState("own-1"); st.ShrankFrom != 0 || st.ShrankTo != 0 || st.LastError != "" {
		t.Errorf("after an accepted update: %+v — want the counts cleared", st)
	}
}

// A switch-on inside a running backoff sends nothing: three toggles after a
// 429 are not three more requests to the host.
func TestSwitchingOnRespectsARunningBackoff(t *testing.T) {
	h := newFeedHarness(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	now := time.Now()
	h.r.now = func() time.Time { return now }
	for range 3 {
		h.r.refreshFrom("own-1", plainSource(srv.URL), true)
	}
	if n := h.hits.Load(); n != 1 {
		t.Fatalf("%d requests from three switch-ons after a failure, want 1", n)
	}
	now = now.Add(time.Hour) // the backoff after one failure has run
	h.r.refreshFrom("own-1", plainSource(srv.URL), true)
	if n := h.hits.Load(); n != 2 {
		t.Errorf("%d requests once the backoff ran out, want 2", n)
	}
}

func TestSwitchingOnFetchesOnlyWhenTheCoreHasNoCopy(t *testing.T) {
	h := newFeedHarness(t, true)
	u := h.list("192.0.2.1\n")
	h.r.refreshFrom("own-1", plainSource(u), true)
	if h.hits.Load() != 0 {
		t.Fatal("fetched although the core holds a copy")
	}
	h.setStored(false)
	h.r.refreshFrom("own-1", plainSource(u), true)
	if h.hits.Load() != 1 || len(h.sent()) != 1 {
		t.Fatalf("hits %d, sent %d: want the first fetch", h.hits.Load(), len(h.sent()))
	}
}

// A pass refreshes what is switched on in Staged or Current, once, and not
// an empty own-feed slot.
func TestAPassRefreshesStagedAndCurrentOnce(t *testing.T) {
	h := newFeedHarness(t, false)
	u := h.list("192.0.2.1\n")
	h.fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged:  shared.Rules{Feeds: []string{"own-1", "own-3"}},
		Current: shared.Rules{Feeds: []string{"own-2", "own-1"}},
	}))
	st := h.r.store
	st.mu.Lock()
	s := st.loadLocked()
	s.Own[0] = OwnFeed{Name: "a", URL: u}
	s.Own[1] = OwnFeed{Name: "b", URL: u}
	st.mu.Unlock()

	// On the install's slot for a daily feed, so a feed never attempted is due.
	pinOffset(st, time.Second)
	now := time.Date(2026, 9, 24, 0, 0, 1, 0, time.UTC)
	h.r.now = func() time.Time { return now }

	h.r.pass()
	var ids []string
	for _, p := range h.sent() {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids, []string{"own-1", "own-2"}) {
		t.Fatalf("refreshed %v, want own-1 and own-2 (own-3 has no URL)", ids)
	}
	if st := h.r.store.fetchState("own-3"); !st.LastAttempt.IsZero() {
		t.Errorf("own-3 has no URL and was attempted: %+v", st)
	}
	h.r.pass()
	if n := len(h.sent()); n != 2 {
		t.Errorf("a second pass at once sent %d updates, want still 2: nothing is due", n)
	}
}

// ── what reaches the log, the file and the core ───────────────────────────

func TestNoCredentialURLOrBodyReachesTheLog(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	h := newFeedHarness(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("BODY-MARKER"))
	}))
	t.Cleanup(srv.Close)
	src := plainSource(srv.URL + "/v1/integrations/SECRET-ID/content")
	src.user, src.password = "USER-MARKER", "PASSWORD-MARKER"
	h.r.refreshFrom("own-1", src, false)
	h.r.refreshFrom("own-1", plainSource(h.list("EMPTY-BODY-MARKER\n")), false)

	out := logs.String()
	if !strings.Contains(out, "feed refresh failed") {
		t.Fatalf("nothing logged: %s", out)
	}
	for _, secret := range []string{"SECRET-ID", "USER-MARKER", "PASSWORD-MARKER", "BODY-MARKER", "127.0.0.1"} {
		if strings.Contains(out, secret) {
			t.Errorf("the log carries %q:\n%s", secret, out)
		}
	}
}

func TestTheFeedStateFileIsPrivateAndTheOffsetIsDrawnOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed_fetch.json")
	fs := newFeedStore(path)
	off := fs.offset()
	if off <= 0 || off >= 24*time.Hour {
		t.Fatalf("offset %v", off)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file %v, %v: want 0600", info, err)
	}
	if again := newFeedStore(path).offset(); again != off {
		t.Errorf("offset %v after a restart, want %v: drawn once per install (P15)", again, off)
	}
}

// ── own feeds ─────────────────────────────────────────────────────────────

func TestOwnFeedURLsAreHttpsOrLoopbackHTTP(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://admin.api.crowdsec.net/v1/integrations/abc/content": true,
		"https://example.org:8443/list.txt":                          true,
		"http://127.0.0.1:41412/security/blocklist":                  true,
		"http://[::1]:41412/x":                                       true,
		"http://localhost:41412/x":                                   false, // resolved by name
		"http://[::ffff:127.0.0.1]:41412/x":                          true,
		"http://example.org/list.txt":                                false,
		"http://192.0.2.1/list.txt":                                  false,
		"https://user:pass@example.org/list.txt":                     false,
		"ftp://example.org/list.txt":                                 false,
		"https:///nohost":                                            false,
		"example.org/list.txt":                                       false,
		"https://example.org/" + strings.Repeat("a", 2048):           false,
	} {
		if got := validOwnFeedURL(raw); got != want {
			t.Errorf("%.60s: %v, want %v", raw, got, want)
		}
	}
}

func TestSavingAnOwnFeed(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	good := OwnFeed{Name: "CrowdSec", URL: "https://admin.api.crowdsec.net/v1/integrations/abc/content", User: "u", Password: "p1"}
	if err := s.saveOwnFeed(1, good); err != nil {
		t.Fatal(err)
	}
	s.feedStore.setFetchState("own-1", feedFetchState{ETag: `"x"`, Failures: 4})

	// An empty password keeps the stored one; so do the validators, the list
	// being the same.
	if err := s.saveOwnFeed(1, OwnFeed{Name: "Renamed", URL: good.URL, User: "u"}); err != nil {
		t.Fatal(err)
	}
	if f, _ := s.feedStore.ownFeed(1); f.Password != "p1" || f.Name != "Renamed" {
		t.Errorf("after a save with no password: %+v", f)
	}
	if st := s.feedStore.fetchState("own-1"); st.ETag != `"x"` {
		t.Errorf("a rename forgot the validators: %+v", st)
	}

	// A new URL is another list: its failures and validators go.
	if err := s.saveOwnFeed(1, OwnFeed{Name: "Renamed", URL: "https://example.org/other.txt", User: "u"}); err != nil {
		t.Fatal(err)
	}
	if st := s.feedStore.fetchState("own-1"); st.ETag != "" || st.Failures != 0 {
		t.Errorf("a new URL kept the old list's state: %+v", st)
	}

	// No user, no credentials.
	_ = s.saveOwnFeed(1, OwnFeed{Name: "Renamed", URL: "https://example.org/other.txt"})
	if f, _ := s.feedStore.ownFeed(1); f.Password != "" {
		t.Errorf("a password kept with no user: %+v", f)
	}

	// A held update belongs to the old list and goes with it.
	s.feeds = newFeedRunner(s.client, s.feedStore)
	s.feeds.pending["own-1"] = pendingFeedUpdate{payload: shared.UpdateFeedPayload{ID: "own-1"}}
	if err := s.saveOwnFeed(1, OwnFeed{Name: "Renamed", URL: "https://example.org/third.txt", User: "u"}); err != nil {
		t.Fatal(err)
	}
	if _, held := s.feeds.pending["own-1"]; held {
		t.Error("an update held for the old URL survived the change")
	}
	s.feeds.pending["own-1"] = pendingFeedUpdate{}
	defer func() { s.feeds = nil }()

	// Empty clears the slot, and what was remembered about its list.
	s.feedStore.setFetchState("own-1", feedFetchState{ETag: `"y"`, Failures: 1})
	if err := s.saveOwnFeed(1, OwnFeed{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.feedStore.ownFeed(1); ok {
		t.Error("an empty save did not clear the slot")
	}
	if st := s.feedStore.fetchState("own-1"); st.ETag != "" || st.Failures != 0 {
		t.Errorf("a cleared slot kept its list's state: %+v", st)
	}
	if _, held := s.feeds.pending["own-1"]; held {
		t.Error("an update held for a cleared slot survived")
	}

	for name, c := range map[string]struct {
		n   int
		f   OwnFeed
		err error
	}{
		"slot 0":       {0, good, errOwnFeedSlot},
		"slot 4":       {4, good, errOwnFeedSlot},
		"no name":      {2, OwnFeed{URL: good.URL}, errOwnFeedName},
		"long name":    {2, OwnFeed{Name: strings.Repeat("n", 65), URL: good.URL}, errOwnFeedName},
		"control name": {2, OwnFeed{Name: "a\nb", URL: good.URL}, errOwnFeedName},
		"plain http":   {2, OwnFeed{Name: "x", URL: "http://example.org/l.txt"}, errOwnFeedURL},
		"colon user":   {2, OwnFeed{Name: "x", URL: good.URL, User: "a:b", Password: "p"}, errOwnFeedLogin},
		"control pass": {2, OwnFeed{Name: "x", URL: good.URL, User: "a", Password: "p\x00"}, errOwnFeedLogin},
	} {
		if err := s.saveOwnFeed(c.n, c.f); !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", name, err, c.err)
		}
	}

	s.cfg.DemoMode = true
	if err := s.saveOwnFeed(2, good); !errors.Is(err, errOwnFeedDemo) {
		t.Errorf("demo: %v — a credential typed into the public demo would be kept on its host", err)
	}
}

// ── the row ───────────────────────────────────────────────────────────────

func TestFeedRowsMergeTheCoreAndThisProcess(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "spamhaus-drop", Stored: true, Entries: 1801, Dropped: 2, ChangedAt: time.Now().Add(-31 * 24 * time.Hour),
			CheckedAt: time.Now(), Packets: 7, CountersRead: true, AllowlistOverlap: 3},
		{ID: "dshield", Stored: true, ChangedAt: time.Now()},
	}}))
	s.feedStore.setFetchState("spamhaus-drop", feedFetchState{Status: FeedUnchanged, Rejected: 1, RejectedSample: []string{"x"}})
	s.feedStore.setFetchState("dshield", feedFetchState{Failures: 3, LastError: feedErrTimeout, Status: FeedFailedNoCopy})
	// Stored with a copy at the time; the core has none now.
	s.feedStore.setFetchState("cins", feedFetchState{Failures: 1, LastError: feedErrHTML, Status: FeedFailedCopy})
	s.feedStore.setFetchState("blocklist-de", feedFetchState{Failures: 1, LastError: feedErrShrank, ShrankFrom: 32002, ShrankTo: 900})
	if err := s.saveOwnFeed(2, OwnFeed{Name: "Mine", URL: "https://example.org:8443/a/b.txt?key=SECRET", User: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	state := &shared.RulesState{
		Staged:  shared.Rules{Feeds: []string{"spamhaus-drop", "dshield", "own-2"}},
		Current: shared.Rules{Feeds: []string{"spamhaus-drop"}},
	}

	rows := s.feedRows(state)
	if len(rows) != len(shared.FeedCatalogue)+shared.MaxOwnFeeds {
		t.Fatalf("%d rows", len(rows))
	}
	for i, f := range shared.FeedCatalogue {
		if rows[i].ID != f.ID {
			t.Fatalf("row %d is %s, want the catalogue order", i, rows[i].ID)
		}
	}
	byID := map[string]feedRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}

	sp := byID["spamhaus-drop"]
	if !sp.Staged || !sp.Enabled || !sp.CoreRead || sp.Entries != 1801 || sp.Dropped != 2 || sp.Packets != 7 ||
		!sp.CountersRead || sp.Status != FeedUnchanged || !sp.Stale || sp.AllowlistOverlap != 3 ||
		sp.Rejected != 1 || sp.FailedInARow != 0 || sp.Verdict != shared.FeedRecommended ||
		sp.BlocksKey != "feed_spamhaus_drop_blocks" || sp.FPWhyKey != "feed_spamhaus_drop_fp_why" ||
		sp.Homepage == "" || sp.Terms == "" || !sp.Configured || sp.Own {
		t.Errorf("spamhaus row %+v", sp)
	}
	// Failed with a copy in the core now, whatever was stored at the time.
	if ds := byID["dshield"]; ds.Status != FeedFailedCopy || ds.FailedInARow != 3 || ds.Stale || !ds.Staged || ds.Enabled {
		t.Errorf("dshield row %+v", ds)
	}
	if c := byID["cins"]; c.Status != FeedFailedNoCopy || c.FailedInARow != 0 || c.LastError != feedErrHTML {
		t.Errorf("cins row %+v", c)
	}
	if bd := byID["blocklist-de"]; bd.ShrankFrom != 32002 || bd.ShrankTo != 900 || bd.LastError != feedErrShrank {
		t.Errorf("blocklist-de row %+v", bd)
	}
	if et := byID["et-compromised"]; et.Status != FeedNeverFetched || et.Staged {
		t.Errorf("et row %+v", et)
	}
	own := byID["own-2"]
	if !own.Own || own.OwnN != 2 || !own.Configured || own.Name != "Mine" || own.Host != "example.org:8443" ||
		!own.HasPassword || own.User != "u" || !own.Staged || own.Verdict != "" || own.Homepage != "" {
		t.Errorf("own-2 row %+v", own)
	}
	if o1 := byID["own-1"]; o1.Configured || o1.Name != "Own feed 1" || o1.Host != "" {
		t.Errorf("own-1 row %+v", o1)
	}

	// The core does not answer: the page still renders, and says it could not read the counts.
	fc.SetResponse(shared.CmdGetFeeds, errorRespFor("busy"))
	for _, r := range s.feedRows(nil) {
		if r.CoreRead || r.Staged || r.Entries != 0 {
			t.Errorf("no core, no rules: row %+v", r)
		}
		if r.ID == "cins" && r.Status != FeedFailedCopy {
			t.Errorf("no core: cins %s, want the status stored with the failure", r.Status)
		}
	}
}

// ── demo mode ─────────────────────────────────────────────────────────────

// The public demo makes no outbound request (spec §3). NewServer, not
// newTestServer, because NewServer is what decides.
func TestDemoModeBuildsNoFeedFetcher(t *testing.T) {
	for _, demo := range []bool{true, false} {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "web.toml")
		if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf(`
bind_addr = "127.0.0.1:19878"
socket_path = "%s/none.sock"
ssl_dir = "%s/ssl"
data_dir = "%s"
session_key = "test-session-key-32bytes-padding!"
demo_mode = %v
update_check = false
username = "admin"
password = ""
[tls]
cert = ""
key  = ""
`, dir, dir, dir, demo)), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewServer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Stop)
		if (s.feeds == nil) != demo {
			t.Errorf("demo=%v: runner %v", demo, s.feeds)
		}
		if s.feedStore == nil {
			t.Errorf("demo=%v: no feed store — the blocklist page reads it in every mode", demo)
		}
		if demo {
			s.refreshFeedNow("dshield") // a no-op, and no panic on the nil runner
		}
	}
}
