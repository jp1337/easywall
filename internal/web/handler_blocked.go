package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// /blocked: what the firewall refused. The web process asks the core, the way
// it asks for the audit log, and opens nothing itself.

type blockedRows struct {
	Entries []shared.PacketLogEntry
	// Query is the active filter, encoded. The live tail asks for the same
	// view, and every row action comes back to it. A plain string is right:
	// measured, html/template leaves an already-encoded query after "?" intact
	// and only turns & into &amp; for the attribute.
	Query string

	// The rest is what blocked_rows' own {{else}} needs to choose its empty
	// message. handleBlocked fills these from the same two calls it already
	// makes; handleBlockedRows — a separate request, on its own poll, handed
	// nothing from the page that opened it — makes them again so the live
	// tail's empty message matches the page's.
	Logging   bool      // any of the ten switches is on
	Listening bool      // the core holds its NFLOG group
	Filtered  bool      // a filter narrowed this view
	Since     time.Time // when the ring began
}

type blockedData struct {
	Filter   shared.PacketLogFilter
	Unread   bool // a filter was given and could not be read, so none is applied
	Result   *shared.PacketLogResult
	Rows     blockedRows
	CoreErr  string
	Logging  bool // any of the ten switches is on
	Staged   int  // rule changes waiting for Apply
	RuleList []string
	Protos   []string
}

// blockedFilter reads the URL. ok is false when anything given could not be
// read, and the filter returned is then empty: a view the operator did not ask
// for is better than a partly applied one they cannot tell from the one they
// asked for.
func blockedFilter(q url.Values) (shared.PacketLogFilter, bool) {
	f := shared.PacketLogFilter{
		Src:   strings.TrimSpace(q.Get("src")),
		Dst:   strings.TrimSpace(q.Get("dst")),
		Proto: q.Get("proto"),
		Rule:  q.Get("rule"),
		InDev: strings.TrimSpace(q.Get("in")),
	}
	if p := strings.TrimSpace(q.Get("port")); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return shared.PacketLogFilter{}, false
		}
		f.Port = uint16(n)
	}
	if f.Validate() != nil {
		return shared.PacketLogFilter{}, false
	}
	return f, true
}

// filterQuery encodes f for a URL. Built from the parsed filter, never from the
// raw query, so nothing the request carried reaches a link unvalidated.
func filterQuery(f shared.PacketLogFilter) string {
	v := url.Values{}
	for k, val := range map[string]string{"src": f.Src, "dst": f.Dst, "proto": f.Proto, "rule": f.Rule, "in": f.InDev} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if f.Port != 0 {
		v.Set("port", strconv.Itoa(int(f.Port)))
	}
	return v.Encode()
}

func (s *Server) handleBlocked(w http.ResponseWriter, r *http.Request) {
	f, ok := blockedFilter(r.URL.Query())
	data := &blockedData{
		Filter: f, Unread: !ok && r.URL.RawQuery != "",
		Rows:     blockedRows{Query: filterQuery(f), Filtered: !f.IsZero()},
		RuleList: append(append([]string{}, shared.PacketLogRules...), shared.PacketLogRuleOther),
		Protos:   shared.PacketLogProtos,
	}

	res, err := s.client.GetPacketLog(f)
	if err != nil {
		slog.Warn("could not get the packet log", "error", err)
		data.CoreErr = err.Error()
	} else {
		data.Result = res
		data.Rows.Entries = res.Entries
		data.Rows.Listening = res.Listening
		data.Rows.Since = res.Since
	}
	// Both only decide what the empty state and the header say. Unreadable,
	// they say nothing rather than something false.
	if opts, err := s.client.GetOptions(); err == nil {
		data.Logging = opts.LogsAnything()
		data.Rows.Logging = data.Logging
	}
	if state, err := s.client.GetRules(); err == nil {
		data.Staged = len(shared.DiffRules(state.Current, state.Staged))
	}
	s.render(w, r, "blocked.html", "blocked", data)
}

// handleBlockedRows is the live tail: the rows alone, polled every five
// seconds. A separate request from handleBlocked's, so it asks the core the
// same two questions again rather than inheriting an answer from a page load
// that may be minutes old.
func (s *Server) handleBlockedRows(w http.ResponseWriter, r *http.Request) {
	f, _ := blockedFilter(r.URL.Query())
	rows := blockedRows{Entries: []shared.PacketLogEntry{}, Query: filterQuery(f), Filtered: !f.IsZero()}
	if res, err := s.client.GetPacketLog(f); err == nil {
		rows.Entries = res.Entries
		rows.Listening = res.Listening
		rows.Since = res.Since
	} else {
		slog.Debug("could not get the packet log for the live tail", "error", err)
	}
	if opts, err := s.client.GetOptions(); err == nil {
		rows.Logging = opts.LogsAnything()
	}
	s.renderPartial(w, r, "blocked_rows", rows)
}
