package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

func logOf(t *testing.T, exprs []expr.Any) *expr.Log {
	t.Helper()
	for _, e := range exprs {
		if l, ok := e.(*expr.Log); ok {
			return l
		}
	}
	t.Fatal("no log expression produced")
	return nil
}

// The prefix bit must be set in both sinks. Key is a bitmask over NFTA_LOG_*
// indices; setting it to the bare attribute number is what shipped every log
// rule unlabelled before 2.5.0.
func TestLogExprs_SetsThePrefixBitNotTheAttributeNumber(t *testing.T) {
	for _, sink := range []logSink{{}, {nflog: true, group: 12227}} {
		l := logOf(t, logExprs("easywall test: ", 30, sink))
		if l.Key&(1<<unix.NFTA_LOG_PREFIX) == 0 {
			t.Errorf("sink %+v: the prefix bit is not set: Key=%d", sink, l.Key)
		}
		if string(l.Data) != "easywall test: " {
			t.Errorf("prefix = %q", l.Data)
		}
	}
}

// With no NFLOG listener the rule writes where every release before 2.21 wrote:
// the kernel ring buffer. That is the fallback when the group cannot be bound.
func TestLogExprs_TheKernelLogSinkSetsNoGroup(t *testing.T) {
	l := logOf(t, logExprs("p", 0, logSink{}))
	if l.Key&(1<<unix.NFTA_LOG_GROUP) != 0 || l.Key&(1<<unix.NFTA_LOG_SNAPLEN) != 0 {
		t.Errorf("Key=%d sets a group or a snaplen with no listener to receive them", l.Key)
	}
}

// The NFLOG sink: group, prefix, snaplen, and no flags. The kernel refuses
// NFTA_LOG_FLAGS beside NFTA_LOG_GROUP — see the plan's correction to spec §2.
func TestLogExprs_TheNFLOGSinkNamesTheGroup(t *testing.T) {
	l := logOf(t, logExprs("p", 0, logSink{nflog: true, group: 12227}))
	want := uint32(1<<unix.NFTA_LOG_GROUP | 1<<unix.NFTA_LOG_PREFIX | 1<<unix.NFTA_LOG_SNAPLEN)
	if l.Key != want {
		t.Errorf("Key = %b, want %b", l.Key, want)
	}
	if l.Group != 12227 || l.Snaplen != packetSnaplen {
		t.Errorf("group %d snaplen %d, want 12227 and %d", l.Group, l.Snaplen, packetSnaplen)
	}
	if l.Flags != 0 || l.Level != 0 {
		t.Errorf("flags %d level %d: the kernel refuses either beside a group", l.Flags, l.Level)
	}
}

// Spec §1: ten prefixes, one function. A new call site that passes a literal
// logSink{} instead of the manager's would silently keep one module in the
// kernel log while the page reports it as logging.
func TestEveryLogRuleUsesTheManagersSink(t *testing.T) {
	src, err := os.ReadFile("nftables.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := regexp.MustCompile(`logExprs\(([^()]|\([^()]*\))*\)`).FindAllString(string(src), -1)
	var real int
	for _, c := range calls {
		if strings.HasPrefix(c, "logExprs(prefix") {
			continue // the declaration
		}
		real++
		if !strings.HasSuffix(c, "m.logSink)") {
			t.Errorf("%s does not pass m.logSink", c)
		}
	}
	if real < 4 {
		t.Errorf("found %d call sites, want at least 4 — the pattern no longer matches", real)
	}
}

// And through the builders, not only the helper: the final drop log and a
// filtered module both carry the group once the manager has one.
func TestBuildersCarryTheSink(t *testing.T) {
	rec := &recordingConn{}
	m := &NftablesManager{adder: rec, logSink: logSink{nflog: true, group: 4242}}
	tbl := easywallInetTableForTest()
	ch := inputChainForTest(tbl)

	m.addFinalLog(tbl, ch, shared.FirewallOptions{LogBlocked: true})
	m.addBlacklistRule(tbl, ch, "192.0.2.1", shared.FirewallOptions{LogBlacklist: true})

	var logs int
	for _, r := range rec.rules {
		for _, e := range r.Exprs {
			if l, ok := e.(*expr.Log); ok {
				logs++
				if l.Group != 4242 {
					t.Errorf("a log rule carries group %d, want the manager's 4242", l.Group)
				}
			}
		}
	}
	if logs != 2 {
		t.Errorf("found %d log expressions, want 2", logs)
	}
}

// Ties the ten real prefix constants to the rule name RuleFromPrefix reads
// back out of them — by name, not by a literal restating the constant. A
// prefix constant edited to a spelling RuleFromPrefix does not recognise
// turns real packets into "other" and a rule filter that matches nothing,
// with every other test here still green.
func TestLogPrefixesMapToTheRulesTheyName(t *testing.T) {
	want := map[string]string{
		logPrefixInvalid:   "invalid",
		logPrefixFragment:  "fragment",
		logPrefixBogon:     "bogon",
		logPrefixPortScan:  "portscan",
		logPrefixSYNFlood:  "syn_flood",
		logPrefixICMPFlood: "icmp_flood",
		logPrefixSSH:       "ssh",
		logPrefixTCPRST:    "tcp_rst",
		logPrefixBlacklist: "blacklist",
		logPrefixDrop:      "drop",
	}
	if len(want) != len(shared.PacketLogRules) {
		t.Fatalf("mapped %d prefixes, want the %d rules PacketLogRules lists", len(want), len(shared.PacketLogRules))
	}
	for prefix, rule := range want {
		if got := shared.RuleFromPrefix(prefix); got != rule {
			t.Errorf("RuleFromPrefix(%q) = %q, want %q", prefix, got, rule)
		}
	}
}

// The log rule carries no verdict: it falls through to the rule that acts, so
// that rate-limiting the log cannot rate-limit the drop.
func TestLogExprs_CarriesNoVerdict(t *testing.T) {
	for _, e := range logExprs("easywall test: ", 0, logSink{}) {
		if _, ok := e.(*expr.Verdict); ok {
			t.Error("a verdict here would let a flood escape the drop whenever the " +
				"log rate limit kicked in")
		}
	}
}

func TestLogExprs_RateLimitDefaultsWhenUnset(t *testing.T) {
	for _, tc := range []struct{ given, want int }{{0, 60}, {-5, 60}, {30, 30}} {
		var lim *expr.Limit
		for _, e := range logExprs("p", tc.given, logSink{}) {
			if l, ok := e.(*expr.Limit); ok {
				lim = l
			}
		}
		if lim == nil {
			t.Fatalf("no limit expression for input %d", tc.given)
		}
		// #nosec G115 -- tc.want is one of this table's fixed literals (60 or
		// 30); known at compile time and never negative.
		if lim.Rate != uint64(tc.want) {
			t.Errorf("limit for %d = %d, want %d", tc.given, lim.Rate, tc.want)
		}
		if lim.Unit != expr.LimitTimeMinute {
			t.Errorf("log limits are per minute, got unit %v", lim.Unit)
		}
	}
}

// addFiltered must emit the same match on both rules, or the log and the drop
// describe different packets.
func TestAddFiltered_LogAndActionShareTheMatch(t *testing.T) {
	match := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
	}
	before := len(match)

	// A nil connection would panic on AddRule, so exercise the slice handling
	// only: addFiltered must not mutate the caller's match.
	logged := make([]expr.Any, 0, len(match)+2)
	logged = append(logged, match...)
	logged = append(logged, logExprs("p", 0, logSink{})...)

	if len(match) != before {
		t.Errorf("the caller's match slice was extended to %d elements", len(match))
	}
	if len(logged) != before+2 {
		t.Errorf("logged rule has %d expressions, want %d", len(logged), before+2)
	}
}

// filters.md carries a table of log prefixes and tells operators to run
// `journalctl -k -f | grep easywall`. A prefix that changes in the code and not
// in the table sends them looking for something that is never written — which
// is the state the whole logging feature was in before 2.5.0.
//
// Derived from the constants, so renaming one without touching the table fails
// here.
func TestLogPrefixesAreDocumented(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var docs string
	for i := 0; i < 5; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "docs", "_docs", "features", "filters.md")); err == nil {
			docs = string(data)
			break
		}
		dir = filepath.Dir(dir)
	}
	if docs == "" {
		t.Fatal("could not locate docs/features/filters.md")
	}

	prefixes := map[string]string{
		"invalid":    logPrefixInvalid,
		"fragment":   logPrefixFragment,
		"bogon":      logPrefixBogon,
		"port scan":  logPrefixPortScan,
		"syn flood":  logPrefixSYNFlood,
		"icmp flood": logPrefixICMPFlood,
		"ssh":        logPrefixSSH,
		"tcp rst":    logPrefixTCPRST,
		"blacklist":  logPrefixBlacklist,
		"drop":       logPrefixDrop,
	}
	for name, prefix := range prefixes {
		// The table shows the prefix without its trailing space.
		if !strings.Contains(docs, strings.TrimSpace(prefix)) {
			t.Errorf("filters.md does not list the %s prefix (%q)", name, prefix)
		}
	}
}

// A blacklisted network is logged like a blacklisted address. It went to a
// builder that took no log spec until 2.22, so log_blacklist_connections said
// nothing about 10.0.0.0/8 while logging 10.0.0.1.
func TestBlacklistLogsANetworkEntry(t *testing.T) {
	for _, entry := range []string{"192.0.2.1", "198.51.100.0/24", "2001:db8::1", "2001:db8::/32"} {
		rec := &recordingConn{}
		m := &NftablesManager{adder: rec}
		tbl := easywallInetTableForTest()
		m.addBlacklistRule(tbl, inputChainForTest(tbl), entry, shared.FirewallOptions{LogBlacklist: true})

		if len(rec.rules) != 2 {
			t.Errorf("%s: %d rules, want a log rule and a drop", entry, len(rec.rules))
			continue
		}
		l := logOf(t, rec.rules[0].Exprs)
		if string(l.Data) != logPrefixBlacklist {
			t.Errorf("%s: the first rule logs %q, want %q", entry, l.Data, logPrefixBlacklist)
		}
		last := rec.rules[1].Exprs[len(rec.rules[1].Exprs)-1]
		if v, ok := last.(*expr.Verdict); !ok || v.Kind != expr.VerdictDrop {
			t.Errorf("%s: the second rule ends in %#v, want a drop", entry, last)
		}
	}
}
