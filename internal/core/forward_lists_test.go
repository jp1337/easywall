package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
)

// forwardWithLists builds the forward chain and then the feeds, in Apply's
// order, and returns one line per rule: "<chain> <what it does>".
func forwardWithLists(t *testing.T, docker shared.DockerConfig, routing shared.RoutingConfig,
	rules shared.Rules, cidrs []string) []string {
	t.Helper()
	c, _ := captureConn(t)
	rec := &recordingConn{}
	m := &NftablesManager{conn: c, adder: rec}
	tbl := easywallInetTableForTest()
	m.buildForwardChain(tbl, &nftables.Chain{Name: forwardChainName, Table: tbl},
		rules, docker, routing, cidrs, shared.IPv6Filter)
	if _, err := m.addFeeds(tbl, inputChainForTest(tbl), rules.Feeds, FeedContents{},
		shared.FirewallOptions{LogFeed: true}); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rec.rules {
		if r.Chain.Name == inputChainName {
			continue
		}
		var parts []string
		for _, e := range r.Exprs {
			switch v := e.(type) {
			case *expr.Ct:
				parts = append(parts, "ct")
			case *expr.Payload:
				if v.Base == expr.PayloadBaseNetworkHeader {
					parts = append(parts, map[uint32]string{12: "saddr", 8: "saddr6", 16: "daddr", 24: "daddr6"}[v.Offset])
				}
			case *expr.Lookup:
				parts = append(parts, "@"+v.SetName)
			case *expr.Log:
				parts = append(parts, "log")
			case *expr.Verdict:
				parts = append(parts, map[expr.VerdictKind]string{expr.VerdictAccept: "accept",
					expr.VerdictDrop: "drop", expr.VerdictReturn: "return",
					expr.VerdictJump: "jump " + v.Chain}[v.Kind])
			}
		}
		if ruleMatchesDport(r, 25) {
			parts = append([]string{"dport25"}, parts...)
		}
		out = append(out, r.Chain.Name+" "+strings.Join(parts, " "))
	}
	// Each chain in its own order; the two chains' rules interleave in the batch.
	slices.SortStableFunc(out, func(a, b string) int {
		return strings.Compare(strings.Fields(a)[0], strings.Fields(b)[0])
	})
	return out
}

var listRules = shared.Rules{
	TCP:       []shared.PortRule{{ID: "aaaaaaaaaaa1", Port: "25", Scope: shared.ScopeForwarded}},
	Blocklist: []string{"# a note", "198.51.100.7"},
	Allowlist: []string{"203.0.113.0/24"},
	Feeds:     []string{"country-de", "dshield"},
}

// The lists sit after the established accept and before the forwarded port
// accept, in the input chain's order; the allowlist returns rather than
// accepts; the feeds look up the sets the input chain uses, without a log.
func TestForwardChain_TheListsComeBeforeTheForwardedAccepts(t *testing.T) {
	got := forwardWithLists(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		listRules, []string{"172.30.0.0/24"})
	want := []string{
		"forward ct accept",
		"forward jump forward-lists",
		"forward dport25 accept",
		"forward daddr saddr drop",
		"forward saddr accept",
		"forward daddr accept",
		"forward-lists saddr return",                 // the container network, as input's bridge accept
		"forward-lists saddr drop",                   // the blocklist
		"forward-lists saddr return",                 // the allowlist: exempt from the feeds, nothing more
		"forward-lists saddr @feed-dshield-v4 drop",  // the lists, then the countries (FeedRuleOrder)
		"forward-lists saddr6 @feed-dshield-v6 drop", //
		"forward-lists saddr @feed-country-de-v4 drop",
		"forward-lists saddr6 @feed-country-de-v6 drop",
	}
	if !slices.Equal(got, want) {
		t.Errorf("forward chain:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// Where easywall leaves forwarding alone, or has nothing to refuse, nothing
// renders; under routing.mode = "open" only traffic to a container jumps.
func TestForwardChain_TheListsRenderOnlyWhereTheChainFilters(t *testing.T) {
	jumps := func(lines []string) []string {
		var out []string
		for _, l := range lines {
			if strings.Contains(l, "jump") {
				out = append(out, l)
			}
		}
		return out
	}
	cidrs := []string{"172.30.0.0/24"}
	for _, tc := range []struct {
		name    string
		docker  shared.DockerConfig
		routing shared.RoutingMode
		rules   shared.Rules
		cidrs   []string
		want    []string
	}{
		{"closed, published open", openDocker(), shared.RoutingClosed, listRules, cidrs,
			[]string{"forward jump forward-lists"}},
		{"networks", openDocker(), shared.RoutingNetworks, listRules, nil,
			[]string{"forward jump forward-lists"}},
		{"open, filtered", filteredDocker(), shared.RoutingOpen, listRules, cidrs,
			[]string{"forward daddr jump forward-lists"}},
		{"open, published open", openDocker(), shared.RoutingOpen, listRules, cidrs, nil},
		{"docker off, closed", shared.DockerConfig{}, shared.RoutingClosed, listRules, nil, nil},
		{"only an allowlist", filteredDocker(), shared.RoutingClosed,
			shared.Rules{TCP: listRules.TCP, Allowlist: listRules.Allowlist, Blocklist: []string{"# only a note"}},
			cidrs, nil},
	} {
		r := shared.RoutingConfig{Mode: tc.routing}
		if tc.routing == shared.RoutingNetworks {
			r.Networks = []string{"10.9.0.0/16"}
		}
		got := forwardWithLists(t, tc.docker, r, tc.rules, tc.cidrs)
		if j := jumps(got); !slices.Equal(j, tc.want) {
			t.Errorf("%s: jumps %v, want %v\n  %s", tc.name, j, tc.want, strings.Join(got, "\n  "))
		}
		if hasLists := slices.ContainsFunc(got, func(l string) bool {
			return strings.HasPrefix(l, forwardListsChainName+" ")
		}); hasLists != (tc.want != nil) {
			t.Errorf("%s: forward-lists rules present = %v, want %v", tc.name, hasLists, tc.want != nil)
		}
	}
}

// fwdLists is this apply's: an apply with no lists after one with lists must
// not append feed drops to the previous apply's chain.
func TestForwardChain_TheListsChainIsNotCarriedIntoTheNextApply(t *testing.T) {
	c, _ := captureConn(t)
	m := &NftablesManager{conn: c, adder: &recordingConn{}}
	tbl := easywallInetTableForTest()
	fwd := &nftables.Chain{Name: forwardChainName, Table: tbl}
	m.buildForwardChain(tbl, fwd, listRules, filteredDocker(), shared.RoutingConfig{}, []string{"172.30.0.0/24"}, shared.IPv6Filter)
	if m.fwdLists == nil {
		t.Fatal("the first apply rendered no forward-lists chain")
	}
	m.buildForwardChain(tbl, fwd, shared.Rules{}, shared.DockerConfig{}, shared.RoutingConfig{}, nil, shared.IPv6Filter)
	if m.fwdLists != nil {
		t.Errorf("the second apply still holds %q", m.fwdLists.Name)
	}
}
