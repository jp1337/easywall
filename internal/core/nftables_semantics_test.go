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
			"ssh brute force", shared.FirewallOptions{SSHBruteForce: true, SSHBruteForceLog: true},
			"sshbrute", logPrefixSSH,
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
