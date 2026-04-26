package web

import (
	"testing"
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

func TestParseIPList_SkipsComments(t *testing.T) {
	result := parseIPList("# comment\n10.0.0.1\n# another comment\n10.0.0.2")
	if len(result) != 2 {
		t.Errorf("expected 2 entries (comments skipped), got %d: %v", len(result), result)
	}
}

func TestParseIPList_SkipsBlankLines(t *testing.T) {
	result := parseIPList("\n\n10.0.0.1\n\n10.0.0.2\n\n")
	if len(result) != 2 {
		t.Errorf("expected 2 entries (blanks skipped), got %d: %v", len(result), result)
	}
}

func TestParseIPList_TrimsWhitespace(t *testing.T) {
	result := parseIPList("  10.0.0.1  \n  10.0.0.2\t")
	if len(result) != 2 || result[0] != "10.0.0.1" || result[1] != "10.0.0.2" {
		t.Errorf("unexpected (whitespace not trimmed): %v", result)
	}
}

func TestParseIPList_AllComments(t *testing.T) {
	result := parseIPList("# only comments\n# here too")
	if len(result) != 0 {
		t.Errorf("expected empty result for all-comment input, got: %v", result)
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
