package web

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jp1337/easywall/internal/shared"
)

// FeedStatusKind is what a feed's row says about its last refresh — Pi-hole's
// five (spec §5).
type FeedStatusKind string

const (
	FeedNeverFetched FeedStatusKind = "never_fetched"
	FeedUpdated      FeedStatusKind = "updated"
	FeedUnchanged    FeedStatusKind = "unchanged"
	FeedFailedCopy   FeedStatusKind = "failed_copy"    // failed, previous copy active
	FeedFailedNoCopy FeedStatusKind = "failed_no_copy" // failed, nothing loaded
)

// AllFeedStatusKinds is every status; each has a label feed_status_<kind>.
var AllFeedStatusKinds = []FeedStatusKind{
	FeedNeverFetched, FeedUpdated, FeedUnchanged, FeedFailedCopy, FeedFailedNoCopy,
}

// Why a refresh failed. A fixed vocabulary, never a response body: what a
// server sends is the one thing on this path nobody here wrote, and it reaches
// neither the page nor the log. Each has a label feed_error_<reason>.
const (
	feedErrTimeout         = "timeout"          // 30 s passed
	feedErrUnreachable     = "unreachable"      // no connection, DNS, TLS
	feedErrTransfer        = "transfer"         // the body broke off or would not decompress
	feedErrRedirected      = "redirected"       // refused, never followed
	feedErrHTTPStatus      = "http_status"      // anything but 200 or an expected 304; HTTPStatus says which
	feedErrRateLimited     = "rate_limited"     // 429: the next try waits a whole interval
	feedErrTooLarge        = "too_large"        // over 8 MiB, over FeedMaxEntries, or over the socket limit
	feedErrHTML            = "html"             // a web page, not a list
	feedErrEmpty           = "empty"            // 200 and not one entry
	feedErrRefused         = "refused"          // the core's guards refused it; the core's audit log says which
	feedErrShrank          = "shrank"           // under 70 % of the live copy; ShrankFrom/ShrankTo say how far
	feedErrCoreUnreachable = "core_unreachable" // the socket, not the list
)

// AllFeedErrors is the vocabulary above, for the label guard.
var AllFeedErrors = []string{
	feedErrTimeout, feedErrUnreachable, feedErrTransfer, feedErrRedirected,
	feedErrHTTPStatus, feedErrRateLimited, feedErrTooLarge, feedErrHTML,
	feedErrEmpty, feedErrRefused, feedErrShrank, feedErrCoreUnreachable,
}

// OwnFeed is a list the operator adds by URL (spec D8): plain text, fetched by
// this process only. The password is write-only in the form, and never logged,
// exported or sent to the core — the core receives entries, never where they
// came from.
type OwnFeed struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

// feedFetchState is what this process remembers about one feed between
// refreshes. The copy itself, and changed_at/checked_at, are the core's.
type feedFetchState struct {
	// ETag and LastModified are the validators of a single-URL feed's last
	// 200. A two-URL feed (Spamhaus) keeps none — see refresh.
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`

	LastError   string         `json:"last_error,omitempty"`  // one of AllFeedErrors, "" after a success
	HTTPStatus  int            `json:"http_status,omitempty"` // with feedErrHTTPStatus
	Failures    int            `json:"failures,omitempty"`    // consecutive
	LastAttempt time.Time      `json:"last_attempt"`          // a request went out (or was about to)
	Status      FeedStatusKind `json:"status,omitempty"`

	Rejected       int      `json:"rejected,omitempty"`
	RejectedSample []string `json:"rejected_sample,omitempty"` // first five, each ≤ 80 bytes

	// ShrankFrom is the core's stored count and ShrankTo the count sent when
	// the core refused the list as a shrink (ErrFeedShrankText). Both zero
	// after the next update the core takes.
	ShrankFrom int `json:"shrank_from,omitempty"`
	ShrankTo   int `json:"shrank_to,omitempty"`
}

// feedStateFile is <data_dir>/web/feed_fetch.json.
type feedStateFile struct {
	// OffsetSeconds places this install's refreshes in each interval, drawn
	// once (P15), so installations do not all fetch on the hour.
	OffsetSeconds int64                       `json:"offset_seconds"`
	Fetch         map[string]feedFetchState   `json:"fetch"`
	Own           [shared.MaxOwnFeeds]OwnFeed `json:"own"`
}

// feedStore holds the file, read once and written whole on every change.
type feedStore struct {
	path string
	mu   sync.Mutex
	st   *feedStateFile
}

func newFeedStore(path string) *feedStore { return &feedStore{path: path} }

// loadLocked reads the file on first use. Missing: a fresh state with a new
// offset. Unreadable: the same, said once — what is lost is validators (one
// unconditional fetch each) and own feeds (entered again), never a copy.
func (fs *feedStore) loadLocked() *feedStateFile {
	if fs.st != nil {
		return fs.st
	}
	st := &feedStateFile{}
	data, err := os.ReadFile(fs.path) // #nosec G304 -- a fixed name in this process's own state dir
	if err == nil {
		err = json.Unmarshal(data, st)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("feed state unreadable; starting from none", "path", fs.path, "error", err)
		st = &feedStateFile{}
	}
	if st.Fetch == nil {
		st.Fetch = map[string]feedFetchState{}
	}
	if st.OffsetSeconds <= 0 || st.OffsetSeconds >= int64(24*time.Hour/time.Second) {
		st.OffsetSeconds = randomFeedOffset()
		_ = fs.saveLocked(st) // logged; the offset is drawn again next start
	}
	fs.st = st
	return st
}

func (fs *feedStore) saveLocked(st *feedStateFile) error {
	data, err := json.Marshal(st)
	if err == nil {
		err = writeFileAtomic(fs.path, data, 0600)
	}
	if err != nil {
		slog.Warn("feed state not saved", "path", fs.path, "error", err)
	}
	return err
}

func (fs *feedStore) offset() time.Duration {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return time.Duration(fs.loadLocked().OffsetSeconds) * time.Second
}

func (fs *feedStore) fetchState(id string) feedFetchState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.loadLocked().Fetch[id]
}

func (fs *feedStore) setFetchState(id string, v feedFetchState) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	st := fs.loadLocked()
	st.Fetch[id] = v
	_ = fs.saveLocked(st) // logged; costs one unconditional fetch at worst
}

// ownFeed is own feed n (1..MaxOwnFeeds); ok is false when the slot has no URL.
func (fs *feedStore) ownFeed(n int) (OwnFeed, bool) {
	if n < 1 || n > shared.MaxOwnFeeds {
		return OwnFeed{}, false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	f := fs.loadLocked().Own[n-1]
	return f, f.URL != ""
}

// randomFeedOffset is a second of the day, from crypto/rand as the telemetry
// spread is. 1 at the least, so 0 can mean "not drawn yet".
func randomFeedOffset() int64 {
	day := int64(24 * time.Hour / time.Second)
	n, err := rand.Int(rand.Reader, big.NewInt(day-1))
	if err != nil {
		return day / 2
	}
	return n.Int64() + 1
}

// Why saveOwnFeed refused. Task 7 renders each under its own key.
var (
	errOwnFeedSlot  = errors.New("no such own feed")
	errOwnFeedName  = errors.New("own feed name: 1 to 64 printable characters")
	errOwnFeedURL   = errors.New("own feed URL: https, or http to this host only, with no credentials in it")
	errOwnFeedLogin = errors.New("own feed user and password: printable, at most 256 characters, no ':' in the user")
	errOwnFeedDemo  = errors.New("the demo keeps no own feeds")
	// errOwnFeedPasswordAgain: an empty password field with a changed scheme
	// or host — kept only when the address did not change (review round 1,
	// finding 1).
	errOwnFeedPasswordAgain = errors.New("own feed password: type it again for the new address")
)

// validOwnFeedURL is P14: https to any host; http only to a loopback host,
// which is where CrowdSec's cs-blocklist-mirror listens. Credentials belong in
// their own fields, where they are write-only, and never in the URL, which
// the row shows the host of.
func validOwnFeedURL(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || u.Opaque != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		// A literal loopback address only. "localhost" is resolved like any
		// name — Go hard-codes it for NSS myhostname alone (net/conf.go) — so
		// a host with no /etc/hosts line would ask DNS, and send Basic
		// credentials in cleartext to whatever it answered.
		a, err := netip.ParseAddr(u.Hostname())
		return err == nil && a.IsLoopback()
	}
	return false
}

// sameOwnFeedHost reports whether two own-feed URLs share a scheme and host
// (including port) — the boundary saveOwnFeed uses to decide whether a
// stored password may follow a save without being retyped. A changed scheme
// or host is a different server; keeping the old password there would hand
// it to whatever "https://attacker.example/…" a session holder typed
// (review round 1, finding 1).
func sameOwnFeedHost(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ua, erra := url.Parse(a)
	ub, errb := url.Parse(b)
	return erra == nil && errb == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

func printable(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// forgetPendingFeed drops an update the runner holds for id. No runner in
// demo mode, and none in a handler test.
func (s *Server) forgetPendingFeed(id string) {
	if s.feeds != nil {
		s.feeds.forget(id)
	}
}

// saveOwnFeed stores own feed n (1..3). An empty Password keeps the stored
// one — the form never shows it back — but only when the scheme and host
// (incl. port) are unchanged; a changed address with an empty password field
// is refused with errOwnFeedPasswordAgain, never sent the old credential
// (review round 1, finding 1). An empty URL and name clear the slot,
// including any stored password. A new URL or user forgets the old
// validators and failures: they described another list.
func (s *Server) saveOwnFeed(n int, f OwnFeed) error {
	if s.cfg.DemoMode {
		// A public page; a credential typed into it would be kept on its host.
		return errOwnFeedDemo
	}
	if n < 1 || n > shared.MaxOwnFeeds {
		return errOwnFeedSlot
	}
	f.Name, f.URL, f.User = strings.TrimSpace(f.Name), strings.TrimSpace(f.URL), strings.TrimSpace(f.User)
	fs := s.feedStore
	fs.mu.Lock()
	defer fs.mu.Unlock()
	st := fs.loadLocked()
	id := shared.OwnFeedID(n)

	if f.Name == "" && f.URL == "" {
		st.Own[n-1] = OwnFeed{}
		delete(st.Fetch, id)
		s.forgetPendingFeed(id)
		return fs.saveLocked(st)
	}
	switch {
	case f.Name == "" || !printable(f.Name, 64):
		return errOwnFeedName
	case !validOwnFeedURL(f.URL):
		return errOwnFeedURL
	case !printable(f.User, 256) || strings.Contains(f.User, ":") || !printable(f.Password, 256):
		return errOwnFeedLogin
	}
	old := st.Own[n-1]
	switch {
	case f.User == "":
		f.Password = "" // no user, no credentials
	case f.Password == "" && !sameOwnFeedHost(old.URL, f.URL):
		// A changed scheme or host is a different server: an empty password
		// field must not hand the old one to it (review round 1, finding 1).
		return errOwnFeedPasswordAgain
	case f.Password == "":
		f.Password = old.Password
	}
	if f.URL != old.URL || f.User != old.User {
		delete(st.Fetch, id)
		s.forgetPendingFeed(id)
	}
	st.Own[n-1] = f
	return fs.saveLocked(st)
}
