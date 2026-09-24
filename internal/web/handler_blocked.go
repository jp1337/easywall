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
	//
	// CoreErr and LoggingKnown make that message tri-state: unreadable is not
	// the same as false. GetPacketLog failing must not read as "nothing
	// refused", and GetOptions failing must not read as "logging is off" —
	// both are a fact the core did not answer, not a fact about the firewall.
	CoreErr      string    // GetPacketLog failed; nothing below except Filtered is known
	Logging      bool      // any of the ten switches is on; meaningful only when LoggingKnown
	LoggingKnown bool      // GetOptions answered
	Listening    bool      // the core holds its NFLOG group
	Filtered     bool      // a filter narrowed this view
	Since        time.Time // when the ring began

	// What a default-drop row's reason is read against: the rules the kernel
	// holds (Current), parsed once here rather than once per row (DropReason's
	// InAnyEntry calls used to reparse the blocklist, the allowlist and every
	// port rule's Sources for each on-screen row, every poll — see Task 8's
	// "Cost per poll"), and the network settings they were applied with.
	// WhyKnown is false when either could not be read, and then no row carries
	// a reason — none is better than one computed from a guess.
	Rules    shared.ParsedRules
	Net      shared.NetworkSettings
	WhyKnown bool
}

// rulesNow fills rows' reason inputs, and asks only when a default-drop row is
// on screen. The cost is two more socket round trips per five-second poll —
// GET_RULES and GET_APPLIED_CONFIG, each one small file the core reads from
// disk — on top of the two the tail already makes. The applied settings rather
// than GET_SETTINGS: a saved setting reaches the kernel at the next apply,
// and "now" means what the kernel holds. shared.ParseRules runs once here, not
// once per row: DropReasonParsed reads the parsed form.
func (s *Server) rulesNow(rows *blockedRows) {
	if !slices.ContainsFunc(rows.Entries, func(e shared.PacketLogEntry) bool { return e.Rule == "drop" }) {
		return
	}
	state, err := s.client.GetRules()
	if err != nil {
		slog.Debug("no drop reasons: the rules could not be read", "error", err)
		return
	}
	applied, err := s.client.GetAppliedConfig()
	if err != nil || !applied.Recorded {
		slog.Debug("no drop reasons: the applied settings are not known", "error", err)
		return
	}
	rows.Rules, rows.Net, rows.WhyKnown = shared.ParseRules(state.Current), applied.Config.Network, true
}

// blockedForm is what the filter form shows back: the raw fields as typed,
// independent of whether they parsed. A filter that could not be read must
// still appear in its field, or the operator cannot see what they typed to
// fix it.
type blockedForm struct{ Src, Dst, Port, Proto, Rule, InDev string }

type blockedData struct {
	Form     blockedForm
	Bad      string // the name of the first field that could not be read, empty when the filter is good
	Unread   bool   // a filter was given and could not be read, so none is applied
	Result   *shared.PacketLogResult
	Rows     blockedRows
	CoreErr  string
	Staged   int // changes waiting for Apply, counted exactly as /apply counts them
	RuleList []string
	Protos   []string
}

// blockedFilter reads the URL. The second value names the first field that
// could not be read ("src", "dst", "port", "proto", "rule", "in"), empty when
// the filter is good; the filter returned is then empty: a view the operator
// did not ask for is better than a partly applied one they cannot tell from
// the one they asked for.
func blockedFilter(q url.Values) (shared.PacketLogFilter, string) {
	f := shared.PacketLogFilter{
		Src:   strings.TrimSpace(q.Get("src")),
		Dst:   strings.TrimSpace(q.Get("dst")),
		Proto: q.Get("proto"),
		// A filter saved before 2.23 says rule=blacklist; read it as the rule's
		// name now, and the redirect in handleBlocked moves the URL on.
		Rule:  shared.CurrentListName(q.Get("rule")),
		InDev: strings.TrimSpace(q.Get("in")),
	}
	if p := strings.TrimSpace(q.Get("port")); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return shared.PacketLogFilter{}, "port"
		}
		f.Port = uint16(n)
	}
	// One field at a time, so the page can say which one it could not read.
	for _, c := range []struct {
		name string
		one  shared.PacketLogFilter
	}{
		{"src", shared.PacketLogFilter{Src: f.Src}},
		{"dst", shared.PacketLogFilter{Dst: f.Dst}},
		{"proto", shared.PacketLogFilter{Proto: f.Proto}},
		{"rule", shared.PacketLogFilter{Rule: f.Rule}},
		{"in", shared.PacketLogFilter{InDev: f.InDev}},
	} {
		if c.one.Validate() != nil {
			return shared.PacketLogFilter{}, c.name
		}
	}
	return f, ""
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
	q := r.URL.Query()
	f, bad := blockedFilter(q)
	// A readable filter lives at one URL. The GET form sends every field, the
	// empty ones too; redirect once to the query filterQuery would have built.
	// Compared re-encoded, not raw: html/template writes an IPv6 link as
	// %3a and url.Values.Encode as %3A, and those are the same query.
	if bad == "" && r.URL.RawQuery != "" && q.Encode() != filterQuery(f) {
		to := "/blocked"
		if enc := filterQuery(f); enc != "" {
			to += "?" + enc
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}

	data := &blockedData{
		Bad: bad, Unread: bad != "",
		Rows:     blockedRows{Query: filterQuery(f), Filtered: !f.IsZero()},
		RuleList: append(append([]string{}, shared.PacketLogRules...), shared.PacketLogRuleOther),
		Protos:   shared.PacketLogProtos,
	}
	if bad == "" {
		data.Form = blockedForm{Src: f.Src, Dst: f.Dst, Proto: f.Proto, Rule: f.Rule, InDev: f.InDev}
		if f.Port != 0 {
			data.Form.Port = strconv.Itoa(int(f.Port))
		}
	} else {
		data.Form = blockedForm{
			Src: strings.TrimSpace(q.Get("src")), Dst: strings.TrimSpace(q.Get("dst")),
			Port: strings.TrimSpace(q.Get("port")), Proto: q.Get("proto"), Rule: q.Get("rule"),
			InDev: strings.TrimSpace(q.Get("in")),
		}
	}

	res, err := s.client.GetPacketLog(f)
	if err != nil {
		slog.Warn("could not get the packet log", "error", err)
		data.CoreErr = err.Error()
		data.Rows.CoreErr = err.Error()
	} else {
		data.Result = res
		data.Rows.Entries = res.Entries
		data.Rows.Listening = res.Listening
		data.Rows.Since = res.Since
		s.rulesNow(&data.Rows)
	}
	// Only decides what the empty state and the header say. Unreadable, it
	// says nothing rather than something false.
	if opts, err := s.client.GetOptions(); err == nil {
		data.Rows.Logging = opts.LogsAnything()
		data.Rows.LoggingKnown = true
	}
	// The apply screen's own total, so the two pages cannot disagree about
	// how many changes are waiting. One number, one source: /apply renders
	// Total even when Incomplete is true (buildPreview already sets Total to
	// the rule count before it returns on an unreadable configuration half),
	// so /blocked must show the same number rather than hide it.
	data.Staged = s.buildPreview(r).Total
	s.render(w, r, "blocked.html", "blocked", data)
}

// handleBlockedRows is the live tail: the rows alone, polled every five
// seconds. A separate request from handleBlocked's, so it asks the core the
// same two questions again rather than inheriting an answer from a page load
// that may be minutes old.
func (s *Server) handleBlockedRows(w http.ResponseWriter, r *http.Request) {
	f, _ := blockedFilter(r.URL.Query()) // an unreadable filter here renders as no filter, exactly like the page
	rows := blockedRows{Entries: []shared.PacketLogEntry{}, Query: filterQuery(f), Filtered: !f.IsZero()}
	if res, err := s.client.GetPacketLog(f); err == nil {
		rows.Entries = res.Entries
		rows.Listening = res.Listening
		rows.Since = res.Since
		s.rulesNow(&rows)
	} else {
		slog.Debug("could not get the packet log for the live tail", "error", err)
		rows.CoreErr = err.Error()
	}
	if opts, err := s.client.GetOptions(); err == nil {
		rows.Logging = opts.LogsAnything()
		rows.LoggingKnown = true
	}
	s.renderPartial(w, r, "blocked_rows", rows)
}

// handleBlockedStage runs one row action and returns to the view it came from.
// The return URL is rebuilt from the validated filter, never copied from the
// form, so there is no redirect a request can steer.
func (s *Server) handleBlockedStage(w http.ResponseWriter, r *http.Request) {
	back := "/blocked"
	if q, err := url.ParseQuery(r.FormValue("q")); err == nil {
		if f, bad := blockedFilter(q); bad == "" {
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
// described, the same entry the blocklist page's own Save produces.
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

	// A /blocked page left open across the upgrade posts the old list name.
	switch act := shared.CurrentListName(r.FormValue("act")); act {
	case "allowlist", "blocklist":
		// One address, never a network: a network is a decision, and the list
		// page is where it is typed. Unmapped and unzoned before anything reads
		// it, so the guard and the kernel see the same spelling.
		addr, err := netip.ParseAddr(strings.TrimSpace(r.FormValue("addr")))
		if err != nil {
			return "blocked_refused_invalid"
		}
		entry := addr.Unmap().WithZone("").String()
		list := &next.Allowlist
		if act == "blocklist" {
			list = &next.Blocklist
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
// the one thing reachVerdict cannot: whether the TCP peer is being blocklisted.
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
	if shared.InAnyEntry(peer, after.Blocklist) && !shared.InAnyEntry(peer, before.Blocklist) {
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

// blockedRuleOption is the /options card behind each rule a /blocked row can
// name, by the card's toml key — its anchor is opt-<key> (options.html). Every
// rule in shared.PacketLogRules has an entry, and an empty one is a decision:
// the blocklist and the final drop are not switches, so no card refused those
// packets. TestEveryBlockedRuleLeadsToItsOption holds both halves.
var blockedRuleOption = map[string]string{
	"ssh":        "ssh_brute_force",
	"icmp_flood": "icmp_flood",
	"syn_flood":  "syn_flood",
	"tcp_rst":    "tcp_rst_flood",
	"portscan":   "port_scan",
	"invalid":    "drop_invalid_packets",
	"fragment":   "drop_fragments",
	"bogon":      "bogon_filter",
	"blocklist":  "",
	"drop":       "",
}
