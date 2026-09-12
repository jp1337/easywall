package core

// The harness the self-test measures in.
//
// A second namespace is not optional. Local delivery to any of this host's own
// addresses goes out lo, and the input chain accepts "iif lo" immediately — a
// probe against the host's own address proves nothing, which
// reach_integration_test.go already records as the one row of the chain it
// structurally cannot exercise. The packet has to arrive on a non-loopback
// interface, and a veth pair into a fresh namespace is the smallest thing that
// arranges it.
//
// A *third* namespace is optional, and AddContainerLeg builds it: a packet
// addressed to the peer is delivered locally there and never reaches the
// forward hook, so the rules 2.19 added to the forward chain cannot be
// measured with two namespaces either. The container sits behind the peer,
// inside a subnet of the same range, and the peer routes between them.
//
// Everything here is netlink and pipes — plus one stdlib read of this host's
// own interface list, in harnessCollision, which touches nothing privileged
// and is why it is allowed. The only program this file execs is
// /proc/self/exe, pinned by TestSelftestUsesNoExternalBinary, because the
// alternative — unshare, nsenter, ip, ping, bash, timeout, which is what the
// veth harness in nftables_forward_test.go uses — would put six external
// binaries inside the process holding CAP_NET_ADMIN to buy a check that runs
// once per upgrade. Six binaries are unremarkable in a test file and
// unacceptable in the privileged path.
//
// The pipe protocol, one line each way, so a hung peer is a read deadline
// rather than a parser:
//
//	child -> parent   "ready\n"                     once, after clone+exec
//	parent -> child   "dial 10.77.9.1 12227 2000\n" address, port, milliseconds
//	child -> parent   "open\n" | "blocked\n"        the verdict
//	child -> parent   "unparsed\n"                  not a verdict; see below
//	child -> parent   "failed <reason>\n"           not a verdict: the dial
//	                                                could not be made at all
//	parent -> child   "route ewst-p ewst-b\n"       forward, and proxy-ARP these
//	child -> parent   "routing\n" | "failed <r>\n"  see peerRoute
//	parent -> child   (pipe closed)                 child exits
//
// "unparsed" exists because "I could not read your line" and "the firewall
// dropped it" must not be the same word. It is unreachable today — the parent
// is the only writer and always formats a well-formed line — but a release
// whose subject is not lying about the firewall's state cannot have a parse
// failure spelling itself as a verdict. Dial's default branch turns it into an
// error, which is what it is.
//
// "failed" exists for the same reason one layer down, and it is reachable.
// Every dial error used to come back as "blocked", so a harness that broke
// between a claim's control and its measurement recorded `failed` against a
// working firewall — the one inversion foldClaims forbids. A timeout or a
// refusal both stay "blocked": a dropping chain can produce nothing but a
// timeout, and a refusal means a TCP stack answered the SYN and declined it —
// both are the negative answer to "is the port open", not a harness fault. An
// unreachable network and an ICMP error are the harness, and they now say so.
// See peerVerdict, which asks inboundCrosses' question on this side and gets
// the same answer for a refusal: that side dials a namespace where nothing
// listens, so a RST is evidence a packet crossed; this side dials the
// harness's own bound listener, so a RST is the same evidence — a stack
// answered — and Dial reports it as "not open" rather than a fault.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// PeerEnvVar is set on the re-executed child. cmd/easywall-core checks it
// before flag parsing and runs RunPeer instead of the daemon; the integration
// test binary's TestMain checks it for the same reason, because under "go test"
// /proc/self/exe is the test binary and not the daemon.
const PeerEnvVar = "EASYWALL_SELFTEST_PEER"

// RunPeer is the child. It is already inside the new namespace when this runs —
// Cloneflags did that before exec — so it creates nothing and only dials what
// it is told to. The parent owns every netlink write, including the ones that
// land inside this namespace, which it reaches through /proc/<pid>/ns/net.
//
// The one thing the child does to its own namespace is peerRoute, because a
// sysctl is the one setting /proc/<pid>/ does not expose to the parent at all.
func RunPeer(stdin io.Reader, stdout io.Writer) int {
	if _, err := fmt.Fprintln(stdout, "ready"); err != nil {
		return 1
	}
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "route ") {
			_, _ = fmt.Fprintln(stdout, peerRoute(strings.Fields(line)[1:]))
			continue
		}
		var addr string
		var port, ms int
		if n, _ := fmt.Sscanf(line, "dial %s %d %d", &addr, &port, &ms); n != 3 {
			// Not "blocked": nothing was dialled, so there is no verdict to
			// report and the parent must not read one.
			_, _ = fmt.Fprintln(stdout, "unparsed")
			continue
		}
		d := net.Dialer{Timeout: time.Duration(ms) * time.Millisecond}
		conn, err := d.Dial("tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
		if err == nil {
			_ = conn.Close()
		}
		_, _ = fmt.Fprintln(stdout, peerVerdict(err))
	}
	return 0
}

// peerVerdict maps a dial error to the one word the parent reads.
//
// It answers the same question inboundCrosses asks on the other side of the
// wire — "did the SYN reach a TCP stack and get an answer" — and ECONNREFUSED
// answers yes on both sides. inboundCrosses dials a namespace where nothing
// listens, so a RST there is positive evidence a packet crossed. This side
// dials the harness's own bound listener, so a RST means the SYN reached that
// stack and was refused — which is a verdict too, just the negative one
// Dial's question ("is the port open") calls for. A refusal is therefore
// `blocked`, the same word a dropping chain produces, because to Dial's
// caller the two are indistinguishable: the port did not open.
//
// So: a timeout or a refusal is `blocked` — the two ways a dial can come back
// negative without the harness having failed. Everything else — an
// unreachable network, a bind failure, an ICMP error the kernel turned into
// EHOSTUNREACH — is the harness, because none of those mean a stack answered.
// Reporting any of them as a verdict lets a broken harness record `failed`
// against a working firewall, which foldClaims' own doc comment forbids.
//
// The reason travels back so the parent can say what happened rather than
// "not a verdict". It is flattened to one line because the pipe protocol is
// one line each way, by design: a hung peer is then a read deadline rather
// than a parser.
func peerVerdict(err error) string {
	if err == nil {
		return "open"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "blocked"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "blocked"
	}
	return "failed " + strings.Join(strings.Fields(err.Error()), " ")
}

// peerRoute turns this namespace into a router, which is what a container host
// is and what the forward chain needs before it is reached at all.
//
// It runs in the child rather than in the parent because /proc/sys/net is the
// *reading* task's network namespace: there is no /proc/<pid>/ path by which
// the parent could write the peer's ip_forward, and setns in a privileged
// daemon to write one byte would be a new mechanism for a sysctl.
//
// proxy_arp on both interfaces is what saves a route on either side. The
// harness's three namespaces all live inside 10.77.9.0/24 — the range
// harnessCollision already guards — with the bridge as the more specific
// 10.77.9.128/25 behind this namespace. Each outer namespace therefore has the
// far side on-link through its own connected route and ARPs for it, and this
// namespace answers because its route to the target leaves by the *other*
// interface. Nothing is added to the operator's routing table, on a host where
// the outer namespace is the real one.
//
// Names are refused unless they are bare interface names. The parent is the
// only writer on this pipe, but a path assembled from a pipe inside the
// process that holds CAP_NET_ADMIN does not get to rely on that.
func peerRoute(ifaces []string) string {
	if len(ifaces) == 0 {
		return "failed no interface was named"
	}
	paths := []string{"/proc/sys/net/ipv4/ip_forward"}
	for _, name := range ifaces {
		if name == "" || strings.ContainsAny(name, "/.") {
			return "failed " + name + " is not an interface name"
		}
		paths = append(paths, "/proc/sys/net/ipv4/conf/"+name+"/proxy_arp")
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("1"), 0o644); err != nil {
			return "failed " + strings.Join(strings.Fields(err.Error()), " ")
		}
	}
	return "routing"
}

// interpretPeerLine turns the peer's line into a verdict or an error.
//
// Split out of Harness.Dial so both ends of the protocol are testable without
// a namespace: the classification is the part that has been wrong, and it
// needs no kernel to check.
func interpretPeerLine(line string) (bool, error) {
	switch {
	case line == "open":
		return true, nil
	case line == "blocked":
		return false, nil
	case strings.HasPrefix(line, "failed "):
		return false, fmt.Errorf("the peer could not dial: %s", strings.TrimPrefix(line, "failed "))
	default:
		return false, fmt.Errorf("the peer answered %q, which is not a verdict", line)
	}
}

// ErrNamespaceUnavailable is the sentinel for "this host will not let us build
// the harness", which is a different thing from "the firewall is wrong".
//
// It is the normal answer in production: easywall-core.service grants
// CAP_NET_ADMIN and bounds the set to it, and CLONE_NEWNET needs CAP_SYS_ADMIN.
// In a container the capability is usually absent altogether. So callers report
// "unprovable" once and move on — never a panic, never a bare error string
// somebody has to match on.
var ErrNamespaceUnavailable = errors.New("a network namespace cannot be created here")

const (
	harnessRouterIf = "ewst-r"
	harnessPeerIf   = "ewst-p"
	harnessPrefix   = 24

	// The container leg, built only by AddContainerLeg. Both interfaces live
	// inside namespaces this harness owns and never appear in the outer one,
	// so neither needs a collision check.
	harnessBridgeIf     = "ewst-b"
	harnessContainerIf  = "ewst-c"
	harnessBridgePrefix = 25

	// harnessBridgeCIDR is what the self-test hands to docker.custom_networks:
	// the range the forward chain's deny is written over.
	harnessBridgeCIDR = "10.77.9.128/25"

	// vethInfoPeer is VETH_INFO_PEER from linux/veth.h. golang.org/x/sys/unix
	// exports every other constant this file needs but not this one, and adding
	// a dependency for a single 1 is not a trade worth making.
	vethInfoPeer = 1

	// How long the child gets to say "ready". It has already been cloned by the
	// time we read, so this only covers exec and one write; five seconds is
	// generous, and a hang here is a broken host rather than a slow one.
	harnessReadyTimeout = 5 * time.Second
)

var (
	harnessRouterAddr = netip.MustParseAddr("10.77.9.1")
	harnessPeerAddr   = netip.MustParseAddr("10.77.9.2")

	// The container leg. The bridge is a *subnet of the harness range* on
	// purpose, so that everything AddContainerLeg builds is already covered by
	// harnessCollision and no second range has to be checked — and so that a
	// production host never has a route or an address added outside the one
	// /24 this harness has always claimed. The prefixes are the second half of
	// that trick: /25 on the peer's side is more specific than its own
	// 10.77.9.0/24, so the peer routes .130 out of the bridge and .1 out of the
	// veth, while /24 on the container's side puts the outer namespace on-link
	// for it. peerRoute's proxy ARP supplies the rest.
	harnessBridgeAddr    = netip.MustParseAddr("10.77.9.129")
	harnessContainerAddr = netip.MustParseAddr("10.77.9.130")
)

// ErrHarnessRangeInUse is the sentinel for "this host already uses what the
// harness needs", which — like ErrNamespaceUnavailable — is a statement about
// the host and not about the firewall.
//
// Callers report "unprovable" with the detail, never "failed". A self-test
// that called an operator's own 10.77.9.0/24 LAN a broken firewall would be
// the exact inversion this release's peer-side fix also closes.
var ErrHarnessRangeInUse = errors.New("the self-test's address range or interface name is already in use on this host")

// harnessCollision reports what on this host stands in the harness's way, or
// "" when nothing does.
//
// Two things are checked and one deliberately is not:
//
//	the range   an interface other than ewst-r holding an address inside
//	            10.77.9.0/24 — an operator whose LAN is that range
//	ewst-p      a host interface with the peer's name, which would make
//	            createVethPair fail EEXIST rather than ErrNamespaceUnavailable
//	ewst-r      NOT checked. wire() deletes it unconditionally and must: a
//	            previous run's router end outlives its own namespace by about
//	            110 ms, and a check here would break the case that deletion
//	            was written for. See wire's own comment.
//
// The interface list and the address accessor are parameters so this is
// testable without touching the host.
func harnessCollision(ifaces []net.Interface, addrsOf func(net.Interface) ([]net.Addr, error)) (string, error) {
	harnessNet := netip.PrefixFrom(harnessRouterAddr, harnessPrefix).Masked()
	for _, iface := range ifaces {
		if iface.Name == harnessPeerIf {
			return fmt.Sprintf("a host interface is already called %s", harnessPeerIf), nil
		}
		if iface.Name == harnessRouterIf {
			continue
		}
		addrs, err := addrsOf(iface)
		if err != nil {
			// Not fatal: an interface that will not report its addresses is not
			// evidence of a collision, and refusing the self-test over it would
			// trade a false "unprovable" for a real measurement.
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			if harnessNet.Contains(ip.Unmap()) {
				return fmt.Sprintf("%s holds %s, inside the harness range %s",
					iface.Name, ip.Unmap(), harnessNet), nil
			}
		}
	}
	return "", nil
}

// peerProc is one child process in a network namespace of its own, the two
// pipes that reach it, and the descriptor that namespace is opened on.
//
// Two of them make the forward harness — the peer that holds the table under
// test and the container behind it — and everything both need is here rather
// than twice: the one-line protocol, the read deadline, the teardown.
type peerProc struct {
	child  *exec.Cmd
	stdin  *os.File
	out    *os.File // kept as the concrete type, for SetReadDeadline
	stdout *bufio.Scanner

	// netnsFd is /proc/<child pid>/ns/net. It stays open for the harness's
	// whole life because the self-test hands it to nftables.WithNetNSFd, which
	// dups nothing and expects the fd to still be there at Apply time.
	netnsFd int
}

// Harness is a peer process in its own network namespace, wired to this one by
// a veth pair — and, once AddContainerLeg has been called, a third namespace
// behind the peer that can only be reached by forwarding.
type Harness struct {
	*peerProc

	// container is the third namespace. Nil unless AddContainerLeg built it:
	// four of the five claims measure an input chain and pay nothing for it.
	container *peerProc

	// peerConn writes netlink inside the child's namespace. Opened on netnsFd,
	// closed by Close.
	peerConn *netlink.Conn
}

// startPeer clones and execs the peer, and hands back the parent's ends of its
// two pipes. On any error it has already closed everything it opened.
//
// A variable, so that a test can make the launch fail with no kernel, no
// capability and no TestMain of its own. The refusal path is the common one in
// production — easywall-core.service grants CAP_NET_ADMIN and bounds the set to
// it, and CLONE_NEWNET needs CAP_SYS_ADMIN, so on most installations this is
// the branch that runs. A branch that common, inside a privileged process,
// cannot rest on somebody having tried it by hand once.
//
// os.Pipe rather than cmd.StdinPipe, and *os.File rather than io.ReadCloser,
// because Dial sets a read deadline on the read end: a peer which died
// mid-handshake has to be an error and not a goroutine parked forever on a pipe
// nobody will ever write to again.
var startPeer = func() (*exec.Cmd, *os.File, *os.File, error) {
	childIn, parentIn, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("harness stdin pipe: %w", err)
	}
	parentOut, childOut, err := os.Pipe()
	if err != nil {
		_, _ = childIn.Close(), parentIn.Close()
		return nil, nil, nil, fmt.Errorf("harness stdout pipe: %w", err)
	}

	cmd := exec.Command("/proc/self/exe")
	cmd.Env = append(os.Environ(), PeerEnvVar+"=1")
	cmd.Stdin = childIn
	cmd.Stdout = childOut
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}

	startErr := cmd.Start()
	// The child's ends are closed in the parent either way: while we hold the
	// write end of its stdout, a dead child never produces EOF.
	_, _ = childIn.Close(), childOut.Close()
	if startErr != nil {
		_, _ = parentIn.Close(), parentOut.Close()
		return nil, nil, nil, startErr
	}
	return cmd, parentIn, parentOut, nil
}

// NewHarness starts a peer in a fresh network namespace and wires it to this one
// with a veth pair.
//
// Both ends sit in one /24, so adding the addresses installs the connected route
// on each side and no RTM_NEWROUTE is sent. A partially built harness is torn
// down here, so a caller that gets an error never has to call Close — and it
// gets a nil *Harness with that error, so there is nothing half-built to use by
// mistake.
func NewHarness() (*Harness, error) {
	p, err := newPeerProc()
	if err != nil {
		return nil, err
	}
	h := &Harness{peerProc: p}
	if err := h.wire(); err != nil {
		h.Close()
		return nil, err
	}
	return h, nil
}

// newPeerProc starts one child in a namespace of its own and waits for it to
// say so. Nothing is wired here: the caller decides what the namespace is for.
//
// A half-built peer is torn down before the error is returned, so a caller that
// gets one never has to close anything.
func newPeerProc() (*peerProc, error) {
	cmd, parentIn, parentOut, err := startPeer()
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL) {
			return nil, fmt.Errorf("%w: %v", ErrNamespaceUnavailable, err)
		}
		return nil, fmt.Errorf("starting the harness peer: %w", err)
	}

	p := &peerProc{
		child:   cmd,
		stdin:   parentIn,
		out:     parentOut,
		stdout:  bufio.NewScanner(parentOut),
		netnsFd: -1,
	}

	// Anything but "ready" is the same refusal as a start error that wraps
	// EPERM: the kernel let us clone and then something else stopped the peer
	// from running, and either way there is no harness to measure in.
	if line, err := p.readLine(harnessReadyTimeout); err != nil || line != "ready" {
		p.close()
		if err != nil {
			return nil, fmt.Errorf("%w: the peer never reported ready: %v",
				ErrNamespaceUnavailable, err)
		}
		return nil, fmt.Errorf("%w: the peer said %q, not \"ready\"",
			ErrNamespaceUnavailable, line)
	}

	fd, err := unix.Open("/proc/"+strconv.Itoa(cmd.Process.Pid)+"/ns/net",
		unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		p.close()
		return nil, fmt.Errorf("%w: opening the peer's netns: %v", ErrNamespaceUnavailable, err)
	}
	p.netnsFd = fd
	return p, nil
}

// wire builds the pair and configures both ends. Every netlink message is a
// named function, so each one can be read against ip-link(8) and
// ip-address(8) rather than against this sequence.
func (h *Harness) wire() error {
	pid := h.child.Process.Pid

	local, err := netlink.Dial(unix.NETLINK_ROUTE, nil)
	if err != nil {
		return fmt.Errorf("rtnetlink in this namespace: %w", err)
	}
	defer func() { _ = local.Close() }()

	// A leftover pair from an earlier harness goes first, because Close is not
	// the end of it: Close returns long before the kernel's cleanup_net
	// destroys the peer's namespace, so the previous router-side end is still
	// registered here — about 110 ms on an idle container, and longer under
	// load, since netns teardown is batched. Without this, a second NewHarness
	// in the same process fails with EEXIST out of createVethPair, which does
	// *not* satisfy errors.Is(err, ErrNamespaceUnavailable): the self-test
	// would report a broken harness instead of "unprovable", on a release whose
	// whole subject is not lying about the firewall's state.
	//
	// The side effect is worth knowing: this deletes any interface in this
	// namespace literally named ewst-r, whoever made it. Deleting one end of a
	// veth takes its peer with it, so one message covers the pair.
	//
	// Naming the links per-pid instead would not have been enough. The old
	// router link still holds 10.77.9.1/24, and addAddress is Create|Excl too,
	// so a uniquely named interface would simply collide one step later.

	// Before anything is created: does this host already use what the harness
	// needs? Nothing checked, and the failure was not a clean "unprovable" —
	// createVethPair's EEXIST does not satisfy errors.Is(err,
	// ErrNamespaceUnavailable), so an operator whose LAN is 10.77.9.0/24 was
	// told the harness was broken. Measured before this check: the ordinary
	// case was honest anyway, reporting unprovable with an accurate detail 2 of
	// 2 runs against a live host table — so this hardens an honest path rather
	// than fixing a dishonest one.
	ifaces, err := net.Interfaces()
	if err == nil { // a host that will not list its interfaces is not evidence of a collision
		if hit, cerr := harnessCollision(ifaces, func(i net.Interface) ([]net.Addr, error) {
			return i.Addrs()
		}); cerr == nil && hit != "" {
			return fmt.Errorf("%w: %s", ErrHarnessRangeInUse, hit)
		}
	}

	if err := deleteLink(local, harnessRouterIf); err != nil {
		return fmt.Errorf("clearing a leftover %s: %w", harnessRouterIf, err)
	}

	if err := createVethPair(local, harnessRouterIf, harnessPeerIf, pid); err != nil {
		return fmt.Errorf("creating the veth pair: %w", err)
	}

	// Only now, because the interface has to exist before anything in the
	// child's namespace can be addressed.
	h.peerConn, err = netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{NetNS: h.netnsFd})
	if err != nil {
		return fmt.Errorf("rtnetlink inside the peer's namespace: %w", err)
	}

	if err := addAddress(local, harnessRouterIf, harnessRouterAddr, harnessPrefix); err != nil {
		return fmt.Errorf("addressing the router side: %w", err)
	}
	if err := addAddress(h.peerConn, harnessPeerIf, harnessPeerAddr, harnessPrefix); err != nil {
		return fmt.Errorf("addressing the peer side: %w", err)
	}
	if err := setLinkUp(local, harnessRouterIf); err != nil {
		return fmt.Errorf("bringing the router side up: %w", err)
	}
	if err := setLinkUp(h.peerConn, harnessPeerIf); err != nil {
		return fmt.Errorf("bringing the peer side up: %w", err)
	}
	return nil
}

// Close kills the peer and waits for it.
//
// Nothing deletes the links. nftables_forward_test.go has to, because its
// router-side ends live in the namespace that outlives the leaf; here the
// router side *is* this namespace and the whole pair is destroyed when the
// peer's namespace dies with its process. Close is safe on a half-built
// harness, which is how NewHarness unwinds.
func (h *Harness) Close() {
	if h.peerConn != nil {
		_ = h.peerConn.Close()
		h.peerConn = nil
	}
	// The container before the peer: its veth pair lives in the peer's
	// namespace, which stops existing when the peer does.
	if h.container != nil {
		h.container.close()
		h.container = nil
	}
	if h.peerProc != nil {
		h.close() // the promoted peerProc.close, not this one
	}
}

// close tears down one child. Safe on a half-built one, which is how
// newPeerProc unwinds.
func (p *peerProc) close() {
	if p.netnsFd >= 0 {
		_ = unix.Close(p.netnsFd)
		p.netnsFd = -1
	}
	// Closing stdin is what tells a live peer to exit; the kill is for one that
	// is wedged somewhere else.
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.child != nil && p.child.Process != nil {
		_ = p.child.Process.Kill()
		_, _ = p.child.Process.Wait()
	}
	if p.out != nil {
		_ = p.out.Close()
	}
}

// RouterAddr is this namespace's end of the pair: the address a packet from the
// peer arrives at, over a real interface that is not lo.
func (h *Harness) RouterAddr() netip.Addr { return harnessRouterAddr }

// PeerAddr is the address the peer dials from.
func (h *Harness) PeerAddr() netip.Addr { return harnessPeerAddr }

// NetNSFd is /proc/<peer pid>/ns/net, opened and owned by the harness. The
// self-test hands it to nftables.WithNetNSFd so that a table written for the
// peer lands in the peer's namespace.
func (h *Harness) NetNSFd() int { return h.netnsFd }

// Dial asks the peer to open a TCP connection to addr:port and reports whether
// it succeeded within timeout.
//
// The read deadline is the dial timeout plus a margin: the peer's own dial is
// what should expire, and this only catches a peer that is gone. A false with a
// nil error is a verdict; an error means the harness itself failed and the
// verdict is unknown, which callers must not read as "blocked".
func (h *Harness) Dial(addr netip.Addr, port uint16, timeout time.Duration) (bool, error) {
	return h.dial(addr, port, timeout)
}

// DialFromContainer is the same question asked from behind the peer, and it is
// the one direction nothing else in this package can ask.
//
// A container's own outbound connection is what the forward chain's deny is
// most dangerous to: the reply comes back with the bridge address as its
// *destination* and a source outside it, which is the deny's exact shape. Only
// a connection opened from inside the container produces that packet, so only
// this call can prove the established accept is still in front of it.
func (h *Harness) DialFromContainer(addr netip.Addr, port uint16, timeout time.Duration) (bool, error) {
	if h.container == nil {
		return false, errors.New("there is no container leg; call AddContainerLeg first")
	}
	return h.container.dial(addr, port, timeout)
}

func (p *peerProc) dial(addr netip.Addr, port uint16, timeout time.Duration) (bool, error) {
	if _, err := fmt.Fprintf(p.stdin, "dial %s %d %d\n", addr, port, timeout.Milliseconds()); err != nil {
		return false, fmt.Errorf("asking the peer to dial: %w", err)
	}
	line, err := p.readLine(timeout + 2*time.Second)
	if err != nil {
		return false, fmt.Errorf("reading the peer's verdict: %w", err)
	}
	return interpretPeerLine(line)
}

func (p *peerProc) readLine(timeout time.Duration) (string, error) {
	if err := p.out.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("setting the peer read deadline: %w", err)
	}
	if !p.stdout.Scan() {
		if err := p.stdout.Err(); err != nil {
			return "", err
		}
		return "", errors.New("the peer closed its pipe")
	}
	return p.stdout.Text(), nil
}

// ContainerAddr is the address behind the peer: inside the bridge range, and
// reachable from here only if the peer's forward chain lets a packet through.
func (h *Harness) ContainerAddr() netip.Addr { return harnessContainerAddr }

// BridgeCIDR is the range to hand to docker.custom_networks, so that the table
// under test writes its deny over the network the container is actually on.
func (h *Harness) BridgeCIDR() string { return harnessBridgeCIDR }

// AddContainerLeg puts a third namespace behind the peer and makes the peer
// route between the two.
//
// This is what turns the harness from something that measures an *input* chain
// into something that measures a *forward* chain, and the distinction is the
// whole of 2.19: a packet addressed to this namespace never reaches the forward
// hook, so a two-namespace harness cannot say anything about the rules that
// decide a published container port. Traffic from the outer namespace to
// ContainerAddr is forwarded by the peer and nothing else — one hop, one
// verdict, no DNAT to arrange, because the address the deny matches is the
// container's own either way.
//
// Idempotent, so a prover that calls it twice gets one leg. Errors are the
// harness and never a finding: every one of them means this claim cannot be
// settled here.
func (h *Harness) AddContainerLeg() error {
	if h.container != nil {
		return nil
	}
	c, err := newPeerProc()
	if err != nil {
		return err
	}
	h.container = c // owned by Close from here, however the rest of this goes

	// Created from inside the peer's namespace, with the far end handed
	// straight to the container's — the same single message the outer pair
	// uses, one namespace further in.
	if err := createVethPair(h.peerConn, harnessBridgeIf, harnessContainerIf,
		c.child.Process.Pid); err != nil {
		return fmt.Errorf("creating the bridge pair inside the peer: %w", err)
	}

	cConn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{NetNS: c.netnsFd})
	if err != nil {
		return fmt.Errorf("rtnetlink inside the container's namespace: %w", err)
	}
	defer func() { _ = cConn.Close() }()

	// The two prefixes differ deliberately; see harnessBridgeAddr's comment.
	if err := addAddress(h.peerConn, harnessBridgeIf, harnessBridgeAddr, harnessBridgePrefix); err != nil {
		return fmt.Errorf("addressing the bridge side: %w", err)
	}
	if err := addAddress(cConn, harnessContainerIf, harnessContainerAddr, harnessPrefix); err != nil {
		return fmt.Errorf("addressing the container: %w", err)
	}
	if err := setLinkUp(h.peerConn, harnessBridgeIf); err != nil {
		return fmt.Errorf("bringing the bridge side up: %w", err)
	}
	if err := setLinkUp(cConn, harnessContainerIf); err != nil {
		return fmt.Errorf("bringing the container up: %w", err)
	}

	if _, err := fmt.Fprintf(h.stdin, "route %s %s\n", harnessPeerIf, harnessBridgeIf); err != nil {
		return fmt.Errorf("asking the peer to route: %w", err)
	}
	line, err := h.readLine(harnessReadyTimeout)
	if err != nil {
		return fmt.Errorf("reading the peer's answer to route: %w", err)
	}
	if line != "routing" {
		return fmt.Errorf("%w: the peer could not become a router: %s",
			ErrNamespaceUnavailable, strings.TrimPrefix(line, "failed "))
	}
	return nil
}

// createVethPair sends one RTM_NEWLINK carrying the whole pair, with the peer's
// namespace named by pid.
//
// One message rather than a create-then-move pair: a move is a second syscall
// that can fail with the peer already visible in this namespace, and the kernel
// has accepted IFLA_NET_NS_PID inside VETH_INFO_PEER since veth existed.
func createVethPair(c *netlink.Conn, routerIf, peerIf string, peerPID int) error {
	// The bound is not defensive noise. IFLA_NET_NS_PID names the namespace one
	// end of this pair is moved into, this runs inside the process holding
	// CAP_NET_ADMIN, and peerPID is an int that arrives from
	// cmd.Process.Pid — which is -1 once the process has been waited for, and 0
	// on a Cmd that was never started. A plain uint32(peerPID) turns -1 into
	// 4294967295 and hands the kernel a namespace nobody asked for, silently.
	// gosec reported that as G115 on the conversion below, and it is the rare
	// G115 that names a real bug rather than a width complaint: .golangci.yml
	// excludes the rule, so only the standalone gosec in security.yml ever said
	// so, and only through code scanning.
	//
	// 1<<22 is Linux's PID_MAX_LIMIT on 64-bit: pid_max cannot be raised past
	// it, so no live process can carry a larger pid and this ceiling cannot
	// refuse a valid peer. That matters more than the width: a check that
	// rejected a good pid would make every self-test report "unprovable", which
	// is the failure mode this release exists to remove.
	if peerPID <= 0 || peerPID > 1<<22 {
		return fmt.Errorf("the peer's pid is %d, which cannot name a namespace", peerPID)
	}
	peer := netlink.NewAttributeEncoder()
	peer.String(unix.IFLA_IFNAME, peerIf)
	peer.Uint32(unix.IFLA_NET_NS_PID, uint32(peerPID))
	peerAttrs, err := peer.Encode()
	if err != nil {
		return fmt.Errorf("encoding the peer's attributes: %w", err)
	}

	ae := netlink.NewAttributeEncoder()
	ae.String(unix.IFLA_IFNAME, routerIf)
	ae.Nested(unix.IFLA_LINKINFO, func(nae *netlink.AttributeEncoder) error {
		nae.String(unix.IFLA_INFO_KIND, "veth")
		nae.Nested(unix.IFLA_INFO_DATA, func(dae *netlink.AttributeEncoder) error {
			// Do, not Nested: VETH_INFO_PEER is the one attribute here whose
			// payload is not purely attributes. rtnl_nla_parse_ifinfomsg skips
			// a struct ifinfomsg before it starts parsing, so the peer's
			// attributes have to sit behind sixteen zero bytes — which is
			// exactly what iproute2's iplink_veth.c reserves before it writes
			// the peer's own arguments.
			dae.Do(vethInfoPeer, func() ([]byte, error) {
				return append(ifInfomsg(0, 0), peerAttrs...), nil
			})
			return nil
		})
		return nil
	})
	attrs, err := ae.Encode()
	if err != nil {
		return fmt.Errorf("encoding the link attributes: %w", err)
	}

	_, err = c.Execute(netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.RTM_NEWLINK),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create | netlink.Excl,
		},
		Data: append(ifInfomsg(0, 0), attrs...),
	})
	return err
}

// addAddress sends RTM_NEWADDR for one interface. c decides which namespace: the
// caller passes this namespace's connection or the peer's.
//
// ifaddrmsg has no name field — inet_rtm_newaddr looks the device up by index,
// and IFA_LABEL only names an alias — so this resolves the index first, in
// whichever namespace c is bound to. net.InterfaceByName would answer for this
// process's namespace and be wrong for half the calls.
func addAddress(c *netlink.Conn, ifName string, addr netip.Addr, prefix uint8) error {
	idx, err := linkIndex(c, ifName)
	if err != nil {
		return err
	}

	v4 := addr.As4()
	ae := netlink.NewAttributeEncoder()
	// Both, as ip-address(8) sends both: on a broadcast interface IFA_ADDRESS
	// is the local address too, and omitting IFA_LOCAL makes the kernel read
	// IFA_ADDRESS as a point-to-point peer instead.
	ae.Bytes(unix.IFA_LOCAL, v4[:])
	ae.Bytes(unix.IFA_ADDRESS, v4[:])
	attrs, err := ae.Encode()
	if err != nil {
		return fmt.Errorf("encoding the address attributes for %s: %w", ifName, err)
	}

	body := make([]byte, unix.SizeofIfAddrmsg)
	body[0] = unix.AF_INET
	body[1] = prefix
	binary.NativeEndian.PutUint32(body[4:8], idx)

	_, err = c.Execute(netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.RTM_NEWADDR),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create | netlink.Excl,
		},
		Data: append(body, attrs...),
	})
	if err != nil {
		return fmt.Errorf("RTM_NEWADDR %s on %s: %w", addr, ifName, err)
	}
	return nil
}

// deleteLink sends RTM_DELLINK for one interface, by name, and reports "there
// was nothing to delete" as success — which is the ordinary case, the first
// harness in a process.
func deleteLink(c *netlink.Conn, ifName string) error {
	ae := netlink.NewAttributeEncoder()
	ae.String(unix.IFLA_IFNAME, ifName)
	attrs, err := ae.Encode()
	if err != nil {
		return fmt.Errorf("encoding the delete for %s: %w", ifName, err)
	}
	_, err = c.Execute(netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.RTM_DELLINK),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append(ifInfomsg(0, 0), attrs...),
	})
	// netlink.OpError unwraps to the raw unix.Errno, so errors.Is reaches it
	// through the wrapping.
	if errors.Is(err, unix.ENODEV) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("RTM_DELLINK %s: %w", ifName, err)
	}
	return nil
}

// setLinkUp sends RTM_NEWLINK with IFF_UP set and only IFF_UP in the change
// mask, so nothing else about the interface is touched. The device is named
// rather than indexed because __rtnl_newlink falls back to IFLA_IFNAME when
// ifi_index is zero.
func setLinkUp(c *netlink.Conn, ifName string) error {
	ae := netlink.NewAttributeEncoder()
	ae.String(unix.IFLA_IFNAME, ifName)
	attrs, err := ae.Encode()
	if err != nil {
		return fmt.Errorf("encoding the link-up attributes for %s: %w", ifName, err)
	}
	_, err = c.Execute(netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.RTM_NEWLINK),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append(ifInfomsg(unix.IFF_UP, unix.IFF_UP), attrs...),
	})
	if err != nil {
		return fmt.Errorf("RTM_NEWLINK IFF_UP on %s: %w", ifName, err)
	}
	return nil
}

// linkIndex resolves an interface name to its index in c's namespace with one
// RTM_GETLINK.
func linkIndex(c *netlink.Conn, ifName string) (uint32, error) {
	ae := netlink.NewAttributeEncoder()
	ae.String(unix.IFLA_IFNAME, ifName)
	attrs, err := ae.Encode()
	if err != nil {
		return 0, fmt.Errorf("encoding the lookup for %s: %w", ifName, err)
	}
	msgs, err := c.Execute(netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.RTM_GETLINK),
			Flags: netlink.Request,
		},
		Data: append(ifInfomsg(0, 0), attrs...),
	})
	if err != nil {
		return 0, fmt.Errorf("RTM_GETLINK %s: %w", ifName, err)
	}
	if len(msgs) == 0 || len(msgs[0].Data) < unix.SizeofIfInfomsg {
		return 0, fmt.Errorf("RTM_GETLINK %s: the kernel answered nothing usable", ifName)
	}
	idx := binary.NativeEndian.Uint32(msgs[0].Data[4:8])
	if idx == 0 {
		return 0, fmt.Errorf("RTM_GETLINK %s: index 0", ifName)
	}
	return idx, nil
}

// ifInfomsg renders a struct ifinfomsg. A netlink message's own header fields
// are host byte order; only the payload of an address attribute is network
// order. The index is left zero throughout this file: every caller names its
// interface instead.
func ifInfomsg(flags, change uint32) []byte {
	b := make([]byte, unix.SizeofIfInfomsg)
	binary.NativeEndian.PutUint32(b[8:12], flags)
	binary.NativeEndian.PutUint32(b[12:16], change)
	return b
}
