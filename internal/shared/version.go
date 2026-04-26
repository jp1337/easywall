package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// CurrentVersion is updated at build time via -ldflags.
	CurrentVersion = "2.0.0-dev"

	githubReleasesURL = "https://api.github.com/repos/jpylypiw/easywall/releases/latest"
	cacheMaxAge       = 24 * time.Hour
)

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

// CheckLatestVersion fetches the latest release from GitHub, using a
// file-based cache at cachePath to avoid hitting the API on every request.
func CheckLatestVersion(cachePath string) (*VersionInfo, error) {
	if cached, ok := loadCache(cachePath); ok {
		return cached, nil
	}

	release, err := fetchLatestRelease()
	if err != nil {
		// Return current version info without update data on network error.
		return &VersionInfo{
			Current:         CurrentVersion,
			Latest:          "",
			UpdateAvailable: false,
			CheckedAt:       time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	info := &VersionInfo{
		Current:         CurrentVersion,
		Latest:          release.TagName,
		UpdateAvailable: release.TagName != CurrentVersion && release.TagName != "v"+CurrentVersion,
		ReleaseURL:      release.HTMLURL,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	_ = saveCache(cachePath, info)
	return info, nil
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
	return &release, nil
}

func loadCache(path string) (*VersionInfo, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var info VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, false
	}

	checked, err := time.Parse(time.RFC3339, info.CheckedAt)
	if err != nil || time.Since(checked) > cacheMaxAge {
		return nil, false
	}

	return &info, true
}

func saveCache(path string, info *VersionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
