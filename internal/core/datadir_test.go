package core

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestTheLastApplyMarkerDoesNotWriteThroughALink is 2.22's finding, measured:
// with last_apply a link to another file, an accepted apply must replace the
// link and leave the target alone. os.WriteFile wrote through it — a root write
// to any file, for anyone who could write data_dir.
func TestTheLastApplyMarkerDoesNotWriteThroughALink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "last_apply")
	if err := os.Symlink(victim, marker); err != nil {
		t.Fatal(err)
	}

	f := &Firewall{cfg: &Config{}}
	f.cfg.DataDir = dir
	f.setLastApply(time.Now())

	if got, _ := os.ReadFile(victim); string(got) != "untouched" { // #nosec G304 -- the test's own file
		t.Fatalf("the link's target now reads %q: root wrote through a link in data_dir", got)
	}
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("last_apply is not a regular file after the apply (%v, %v)", info, err)
	}
	if readLastApply(marker).IsZero() {
		t.Error("the marker written in the link's place does not read back")
	}
}

// TestTheLastApplyMarkerIsNotReadThroughALink: the parse error quotes what was
// read, so a link would copy the head of any root-readable file into the log.
// The target here holds a valid time, so a read that followed the link would
// return it.
func TestTheLastApplyMarkerIsNotReadThroughALink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("2026-01-01T00:00:00Z"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "last_apply")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if got := readLastApply(link); !got.IsZero() {
		t.Errorf("readLastApply followed a link and read %v", got)
	}
}

// TestTheLastApplyMarkerIsReadWithoutBlockingOnAFIFO: the 2.21 layout let the
// web user create any name in data_dir, including a FIFO. Opening one for
// read without O_NONBLOCK blocks until something opens it for write — which
// never happens here — and would hang the core at every start.
func TestTheLastApplyMarkerIsReadWithoutBlockingOnAFIFO(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "last_apply")
	if err := syscall.Mkfifo(marker, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan time.Time, 1)
	go func() { done <- readLastApply(marker) }()
	select {
	case got := <-done:
		if !got.IsZero() {
			t.Errorf("readLastApply(a FIFO) = %v, want the zero value", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("readLastApply blocked on a FIFO for 1s")
	}
}

// TestTheLastApplyMarkerIsNotReadFromAFIFOSomebodyHoldsOpen: the case the
// Stat check is for. With no writer, a non-blocking read of a FIFO ends at once
// on EOF; with one holding it open, Go parks the read in its poller until data
// arrives — which is never, so the core would hang at start just the same.
func TestTheLastApplyMarkerIsNotReadFromAFIFOSomebodyHoldsOpen(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "last_apply")
	if err := syscall.Mkfifo(marker, 0600); err != nil {
		t.Fatal(err)
	}
	// O_RDWR opens a FIFO on Linux without waiting for a reader.
	w, err := os.OpenFile(marker, os.O_RDWR, 0) // #nosec G304 -- the test's own FIFO
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	done := make(chan time.Time, 1)
	go func() { done <- readLastApply(marker) }()
	select {
	case got := <-done:
		if !got.IsZero() {
			t.Errorf("readLastApply(a held FIFO) = %v, want the zero value", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("readLastApply blocked for 1s on a FIFO somebody holds open")
	}
}

// TestASharedDataDirIsSaidOutLoud: the 2.21 mode warns, the 2.22 one does not.
func TestASharedDataDirIsSaidOutLoud(t *testing.T) {
	for mode, want := range map[os.FileMode]bool{0o770: true, 0o757: true, 0o750: false, 0o700: false} {
		dir := t.TempDir()
		if err := os.Chmod(dir, mode); err != nil { // #nosec G302 -- the modes under test
			t.Fatal(err)
		}
		if got := dataDirIsShared(dir); got != want {
			t.Errorf("data_dir %04o: dataDirIsShared = %v, want %v", mode, got, want)
		}
	}
}

// TestEveryCoreDataPathIsDirectlyInDataDir holds the core's half of the 2.22
// split: every file it keeps under data_dir sits in data_dir itself — root's
// directory — and none in data_dir/web, the web user's. Walks every *Path
// method on Config, so a path added later is covered without anybody
// remembering to list it here.
func TestEveryCoreDataPathIsDirectlyInDataDir(t *testing.T) {
	c := &Config{}
	c.DataDir = "/d"
	c.LogDir = "/l"
	v := reflect.ValueOf(c)
	seen := 0
	for i := 0; i < v.NumMethod(); i++ {
		m := v.Type().Method(i)
		if !strings.HasSuffix(m.Name, "Path") || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 ||
			m.Type.Out(0).Kind() != reflect.String {
			continue
		}
		p := v.Method(i).Call(nil)[0].String()
		seen++
		switch {
		case strings.HasPrefix(p, "/d/"):
			if filepath.Dir(p) != "/d" {
				t.Errorf("%s is %s: under data_dir but not directly in it. data_dir/web is the web "+
					"user's, and a root write there goes through whatever that user left", m.Name, p)
			}
		case strings.HasPrefix(p, "/l/"):
		default:
			t.Errorf("%s is %s: outside both data_dir and log_dir, so this test knows nothing about who can write it", m.Name, p)
		}
	}
	if seen < 8 {
		t.Fatalf("found %d *Path methods on Config, want at least 8: the walk is reading nothing", seen)
	}
}

// TestTheCoreSaysAtStartThatDataDirIsShared holds the call, not the helper:
// NewDaemon is where the warning has to be said, and a helper nobody calls
// tells no one. The log directory is made to fail so NewDaemon returns before
// it reaches nftables.
func TestTheCoreSaysAtStartThatDataDirIsShared(t *testing.T) {
	cfg := newTestConfig(t)
	if err := os.Chmod(cfg.DataDir, 0o770); err != nil { // #nosec G302 -- the 2.21 mode, under test
		t.Fatal(err)
	}
	cfg.LogDir = "/proc/easywall-unit-test-log"
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	_, _ = NewDaemon(cfg)

	if !strings.Contains(buf.String(), "data_dir is writable by more than its owner") {
		t.Errorf("NewDaemon started on a 0770 data_dir without saying so; the log was:\n%s", buf.String())
	}
}

// TestADataDirSomebodyElseOwnsIsSaidOutLoud: the owner of a directory can
// replace anything in it whatever its mode, so a data_dir the core does not
// own is shared with whoever does. "/" stands in for one: it is root's, and
// this test needs to run as somebody else.
func TestADataDirSomebodyElseOwnsIsSaidOutLoud(t *testing.T) {
	info, err := os.Stat("/")
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Uid == uint32(os.Geteuid()) { // #nosec G115 -- a uid
		t.Skip("/ is owned by this process's user; there is no directory here somebody else owns")
	}
	if !dataDirIsShared("/") {
		t.Error("dataDirIsShared(a directory owned by another user) = false, want true")
	}
}
