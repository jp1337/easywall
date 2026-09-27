//go:build integration

package core

// 2.25, measured over the router harness (10.77.1.2 and fd77:1::2 on side A
// reach this host at 10.77.1.1 and fd77:1::1): ping is answered when the
// switch says so and dropped when it does not, in both families (D1); this
// host's own ping gets its reply either way, and the ICMP errors a host needs
// arrive with no type accept, as return traffic (D3); an allowlisted monitor
// is answered with the switch off (D6); a timestamp request is dropped (D8).

import (
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// dualStackRouter is newRouter with the IPv6 addresses the ICMP flood test
// also uses. applyRules applies ndAllowed, so neighbour discovery works.
func dualStackRouter(t *testing.T) *router {
	t.Helper()
	r := newRouter(t)
	r.run("ip", "-6", "addr", "add", "fd77:1::1/64", "dev", "va-r", "nodad")
	r.inNS(r.pidA, "ip", "-6", "addr", "add", "fd77:1::2/64", "dev", "va", "nodad")
	return r
}

// hostPingReceived pings from this namespace — the firewalled host itself.
func hostPingReceived(t *testing.T, args ...string) int {
	t.Helper()
	out, _ := exec.Command("ping", append([]string{"-q"}, args...)...).CombinedOutput()
	return pingSummary(t, out)
}

func TestIntegration_TheSwitchDecidesWhetherPingIsAnswered(t *testing.T) {
	m := newIntegrationManager(t)
	r := dualStackRouter(t)
	// On first: it is the control that the harness answers pings at all.
	for _, tc := range []struct {
		on   bool
		want int
	}{{true, 3}, {false, 0}} {
		applyRules(t, m, shared.Rules{}, shared.FirewallOptions{ICMPAllowEchoRequest: tc.on})
		for _, to := range []string{"10.77.1.1", "fd77:1::1"} {
			if got := pingReceived(t, r, "-c", "3", "-i", "0.2", "-W", "1", to); got != tc.want {
				t.Errorf("switch %t: %d of 3 pings to %s answered, want %d\n  %s",
					tc.on, got, to, tc.want, strings.Join(chainText(t, "input"), "\n  "))
			}
		}
		if t.Failed() && tc.on {
			t.Fatal("control failed: the harness does not answer pings with the switch on")
		}
	}
}

// D3: the reply to this host's own echo is established — no type 0 accept.
func TestIntegration_ThisHostsOwnPingIsAnsweredWithTheSwitchOff(t *testing.T) {
	m := newIntegrationManager(t)
	r := dualStackRouter(t)
	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{})
	for _, to := range []string{r.addrA, "fd77:1::2"} {
		if got := hostPingReceived(t, "-c", "3", "-i", "0.2", "-W", "1", to); got != 3 {
			t.Errorf("%d of 3 replies to this host's own ping to %s arrived with the switch off", got, to)
		}
	}
}

// D6: the allowlist matches the source and no protocol, so a monitor on it is
// answered with the switch off (Review Focus 4).
func TestIntegration_AnAllowlistedMonitorIsAnsweredWithTheSwitchOff(t *testing.T) {
	m := newIntegrationManager(t)
	r := dualStackRouter(t)
	applyRules(t, m, shared.Rules{Allowlist: []string{r.addrA, "fd77:1::2"}}, shared.FirewallOptions{})
	for _, to := range []string{"10.77.1.1", "fd77:1::1"} {
		if got := pingReceived(t, r, "-c", "3", "-i", "0.2", "-W", "1", to); got != 3 {
			t.Errorf("%d of 3 pings from an allowlisted monitor to %s answered with the switch off", got, to)
		}
	}
}

// D3: side A becomes a router with a 1280-byte link behind it. An echo from
// this host with TTL 1 comes back as time exceeded, and a 1400-byte one with
// DF as fragmentation needed / packet too big. Both are errors about a flow
// this host started, which conntrack calls related (nf_conntrack_inet_error),
// so the established,related accept admits them with the switch off and no
// type accept anywhere.
func TestIntegration_ICMPErrorsForATrackedFlowArrive(t *testing.T) {
	m := newIntegrationManager(t)
	r := dualStackRouter(t)
	for _, c := range [][]string{
		{"sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward && echo 1 > /proc/sys/net/ipv6/conf/all/forwarding"},
		{"ip", "link", "add", "d0", "type", "dummy"},
		{"ip", "link", "set", "d0", "mtu", "1280", "up"},
		{"ip", "route", "add", "10.77.9.0/24", "dev", "d0"},
		{"ip", "-6", "route", "add", "fd77:9::/64", "dev", "d0"},
	} {
		r.inNS(r.pidA, c...)
	}
	// Removed with va-r, which newRouter's cleanup deletes.
	r.run("ip", "route", "add", "10.77.9.0/24", "via", r.addrA)
	r.run("ip", "-6", "route", "add", "fd77:9::/64", "via", "fd77:1::2")

	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{})

	exceeded := regexp.MustCompile(`(?i)time to live exceeded|time exceeded`)
	for _, to := range []string{"10.77.9.1", "fd77:9::1"} {
		out, _ := exec.Command("ping", "-c", "1", "-t", "1", "-W", "1", to).CombinedOutput()
		if !exceeded.Match(out) {
			t.Errorf("no time exceeded for a TTL-1 ping to %s — a traceroute would show nothing:\n%s", to, out)
		}
	}
	for fam, to := range map[string]string{"-4": "10.77.9.1", "-6": "fd77:9::1"} {
		_, _ = exec.Command("ping", "-c", "1", "-W", "1", "-M", "do", "-s", "1400", to).CombinedOutput()
		out, err := exec.Command("ip", fam, "route", "get", to).CombinedOutput()
		if err != nil || !strings.Contains(string(out), "mtu 1280") {
			t.Errorf("path MTU to %s not learned — the fragmentation-needed / packet-too-big error did not arrive:\n%s", to, out)
		}
	}
}

// D8: a timestamp request is dropped. Linux answers one that reaches it, so
// the control — the same request from an allowlisted source — gets a reply,
// and the drop is what hides the clock.
func TestIntegration_ATimestampRequestIsDropped(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)
	c := icmpSocketIn(t, r.pidA)

	applyRules(t, m, shared.Rules{Allowlist: []string{r.netA}}, shared.FirewallOptions{ICMPAllowEchoRequest: true})
	if !timestampAnswered(t, c, "10.77.1.1", 1) {
		t.Fatal("control: an allowlisted timestamp request got no reply; the kernel or the harness does not answer them")
	}
	// A new identifier: conntrack keys a timestamp on it, and the control's
	// exchange is an established entry that outlives the re-apply.
	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{ICMPAllowEchoRequest: true})
	if timestampAnswered(t, c, "10.77.1.1", 2) {
		t.Errorf("a timestamp request was answered with only Answer pings on\n  %s",
			strings.Join(chainText(t, "input"), "\n  "))
	}
}

// icmpSocketIn opens a raw ICMPv4 socket in pid's network namespace. The
// socket keeps that namespace after the thread returns to its own — the
// pattern router6.listenInB uses.
func icmpSocketIn(t *testing.T, pid string) net.PacketConn {
	t.Helper()
	runtime.LockOSThread()
	self, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(t, "cannot open this thread's namespace: "+err.Error())
	}
	defer func() { _ = self.Close() }()
	target, err := os.Open("/proc/" + pid + "/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(t, "cannot open side A's namespace: "+err.Error())
	}
	defer func() { _ = target.Close() }()
	if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(t, "setns into side A: "+err.Error())
	}
	c, lerr := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err := unix.Setns(int(self.Fd()), unix.CLONE_NEWNET); err != nil {
		// The thread is in the wrong namespace: leave it locked so it dies
		// with this goroutine instead of running anything else.
		t.Fatalf("setns back from side A: %v", err)
	}
	runtime.UnlockOSThread()
	if lerr != nil {
		skipOrFailUnprovable(t, "raw ICMP socket in side A: "+lerr.Error())
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// timestampAnswered sends one ICMP timestamp request (type 13) and waits a
// second for its reply (type 14, same identifier). Go strips the IPv4 header
// from what an ip4 socket reads.
func timestampAnswered(t *testing.T, c net.PacketConn, to string, id uint16) bool {
	t.Helper()
	msg := make([]byte, 20)
	msg[0] = 13
	binary.BigEndian.PutUint16(msg[4:], id)
	binary.BigEndian.PutUint16(msg[6:], 1)
	binary.BigEndian.PutUint16(msg[2:], icmpChecksum(msg))
	if _, err := c.WriteTo(msg, &net.IPAddr{IP: net.ParseIP(to)}); err != nil {
		t.Fatalf("send timestamp request: %v", err)
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1500)
	for {
		n, _, err := c.ReadFrom(buf)
		if err != nil {
			return false
		}
		if n >= 8 && buf[0] == 14 && binary.BigEndian.Uint16(buf[4:]) == id {
			return true
		}
	}
}

// icmpChecksum is RFC 1071's one's-complement sum.
func icmpChecksum(b []byte) uint16 {
	var s uint32
	for i := 0; i+1 < len(b); i += 2 {
		s += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		s += uint32(b[len(b)-1]) << 8
	}
	for s>>16 != 0 {
		s = s&0xffff + s>>16
	}
	return ^uint16(s)
}

// Final review, 2.25: the echo accepts sit after the blocklist and the feeds.
// Placed with the ICMP accepts right after established, they answered a
// blocklisted source — and every later ping of that sweep is established —
// against blocklist.md's "consulted before every other rule that can accept a
// packet".
func TestIntegration_ABlocklistedSourceGetsNoPingReply(t *testing.T) {
	m := newIntegrationManager(t)
	r := dualStackRouter(t)
	applyRules(t, m, shared.Rules{Blocklist: []string{r.addrA, "fd77:1::2"}},
		shared.FirewallOptions{ICMPAllowEchoRequest: true})
	for _, to := range []string{"10.77.1.1", "fd77:1::1"} {
		if got := pingReceived(t, r, "-c", "3", "-i", "0.2", "-W", "1", to); got != 0 {
			t.Errorf("%d of 3 pings from a blocklisted source to %s answered\n  %s",
				got, to, strings.Join(chainText(t, "input"), "\n  "))
		}
	}
	input := chainText(t, "input")
	block := indexOfRule(input, r.addrA, "drop")
	for _, echo := range []string{"icmp type echo-request accept", "icmpv6 type echo-request accept"} {
		if at := indexOfRule(input, echo); at < block {
			t.Errorf("%q is rule %d, the blocklist drop %d; the blocklist must come first", echo, at, block)
		}
	}
}
