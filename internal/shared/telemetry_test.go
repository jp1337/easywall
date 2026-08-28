package shared

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// recorder is a stand-in endpoint that remembers what reached it.
type recorder struct {
	mu     sync.Mutex
	hits   []url.Values
	agent  string
	method string
	srv    *httptest.Server
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	rec := &recorder{}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.hits = append(rec.hits, r.URL.Query())
		rec.agent = r.Header.Get("User-Agent")
		rec.method = r.Method
		rec.mu.Unlock()
		w.WriteHeader(http.StatusNoContent) // 204: what the live endpoint answered on 2026-08-28
	}))
	t.Cleanup(rec.srv.Close)

	previous := TelemetryEndpoint
	TelemetryEndpoint = rec.srv.URL + "/v1/count"
	t.Cleanup(func() { TelemetryEndpoint = previous })
	return rec
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hits)
}

func (r *recorder) last() url.Values {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.hits) == 0 {
		return nil
	}
	return r.hits[len(r.hits)-1]
}

func newTestReporter(t *testing.T, enabled func() bool) *Reporter {
	t.Helper()
	return NewReporter(filepath.Join(t.TempDir(), "telemetry.json"), enabled)
}

// Consent is the whole feature. Off means nothing leaves the machine — not a
// smaller request, not one without an identifier: none.
func TestReporter_SendsNothingWhenSwitchedOff(t *testing.T) {
	rec := newRecorder(t)
	r := newTestReporter(t, func() bool { return false })

	r.reportIfDue()

	if n := rec.count(); n != 0 {
		t.Errorf("%d requests were made with telemetry off", n)
	}
	if _, err := os.Stat(r.StatePath); err == nil {
		t.Error("an identifier was created for an installation that declined to be counted")
	}
}

// A nil Enabled is the zero value of a struct someone built by hand. It has to
// mean no, because every other answer would be an assumption.
func TestReporter_SendsNothingWithoutAConsentFunction(t *testing.T) {
	rec := newRecorder(t)
	r := &Reporter{StatePath: filepath.Join(t.TempDir(), "telemetry.json"), now: time.Now}

	r.reportIfDue()

	if n := rec.count(); n != 0 {
		t.Errorf("%d requests were made without a consent function", n)
	}
}

// What is documented is what is sent: an identifier and a version, and nothing
// else at all.
//
// This is also, verbatim, the request that reached
// https://telemetry.wdkro.de/v1/count by hand on 2026-08-28 and was answered
// 204 (recorder above returns the same code for that reason). A test against
// a local httptest server cannot prove the endpoint is up; what it proves is
// that the request has not drifted since — a third parameter, a renamed one,
// or a POST where a GET was accepted would all be silent here without it.
func TestReporter_SendsOnlyTheIdentifierAndTheVersion(t *testing.T) {
	rec := newRecorder(t)
	r := newTestReporter(t, func() bool { return true })

	r.reportIfDue()

	if n := rec.count(); n != 1 {
		t.Fatalf("expected exactly one report, got %d", n)
	}
	if rec.method != http.MethodGet {
		t.Errorf("method = %s, want GET", rec.method)
	}
	q := rec.last()
	if len(q) != 2 {
		t.Errorf("the request carried %d parameters, not 2: %v", len(q), q)
	}
	if q.Get("v") != CurrentVersion {
		t.Errorf("version %q, want %q", q.Get("v"), CurrentVersion)
	}
	id := q.Get("id")
	if len(id) != 32 {
		t.Errorf("identifier %q is not 16 bytes of hex", id)
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("identifier %q is not hex", id)
		}
	}
	if rec.agent != "easywall/"+CurrentVersion {
		t.Errorf("User-Agent %q", rec.agent)
	}
}

// Two machines must not be able to end up as one, and one machine must not be
// able to end up as many. The identifier is random and it is kept.
func TestReporter_IdentifierIsStableAndUnique(t *testing.T) {
	rec := newRecorder(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.json")
	first := NewReporter(path, func() bool { return true })
	first.reportIfDue()
	id := rec.last().Get("id")

	// A restart: same file, new Reporter, no in-memory state.
	second := NewReporter(path, func() bool { return true })
	second.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	second.reportIfDue()
	if again := rec.last().Get("id"); again != id {
		t.Errorf("the identifier changed across a restart: %q became %q", id, again)
	}

	other := newTestReporter(t, func() bool { return true })
	other.reportIfDue()
	if elsewhere := rec.last().Get("id"); elsewhere == id {
		t.Error("two installations produced the same identifier")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("state file is %v, want 0600", perm)
	}
}

// Once a day, not once an hour. The reporter wakes up far more often than it
// sends, and the interval is what keeps that from being a stream.
func TestReporter_SendsAtMostOncePerDay(t *testing.T) {
	rec := newRecorder(t)
	base := time.Now()
	r := newTestReporter(t, func() bool { return true })
	r.now = func() time.Time { return base }

	for i := 0; i < 5; i++ {
		r.reportIfDue()
	}
	if n := rec.count(); n != 1 {
		t.Fatalf("expected 1 report within the interval, got %d", n)
	}

	r.now = func() time.Time { return base.Add(23 * time.Hour) }
	r.reportIfDue()
	if n := rec.count(); n != 1 {
		t.Errorf("reported again after 23 hours (%d total)", n)
	}

	r.now = func() time.Time { return base.Add(25 * time.Hour) }
	r.reportIfDue()
	if n := rec.count(); n != 2 {
		t.Errorf("did not report after the interval elapsed (%d total)", n)
	}
}

// A host that is offline for a week should report on the day it comes back,
// not a day after that. A failed attempt must not count as a report.
func TestReporter_AFailedReportIsNotRecorded(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	previous := TelemetryEndpoint
	TelemetryEndpoint = srv.URL + "/v1/count"
	defer func() { TelemetryEndpoint = previous }()

	r := newTestReporter(t, func() bool { return true })
	r.reportIfDue() // fails
	r.reportIfDue() // must try again immediately, not in 24 hours

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("expected a retry after a failure, got %d attempts", hits)
	}
}

// The endpoint is named in the documentation. Following a redirect would let
// whoever controls its DNS send the reports somewhere else entirely.
func TestReporter_RefusesToFollowARedirect(t *testing.T) {
	var reached bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer elsewhere.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer srv.Close()

	previous := TelemetryEndpoint
	TelemetryEndpoint = srv.URL + "/v1/count"
	defer func() { TelemetryEndpoint = previous }()

	r := newTestReporter(t, func() bool { return true })
	r.reportIfDue()

	if reached {
		t.Error("the report followed a redirect to a different host")
	}
}

// An endpoint that accepts the connection and never answers must not hold a
// goroutine open for the life of the process.
func TestReporter_GivesUpOnAnEndpointThatNeverAnswers(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block
	}))
	defer func() { close(block); srv.Close() }()

	previous := TelemetryEndpoint
	TelemetryEndpoint = srv.URL + "/v1/count"
	defer func() { TelemetryEndpoint = previous }()

	previousTimeout := telemetryTimeout
	telemetryTimeout = 200 * time.Millisecond
	defer func() { telemetryTimeout = previousTimeout }()

	r := newTestReporter(t, func() bool { return true })
	done := make(chan error, 1)
	go func() { done <- r.send("0123456789abcdef0123456789abcdef") }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("an endpoint that never answered was treated as a successful report")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the report did not give up — a stuck endpoint holds the goroutine forever")
	}

	if previousTimeout > 30*time.Second {
		t.Errorf("telemetry timeout is %v — too long for a background task", previousTimeout)
	}
}

// Consent is read on every attempt, so withdrawing it works without a restart.
func TestReporter_StopsWhenConsentIsWithdrawn(t *testing.T) {
	rec := newRecorder(t)
	consent := true
	base := time.Now()
	r := newTestReporter(t, func() bool { return consent })
	r.now = func() time.Time { return base }

	r.reportIfDue()
	if rec.count() != 1 {
		t.Fatal("the first report did not happen")
	}

	consent = false
	r.now = func() time.Time { return base.Add(48 * time.Hour) }
	r.reportIfDue()

	if n := rec.count(); n != 1 {
		t.Errorf("reported after consent was withdrawn (%d total)", n)
	}
}

// Run must return when it is told to, and must not send on the way out.
func TestReporter_RunStopsWhenAsked(t *testing.T) {
	rec := newRecorder(t)
	r := newTestReporter(t, func() bool { return true })
	r.spread = func() time.Duration { return time.Hour } // never reached

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { r.Run(stop); close(done) }()

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return when the stop channel closed")
	}
	if n := rec.count(); n != 0 {
		t.Errorf("a report was sent while shutting down (%d)", n)
	}
}

// The live endpoint answered 204 on the day it was verified, not the 200 the
// plan guessed at. send already treats anything under 300 as success
// (resp.StatusCode >= 300 is the only check), so 204 was already accepted —
// this pins that boundary directly instead of one status code that happened
// to be true. If the boundary ever narrowed to "only 200", the 204 case below
// would start failing even though the live endpoint never changed.
func TestReporter_AcceptsWhatTheLiveEndpointSentAndRejectsFailureStatuses(t *testing.T) {
	respond := func(t *testing.T, status int) error {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		defer srv.Close()

		previous := TelemetryEndpoint
		TelemetryEndpoint = srv.URL + "/v1/count"
		defer func() { TelemetryEndpoint = previous }()

		return newTestReporter(t, func() bool { return true }).send("0123456789abcdef0123456789abcdef")
	}

	if err := respond(t, http.StatusNoContent); err != nil {
		t.Errorf("204 (what the live endpoint sent on 2026-08-28) was treated as a failure: %v", err)
	}
	if err := respond(t, http.StatusMultipleChoices); err == nil {
		t.Error("300 was treated as success — the >= 300 boundary has drifted")
	}
}

// A corrupt state file costs one double-count, not a crash and not a stuck
// reporter.
func TestReporter_RecoversFromAnUnreadableStateFile(t *testing.T) {
	rec := newRecorder(t)
	path := filepath.Join(t.TempDir(), "telemetry.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	r := NewReporter(path, func() bool { return true })
	r.reportIfDue()

	if rec.count() != 1 {
		t.Fatal("no report was made after a corrupt state file")
	}
	if id := rec.last().Get("id"); len(id) != 32 {
		t.Errorf("no usable identifier was created, got %q", id)
	}
}
