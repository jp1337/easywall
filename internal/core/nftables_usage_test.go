package core

import (
	"strings"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/google/nftables/userdata"
	"github.com/jp1337/easywall/internal/shared"
)

// A UI rule with two sources is two kernel rules, and both have to answer to
// the same id or the counter is summed from half the traffic. Drop the UserData
// assignment and this goes red — which is the point: nothing else in the suite
// reads what addPortAccept builds, because it cannot be called without a
// netlink connection.
func TestPortAcceptRules_EveryPortKernelRuleCarriesItsRuleID(t *testing.T) {
	rule := shared.PortRule{
		ID: "abcdef012345", Port: "443", Description: "HTTPS",
		Sources: []string{"10.0.0.0/8", "192.168.1.5"},
	}
	built := portAcceptRules(nil, nil, "tcp", rule)
	if len(built) != 2 {
		t.Fatalf("two sources produced %d kernel rules, want 2", len(built))
	}
	for i, r := range built {
		got, ok := userdata.GetString(r.UserData, userdata.TypeComment)
		if !ok {
			t.Errorf("kernel rule %d carries no comment; the collector has nothing to key on", i)
			continue
		}
		if got != rule.ID {
			t.Errorf("kernel rule %d is tagged %q, want %q", i, got, rule.ID)
		}
	}

	// And a rule with no sources — one kernel rule, tagged the same way.
	plain := portAcceptRules(nil, nil, "udp", shared.PortRule{ID: "0123456789ab", Port: "53"})
	if len(plain) != 1 {
		t.Fatalf("a rule with no sources produced %d kernel rules, want 1", len(plain))
	}
	if got, _ := userdata.GetString(plain[0].UserData, userdata.TypeComment); got != "0123456789ab" {
		t.Errorf("the sourceless rule is tagged %q, want 0123456789ab", got)
	}
}

// After the match, before the verdict.
//
// Placed before the matches the counter counts every packet that *reaches* the
// rule rather than every packet that *matches* it — the number would then be a
// measure of the rules above it and of nothing else. That is the one mistake
// available here, and it produces a plausible, monotonically rising, entirely
// wrong figure that an operator would close a port on.
func TestPortAcceptRules_TheCounterSitsAfterTheMatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule shared.PortRule
	}{
		{"no sources", shared.PortRule{ID: "aaaaaaaaaaaa", Port: "80"}},
		{"one source", shared.PortRule{ID: "bbbbbbbbbbbb", Port: "80", Sources: []string{"10.0.0.0/8"}}},
		{"a range", shared.PortRule{ID: "cccccccccccc", Port: "8000:9000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, r := range portAcceptRules(nil, nil, "tcp", tc.rule) {
				exprs := r.Exprs
				if len(exprs) < 3 {
					t.Fatalf("kernel rule %d has %d expressions; too few to hold a match, a counter and a verdict",
						i, len(exprs))
				}
				counterAt := -1
				for j, e := range exprs {
					if _, ok := e.(*expr.Counter); ok {
						if counterAt >= 0 {
							t.Fatalf("kernel rule %d carries two counters", i)
						}
						counterAt = j
					}
				}
				if counterAt < 0 {
					t.Fatalf("kernel rule %d carries no counter; the port can never report a last use", i)
				}
				if _, ok := exprs[len(exprs)-1].(*expr.Verdict); !ok {
					t.Fatalf("kernel rule %d does not end in a verdict", i)
				}
				if counterAt != len(exprs)-2 {
					t.Errorf("kernel rule %d puts the counter at %d of %d; it belongs immediately "+
						"before the verdict, or it counts what reached the rule rather than what matched it",
						i, counterAt, len(exprs))
				}
			}
		})
	}
}

// The collector reads the input chain, and addPortAccept writes to it and to no
// other. Point one of them somewhere else and they stop describing the same
// rules — the counters would be read from a chain that has none, and every port
// would report "never" for ever.
//
// Source-read rather than behavioural: the chain a manager writes to is decided
// by an argument at two call sites, and no runtime assertion short of a kernel
// can see which one was passed.
func TestCollectionReadsTheInputChain(t *testing.T) {
	if inputChainName != "input" {
		t.Fatalf("inputChainName = %q; the kernel's base input chain is called \"input\"", inputChainName)
	}

	src := coreSource(t, "nftables.go")

	// Every port rule goes into the chain the collector reads.
	calls := indexesOf(src, "m.addPortAccept(")
	if len(calls) != 2 {
		t.Fatalf("found %d calls to m.addPortAccept, want 2 (tcp and udp); a third call site "+
			"has to be checked against this guard's assumption that they all name inputChain",
			len(calls))
	}
	for _, at := range calls {
		line := src[at:min(at+120, len(src))]
		if !strings.Contains(line, "inputChain") {
			t.Errorf("a call to addPortAccept does not pass inputChain: %s",
				strings.SplitN(line, "\n", 2)[0])
		}
	}

	// And the collector reads that chain by the same constant, not by a literal
	// of its own that could drift.
	body := funcBody(t, src, "nftables.go", "func (m *NftablesManager) RuleCounters(")
	if !strings.Contains(body, "inputChainName") {
		t.Error("RuleCounters does not name inputChainName; it is reading some other chain, " +
			"or a literal that can drift away from the one addPortAccept writes to")
	}
	for _, other := range []string{`"forward"`, `"output"`, `"prerouting"`, `"sshbrute"`} {
		if strings.Contains(body, other) {
			t.Errorf("RuleCounters mentions %s; the port counters live in the input chain alone", other)
		}
	}
}
