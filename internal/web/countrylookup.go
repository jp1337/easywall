package web

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The country lookup (2.27, spec §1): ipverse's release archive, fetched once
// a day by this process while web.toml's country_lookup is true — the
// default — kept in <data_dir>/web/countries.tar.gz, and asked by /blocked for
// each row's source. Off, none of this exists: NewServer builds no lookup, so
// nothing here can make a request.

// countryArchiveURL is ipverse's newest release asset: every country's
// aggregated.json in one file, 1 747 173 bytes on 2026-10-14. It answers
// through two redirects, and the last host honours If-None-Match and
// If-Modified-Since with a 304 (measured with curl -L). A var so a test can
// point it at a listener of its own.
var countryArchiveURL = "https://github.com/ipverse/country-ip-blocks/releases/latest/download/country-ip-blocks.tar.gz"

// countryLookupKey is the lookup's entry in feed_fetch.json — validators, last
// attempt, failures — beside the feeds'. Not "country-": it is no feed, and
// nothing that walks the feeds' ids reaches it.
const countryLookupKey = "country_lookup"

// countryArchiveHosts and countryArchiveMaxRedirects are the only redirects
// the download follows (spec D2). Measured 2026-10-14: github.com/…/latest/
// download/… → 302 github.com/…/download/latest/… → 302
// release-assets.githubusercontent.com/…. The feeds' client refuses every
// redirect (2.23); this one is its own client, not a loosening of theirs.
var countryArchiveHosts = []string{"github.com", "release-assets.githubusercontent.com"}

const countryArchiveMaxRedirects = 3

// countryArchiveRedirect follows a redirect only over https, on the default
// port, with no user in the URL, to one of countryArchiveHosts, and at most
// countryArchiveMaxRedirects of them. via holds the requests already sent, so
// len(via) is the number of the redirect being asked about.
func countryArchiveRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > countryArchiveMaxRedirects || req.URL.Scheme != "https" || req.URL.Port() != "" ||
		req.URL.User != nil || !slices.Contains(countryArchiveHosts, req.URL.Hostname()) {
		return errFeedRedirect
	}
	return nil
}

type countryLookup struct {
	path  string
	store *feedStore
	http  *http.Client
	now   func() time.Time
	// table is swapped whole by one atomic store: a lookup reads the old
	// table or the new one, never a half-built one, and takes no lock.
	table atomic.Pointer[countryTable]
	busy  sync.Mutex
}

func newCountryLookup(path string, store *feedStore) *countryLookup {
	return &countryLookup{
		path: path, store: store, now: time.Now,
		// The zero Transport is http.DefaultTransport, as the feeds' is.
		http: &http.Client{Timeout: feedFetchTimeout, CheckRedirect: countryArchiveRedirect},
	}
}

// country is the code addr's block is registered to, lower case; "" when
// none is, or while no table is loaded.
func (l *countryLookup) country(addr netip.Addr) string { return l.table.Load().lookup(addr) }

// countryLookupStatus is the one line /blocked shows about the table.
type countryLookupStatus struct {
	Loaded     bool
	Countries  int
	Ranges     int
	CheckedAt  time.Time // the last attempt; zero before the first
	LastError  string    // one of AllFeedErrors; "" after a success
	HTTPStatus int
}

// ReasonKey is the locale key that names LastError: the feeds' sentence,
// except for a redirect. A feed refuses every redirect; the lookup follows
// GitHub's own and refuses only one away from them, which the feeds' "the
// server redirected" would misstate (pre-flight M5).
func (s countryLookupStatus) ReasonKey() string {
	if s.LastError == feedErrRedirected {
		return "blocked_lookup_redirected"
	}
	return "feed_error_" + s.LastError
}

func (l *countryLookup) status() countryLookupStatus {
	st := l.store.fetchState(countryLookupKey)
	out := countryLookupStatus{CheckedAt: st.LastAttempt, LastError: st.LastError, HTTPStatus: st.HTTPStatus}
	if t := l.table.Load(); t != nil {
		out.Loaded, out.Countries, out.Ranges = true, t.countries, t.ranges()
	}
	return out
}

// load reads the copy a previous run kept. Missing or unreadable, the table
// stays empty and due says fetch.
func (l *countryLookup) load() {
	raw, err := os.ReadFile(l.path) // #nosec G304 -- a fixed name in this process's own state dir
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("country table not read; fetching it again", "path", l.path, "error", err)
		}
		return
	}
	t, err := parseCountryArchive(bytes.NewReader(raw))
	if err != nil {
		slog.Warn("country table on disk unreadable; fetching it again", "path", l.path, "error", err)
		return
	}
	l.table.Store(t)
	// A build allocates about 69 MB for a table that keeps 3.6 MB. Left to
	// the scavenger, easywall-web stayed 27 MB above its size without the
	// lookup; handed back at once, 10 MB after a download and 4 MB after a
	// restart (measured, 2.27 plan, D4).
	debug.FreeOSMemory()
}

// due: with no table, at once — unless a failure's backoff is running; with
// one, a day after the last attempt, as a country's feed (spec D1).
func (l *countryLookup) due(now time.Time) bool {
	st := l.store.fetchState(countryLookupKey)
	if st.LastAttempt.IsZero() || l.table.Load() == nil && st.Failures == 0 {
		return true
	}
	return !now.Before(nextFeedAttempt(st, shared.CountryInterval))
}

// refresh fetches the archive — conditionally while a table is loaded — and
// swaps the new table in whole. Any failure keeps the table in use and the
// copy on disk.
func (l *countryLookup) refresh() {
	if !l.busy.TryLock() {
		return
	}
	defer l.busy.Unlock()
	st := l.store.fetchState(countryLookupKey)
	have := l.table.Load() != nil
	st.LastAttempt = l.now()
	l.store.setFetchState(countryLookupKey, st)

	req, err := http.NewRequest(http.MethodGet, countryArchiveURL, nil)
	if err != nil {
		l.fail(st, err)
		return
	}
	req.Header.Set("User-Agent", feedUserAgent())
	if have {
		setValidators(req, st)
	}
	res, err := readFeedResponse(l.http, req, have)
	if err != nil {
		l.fail(st, err)
		return
	}
	st.Status = FeedUnchanged
	if !res.notModified {
		t, err := parseCountryArchive(bytes.NewReader(res.body))
		if err != nil {
			reason := feedErrTransfer // broke off, or would not decompress
			switch {
			case errors.Is(err, errCountryArchiveEmpty):
				reason = feedErrEmpty
			case errors.Is(err, errCountryArchiveTooLong):
				reason = feedErrTooLarge
			}
			l.fail(st, &feedFailure{reason: reason})
			return
		}
		// The validators describe the copy on disk, so they change only
		// with it: kept with a copy that was not written, a restart would
		// confirm yesterday's copy with today's ETag (pre-flight M3).
		if err := writeFileAtomic(l.path, res.body, 0600); err != nil {
			slog.Warn("country table not kept on disk; a restart fetches it again", "path", l.path, "error", err)
		} else {
			st.ETag, st.LastModified = res.etag, res.lastModified
		}
		l.table.Store(t)
		debug.FreeOSMemory() // as in load
		st.Status = FeedUpdated
	}
	st.Failures, st.LastError, st.HTTPStatus = 0, "", 0
	l.store.setFetchState(countryLookupKey, st)
	s := l.status()
	slog.Info("country table refreshed", "not_modified", res.notModified, "countries", s.Countries, "ranges", s.Ranges)
}

// fail records a refresh that changed nothing. The reason only, never the
// body: what a server sends is the one thing here nobody wrote.
func (l *countryLookup) fail(st feedFetchState, err error) {
	var f *feedFailure
	if !errors.As(err, &f) {
		f = &feedFailure{reason: feedErrUnreachable}
	}
	st.LastError, st.HTTPStatus = f.reason, f.status
	st.Failures++
	st.Status = FeedFailedNoCopy
	if l.table.Load() != nil {
		st.Status = FeedFailedCopy
	}
	l.store.setFetchState(countryLookupKey, st)
	slog.Warn("country table refresh failed", "reason", f.reason, "http_status", f.status, "failures", st.Failures)
}
