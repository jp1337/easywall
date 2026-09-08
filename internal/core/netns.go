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
// Everything here is netlink and pipes. The only program this file execs is
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
//	parent -> child   (pipe closed)                 child exits

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
// Cloneflags did that before exec — so it configures nothing and only dials
// what it is told to. The parent owns every netlink write, including the ones
// that land inside this namespace, which it reaches through /proc/<pid>/ns/net.
func RunPeer(stdin io.Reader, stdout io.Writer) int {
	if _, err := fmt.Fprintln(stdout, "ready"); err != nil {
		return 1
	}
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		var addr string
		var port, ms int
		if n, _ := fmt.Sscanf(sc.Text(), "dial %s %d %d", &addr, &port, &ms); n != 3 {
			_, _ = fmt.Fprintln(stdout, "blocked")
			continue
		}
		d := net.Dialer{Timeout: time.Duration(ms) * time.Millisecond}
		conn, err := d.Dial("tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
		if err != nil {
			_, _ = fmt.Fprintln(stdout, "blocked")
			continue
		}
		_ = conn.Close()
		_, _ = fmt.Fprintln(stdout, "open")
	}
	return 0
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
)

// Harness is a peer process in its own network namespace, wired to this one by
// a veth pair.
type Harness struct {
	child  *exec.Cmd
	stdin  *os.File
	out    *os.File // kept as the concrete type, for SetReadDeadline
	stdout *bufio.Scanner

	// netnsFd is /proc/<child pid>/ns/net. It stays open for the harness's
	// whole life because the self-test hands it to nftables.WithNetNSFd, which
	// dups nothing and expects the fd to still be there at Apply time.
	netnsFd int

	// peerConn writes netlink inside the child's namespace. Opened on netnsFd,
	// closed by Close.
	peerConn *netlink.Conn
}

// NewHarness starts a peer in a fresh network namespace and wires it to this one
// with a veth pair.
//
// Both ends sit in one /24, so adding the addresses installs the connected route
// on each side and no RTM_NEWROUTE is sent. A partially built harness is torn
// down here, so a caller that gets an error never has to call Close.
func NewHarness() (*Harness, error) {
	// os.Pipe rather than cmd.StdinPipe, because the read end has to be an
	// *os.File: Dial sets a read deadline on it so that a peer which died
	// mid-handshake is an error instead of a goroutine parked forever on a pipe
	// nobody will ever write to again.
	childIn, parentIn, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("harness stdin pipe: %w", err)
	}
	parentOut, childOut, err := os.Pipe()
	if err != nil {
		_, _ = childIn.Close(), parentIn.Close()
		return nil, fmt.Errorf("harness stdout pipe: %w", err)
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
		if errors.Is(startErr, syscall.EPERM) || errors.Is(startErr, syscall.EINVAL) {
			return nil, fmt.Errorf("%w: %v", ErrNamespaceUnavailable, startErr)
		}
		return nil, fmt.Errorf("starting the harness peer: %w", startErr)
	}

	h := &Harness{
		child:   cmd,
		stdin:   parentIn,
		out:     parentOut,
		stdout:  bufio.NewScanner(parentOut),
		netnsFd: -1,
	}

	// Anything but "ready" is the same refusal as a start error that wraps
	// EPERM: the kernel let us clone and then something else stopped the peer
	// from running, and either way there is no harness to measure in.
	if line, err := h.readLine(harnessReadyTimeout); err != nil || line != "ready" {
		h.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: the peer never reported ready: %v",
				ErrNamespaceUnavailable, err)
		}
		return nil, fmt.Errorf("%w: the peer said %q, not \"ready\"",
			ErrNamespaceUnavailable, line)
	}

	if err := h.wire(); err != nil {
		h.Close()
		return nil, err
	}
	return h, nil
}

// wire builds the pair and configures both ends. Four kinds of netlink message,
// each one a named function so it can be read against ip-link(8) and
// ip-address(8) rather than against this sequence.
func (h *Harness) wire() error {
	pid := h.child.Process.Pid

	fd, err := unix.Open("/proc/"+strconv.Itoa(pid)+"/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("%w: opening the peer's netns: %v", ErrNamespaceUnavailable, err)
	}
	h.netnsFd = fd

	local, err := netlink.Dial(unix.NETLINK_ROUTE, nil)
	if err != nil {
		return fmt.Errorf("rtnetlink in this namespace: %w", err)
	}
	defer func() { _ = local.Close() }()

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
	if h.netnsFd >= 0 {
		_ = unix.Close(h.netnsFd)
		h.netnsFd = -1
	}
	// Closing stdin is what tells a live peer to exit; the kill is for one that
	// is wedged somewhere else.
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
	if h.child != nil && h.child.Process != nil {
		_ = h.child.Process.Kill()
		_, _ = h.child.Process.Wait()
	}
	if h.out != nil {
		_ = h.out.Close()
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
	if _, err := fmt.Fprintf(h.stdin, "dial %s %d %d\n", addr, port, timeout.Milliseconds()); err != nil {
		return false, fmt.Errorf("asking the peer to dial: %w", err)
	}
	line, err := h.readLine(timeout + 2*time.Second)
	if err != nil {
		return false, fmt.Errorf("reading the peer's verdict: %w", err)
	}
	switch line {
	case "open":
		return true, nil
	case "blocked":
		return false, nil
	default:
		return false, fmt.Errorf("the peer answered %q, which is not a verdict", line)
	}
}

func (h *Harness) readLine(timeout time.Duration) (string, error) {
	if err := h.out.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("setting the peer read deadline: %w", err)
	}
	if !h.stdout.Scan() {
		if err := h.stdout.Err(); err != nil {
			return "", err
		}
		return "", errors.New("the peer closed its pipe")
	}
	return h.stdout.Text(), nil
}

// createVethPair sends one RTM_NEWLINK carrying the whole pair, with the peer's
// namespace named by pid.
//
// One message rather than a create-then-move pair: a move is a second syscall
// that can fail with the peer already visible in this namespace, and the kernel
// has accepted IFLA_NET_NS_PID inside VETH_INFO_PEER since veth existed.
func createVethPair(c *netlink.Conn, routerIf, peerIf string, peerPID int) error {
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
