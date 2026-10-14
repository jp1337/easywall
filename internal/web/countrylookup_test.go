package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The hop chain measured on 2026-10-14 (curl -L): the latest-release URL,
// GitHub's own second form of it, then the signed asset URL.
const (
	archiveHop2 = "https://github.com/ipverse/country-ip-blocks/releases/download/latest/country-ip-blocks.tar.gz"
	archiveHop3 = "https://release-assets.githubusercontent.com/github-production-release-asset/422834607/2f20eaed?sig=x"
)

// archiveServer answers the lookup's requests in-process: no request leaves
// the test. next maps a URL to the one it redirects to; body is what the last
// hop serves, with etag, and a 304 to a request that carries etag already.
type archiveServer struct {
	mu   sync.Mutex
	next map[string]string
	body []byte
	etag string
	seen []string
}

func (a *archiveServer) RoundTrip(req *http.Request) (*http.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	u := req.URL.String()
	a.seen = append(a.seen, u)
	if to, ok := a.next[u]; ok {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {to}},
			Body: http.NoBody, Request: req}, nil
	}
	if req.Header.Get("If-None-Match") == a.etag {
		return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{}, Body: http.NoBody, Request: req}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Request: req,
		Header: http.Header{"Content-Type": {"application/octet-stream"}, "Etag": {a.etag}},
		Body:   io.NopCloser(bytes.NewReader(a.body))}, nil
}

func (a *archiveServer) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.seen)
}

func (a *archiveServer) serve(body []byte, etag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.body, a.etag = body, etag
}

func githubsChain() map[string]string {
	return map[string]string{countryArchiveURL: archiveHop2, archiveHop2: archiveHop3}
}

func testLookup(t *testing.T, a *archiveServer) *countryLookup {
	t.Helper()
	dir := t.TempDir()
	l := newCountryLookup(filepath.Join(dir, "countries.tar.gz"), newFeedStore(filepath.Join(dir, "feed_fetch.json")))
	l.http.Transport = a
	return l
}

var (
	de = netip.MustParseAddr("192.0.2.80")
	ie = netip.MustParseAddr("198.51.100.7")
)

// Spec D1, D2, D4: GitHub's two redirects are followed, the table is swapped
// in and kept on disk, the next day's request is answered 304, and a restart
// reads the copy instead of fetching.
func TestTheCountryArchiveFollowsGitHubsTwoRedirects(t *testing.T) {
	a := &archiveServer{next: githubsChain(), body: countryArchive(t, sampleCountryFiles...), etag: `"0x8DF2"`}
	l := testLookup(t, a)

	l.refresh()
	if got := a.requests(); !slices.Equal(got, []string{countryArchiveURL, archiveHop2, archiveHop3}) {
		t.Fatalf("requests %v, want the measured chain", got)
	}
	st := l.status()
	if !st.Loaded || st.Countries != 3 || st.LastError != "" || st.CheckedAt.IsZero() {
		t.Fatalf("status after the first refresh: %+v", st)
	}
	if got := l.country(de); got != "de" {
		t.Errorf("country(%s) = %q, want de", de, got)
	}
	if raw, err := os.ReadFile(l.path); err != nil || !bytes.Equal(raw, a.body) {
		t.Fatalf("the archive was not kept on disk as fetched: %v", err)
	}

	before := l.table.Load()
	l.refresh()
	if fs := l.store.fetchState(countryLookupKey); fs.Status != FeedUnchanged || fs.LastError != "" {
		t.Errorf("the second refresh: %+v, want unchanged — a 304 through both redirects", fs)
	}
	if l.table.Load() != before {
		t.Error("a 304 replaced the table")
	}

	// A restart: the runner loads the copy before its first pass asks
	// whether the table is due, so the pass fetches nothing.
	n := len(a.requests())
	again := newCountryLookup(l.path, l.store)
	again.http.Transport = a
	r := testRunner(t)
	r.lookup = again
	done := make(chan struct{})
	close(done)
	r.run(done) // one pass, then done
	if got := again.country(de); got != "de" || len(a.requests()) != n {
		t.Errorf("after a restart: country %q, %d requests; want de and none", got, len(a.requests())-n)
	}
}

// A copy on disk that does not parse is no table: load leaves it empty, the
// lookup is due at once, and the next refresh replaces the file.
func TestACorruptCopyIsRefusedAndFetchedAgain(t *testing.T) {
	a := &archiveServer{next: githubsChain(), body: countryArchive(t, sampleCountryFiles...), etag: `"1"`}
	l := testLookup(t, a)
	if err := os.WriteFile(l.path, []byte("not an archive"), 0600); err != nil {
		t.Fatal(err)
	}
	l = newCountryLookup(l.path, l.store) // a start, reading that copy
	l.http.Transport = a
	l.load()
	if l.table.Load() != nil {
		t.Fatal("a corrupt copy was loaded as a table")
	}
	if !l.due(time.Now()) {
		t.Fatal("a corrupt copy on disk: not due")
	}
	l.refresh()
	if len(a.requests()) == 0 {
		t.Fatal("refresh made no request")
	}
	if got := l.country(de); got != "de" {
		t.Errorf("country(%s) = %q after the fetch, want de", de, got)
	}
	if raw, err := os.ReadFile(l.path); err != nil || !bytes.Equal(raw, a.body) {
		t.Errorf("the corrupt copy was not replaced: %v", err)
	}
}

// Spec D2: any other redirect is refused before it is followed — a third
// host, plain http, another port, a user in the URL, a fourth hop — and the
// table in use stays.
func TestTheCountryArchiveRefusesEveryOtherRedirect(t *testing.T) {
	for name, tc := range map[string]struct {
		chain   map[string]string
		refused string // the URL that must never be asked
	}{
		"a third host": {map[string]string{countryArchiveURL: "https://objects.example.net/country-ip-blocks.tar.gz"},
			"https://objects.example.net/country-ip-blocks.tar.gz"},
		"plain http": {map[string]string{countryArchiveURL: "http://release-assets.githubusercontent.com/a"},
			"http://release-assets.githubusercontent.com/a"},
		"another port": {map[string]string{countryArchiveURL: "https://github.com:8443/a"},
			"https://github.com:8443/a"},
		"a user in the URL": {map[string]string{countryArchiveURL: "https://someone:pw!@github.com/a"},
			"https://someone:pw!@github.com/a"},
		"a fourth redirect": {map[string]string{countryArchiveURL: archiveHop2, archiveHop2: "https://github.com/a",
			"https://github.com/a": "https://github.com/b", "https://github.com/b": archiveHop3}, archiveHop3},
	} {
		t.Run(name, func(t *testing.T) {
			a := &archiveServer{next: githubsChain(), body: countryArchive(t, sampleCountryFiles...), etag: `"1"`}
			l := testLookup(t, a)
			l.refresh() // a good copy first
			a.next, a.etag, a.seen = tc.chain, `"2"`, nil

			l.refresh()
			if slices.Contains(a.requests(), tc.refused) {
				t.Errorf("%s was requested: %v", tc.refused, a.requests())
			}
			if fs := l.store.fetchState(countryLookupKey); fs.LastError != feedErrRedirected || fs.Status != FeedFailedCopy {
				t.Errorf("fetch state %+v, want redirected with the copy kept", fs)
			}
			// Its own sentence, not the feeds': a feed refuses every redirect,
			// the lookup only those away from GitHub (pre-flight M5).
			if got := l.status().ReasonKey(); got != "blocked_lookup_redirected" {
				t.Errorf("the refusal reads %q, want blocked_lookup_redirected", got)
			}
			if got := l.country(de); got != "de" {
				t.Errorf("the table in use was dropped: country = %q", got)
			}
		})
	}
	// The boundary: three redirects are allowed.
	a := &archiveServer{next: map[string]string{countryArchiveURL: archiveHop2, archiveHop2: "https://github.com/a",
		"https://github.com/a": archiveHop3}, body: countryArchive(t, sampleCountryFiles...), etag: `"3"`}
	l := testLookup(t, a)
	l.refresh()
	if !l.status().Loaded {
		t.Errorf("three redirects were refused: %+v", l.store.fetchState(countryLookupKey))
	}
}

// Spec §3: a truncated archive is refused and the old table stays — in
// memory and on disk.
func TestABrokenArchiveKeepsTheTableInUse(t *testing.T) {
	good := countryArchive(t, sampleCountryFiles...)
	a := &archiveServer{next: githubsChain(), body: good, etag: `"1"`}
	l := testLookup(t, a)
	l.refresh()
	a.body, a.etag = good[:len(good)/2], `"2"`

	l.refresh()
	if fs := l.store.fetchState(countryLookupKey); fs.LastError != feedErrTransfer || fs.Failures != 1 {
		t.Errorf("fetch state %+v, want one transfer failure", fs)
	}
	if got := l.status().ReasonKey(); got != "feed_error_transfer" {
		t.Errorf("a transfer failure reads %q, want the feeds' feed_error_transfer", got)
	}
	if got := l.country(de); got != "de" {
		t.Errorf("country = %q after a broken archive, want the old table's de", got)
	}
	if raw, _ := os.ReadFile(l.path); !bytes.Equal(raw, good) {
		t.Error("the broken archive replaced the copy on disk")
	}
}

func TestTheCountryLookupIsDue(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	l := testLookup(t, &archiveServer{})
	if !l.due(now) {
		t.Error("never fetched: not due")
	}
	l.store.setFetchState(countryLookupKey, feedFetchState{LastAttempt: now.Add(-10 * time.Minute), Failures: 2, LastError: feedErrTimeout})
	if l.due(now) {
		t.Error("no table, inside a two-failure backoff: due")
	}
	l.store.setFetchState(countryLookupKey, feedFetchState{LastAttempt: now.Add(-10 * time.Minute)})
	if !l.due(now) {
		t.Error("no table and no failure — the copy was lost: not due")
	}
	l.table.Store(&countryTable{})
	if l.due(now) {
		t.Error("a table checked ten minutes ago: due")
	}
	l.store.setFetchState(countryLookupKey, feedFetchState{LastAttempt: now.Add(-shared.CountryInterval)})
	if !l.due(now) {
		t.Error("a table checked a day ago: not due")
	}
}

// Spec D3, on by default (maintainer, 2026-10-14): off, there is no lookup
// anywhere — nothing that could make the request exists, at start or on any
// later pass. On, the runner refreshes the one /blocked asks. Counted against
// a real listener, so "no request" is what the network saw.
func TestTheCountryLookupExistsOnlyWhenSwitchedOn(t *testing.T) {
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	was := countryArchiveURL
	countryArchiveURL = srv.URL + "/country-ip-blocks.tar.gz"
	t.Cleanup(func() { countryArchiveURL = was })

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	for _, on := range []bool{false, true} {
		asked.Store(0)
		s.cfg.CountryLookup = on
		ws, err := NewServer(s.cfg)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		if got := ws.lookup != nil; got != on {
			t.Errorf("country_lookup = %v: lookup built = %v", on, got)
		}
		if ws.feeds.lookup != ws.lookup {
			t.Errorf("country_lookup = %v: the runner refreshes %p, the page asks %p", on, ws.feeds.lookup, ws.lookup)
		}
		done := make(chan struct{})
		close(done)
		ws.feeds.run(done) // the start: load, then one pass
		if on {
			// A day later, as the next tick after a failure would.
			ws.feeds.now = func() time.Time { return time.Now().Add(shared.CountryInterval) }
		}
		ws.feeds.pass()
		want := int32(0)
		if on {
			want = 2
		}
		if got := asked.Load(); got != want {
			t.Errorf("country_lookup = %v: %d requests at start and on refresh, want %d", on, got, want)
		}
		ws.Stop()
	}
}

// A state dir that cannot be written: the table fetched is used anyway, and
// is not due again until tomorrow — a failing write must not turn into a
// download every minute.
func TestAFetchThatCannotBeKeptIsNotFetchedAgainAtOnce(t *testing.T) {
	a := &archiveServer{next: githubsChain(), body: countryArchive(t, sampleCountryFiles...), etag: `"1"`}
	l := testLookup(t, a)
	l.path = filepath.Join(t.TempDir(), "gone", "countries.tar.gz") // its directory does not exist
	now := time.Now()
	l.now = func() time.Time { return now }
	l.refresh()
	if got := l.country(de); got != "de" {
		t.Fatalf("country = %q, want de from the table in memory", got)
	}
	if _, err := os.Stat(l.path); err == nil {
		t.Fatal("the archive was written where no directory exists")
	}
	if l.due(now) {
		t.Error("due again at once after a fetch that could not be kept")
	}
	if l.due(now.Add(2 * time.Hour)) {
		t.Error("due again two hours after a fetch that could not be kept")
	}
}

// Pre-flight M3: the validators describe the copy on disk, not the one in
// memory. A fetch that could not be written must leave the old ones, or after
// a restart the copy from the day before is asked for with the new copy's
// ETag, answered 304, and kept for good.
func TestValidatorsAreKeptOnlyWithTheirCopy(t *testing.T) {
	old := countryArchive(t, sampleCountryFiles[0]) // de only
	a := &archiveServer{next: githubsChain(), body: old, etag: `"old"`}
	l := testLookup(t, a)
	l.refresh()
	kept := l.path

	a.serve(countryArchive(t, sampleCountryFiles...), `"new"`) // ie too
	l.path = filepath.Join(t.TempDir(), "gone", "countries.tar.gz")
	l.refresh()
	if got := l.country(ie); got != "ie" {
		t.Fatalf("country(%s) = %q, want ie from the new table in memory", ie, got)
	}
	if fs := l.store.fetchState(countryLookupKey); fs.ETag != `"old"` {
		t.Errorf("ETag %q after a copy that was not written, want the one on disk", fs.ETag)
	}

	again := newCountryLookup(kept, l.store) // the restart, reading yesterday's copy
	again.http.Transport = a
	again.load()
	again.refresh()
	if got := again.country(ie); got != "ie" {
		t.Errorf("after a restart, country(%s) = %q: the old copy was confirmed with the new copy's ETag", ie, got)
	}
}

// The swap is one atomic pointer store: a lookup while a refresh swaps the
// table reads the old table or the new, never half of either. Run with -race.
func TestALookupDuringASwapIsSafe(t *testing.T) {
	bodies := [][]byte{countryArchive(t, sampleCountryFiles[0]), countryArchive(t, sampleCountryFiles...)}
	a := &archiveServer{next: githubsChain(), body: bodies[0], etag: `"0"`}
	l := testLookup(t, a)
	l.refresh()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if got := l.country(de); got != "de" {
				t.Errorf("country(%s) = %q mid-swap", de, got)
				return
			}
			_ = l.country(ie)
			_ = l.status()
		}
	}()
	for i := 1; i <= 20; i++ {
		a.serve(bodies[i%2], string(rune('0'+i%10))+"x")
		l.refresh()
	}
	close(stop)
	wg.Wait()
	if fs := l.store.fetchState(countryLookupKey); fs.Failures != 0 || fs.Status != FeedUpdated {
		t.Errorf("fetch state %+v after twenty swaps, want updated", fs)
	}
}
