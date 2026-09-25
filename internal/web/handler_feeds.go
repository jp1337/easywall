package web

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/jp1337/easywall/internal/shared"
)

// The Feeds card on /blocklist (spec §5). Switching a feed is a rule change
// (D2): the card stages it with SAVE_RULES("feeds"), and the apply screen
// applies it inside the acceptance window, like the list above it. Two forms
// of their own, because an HTML form cannot nest and the blocklist's textarea
// has one already.

// feedsCard is what the card renders. Callers build one only once GET_RULES
// has answered, and leave it nil otherwise: a toggle drawn without knowing
// what is staged would switch every feed off on its first save.
type feedsCard struct {
	Rows     []feedRow
	CoreRead bool // GET_FEEDS answered; otherwise counts, times and counters are not shown

	// A refused own-feed save, rendered beside its slot with what was typed.
	OwnErrSlot int    // 1..3, 0 for none
	OwnErrKey  string // a feeds_own_err_* key
}

func (s *Server) feedsCard(state *shared.RulesState) *feedsCard {
	rows := s.feedRows(state)
	return &feedsCard{Rows: rows, CoreRead: len(rows) > 0 && rows[0].CoreRead}
}

// ownFeedN is n for "own-n", 0 for anything else.
func ownFeedN(id string) int {
	for n := 1; n <= shared.MaxOwnFeeds; n++ {
		if id == shared.OwnFeedID(n) {
			return n
		}
	}
	return 0
}

// stagedFeeds is what a save of the card stages: the ids still switched on,
// in the order they were switched on (Rules.Feeds' order), then the newly
// switched on, in the card's order. The second value is a flash key when the
// save is refused, and then nothing is staged.
//
// A ✗ feed is only newly switched on with its own confirmation box ticked —
// checked here, not in the browser, so a form posted without JavaScript or
// by hand is held to it too (spec §1). An own feed with no URL cannot be
// newly switched on: nothing could fetch it. One already staged may stay,
// which is how an imported own-N is switched off again.
func stagedFeeds(before, on, confirmed []string, configured func(n int) bool) ([]string, string) {
	want := map[string]bool{}
	for _, id := range on {
		if !shared.KnownFeedID(id) {
			return nil, "save_error"
		}
		want[id] = true
	}
	next := []string{}
	for _, id := range before {
		if want[id] {
			next = append(next, id)
			delete(want, id)
		}
	}
	order := make([]string, 0, len(shared.FeedCatalogue)+shared.MaxOwnFeeds)
	for _, f := range shared.FeedCatalogue {
		order = append(order, f.ID)
	}
	for n := 1; n <= shared.MaxOwnFeeds; n++ {
		order = append(order, shared.OwnFeedID(n))
	}
	for _, id := range order {
		if !want[id] {
			continue
		}
		if f, ok := shared.CatalogueFeedByID(id); ok && f.Verdict == shared.FeedDeliberate && !slices.Contains(confirmed, id) {
			return nil, "feeds_refused_confirm"
		}
		if n := ownFeedN(id); n > 0 && !configured(n) {
			return nil, "feeds_refused_unconfigured"
		}
		next = append(next, id)
	}
	return next, ""
}

// handleFeedsPOST saves the card's switches.
func (s *Server) handleFeedsPOST(w http.ResponseWriter, r *http.Request) {
	back := "/blocklist#feeds"
	if err := r.ParseForm(); err != nil {
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	configured := func(n int) bool { _, ok := s.feedStore.ownFeed(n); return ok }
	next, refused := stagedFeeds(state.Staged.Feeds, r.PostForm["feed"], r.PostForm["confirm"], configured)
	if refused != "" {
		slog.Info("feeds not staged", "reason", refused)
		s.setFlash(w, r, refused)
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err := s.client.SaveRules("feeds", next); err != nil {
		slog.Warn("save feeds error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	// After the save, so the runner's own GET_RULES already sees the id
	// staged; and only for the new ones — refreshNow does nothing when the
	// core has a copy, but a toggle that stayed on is not a reason to ask.
	for _, id := range next {
		if !slices.Contains(state.Staged.Feeds, id) {
			s.refreshFeedNow(id)
		}
	}
	s.setFlash(w, r, "saved")
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// ownFeedErrKey names saveOwnFeed's refusal for the page.
func ownFeedErrKey(err error) string {
	switch {
	case errors.Is(err, errOwnFeedDemo):
		return "feeds_own_err_demo"
	case errors.Is(err, errOwnFeedName):
		return "feeds_own_err_name"
	case errors.Is(err, errOwnFeedURL):
		return "feeds_own_err_url"
	case errors.Is(err, errOwnFeedLogin):
		return "feeds_own_err_login"
	}
	return "save_error" // no such slot (a forged form), or the state file was not written
}

// handleOwnFeedPOST saves one own-feed slot. The password is write-only: an
// empty field keeps the stored one, and nothing here writes it back to a
// page or a log line.
func (s *Server) handleOwnFeedPOST(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.PostFormValue("n"))
	f := OwnFeed{
		Name: r.PostFormValue("name"), URL: r.PostFormValue("url"),
		User: r.PostFormValue("user"), Password: r.PostFormValue("password"),
	}
	if r.PostFormValue("clear") != "" {
		f = OwnFeed{}
	}
	err := s.saveOwnFeed(n, f)
	if err == nil {
		s.setFlash(w, r, "feeds_own_saved")
		http.Redirect(w, r, "/blocklist#own-feeds", http.StatusSeeOther)
		return
	}
	key := ownFeedErrKey(err)
	slog.Info("own feed not saved", "slot", n, "reason", key)
	state, gerr := s.client.GetRules()
	if key == "save_error" || key == "feeds_own_err_demo" || gerr != nil {
		s.setFlash(w, r, key)
		http.Redirect(w, r, "/blocklist#own-feeds", http.StatusSeeOther)
		return
	}
	// A field the operator can correct: the page again, with what they typed
	// in its slot and the reason beside it — never the password, which is
	// the one field not shown back (rejectIPList's reasoning).
	// A URL refused for carrying a user and password is shown back without
	// them: the password is the one thing never written into the page.
	if u, err := url.Parse(f.URL); err == nil && u.User != nil {
		u.User = nil
		f.URL = u.String()
	}
	card := s.feedsCard(state)
	for i := range card.Rows {
		if row := &card.Rows[i]; row.Own && row.OwnN == n {
			row.Name, row.URL, row.User = f.Name, f.URL, f.User
		}
	}
	card.OwnErrSlot, card.OwnErrKey = n, key
	s.setFlash(w, r, key)
	s.render(w, r, "blocklist.html", "blocklist", &ipListData{
		Title: "blocklist", Entries: state.Staged.Blocklist, Feeds: card,
	})
}

// feedLabel is what /blocked calls the feed a row names: the catalogue name,
// an own feed's configured name, or "Own feed N" in the page's language. An
// id no catalogue knows (a core newer than this process) is shown as it is.
func (s *Server) feedLabel(tFunc func(string, ...interface{}) string, id string) string {
	if n := ownFeedN(id); n > 0 {
		if f, ok := s.feedStore.ownFeed(n); ok {
			return f.Name
		}
		return tFunc("feeds_own_default", map[string]interface{}{"N": n})
	}
	return shared.FeedDisplayName(id)
}
