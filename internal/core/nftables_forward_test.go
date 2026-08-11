//go:build integration

package core

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The forward chain, measured by routing a packet through it.
//
// Reading this chain tells you nothing: it is a base chain at the forward hook
// with policy drop and — before the fix — no rules, which looks like "easywall
// takes no interest in routed traffic" and behaves like "easywall destroys it".
// The existing check asserted the policy, which was the part that was right.
//
// So the test builds a router: two namespaces either side of this one, a route
// between them, and a ping. Nothing here inspects a rule.

// router is a host with a network on each side, which is what a Docker host is.
type router struct {
	t            *testing.T
	pidA, pidB   string
	netA, netB   string // the networks behind each side
	addrA, addrB string
	holdA, holdB *exec.Cmd
}

// newRouter puts a namespace on each side of this one and routes between them.
// It skips the test when the tools or the privileges are not there.
func newRouter(t *testing.T) *router {
	t.Helper()
	for _, bin := range []string{"unshare", "nsenter", "ip", "ping"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test routes real packets", bin)
		}
	}

	r := &router{
		t:     t,
		netA:  "10.77.1.0/24",
		netB:  "10.77.2.0/24",
		addrA: "10.77.1.2",
		addrB: "10.77.2.2",
	}

	// Each leaf namespace is held open by a sleeping process; the veth end is
	// moved into it by pid. `ip netns add` would need /run/netns, which is not
	// writable inside a user namespace.
	r.holdA, r.pidA = r.holdNamespace()
	r.holdB, r.pidB = r.holdNamespace()
	t.Cleanup(func() {
		_ = r.holdA.Process.Kill()
		_ = r.holdB.Process.Kill()
		_, _ = r.holdA.Process.Wait()
		_, _ = r.holdB.Process.Wait()
		// The router-side ends stay behind when the leaf namespaces go, and a
		// second test in the same namespace would then fail to create them.
		for _, dev := range []string{"va-r", "vb-r"} {
			_ = exec.Command("ip", "link", "del", dev).Run()
		}
	})

	r.run("ip", "link", "add", "va", "type", "veth", "peer", "name", "va-r")
	r.run("ip", "link", "add", "vb", "type", "veth", "peer", "name", "vb-r")
	r.run("ip", "link", "set", "va", "netns", r.pidA)
	r.run("ip", "link", "set", "vb", "netns", r.pidB)

	r.run("ip", "addr", "add", "10.77.1.1/24", "dev", "va-r")
	r.run("ip", "link", "set", "va-r", "up")
	r.run("ip", "addr", "add", "10.77.2.1/24", "dev", "vb-r")
	r.run("ip", "link", "set", "vb-r", "up")

	r.inNS(r.pidA, "ip", "addr", "add", "10.77.1.2/24", "dev", "va")
	r.inNS(r.pidA, "ip", "link", "set", "va", "up")
	r.inNS(r.pidA, "ip", "route", "add", "default", "via", "10.77.1.1")
	r.inNS(r.pidB, "ip", "addr", "add", "10.77.2.2/24", "dev", "vb")
	r.inNS(r.pidB, "ip", "link", "set", "vb", "up")
	r.inNS(r.pidB, "ip", "route", "add", "default", "via", "10.77.2.1")

	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		t.Skipf("skipping: cannot enable forwarding in this namespace: %v", err)
	}
	return r
}

func (r *router) holdNamespace() (*exec.Cmd, string) {
	r.t.Helper()
	cmd := exec.Command("unshare", "-n", "sleep", "120")
	if err := cmd.Start(); err != nil {
		r.t.Skipf("skipping: cannot create a network namespace: %v", err)
	}
	// The namespace exists once unshare has execed sleep; give it a moment.
	pid := strconv.Itoa(cmd.Process.Pid)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + pid + "/ns/net"); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cmd, pid
}

func (r *router) run(args ...string) {
	r.t.Helper()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		r.t.Skipf("skipping: %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func (r *router) inNS(pid string, args ...string) {
	r.t.Helper()
	r.run(append([]string{"nsenter", "-t", pid, "-n"}, args...)...)
}

// reachable pings from the namespace on one side to the address on the other.
func (r *router) reachable() bool {
	r.t.Helper()
	cmd := exec.Command("nsenter", "-t", r.pidA, "-n", "ping", "-c", "1", "-W", "1", r.addrB)
	return cmd.Run() == nil
}

// TestIntegration_Forward_DockerNetworksAreRouted is the regression test for a
// forward chain that dropped everything the host routed.
//
// Before the fix the third step failed: the accepts for the allowed networks
// existed in the input chain and nothing put them in the forward chain, so a
// container could not reach anything and nothing could reach a published port.
func TestIntegration_Forward_DockerNetworksAreRouted(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)

	if !r.reachable() {
		t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
	}

	// easywall with Docker coexistence off. The forward chain is closed, which
	// is the right default for a host that does not route — and is asserted
	// here so that the exception below is measured against something.
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if r.reachable() {
		t.Error("with Docker coexistence off the forward chain should be closed, and it is not")
	}

	// The same, with the two networks named as Docker networks. This is what a
	// container host looks like: the bridge range on one side, the world on the
	// other.
	docker := shared.DockerConfig{Enabled: true, CustomNetworks: []string{r.netA, r.netB}}
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{Docker: docker}); err != nil {
		t.Fatalf("Apply with docker networks: %v", err)
	}
	if !r.reachable() {
		t.Errorf("a packet between two allowed Docker networks was dropped by the forward chain\n"+
			"forward chain the kernel is holding:\n  %s",
			strings.Join(chainText(t, "forward"), "\n  "))
	}
}

// TestIntegration_Forward_RoutingModes drives the three positions of
// routing.mode with real packets.
//
// The middle one is the reason the setting exists: before it, a host that
// routed for any reason other than Docker had to describe its networks as
// Docker networks, which worked and said something untrue in the config file.
func TestIntegration_Forward_RoutingModes(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)

	if !r.reachable() {
		t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
	}

	for _, tc := range []struct {
		name    string
		routing shared.RoutingConfig
		want    bool
	}{
		{
			name:    "closed routes nothing",
			routing: shared.RoutingConfig{Mode: shared.RoutingClosed, Networks: []string{r.netA, r.netB}},
			// The networks are listed and must still be ignored: naming them is
			// not the same as switching the mode that consults them.
			want: false,
		},
		{
			name:    "networks routes the ones named",
			routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{r.netA, r.netB}},
			want:    true,
		},
		{
			name:    "networks routes nothing it was not given",
			routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{"192.0.2.0/24"}},
			want:    false,
		},
		{
			name:    "open routes everything, with no list at all",
			routing: shared.RoutingConfig{Mode: shared.RoutingOpen},
			want:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.Apply(emptyState(), shared.FirewallOptions{},
				shared.NetworkSettings{Routing: tc.routing}); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if got := r.reachable(); got != tc.want {
				t.Errorf("routed packet reachable = %v, want %v\nforward chain the kernel is holding:\n  %s",
					got, tc.want, strings.Join(chainText(t, "forward"), "\n  "))
			}
		})
	}
}

// TestIntegration_Forward_DockerCrossesEveryRoutingMode pins the decision that
// Docker's networks are not subject to routing.mode.
//
// Making them subject to it would reintroduce the defect this whole chain of
// work started from: an operator who upgrades and does not discover the new key
// loses every container's network, silently, on the next apply.
func TestIntegration_Forward_DockerCrossesEveryRoutingMode(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)

	if !r.reachable() {
		t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
	}

	docker := shared.DockerConfig{Enabled: true, CustomNetworks: []string{r.netA, r.netB}}
	for _, mode := range []shared.RoutingMode{shared.RoutingClosed, shared.RoutingNetworks} {
		if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{
			Docker:  docker,
			Routing: shared.RoutingConfig{Mode: mode},
		}); err != nil {
			t.Fatalf("Apply with routing.mode=%s: %v", mode, err)
		}
		if !r.reachable() {
			t.Errorf("routing.mode=%s stopped an allowed Docker network\n"+
				"forward chain the kernel is holding:\n  %s",
				mode, strings.Join(chainText(t, "forward"), "\n  "))
		}
	}
}

// TestIntegration_Forward_OneSidedNetworkStillRoutes covers the direction that
// is easy to lose: a published port arrives with only its *destination* inside
// the bridge network, because Docker has already translated it.
func TestIntegration_Forward_OneSidedNetworkStillRoutes(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)

	if !r.reachable() {
		t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
	}

	// Only the far side is a Docker network. The ping out of namespace A has a
	// source that is not in it and a destination that is.
	docker := shared.DockerConfig{Enabled: true, CustomNetworks: []string{r.netB}}
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{Docker: docker}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !r.reachable() {
		t.Errorf("traffic *to* an allowed Docker network was dropped — a published container port "+
			"reaches the forward chain with only its destination inside the bridge range\n"+
			"forward chain the kernel is holding:\n  %s",
			strings.Join(chainText(t, "forward"), "\n  "))
	}
}
