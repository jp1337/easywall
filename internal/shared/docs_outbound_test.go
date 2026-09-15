package shared

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Two pages count easywall's outbound requests, and nothing bound them together.
//
// Both say "<number>, and this is the whole list." above a table of
// destinations — docs/_docs/security.md under *Every request that goes out*,
// docs/_docs/configuration.md under *Every request that leaves the host*.
// TestEveryConfigKeyIsDocumented reads the configuration page and no other, so
// a number written out in a sentence was held up by nothing but whoever
// remembered both pages.
//
// Three sources have to agree here, and the third is what makes this more than
// a spell-check: the code. Every file under internal/ and cmd/ that reaches out
// is named below with the row it documents, so a new request cannot be added
// without either extending both pages or saying here why it is not on them.
//
// It found two defects on the branch that added it. 2.20's notification was on
// the configuration page and not on the security page, which still said **two**
// above a two-row table. And ACME, shipped in 2.18, was on neither: autocert
// fetches a certificate from the directory and renews it on its own schedule,
// while security.md's ACME section describes only the inbound half — the
// authority connecting to port 80 to read a token back. A request easywall
// makes through a library is still a request easywall makes, which is why the
// marker list below is not only http.NewRequest.
//
// The number and the table are checked separately, deliberately. A count that
// matches above a table missing a row is the exact shape of what shipped, and a
// guard that read only the sentence would have passed on it — measured: it did,
// on a mutation that deleted the ACME row and left the word "Four" standing.
//
// The marker list and the walk are both wider than today's code needs, for the
// same reason. The first version matched http.NewRequest alone and walked
// internal/ alone — true of every existing call site by coincidence, not by
// rule. A review added http.Get to internal/core/daemon.go, http.Post beside
// it, and http.NewRequest to cmd/easywall-web/main.go, and all three were
// silent. http.Get is the most idiomatic way in Go to make exactly the request
// this guard exists to notice. Nothing under cmd/ reaches out today, so walking
// it costs nothing and closes the other half.
func TestBothPagesCountTheSameOutboundRequests(t *testing.T) {
	root := repoRootDir(t)

	// What makes a file reach out. The http.* forms are easywall building the
	// request itself; the autocert import is easywall handing that job to
	// x/crypto, which is the same thing from the host's point of view and is
	// exactly what both pages missed for two releases.
	//
	// This list is a HEURISTIC and not a proof, and saying so is the point: a
	// guard that claimed completeness it cannot have would be the defect this
	// release keeps finding. A call on an *http.Client value — c.Get(, c.Do( —
	// matches nothing here unless the request was built with http.NewRequest,
	// which today's three call sites all do. It also matches a mention inside a
	// comment, and _test.go is skipped entirely. Loud and cheap over quiet and
	// leaky: a false positive costs one line in the map below, a false negative
	// is a request nobody documented.
	markers := []string{
		"http.NewRequest(", "http.Get(", "http.Post(", "http.PostForm(",
		"http.Head(", "acme/autocert",
	}

	// Both binaries' trees, not only the library. A request added to a main
	// package is a request all the same, and the first version of this test
	// could not see one.
	trees := []string{"internal", "cmd"}

	type row struct {
		name string // how a failure message names it
		// token is a string that must appear inside BOTH tables. The two pages
		// word their rows differently — one says "Installation count" where the
		// other says "Counting installations" — so this is the destination or
		// the key, never the label.
		token string
	}
	certificate := row{"A certificate — the ACME directory, Let's Encrypt by default", "acme"}

	// source file → the row in both tables that documents it. Two files can
	// carry one row: the count the pages must match is the number of distinct
	// rows, not of files.
	outbound := map[string]row{
		"internal/shared/version.go":   {"Update check — api.github.com", "api.github.com"},
		"internal/shared/telemetry.go": {"Counting installations — telemetry.wdkro.de", "telemetry.wdkro.de"},
		"internal/web/notify.go":       {"Notifications — an address the operator chooses", "Notifications"},
		"internal/web/acme.go":         certificate,
		"internal/web/tlscert.go":      certificate,
	}

	var found []string
	for _, tree := range trees {
		err := filepath.WalkDir(filepath.Join(root, tree), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, marker := range markers {
				if strings.Contains(string(raw), marker) {
					rel, _ := filepath.Rel(root, path)
					found = append(found, filepath.ToSlash(rel))
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s/: %v", tree, err)
		}
	}
	sort.Strings(found)

	rows := map[string]row{}
	for _, f := range found {
		r, ok := outbound[f]
		if !ok {
			// No exemption list, deliberately: every entry in the map below
			// demands a matching token in both published tables, so there is
			// nowhere to record "this one does not count". An offer of one in
			// this message would be a mechanism that does not exist.
			t.Errorf("%s reaches out and is not one of the rows in security.md and "+
				"configuration.md — add the row to both pages and name it here. If it "+
				"is a false positive (the marker appears in a comment, say), the entry "+
				"still needs a row: this guard has no exemption list", f)
			continue
		}
		rows[r.name] = r
	}

	// A named file that stopped matching is a dead entry pointing at nothing,
	// and it would hold the count up on its own. Same reasoning as the exemption
	// check in TestTheOldDemoHostIsNotPublished.
	for f, r := range outbound {
		raw, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Errorf("%s is named here as a source of %q and does not exist: %v", f, r.name, err)
			continue
		}
		matched := false
		for _, marker := range markers {
			matched = matched || strings.Contains(string(raw), marker)
		}
		if !matched {
			t.Errorf("%s is named here as a source of %q and no longer matches any of %v — "+
				"delete the entry rather than leaving it holding the count up", f, r.name, markers)
		}
	}

	// The count both pages must say, spelled as they spell it.
	words := map[int]string{1: "One", 2: "Two", 3: "Three", 4: "Four", 5: "Five", 6: "Six"}
	want := words[len(rows)]
	if want == "" {
		t.Fatalf("no word for %d outbound requests — extend the map above", len(rows))
	}

	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, strconv.Quote(name))
	}
	sort.Strings(names)

	claim := regexp.MustCompile(`(?m)^([A-Z][a-z]+), and this is the whole list\.$`)
	for _, page := range []string{"docs/_docs/security.md", "docs/_docs/configuration.md"} {
		raw, err := os.ReadFile(filepath.Join(root, page))
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		body := string(raw)
		m := claim.FindAllStringSubmatchIndex(body, -1)
		if len(m) != 1 {
			t.Errorf("%s has %d sentences reading \"<number>, and this is the whole list.\", want 1 — "+
				"that sentence is what this test compares against the code", page, len(m))
			continue
		}
		if got := body[m[0][2]:m[0][3]]; got != want {
			t.Errorf("%s says %q outbound requests; the tree makes %d (%s)",
				page, got, len(rows), strings.Join(names, ", "))
		}
		table := tableAfter(body[m[0][1]:])
		if table == "" {
			t.Errorf("%s has no table under that sentence — this test reads the rows there", page)
			continue
		}
		for _, r := range rows {
			if !strings.Contains(table, r.token) {
				t.Errorf("%s says %q above a table that never mentions %q — the row for %s is "+
					"missing, which is the shape the number alone cannot see", page, want, r.token, r.name)
			}
		}
	}
}

// tableAfter returns the first run of consecutive "|" lines in s, which is the
// table immediately under the claim sentence. Empty if there is none before the
// next paragraph of prose.
func tableAfter(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "|") {
			out = append(out, line)
			continue
		}
		if len(out) > 0 {
			break
		}
		if line != "" {
			return "" // prose before any table: not the shape this reads
		}
	}
	return strings.Join(out, "\n")
}
