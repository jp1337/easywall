package core

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func writeFeedsFile(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "feeds.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFeedStore_ContentsReadsTheStoredCopies(t *testing.T) {
	path := writeFeedsFile(t, t.TempDir(), `{
		"dshield": {"prefixes": ["198.51.100.0/24", "2001:db8::/32", "not a prefix"], "entries": 3},
		"cins": {"prefixes": ["203.0.113.7/32"], "entries": 1}
	}`)
	got, err := NewFeedStore(path).Contents([]string{"dshield", "tor-exits"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["tor-exits"]; ok {
		t.Error("an id with no copy is in the result; Apply must build its set empty, not from nothing")
	}
	if _, ok := got["cins"]; ok {
		t.Error("a stored feed that was not asked for is in the result")
	}
	want := []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("2001:db8::/32")}
	if d := got["dshield"]; len(d) != 2 || d[0] != want[0] || d[1] != want[1] {
		t.Errorf("dshield = %v, want %v (the unparseable line skipped)", d, want)
	}
}

// The cache is read once: an apply, a rollback and a boot restore do not parse
// a file of up to a million prefixes each time.
func TestFeedStore_ReadsTheFileOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeFeedsFile(t, dir, `{"dshield": {"prefixes": ["198.51.100.0/24"]}}`)
	s := NewFeedStore(path)
	if _, err := s.Contents([]string{"dshield"}); err != nil {
		t.Fatal(err)
	}
	writeFeedsFile(t, dir, `{"dshield": {"prefixes": []}}`)
	if got, _ := s.Contents([]string{"dshield"}); len(got["dshield"]) != 1 {
		t.Errorf("the store re-read the file: %v", got)
	}
}

func TestFeedStore_AMissingOrDamagedFileIsNoCopies(t *testing.T) {
	dir := t.TempDir()
	for name, path := range map[string]string{
		"missing": filepath.Join(dir, "absent.json"),
		"damaged": writeFeedsFile(t, dir, `{"dshield": {"prefixes": [`),
	} {
		got, err := NewFeedStore(path).Contents([]string{"dshield"})
		if err != nil || len(got) != 0 {
			t.Errorf("%s: Contents = %v, %v; want no copies and no error — an error here fails "+
				"every apply and every rollback", name, got, err)
		}
	}
}

// An I/O error is not a damaged file: it is reported, and the store does not
// cache the failure as "no copies".
func TestFeedStore_AnUnreadableFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feeds.json")
	if err := os.Mkdir(path, 0700); err != nil { // reading a directory fails with EISDIR
		t.Fatal(err)
	}
	s := NewFeedStore(path)
	if _, err := s.Contents([]string{"dshield"}); err == nil {
		t.Fatal("reading a directory as feeds.json succeeded")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writeFeedsFile(t, dir, `{"dshield": {"prefixes": ["198.51.100.0/24"]}}`)
	if got, err := s.Contents([]string{"dshield"}); err != nil || len(got["dshield"]) != 1 {
		t.Errorf("after the error went away: %v, %v", got, err)
	}
}

// Every production writer goes through feedContents (TestNoProductionCodeCallsApplyWithoutFeeds
// holds the call sites); this holds what it loads.
func TestFeedContentsLoadsWhatTheRulesSwitchOn(t *testing.T) {
	cfg := newTestConfig(t)
	writeFeedsFile(t, cfg.DataDir, `{"dshield": {"prefixes": ["198.51.100.0/24"]}, "cins": {"prefixes": ["203.0.113.7/32"]}}`)
	f := &Firewall{cfg: cfg, feeds: NewFeedStore(cfg.FeedsPath())}

	got := f.feedContents(shared.Rules{Feeds: []string{"dshield"}})
	if len(got) != 1 || len(got["dshield"]) != 1 {
		t.Errorf("feedContents = %v, want dshield's copy and nothing else", got)
	}
	if got := f.feedContents(shared.Rules{}); got != nil {
		t.Errorf("no feed switched on, and feedContents read %v", got)
	}
	if got := (&Firewall{cfg: cfg}).feedContents(shared.Rules{Feeds: []string{"dshield"}}); got != nil {
		t.Errorf("a Firewall with no store returned %v", got)
	}
}

// The path the walk in TestEveryCoreDataPathIsDirectlyInDataDir also covers,
// named so a rename is visible here.
func TestFeedsPathIsInDataDir(t *testing.T) {
	c := &Config{}
	c.DataDir = "/var/lib/easywall"
	if got := c.FeedsPath(); got != "/var/lib/easywall/feeds.json" {
		t.Errorf("FeedsPath = %s", got)
	}
}
