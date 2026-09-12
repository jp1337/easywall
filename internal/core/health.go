package core

import (
	"log/slog"

	"github.com/jp1337/easywall/internal/shared"
)

// Whether the firewall is doing what every surface says it is doing.
//
// For five releases the input chain's `ct state established,related accept`
// matched no packet: the conntrack masks were byte-reversed, so the mask the
// kernel compared against was `0x02000000` where `0x00000002` was meant. The
// table read correctly in `nft list ruleset`, the daemon was running, the
// dashboard said "rules are live", and the stateful half of the input chain
// enforced nothing. Every signal easywall had was a signal about *intent* —
// a table exists, a daemon is up, a config was applied — and none of them was
// a signal about *effect*. This file is the difference.
//
// Three facts, evaluated in order, and deliberately not a composite boolean:
// the state is useless to an operator without the reason, and a boolean has
// nowhere to put one. Spec §2 draws the same machine as a flowchart.
//
// computeHealth is separated from Firewall.Health for one reason: the state
// machine is the part with the branches, and it is testable without a kernel,
// a file or a daemon. Health() is the gathering, which is not.

// healthInputs is everything the decision is made from. A plain struct, so the
// table-driven test states each combination as data rather than building a
// Firewall to reach a branch.
type healthInputs struct {
	// enforcing is NftablesManager.Enforcing: the table exists, it has an input
	// chain, and that chain has rules in it. An error reading the kernel is
	// reported as false — being unable to confirm that a firewall is up is not
	// the same as it being up.
	enforcing bool

	// panicEngaged is Firewall.PanicEngaged, which answers an unreadable marker
	// with "engaged". That is the safe direction here as well as at an apply: a
	// health check that cannot tell whether the machine was deliberately
	// unfiltered should name the louder of the two causes, not the blander one.
	//
	// It selects the *reason* and never the state, since 2026-09-09. It used to
	// select both — degraded, per spec §2 — and that could not happen: see
	// computeHealth's first branch for why, and for the ruling that replaced it.
	panicEngaged bool

	// establishedPkts is the input chain's `ct state established,related accept`
	// counter, and portPkts is the sum of every operator rule's. See
	// computeHealth for why the alarm needs both.
	establishedPkts uint64
	portPkts        uint64

	// stamp is what the self-test last proved, and against which version and
	// kernel. The zero value is "never recorded", which is what a fresh install
	// and an unreadable file both look like.
	stamp shared.SelftestStamp

	// buildFindings is len(NftablesManager.LastFindings): what layer B said
	// about the last table this daemon built. Zero is the healthy answer.
	//
	// The count, not the findings themselves. The text names a chain and a rule
	// index and this result is rendered by an unauthenticated endpoint; see
	// shared.HealthSelftest.
	buildFindings int
}

// computeHealth is the state machine. Order is the substance of it, and each
// step is asserted by a named subtest in TestHealthStateMachine.
func computeHealth(in healthInputs) shared.HealthResult {
	out := shared.HealthResult{Selftest: shared.HealthSelftest{
		Version: in.stamp.Version,
		Kernel:  in.stamp.Kernel,
		Result:  in.stamp.Result,
		At:      in.stamp.At,
	}}

	// Nothing else is worth evaluating against a kernel that is not carrying
	// the rules. A counter of zero on a table that does not exist is not a
	// signal about the stateful half, and a self-test that passed proves
	// something about a binary rather than about this machine right now.
	//
	// The reason is chosen *inside* this branch, and that is a correction to
	// this release's own spec rather than a deviation from it.
	//
	// §2 and §7 said panic mode reads `degraded`, and a `panicEngaged` check
	// used to sit after this one to produce it. It could never fire on a real
	// host. Firewall.Panic calls nft.Reset (restore.go:377), which recreates the
	// table with an empty input chain, and Enforcing() reports an empty input
	// chain as not enforcing — its own comment says so, because a chain with no
	// rules under a drop policy is not "live rules" either. So every panicked
	// machine returned here, at the first branch, and `panic` was unreachable
	// while three comments, a flowchart and a failure table all promised it.
	//
	// The ruling, made against the spec on 2026-09-09: `fail` with reason
	// `panic`. Not `degraded`. Under panic mode the machine is not filtering at
	// all, and `degraded` means "still filtering, something is off" — it would
	// understate, and understating the state of a firewall is the failure this
	// release exists to remove whichever direction it points in. What was
	// missing was never the state; it was the *cause*. `not_enforcing` says the
	// kernel is not carrying rules and stops there. `panic` says somebody
	// unfiltered this machine deliberately and it stays that way across a
	// reboot until they run `easywall-core resume` — Daemon.Start declines to
	// filter while the marker is on disk. Those are different things to whoever
	// is reading an alert at three in the morning.
	//
	// The divergence from `easywall-core status`, which exits 0 under panic,
	// gets wider rather than narrower: health now exits 2 where it exited 1.
	// That exit code was ruled on and is not being reopened — a machine
	// somebody chose to unfilter is in a state somebody chose, and a console
	// asking after *intent* is right to be quiet. A monitoring system asking
	// after *health* is asking a different question, and this is where the two
	// are allowed to differ. It still closes carried-forward.md's entry open
	// since 2.7 — "a forgotten panic mode is invisible to monitoring" — and it
	// closes it louder.
	//
	// PanicEngaged answers an unreadable marker with "engaged", which is the
	// safe direction here as it is at an apply: a health check that cannot tell
	// whether the machine was deliberately unfiltered should not report the
	// blander of the two reasons.
	if !in.enforcing {
		out.State, out.Reason = shared.HealthFail, shared.HealthReasonNotEnforcing
		if in.panicEngaged {
			out.Reason = shared.HealthReasonPanic
		}
		return out
	}

	// Panic mode with the table still filtering, which is the
	// panicLandedDuringWrite race and nothing else: the marker is on disk and
	// the teardown has not landed, or failed. Still `fail` — the machine is
	// recorded as unfiltered and the next boot will not re-arm it, so an
	// operator reading `degraded` here would be told the milder half of the
	// truth. The reason is the same one, because it is the same cause.
	//
	// Before the counter, because in this window the counters are whatever the
	// old table had: reporting "the stateful half matches nothing" about a
	// machine that is deliberately unfiltered would send whoever reads it
	// hunting a rule-building defect that is not there.
	if in.panicEngaged {
		out.State, out.Reason = shared.HealthFail, shared.HealthReasonPanic
		return out
	}

	// The signature of the byte-reversed-mask class, and the alarm needs both
	// halves of it.
	//
	// `established == 0` on a machine with no traffic is honest — that is what
	// an idle host looks like, and a one-sided condition would fire on every one
	// of them and be switched off within a week, taking the signal with it. The
	// defect's actual signature is `established == 0` *while* the port counters
	// have moved: a port accepted a SYN and no reply found a rule to match.
	// TestHealthIsOkOnASilentHost is the guard on the second half.
	//
	// What the second half costs, because a comment that only argues for its own
	// code is one a later reader stops trusting. `established == 0 && ports > 0`
	// is not *only* the reversed-mask signature. It also describes a port that
	// accepted packets whose flow never reached ESTABLISHED, and there are three
	// real shapes of that:
	//
	//   - an open port with no listener being scanned. The SYN is accepted, the
	//     kernel answers RST, the conntrack entry closes, and the next SYN is
	//     NEW again — so the port counter climbs and the established one never
	//     moves.
	//   - a write-only UDP receiver: syslog, netflow, a metrics collector. It
	//     is sent to and never replies, so nothing about it is ever a reply.
	//   - the interval between an apply's first inbound SYN and its ACK. Small,
	//     but a read landing inside it sees exactly the same two numbers.
	//
	// On a host with no other inbound TCP at all, any of the three reports
	// `degraded/stateful_dead` and sends whoever reads it hunting a
	// rule-building defect that is not there — the same harm the panic-before-
	// counter ordering above was chosen to avoid, in a release whose whole
	// subject is not making false statements about the firewall.
	//
	// The class is narrow, and this is the reason it is narrow rather than an
	// assurance that it is: all three need a host with inbound traffic and *no
	// completed inbound connection whatsoever*, and one completed inbound TCP
	// connection defuses all three at once. Asking `/healthz` is itself a
	// completed inbound TCP connection. So is an admin session. A host that
	// nobody monitors and nobody administers can produce this; a host that
	// anybody watches cannot, and the second kind is the kind that has somebody
	// to mislead.
	//
	// The tightening, if it ever proves necessary: require the condition to hold
	// across two consecutive reads, since all three shapes above are transient
	// and the defect is permanent — a byte-reversed mask is identical on every
	// start of the same binary. Deliberately not built. It needs state carried
	// between calls, and Health() is a pure read by design; that is a real cost
	// to pay against a class this narrow, and the day it is worth paying is the
	// day somebody reports the false positive.
	//
	// A counter read that failed leaves both sums at zero, so it reads as a
	// silent host and the state stays ok. That is the honest answer and not a
	// swallowed error: the signal is *unavailable*, which is different from
	// negative, and Health() logs the failure where it happens. An unreadable
	// counter must not manufacture an alarm about a firewall that may be fine.
	if in.establishedPkts == 0 && in.portPkts > 0 {
		out.State, out.Reason = shared.HealthDegraded, shared.HealthReasonStatefulDead
		return out
	}

	// Layer B found an expression it cannot believe in a rule that went to the
	// kernel anyway — checkBuilt never refuses a write, because a check reading
	// false on a configured host would quietly stop enforcing rules somebody
	// wrote. This is where that finding becomes visible instead.
	//
	// Ahead of the stamp because it is about *this* table: the stamp is a claim
	// about the binary, proven once per version and kernel, while a finding is a
	// claim about the ruleset now in the kernel.
	if in.buildFindings > 0 {
		out.State, out.Reason = shared.HealthDegraded, shared.HealthReasonBuildFindings
		return out
	}

	// The stamp is read last, and only `failed` counts against the state.
	//
	// `unprovable` is not degraded: being unable to prove something is not the
	// same as it being broken. It is also the *common* production state — on a
	// normal systemd installation the daemon holds CAP_NET_ADMIN and not
	// CAP_SYS_ADMIN, so it cannot build the namespace the proof needs, and
	// treating that as degraded would ship a release in which the ordinary
	// installation reports itself broken. An absent stamp says the same thing
	// with less information and is treated the same way.
	if in.stamp.Result == shared.SelftestFailed {
		out.State, out.Reason = shared.HealthDegraded, shared.HealthReasonSelftestFailed
		return out
	}

	out.State, out.Reason = shared.HealthOK, shared.HealthReasonHealthy
	return out
}

// Health is what GET_HEALTH answers with.
//
// Two netlink reads and one file read. Not cached: statusForRender's ~2 s TTL
// exists because the topbar countdown put a clock on every page, and nothing
// polls this on a page render — Docker asks every ten seconds and a monitoring
// system less often than that.
func (f *Firewall) Health() shared.HealthResult {
	counters, err := f.nft.RuleCounters()
	if err != nil {
		// Logged here rather than folded into the result, because the result has
		// nowhere honest to put it: see computeHealth's counter branch for why
		// both sums staying at zero is the right answer to an unavailable
		// signal, and why it must not become an alarm.
		slog.Warn("could not read the kernel counters for the health check; "+
			"the stateful-half signal is unavailable this time", "error", err)
	}

	// RuleCounters reports reserved ids and does not filter them — that is
	// deliberate, and this loop is the only consumer that needs one. The
	// established accept is the reserved id whose counter is the whole point;
	// any other reserved id is easywall's own accounting and belongs in neither
	// sum, or it would be indistinguishable from a port an operator opened.
	//
	// Only the input chain's copy of the established rule is tagged, and
	// RuleCounters reads the input chain only, so there is exactly one of it.
	// buildForwardChain adds an untagged copy to the forward chain specifically
	// so that a reader here cannot sum input and forward traffic into one
	// figure.
	var established, ports uint64
	for id, c := range counters {
		if id == ReservedIDEstablished {
			established = c.Packets
			continue
		}
		if IsReservedRuleID(id) {
			continue
		}
		ports += c.Packets
	}

	// NewFirewall always builds a StampStore, but the tests in this package
	// construct Firewall literals directly — daemon_test.go, restore_test.go and
	// firewall_integration_test.go each have a helper that fills in the three or
	// four fields it needs. A health check that nil-panicked in whichever of
	// those a later task reused would be a crash in the privileged process
	// reached by an unauthenticated endpoint, so the zero stamp stands in: it
	// says "the proof has not been recorded", which is what a missing file says
	// too and what StampStore.Read would have answered.
	stamp := shared.SelftestStamp{}
	if f.stamp != nil {
		stamp = f.stamp.Read()
	}

	return computeHealth(healthInputs{
		enforcing:       f.nft.Enforcing(),
		panicEngaged:    f.PanicEngaged(),
		establishedPkts: established,
		portPkts:        ports,
		stamp:           stamp,
		buildFindings:   len(f.nft.LastFindings()),
	})
}
