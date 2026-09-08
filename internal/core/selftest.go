package core

// Layer C: four claims about the table easywall builds, proven with a real
// packet against a real kernel, inside a namespace of this proof's own.
//
// It builds its own table in its own namespace — the peer's, reached through
// the descriptor Harness owns. The host's real table is never read and never
// written: a health check may not reconfigure the host it is checking, which is
// why spec §9 excludes a self-test against the live table.
//
// A false claim is a `failed` stamp, never an error and never a refusal to
// filter. Spec §7. The error return of a prover means one thing only — the
// harness could not carry a packet — and every error becomes `unprovable`,
// because a host that cannot be asked and a firewall that is wrong are
// opposite findings and conflating them would make the ordinary container read
// as a broken installation. That inversion is the exact defect this release
// exists to remove.
//
// # Which direction each claim is measured in, and why it has to differ
//
// The table lives in the peer's namespace, so the input chain under test is the
// peer's. Two probe directions reach it, and each claim needs a specific one:
//
//	claim 1  peer -> router   the reply to an outbound connection arrives at the
//	                          peer's input chain with an *ephemeral* destination
//	                          port. No port rule can match it, so the
//	                          established,related accept is the only rule in the
//	                          chain that can let it through. That uniqueness is
//	                          the whole test: measured the other way round, the
//	                          data packets of a connection to an open port carry
//	                          that port as their destination and `dport N accept`
//	                          would match them too — the mask could be reversed
//	                          and the test would stay green.
//	claims 2-4  router -> peer a *new* connection has to arrive at the peer for a
//	                          port rule or a blacklist entry to have anything to
//	                          say. Nothing listens inside the peer, which is what
//	                          makes the measurement sharp rather than a problem:
//	                          a SYN the chain accepts reaches the peer's TCP
//	                          stack and is answered with a RST — ECONNREFUSED,
//	                          in microseconds — while a SYN the chain drops
//	                          produces no packet at all and the dial times out.
//	                          "Refused" is therefore positive evidence that the
//	                          packet crossed the veth and cleared the chain.
//
// The consequence worth knowing about claim 1: its control case needs an
// inbound TCP connection to be accepted on *this* side, and on a host whose own
// easywall table is already live that SYN is dropped by the host's default
// policy. The proof then reports `unprovable`, which is honest and is why
// easywall-selftest.service runs `Before=easywall-core.service` — at boot there
// is no table in the way. Claims 2-4 need only that the peer's replies get back
// in, which any working table's established,related accept allows.
//
// No subprocess is started here. The only program this package execs is
// /proc/self/exe, in netns.go, and TestSelftestUsesNoExternalBinary parses this
// file to hold it to that.

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

const (
	// The router-side listener, for claim 1. Distinct from every port
	// nftables_forward_test.go and netns_integration_test.go bind, because the
	// whole integration suite runs in one namespace.
	selftestListenPort = 12231

	// Opened in the peer's table by claims 2, 3 and 4.
	selftestOpenPort = 12232

	// Never opened by anything. Claim 3's subject and claim 2's discriminator.
	selftestClosedPort = 12233

	// How long a probe waits. A packet the chain accepts is answered in
	// microseconds — a listener's SYN-ACK or the peer stack's RST — so this
	// bounds only the cases where the chain is dropping, and every claim that
	// asserts a drop pays it once. Two seconds rather than one because a
	// container under CPU contention can delay the *scheduling* of the reply,
	// and a false "dropped" would report a working firewall as broken.
	selftestProbeTimeout = 2 * time.Second
)

// RunSelftest proves the claims and returns what to record.
//
// It stops at the first claim it cannot settle. A stamp carries one outcome and
// one detail, so continuing past a false claim would only decide which of two
// failures to name — and the first one is the one that broke.
func RunSelftest() shared.SelftestStamp {
	stamp := shared.SelftestStamp{
		Version: shared.CurrentVersion,
		Kernel:  KernelRelease(),
		At:      time.Now().UTC(),
	}

	claims := []struct {
		name  string
		prove func() (bool, string, error)
	}{
		{"a reply on an established connection passes", proveEstablishedPasses},
		{"an open port accepts a connection", proveOpenPortAccepts},
		{"a closed port does not", proveClosedPortRefuses},
		{"a blacklisted address does not reach an open port", proveBlacklistWins},
	}

	for _, c := range claims {
		ok, detail, err := c.prove()
		if err != nil {
			// Every error, not only ErrNamespaceUnavailable. A prover's error
			// says the harness could not carry a packet, whatever stopped it —
			// a refused clone, a port already bound, a netlink write the kernel
			// would not take. None of those is evidence about the firewall, and
			// reporting any of them as `failed` would put a red state on a host
			// whose rules were never measured.
			stamp.Result = shared.SelftestUnprovable
			stamp.Detail = c.name + " — " + err.Error()
			return stamp
		}
		if !ok {
			stamp.Result = shared.SelftestFailed
			stamp.Detail = c.name + " — " + detail
			return stamp
		}
	}
	stamp.Result = shared.SelftestPassed
	return stamp
}

// proof is the fixture the four provers share: a namespace with a peer in it, a
// manager bound to that namespace, and a listener on this side.
//
// It holds the *Harness and never a copy of NetNSFd()'s value. After Close the
// descriptor is closed and the field is -1; a cached integer could hand setns a
// number the kernel has recycled onto an unrelated file.
type proof struct {
	h *Harness
	m *NftablesManager
}

// withProof builds the fixture, runs fn against it and tears everything down.
//
// One harness per prover, not one for all four. Each prover applies its own
// table and its own control table, and NewHarness is deliberately cheap enough
// for that — TestIntegration_HarnessCanBeBuiltTwice exists because building
// them back to back is the case that used to fail.
//
// fn returns the same triple its prover does: an outcome, a detail for a false
// claim, and an error for a harness that could not answer.
func withProof(fn func(p *proof) (bool, string, error)) (bool, string, error) {
	h, err := NewHarness()
	if err != nil {
		return false, "", err
	}
	defer h.Close()

	m, err := NewNftablesManagerInNamespace(h.NetNSFd())
	if err != nil {
		return false, "", err
	}

	// The listener only claim 1 dials, and it is built for every prover anyway:
	// one socket, and a branch here would be a second thing to keep in step
	// with the claims list.
	ln, err := net.Listen("tcp", net.JoinHostPort(h.RouterAddr().String(),
		strconv.Itoa(selftestListenPort)))
	if err != nil {
		return false, "", fmt.Errorf("listening on the router side: %w", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // ln.Close, from the defer above
			}
			_ = conn.Close()
		}
	}()

	return fn(&proof{h: h, m: m})
}

// apply writes one table into the peer's namespace. Nothing else on this host
// is touched: the manager's every netlink socket is opened on the peer's
// namespace descriptor.
func (p *proof) apply(rules shared.Rules, opts shared.FirewallOptions) error {
	if err := p.m.Apply(shared.RulesState{Current: rules}, opts, shared.NetworkSettings{}); err != nil {
		return fmt.Errorf("applying the proof's own table inside the namespace: %w", err)
	}
	return nil
}

// peerReachesRouter asks the peer to open a connection to the listener on this
// side and reports whether it completed.
//
// What this measures is the *peer's* input chain, not this side's: the
// handshake finishes only if the SYN-ACK coming back is let in, and its
// destination port is the peer's ephemeral one.
func (p *proof) peerReachesRouter() (bool, error) {
	return p.h.Dial(p.h.RouterAddr(), selftestListenPort, selftestProbeTimeout)
}

// inboundCrosses dials the peer from this side and reports whether the SYN got
// through the peer's input chain.
//
// Nothing listens in the peer's namespace, so there are exactly two outcomes
// worth reading and they are not ambiguous:
//
//	ECONNREFUSED  the SYN cleared the chain, reached the peer's TCP stack and
//	              was answered with a RST. Positive evidence that a packet
//	              crossed the veth — a harness that is not wired produces
//	              ENETUNREACH or nothing at all, never a refusal.
//	timeout       no answer came back, which for a dropping chain is the only
//	              thing it can produce.
//
// Anything else — unreachable, a bind failure, an ICMP error the kernel turned
// into EHOSTUNREACH — is the harness and not a verdict, so it is an error. A
// nil error would mean something in the namespace answered a SYN, which also
// means it crossed.
func (p *proof) inboundCrosses(port uint16) (bool, error) {
	d := net.Dialer{Timeout: selftestProbeTimeout}
	addr := net.JoinHostPort(p.h.PeerAddr().String(), strconv.Itoa(int(port)))
	conn, err := d.Dial("tcp", addr)
	if err == nil {
		_ = conn.Close()
		return true, nil
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true, nil
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return false, nil
	}
	return false, fmt.Errorf("dialling %s from the router side: %w", addr, err)
}

// proveEstablishedPasses is the reported defect, measured.
//
// The peer opens a connection to the listener on this side. The SYN-ACK coming
// back crosses the peer's input chain as an established packet, addressed to the
// peer's ephemeral source port — so no port rule can match it, and with the
// modules off and no rules configured the established,related accept is the
// only rule in the chain. Nothing else can produce a completed handshake, and
// with the mask byte-reversed the reply has no rule at all, the policy drops it
// and the peer's dial times out. That is what an operator sees as "the machine
// went away": no DNS, no apt update, and the open SSH session gone.
//
// The control runs first, against a namespace with no easywall table in it. A
// dial that fails is also what an unwired harness, a listener that never bound
// and a host firewall dropping the inbound SYN all look like, and none of those
// is a finding about the established rule.
func proveEstablishedPasses() (bool, string, error) {
	return withProof(func(p *proof) (bool, string, error) {
		open, err := p.peerReachesRouter()
		if err != nil {
			return false, "", err
		}
		if !open {
			return false, "", fmt.Errorf("control: with no table in the namespace the peer could not "+
				"reach the listener on %s:%d, so the harness is not carrying a packet and nothing "+
				"below would mean anything", p.h.RouterAddr(), selftestListenPort)
		}

		if err := p.apply(shared.Rules{}, shared.FirewallOptions{}); err != nil {
			return false, "", err
		}

		open, err = p.peerReachesRouter()
		if err != nil {
			return false, "", err
		}
		if !open {
			return false, "the same connection that succeeded a moment ago timed out with the table " +
				"applied: the reply had no rule to match, so ct state established,related accept " +
				"matched nothing", nil
		}
		return true, "", nil
	})
}

// proveOpenPortAccepts measures the port rule.
//
// A SYN from this side to a port the table opens must clear the peer's input
// chain, which the peer's stack then answers with a RST because nothing is
// listening. Two things could produce that refusal other than the port rule, so
// both are ruled out here rather than assumed:
//
//   - the harness not being in the way at all. The control runs first, with no
//     table in the namespace, where the default policy accepts and the refusal
//     is guaranteed.
//   - the chain accepting everything. A port no rule opens is dialled under the
//     very same table and must *not* be answered. With that pair in one table,
//     the difference between the two ports can only be the rule for one of them.
func proveOpenPortAccepts() (bool, string, error) {
	return withProof(func(p *proof) (bool, string, error) {
		crossed, err := p.inboundCrosses(selftestOpenPort)
		if err != nil {
			return false, "", err
		}
		if !crossed {
			return false, "", fmt.Errorf("control: with no table in the namespace a connection to "+
				"%s:%d was not answered at all, so the harness is not carrying an inbound packet",
				p.h.PeerAddr(), selftestOpenPort)
		}

		rules := shared.Rules{TCP: []shared.PortRule{{
			Port:        strconv.Itoa(selftestOpenPort),
			Description: "the self-test's open port",
		}}}
		if err := p.apply(rules, shared.FirewallOptions{}); err != nil {
			return false, "", err
		}

		crossed, err = p.inboundCrosses(selftestOpenPort)
		if err != nil {
			return false, "", err
		}
		if !crossed {
			return false, fmt.Sprintf("a connection to open port %d was dropped: the rule that opens "+
				"it accepted nothing", selftestOpenPort), nil
		}

		crossed, err = p.inboundCrosses(selftestClosedPort)
		if err != nil {
			return false, "", err
		}
		if crossed {
			return false, fmt.Sprintf("port %d, which no rule opens, was answered under the same "+
				"table, so the accept on %d is the chain's disposition and not the port rule",
				selftestClosedPort, selftestOpenPort), nil
		}
		return true, "", nil
	})
}

// proveClosedPortRefuses measures the input chain's default policy.
//
// A port no rule opens must produce no answer whatsoever: the base chain's
// policy is drop, and with the modules off nothing else in the chain matches a
// new TCP connection to an arbitrary port.
//
// Silence proves that only if a refusal is producible at the same moment, which
// is what the control is for — the open port, under the very table being
// measured. Without it, an unwired harness, a dead peer and a chain that drops
// everything are one indistinguishable outcome, and this prover would report
// success on a namespace where nothing was ever sent.
func proveClosedPortRefuses() (bool, string, error) {
	return withProof(func(p *proof) (bool, string, error) {
		rules := shared.Rules{TCP: []shared.PortRule{{
			Port:        strconv.Itoa(selftestOpenPort),
			Description: "the self-test's open port",
		}}}
		if err := p.apply(rules, shared.FirewallOptions{}); err != nil {
			return false, "", err
		}

		crossed, err := p.inboundCrosses(selftestOpenPort)
		if err != nil {
			return false, "", err
		}
		if !crossed {
			return false, "", fmt.Errorf("control: the open port %d was not answered either, so a "+
				"silence on %d says nothing about the policy", selftestOpenPort, selftestClosedPort)
		}

		crossed, err = p.inboundCrosses(selftestClosedPort)
		if err != nil {
			return false, "", err
		}
		if crossed {
			return false, fmt.Sprintf("port %d, which no rule opens, answered a connection: the "+
				"input chain is not dropping what it was not told to accept", selftestClosedPort), nil
		}
		return true, "", nil
	})
}

// proveBlacklistWins measures the blacklist against an open port.
//
// Apply puts the blacklist ahead of the port rules, so a source on the list
// must not reach a port the table opens. The address on the list is this side's
// end of the veth, which is the source of every probe.
//
// The control is the same table minus the one blacklist entry, so the two runs
// differ by exactly one rule. Without it a drop would also be what a harness
// that stopped routing between the two applies looks like — and that is the
// confusion reach_integration_test.go:184's comment records, where the
// SSH brute-force chain's accept outranked the very list under test.
func proveBlacklistWins() (bool, string, error) {
	return withProof(func(p *proof) (bool, string, error) {
		open := shared.Rules{TCP: []shared.PortRule{{
			Port:        strconv.Itoa(selftestOpenPort),
			Description: "the self-test's open port",
		}}}
		if err := p.apply(open, shared.FirewallOptions{}); err != nil {
			return false, "", err
		}
		crossed, err := p.inboundCrosses(selftestOpenPort)
		if err != nil {
			return false, "", err
		}
		if !crossed {
			return false, "", fmt.Errorf("control: with %s not blacklisted, a connection to open "+
				"port %d was still not answered, so the drop below would not be the blacklist's doing",
				p.h.RouterAddr(), selftestOpenPort)
		}

		blocked := open
		blocked.Blacklist = []string{p.h.RouterAddr().String()}
		if err := p.apply(blocked, shared.FirewallOptions{}); err != nil {
			return false, "", err
		}
		crossed, err = p.inboundCrosses(selftestOpenPort)
		if err != nil {
			return false, "", err
		}
		if crossed {
			return false, fmt.Sprintf("%s is on the blacklist and still reached open port %d",
				p.h.RouterAddr(), selftestOpenPort), nil
		}
		return true, "", nil
	})
}
