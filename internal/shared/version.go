package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CurrentVersion is the release this binary belongs to. Every build overrides it
// with -ldflags "-X github.com/jp1337/easywall/internal/shared.CurrentVersion=…".
//
// It is a var, and it has to be: the linker's -X can only write to a variable,
// and it reports nothing when asked to write to a constant. This was a const,
// so every -ldflags in the repository — Makefile, .goreleaser.yaml, Dockerfile,
// debian/rules and two workflows — succeeded and changed nothing. Released
// binaries all carried the literal below whatever tag they were built from,
// which meant the dashboard advertised an update to a version that was already
// installed, the installation count reported every host on one release, and the
// stylesheet URL never changed across an upgrade even though it is versioned
// precisely so that it does. `easywall-core --version` prints this, so a build
// can be checked rather than assumed.
var CurrentVersion = "2.7.0"

const (
	// cacheMaxAge is how long a successful check is trusted.
	cacheMaxAge = 24 * time.Hour

	// failureCacheMaxAge is how long a failed check is remembered. Shorter than
	// a success, because the usual cause is a passing network problem — but a
	// failure has to be remembered at all. easywall is built for hosts with no
	// outbound route, and there the check cannot succeed, ever. Without this,
	// every single dashboard load paid the full connection timeout again.
	failureCacheMaxAge = time.Hour
)

// githubReleasesURL is a var so tests can override it with an httptest server URL.
var githubReleasesURL = "https://api.github.com/repos/jp1337/easywall/releases/latest"

// VersionInfo holds the result of a version check.
type VersionInfo struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
	CheckedAt       string `json:"checked_at"` // RFC3339
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// Checker answers "is there a newer release" for the dashboard without ever
// making the page wait for the network.
//
// The dashboard is the first thing an operator sees and the page they reload
// most; it must not depend on api.github.com being reachable, or being
// reachable quickly. Info returns immediately from cache and, when that cache
// has aged out, refreshes it in the background for the next page load.
type Checker struct {
	cachePath string
	enabled   bool

	mu        sync.Mutex
	cached    *VersionInfo
	inflight  bool
	refreshed chan struct{} // non-nil while a refresh is running; closed when it ends
}

// NewChecker returns a Checker writing its cache to cachePath. When enabled is
// false it never makes a request and reports only the running version.
func NewChecker(cachePath string, enabled bool) *Checker {
	return &Checker{cachePath: cachePath, enabled: enabled}
}

// Info returns the current version, and the latest one if a check has managed
// to establish it. It never blocks on the network.
func (c *Checker) Info() *VersionInfo {
	if !c.enabled {
		return &VersionInfo{Current: CurrentVersion}
	}

	c.mu.Lock()
	if c.cached == nil {
		if cached, ok := loadCache(c.cachePath); ok {
			c.cached = cached
		}
	}
	info := c.cached
	if !c.freshLocked() && !c.inflight {
		c.inflight = true
		c.refreshed = make(chan struct{})
		go c.refresh()
	}
	c.mu.Unlock()

	if info == nil {
		return &VersionInfo{Current: CurrentVersion}
	}

	out := *info
	// The running binary decides what "current" is, and whether the cached
	// Latest is still ahead of it. A cache written before an upgrade otherwise
	// keeps advertising an update the operator has already installed.
	out.Current = CurrentVersion
	out.UpdateAvailable = isNewer(out.Latest, CurrentVersion)
	return &out
}

// freshLocked reports whether the cached answer is still worth reusing.
// c.mu must be held.
func (c *Checker) freshLocked() bool {
	if c.cached == nil {
		return false
	}
	checked, err := time.Parse(time.RFC3339, c.cached.CheckedAt)
	if err != nil {
		return false
	}
	maxAge := cacheMaxAge
	if c.cached.Latest == "" {
		maxAge = failureCacheMaxAge
	}
	return time.Since(checked) < maxAge
}

func (c *Checker) refresh() {
	info := fetchVersionInfo()
	if err := saveCache(c.cachePath, info); err != nil {
		slog.Debug("could not write version cache", "path", c.cachePath, "error", err)
	}

	c.mu.Lock()
	c.cached = info
	c.inflight = false
	if c.refreshed != nil {
		close(c.refreshed)
		c.refreshed = nil
	}
	c.mu.Unlock()
}

// fetchVersionInfo performs the actual request. A failure still produces a
// VersionInfo with a timestamp: the check happened and did not work, which is
// what stops the next page load from trying again immediately.
func fetchVersionInfo() *VersionInfo {
	now := time.Now().UTC().Format(time.RFC3339)

	release, err := fetchLatestRelease()
	if err != nil {
		slog.Debug("update check failed", "error", err)
		return &VersionInfo{Current: CurrentVersion, CheckedAt: now}
	}

	return &VersionInfo{
		Current:         CurrentVersion,
		Latest:          release.TagName,
		UpdateAvailable: isNewer(release.TagName, CurrentVersion),
		ReleaseURL:      release.HTMLURL,
		CheckedAt:       now,
	}
}

func fetchLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("easywall/%s", CurrentVersion))
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}
	if release.Draft || release.Prerelease {
		// /releases/latest is documented to exclude both, so this only fires if
		// that ever changes. Announcing a draft as the newest release would
		// point operators at something that does not exist yet.
		return nil, fmt.Errorf("latest release is a draft or prerelease")
	}
	return &release, nil
}

// isNewer reports whether the release tag is a later version than current.
//
// This used to be `latest != current`, which is true in both directions. Anyone
// running ahead of the newest release — a build from main, a release candidate,
// the maintainer — was told on every dashboard load that an update was
// available, and pointed at an older version.
func isNewer(latest, current string) bool {
	if latest == "" {
		return false
	}
	return compareVersions(latest, current) > 0
}

// compareVersions orders two version strings, returning -1, 0 or 1. A leading
// "v" is ignored, so "v2.5.0" and "2.5.0" are the same version. A pre-release
// suffix sorts below the release it precedes, per semver: 2.5.0-rc1 < 2.5.0.
//
// Anything that does not parse as a dotted number compares equal, so a tag in
// an unexpected shape produces no claim rather than a wrong one.
func compareVersions(a, b string) int {
	an, apre, aok := splitVersion(a)
	bn, bpre, bok := splitVersion(b)
	if !aok || !bok {
		return 0
	}

	for i := 0; i < len(an) || i < len(bn); i++ {
		var x, y int
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}

	switch {
	case apre == bpre:
		return 0
	case apre == "": // a is the release, b a pre-release of it
		return 1
	case bpre == "":
		return -1
	case apre < bpre:
		return -1
	default:
		return 1
	}
}

// splitVersion turns "v2.5.0-rc1" into ([2 5 0], "rc1", true).
func splitVersion(v string) ([]int, string, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return nil, "", false
	}

	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		pre, v = v[i+1:], v[:i]
	}

	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	return nums, pre, true
}

func loadCache(path string) (*VersionInfo, bool) {
	// #nosec G304 -- the cache path is built from data_dir in the config, not from
	// anything the network supplied.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var info VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, false
	}

	if _, err := time.Parse(time.RFC3339, info.CheckedAt); err != nil {
		return nil, false
	}

	return &info, true
}

func saveCache(path string, info *VersionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
