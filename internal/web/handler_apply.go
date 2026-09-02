package web

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// previewSetOrder is the order the rule sets appear in on the apply screen. The
// sidebar's order, so the page reads the way the navigation is organised rather
// than the way a struct happens to be declared.
var previewSetOrder = []string{"tcp", "udp", "blacklist", "whitelist", "forwarding", "custom"}

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
		data.LiveCount = s.liveChangeCount()
	case status.HasPending:
		data.Preview = s.buildPreview(r)
	}
	s.render(w, r, "apply.html", "apply", data)
}

// liveChangeCount is how much of what is in the kernel has not been confirmed.
// Backup is the set that was in force before this apply and Current is the set it
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
func (s *Server) liveChangeCount() int {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("could not read the rules to count what is live", "error", err)
		return 0
	}
	return len(shared.DiffRules(state.Backup, state.Current))
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

// reachVerdict answers whether a new connection from the address this request
// came from would still reach this interface once the staged set is live.
//
// It is deliberately about a *new* connection. The one the operator is reading
// this on stays ESTABLISHED through an apply — flushing the table does not touch
// conntrack — so a page that goes on answering proves nothing, which is what the
// copy says and why the verdict is worth showing at all.
func (s *Server) reachVerdict(r *http.Request, staged shared.Rules,
	o shared.FirewallOptions, n shared.NetworkSettings) *applyVerdict {

	rawAddr, proxied := s.clientAddr(r)
	addr, err := netip.ParseAddr(rawAddr)
	if err != nil {
		// Not nil: reach_no_address is exactly the sentence for this, and it is
		// already labelled in both locales. A nil verdict here used to make the
		// whole block vanish, which reads as "nothing to warn about" — the one
		// claim this page must never make by omission.
		slog.Warn("cannot read the peer address, so the verdict is reach_no_address",
			"remote_addr", r.RemoteAddr, "error", err)
		return &applyVerdict{Verdict: shared.ReachUnknown, Reason: shared.ReasonNoAddress, Addr: rawAddr}
	}
	rawPort := s.webPort()
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil {
		slog.Warn("cannot read the listening port, so the verdict is reach_no_address", "error", err)
		return &applyVerdict{Verdict: shared.ReachUnknown, Reason: shared.ReasonNoAddress, Addr: addr.String()}
	}

	verdict, reason := shared.Reachable(staged, o, n, addr, uint16(port),
		proxied, addressIsLocal(addr))
	return &applyVerdict{
		Verdict: verdict,
		Reason:  reason,
		Addr:    addr.String(),
		Port:    strconv.FormatUint(port, 10),
	}
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
		// job, and the operator's next move is to confirm the apply they already
		// started. The page hides the Start button in that state, so getting
		// here means a second tab, a double submit, or the back button.
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
