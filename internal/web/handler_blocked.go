package web

import (
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
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

// handleBlockedStage runs one row action and returns to the view it came from.
// The return URL is rebuilt from the validated filter, never copied from the
// form, so there is no redirect a request can steer.
func (s *Server) handleBlockedStage(w http.ResponseWriter, r *http.Request) {
	back := "/blocked"
	if q, err := url.ParseQuery(r.FormValue("q")); err == nil {
		if f, ok := blockedFilter(q); ok {
			if enc := filterQuery(f); enc != "" {
				back += "?" + enc
			}
		}
	}
	s.setFlash(w, r, s.stageFromLog(r))
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// stageFromLog stages one of the three actions and returns the flash that says
// what happened. It stages; it never applies. Every other page saves and lets
// the apply screen apply, and a log page that armed the rollback timer on one
// click would be the only surprise in a product whose argument is that it has
// none.
//
// The audit entry is the core's: SAVE_RULES writes rules_saved with the change
// described, the same entry the blacklist page's own Save produces.
func (s *Server) stageFromLog(r *http.Request) string {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("could not read the rules to stage a row action", "error", err)
		return "save_error"
	}
	next := state.Staged
	var ruleType string
	var payload any
	var done string

	switch act := r.FormValue("act"); act {
	case "whitelist", "blacklist":
		// One address, never a network: a network is a decision, and the list
		// page is where it is typed. Unmapped and unzoned before anything reads
		// it, so the guard and the kernel see the same spelling.
		addr, err := netip.ParseAddr(strings.TrimSpace(r.FormValue("addr")))
		if err != nil {
			return "blocked_refused_invalid"
		}
		entry := addr.Unmap().WithZone("").String()
		list := &next.Whitelist
		if act == "blacklist" {
			list = &next.Blacklist
		}
		if shared.InAnyEntry(addr.Unmap().WithZone(""), *list) {
			return "blocked_refused_already"
		}
		*list = append(slices.Clone(*list), entry)
		ruleType, payload, done = act, *list, "blocked_staged_"+act

	case "open":
		proto := r.FormValue("proto")
		port, err := strconv.ParseUint(r.FormValue("port"), 10, 16)
		if (proto != "tcp" && proto != "udp") || err != nil || port == 0 {
			return "blocked_refused_invalid"
		}
		rules := &next.TCP
		if proto == "udp" {
			rules = &next.UDP
		}
		for _, pr := range *rules {
			if pr.FiltersHost() && len(pr.Sources) == 0 && shared.PortInRule(pr.Port, uint16(port)) {
				return "blocked_refused_already"
			}
		}
		loc := NewLocalizer(s.bundle, r, s.cfg.Language)
		*rules = append(slices.Clone(*rules), shared.PortRule{
			Port:        strconv.FormatUint(port, 10),
			Description: T(loc, "blocked_port_description"),
		})
		ruleType, payload, done = proto, *rules, "blocked_staged_port"

	default:
		return "blocked_refused_invalid"
	}

	if refusal := s.lockoutRefusal(r, state.Staged, next); refusal != "" {
		return refusal
	}
	if err := s.client.SaveRules(ruleType, payload); err != nil {
		slog.Warn("could not stage a row action", "type", ruleType, "error", err)
		return "save_error"
	}
	return done
}

// lockoutRefusal answers whether staging after instead of before would take
// the operator's way in away, and names why. Empty means go ahead.
//
// It asks reachVerdict — the function the apply screen asks — and not
// shared.Reachable, because reachVerdict is what resolves the operator's
// address through trusted_proxies and knows the address is local. Then it asks
// the one thing reachVerdict cannot: whether the TCP peer is being blacklisted.
// Behind a proxy the address in the log is the proxy's, and the verdict about
// the operator's own address stays open while everyone who comes through that
// proxy is cut off.
func (s *Server) lockoutRefusal(r *http.Request, before, after shared.Rules) string {
	opts, oErr := s.client.GetOptions()
	nets, nErr := s.client.GetSettings()
	if oErr != nil || nErr != nil {
		slog.Warn("cannot ask the lockout question, so the row action is not staged",
			"options", oErr, "settings", nErr)
		return "blocked_refused_unknown"
	}
	was := s.reachVerdict(r, before, *opts, *nets)
	now := s.reachVerdict(r, after, *opts, *nets)
	if now.Verdict == shared.ReachBlocked && was.Verdict != shared.ReachBlocked {
		return "blocked_refused_lockout"
	}

	peer, err := netip.ParseAddr(peerIP(r))
	if err != nil {
		return ""
	}
	peer = peer.Unmap().WithZone("")
	if shared.InAnyEntry(peer, after.Blacklist) && !shared.InAnyEntry(peer, before.Blacklist) {
		// "Proxy" only when the peer is a configured trusted proxy — with or
		// without a header naming a client behind it. Not clientAddr's proxied:
		// that is header presence, and any untrusted caller can send a header
		// about itself. "The client resolved past the peer" needs no second
		// test: resolveClient only reads the header from a trusted peer.
		if shared.InAnyEntry(peer, s.cfg.TrustedProxies) {
			return "blocked_refused_proxy"
		}
		return "blocked_refused_lockout"
	}
	return ""
}
