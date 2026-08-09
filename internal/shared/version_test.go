package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// releaseServer stands in for the GitHub API and counts how often it was asked.
//
// Two of the tests this file replaced called the real api.github.com: the suite
// depended on GitHub being reachable, and cost five seconds per test when it
// was not.
func releaseServer(t *testing.T, rel githubRelease) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	old := githubReleasesURL
	githubReleasesURL = srv.URL
	t.Cleanup(func() { githubReleasesURL = old })
	return srv, &hits
}

// unreachableAPI points the checker at a port nothing is listening on.
func unreachableAPI(t *testing.T) {
	t.Helper()
	old := githubReleasesURL
	githubReleasesURL = "http://127.0.0.1:1"
	t.Cleanup(func() { githubReleasesURL = old })
}

// waitForRefresh blocks until the checker's background refresh has finished.
func waitForRefresh(t *testing.T, c *Checker) {
	t.Helper()
	c.mu.Lock()
	ch := c.refreshed
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("background refresh did not finish")
	}
}

func writeCache(t *testing.T, path string, info *VersionInfo) {
	t.Helper()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func cachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "version_cache.json")
}

// --- Checker: the dashboard must never wait for the network ---------------

func TestChecker_InfoDoesNotWaitForTheNetwork(t *testing.T) {
	// A server that answers only after longer than any page render should wait.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v99.0.0"})
	}))
	defer slow.Close()
	old := githubReleasesURL
	githubReleasesURL = slow.URL
	defer func() { githubReleasesURL = old }()

	c := NewChecker(cachePath(t), true)

	start := time.Now()
	info := c.Info()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Info blocked for %v; the dashboard must render without waiting", elapsed)
	}
	if info.Current != CurrentVersion {
		t.Errorf("expected Current=%s, got %s", CurrentVersion, info.Current)
	}
	if info.Latest != "" {
		t.Errorf("nothing is known yet, so Latest must be empty, got %q", info.Latest)
	}
	waitForRefresh(t, c)
}

func TestChecker_SecondLoadSeesTheRefreshedAnswer(t *testing.T) {
	_, hits := releaseServer(t, githubRelease{
		TagName: "v99.0.0",
		HTMLURL: "https://example.com/releases/v99.0.0",
	})

	c := NewChecker(cachePath(t), true)
	_ = c.Info()
	waitForRefresh(t, c)

	info := c.Info()
	if info.Latest != "v99.0.0" {
		t.Errorf("expected Latest=v99.0.0, got %q", info.Latest)
	}
	if !info.UpdateAvailable {
		t.Error("v99.0.0 is newer than the running version")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected exactly one request, got %d", got)
	}
}

func TestChecker_AFailedCheckIsNotRetriedOnEveryLoad(t *testing.T) {
	// The case easywall is built for: a host with no route out. Before the
	// failure was cached, every dashboard render paid the connection timeout.
	unreachableAPI(t)

	path := cachePath(t)
	c := NewChecker(path, true)
	_ = c.Info()
	waitForRefresh(t, c)

	cached, ok := loadCache(path)
	if !ok {
		t.Fatal("a failed check must still be recorded, or it repeats forever")
	}
	if cached.Latest != "" {
		t.Errorf("a failed check knows no latest version, got %q", cached.Latest)
	}

	// A fresh process reading that cache must not start another request.
	c2 := NewChecker(path, true)
	_ = c2.Info()
	c2.mu.Lock()
	inflight := c2.inflight
	c2.mu.Unlock()
	if inflight {
		t.Error("a recent failure must suppress the next attempt")
	}
}

func TestChecker_AFailedCheckIsRetriedOnceItAges(t *testing.T) {
	path := cachePath(t)
	writeCache(t, path, &VersionInfo{
		Current:   CurrentVersion,
		CheckedAt: time.Now().Add(-2 * failureCacheMaxAge).UTC().Format(time.RFC3339),
	})
	_, hits := releaseServer(t, githubRelease{TagName: "v99.0.0"})

	c := NewChecker(path, true)
	_ = c.Info()
	waitForRefresh(t, c)

	if hits.Load() != 1 {
		t.Errorf("an hour-old failure must be retried, got %d requests", hits.Load())
	}
	if got := c.Info().Latest; got != "v99.0.0" {
		t.Errorf("expected the retry to land, got Latest=%q", got)
	}
}

func TestChecker_SuccessIsTrustedForADay(t *testing.T) {
	path := cachePath(t)
	writeCache(t, path, &VersionInfo{
		Current:   CurrentVersion,
		Latest:    "v9.9.9",
		CheckedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	})
	_, hits := releaseServer(t, githubRelease{TagName: "v1.0.0"})

	c := NewChecker(path, true)
	info := c.Info()
	if info.Latest != "v9.9.9" {
		t.Errorf("expected the cached answer, got %q", info.Latest)
	}
	if hits.Load() != 0 {
		t.Errorf("a two-hour-old success must not be refetched, got %d requests", hits.Load())
	}
}

func TestChecker_StaleSuccessIsRefetched(t *testing.T) {
	path := cachePath(t)
	writeCache(t, path, &VersionInfo{
		Current:   CurrentVersion,
		Latest:    "v0.0.1",
		CheckedAt: time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339),
	})
	_, hits := releaseServer(t, githubRelease{TagName: "v99.0.0"})

	c := NewChecker(path, true)
	_ = c.Info()
	waitForRefresh(t, c)

	if hits.Load() != 1 {
		t.Errorf("a day-old answer must be refreshed, got %d requests", hits.Load())
	}
	if got := c.Info().Latest; got != "v99.0.0" {
		t.Errorf("expected the refreshed value, got %q", got)
	}
}

func TestChecker_DisabledNeverAsks(t *testing.T) {
	_, hits := releaseServer(t, githubRelease{TagName: "v99.0.0"})

	c := NewChecker(cachePath(t), false)
	info := c.Info()
	time.Sleep(50 * time.Millisecond) // a background request would have started by now

	if hits.Load() != 0 {
		t.Errorf("update_check = false must make no request at all, got %d", hits.Load())
	}
	if info.Current != CurrentVersion {
		t.Errorf("the running version is still reported, got %q", info.Current)
	}
	if info.Latest != "" || info.UpdateAvailable {
		t.Errorf("nothing was checked, so nothing may be claimed: %+v", info)
	}
}

func TestChecker_CachedUpdateClaimIsRecheckedAgainstTheRunningBinary(t *testing.T) {
	// The cache outlives the binary that wrote it. After an upgrade, a cache
	// saying "v2.4.2 is available" must not keep nagging.
	path := cachePath(t)
	writeCache(t, path, &VersionInfo{
		Current:         "0.1.0",
		Latest:          CurrentVersion,
		UpdateAvailable: true,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	})

	c := NewChecker(path, true)
	info := c.Info()
	if info.UpdateAvailable {
		t.Error("the cached release equals the running version; no update is available")
	}
	if info.Current != CurrentVersion {
		t.Errorf("expected Current=%s, got %s", CurrentVersion, info.Current)
	}
}

func TestChecker_ConcurrentInfoStartsOneRefresh(t *testing.T) {
	_, hits := releaseServer(t, githubRelease{TagName: "v99.0.0"})

	c := NewChecker(cachePath(t), true)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() { _ = c.Info(); done <- struct{}{} }()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	waitForRefresh(t, c)

	if got := hits.Load(); got != 1 {
		t.Errorf("eight simultaneous dashboard loads must produce one request, got %d", got)
	}
}

// --- Version comparison ----------------------------------------------------

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.5.0", "2.4.2", 1},
		{"2.4.2", "2.5.0", -1},
		{"2.4.2", "2.4.2", 0},
		{"v2.4.2", "2.4.2", 0},
		{"2.4.2", "v2.4.2", 0},
		{"2.10.0", "2.9.0", 1}, // a string compare gets this one backwards
		{"3.0.0", "2.99.99", 1},
		{"2.5", "2.5.0", 0},
		{"2.5.1", "2.5", 1},
		{"2.5.0-rc1", "2.5.0", -1},
		{"2.5.0", "2.5.0-rc1", 1},
		{"2.5.0-rc2", "2.5.0-rc1", 1},
		{"nightly", "2.4.2", 0}, // unparseable: make no claim
		{"", "2.4.2", 0},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestIsNewer_DoesNotOfferADowngrade(t *testing.T) {
	// Running ahead of the newest release — a build from main, or the
	// maintainer. The old string inequality announced an "update" pointing
	// backwards on every dashboard load.
	if isNewer("2.4.2", "2.5.0") {
		t.Error("an older release is not an available update")
	}
	if isNewer("v2.5.0", "2.5.0") {
		t.Error("the same version tagged with a v is not an update")
	}
	if isNewer("", "2.5.0") {
		t.Error("an unknown latest version is not an update")
	}
	if !isNewer("v2.6.0", "2.5.0") {
		t.Error("a later release is an available update")
	}
}

// --- Fetch and cache primitives -------------------------------------------

func TestFetchLatestRelease_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	if _, err := fetchLatestRelease(); err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestFetchLatestRelease_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	if _, err := fetchLatestRelease(); err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestFetchLatestRelease_Success(t *testing.T) {
	releaseServer(t, githubRelease{TagName: "v3.0.0", HTMLURL: "https://example.com/v3.0.0"})

	rel, err := fetchLatestRelease()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.TagName != "v3.0.0" {
		t.Errorf("expected TagName=v3.0.0, got %s", rel.TagName)
	}
}

func TestFetchLatestRelease_RejectsDraftsAndPrereleases(t *testing.T) {
	for _, rel := range []githubRelease{
		{TagName: "v9.0.0", Draft: true},
		{TagName: "v9.0.0", Prerelease: true},
	} {
		releaseServer(t, rel)
		if _, err := fetchLatestRelease(); err == nil {
			t.Errorf("a draft or prerelease must not be announced as the newest version: %+v", rel)
		}
	}
}

func TestFetchLatestRelease_NewRequestError(t *testing.T) {
	old := githubReleasesURL
	// Null byte in URL causes http.NewRequest to fail.
	githubReleasesURL = "http://invalid\x00host"
	defer func() { githubReleasesURL = old }()

	if _, err := fetchLatestRelease(); err == nil {
		t.Error("expected error for URL with null byte")
	}
}

func TestFetchLatestRelease_ReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement Hijacker")
			return
		}
		conn, bufw, err := hj.Hijack()
		if err != nil {
			return
		}
		// Content-Length claims 1000 bytes; send 10 and hang up.
		_, _ = bufw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n{\"partial\"")
		_ = bufw.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	if _, err := fetchLatestRelease(); err == nil {
		t.Error("expected error when connection closed mid-body")
	}
}

func TestLoadCache_MissingFile(t *testing.T) {
	info, ok := loadCache("/nonexistent/path/cache.json")
	if ok || info != nil {
		t.Error("expected no cache for a missing file")
	}
}

func TestLoadCache_InvalidJSON(t *testing.T) {
	path := cachePath(t)
	_ = os.WriteFile(path, []byte("not json"), 0600)
	if _, ok := loadCache(path); ok {
		t.Error("expected ok=false for invalid JSON")
	}
}

func TestLoadCache_InvalidTimestamp(t *testing.T) {
	path := cachePath(t)
	writeCache(t, path, &VersionInfo{Current: "1.0.0", CheckedAt: "not-a-timestamp"})
	if _, ok := loadCache(path); ok {
		t.Error("expected ok=false for invalid timestamp")
	}
}

func TestSaveCache_Roundtrip(t *testing.T) {
	path := cachePath(t)
	info := &VersionInfo{
		Current:         "2.0.0",
		Latest:          "v2.1.0",
		UpdateAvailable: true,
		ReleaseURL:      "https://example.com",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveCache(path, info); err != nil {
		t.Fatal(err)
	}
	loaded, ok := loadCache(path)
	if !ok {
		t.Fatal("expected ok=true after saveCache")
	}
	if loaded.Latest != info.Latest {
		t.Errorf("roundtrip mismatch: got %s", loaded.Latest)
	}
}

func TestSaveCache_InvalidPath(t *testing.T) {
	err := saveCache("/nonexistent/directory/cache.json", &VersionInfo{Current: CurrentVersion})
	if err == nil {
		t.Error("expected error writing to nonexistent directory")
	}
}

func TestChecker_UnwritableCacheStillServesTheAnswer(t *testing.T) {
	releaseServer(t, githubRelease{TagName: "v4.0.0", HTMLURL: "https://example.com"})

	c := NewChecker("/nonexistent/dir/cache.json", true)
	_ = c.Info()
	waitForRefresh(t, c)

	if got := c.Info().Latest; got != "v4.0.0" {
		t.Errorf("a cache that cannot be written must not lose the result, got %q", got)
	}
}
