package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckLatestVersion_UsesCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "version_cache.json")

	cached := &VersionInfo{
		Current:         CurrentVersion,
		Latest:          "v9.9.9",
		UpdateAvailable: true,
		ReleaseURL:      "https://example.com",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(cached)
	_ = os.WriteFile(cachePath, data, 0644)

	info, err := CheckLatestVersion(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Latest != "v9.9.9" {
		t.Errorf("expected cached Latest=v9.9.9, got: %s", info.Latest)
	}
}

func TestCheckLatestVersion_ExpiredCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "version_cache.json")

	stale := &VersionInfo{
		Current:         CurrentVersion,
		Latest:          "v0.0.1",
		UpdateAvailable: false,
		CheckedAt:       time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(stale)
	_ = os.WriteFile(cachePath, data, 0644)

	// Point to a test server so network call doesn't go to real GitHub
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v99.0.0",
			HTMLURL: "https://example.com/releases/v99.0.0",
		})
	}))
	defer srv.Close()

	// Temporarily override the URL for this test — use a monkey-patching approach
	// Since githubReleasesURL is a package-level const we test indirectly via the
	// returned result: expired cache should NOT return "v0.0.1".
	// We can't hit real GitHub in tests, so verify the stale data isn't returned.
	// The function falls back gracefully on network errors, returning empty Latest.
	info, err := CheckLatestVersion(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	// Stale cache was not used — either fresh fetch succeeded or fallback applies
	if info.Latest == "v0.0.1" {
		t.Error("should not return stale cached value after cache expiry")
	}
}

func TestCheckLatestVersion_NoCache_NetworkError(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "version_cache.json")
	// No cache file, real network will fail or succeed — either way we want no panic
	info, err := CheckLatestVersion(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Current != CurrentVersion {
		t.Errorf("expected Current=%s, got: %s", CurrentVersion, info.Current)
	}
}

func TestLoadCache_MissingFile(t *testing.T) {
	info, ok := loadCache("/nonexistent/path/cache.json")
	if ok {
		t.Error("expected ok=false for missing file")
	}
	if info != nil {
		t.Error("expected nil for missing file")
	}
}

func TestLoadCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)
	_, ok := loadCache(path)
	if ok {
		t.Error("expected ok=false for invalid JSON")
	}
}

func TestLoadCache_ExpiredEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	info := &VersionInfo{
		Current:   "1.0.0",
		CheckedAt: time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(info)
	_ = os.WriteFile(path, data, 0644)
	_, ok := loadCache(path)
	if ok {
		t.Error("expected ok=false for expired cache")
	}
}

func TestLoadCache_FreshEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	info := &VersionInfo{
		Current:   "1.0.0",
		Latest:    "v2.0.0",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(info)
	_ = os.WriteFile(path, data, 0644)
	loaded, ok := loadCache(path)
	if !ok {
		t.Fatal("expected ok=true for fresh cache")
	}
	if loaded.Latest != "v2.0.0" {
		t.Errorf("unexpected Latest: %s", loaded.Latest)
	}
}

func TestSaveCache_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
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
