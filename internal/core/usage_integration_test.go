//go:build integration

package core

import (
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Open a port, send packets to it, and watch the number move.
//
// Everything else this release ships is a guard on one link of the chain: the
// counter is in the rule, the id is in the comment, the collect happens before
// the flush, the baseline survives a restart. Each was verified by breaking it —
// and all eight could be green with the feature reporting "never" for every
// port, because none of them puts a packet on a wire.
//
// This one does. It is the test that says the chain works end to end, and it is
// the only one that can.
func TestIntegration_TheCounterMovesWhenAPortIsUsed(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test opens a real TCP connection", bin)
		}
	}

	f := newTestFirewallWithRealNft(t)
	r := newRouter(t)

	const port = 12228
	rule := shared.PortRule{ID: "cafebabe1234", Port: strconv.Itoa(port), Description: "usage proof"}
	rules := shared.Rules{TCP: []shared.PortRule{rule}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}

	if err := f.rules.save(state); err != nil {
		t.Fatalf("seed the rules file: %v", err)
	}
	if err := f.nft.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The collect before any traffic: this is the baseline the delta is measured
	// from, and the instant LastSeen has to advance past.
	if err := f.CollectUsage(); err != nil {
		t.Fatalf("the first collect: %v", err)
	}
	before, err := f.Usage()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := before.Usage[rule.ID]; ok && got.Packets != 0 {
		t.Fatalf("the port had already carried %d packets before anything was sent", got.Packets)
	}
	baselineAt := time.Now()

	// A second between the two collects, so "advanced" is a real comparison and
	// not two timestamps from the same millisecond.
	time.Sleep(1100 * time.Millisecond)

	// Three real connections from 10.77.1.2. No listener: a SYN that the input
	// chain's accept rule matches is counted whether or not anything answers it,
	// and that is exactly what the column is about — the packet reached the port.
	for i := 0; i < 3; i++ {
		tcpReaches(t, r.pidA, "10.77.1.1", port)
	}

	// And one connection to a neighbouring port nothing ever opened. Nothing in
	// the chain accepts it, so it is refused by the policy at the end rather
	// than by this rule, and this rule's counter — correctly placed, after the
	// match — must not move for it.
	//
	// This is the check the first row of the mutation table asks for: moving
	// expr.Counter{} to the front of portMatch's expression list makes it count
	// every packet that *reaches* the rule's position in the chain, whatever its
	// protocol or port, rather than every packet the rule *matches*. With only
	// the three connections above, that mutation is invisible — nothing else
	// reaches this rule's position, so "reaches" and "matches" report the same
	// three. A fourth connection, refused and retried by the kernel's own TCP
	// stack while nothing here ever answers it, reaches that position more than
	// once and is what turns the mutation red.
	tcpReaches(t, r.pidA, "10.77.1.1", port+1)

	if err := f.CollectUsage(); err != nil {
		t.Fatalf("the second collect: %v", err)
	}
	after, err := f.Usage()
	if err != nil {
		t.Fatal(err)
	}

	got, ok := after.Usage[rule.ID]
	if !ok {
		t.Fatalf("the collector has nothing for rule id %s. Either the kernel rule carries "+
			"no comment, or the counter is not being read off the input chain — the two "+
			"halves TestCollectionReadsInputAndForwardChains pins textually and this test pins "+
			"against a kernel", rule.ID)
	}
	if got.Packets == 0 {
		t.Error("packets reached the port and the counter did not move. The rule carries an " +
			"expr.Counter and the id matched, so the counter is in the wrong place in the " +
			"expression list — before the match, where it counts what the rules above it let " +
			"through and nothing about this port")
	}
	if got.Packets != 3 {
		t.Errorf("the counter booked %d packets for three connections to this port and a "+
			"fourth to a different one; the difference is exactly what a counter placed "+
			"before the match instead of after it would pick up — traffic the rules above "+
			"it let through, and nothing about this port", got.Packets)
	}
	if got.Bytes == 0 {
		t.Error("packets were counted and no bytes were; the Counter's Bytes field is not " +
			"being read")
	}
	if got.FirstSeen.IsZero() || got.LastSeen.IsZero() {
		t.Fatalf("traffic was booked and the dates were not set: %+v", got)
	}
	if !got.LastSeen.After(baselineAt) {
		t.Errorf("LastSeen is %s, which is not after the collect that preceded the traffic "+
			"(%s) — the column would report a use that happened before it did",
			got.LastSeen, baselineAt)
	}

	// And an apply does not lose it. The flush zeroes every kernel counter; the
	// total is what has to survive, because it is the thing on the screen.
	if err := f.nft.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	f.resetUsageBaselines()
	if err := f.CollectUsage(); err != nil {
		t.Fatalf("the collect after the apply: %v", err)
	}
	final, _ := f.Usage()
	if final.Usage[rule.ID].Packets < got.Packets {
		t.Errorf("the total fell from %d to %d across an apply; the flush took the history "+
			"with it", got.Packets, final.Usage[rule.ID].Packets)
	}
	if !final.Usage[rule.ID].LastSeen.Equal(got.LastSeen) {
		t.Errorf("LastSeen moved across an apply that carried no traffic: %s then %s",
			got.LastSeen, final.Usage[rule.ID].LastSeen)
	}
}
