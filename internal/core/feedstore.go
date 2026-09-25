package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The core's own copy of every feed (2.23, spec §2): <data_dir>/feeds.json,
// root 0600, directly in data_dir like every file the core keeps
// (TestEveryCoreDataPathIsDirectlyInDataDir). The web process fetches and
// parses; what reaches the kernel is only ever what this file holds, and it
// holds only what UPDATE_FEED let through.
//
// Not rules.json: that holds three copies of the rule set and Status()
// re-marshals it on every poll, and a feed is up to 100 000 prefixes.

// storedFeed is one feed's record in feeds.json.
type storedFeed struct {
	Prefixes  []string  `json:"prefixes"` // canonical, masked, deduplicated
	Entries   int       `json:"entries"`
	Dropped   int       `json:"dropped"` // not globally routable, removed by the core
	ChangedAt time.Time `json:"changed_at"`
	CheckedAt time.Time `json:"checked_at"`
}

// cachedFeed is a record with its prefixes parsed and merged, so an apply, a
// refresh and a GET_FEEDS never parse the file again. rec.Prefixes is dropped
// once parsed: a write renders them from prefixes.
type cachedFeed struct {
	rec              storedFeed
	prefixes         []netip.Prefix
	ranges4, ranges6 []addrRange
}

func newCachedFeed(id string, rec storedFeed) *cachedFeed {
	c := &cachedFeed{rec: rec, prefixes: make([]netip.Prefix, 0, len(rec.Prefixes))}
	for _, s := range rec.Prefixes {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			// Nothing but UPDATE_FEED writes this file, and it writes
			// p.String(). A line that does not parse is a hand edit or a
			// damaged disk, and it is skipped rather than allowed to cost the
			// rest of the feed.
			slog.Warn("feeds.json holds a prefix that does not parse; skipping it", "feed", id, "prefix", s)
			continue
		}
		c.prefixes = append(c.prefixes, p)
	}
	c.rec.Prefixes = nil
	c.ranges4, c.ranges6 = splitFamilies(mergeRanges(c.prefixes))
	return c
}

// FeedStore owns <data_dir>/feeds.json. mu serialises the cache and the
// read-modify-write Task 4's Put does.
type FeedStore struct {
	path string

	mu     sync.Mutex
	loaded bool
	feeds  map[string]*cachedFeed
}

// NewFeedStore returns a store over path. Nothing is read until the first
// call; a missing file is a store with no copies.
func NewFeedStore(path string) *FeedStore {
	return &FeedStore{path: path}
}

// load fills the cache once. The caller holds mu.
//
// An unparseable file is warned about and read as empty, UsageStore's answer
// and for its reason: a hard error would make the file permanent, and here it
// would also make every apply fail, which turns a damaged bookkeeping file into
// a firewall that cannot be changed. Empty means every enabled feed gets an
// empty set until its next refresh writes a fresh file. An I/O error stays an
// error: a file that cannot be read is not corrupt, and overwriting it would
// throw away a good copy.
func (s *FeedStore) load() error {
	if s.loaded {
		return nil
	}
	data, err := os.ReadFile(s.path) // #nosec G304 -- path is built from the daemon's own config
	var recs map[string]storedFeed
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return fmt.Errorf("read feeds: %w", err)
	default:
		if err := json.Unmarshal(data, &recs); err != nil {
			slog.Warn("feeds.json could not be read and is treated as empty; every enabled feed "+
				"gets an empty set until its next refresh", "path", s.path, "error", err)
			recs = nil
		}
	}
	s.feeds = make(map[string]*cachedFeed, len(recs))
	for id, rec := range recs {
		s.feeds[id] = newCachedFeed(id, rec)
	}
	s.loaded = true
	return nil
}

// Contents returns the stored prefixes of each id in ids. An id with no copy
// is absent, and Apply builds its set empty. The slices are the cache's own:
// read them, never write them.
func (s *FeedStore) Contents(ids []string) (FeedContents, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return nil, err
	}
	out := FeedContents{}
	for _, id := range ids {
		if c, ok := s.feeds[id]; ok {
			out[id] = c.prefixes
		}
	}
	return out, nil
}

// All returns every stored record without its prefixes.
func (s *FeedStore) All() (map[string]storedFeed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return nil, err
	}
	out := make(map[string]storedFeed, len(s.feeds))
	for id, c := range s.feeds {
		out[id] = c.rec
	}
	return out, nil
}

// Get returns one stored record, prefixes included.
func (s *FeedStore) Get(id string) (storedFeed, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return storedFeed{}, false, err
	}
	c, ok := s.feeds[id]
	if !ok {
		return storedFeed{}, false, nil
	}
	rec := c.rec
	rec.Prefixes = prefixStrings(c.prefixes)
	return rec, true, nil
}

// Put stores f as id's copy and drops every copy whose id is not in keep
// (plan P13: keep is Current ∪ Staged ∪ Backup, so a rollback always finds
// the copy the rules it restores switch on). Atomic: a temporary file, then a
// rename, and the cache changes only once the file has.
func (s *FeedStore) Put(id string, f storedFeed, keep []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	// Only checked_at moved — a 304, or the same list again: the cache takes
	// it and the disk does not. A full rewrite is 2.7 MB with the catalogue
	// (14 ms, measured), and hourly lists would write ≈ 330 MB a day to an SD
	// card for a timestamp. After a restart checked_at reads as of the last
	// change until the next check.
	if c, ok := s.feeds[id]; ok && c.rec.Entries == f.Entries && c.rec.Dropped == f.Dropped &&
		c.rec.ChangedAt.Equal(f.ChangedAt) && slices.Equal(prefixStrings(c.prefixes), f.Prefixes) &&
		!slices.ContainsFunc(slices.Collect(maps.Keys(s.feeds)), func(k string) bool { return !slices.Contains(keep, k) }) {
		c.rec.CheckedAt = f.CheckedAt
		return nil
	}
	next := maps.Clone(s.feeds)
	next[id] = newCachedFeed(id, f)
	maps.DeleteFunc(next, func(k string, _ *cachedFeed) bool { return !slices.Contains(keep, k) })
	if err := s.write(next); err != nil {
		return err
	}
	s.feeds = next
	return nil
}

// write is writeLastApply's shape — CreateTemp (O_EXCL, 0600, never through a
// link), then rename — for the reason given there.
func (s *FeedStore) write(feeds map[string]*cachedFeed) error {
	recs := make(map[string]storedFeed, len(feeds))
	for id, c := range feeds {
		rec := c.rec
		rec.Prefixes = prefixStrings(c.prefixes)
		recs[id] = rec
	}
	data, err := json.Marshal(recs)
	if err != nil {
		return fmt.Errorf("marshal feeds: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "feeds-*.json.tmp")
	if err != nil {
		return fmt.Errorf("write feeds: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write feeds: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write feeds: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write feeds: %w", err)
	}
	return nil
}

func prefixStrings(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

// Contains reports whether id's copy holds addr.
func (s *FeedStore) Contains(id string, addr netip.Addr) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load() // an unreadable file is no copies here; All and Get report it
	c, ok := s.feeds[id]
	if !ok || !addr.IsValid() {
		return false
	}
	return overlapsAny(c.family(addr), addrRange{addr, addr})
}

// AllowlistOverlap counts the entries of an allowlist that id's copy covers
// at least in part. They stay reachable — the allowlist is evaluated first —
// and the feed's row says so. A comment or a blank line parses as neither an
// address nor a network, and is skipped with them.
func (s *FeedStore) AllowlistOverlap(id string, allowlist []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load() // an unreadable file is no copies here; All and Get report it
	c, ok := s.feeds[id]
	if !ok {
		return 0
	}
	n := 0
	for _, e := range allowlist {
		e = strings.TrimSpace(e)
		var r addrRange
		if a, err := netip.ParseAddr(e); err == nil {
			a = a.Unmap()
			r = addrRange{a, a}
		} else if p, err := shared.ParseNetwork(e); err == nil {
			r = addrRange{p.Addr(), lastAddr(p)}
		} else {
			continue
		}
		if overlapsAny(c.family(r.from), r) {
			n++
		}
	}
	return n
}

func (c *cachedFeed) family(a netip.Addr) []addrRange {
	if a.Is4() {
		return c.ranges4
	}
	return c.ranges6
}

// overlapsAny reports whether r shares an address with one of rs, which are
// sorted and disjoint (mergeRanges).
func overlapsAny(rs []addrRange, r addrRange) bool {
	i := sort.Search(len(rs), func(i int) bool { return rs[i].to.Compare(r.from) >= 0 })
	return i < len(rs) && rs[i].from.Compare(r.to) <= 0
}
