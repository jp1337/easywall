//go:build integration

package core

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The rest of the integration suite asserts rule *counts*: "expected base+3
// rules, got 3". That catches a rule that was never added, and nothing else. A
// blacklist entry that emitted ACCEPT instead of DROP, compared the destination
// address instead of the source, or read the wrong offset out of an IPv6
// header would pass every one of those tests.
//
// These tests read back what the kernel actually holds and assert on its
// meaning. `nft list ruleset` is the same view an operator gets, which is the
// point: if the rule does not say what it should here, it does not say it on
// their machine either.

// ruleset returns the kernel's own rendering of the easywall table.
func ruleset(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("nft", "list", "table", "inet", tableName).CombinedOutput()
	if err != nil {
		t.Fatalf("nft list table: %v: %s", err, out)
	}
	return string(out)
}

// mustContain fails with the full ruleset attached, because a missing rule is
// impossible to diagnose from a boolean.
func mustContain(t *testing.T, rs, want, why string) {
	t.Helper()
	if !strings.Contains(rs, want) {
		t.Errorf("ruleset is missing %q\n  %s\n--- ruleset ---\n%s", want, why, rs)
	}
}

func mustNotContain(t *testing.T, rs, unwanted, why string) {
	t.Helper()
	if strings.Contains(rs, unwanted) {
		t.Errorf("ruleset unexpectedly contains %q\n  %s\n--- ruleset ---\n%s", unwanted, why, rs)
	}
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// The log prefix is the whole feature: filters.md tells operators to run
// `journalctl -k -f | grep easywall`, and without a prefix on the rule there is
// nothing for that to match. expr.Log.Key is a bitmask over the NFTA_LOG_*
// indices; setting it to the bare attribute number set the group bit and left
// the prefix bit clear, so every log rule shipped unlabelled.
func TestIntegration_FinalLog_CarriesPrefix(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{LogBlocked: true})

	rs := ruleset(t)
	mustContain(t, rs, `prefix "`+logPrefixDrop+`"`,
		"without it the kernel log line has no marker to grep for")
}

func TestIntegration_FinalLog_IsRateLimited(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{LogBlocked: true, LogBlockedLimit: 30})

	rs := ruleset(t)
	mustContain(t, rs, "limit rate 30/minute",
		"an unlimited log rule lets a flood fill the disk")
}

// Per-module logging: the switch exists on the options page for eight modules
// and produced no rule at all before 2.5.0.
func TestIntegration_ModuleLogging_EmitsOneLabelledRulePerModule(t *testing.T) {
	cases := []struct {
		name   string
		opts   shared.FirewallOptions
		prefix string
	}{
		{"invalid packets", shared.FirewallOptions{InvalidPackets: true, InvalidPacketsLog: true}, logPrefixInvalid},
		{"fragments", shared.FirewallOptions{Fragments: true, FragmentsLog: true}, logPrefixFragment},
		{"bogons", shared.FirewallOptions{Bogons: true, BogonsLog: true}, logPrefixBogon},
		{"syn flood", shared.FirewallOptions{SYNFlood: true, SYNFloodLog: true}, logPrefixSYNFlood},
		{"icmp flood", shared.FirewallOptions{ICMPFlood: true, ICMPFloodLog: true}, logPrefixICMPFlood},
		{"tcp rst flood", shared.FirewallOptions{TCPRSTFlood: true, TCPRSTFloodLog: true}, logPrefixTCPRST},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			applyEmpty(t, m, tc.opts)
			mustContain(t, ruleset(t), `prefix "`+tc.prefix+`"`,
				"the module's log switch is on, so its drops must be labelled")
		})
	}
}

// Port scan and SSH brute force drop inside their own named chain, so their log
// rule belongs there too — one rule covering everything that jumped in, placed
// before the drop that ends the chain.
func TestIntegration_ChainModuleLogging_LogsInsideTheNamedChain(t *testing.T) {
	cases := []struct {
		name        string
		opts        shared.FirewallOptions
		chain       string
		prefix      string
		expectation string
	}{
		{
			"port scan", shared.FirewallOptions{PortScan: true, PortScanLog: true},
			"portscan", logPrefixPortScan,
			"everything that jumps to this chain is a scan by definition",
		},
		{
			// The SSH log moved with the drop into sshbrute-over when the rate
			// became per source: the meter has to be evaluated exactly once per
			// packet, so the log cannot repeat the match beside it.
			"ssh brute force", shared.FirewallOptions{SSHBruteForce: true, SSHBruteForceLog: true},
			"sshbrute-over", logPrefixSSH,
			"only connections over the rate reach the drop, and those are the event",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			applyEmpty(t, m, tc.opts)
			rs := ruleset(t)

			mustContain(t, rs, `prefix "`+tc.prefix+`"`, tc.expectation)

			// The log must precede the chain's drop, or it never fires.
			chainAt := strings.Index(rs, "chain "+tc.chain+" {")
			if chainAt < 0 {
				t.Fatalf("no %s chain\n--- ruleset ---\n%s", tc.chain, rs)
			}
			body := rs[chainAt:]
			if end := strings.Index(body, "\n\t}"); end > 0 {
				body = body[:end]
			}
			logAt, dropAt := strings.Index(body, tc.prefix), strings.Index(body, "drop")
			if logAt < 0 || dropAt < 0 || logAt > dropAt {
				t.Errorf("log rule is not before the drop in chain %s\n--- chain ---\n%s",
					tc.chain, body)
			}
		})
	}
}

// The log switch must be a switch: off means no log rule, not a silent one.
func TestIntegration_ModuleLogging_OffEmitsNoLogRule(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{InvalidPackets: true, InvalidPacketsLog: false})

	mustNotContain(t, ruleset(t), "easywall invalid",
		"logging is off for this module")
}

// ---------------------------------------------------------------------------
// Blacklist and whitelist — verdict and direction, not rule count
// ---------------------------------------------------------------------------

func TestIntegration_Blacklist_DropsTheSourceAddress(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Blacklist = []string{"192.0.2.1"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	mustContain(t, rs, "ip saddr 192.0.2.1",
		"a blacklist that matched the destination would block replies, not attackers")
	mustNotContain(t, rs, "ip daddr 192.0.2.1",
		"the blacklist is about where a packet came from")

	for _, line := range strings.Split(rs, "\n") {
		if strings.Contains(line, "192.0.2.1") && !strings.Contains(line, "drop") {
			t.Errorf("blacklist rule does not drop: %s", strings.TrimSpace(line))
		}
	}
}

func TestIntegration_Blacklist_IPv6UsesTheSourceOffset(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Blacklist = []string{"2001:db8::1"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Offset 8 is the source address in an IPv6 header; 24 is the destination.
	// A count-based test cannot tell those apart.
	mustContain(t, ruleset(t), "ip6 saddr 2001:db8::1",
		"the IPv6 blacklist must read the source, at header offset 8")
}

func TestIntegration_Whitelist_AcceptsRatherThanDrops(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Whitelist = []string{"10.0.0.0/8"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	for _, line := range strings.Split(rs, "\n") {
		if strings.Contains(line, "10.0.0.0/8") {
			if !strings.Contains(line, "accept") {
				t.Errorf("whitelist rule does not accept: %s", strings.TrimSpace(line))
			}
			return
		}
	}
	t.Errorf("no rule for the whitelisted network\n--- ruleset ---\n%s", rs)
}

// Order is the one property of a firewall ruleset that cannot be checked by
// looking at any single rule. rule-order is documented on the landing page:
// a blacklisted address is dropped before the whitelist is ever consulted.
func TestIntegration_BlacklistIsEvaluatedBeforeWhitelist(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Blacklist = []string{"192.0.2.1"}
	state.Current.Whitelist = []string{"192.0.2.0/24"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	blacklistAt := strings.Index(rs, "192.0.2.1")
	whitelistAt := strings.Index(rs, "192.0.2.0/24")
	if blacklistAt < 0 || whitelistAt < 0 {
		t.Fatalf("expected both rules\n--- ruleset ---\n%s", rs)
	}
	if blacklistAt > whitelistAt {
		t.Errorf("whitelist precedes blacklist: a blacklisted host inside a whitelisted "+
			"range would be let through\n--- ruleset ---\n%s", rs)
	}
}

// ---------------------------------------------------------------------------
// Port forwarding — which port is matched and which is redirected to
// ---------------------------------------------------------------------------

// The interface labels these "Incoming port" and "Forward to", the help text
// says "a request to 8080 is served by whatever listens on 80", and the export
// format documents {"source_port": 2222, "dest_port": 22} as SSH reachable on
// 2222. All three agree: match SourcePort, redirect to DestPort.
//
// The existing integration tests count rules and check that a prerouting chain
// exists, which is true whichever way round the ports go.
func TestIntegration_Forwarding_MatchesIncomingAndRedirectsToTarget(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	// The documented example: reach SSH on 2222.
	state.Current.Forwarding = []shared.ForwardingRule{
		{Protocol: "tcp", SourcePort: 2222, DestPort: 22},
	}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	var rule string
	for _, line := range strings.Split(rs, "\n") {
		if strings.Contains(line, "redirect") {
			rule = strings.TrimSpace(line)
		}
	}
	if rule == "" {
		t.Fatalf("no redirect rule\n--- ruleset ---\n%s", rs)
	}

	if !strings.Contains(rule, "dport 2222") {
		t.Errorf("the rule does not match the incoming port 2222: %q\n"+
			"  reversing these silently redirects the target port instead — with the "+
			"documented example that would capture SSH on 22 and send it to 2222", rule)
	}
	if !strings.Contains(rule, "redirect to :22") {
		t.Errorf("the rule does not redirect to the target port 22: %q", rule)
	}
}

// ---------------------------------------------------------------------------
// Is the firewall actually up?
// ---------------------------------------------------------------------------

// The dashboard renders Active as "the core daemon is running and rules are
// live". That is a claim about the kernel, and it used to be answered with a
// hardcoded true meaning "the daemon is running".
func TestIntegration_Enforcing_TrueOnlyWhenRulesAreInstalled(t *testing.T) {
	m := newIntegrationManager(t)

	if m.Enforcing() {
		t.Error("no table exists yet, so nothing is being enforced")
	}

	applyEmpty(t, m, shared.FirewallOptions{})
	if !m.Enforcing() {
		t.Error("rules are installed, so this must report enforcing")
	}
}

// The case the old implementation got wrong, and the one an operator is most
// likely to hit: something removed the table out from under the daemon.
func TestIntegration_Enforcing_FalseAfterTheTableIsDeleted(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})
	if !m.Enforcing() {
		t.Fatal("precondition: rules should be live")
	}

	if out, err := exec.Command("nft", "delete", "table", "inet", tableName).CombinedOutput(); err != nil {
		t.Fatalf("delete table: %v: %s", err, out)
	}

	if m.Enforcing() {
		t.Error("the table is gone; the dashboard would still be showing green")
	}
}

// A table with an empty input chain is not "live rules" either — the policy
// would drop everything, which is a different problem, not a working firewall.
func TestIntegration_Enforcing_FalseWhenTheInputChainIsEmpty(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if m.Enforcing() {
		t.Error("the table exists but holds no rules")
	}
}

// ---------------------------------------------------------------------------
// Refusing bad rules rather than skipping them
// ---------------------------------------------------------------------------

// An address that will not parse used to be stored, listed in the interface as
// blocked, and then quietly skipped by the parse guard in addCIDRDrop. Apply
// now refuses the whole set.
func TestIntegration_Apply_RefusesAnUnparseableEntry(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Blacklist = []string{"192.0.2.1", "192.168.1.999"}

	err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{})
	if err == nil {
		t.Fatal("Apply accepted an address that cannot become a rule; it would be " +
			"listed as blocked and never enforced")
	}
	if !strings.Contains(err.Error(), "192.168.1.999") {
		t.Errorf("the error should name the offending entry, got: %v", err)
	}
}

// And it must refuse *before* Reset, or the check costs the operator the
// working ruleset it was meant to protect.
func TestIntegration_Apply_RefusalLeavesThePreviousRulesInPlace(t *testing.T) {
	m := newIntegrationManager(t)

	good := emptyState()
	good.Current.Blacklist = []string{"192.0.2.1"}
	if err := m.Apply(good, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := ruleset(t)

	bad := emptyState()
	bad.Current.Blacklist = []string{"not-an-address"}
	if err := m.Apply(bad, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err == nil {
		t.Fatal("expected refusal")
	}

	if after := ruleset(t); after != before {
		t.Errorf("the refused apply changed the live ruleset\n--- before ---\n%s\n--- after ---\n%s",
			before, after)
	}
	mustContain(t, ruleset(t), "ip saddr 192.0.2.1",
		"the previously applied blacklist must survive a refused apply")
}

// ---------------------------------------------------------------------------
// Modules that produced nothing before 2.5.0
// ---------------------------------------------------------------------------

func TestIntegration_ConnectionLimit_LimitsPerSourceAddress(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{ConnectionLimit: true, ConnectionLimitMax: 42})

	rs := ruleset(t)
	mustContain(t, rs, "ct count over 42",
		"the option is documented as a cap on simultaneous connections")
	mustContain(t, rs, "saddr",
		"the cap is per source address; a global counter would lock everyone out at once")
}

func TestIntegration_TCPRSTFlood_RateLimitsResetPackets(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{TCPRSTFlood: true, TCPRSTFloodLimit: 7})

	rs := ruleset(t)
	mustContain(t, rs, "rst", "the module is specifically about RST packets")
	mustContain(t, rs, "limit rate over 7/second", "above the configured rate, drop")
}

func TestIntegration_DropAnycast_MatchesTheDestinationAddressType(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{DropAnycast: true})

	mustContain(t, ruleset(t), "fib daddr type anycast",
		"anycast is a property of the destination address, resolved through the FIB")
}

func TestIntegration_LogBlacklist_LabelsHitsBeforeDropping(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Blacklist = []string{"192.0.2.1"}
	opts := shared.FirewallOptions{LogBlacklist: true, LogBlacklistLimit: 20}
	if err := m.Apply(state, opts, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	mustContain(t, rs, `prefix "`+logPrefixBlacklist+`"`, "the switch is on")

	logAt := strings.Index(rs, logPrefixBlacklist)
	dropAt := strings.Index(rs, "192.0.2.1 drop")
	if logAt < 0 || dropAt < 0 {
		t.Fatalf("expected a log rule and a drop rule\n--- ruleset ---\n%s", rs)
	}
	if logAt > dropAt {
		t.Errorf("the drop precedes the log, so nothing is ever logged\n--- ruleset ---\n%s", rs)
	}
}

// A port range is two comparisons on one payload load. The count-based tests
// cannot tell a range from a single port — both are one rule.
func TestIntegration_PortRange_ReachesTheKernelAsARange(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "8000:9000"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	mustContain(t, ruleset(t), "dport 8000-9000",
		"a range that collapsed to a single port would open far less than the rule says")
}

// A port marked for SSH protection jumps to the brute-force chain instead of
// accepting outright. Marking it must not stop it being reachable.
func TestIntegration_SSHFlaggedPort_JumpsToTheBruteForceChain(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "2222", SSH: true}}
	opts := shared.FirewallOptions{SSHBruteForce: true}
	if err := m.Apply(state, opts, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	mustContain(t, rs, "dport 2222 jump sshbrute",
		"the flagged port must go through the rate limiter")
	mustContain(t, rs, "chain sshbrute",
		"and that chain has to exist")
}

// ---------------------------------------------------------------------------
// IPv6 disposition
// ---------------------------------------------------------------------------

// The three modes replace a boolean whose "off" was documented as leaving IPv6
// unfiltered and did the opposite. Each mode is a claim about all IPv6 traffic,
// so each is checked against what the kernel ends up holding.
func TestIntegration_IPv6Mode(t *testing.T) {
	// A v6 blacklist entry and an open port: under filter both must appear,
	// under passthrough and block neither may be reachable, because the
	// family-wide rule comes first.
	state := func() shared.RulesState {
		s := emptyState()
		s.Current.Blacklist = []string{"2001:db8::1"}
		s.Current.TCP = []shared.PortRule{{Port: "443"}}
		return s
	}

	t.Run("filter applies every rule to IPv6", func(t *testing.T) {
		m := newIntegrationManager(t)
		cfg := shared.IPv6Config{Mode: shared.IPv6Filter, ICMPAllowNeighborAdvertisement: true}
		if err := m.Apply(state(), shared.FirewallOptions{}, cfg, shared.DockerConfig{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		rs := ruleset(t)
		mustContain(t, rs, "ip6 saddr 2001:db8::1", "the v6 blacklist entry belongs in the table")
		mustContain(t, rs, "icmpv6", "IPv6 needs its ICMPv6 types to work at all")
		mustNotContain(t, rs, "meta nfproto ipv6 accept", "filter mode waves nothing through")
	})

	t.Run("passthrough accepts IPv6 before any rule", func(t *testing.T) {
		m := newIntegrationManager(t)
		cfg := shared.IPv6Config{Mode: shared.IPv6Passthrough}
		if err := m.Apply(state(), shared.FirewallOptions{}, cfg, shared.DockerConfig{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		rs := ruleset(t)
		mustContain(t, rs, "meta nfproto ipv6 accept",
			"passthrough means exactly this, and it is the whole feature")

		// It has to come before the v6 blacklist, or the blacklist still bites
		// and "not filtered at all" is untrue again.
		acceptAt := strings.Index(rs, "meta nfproto ipv6 accept")
		blacklistAt := strings.Index(rs, "ip6 saddr 2001:db8::1")
		if blacklistAt >= 0 && acceptAt > blacklistAt {
			t.Errorf("IPv6 is still filtered before it is waved through\n--- ruleset ---\n%s", rs)
		}
		// And loopback must still be first, ahead of it.
		if lo := strings.Index(rs, `iifname "lo" accept`); lo < 0 || lo > acceptAt {
			t.Errorf("loopback must stay the first rule\n--- ruleset ---\n%s", rs)
		}
	})

	t.Run("block drops IPv6 but never loopback", func(t *testing.T) {
		m := newIntegrationManager(t)
		cfg := shared.IPv6Config{Mode: shared.IPv6Block}
		if err := m.Apply(state(), shared.FirewallOptions{}, cfg, shared.DockerConfig{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		rs := ruleset(t)
		mustContain(t, rs, "meta nfproto ipv6 drop", "block means all IPv6 goes")

		dropAt := strings.Index(rs, "meta nfproto ipv6 drop")
		lo := strings.Index(rs, `iifname "lo" accept`)
		if lo < 0 || lo > dropAt {
			t.Errorf("dropping IPv6 ahead of loopback kills ::1 and every local "+
				"service bound to it\n--- ruleset ---\n%s", rs)
		}
		mustNotContain(t, rs, "icmpv6",
			"there is nothing for ICMPv6 to permit once IPv6 is dropped")
	})
}

// A zero-valued IPv6Config must filter. An unset mode that skipped the ICMPv6
// rules while every other rule still applied to IPv6 is exactly the behaviour
// the three modes were introduced to remove, and it would have come back
// through any caller that built the struct by hand.
func TestIntegration_IPv6Mode_ZeroValueFilters(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.Apply(emptyState(), shared.FirewallOptions{},
		shared.IPv6Config{ICMPAllowNeighborAdvertisement: true}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	rs := ruleset(t)
	mustContain(t, rs, "icmpv6", "an unset mode must behave as filter")
	mustNotContain(t, rs, "meta nfproto ipv6", "and must not wave IPv6 through or drop it")
}

// ---------------------------------------------------------------------------
// Per-source rate limits
// ---------------------------------------------------------------------------

// Four modules described a per-source rate in the interface, in the
// documentation and in the JSON schema, and enforced a single counter shared by
// every source. That is not a weaker version of the promise — it inverts it. An
// attacker who spends the budget locks out everyone else, so the module sold as
// protection against being locked out is the thing that locks you out.
//
// `nft` renders a per-source meter as a dynamic set update with a limit
// attached; a shared counter renders as a bare `limit rate over` on the rule.
// The two are easy to tell apart, which is what these tests do.
func TestIntegration_SSHBruteForce_LimitsEachSourceSeparately(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{
		SSHBruteForce:                true,
		SSHBruteForceConnectionLimit: 5,
	})

	rs := ruleset(t)
	mustContain(t, rs, "set sshbrute-v4", "the per-source rate needs a set keyed by IPv4 source address")
	mustContain(t, rs, "set sshbrute-v6", "IPv6 sources need their own set — the key is a different width")
	mustContain(t, rs, "update @sshbrute-v4 { ip saddr timeout 10m limit rate over 5/minute",
		"the limit has to live inside the set, keyed by source, not on the rule")
	mustContain(t, rs, "update @sshbrute-v6 { ip6 saddr timeout 10m limit rate over 5/minute",
		"the IPv6 rule must key on ip6 saddr")

	// A source within its rate is ordinary traffic and must be accepted.
	mustContain(t, rs, "chain sshbrute", "SSH still goes through its own chain")
	if !strings.Contains(rs, "jump sshbrute-over") {
		t.Error("an over-rate source must be sent to the chain that logs and drops")
	}
}

func TestIntegration_SYNFlood_LimitsEachSourceSeparately(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{SYNFlood: true, SYNFloodLimit: 100})

	rs := ruleset(t)
	mustContain(t, rs, "update @synflood-v4 { ip saddr timeout 1m limit rate over 100/second",
		"one host must not be able to consume the SYN budget for the whole machine")
	mustContain(t, rs, "update @synflood-v6 { ip6 saddr timeout 1m limit rate over 100/second",
		"IPv6 sources are rate limited separately too")
}

func TestIntegration_ICMPFlood_LimitsEachSourceSeparately(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{ICMPFlood: true, ICMPFloodConnectionLimit: 10})

	rs := ruleset(t)
	mustContain(t, rs, "update @icmpflood-v4 { ip saddr timeout 1m limit rate over 10/second",
		"an echo flood from one source must not stop pings from another")
	mustContain(t, rs, "update @icmpflood-v6 { ip6 saddr timeout 1m limit rate over 10/second",
		"a ping flood over IPv6 used to pass the module entirely")
	mustContain(t, rs, "icmpv6 type echo-request",
		"the IPv6 rule has to match ICMPv6 type 128, not ICMP type 8")
	mustContain(t, rs, "icmp type echo-request",
		"the IPv4 rule still matches ICMP type 8")
}

func TestIntegration_TCPRSTFlood_LimitsEachSourceSeparately(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{TCPRSTFlood: true, TCPRSTFloodLimit: 100})

	rs := ruleset(t)
	mustContain(t, rs, "update @tcprst-v4 { ip saddr timeout 1m limit rate over 100/second",
		"a reset flood from one source must not tear down everyone's budget")
	mustContain(t, rs, "update @tcprst-v6 { ip6 saddr timeout 1m limit rate over 100/second",
		"IPv6 resets are limited per source as well")
}

// The meter must not be evaluated twice for the same packet. addFiltered emits
// the match once for a log rule and once for the action; with a stateful
// expression in the match that would consume two tokens per packet and halve
// the configured rate. The log therefore sits in the target chain instead.
func TestIntegration_PerSourceLogging_DoesNotDoubleCountTheRate(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{
		SYNFlood: true, SYNFloodLimit: 100, SYNFloodLog: true,
	})

	rs := ruleset(t)
	if n := strings.Count(rs, "@synflood-v4"); n != 1 {
		t.Errorf("the IPv4 meter must be updated by exactly one rule, found %d\n%s", n, rs)
	}
	mustContain(t, rs, logPrefixSYNFlood, "the log prefix belongs to the over-rate chain")
	mustContain(t, rs, "chain synflood-over", "the log and the drop share one chain")
}

// Elements have to expire, or a spoofed-source flood fills the set instead of
// the connection table.
func TestIntegration_PerSourceSets_ExpireTheirEntries(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{SYNFlood: true, SYNFloodLimit: 100})

	rs := ruleset(t)
	mustContain(t, rs, "flags dynamic,timeout", "the set must drop idle sources")
}

// A count cannot tell you which ranges are covered, and the documentation named
// two that were not in the code while omitting two that were. This asserts the
// list itself.
func TestIntegration_BogonFilter_CoversTheDocumentedRanges(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{Bogons: true})
	rs := ruleset(t)

	for _, cidr := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.2.0/24", "192.168.0.0/16",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	} {
		mustContain(t, rs, "ip saddr "+cidr,
			"filters.md lists this range as one the bogon filter drops")
	}
	mustNotContain(t, rs, "ip daddr 10.0.0.0/8",
		"a bogon is a claim about where a packet came from, not where it is going")
}
