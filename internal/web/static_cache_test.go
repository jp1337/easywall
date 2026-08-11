package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// staticRefPattern finds every reference to a file under /static/ in a template,
// with whatever query string follows it.
var staticRefPattern = regexp.MustCompile(`/static/([A-Za-z0-9._/-]+)(\?[^"'\s>]*)?`)

// Cache-busting has to cover every file a release can change, not just the one
// somebody remembered.
//
// The stylesheet carried ?v= and the scripts did not. Measured in Chrome against
// two builds, with the assets given the mtime dpkg preserves from the build:
// after upgrading 2.5.0 to 2.6.0 the browser fetched the new style.css — its URL
// had changed — and served app.js out of its own cache. The upgraded interface
// therefore ran new markup and new CSS against the previous release's
// JavaScript, which is a harder failure to recognise than a stale stylesheet:
// nothing looks wrong, the editors just stop saving what you typed.
func TestVersionedStaticAssetsCarryTheReleaseInTheirURL(t *testing.T) {
	dir := repoTemplates(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}

	// Files whose content a release can change. Fonts and icons are excluded on
	// purpose: they are replaced only when the design changes, and the no-cache
	// header below makes even that safe.
	versioned := func(name string) bool {
		return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css")
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		raw, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range staticRefPattern.FindAllStringSubmatch(string(raw), -1) {
			file, query := m[1], m[2]
			if !versioned(file) {
				continue
			}
			checked++
			if !strings.Contains(query, "v={{.Asset}}") {
				t.Errorf("%s references /static/%s as %q; it needs ?v={{.Asset}} or an "+
					"upgrade leaves the browser on the previous release's copy",
					e.Name(), file, file+query)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no versioned static references found — the pattern stopped matching")
	}
	t.Logf("%d versioned static references checked", checked)
}

// The query string only helps if the response is cacheable in the first place,
// and only if the files without one are not. http.FileServer sends neither
// header, which leaves both to a browser heuristic derived from the file's
// mtime — and a packaged file's mtime is the build date.
//
// Rooted at the repository's real static directory on purpose: since Go 1.23
// net/http strips Cache-Control on the error path, so a 404 proves nothing here.
// Every URL below is a file that exists.
func TestStaticFilesSayHowLongTheyMayBeKept(t *testing.T) {
	h := staticCacheHeaders(http.StripPrefix("/static/",
		http.FileServer(http.Dir(repoDir(t, "web", "static")))))

	for _, tc := range []struct {
		url, want string
	}{
		{"/static/app.js?v=2.5.0", "public, max-age=31536000, immutable"},
		{"/static/style.css?v=2.5.0", "public, max-age=31536000, immutable"},
		{"/static/app.js", "no-cache"},
		{"/static/fonts/inter-var.woff2", "no-cache"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d — the assertion needs a file that exists", tc.url, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.url, got, tc.want)
		}
	}
}
