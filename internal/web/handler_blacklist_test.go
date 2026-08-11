package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestParseIPList_Empty(t *testing.T) {
	result := parseIPList("")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got: %v", result)
	}
}

func TestParseIPList_SingleIP(t *testing.T) {
	result := parseIPList("192.168.1.1")
	if len(result) != 1 || result[0] != "192.168.1.1" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestParseIPList_MultipleIPs(t *testing.T) {
	result := parseIPList("10.0.0.1\n10.0.0.2\n10.0.0.3")
	if len(result) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(result), result)
	}
}

// Comments are part of the list, not noise to filter out on the way to storage.
// Dropping them here deleted an operator's notes on every save — including the
// comments that explain a hand-written nftables rule on the custom page, where
// the note is often the only thing that says what the rule is for.
func TestParseIPList_KeepsComments(t *testing.T) {
	result := parseIPList("# scanners\n10.0.0.1\n# from the abuse report\n10.0.0.2")
	want := []string{"# scanners", "10.0.0.1", "# from the abuse report", "10.0.0.2"}
	if len(result) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(result), result)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, result[i], want[i])
		}
	}
}

// Blank lines between groups are the operator's own structure. Trailing ones
// carry nothing and would pile up on every save.
func TestParseIPList_KeepsInteriorBlanksAndDropsTrailingOnes(t *testing.T) {
	result := parseIPList("10.0.0.1\n\n10.0.0.2\n\n\n")
	want := []string{"10.0.0.1", "", "10.0.0.2"}
	if len(result) != len(want) {
		t.Fatalf("expected %v, got %v", want, result)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, result[i], want[i])
		}
	}
}

func TestIsListComment(t *testing.T) {
	for _, s := range []string{"", "   ", "# note", "  # indented note"} {
		if !shared.IsListComment(s) {
			t.Errorf("%q is a comment or spacer", s)
		}
	}
	for _, s := range []string{"10.0.0.1", "2001:db8::/32", "198.51.100.0/24"} {
		if shared.IsListComment(s) {
			t.Errorf("%q is an address", s)
		}
	}
}

func TestParseIPList_TrimsWhitespace(t *testing.T) {
	result := parseIPList("  10.0.0.1  \n  10.0.0.2\t")
	if len(result) != 2 || result[0] != "10.0.0.1" || result[1] != "10.0.0.2" {
		t.Errorf("unexpected (whitespace not trimmed): %v", result)
	}
}

func TestParseIPList_AllComments(t *testing.T) {
	// Kept, and enforcing nothing: countEntries reports zero entries, and the
	// core turns none of them into rules.
	result := parseIPList("# only comments\n# here too")
	if len(result) != 2 {
		t.Errorf("expected both comment lines to survive, got: %v", result)
	}
}

func TestParseIPList_CIDR(t *testing.T) {
	result := parseIPList("10.0.0.0/8\n192.168.0.0/16")
	if len(result) != 2 {
		t.Errorf("expected 2 CIDR entries, got: %v", result)
	}
}

func TestParseIPList_ReturnsSliceNotNil(t *testing.T) {
	result := parseIPList("")
	if result == nil {
		t.Error("expected non-nil empty slice")
	}
}

// A rejected list must come back with the operator's text and the line numbers.
//
// Both editors used to redirect, which repopulated the textarea from the stored
// list — so paste forty addresses with one typo among them and all forty were
// gone, under a message saying "the line numbers are listed above the editor"
// pointing at an empty panel.
func TestHandleBlacklistPOST_RejectedListKeepsTheTextAndNamesTheLines(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var reached bool
	fc.OnCommand(shared.CmdSaveRules, func(shared.Command) { reached = true })

	entries := "# a note I just wrote\n192.168.1.999\n203.0.113.77"
	rec := doAuthFormRequest(t, s, "/blacklist", "entries="+urlEncode(entries))

	if reached {
		t.Error("an invalid address list was forwarded to the core")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected the editor to be re-rendered, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"a note I just wrote", "192.168.1.999", "203.0.113.77"} {
		if !strings.Contains(body, want) {
			t.Errorf("the response lost %q — everything typed must survive a rejection", want)
		}
	}
	// The line number of the bad address, which is what the message promises.
	if !strings.Contains(body, "2") || !strings.Contains(body, "validate_invalid") && !strings.Contains(body, "Invalid entries") {
		t.Error("the response does not name the rejected line")
	}
}

func TestHandleWhitelistPOST_RejectedListKeepsTheText(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var reached bool
	fc.OnCommand(shared.CmdSaveRules, func(shared.Command) { reached = true })

	rec := doAuthFormRequest(t, s, "/whitelist", "entries="+urlEncode("203.0.113.10\nnot-an-address"))

	if reached {
		t.Error("an invalid address list was forwarded to the core")
	}
	if !strings.Contains(rec.Body.String(), "not-an-address") {
		t.Error("the rejected line is not in the response")
	}
	if !strings.Contains(rec.Body.String(), "203.0.113.10") {
		t.Error("the valid line the operator also typed was lost")
	}
}
