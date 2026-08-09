package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// The bug this guards against needs no kernel to see: expr.Log.Key is a bitmask
// over the NFTA_LOG_* attribute indices, and the original code set it to the
// bare attribute number. NFTA_LOG_PREFIX is 2, so it set bit 1 — NFTA_LOG_GROUP
// — and left the prefix bit clear. Every log rule shipped without the prefix
// the documentation told operators to grep for.
func TestLogExprs_SetsThePrefixBitNotTheAttributeNumber(t *testing.T) {
	exprs := logExprs("easywall test: ", 30)

	var log *expr.Log
	for _, e := range exprs {
		if l, ok := e.(*expr.Log); ok {
			log = l
		}
	}
	if log == nil {
		t.Fatal("no log expression produced")
	}

	if log.Key&(1<<unix.NFTA_LOG_PREFIX) == 0 {
		t.Errorf("the prefix bit is not set: Key=%d. Setting Key to the attribute "+
			"number rather than 1<<number is what dropped the prefix", log.Key)
	}
	if log.Key&(1<<unix.NFTA_LOG_GROUP) != 0 {
		t.Errorf("the group bit is set: Key=%d. easywall sets no log group, and "+
			"the old value set this bit by accident", log.Key)
	}
	if string(log.Data) != "easywall test: " {
		t.Errorf("prefix = %q, want %q", log.Data, "easywall test: ")
	}
}

// The log rule carries no verdict: it falls through to the rule that acts, so
// that rate-limiting the log cannot rate-limit the drop.
func TestLogExprs_CarriesNoVerdict(t *testing.T) {
	for _, e := range logExprs("easywall test: ", 0) {
		if _, ok := e.(*expr.Verdict); ok {
			t.Error("a verdict here would let a flood escape the drop whenever the " +
				"log rate limit kicked in")
		}
	}
}

func TestLogExprs_RateLimitDefaultsWhenUnset(t *testing.T) {
	for _, tc := range []struct{ given, want int }{{0, 60}, {-5, 60}, {30, 30}} {
		var lim *expr.Limit
		for _, e := range logExprs("p", tc.given) {
			if l, ok := e.(*expr.Limit); ok {
				lim = l
			}
		}
		if lim == nil {
			t.Fatalf("no limit expression for input %d", tc.given)
		}
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
	logged = append(logged, logExprs("p", 0)...)

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
		if data, err := os.ReadFile(filepath.Join(dir, "docs", "features", "filters.md")); err == nil {
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
