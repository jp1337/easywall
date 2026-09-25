package web

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The web process fetches other people's lists, parses them, and hands the
// entries to the core, which trusts none of it (spec §3). Nothing here runs in
// demo mode: the public demo makes no outbound request, and NewServer does not
// build a runner there.

// feedFetchTimeout bounds one request, body included (spec §3). A var so a
// test can prove the bound without waiting thirty seconds — notifyTimeout's
// reason.
var feedFetchTimeout = 30 * time.Second

// feedTick is how often the runner looks for a feed that is due. A var for
// the same reason.
var feedTick = time.Minute

const (
	// feedMaxBody caps a body after decompression (spec §3). The transport
	// asks for gzip and decompresses on its own while nobody sets
	// Accept-Encoding (net/http transport.go, requestedGzip); the cap is read
	// through that, so it bounds what a gzip bomb expands to.
	feedMaxBody = 8 << 20

	// feedApplyRetry: the core refuses UPDATE_FEED while an apply cycle holds
	// the slot, which it does for the whole window (P7). The parsed update is
	// kept in memory and offered again — not fetched again, which ET's "not
	// more than once per hour" would not allow.
	feedApplyRetry = 5 * time.Minute

	// feedStaleAfter: a copy the core has not seen change for this long gets a
	// warning. Feodo went quiet for six months with HTTP 200 every time.
	feedStaleAfter = 30 * 24 * time.Hour

	// feedFailWarnAfter: the row says "failed N times in a row" from here.
	feedFailWarnAfter = 2
)

var errFeedRedirect = errors.New("feed redirected")

// feedFailure is why one refresh did not reach the core: a reason from
// AllFeedErrors, and the HTTP status when that is the reason.
type feedFailure struct {
	reason string
	status int
}

func (e *feedFailure) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("%s %d", e.reason, e.status)
	}
	return e.reason
}

// feedSource is where a feed comes from and how it is read.
type feedSource struct {
	urls           []string
	format         shared.FeedFormat
	interval       time.Duration
	user, password string
}

type pendingFeedUpdate struct {
	payload shared.UpdateFeedPayload
	next    feedFetchState // stored once the core takes it
	at      time.Time
}

// feedRunner schedules and performs refreshes. One per non-demo server.
type feedRunner struct {
	client *CoreClient
	store  *feedStore
	http   *http.Client
	now    func() time.Time

	mu       sync.Mutex
	inflight map[string]bool
	pending  map[string]pendingFeedUpdate
	// firstSeen is when a pass first found a feed with no attempt on
	// record; its first pull waits for this install's slot after it.
	firstSeen map[string]time.Time
}

func newFeedRunner(client *CoreClient, store *feedStore) *feedRunner {
	return &feedRunner{
		client: client,
		store:  store,
		// The zero Transport is http.DefaultTransport: system roots and
		// ProxyFromEnvironment, like every other outbound request here.
		http: &http.Client{
			Timeout: feedFetchTimeout,
			// Refused, not followed — shared.Reporter's reasoning, and an own
			// feed's credentials must not travel to wherever it points.
			CheckRedirect: func(*http.Request, []*http.Request) error { return errFeedRedirect },
		},
		now:       time.Now,
		inflight:  map[string]bool{},
		pending:   map[string]pendingFeedUpdate{},
		firstSeen: map[string]time.Time{},
	}
}

func feedUserAgent() string {
	// DShield asks for a User-Agent that identifies the client.
	return "easywall/" + shared.CurrentVersion + " (+https://easywall-project.org)"
}

// run refreshes whatever is due, now and every feedTick, until done closes.
func (r *feedRunner) run(done <-chan struct{}) {
	t := time.NewTicker(feedTick)
	defer t.Stop()
	for {
		r.pass()
		select {
		case <-t.C:
		case <-done:
			return
		}
	}
}

// pass refreshes, one after another, every feed switched on in Staged or
// Current that is due. An id in both is seen twice and refreshed once: the
// first refresh makes it not due.
func (r *feedRunner) pass() {
	state, err := r.client.GetRules()
	if err != nil {
		slog.Debug("feeds: rules not read; no refresh this pass", "error", err)
		return
	}
	on := append(slices.Clone(state.Staged.Feeds), state.Current.Feeds...)
	r.dropSwitchedOff(on)
	now := r.now()
	for _, id := range on {
		src, ok := r.source(id)
		if ok && r.due(id, src.interval, now) {
			r.refresh(id, false)
		}
	}
}

// dropSwitchedOff drops the update held for the apply window of every feed
// no longer switched on, so it is never resent (Review Focus 1). The failure
// stays in the store — it drives the backoff, and switching the feed off and
// on again must not be a way around it (X7); feedRows hides it instead.
func (r *feedRunner) dropSwitchedOff(on []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.pending {
		if !slices.Contains(on, id) {
			delete(r.pending, id)
		}
	}
}

// refreshNow is the first fetch after a feed is switched on, so the apply
// finds a copy (spec §3). Nothing happens when the core already holds one.
func (r *feedRunner) refreshNow(id string) {
	go r.refresh(id, true)
}

// source resolves an id to what is fetched. False for an id nothing knows and
// for an own-feed slot with no URL.
func (r *feedRunner) source(id string) (feedSource, bool) {
	if f, ok := shared.CatalogueFeedByID(id); ok {
		return feedSource{urls: f.URLs, format: f.Format, interval: f.Interval}, true
	}
	for n := 1; n <= shared.MaxOwnFeeds; n++ {
		if id == shared.OwnFeedID(n) {
			o, ok := r.store.ownFeed(n)
			return feedSource{
				urls: []string{o.URL}, format: shared.FeedFormatPlain,
				interval: shared.OwnFeedInterval, user: o.User, password: o.Password,
			}, ok
		}
	}
	return feedSource{}, false
}

// due: an update the core deferred waits for its retry; a feed with an
// attempt on record waits nextFeedAttempt; one with none waits for this
// install's first slot after a pass first found it.
func (r *feedRunner) due(id string, interval time.Duration, now time.Time) bool {
	r.mu.Lock()
	p, held := r.pending[id]
	seen, known := r.firstSeen[id]
	r.mu.Unlock()
	if held {
		return !now.Before(p.at)
	}
	st := r.store.fetchState(id)
	if !st.LastAttempt.IsZero() {
		return !now.Before(nextFeedAttempt(st, interval))
	}
	if !known {
		seen = now
		r.mu.Lock()
		r.firstSeen[id] = now
		r.mu.Unlock()
	}
	return !now.Before(firstFeedSlot(seen, interval, r.store.offset()))
}

// nextFeedAttempt is when a feed with an attempt on record is fetched again:
// one whole interval after the last, never earlier — catalogue and own feeds
// alike, so ET's one an hour and CrowdSec's one a day (a pull at 23 h 59 min
// is a 429) both hold — or the backoff after a failure.
func nextFeedAttempt(st feedFetchState, interval time.Duration) time.Time {
	if st.Failures > 0 {
		return st.LastAttempt.Add(feedRetryDelay(st, interval))
	}
	return st.LastAttempt.Add(interval)
}

// firstFeedSlot is the first slot at or after seen. Slots sit at this
// install's offset within each interval, counted from the Unix epoch (P15):
// installations that start together do not all make their first pull of a
// list at the same moment. Only a feed with no attempt on record waits for
// one — switching a feed on fetches at once (refreshFeedNow).
func firstFeedSlot(seen time.Time, interval, offset time.Duration) time.Time {
	base := time.Unix(0, 0).Add(offset % interval)
	slot := base.Add(seen.Sub(base) / interval * interval)
	if slot.Before(seen) {
		slot = slot.Add(interval)
	}
	return slot
}

// feedRetryDelay is the wait after a failure: an hour, doubling, never beyond
// the feed's own interval. An hour is the floor because a failed request is
// still a request — Spamhaus asks for an hour between downloads, ET for no
// more than one an hour. A 429 waits the whole interval: the list said "too
// often" (DShield asks for 5 minutes, CrowdSec's free tier for 24 hours; the
// interval satisfies both).
func feedRetryDelay(st feedFetchState, interval time.Duration) time.Duration {
	if st.LastError == feedErrRateLimited {
		return interval
	}
	d := time.Hour << min(st.Failures-1, 5)
	return min(d, interval)
}

func (r *feedRunner) claim(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inflight[id] {
		return false
	}
	r.inflight[id] = true
	return true
}

func (r *feedRunner) release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inflight, id)
}

// coreCopy asks the core what it stores for id. The zero status — no copy —
// when it cannot say, which only makes the next request unconditional.
func (r *feedRunner) coreCopy(id string) shared.FeedStatus {
	res, err := r.client.GetFeeds("")
	if err != nil {
		return shared.FeedStatus{}
	}
	for _, f := range res.Feeds {
		if f.ID == id {
			return f
		}
	}
	return shared.FeedStatus{}
}

// forget drops an update held for id, and when it was first seen due.
// saveOwnFeed calls it when the slot's list changes: the held entries and
// validators are the old list's, and so is firstSeen — without clearing it,
// a source pointed at a new address inherited a sighting from whenever a
// pass first found the old one, days earlier, and firstFeedSlot computed a
// slot already in the past, making the next tick pull it at once (review
// round 1, finding 2). Clearing it here, rather than keeping the deleted
// LastAttempt, treats an edited source as what it is — never yet pulled from
// its new address — so it waits for a fresh install slot like any other
// feed with no attempt on record (P25 governs only a feed's first pull).
func (r *feedRunner) forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
	delete(r.firstSeen, id)
}

// refresh fetches one feed, parses it and hands it to the core. With
// onlyWithoutCopy it does nothing if the core already holds a copy.
func (r *feedRunner) refresh(id string, onlyWithoutCopy bool) {
	if !r.claim(id) {
		return
	}
	defer r.release(id)

	r.mu.Lock()
	p, ok := r.pending[id]
	delete(r.pending, id)
	r.mu.Unlock()
	if ok {
		r.deliver(id, p.payload, p.next, r.coreCopy(id))
		return
	}

	if src, ok := r.source(id); ok {
		r.refreshFrom(id, src, onlyWithoutCopy)
	}
}

// refreshFrom is refresh for a source already resolved — the seam the tests
// point at an httptest server.
func (r *feedRunner) refreshFrom(id string, src feedSource, onlyWithoutCopy bool) {
	core := r.coreCopy(id)
	stored := core.Stored
	st := r.store.fetchState(id)
	// A switch-on fetches only for a feed with no copy, and never inside a
	// running backoff: after a 429 or a timeout, each off-and-on would
	// otherwise be one more request to the same host.
	if onlyWithoutCopy && (stored || st.Failures > 0 &&
		r.now().Before(nextFeedAttempt(st, src.interval))) {
		return
	}

	st.LastAttempt = r.now()
	// Before the request: a process that dies mid-refresh does not come back
	// and fetch again at once.
	r.store.setFetchState(id, st)

	// Conditional only when the core holds the copy a 304 would point at —
	// otherwise a feed switched off, pruned by the core, and switched on
	// again would be told "not modified" for ever. And only for a one-URL
	// feed: a 304 on one of Spamhaus's two files says nothing about the
	// other, and this process keeps no entries to merge it with.
	conditional := stored && len(src.urls) == 1

	next := st
	var entries []string
	next.Rejected, next.RejectedSample = 0, nil
	payload := shared.UpdateFeedPayload{ID: id}
	for _, u := range src.urls {
		res, err := r.fetch(u, st, conditional, src)
		if err != nil {
			r.fail(id, st, err, stored)
			return
		}
		if res.notModified {
			payload.NotModified = true
			next.Rejected, next.RejectedSample = st.Rejected, st.RejectedSample
			break
		}
		// Every URL is one of the catalogue's three formats or an own
		// feed's plain text, so the format error cannot happen.
		e, rejected, sample, _ := parseFeed(src.format, res.body)
		entries = append(entries, e...)
		next.Rejected += rejected
		for _, line := range sample {
			if len(next.RejectedSample) < feedSampleLines {
				next.RejectedSample = append(next.RejectedSample, line)
			}
		}
		if len(src.urls) == 1 {
			next.ETag, next.LastModified = res.etag, res.lastModified
		} else {
			next.ETag, next.LastModified = "", ""
		}
	}

	if !payload.NotModified {
		if len(src.urls) > 1 {
			slices.Sort(entries)
			entries = slices.Compact(entries)
		}
		switch {
		case len(entries) == 0:
			// dan.me.uk's "you can only fetch every 30 minutes" is a 200:
			// every line rejected, and the sample shows the sentence.
			st.Rejected, st.RejectedSample = next.Rejected, next.RejectedSample
			r.fail(id, st, &feedFailure{reason: feedErrEmpty}, stored)
			return
		case len(entries) > shared.FeedMaxEntries:
			r.fail(id, st, &feedFailure{reason: feedErrTooLarge}, stored)
			return
		}
		payload.Entries = entries
	}
	r.deliver(id, payload, next, core)
}

// deliver sends one update to the core and records what came of it.
func (r *feedRunner) deliver(id string, payload shared.UpdateFeedPayload, next feedFetchState, core shared.FeedStatus) {
	stored := core.Stored
	res, err := r.client.UpdateFeed(payload)
	switch {
	case err == nil:
		next.Failures, next.LastError, next.HTTPStatus = 0, "", 0
		next.ShrankFrom, next.ShrankTo = 0, 0
		next.Status = FeedUnchanged
		if !payload.NotModified && (res.Changed || !stored) {
			next.Status = FeedUpdated
		}
		r.store.setFetchState(id, next)
		slog.Info("feed refreshed", "feed", id, "not_modified", payload.NotModified,
			"sent", len(payload.Entries), "stored", res.After, "dropped", res.Dropped,
			"rejected_lines", next.Rejected)
	case strings.Contains(err.Error(), shared.ErrApplyInProgressText):
		r.mu.Lock()
		r.pending[id] = pendingFeedUpdate{payload: payload, next: next, at: r.now().Add(feedApplyRetry)}
		r.mu.Unlock()
		slog.Info("feed update deferred until the apply window closes", "feed", id)
	case errors.Is(err, shared.ErrRequestTooLarge):
		r.fail(id, r.store.fetchState(id), &feedFailure{reason: feedErrTooLarge}, stored)
	case strings.Contains(err.Error(), shared.ErrFeedShrankText):
		// Refused for the live set's sake (spec D5, ruling X1). The row
		// says from what to what, and how to take the smaller list.
		st := r.store.fetchState(id)
		st.ShrankFrom, st.ShrankTo = core.Entries, len(payload.Entries)
		r.fail(id, st, &feedFailure{reason: feedErrShrank}, true)
	case strings.HasPrefix(err.Error(), "core error: "):
		// The core's own audit entry (feed_refused) says which guard.
		r.fail(id, r.store.fetchState(id), &feedFailure{reason: feedErrRefused}, stored)
	default:
		r.fail(id, r.store.fetchState(id), &feedFailure{reason: feedErrCoreUnreachable}, stored)
	}
}

// fail records a refresh that changed nothing: the previous copy, if any,
// stays in the kernel and on disk, and so do the old validators.
func (r *feedRunner) fail(id string, st feedFetchState, err error, stored bool) {
	var f *feedFailure
	if !errors.As(err, &f) {
		f = &feedFailure{reason: feedErrUnreachable}
	}
	st.LastError, st.HTTPStatus = f.reason, f.status
	st.Failures++
	st.Status = FeedFailedNoCopy
	if stored {
		st.Status = FeedFailedCopy
	}
	r.store.setFetchState(id, st)
	// The reason only: never the body, the URL (an own feed's may carry its
	// integration id) or the credentials.
	slog.Warn("feed refresh failed", "feed", id, "reason", f.reason,
		"http_status", f.status, "failures", st.Failures)
}

type fetched struct {
	body               []byte
	etag, lastModified string
	notModified        bool
}

// fetch performs one GET. Every failure is a *feedFailure.
func (r *feedRunner) fetch(rawURL string, st feedFetchState, conditional bool, src feedSource) (fetched, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fetched{}, &feedFailure{reason: feedErrUnreachable}
	}
	req.Header.Set("User-Agent", feedUserAgent())
	if conditional {
		// Both: GitHub raw honours only If-None-Match (research §pihole).
		if st.ETag != "" {
			req.Header.Set("If-None-Match", st.ETag)
		}
		if st.LastModified != "" {
			req.Header.Set("If-Modified-Since", st.LastModified)
		}
	}
	if src.user != "" {
		req.SetBasicAuth(src.user, src.password)
	}

	// #nosec G704 -- a catalogue URL, or an own feed's, which an authenticated
	// operator set and saveOwnFeed held to https or loopback http (P14).
	resp, err := r.http.Do(req)
	if err != nil {
		return fetched{}, requestFailure(err, feedErrUnreachable)
	}
	defer resp.Body.Close() //nolint:errcheck // read in full or abandoned

	switch {
	case resp.StatusCode == http.StatusNotModified && conditional:
		return fetched{notModified: true}, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return fetched{}, &feedFailure{reason: feedErrRateLimited}
	case resp.StatusCode != http.StatusOK:
		return fetched{}, &feedFailure{reason: feedErrHTTPStatus, status: resp.StatusCode}
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "text/html" {
		return fetched{}, &feedFailure{reason: feedErrHTML}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody+1))
	if err != nil {
		return fetched{}, requestFailure(err, feedErrTransfer)
	}
	if len(body) > feedMaxBody {
		return fetched{}, &feedFailure{reason: feedErrTooLarge}
	}
	if t := bytes.TrimLeft(body, " \t\r\n"); len(t) > 0 && t[0] == '<' {
		return fetched{}, &feedFailure{reason: feedErrHTML}
	}
	return fetched{body: body, etag: resp.Header.Get("ETag"), lastModified: resp.Header.Get("Last-Modified")}, nil
}

// requestFailure names a transport error: a refused redirect, the timeout,
// or otherwise the reason given.
func requestFailure(err error, otherwise string) *feedFailure {
	var ne net.Error
	switch {
	case errors.Is(err, errFeedRedirect):
		return &feedFailure{reason: feedErrRedirected}
	case errors.As(err, &ne) && ne.Timeout():
		return &feedFailure{reason: feedErrTimeout}
	}
	return &feedFailure{reason: otherwise}
}

// refreshFeedNow starts the first fetch of a feed just switched on. No-op in
// demo mode, which has no runner.
func (s *Server) refreshFeedNow(id string) {
	if s.feeds != nil {
		s.feeds.refreshNow(id)
	}
}

// feedRow is one row of the Feeds card: everything Task 7 renders for a feed.
type feedRow struct {
	ID   string
	Name string // the catalogue's; an own feed's own name, or "Own feed N" for an empty slot
	Own  bool
	OwnN int // 1..3 for an own feed

	// Configured is false only for an own-feed slot with no URL: an enabled
	// id there renders "not configured" (P14).
	Configured bool

	// From the catalogue; zero for an own feed.
	Verdict        shared.FeedVerdict
	FalsePositives shared.FeedFalsePositives
	BlocksKey      string // FeedLocaleKey(id, "blocks")
	FPWhyKey       string // FeedLocaleKey(id, "fp_why")
	ConfirmKey     string // FeedLocaleKey(id, "confirm") for a ✗ feed: whom it locks out; "" otherwise
	Homepage       string // P17: the source link
	Terms          string // P17: the terms link

	// An own feed's: Host is its URL's host[:port] — the row's source link
	// text, never the path, query or credentials (P17). URL and User fill the
	// edit form; HasPassword says a password is stored, which the form never
	// shows back.
	Host        string
	URL         string
	User        string
	HasPassword bool

	Interval time.Duration

	Staged  bool // switched on in Staged: the toggle's position
	Enabled bool // on in Current: in the kernel now

	Status FeedStatusKind

	// The core's view (GET_FEEDS). CoreRead is false when it did not answer;
	// then everything below it from the core is zero and the card says so.
	CoreRead     bool
	Stored       bool
	Entries      int
	Dropped      int // not globally routable, stripped by the core
	ChangedAt    time.Time
	CheckedAt    time.Time
	Packets      uint64
	CountersRead bool

	// This process's view.
	Rejected       int      // lines that were not an entry, last body
	RejectedSample []string // the first five of them

	// ShrankFrom and ShrankTo: the core refused the last list as a shrink
	// of the live copy (X1) — its stored count, and the count sent. Zero
	// otherwise; cleared by the next update the core takes.
	ShrankFrom  int
	ShrankTo    int
	LastAttempt time.Time
	LastError   string // one of AllFeedErrors; "" after a success
	HTTPStatus  int
	Failures    int

	// Warnings (spec §5), each zero when it does not apply.
	Stale            bool // a copy unchanged for 30 days
	AllowlistOverlap int  // staged allowlist entries this copy covers
	FailedInARow     int  // Failures, from feedFailWarnAfter on
}

// feedRows is the Feeds card: the catalogue in its order, then the three own
// feed slots. state is the page's own GET_RULES answer, nil when it failed.
func (s *Server) feedRows(state *shared.RulesState) []feedRow {
	var staged, current []string
	if state != nil {
		staged, current = state.Staged.Feeds, state.Current.Feeds
	}
	core := map[string]shared.FeedStatus{}
	res, err := s.client.GetFeeds("")
	coreRead := err == nil
	if err != nil {
		slog.Debug("feeds: core status not read", "error", err)
	}
	for _, f := range res.Feeds {
		core[f.ID] = f
	}

	rows := make([]feedRow, 0, len(shared.FeedCatalogue)+shared.MaxOwnFeeds)
	for _, f := range shared.FeedCatalogue {
		row := feedRow{
			ID: f.ID, Name: f.Name, Configured: true,
			Verdict: f.Verdict, FalsePositives: f.FalsePositives,
			BlocksKey: shared.FeedLocaleKey(f.ID, "blocks"), FPWhyKey: shared.FeedLocaleKey(f.ID, "fp_why"),
			Homepage: f.Homepage, Terms: f.Terms, Interval: f.Interval,
		}
		if f.Verdict == shared.FeedDeliberate {
			row.ConfirmKey = shared.FeedLocaleKey(f.ID, "confirm")
		}
		rows = append(rows, row)
	}
	for n := 1; n <= shared.MaxOwnFeeds; n++ {
		id := shared.OwnFeedID(n)
		o, ok := s.feedStore.ownFeed(n)
		row := feedRow{ID: id, Name: shared.FeedDisplayName(id), Own: true, OwnN: n,
			Configured: ok, Interval: shared.OwnFeedInterval}
		if ok {
			row.Name, row.URL, row.User, row.HasPassword = o.Name, o.URL, o.User, o.Password != ""
			if u, err := url.Parse(o.URL); err == nil {
				row.Host = u.Host
			}
		}
		rows = append(rows, row)
	}

	now := time.Now()
	for i := range rows {
		r := &rows[i]
		r.Staged, r.Enabled = slices.Contains(staged, r.ID), slices.Contains(current, r.ID)
		c := core[r.ID]
		r.CoreRead, r.Stored, r.Entries, r.Dropped = coreRead, c.Stored, c.Entries, c.Dropped
		r.ChangedAt, r.CheckedAt, r.Packets, r.CountersRead = c.ChangedAt, c.CheckedAt, c.Packets, c.CountersRead
		r.AllowlistOverlap = c.AllowlistOverlap

		st := s.feedStore.fetchState(r.ID)
		if !r.Staged && !r.Enabled {
			// Switched off: no error on the row — often it is the core's "not
			// switched on" for a fetch that was in flight. The store keeps the
			// failure for the backoff; only the display forgets it.
			st.LastError, st.HTTPStatus, st.Failures, st.Status = "", 0, 0, ""
			st.ShrankFrom, st.ShrankTo = 0, 0
		}
		r.Rejected, r.RejectedSample, r.LastAttempt = st.Rejected, st.RejectedSample, st.LastAttempt
		r.ShrankFrom, r.ShrankTo = st.ShrankFrom, st.ShrankTo
		r.LastError, r.HTTPStatus, r.Failures = st.LastError, st.HTTPStatus, st.Failures
		r.Status = feedStatusOf(st, c.Stored, coreRead)

		r.Stale = c.Stored && !c.ChangedAt.IsZero() && now.Sub(c.ChangedAt) >= feedStaleAfter
		if st.Failures >= feedFailWarnAfter {
			r.FailedInARow = st.Failures
		}
	}
	return rows
}

// feedStatusOf merges the two views. A failure's copy-or-not is the core's
// answer now, not the one at the time; when the core did not answer, the
// status stored with the failure stands.
func feedStatusOf(st feedFetchState, stored, coreRead bool) FeedStatusKind {
	switch {
	case st.Failures > 0 && !coreRead:
		return st.Status
	case st.Failures > 0 && stored:
		return FeedFailedCopy
	case st.Failures > 0:
		return FeedFailedNoCopy
	case st.Status == FeedUpdated || st.Status == FeedUnchanged:
		return st.Status
	case stored:
		// A copy this process has no record of — its state file was lost.
		return FeedUnchanged
	}
	return FeedNeverFetched
}
