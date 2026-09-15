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
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// dockerNATFixture programs the three rules a published port can produce, in
// the table and chain Docker uses with the iptables-nft backend.
//
//	53 on a bridge gateway   the reporting host's resolver
//	8080 on every address    `-p 8080:80`, which has no `ip daddr` at all
//	9999 to somewhere else   a DNAT that is not a container's, and must not be
//	                         reported as one
func dockerNATFixture(t *testing.T) {
	t.Helper()
	const ruleset = `
table ip nat {
	chain DOCKER {
		ip daddr 172.17.0.1 udp dport 53 counter dnat to 172.18.0.2:53
		tcp dport 8080 counter dnat to 172.18.0.3:80
		tcp dport 9999 counter dnat to 192.0.2.7:9999
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

	got := detectPublishedPorts([]string{"172.18.0.0/16"})

	want := map[string]bool{
		"udp 53 on 172.17.0.1": false,
		"tcp 8080 on 0.0.0.0":  false,
	}
	for _, p := range got {
		key := p.proto + " " + strconv.Itoa(int(p.port)) + " on " + p.addr
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

	if got := detectPublishedPorts(nil); len(got) != 0 {
		t.Errorf("with no bridge network detected the deny renders nothing, so "+
			"nothing is closed and nothing may be named; got %v", got)
	}
}
