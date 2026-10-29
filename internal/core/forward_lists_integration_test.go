//go:build integration

package core

// The blocklist, the allowlist and the feeds on a forwarded packet, measured
// through the container leg. Until this test they were rendered into the input
// chain only, so under published_ports = "filtered" a blocklisted client reached
// a container on its forwarded port: `ip saddr 10.9.0.2 drop` sat in input, and
// the forward chain accepted `tcp dport 80` before anything asked who was
// sending it.

import (
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// peerRuleset is `nft list ruleset` inside the harness's peer, for a failure
// message an operator can read.
func peerRuleset(h *Harness) string {
	out, err := exec.Command("nsenter", fmt.Sprintf("--net=/proc/%d/fd/%d", os.Getpid(), h.NetNSFd()),
		"nft", "list", "ruleset").CombinedOutput() // #nosec G204 -- fixed binary, a descriptor we own
	if err != nil {
		return fmt.Sprintf("(nft list ruleset in the peer failed: %v)\n%s", err, out)
	}
	return string(out)
}

// TestIntegration_TheListsRefuseAForwardedSource: the client dials the
// container's forwarded port from the router address. Blocklisted, or in an
// enabled feed or country, it is refused; allowlisted as well, a feed no longer
// refuses it — and the allowlist does not open what the forward chain closes,
// because in that chain it only exempts from the feeds.
func TestIntegration_TheListsRefuseAForwardedSource(t *testing.T) {
	h, m := forwardScopeHarness(t)

	crossed, err := crossesTo(h.ContainerAddr(), selftestForwardedPort)
	if err != nil {
		skipOrFailUnprovable(t, err.Error())
	}
	if !crossed {
		skipOrFailUnprovable(t, "with no table in the peer the container's port was not answered, "+
			"so the leg is not carrying packets")
	}

	client := h.RouterAddr().String()
	fwd := shared.PortRule{Port: strconv.Itoa(selftestForwardedPort), Scope: shared.ScopeForwarded}
	net := shared.NetworkSettings{Docker: shared.DockerConfig{
		Enabled:        true,
		CustomNetworks: []string{h.BridgeCIDR()},
		PublishedPorts: shared.PublishedPortsFiltered,
	}}
	feeds := FeedContents{
		"dshield":    {netip.PrefixFrom(h.RouterAddr(), 32)},
		"country-de": {netip.PrefixFrom(h.RouterAddr(), 32)},
	}

	for _, tc := range []struct {
		name  string
		rules shared.Rules
		port  uint16
		want  bool
	}{
		{"control: no list", shared.Rules{}, selftestForwardedPort, true},
		{"blocklisted", shared.Rules{Blocklist: []string{client}}, selftestForwardedPort, false},
		{"in a feed", shared.Rules{Feeds: []string{"dshield"}}, selftestForwardedPort, false},
		{"in a country", shared.Rules{Feeds: []string{"country-de"}}, selftestForwardedPort, false},
		{"in a feed and allowlisted", shared.Rules{Allowlist: []string{client}, Feeds: []string{"dshield"}},
			selftestForwardedPort, true},
		{"blocklisted and allowlisted", shared.Rules{Blocklist: []string{client}, Allowlist: []string{client}},
			selftestForwardedPort, false},
		{"allowlisted, no forwarded rule", shared.Rules{Allowlist: []string{client}, Feeds: []string{"dshield"}},
			selftestUnforwardedPort, false},
	} {
		rules := tc.rules
		rules.TCP = []shared.PortRule{fwd}
		if err := m.ApplyWithFeeds(shared.RulesState{Current: rules}, shared.FirewallOptions{}, net, feeds); err != nil {
			t.Fatalf("%s: applying: %v", tc.name, err)
		}
		got, err := crossesTo(h.ContainerAddr(), tc.port)
		if err != nil {
			t.Fatalf("%s: dialling: %v", tc.name, err)
		}
		t.Logf("%s: %s -> container:%d crossed=%v", tc.name, client, tc.port, got)
		if got != tc.want {
			t.Errorf("%s: %s -> container:%d crossed=%v, want %v\n%s",
				tc.name, client, tc.port, got, tc.want, peerRuleset(h))
		}
	}
}
