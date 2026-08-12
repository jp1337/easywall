package shared

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No address in this repository belongs to a person.
//
// debian/control needs a Maintainer and debian/changelog needs a signer, and
// both were filled in with the maintainer's private Gmail address — where it sat
// in a public repository from the 2.0.0 rewrite onwards, readable by anyone and
// by every scraper that walks GitHub. Packaging metadata is exactly the place
// this happens: it is written once, at the start, and nobody reads it again.
//
// A project address or a GitHub noreply address does the same job. This test is
// what stops the private one coming back the next time someone adds a changelog
// entry by copying the one above it — which is how the second occurrence got
// there.
//
// Allowed: the reserved example domains (RFC 2606/6761), which is what test
// fixtures and documentation use, and GitHub's noreply addresses, which are
// public by construction.
func TestNoPersonalEmailAddressesAreTracked(t *testing.T) {
	root := repoRoot(t)

	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	address := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	allowed := func(addr string) bool {
		lower := strings.ToLower(addr)
		switch {
		case strings.HasSuffix(lower, "@users.noreply.github.com"),
			strings.HasSuffix(lower, "@noreply.github.com"):
			return true // public by construction
		}
		// Reserved for documentation and tests; can never reach a person.
		for _, suffix := range []string{".example", ".invalid", ".test", ".localhost",
			"example.com", "example.org", "example.net"} {
			if strings.HasSuffix(lower, suffix) {
				return true
			}
		}
		return false
	}

	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name == "" || skipPath(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- a path git listed in this repository
		if err != nil {
			continue // unreadable or a submodule; not this test's business
		}
		for _, found := range address.FindAllString(string(data), -1) {
			if !allowed(found) {
				t.Errorf("%s contains %q — a real address in a public repository. "+
					"Use the project address or a GitHub noreply one; see the note on this test.",
					name, found)
			}
		}
	}
}

// Nor does any funding link.
//
// .github/FUNDING.yml carried a `custom:` entry pointing at a payment handle
// built from the maintainer's real name, in a public repository, rendered by
// GitHub as a Sponsor button on every page of it. Same shape as the address in
// debian/control: written once when the project started, never reread.
//
// The handle itself is deliberately not quoted here — this test scans every
// tracked file, its own source included, and an example would make it fail on
// itself. That is the correct behaviour, and the reason this comment describes
// rather than demonstrates.
//
// GitHub Sponsors does the same job through an account that is already public.
// This test exists because the funding file is edited roughly never, so a
// reintroduction would sit there just as long as the first one did.
func TestNoPersonalPaymentLinksAreTracked(t *testing.T) {
	root := repoRoot(t)

	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	// Payment services whose URL carries a handle. The handle is what matters,
	// not the platform: ko-fi.com/jp1337 is the same public identity as the
	// GitHub account this repository lives under, and paypal.me/<real name> is
	// not. So: match the links, then allow the ones that are already public.
	link := regexp.MustCompile(`(?i)\b(?:paypal\.me|paypal\.com/paypalme|ko-fi\.com|buymeacoffee\.com|liberapay\.com|patreon\.com)/([A-Za-z0-9_.-]+)`)

	// The project's public identity, the same string as the GitHub account.
	const publicHandle = "jp1337"

	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		// The changelog records what happened, including that a funding platform
		// was once added; rewriting history is not this test's business.
		if name == "" || skipPath(name) || name == "CHANGELOG.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- a path git listed in this repository
		if err != nil {
			continue
		}
		for _, m := range link.FindAllStringSubmatch(string(data), -1) {
			if strings.EqualFold(m[1], publicHandle) {
				continue
			}
			t.Errorf("%s links %q — a payment handle that is not %q is a personal "+
				"identifier in a public repository. See the note on this test.",
				name, m[0], publicHandle)
		}
	}
}

// skipPath drops what cannot usefully be scanned: binaries, generated bundles
// and the vendored htmx, all of which are noise rather than authorship.
func skipPath(name string) bool {
	switch filepath.Ext(name) {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".woff", ".woff2", ".svg", ".pdf":
		return true
	}
	return strings.HasSuffix(name, "htmx.min.js") ||
		strings.HasSuffix(name, "style.css") ||
		strings.HasPrefix(name, "docs/assets/")
}

// repoRoot walks up to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's working directory")
		}
		dir = parent
	}
}
