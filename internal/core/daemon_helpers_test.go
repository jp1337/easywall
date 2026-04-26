package core

import (
	"os"
	"testing"
)

// TestLookupGroup_WithCustomFile verifies that lookupGroup correctly parses
// a synthetic /etc/group file, covering multi-field lines and GID extraction.
func TestLookupGroup_WithCustomFile(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString("root:x:0:root\n")
	_, _ = f.WriteString("daemon:x:1:daemon\n")
	_, _ = f.WriteString("testgroup:x:1234:user1,user2\n")
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	gid, err := lookupGroup("testgroup")
	if err != nil {
		t.Fatalf("lookupGroup: %v", err)
	}
	if gid != 1234 {
		t.Errorf("expected GID 1234, got %d", gid)
	}
}

func TestLookupGroup_FirstEntry(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString("root:x:0:\n")
	_, _ = f.WriteString("wheel:x:10:\n")
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	gid, err := lookupGroup("root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gid != 0 {
		t.Errorf("expected GID 0, got %d", gid)
	}
}

func TestLookupGroup_LastEntry(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString("root:x:0:\n")
	_, _ = f.WriteString("last:x:999:\n")
	// no trailing newline — scanner must still find this line
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	gid, err := lookupGroup("last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gid != 999 {
		t.Errorf("expected GID 999, got %d", gid)
	}
}

func TestLookupGroup_NotFound_CustomFile(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString("root:x:0:root\n")
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	_, err = lookupGroup("nonexistent")
	if err == nil {
		t.Error("expected error for missing group")
	}
}

func TestLookupGroup_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	_, err = lookupGroup("root")
	if err == nil {
		t.Error("expected error for empty group file")
	}
}

func TestLookupGroup_MalformedLine(t *testing.T) {
	f, err := os.CreateTemp("", "group-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	// Line with only 2 colon-separated fields — too few to extract GID
	_, _ = f.WriteString("badline:x\n")
	_, _ = f.WriteString("goodgroup:x:42:\n")
	f.Close()

	old := groupFilePath
	groupFilePath = f.Name()
	defer func() { groupFilePath = old }()

	gid, err := lookupGroup("goodgroup")
	if err != nil {
		t.Fatalf("should find goodgroup despite preceding malformed line: %v", err)
	}
	if gid != 42 {
		t.Errorf("expected GID 42, got %d", gid)
	}
}
