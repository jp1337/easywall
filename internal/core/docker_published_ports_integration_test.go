//go:build integration

package core

// The decoder, against rules the kernel built.
//
// The unit tests beside this one fake detectPublishedPortsFn, because the
// comparison they are about cannot reach a netlink socket. That leaves the half
// that reads Docker's own DNAT rules unproven, and a decoder that returns
// nothing is indistinguishable from a host with nothing published — silence
// that looks exactly like the answer. So this writes the rules `docker run -p`
// writes, through nft, and reads them back.

import (
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// dockerNATFixture programs the four rules a published port can produce, in
// the table and chain Docker uses with the iptables-nft backend.
//
//	53 on a bridge gateway   the reporting host's resolver
//	8080 on every address    `-p 8080:80`, which has no `ip daddr` at all and
//	                         reaches the container on a different number
//	9999 to somewhere else   a DNAT that is not a container's, and must not be
//	                         reported as one
//	7070 with no target port a DNAT that translates the address only, so the
//	                         container is reached on the number that arrived
func dockerNATFixture(t *testing.T) {
	t.Helper()
	const ruleset = `
table ip nat {
	chain DOCKER {
		ip daddr 172.17.0.1 udp dport 53 counter dnat to 172.18.0.2:53
		tcp dport 8080 counter dnat to 172.18.0.3:80
		tcp dport 9999 counter dnat to 192.0.2.7:9999
		tcp dport 7070 counter dnat to 172.18.0.9
	}
}
`
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		skipOrFailUnprovable(t, "the nat table could not be written: "+
			err.Error()+": "+string(out))
	}
	t.Cleanup(func() {
		_ = exec.Command("nft", "delete", "table", "ip", "nat").Run()
	})
}

func TestIntegration_PublishedPortsAreReadFromDockersOwnRules(t *testing.T) {
	dockerNATFixture(t)

	got := detectPublishedPorts([]string{"172.18.0.0/16"}, 0)

	// Both numbers in the key, because `-p 8080:80` has two and only one of
	// them is the number a forwarded rule can name: the forward chain runs
	// after this DNAT, so the packet arrives there carrying 80. A decoder that
	// reports 8080 as the container port sends the operator to write a rule
	// that matches nothing — which is what this asserted until 2.20.1.
	want := map[string]bool{
		"udp 53 on 172.17.0.1 -> 53":  false,
		"tcp 8080 on 0.0.0.0 -> 80":   false,
		"tcp 7070 on 0.0.0.0 -> 7070": false,
	}
	for _, p := range got {
		key := p.proto + " " + strconv.Itoa(int(p.port)) + " on " + p.addr +
			" -> " + strconv.Itoa(int(p.containerPort))
		if _, expected := want[key]; !expected {
			t.Errorf("a DNAT that is not a published container port was reported "+
				"as one: %s. Only a target inside a detected bridge is a container.", key)
			continue
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Errorf("%s was published and the decoder did not see it; it read %v.\n"+
				"  A decoder that returns nothing looks exactly like a host with "+
				"nothing published, which is why this test exists.", key, got)
		}
	}
}

// The filter is the bridge list, and an empty one means the caller detected no
// container network — the case addForwardPortRules already refuses to render a
// deny for. Nothing may be reported there either, or the warning outlives the
// rule it is about.
func TestIntegration_NoBridgeMeansNoPublishedPorts(t *testing.T) {
	dockerNATFixture(t)

	if got := detectPublishedPorts(nil, 0); len(got) != 0 {
		t.Errorf("with no bridge network detected the deny renders nothing, so "+
			"nothing is closed and nothing may be named; got %v", got)
	}
}

// The detection reads the namespace the manager writes into, not this process's.
//
// Threading NftablesManager.nsFD through was asserted by nothing: the unit tests
// fake the decoder away, and every other integration test runs both halves in one
// namespace, where the two numbers are the same. This is the arrangement that can
// tell them apart — Docker's NAT rules in *this* process's namespace, a manager
// bound to a fresh one that has none. A detection reading the wrong kernel names
// a port the ruleset it is warning about never closed, which is the one kind of
// line this warning cannot afford: an operator who checks it and finds nothing
// stops believing the next one.
func TestIntegration_ThePublishedPortDetectionReadsTheManagersNamespace(t *testing.T) {
	dockerNATFixture(t)

	// The control. Without it, a decoder that reads nothing anywhere produces
	// exactly the silence this test would call a pass.
	if got := detectPublishedPorts([]string{"172.18.0.0/16"}, 0); len(got) == 0 {
		skipOrFailUnprovable(t, "the fixture is not readable from this process's "+
			"namespace at all, so a silence from the manager's would prove nothing")
	}

	h, err := NewHarness()
	if err != nil {
		skipOrFailUnprovable(t, "the harness could not be built: "+err.Error())
	}
	t.Cleanup(h.Close)
	m, err := NewNftablesManagerInNamespace(h.NetNSFd())
	if err != nil {
		skipOrFailUnprovable(t, "no nftables manager for the peer's namespace: "+err.Error())
	}

	var lines []string
	prevLog := slog.Default()
	slog.SetDefault(slog.New(recordingLogHandler{lines: &lines}))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	m.warnUnruledPublishedPorts(shared.Rules{}, []string{"172.18.0.0/16"})

	for _, line := range lines {
		if strings.Contains(line, "no forwarded rule") {
			t.Fatalf("the manager's namespace holds no Docker NAT rules, and the "+
				"warning named a published port out of this process's namespace "+
				"instead: %q", line)
		}
	}
}

// dockerNAT6Fixture is dockerNATFixture's ip6 twin: what Docker writes with
// ip6tables enabled and a port published on [::].
func dockerNAT6Fixture(t *testing.T) {
	t.Helper()
	const ruleset = `
table ip6 nat {
	chain DOCKER {
		ip6 daddr 2001:db8::1 udp dport 53 counter dnat to [fd00:ea5e::2]:53
		tcp dport 8080 counter dnat to [fd00:ea5e::3]:80
		tcp dport 9999 counter dnat to [2001:db8::7]:9999
		tcp dport 7070 counter dnat to fd00:ea5e::9
	}
}
`
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		skipOrFailUnprovable(t, "the ip6 nat table could not be written: "+err.Error()+": "+string(out))
	}
	t.Cleanup(func() { _ = exec.Command("nft", "delete", "table", "ip6", "nat").Run() })
}

// dockerIptablesFixture writes what Docker's default iptables backend writes:
// the same DNAT rules through iptables-nft and ip6tables-nft, which encode
// -j DNAT as an xtables target. The fixture above, written with nft -f,
// produces the nat-expression encoding instead — which is why the check's
// blindness to Docker's real rules went unnoticed from 2.20.1 to 2.24.
func dockerIptablesFixture(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"iptables-nft", "ip6tables-nft"} {
		if _, err := exec.LookPath(bin); err != nil {
			skipOrFailUnprovable(t, bin+" is not installed, and it is what Docker's iptables backend runs")
		}
	}
	for _, c := range [][]string{
		{"iptables-nft", "-t", "nat", "-N", "DOCKER"},
		{"iptables-nft", "-t", "nat", "-A", "DOCKER", "-d", "172.17.0.1/32", "-p", "udp", "-m", "udp",
			"--dport", "53", "-j", "DNAT", "--to-destination", "172.18.0.2:53"},
		{"iptables-nft", "-t", "nat", "-A", "DOCKER", "-p", "tcp", "-m", "tcp",
			"--dport", "7070", "-j", "DNAT", "--to-destination", "172.18.0.9"},
		{"ip6tables-nft", "-t", "nat", "-N", "DOCKER"},
		{"ip6tables-nft", "-t", "nat", "-A", "DOCKER", "-d", "2001:db8::1/128", "-p", "tcp", "-m", "tcp",
			"--dport", "25", "-j", "DNAT", "--to-destination", "[fd00:ea5e::4]:2525"},
	} {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			skipOrFailUnprovable(t, strings.Join(c, " ")+": "+err.Error()+": "+string(out))
		}
	}
	t.Cleanup(func() {
		_ = exec.Command("nft", "delete", "table", "ip", "nat").Run()
		_ = exec.Command("nft", "delete", "table", "ip6", "nat").Run()
	})
}

// D6's bug: Docker's own rules, as its default backend writes them, in both
// families — and the warning an operator reads, end to end.
func TestIntegration_DockersIptablesRulesAreReadAndNamed(t *testing.T) {
	dockerIptablesFixture(t)

	got := detectPublishedPorts([]string{"172.18.0.0/16", "fd00:ea5e::/64"}, 0)
	want := map[string]bool{
		"udp 53 on 172.17.0.1 -> 53":    false,
		"tcp 7070 on 0.0.0.0 -> 7070":   false,
		"tcp 25 on 2001:db8::1 -> 2525": false,
	}
	for _, p := range got {
		key := p.proto + " " + strconv.Itoa(int(p.port)) + " on " + p.addr +
			" -> " + strconv.Itoa(int(p.containerPort))
		if _, expected := want[key]; !expected {
			t.Errorf("reported %s, which the fixture did not publish", key)
			continue
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Errorf("%s is in Docker's iptables rules and was not read; got %v", key, got)
		}
	}

	var lines []string
	prevLog := slog.Default()
	slog.SetDefault(slog.New(recordingLogHandler{lines: &lines}))
	t.Cleanup(func() { slog.SetDefault(prevLog) })
	newIntegrationManager(t).warnUnruledPublishedPorts(shared.Rules{}, []string{"fd00:ea5e::/64"})
	const line = "25 published on 2001:db8::1 reaches the container on 2525: no forwarded rule for 2525"
	for _, l := range lines {
		if strings.Contains(l, line) {
			return
		}
	}
	t.Errorf("the IPv6-only published port was not named.\n  want: %q\n  got: %q", line, lines)
}

// D6: a port published on IPv6 is read like its IPv4 twin. Without this the
// check's silence about an IPv6 port is a false green, and features/docker.md
// had to tell operators to check IPv6 by hand.
func TestIntegration_IPv6PublishedPortsAreRead(t *testing.T) {
	dockerNAT6Fixture(t)

	got := detectPublishedPorts([]string{"fd00:ea5e::/64"}, 0)
	want := map[string]bool{
		"udp 53 on 2001:db8::1 -> 53": false,
		"tcp 8080 on :: -> 80":        false,
		"tcp 7070 on :: -> 7070":      false,
	}
	for _, p := range got {
		key := p.proto + " " + strconv.Itoa(int(p.port)) + " on " + p.addr +
			" -> " + strconv.Itoa(int(p.containerPort))
		if _, expected := want[key]; !expected {
			t.Errorf("reported %s, which is not a published container port", key)
			continue
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Errorf("%s was published on IPv6 and not read; got %v", key, got)
		}
	}
}
