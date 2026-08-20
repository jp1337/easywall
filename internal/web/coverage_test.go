package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocaleCoverage_EnglishIsTheYardstickAndIsComplete(t *testing.T) {
	cov, err := LocaleCoverage(localesDir(t))
	if err != nil {
		t.Fatalf("LocaleCoverage: %v", err)
	}
	if len(cov) == 0 {
		t.Fatal("no coverage reported")
	}
	for _, c := range cov {
		if c.Lang != "en" {
			continue
		}
		if len(c.Missing) != 0 || c.Percent() != 100 {
			t.Errorf("en is the yardstick and must be 100%%, got %d%% missing %v",
				c.Percent(), c.Missing)
		}
	}
}

// Percent must never round a gap up to a full hundred. "100%" is a promise, and
// a language one key short has not kept it.
func TestCoveragePercent_NeverRoundsAGapUpToAHundred(t *testing.T) {
	c := Coverage{Have: 9999, Total: 10000, Missing: []string{"one_key"}}
	if c.Percent() == 100 {
		t.Error("a language one key short reported 100%")
	}
	full := Coverage{Have: 10, Total: 10}
	if full.Percent() != 100 {
		t.Errorf("a complete language reported %d%%", full.Percent())
	}
}

// Pasting the English string must not raise the number.
//
// The fallback renders English for a missing key anyway, so a copied line
// changes nothing a user sees — it only changes what the report claims. The
// roadmap named this exact failure when it argued against exact parity.
func TestCoverage_CopiedEnglishDoesNotCount(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("en.json", `[{"id":"a","translation":"Apply"},{"id":"b","translation":"Cancel"}]`)
	write("xx.json", `[{"id":"a","translation":"Appliquer"},{"id":"b","translation":"Cancel"}]`)
	write("status.json", `{"en":{"reviewed":true},"xx":{"reviewed":false}}`)

	cov, err := LocaleCoverage(dir)
	if err != nil {
		t.Fatalf("LocaleCoverage: %v", err)
	}
	for _, c := range cov {
		if c.Lang != "xx" {
			continue
		}
		if c.Have != 1 {
			t.Errorf("Have = %d, want 1 — \"Cancel\" is the English string verbatim", c.Have)
		}
		if len(c.Missing) != 1 || c.Missing[0] != "b" {
			t.Errorf("Missing = %v, want [b]", c.Missing)
		}
	}
}

func TestLocaleCoverage_ReportsTheReviewedFlag(t *testing.T) {
	cov, err := LocaleCoverage(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cov {
		if c.Lang == "de" && !c.Reviewed {
			t.Error("de is marked unreviewed in the coverage report")
		}
	}
}

// TestLocaleCoverageReport never fails. It exists to be read with -v: a report
// you have to remember to run separately is a report nobody runs.
func TestLocaleCoverageReport(t *testing.T) {
	cov, err := LocaleCoverage(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cov {
		reviewed := "unreviewed"
		if c.Reviewed {
			reviewed = "reviewed"
		}
		t.Logf("%-4s %3d%%  (%d missing, %s)", c.Lang, c.Percent(), len(c.Missing), reviewed)
	}
}
