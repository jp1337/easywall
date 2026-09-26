//go:build integration

package core

// 2.24's claim, measured: under published_ports = "filtered" an IPv6 client
// reaches a named port on a container and no other, and IPv4 in the same run
// is unchanged. The container network is found by the real bridge detection —
// the router-side veth is named br-ew6 — so the kernel's own fe80:: address on
// it is part of the test (spec D4).

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

const (
	ipv6ForwardedPort   = 12251
	ipv6UnforwardedPort = 12252
	ipv6BridgeCIDR      = "fd00:ea5e:77::/64"
)

// router6 reuses nftables_forward_test.go's router for holding namespaces
// and running commands (holdNamespace, run, inNS — same package, same build
// tag) and adds the IPv6 legs.
type router6 struct {
	*router
}

// newRouter6 builds client A and container B around this namespace, dual stack.
func newRouter6(t *testing.T) *router6 {
	t.Helper()
	for _, bin := range []string{"nsenter", "ip", "bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed", bin)
		}
	}
	r := &router6{router: &router{t: t}}
	r.holdA, r.pidA = r.holdNamespace()
	r.holdB, r.pidB = r.holdNamespace()
	t.Cleanup(func() {
		_ = r.holdA.Process.Kill()
		_ = r.holdB.Process.Kill()
		_, _ = r.holdA.Process.Wait()
		_, _ = r.holdB.Process.Wait()
		for _, dev := range []string{"va6-r", "br-ew6"} {
			_ = exec.Command("ip", "link", "del", dev).Run()
		}
	})

	r.run("ip", "link", "add", "va6", "type", "veth", "peer", "name", "va6-r")
	r.run("ip", "link", "add", "vb6", "type", "veth", "peer", "name", "br-ew6")
	r.run("ip", "link", "set", "va6", "netns", r.pidA)
	r.run("ip", "link", "set", "vb6", "netns", r.pidB)

	r.run("ip", "addr", "add", "10.78.1.1/24", "dev", "va6-r")
	r.run("ip", "-6", "addr", "add", "2001:db8:78::1/64", "dev", "va6-r", "nodad")
	r.run("ip", "link", "set", "va6-r", "up")
	r.run("ip", "addr", "add", "10.78.2.1/24", "dev", "br-ew6")
	r.run("ip", "-6", "addr", "add", "fd00:ea5e:77::1/64", "dev", "br-ew6", "nodad")
	r.run("ip", "link", "set", "br-ew6", "up")

	r.inNS(r.pidA, "ip", "addr", "add", "10.78.1.2/24", "dev", "va6")
	r.inNS(r.pidA, "ip", "-6", "addr", "add", "2001:db8:78::2/64", "dev", "va6", "nodad")
	r.inNS(r.pidA, "ip", "link", "set", "va6", "up")
	r.inNS(r.pidA, "ip", "route", "add", "default", "via", "10.78.1.1")
	r.inNS(r.pidA, "ip", "-6", "route", "add", "default", "via", "2001:db8:78::1")
	r.inNS(r.pidB, "ip", "addr", "add", "10.78.2.2/24", "dev", "vb6")
	r.inNS(r.pidB, "ip", "-6", "addr", "add", "fd00:ea5e:77::2/64", "dev", "vb6", "nodad")
	r.inNS(r.pidB, "ip", "link", "set", "vb6", "up")
	r.inNS(r.pidB, "ip", "route", "add", "default", "via", "10.78.2.1")
	r.inNS(r.pidB, "ip", "-6", "route", "add", "default", "via", "fd00:ea5e:77::1")

	for _, p := range []string{"/proc/sys/net/ipv4/ip_forward", "/proc/sys/net/ipv6/conf/all/forwarding"} {
		if err := os.WriteFile(p, []byte("1"), 0o644); err != nil { // #nosec G306 -- a /proc/sys knob
			skipOrFailUnprovable(t, "cannot enable forwarding in this namespace: "+err.Error())
		}
	}

	// Permanent neighbour entries on every side, so NDP — untracked, and
	// dropped under block by the input chain's family drop — cannot decide
	// the result as the control phase's entries age (plan review #3). The
	// forward chain is then the only variable.
	// ip, not /sys/class/net: nsenter -n keeps this namespace's sysfs mount.
	mac := func(pid, dev string) string {
		args := []string{"ip", "-br", "link", "show", "dev", dev}
		if pid != "" {
			args = append([]string{"nsenter", "-t", pid, "-n"}, args...)
		}
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			skipOrFailUnprovable(t, "reading the address of "+dev+": "+err.Error())
		}
		f := strings.Fields(string(out)) // "va6@if5  UP  aa:bb:cc:dd:ee:ff <...>"
		if len(f) < 3 {
			skipOrFailUnprovable(t, "no link address for "+dev+" in "+string(out))
		}
		return f[2]
	}
	r.run("ip", "-6", "neigh", "replace", "2001:db8:78::2", "lladdr", mac(r.pidA, "va6"), "dev", "va6-r", "nud", "permanent")
	r.run("ip", "-6", "neigh", "replace", "fd00:ea5e:77::2", "lladdr", mac(r.pidB, "vb6"), "dev", "br-ew6", "nud", "permanent")
	r.inNS(r.pidA, "ip", "-6", "neigh", "replace", "2001:db8:78::1", "lladdr", mac("", "va6-r"), "dev", "va6", "nud", "permanent")
	r.inNS(r.pidB, "ip", "-6", "neigh", "replace", "fd00:ea5e:77::1", "lladdr", mac("", "br-ew6"), "dev", "vb6", "nud", "permanent")
	return r
}

// listenInB binds port in both families inside B's namespace from this
// process: a socket keeps B's namespace after the thread returns to its own.
// tcp4 and tcp6, not "tcp" on [::]: TestMain's namespace starts with lo down,
// so Go's one-time IPv6 probe (a bind to ::1) fails and "tcp" on [::] quietly
// binds 0.0.0.0 — the IPv6 control then fails for a reason that is not ours.
func (r *router6) listenInB(port int) {
	r.t.Helper()
	runtime.LockOSThread()
	self, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(r.t, "cannot open this thread's namespace: "+err.Error())
	}
	defer func() { _ = self.Close() }()
	target, err := os.Open("/proc/" + r.pidB + "/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(r.t, "cannot open B's namespace: "+err.Error())
	}
	defer func() { _ = target.Close() }()
	if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		runtime.UnlockOSThread()
		skipOrFailUnprovable(r.t, "setns into B: "+err.Error())
	}
	ln4, lerr := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	var ln6 net.Listener
	if lerr == nil {
		ln6, lerr = net.Listen("tcp6", net.JoinHostPort("::", strconv.Itoa(port)))
		if lerr != nil {
			_ = ln4.Close()
		}
	}
	if err := unix.Setns(int(self.Fd()), unix.CLONE_NEWNET); err != nil {
		// The thread is in the wrong namespace: leave it locked so it dies
		// with this goroutine instead of running anything else.
		r.t.Fatalf("setns back from B: %v", err)
	}
	runtime.UnlockOSThread()
	if lerr != nil {
		skipOrFailUnprovable(r.t, "listen in B on port "+strconv.Itoa(port)+": "+lerr.Error())
	}
	for _, ln := range []net.Listener{ln4, ln6} {
		r.t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
	}
}

// filteredIPv6Settings is the shipped default for neighbour advertisements
// (config/easywall.toml) plus the mode under test.
func filteredIPv6Settings(mode shared.IPv6Mode) shared.NetworkSettings {
	return shared.NetworkSettings{
		IPv6: shared.IPv6Config{Mode: mode, ICMPAllowNeighborAdvertisement: true},
		Docker: shared.DockerConfig{
			Enabled: true, AllowBridgeNetworks: true,
			PublishedPorts: shared.PublishedPortsFiltered,
		},
	}
}

func forwardedTCP(port int) shared.Rules {
	return shared.Rules{TCP: []shared.PortRule{{
		ID: "00000000a624", Port: strconv.Itoa(port), Scope: shared.ScopeForwarded,
	}}}
}

func TestIntegration_AnIPv6ClientReachesANamedContainerPortAndNoOther(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter6(t)
	for _, port := range []int{ipv6ForwardedPort, ipv6UnforwardedPort} {
		r.listenInB(port)
	}

	// The control: both ports, both families, before any table. Without it a
	// silence below is also what an unrouted harness looks like.
	for _, addr := range []string{"10.78.2.2", "fd00:ea5e:77::2"} {
		for _, port := range []int{ipv6ForwardedPort, ipv6UnforwardedPort} {
			if !tcpReaches(t, r.pidA, addr, port) {
				skipOrFailUnprovable(t, "with no table, "+addr+" port "+strconv.Itoa(port)+
					" was not reachable from A, so nothing measured below would mean anything")
			}
		}
	}

	// D4 needs something to ignore: without a kernel fe80:: on br-ew6 when
	// Apply reads it (addr_gen_mode none, disable_ipv6), the link-local check
	// below would pass with nothing to catch.
	if out, err := exec.Command("ip", "-6", "addr", "show", "dev", "br-ew6", "scope", "link").Output(); err != nil ||
		!strings.Contains(string(out), "inet6 fe80:") {
		skipOrFailUnprovable(t, "br-ew6 carries no fe80:: address, so D4 has nothing to ignore: "+string(out))
	}

	if err := m.Apply(shared.RulesState{Current: forwardedTCP(ipv6ForwardedPort)},
		shared.FirewallOptions{}, filteredIPv6Settings(shared.IPv6Filter)); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, addr := range []string{"10.78.2.2", "fd00:ea5e:77::2"} {
		if !tcpReaches(t, r.pidA, addr, ipv6ForwardedPort) {
			t.Errorf("%s port %d has a forwarded rule and is not reachable", addr, ipv6ForwardedPort)
		}
		if tcpReaches(t, r.pidA, addr, ipv6UnforwardedPort) {
			t.Errorf("%s port %d has no forwarded rule and is reachable — the deny did nothing", addr, ipv6UnforwardedPort)
		}
	}

	baked := m.BakedBridges()
	if !slices.Contains(baked, ipv6BridgeCIDR) {
		t.Errorf("the rules in force were not built over %s: %v", ipv6BridgeCIDR, baked)
	}
	for _, c := range baked {
		if strings.HasPrefix(c, "fe80:") {
			t.Errorf("the kernel's link-local address on br-ew6 became a container network: %v", baked)
		}
	}
}

// D3 on a real kernel: under block the same rule opens IPv4 and not IPv6.
func TestIntegration_BlockKeepsAnIPv6ClientAwayFromContainers(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter6(t)
	r.listenInB(ipv6ForwardedPort)
	for _, addr := range []string{"fd00:ea5e:77::2", "10.78.2.2"} {
		if !tcpReaches(t, r.pidA, addr, ipv6ForwardedPort) {
			skipOrFailUnprovable(t, addr+" did not answer before any table existed")
		}
	}

	if err := m.Apply(shared.RulesState{Current: forwardedTCP(ipv6ForwardedPort)},
		shared.FirewallOptions{}, filteredIPv6Settings(shared.IPv6Block)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if tcpReaches(t, r.pidA, "fd00:ea5e:77::2", ipv6ForwardedPort) {
		t.Error("under ipv6.mode = block an IPv6 client reached a container")
	}
	if !tcpReaches(t, r.pidA, "10.78.2.2", ipv6ForwardedPort) {
		t.Error("under ipv6.mode = block the IPv4 half of the same rule stopped working")
	}
	// The kernel's own copy: not one forward-chain rule tests IPv6. Without
	// this the unreachable port could also be the input chain's family drop
	// eating NDP — the answer for the wrong reason (plan review #3).
	fwd, err := m.conn.GetRules(easywallInetTable(), &nftables.Chain{Name: "forward"})
	if err != nil {
		t.Fatalf("reading the forward chain back: %v", err)
	}
	for _, rule := range fwd {
		// ruleFamily sees only a rule that starts with meta nfproto + cmp;
		// every forward-chain builder today emits exactly that shape.
		if fam, ok := ruleFamily(rule.Exprs); ok && fam == unix.NFPROTO_IPV6 {
			var exprs []string
			for _, e := range rule.Exprs {
				exprs = append(exprs, fmt.Sprintf("%T%+v", e, e))
			}
			t.Errorf("under ipv6.mode = block the kernel's forward chain holds an IPv6 rule: %s", strings.Join(exprs, " "))
		}
	}
}
