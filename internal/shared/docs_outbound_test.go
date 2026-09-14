package shared

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Two pages count easywall's outbound requests, and nothing bound them together.
//
// Both say "<number>, and this is the whole list." above a table of
// destinations — docs/_docs/security.md under *Every request that goes out*,
// docs/_docs/configuration.md under *Every request that leaves the host*.
// 2.20 added a third request, and the configuration page was updated by the
// task that added the keys. TestEveryConfigKeyIsDocumented only reads that
// page, so security.md went on saying **two** with two rows while the feature
// shipped a third. A number in a sentence is not something a key-coverage test
// can see.
//
// Three sources have to agree here, and the third is the one that makes this
// more than a spell-check: the code. Every file under internal/ that builds an
// outbound HTTP request is named below with the row it documents, so a fourth
// request cannot be added without either extending both pages or saying here
// why it is not on them.
func TestBothPagesCountTheSameOutboundRequests(t *testing.T) {
	root := repoRootDir(t)

	// source file → the row in both tables that documents it.
	outbound := map[string]string{
		"internal/shared/version.go":   "Update check — api.github.com",
		"internal/shared/telemetry.go": "Counting installations — telemetry.wdkro.de",
		"internal/web/notify.go":       "Notifications — an address the operator chooses",
	}

	// ACME is not on this list and not in those tables: autocert talks to the
	// certificate authority the operator configured under [tls], from inside
	// x/crypto rather than from any file here. security.md gives it its own
	// section. This walk only sees requests easywall builds itself, which is
	// exactly the set those two tables describe.
	var found []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
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
		if strings.Contains(string(raw), "http.NewRequest(") {
			rel, _ := filepath.Rel(root, path)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	sort.Strings(found)

	for _, f := range found {
		if outbound[f] == "" {
			t.Errorf("%s builds an outbound HTTP request and is not one of the %d rows "+
				"in security.md and configuration.md — add the row to both pages and name it "+
				"here, or say here why it is not a request an operator should be told about",
				f, len(outbound))
		}
	}
	for f, row := range outbound {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("%s is named here as the source of %q and does not exist: %v", f, row, err)
		}
	}

	// The count both pages must say, spelled as they spell it.
	words := map[int]string{1: "One", 2: "Two", 3: "Three", 4: "Four", 5: "Five", 6: "Six"}
	want := words[len(outbound)]
	if want == "" {
		t.Fatalf("no word for %d outbound requests — extend the map above", len(outbound))
	}

	claim := regexp.MustCompile(`(?m)^([A-Z][a-z]+), and this is the whole list\.$`)
	for _, page := range []string{"docs/_docs/security.md", "docs/_docs/configuration.md"} {
		raw, err := os.ReadFile(filepath.Join(root, page))
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		m := claim.FindAllStringSubmatch(string(raw), -1)
		if len(m) != 1 {
			t.Errorf("%s has %d sentences reading \"<number>, and this is the whole list.\", want 1 — "+
				"that sentence is what this test compares against the code", page, len(m))
			continue
		}
		if m[0][1] != want {
			t.Errorf("%s says %q outbound requests; internal/ builds %d (%s)",
				page, m[0][1], len(outbound), strings.Join(rowNames(outbound), ", "))
		}
	}
}

func rowNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, fmt.Sprintf("%q", v))
	}
	sort.Strings(out)
	return out
}
