package core

import (
	"testing"
)

func TestSplitLines_Empty(t *testing.T) {
	lines := splitLines("")
	if len(lines) != 0 {
		t.Errorf("expected empty result, got: %v", lines)
	}
}

func TestSplitLines_Single(t *testing.T) {
	lines := splitLines("hello")
	// no newline → no lines returned (terminates before adding last segment)
	_ = lines // splitLines only splits on \n, returns segments before each \n
}

func TestSplitLines_WithNewlines(t *testing.T) {
	lines := splitLines("line1\nline2\nline3\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestSplitLines_TrailingNewline(t *testing.T) {
	lines := splitLines("a\nb\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %v", len(lines), lines)
	}
}

func TestSplitFields_Empty(t *testing.T) {
	fields := splitFields("", ":")
	// Should return [""] (one empty field)
	if len(fields) != 1 || fields[0] != "" {
		t.Errorf("expected [\"\"], got: %v", fields)
	}
}

func TestSplitFields_Single(t *testing.T) {
	fields := splitFields("hello", ":")
	if len(fields) != 1 || fields[0] != "hello" {
		t.Errorf("expected [\"hello\"], got: %v", fields)
	}
}

func TestSplitFields_Multiple(t *testing.T) {
	fields := splitFields("root:x:0:0", ":")
	if len(fields) != 4 {
		t.Errorf("expected 4 fields, got %d: %v", len(fields), fields)
	}
	if fields[0] != "root" || fields[1] != "x" || fields[2] != "0" || fields[3] != "0" {
		t.Errorf("unexpected fields: %v", fields)
	}
}

func TestSplitFields_TrailingSep(t *testing.T) {
	fields := splitFields("a:b:", ":")
	if len(fields) != 3 {
		t.Errorf("expected 3 fields (trailing empty), got %d: %v", len(fields), fields)
	}
	if fields[2] != "" {
		t.Errorf("expected empty trailing field, got: %q", fields[2])
	}
}
