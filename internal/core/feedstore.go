package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"sync"
	"time"
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

// cachedFeed is a record with its prefixes parsed, so an apply never parses
// the file again. rec.Prefixes is dropped once parsed: a write renders them
// from prefixes.
type cachedFeed struct {
	rec      storedFeed
	prefixes []netip.Prefix
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
