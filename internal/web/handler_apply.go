package web

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// previewSetOrder is the order the rule sets appear in on the apply screen. The
// sidebar's order, so the page reads the way the navigation is organised rather
// than the way a struct happens to be declared.
var previewSetOrder = []string{"tcp", "udp", "blocklist", "allowlist", "feeds", "forwarding", "custom"}

type applyData struct {
	Status  *shared.FirewallStatus
	CoreErr string

	// Preview is non-nil only when a window is closed and something is staged.
	// While a window is open the change is already live and a preview of it is
	// history; with nothing staged there is nothing to preview.
	Preview *applyPreview

	// LiveCount is how many rule changes went in with the apply whose window is
	// open. Counted from Backup against Current, which is exactly what was
	// promoted and not yet confirmed.
	LiveCount int

	// Live is what the open window is holding, grouped the way the preview is.
	// The preview is nil while a window is open — a preview of what is already
	// live is history — but the operator is being told to check that their
	// services still answer, and the count alone does not name a port.
	Live []applyPreviewSet
}

type applyPreview struct {
	Sets   []applyPreviewSet
	Config []shared.ConfigDelta
	Total  int

	// Verdict is nil only when Incomplete is also true — reachVerdict itself
	// never returns nil; the worst it reports is ReachUnknown /
	// ReasonNoAddress. A page that showed no verdict block used to mean
	// "nothing to warn about" on the one screen whose argument is that silence
	// is the defect, so a read failure has to say so rather than vanish.
	Verdict *applyVerdict

	// Incomplete is true when a read this preview depends on failed outright —
	// GetRules, GetOptions, or GetSettings — so the rule diff, the
	// configuration drift, or the verdict itself is simply missing rather than
	// clean. The template renders an alert whenever this is true, so an absent
	// section is never silent.
	Incomplete bool

	// Unrecorded is true on an installation that has not applied or restarted
	// since 2.10. The page says so once: the configuration that went into the
	// kernel was not recorded before this version, and the next apply records it.
	Unrecorded bool
}

type applyPreviewSet struct {
	Set    string
	Deltas []shared.RuleDelta
}

type applyVerdict struct {
	Verdict shared.ReachVerdict
	Reason  shared.ReachReason
	Addr    string
	Port    string
	// Feed names the feed that drops Addr, for ReasonInFeed: "your address
	// is in the feed Spamhaus DROP".
	Feed string
}

func (s *Server) handleApplyGET(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.GetStatus()
	if err != nil {
		slog.Warn("get status error", "error", err)
		s.render(w, r, "apply.html", "apply", &applyData{CoreErr: err.Error()})
		return
	}

	data := &applyData{Status: status}
	switch {
	case status.Acceptance == shared.AcceptancePending:
		data.Live, data.LiveCount = s.liveChanges(r)
	case status.HasPending:
		data.Preview = s.buildPreview(r)
	}
	s.render(w, r, "apply.html", "apply", data)
}

// liveChanges is what of the kernel's contents has not been confirmed. Backup is
// the set that was in force before this apply and Current is the set it
// promoted, so the difference between them is the change the open window is
// holding.
//
// Rules only, deliberately: there is no "Backup" for the configuration half.
// Firewall options and network settings live in one place, this daemon's
// config, and the apply that opened this window already overwrote it with the
// new values — the applied-config snapshot holds what just went in, not what
// came before it. So an apply that changed options only, with no rule diff at
// all, legitimately shows no live count here; it is not a bug in this
// function, there being nothing else here to count it against.
//
// Grouped by set in the sidebar's order, so it renders through the same .diff
// markup the preview uses. The heading is not the preview's — apply_live_title,
// "Live now — unconfirmed", against apply_preview_title, "What changes" — because
// the two say different things about the same rows.
func (s *Server) liveChanges(r *http.Request) ([]applyPreviewSet, int) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("could not read the rules to list what is live", "error", err)
		return nil, 0
	}
	deltas := shared.DiffRules(state.Backup, state.Current)
	s.ownFeedDiffLabels(r, deltas)

	var sets []applyPreviewSet
	for _, set := range previewSetOrder {
		var group []shared.RuleDelta
		for _, d := range deltas {
			if d.Set == set {
				group = append(group, d)
			}
		}
		if len(group) > 0 {
			sets = append(sets, applyPreviewSet{Set: set, Deltas: group})
		}
	}
	return sets, len(deltas)
}

// buildPreview assembles what the operator is about to do. Five reads over a
// local Unix socket, on a page that is opened deliberately rather than polled.
//
// A read that fails costs its own section and not the page: a preview missing
// the configuration drift is still worth more than an apply screen that will not
// render, and the sections that did load are the ones that are shown. But a
// failed read is never simply omitted — Incomplete is set so the template says
// so, because an apply screen that quietly drops the lockout verdict reads as
// "nothing to warn about".
func (s *Server) buildPreview(r *http.Request) *applyPreview {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("could not read the rules for the apply preview", "error", err)
		return &applyPreview{Incomplete: true}
	}

	p := &applyPreview{}
	deltas := shared.DiffRules(state.Current, state.Staged)
	s.ownFeedDiffLabels(r, deltas)
	for _, set := range previewSetOrder {
		var group []shared.RuleDelta
		for _, d := range deltas {
			if d.Set == set {
				group = append(group, d)
			}
		}
		if len(group) > 0 {
			p.Sets = append(p.Sets, applyPreviewSet{Set: set, Deltas: group})
		}
	}

	// The verdict and the configuration drift need different things, and they
	// fail separately on purpose. The verdict wants the staged rules and the live
	// configuration; the drift additionally wants the snapshot. An unreadable
	// snapshot — the core answers GET_APPLIED_CONFIG with an error rather than
	// with recorded:false — must therefore cost the Options section and nothing
	// else. Dropping the lockout warning because a bookkeeping file could not be
	// read would remove the one thing on this page that has to be there.
	opts, oErr := s.client.GetOptions()
	nets, nErr := s.client.GetSettings()
	if oErr != nil || nErr != nil {
		slog.Warn("the apply preview has no configuration half and no verdict",
			"options", oErr, "settings", nErr)
		p.Total = len(deltas)
		p.Incomplete = true
		return p
	}

	p.Verdict = s.reachVerdict(r, state.Staged, *opts, *nets)

	switch applied, err := s.client.GetAppliedConfig(); {
	case err != nil:
		// Not Unrecorded: that sentence says the configuration was never
		// recorded, and this is a snapshot that exists and could not be read. The
		// journal carries the fault; the page shows the rule diff and the verdict
		// rather than a claim it cannot support.
		slog.Warn("could not read the configuration that went into the kernel; "+
			"the preview lists rule changes only", "error", err)
	case applied.Recorded:
		p.Config = shared.DiffConfig(applied.Config, shared.AppliedConfig{
			Firewall: *opts, Network: *nets,
		})
	default:
		p.Unrecorded = true
	}

	p.Total = len(deltas) + len(p.Config)
	return p
}

// ownFeedDiffLabels replaces an own feed's diff label. shared.DiffRules calls
// shared.FeedDisplayName, which knows no locale or configured name and always
// renders the English "Own feed N" (carry-in, controller ruling: an own feed
// must never show that literal on a German page) — here it becomes the
// operator's own name when set, else the localized default, the same as the
// card and the /blocked chip.
func (s *Server) ownFeedDiffLabels(r *http.Request, deltas []shared.RuleDelta) {
	loc := NewLocalizer(s.bundle, r, s.cfg.Language)
	tFunc := func(id string, args ...interface{}) string { return T(loc, id, args...) }
	for i := range deltas {
		if deltas[i].Set == "feeds" && ownFeedN(deltas[i].Key) > 0 {
			deltas[i].Label = s.feedLabel(tFunc, deltas[i].Key)
		}
	}
}

// reachVerdict answers whether a new connection from the address this request
// came from would still reach this interface once the staged set is live.
//
// It is deliberately about a *new* connection. The one the operator is reading
// this on stays ESTABLISHED through an apply — flushing the table does not touch
// conntrack — so a page that goes on answering proves nothing, which is what the
// copy says and why the verdict is worth showing at all.
func (s *Server) reachVerdict(r *http.Request, staged shared.Rules,
	o shared.FirewallOptions, n shared.NetworkSettings) *applyVerdict {

	addr, port, proxied, fallback := s.requestAddrAndPort(r)
	if fallback != nil {
		return fallback
	}
	hits, unknown := s.feedHits(addr, staged)
	return s.verdictFor(r, staged, o, n, addr, port, proxied, hits, unknown)
}

// requestAddrAndPort resolves the request's address and this host's web port —
// the half of reachVerdict that costs no round trip to the core, so
// lockoutRefusal can do it once and ask s.feedHits once for both verdicts it
// compares, instead of paying for GET_FEEDS twice for the same address.
//
// verdict is non-nil only when resolution failed; addr, port and proxied are
// then meaningless and the caller must return it as-is.
func (s *Server) requestAddrAndPort(r *http.Request) (addr netip.Addr, port uint16, proxied bool, verdict *applyVerdict) {
	rawAddr, proxied := s.clientAddr(r)
	addr, err := netip.ParseAddr(rawAddr)
	if err != nil {
		// Not nil: reach_no_address is exactly the sentence for this, and it is
		// already labelled in both locales. A nil verdict here used to make the
		// whole block vanish, which reads as "nothing to warn about" — the one
		// claim this page must never make by omission.
		slog.Warn("cannot read the peer address, so the verdict is reach_no_address",
			"remote_addr", r.RemoteAddr, "error", err)
		return netip.Addr{}, 0, proxied, &applyVerdict{Verdict: shared.ReachUnknown, Reason: shared.ReasonNoAddress, Addr: rawAddr}
	}
	rawPort := s.webPort()
	parsedPort, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil {
		slog.Warn("cannot read the listening port, so the verdict is reach_no_address", "error", err)
		return netip.Addr{}, 0, proxied, &applyVerdict{Verdict: shared.ReachUnknown, Reason: shared.ReasonNoAddress, Addr: addr.String()}
	}
	return addr, uint16(parsedPort), proxied, nil
}

// verdictFor is reachVerdict's core, given the feed hits and unknown ids
// already fetched. Split out so lockoutRefusal can call it twice — once for
// the rules as they stand, once for what staging the row action would make
// them — after asking s.feedHits only once: the two calls are for the same
// address and (every caller today) the same staged.Feeds, so asking the core
// twice would be asking it the identical question twice.
func (s *Server) verdictFor(r *http.Request, staged shared.Rules, o shared.FirewallOptions, n shared.NetworkSettings,
	addr netip.Addr, port uint16, proxied bool, hits, unknown []string) *applyVerdict {

	local := addressIsLocal(addr)
	verdict, reason := shared.Reachable(staged, o, n, addr, port, proxied, local, hits)
	v := &applyVerdict{Verdict: verdict, Reason: reason, Addr: addr.String(), Port: strconv.FormatUint(uint64(port), 10)}
	if len(unknown) > 0 {
		// Not "in none of them": that is the one claim this cannot make about
		// a feed GET_FEEDS could not answer for, or one with no copy yet —
		// whose copy then loads with no acceptance window (rulings X4). If
		// being in all of them would change the answer, the answer is unknown;
		// if it would not, it stands.
		if worst, _ := shared.Reachable(staged, o, n, addr, port, proxied, local,
			append(slices.Clone(hits), unknown...)); worst != verdict {
			v.Verdict, v.Reason = shared.ReachUnknown, shared.ReasonFeedsUnreadable
		}
	}
	// Naming runs independently of the branch above: an unrelated feed with no
	// answer yet must not swallow the name of the one that already decided it
	// (Fix round 1, Finding 1) — a second staged feed answering "unknown" is
	// not a reason to leave the page saying "in the feed , which drops it".
	if v.Reason == shared.ReasonInFeed {
		// The first staged feed that holds it — the one whose rule comes first.
		loc := NewLocalizer(s.bundle, r, s.cfg.Language)
		tFunc := func(id string, args ...interface{}) string { return T(loc, id, args...) }
		for _, id := range staged.Feeds {
			if slices.Contains(hits, id) {
				v.Feed = s.feedLabel(tFunc, id)
				break
			}
		}
	}
	return v
}

// feedHits asks the core which stored copies hold addr (plan P9), and names
// the staged feeds it cannot answer for: every one when GET_FEEDS fails, and
// otherwise each with no stored copy. With no feed staged there is nothing to
// ask, and nothing to fail.
func (s *Server) feedHits(addr netip.Addr, staged shared.Rules) (hits, unknown []string) {
	if len(staged.Feeds) == 0 {
		return nil, nil
	}
	res, err := s.client.GetFeeds(addr.String())
	if err != nil {
		slog.Warn("cannot ask the core which feeds hold this address", "error", err)
		return nil, staged.Feeds
	}
	stored := map[string]bool{}
	for _, f := range res.Feeds {
		stored[f.ID] = f.Stored
		if f.ContainsAddr {
			hits = append(hits, f.ID)
		}
	}
	for _, id := range staged.Feeds {
		if !stored[id] {
			unknown = append(unknown, id)
		}
	}
	return hits, unknown
}

// addressIsLocal reports whether addr is one of the addresses this host holds.
//
// The kernel's first input rule accepts on the arrival interface — `iifname
// "lo"` — and a connection to any local address is routed over lo, so it is
// accepted before a rule is consulted. shared.Reachable cannot see that: it is
// given an address, not an interface, and 127.0.0.0/8 is only one spelling of
// "local". Without this an operator at the console, reaching the interface on
// the host's own LAN address, would be told their working connection is about
// to be cut.
func addressIsLocal(addr netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Not knowing is not the same as "not local", but the only thing this
		// costs is the loopback shortcut: the chain below still answers, and a
		// console-local operator gets the ordinary verdict rather than a wrong
		// one.
		slog.Warn("cannot read this host's own addresses; the apply verdict will "+
			"not recognise a connection from one of them", "error", err)
		return false
	}
	for _, a := range addrs {
		prefix, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if own, ok := netip.AddrFromSlice(prefix.IP); ok && own.Unmap() == addr {
			return true
		}
	}
	return false
}

// handleApplyStart triggers an asynchronous rule application on the core.
// The core applies rules and enters the acceptance window.
func (s *Server) handleApplyStart(w http.ResponseWriter, r *http.Request) {
	if err := s.client.ApplyRules(); err != nil {
		slog.Warn("apply rules error", "error", err)
		// The core refuses a second apply while a window is open, and that is
		// not a failure to report as one: it is the safety mechanism doing its
		// job. The page hides the Start button in that state, so getting here
		// means a second tab, a double submit, the back button — or a feed
		// refresh holding the slot for under a second, which the web process
		// cannot tell apart, so the copy covers both.
		flash := "apply_error"
		switch {
		case strings.Contains(err.Error(), shared.ErrApplyInProgressText):
			flash = "apply_already_running"
		// A human took the firewall down at the console. The web interface may
		// not be the thing that re-arms it — that refusal is core.ErrPanicEngaged,
		// not a bug — so the operator needs to be told why, not handed the same
		// text a broken socket would produce.
		case strings.Contains(err.Error(), shared.ErrPanicEngagedText):
			flash = "apply_panic_engaged"
		}
		s.setFlash(w, r, flash)
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/apply", http.StatusSeeOther)
}

// handleApplyConfirm sends the acceptance signal to the core.
// Must be called while the acceptance window is pending.
func (s *Server) handleApplyConfirm(w http.ResponseWriter, r *http.Request) {
	accepted, err := s.client.Accept()
	if err != nil {
		slog.Warn("accept error", "error", err)
		s.setFlash(w, r, "accept_error")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	// A confirmation that arrives after the window closed changes nothing: the
	// rules were rolled back when it expired. Saying "accepted and applied
	// successfully" here told the operator their change was live at the one
	// moment it was not, and sent them to the dashboard to admire it.
	if !accepted {
		slog.Info("confirmation arrived after the acceptance window closed")
		s.setFlash(w, r, "accept_too_late")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "rules_accepted")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleApplyRollback ends the open window now, instead of waiting it out.
//
// It grants nothing. What comes back is the last *confirmed* rule set — the
// state the operator already approved — and doing nothing for the rest of the
// window reaches the identical outcome. The button saves the wait; it is not a
// new capability, which is why it exists on the network-facing side while the
// panic banner deliberately carries no control at all. See
// docs-tech/threat-model.md.
func (s *Server) handleApplyRollback(w http.ResponseWriter, r *http.Request) {
	cancelled, err := s.client.CancelAcceptance()
	if err != nil {
		slog.Warn("rollback error", "error", err)
		s.setFlash(w, r, "rollback_error")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	// The window had already closed, so this undid nothing: the previous rules
	// came back on their own when it expired. Reporting a rollback here would
	// tell the operator they acted at the one moment they did not — the same
	// shape of untruth accept_too_late exists to prevent.
	if !cancelled {
		slog.Info("a rollback arrived after the acceptance window had closed")
		s.setFlash(w, r, "rollback_too_late")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "rules_rolled_back")
	http.Redirect(w, r, "/apply", http.StatusSeeOther)
}

// handleApplyStatus returns the current acceptance status as JSON for HTMX polling.
func (s *Server) handleApplyStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.GetStatus()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if encErr := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); encErr != nil {
			slog.Warn("encode status error response", "error", encErr)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(status); encErr != nil {
		slog.Warn("encode status response", "error", encErr)
	}
}
