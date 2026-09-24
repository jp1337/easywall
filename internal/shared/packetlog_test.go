package shared

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// The prefix is what the kernel hands back in NFULA_PREFIX, NUL and all. A
// prefix this code does not recognise — a custom rule logging into the same
// group — is "other", never a guess at one of the ten.
func TestRuleFromPrefix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"easywall drop: ", "drop"},
		{"easywall syn-flood: ", "syn_flood"},
		{"easywall icmp-flood: \x00", "icmp_flood"},
		{"easywall tcp-rst:", "tcp_rst"},
		{"easywall portscan: ", "portscan"},
		{"legacy-admin: ", PacketLogRuleOther},
		{"easywall something-new: ", PacketLogRuleOther},
		{"", PacketLogRuleOther},
	} {
		if got := RuleFromPrefix(tc.in); got != tc.want {
			t.Errorf("RuleFromPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPacketLogFilterMatches(t *testing.T) {
	e := PacketLogEntry{
		Rule: "drop", InDev: "eth0", Proto: "tcp", DstPort: 22,
		Src: netip.MustParseAddr("203.0.113.9"), Dst: netip.MustParseAddr("198.51.100.1"),
	}
	for _, tc := range []struct {
		name string
		f    PacketLogFilter
		want bool
	}{
		{"empty matches everything", PacketLogFilter{}, true},
		{"source address", PacketLogFilter{Src: "203.0.113.9"}, true},
		{"source network", PacketLogFilter{Src: "203.0.113.0/24"}, true},
		{"source elsewhere", PacketLogFilter{Src: "192.0.2.0/24"}, false},
		{"mapped spelling of the same source", PacketLogFilter{Src: "::ffff:203.0.113.9"}, true},
		{"mapped network source", PacketLogFilter{Src: "::ffff:203.0.113.0/120"}, true},
		{"destination", PacketLogFilter{Dst: "198.51.100.1"}, true},
		{"port is the destination port", PacketLogFilter{Port: 22}, true},
		{"another port", PacketLogFilter{Port: 443}, false},
		{"protocol", PacketLogFilter{Proto: "udp"}, false},
		{"rule", PacketLogFilter{Rule: "drop"}, true},
		{"interface", PacketLogFilter{InDev: "eth1"}, false},
		{"all at once", PacketLogFilter{Src: "203.0.113.0/24", Port: 22, Proto: "tcp", Rule: "drop", InDev: "eth0"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := tc.f.Matcher()
			if err != nil {
				t.Fatal(err)
			}
			if got := m(e); got != tc.want {
				t.Errorf("match = %v, want %v", got, tc.want)
			}
		})
	}
}

// The web process validates the filter at its trust boundary and the core
// validates it again, with the same function: the core is the privileged side
// and does not take the unprivileged one's word for anything.
func TestPacketLogFilterRefusesWhatItCannotRead(t *testing.T) {
	for _, f := range []PacketLogFilter{
		{Src: "not-an-ip"},
		{Dst: "10.0.0.0/33"},
		{Proto: "sctp"},
		{Rule: "<script>"},
		{InDev: "eth0; rm -rf /"},
		{InDev: "a-name-longer-than-fifteen"},
		{Limit: -1},
	} {
		if err := f.Validate(); err == nil {
			t.Errorf("Validate(%+v) accepted it", f)
		}
	}
	if err := (PacketLogFilter{Rule: PacketLogRuleOther, Proto: "icmpv6", InDev: "wg0"}).Validate(); err != nil {
		t.Errorf("a valid filter was refused: %v", err)
	}
}

func TestEffectiveLimit(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 200}, {50, 50}, {5000, 1000}} {
		if got := (PacketLogFilter{Limit: tc.in}).EffectiveLimit(); got != tc.want {
			t.Errorf("EffectiveLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The empty state on /blocked says "nothing is switched on" when this is false.
// A log switch added to FirewallOptions and forgotten here would make the page
// tell an operator who has switched it on that they have not.
func TestLogsAnythingCountsEverySwitch(t *testing.T) {
	typ := reflect.TypeOf(FirewallOptions{})
	found := 0
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		key := f.Tag.Get("toml")
		if f.Type.Kind() != reflect.Bool ||
			(!strings.HasSuffix(key, "_log") && !strings.HasPrefix(key, "log_")) {
			continue
		}
		found++
		var o FirewallOptions
		reflect.ValueOf(&o).Elem().Field(i).SetBool(true)
		if !o.LogsAnything() {
			t.Errorf("%s is on and LogsAnything() says nothing is logged", key)
		}
	}
	if found != 11 {
		t.Errorf("found %d log switches, want the eleven filters.md documents", found)
	}
	if (FirewallOptions{}).LogsAnything() {
		t.Error("nothing is switched on and LogsAnything() says otherwise")
	}
}

// The input chain is modules → blocklist → allowlist → ports → final drop
// (internal/core/nftables.go Apply; Reachable steps 5–9). An action is offered
// only where it could have changed this packet's verdict.
func remedyTCP(rule string) PacketLogEntry {
	return PacketLogEntry{Rule: rule, Proto: "tcp", DstPort: 22, Hook: "input"}
}

var remedyCases = []struct {
	e    PacketLogEntry
	want Remedy
}{
	{remedyTCP("drop"), Remedy{Allowlist: true, Blocklist: true, Open: true}},
	{remedyTCP("bogon"), Remedy{Allowlist: true, Blocklist: true}},            // the bogon filter exempts the allowlist
	{remedyTCP(PacketLogRuleOther), Remedy{Allowlist: true, Blocklist: true}}, // custom rules run after the allowlist
	{remedyTCP("blocklist"), Remedy{}},                                        // the blocklist runs before the allowlist
	{remedyTCP("feed"), Remedy{Allowlist: true, Blocklist: true}},             // the feeds run after the allowlist (spec D3)
	{remedyTCP("ssh"), Remedy{Blocklist: true}},
	{remedyTCP("syn_flood"), Remedy{Blocklist: true}},
	{remedyTCP("tcp_rst"), Remedy{Blocklist: true}},
	{remedyTCP("portscan"), Remedy{Blocklist: true}},
	{remedyTCP("invalid"), Remedy{Blocklist: true}},
	{remedyTCP("fragment"), Remedy{Blocklist: true}},
	{PacketLogEntry{Rule: "icmp_flood", Proto: "icmp"}, Remedy{Blocklist: true}},
	{PacketLogEntry{Rule: "drop", Proto: "icmp"}, Remedy{Allowlist: true, Blocklist: true}},                           // no port to open
	{PacketLogEntry{Rule: "drop", Proto: "tcp", Hook: "forward", DstPort: 25}, Remedy{}},                              // the lists and port rules are input-chain only
	{PacketLogEntry{Rule: "drop", Proto: "tcp", DstPort: 0, Hook: "input"}, Remedy{Allowlist: true, Blocklist: true}}, // tcp, but no port decoded — nothing to open
	{PacketLogEntry{Rule: "drop", Proto: "132", DstPort: 9, Hook: "input"}, Remedy{Allowlist: true, Blocklist: true}}, // SCTP: a port, but no port rule speaks it
}

func TestRemediesFollowTheChainOrder(t *testing.T) {
	for _, tc := range remedyCases {
		if got := tc.e.Remedies(); got != tc.want {
			t.Errorf("%s/%s: Remedies() = %+v, want %+v", tc.e.Rule, tc.e.Proto, got, tc.want)
		}
	}
}

// A rule added to PacketLogRules must be placed in the table above on purpose.
func TestRemediesKnowsEveryRule(t *testing.T) {
	known := map[string]bool{}
	for _, c := range remedyCases {
		known[c.e.Rule] = true
	}
	for _, r := range PacketLogRules {
		if !known[r] {
			t.Errorf("rule %q has no row in TestRemediesFollowTheChainOrder — decide where it sits in the chain", r)
		}
	}
}

// Plan P10. The prefix carries the feed's id, and only an id the catalogue or
// the own-feed slots know is believed: a custom rule logging "easywall feed:
// anything" into easywall's group is a feed row with no feed named.
func TestParseLogPrefix(t *testing.T) {
	for _, tc := range []struct{ in, rule, feed string }{
		{FeedLogPrefix("spamhaus-drop"), "feed", "spamhaus-drop"},
		{FeedLogPrefix("own-3") + "\x00", "feed", "own-3"},
		{"easywall feed: tor-exits", "feed", "tor-exits"},
		{"easywall feed: own-9 ", "feed", ""},
		{"easywall feed: ", "feed", ""},
		{"easywall feed:", "feed", ""},
		{"easywall blocklist: ", "blocklist", ""},
		{"easywall syn-flood: ", "syn_flood", ""},
		{"legacy-admin: spamhaus-drop", PacketLogRuleOther, ""},
		{"", PacketLogRuleOther, ""},
	} {
		rule, feed := ParseLogPrefix(tc.in)
		if rule != tc.rule || feed != tc.feed {
			t.Errorf("ParseLogPrefix(%q) = %q, %q; want %q, %q", tc.in, rule, feed, tc.rule, tc.feed)
		}
		if got := RuleFromPrefix(tc.in); got != tc.rule {
			t.Errorf("RuleFromPrefix(%q) = %q, want %q", tc.in, got, tc.rule)
		}
	}
}

// nfLogPrefixMax is NF_LOG_PREFIXLEN - 1: include/uapi/linux/netfilter/nf_log.h
// defines NF_LOG_PREFIXLEN 128, and net/netfilter/nft_log.c declares
// NFTA_LOG_PREFIX as NLA_STRING with .len = NF_LOG_PREFIXLEN - 1, which
// lib/nlattr.c enforces on the string without its NUL (v6.12). One byte more
// and the kernel refuses the rule, and with it the whole apply.
const nfLogPrefixMax = 127

func TestFeedLogPrefixesFitTheKernel(t *testing.T) {
	ids := []string{OwnFeedID(MaxOwnFeeds)}
	for _, f := range FeedCatalogue {
		ids = append(ids, f.ID)
	}
	for _, id := range ids {
		if p := FeedLogPrefix(id); len(p) > nfLogPrefixMax {
			t.Errorf("FeedLogPrefix(%q) is %d bytes; the kernel takes %d", id, len(p), nfLogPrefixMax)
		}
		if rule, feed := ParseLogPrefix(FeedLogPrefix(id)); rule != "feed" || feed != id {
			t.Errorf("FeedLogPrefix(%q) reads back as %q, %q", id, rule, feed)
		}
	}
}
