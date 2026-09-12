package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/userdata"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// nftBinary is the nft executable, and nftTimeout bounds every call to it.
// Both are vars so a test can substitute a program that hangs.
var (
	nftBinary = "nft"
	// The value lives in shared because easywall-web needs it: two commands run
	// nft while the caller waits, so the client's deadline is derived from this
	// one. A var here so a test can substitute a shorter bound.
	nftTimeout = shared.NftTimeout
	// nftWaitDelay bounds how long Wait may keep waiting on the output pipes
	// after the process itself has been killed.
	nftWaitDelay = 2 * time.Second
)

const (
	tableName = "easywall"

	// inputChainName is the base input chain, where a host-scoped port rule is
	// written. The usage collector reads it by this name — see RuleCounters and
	// TestCollectionReadsInputAndForwardChains, which is what keeps the two from
	// drifting apart into a collector that reads a chain nothing writes.
	inputChainName = "input"

	// forwardChainName is the base forward chain, where a forwarded-scope port
	// rule is written instead — see buildForwardChain. RuleCounters reads this
	// chain too, for the same reason it reads inputChainName: a rule that
	// renders here and nowhere else must still have its counter read somewhere.
	forwardChainName = "forward"

	// nftables chain priorities
	prioFilter = 0
	prioNAT    = -100

	// nftConnlimitInvert makes a connlimit match when the count is *over* the
	// configured value rather than under it — `ct count over N`. golang.org/x/sys
	// does not export it; it is NFT_CONNLIMIT_F_INV from
	// include/uapi/linux/netfilter/nf_tables.h.
	nftConnlimitInvert = 1 << 0
)

// NftablesManager manages the easywall nftables table via netlink.
// It only ever touches "table inet easywall", leaving all other tables
// (including Docker's chains) completely untouched.
type NftablesManager struct {
	// mu serialises every access to conn.
	//
	// Every writer used to funnel through Firewall's beginApply/endApply slot,
	// one cycle at a time, so this connection never had two callers at once.
	// Panic broke that: it has to be able to tear the table down while an
	// apply already holds the slot — from the console, on demand, which is
	// the entire point of a panic button — so it calls Reset() without ever
	// taking it. Two goroutines on the same *nftables.Conn at once is not a
	// data race the type happens to have; it corrupts the protocol. AddRule
	// appends to a buffer shared by the connection, and Flush ships whatever
	// is in that buffer and empties it for whoever calls Flush next — so
	// goroutine A's Flush can send goroutine B's rules to the kernel, and
	// goroutine B's own Flush then finds nothing queued and returns nil. A
	// rollback reporting success having programmed nothing is worse than one
	// that reports failure, because nothing afterwards ever looks again.
	mu   sync.Mutex
	conn *nftables.Conn

	// adder is where the builders write. In production it is a builtRecorder
	// wrapping m.conn; in tests it is a recordingConn. Never nil after
	// NewNftablesManager.
	adder ruleAdder

	// built is every rule this transaction added, kept so CheckRules can read
	// them before the flush that sends them to the kernel. Cleared at the top of
	// Apply — not in reset(), which Reset() also calls on its own to tear the
	// table down and which adds no rules at all.
	//
	// Not a second source of truth: these are the same pointers the connection
	// holds, appended by builtRecorder so no builder has to remember to.
	built []*nftables.Rule

	// lastFindings is what CheckRules said about the last transaction. Read
	// through LastFindings, which the health status and the CI gate both use.
	lastFindings []Finding

	// checksRun counts every call to checkBuilt over this manager's life. It
	// exists for one reason: without it the CI gate passes while inspecting
	// nothing.
	//
	// Deleting both checkBuilt calls from Apply left the whole integration
	// suite green — builtRecorder still filled m.built, so the recorder's own
	// guard was satisfied, and m.lastFindings stayed nil so the gate's findings
	// loop iterated zero times and reported success. A CI gate reporting
	// success while reading nothing is the defect this release exists to
	// prevent, arriving through the guard built to prevent it.
	//
	// A count rather than a "was it nil" test, because nil-means-unchecked only
	// works while CheckRules returns a nil slice when it finds nothing — a
	// distinction no reader would know was load-bearing. And a count pins the
	// number, which is the other half: with LogBlocked on it must be exactly
	// two, because the final log rule is added after the first flush and a
	// later refactor collapsing the two checks into one would otherwise be
	// silent.
	checksRun int
}

// builtRecorder is the production adder: it records the rule and forwards it to
// the connection.
//
// It exists so the check in Apply reads what was actually built rather than a
// second reconstruction of it. Thirty-one call sites add rules across
// twenty-one builders, and asking each of them to also append to a slice is
// thirty-one chances to forget — which is the shape of the omission that let
// three byte-reversed masks ship: every individual place looked right.
type builtRecorder struct{ m *NftablesManager }

func (b builtRecorder) AddRule(r *nftables.Rule) *nftables.Rule {
	b.m.built = append(b.m.built, r)
	return b.m.conn.AddRule(r)
}

// ruleAdder is the one method every add* builder in this file uses. It exists so
// those builders can be tested without a kernel; NftablesManager.conn satisfies
// it, and nothing else in this package takes it.
//
// Thirty-one builders called m.conn.AddRule directly, which meant an
// expression-level assertion about what any of them builds could only run under
// -tags integration, as root, against a namespace — so the two things that
// matter most about a rule, the counter's position and the id in its comment,
// were the two things no test under `make test` could see. portAcceptRules was
// split out of addPortAccept for exactly that reason; this is the same seam,
// applied once rather than per builder.
type ruleAdder interface {
	AddRule(*nftables.Rule) *nftables.Rule
}

// NewNftablesManager creates a new manager and verifies netlink connectivity.
func NewNftablesManager() (*NftablesManager, error) {
	conn, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("open netlink connection: %w", err)
	}
	m := &NftablesManager{conn: conn}
	m.adder = builtRecorder{m}
	return m, nil
}

// NewNftablesManagerInNamespace is NewNftablesManager against the network
// namespace at nsFD. Only the self-test uses it.
//
// A separate constructor rather than an option on the existing one, because
// every other caller in this repository must reach the host's namespace and a
// defaulted parameter is how that stops being true — one caller forgetting to
// pass it would be a manager that silently rewrote the host's table, which is
// the one thing a health check may never do.
//
// nsFD is not dup'ed here, by the library or by us: nftables.New is
// non-lasting, so every Flush dials a fresh netlink socket with this fd in
// netlink.Config.NetNS. The caller must therefore keep the *Harness that owns
// the descriptor alive for as long as it holds this manager. A cached copy of
// Harness.NetNSFd() outliving its Close would hand setns a number the kernel
// has since recycled onto an unrelated file.
//
// m.adder is set for the same reason NewNftablesManager sets it: without the
// recording wrapper the expression check in Apply reads an empty slice, and a
// check that inspects nothing is the defect this release exists to prevent
// arriving through the guard built to prevent it.
func NewNftablesManagerInNamespace(nsFD int) (*NftablesManager, error) {
	conn, err := nftables.New(nftables.WithNetNSFd(nsFD))
	if err != nil {
		return nil, fmt.Errorf("reach nftables in the self-test namespace: %w", err)
	}
	m := &NftablesManager{conn: conn}
	m.adder = builtRecorder{m}
	return m, nil
}

// LastFindings is what the expression check said about the last table this
// manager built. Empty is the healthy answer.
//
// A copy, under the lock: the caller is the health status, on another
// goroutine, and handing out the slice would let it read a length while Apply
// is appending to it.
func (m *NftablesManager) LastFindings() []Finding {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Finding(nil), m.lastFindings...)
}

// checkBuilt runs the expression check over everything built so far and records
// what it found. The caller must hold m.mu.
//
// A finding never refuses the write. A check that read false on a configured
// host would quietly stop enforcing rules somebody wrote, which is the failure
// everConfigured exists to avoid (restore.go:218) — so this logs, it degrades
// health through LastFindings, and CI is where it is fatal:
// TestIntegration_TheBuiltTableHasNoFindings runs it over a table with every
// protection module on and fails the build.
//
// Called before each of Apply's two flushes, over the whole accumulated slice.
// Re-reading the first batch is idempotent and the rule count is in the
// hundreds; the alternative — checking only what the second flush adds — would
// need incremental bookkeeping to save nothing, and the final log rule reaching
// the kernel unchecked is exactly the shape of defect this release is about.
func (m *NftablesManager) checkBuilt() {
	m.checksRun++
	m.lastFindings = CheckRules(m.built, nil)
	for _, f := range m.lastFindings {
		slog.Error("a rule about to be written to the kernel cannot be right; "+
			"the firewall is being applied anyway and health will report degraded",
			"finding", f.String())
	}
}

// Snapshot captures the current kernel nftables state as structured JSON.
// For each table it records chain names and rule counts, providing a meaningful
// diagnostic snapshot for post-incident analysis.
//
// A chain belongs to a table by name *and family*. Matching on the name alone —
// which this did — credits every table with the chains of every same-named table
// in another family, and the numbers beside them are read from the wrong table.
// A name collision is not exotic: `easywall` in the ip family is what a
// hand-written ruleset alongside easywall looks like, and nft itself allows it.
// Measured against a kernel holding `table ip easywall` (chains input, decoy)
// and `table inet easywall` (chain input):
//
//	ip   easywall: input(1), decoy(1), input(1)   ← two chains reported as three
//	inet easywall: input(1), decoy(0), input(1)   ← one chain reported as three
//
// Each table listed the union, the `inet` table was credited with a `decoy` it
// does not have, and that entry's `0` was a failed lookup — GetRules errored and
// the count stayed at its zero value, which reads as a chain that exists and is
// empty. This file is written to log_dir on every apply and is the thing an
// operator opens after a lockout, so a chain that is not there and a rule count
// that was never read are the two worst things it could contain.
func (m *NftablesManager) Snapshot() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn == nil {
		return nil, fmt.Errorf("nftables connection not available")
	}
	tables, err := m.conn.ListTables()
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	type chainSnap struct {
		Name string `json:"name"`
		// Rules is nil when the count could not be read. Distinguished from 0 on
		// purpose: "empty" and "not known" are different states, and conflating
		// them is what made a chain that does not exist look like an empty one.
		Rules *int   `json:"rules"`
		Error string `json:"error,omitempty"`
	}
	type tableSnap struct {
		Name   string      `json:"name"`
		Family string      `json:"family"`
		Chains []chainSnap `json:"chains"`
		Error  string      `json:"error,omitempty"`
	}

	// Once, not once per table. This was inside the loop, which asked the kernel
	// for the whole chain list again for every table it had.
	chains, chainsErr := m.conn.ListChains()

	tableSnaps := make([]tableSnap, 0, len(tables))
	for _, tbl := range tables {
		snap := tableSnap{Name: tbl.Name, Family: tableFamilyName(tbl.Family)}
		if chainsErr != nil {
			snap.Error = "list chains: " + chainsErr.Error()
			tableSnaps = append(tableSnaps, snap)
			continue
		}
		for _, ch := range chains {
			if ch.Table == nil || ch.Table.Name != tbl.Name || ch.Table.Family != tbl.Family {
				continue
			}
			cs := chainSnap{Name: ch.Name}
			rules, err := m.conn.GetRules(tbl, ch)
			if err != nil {
				cs.Error = err.Error()
			} else {
				n := len(rules)
				cs.Rules = &n
			}
			snap.Chains = append(snap.Chains, cs)
		}
		tableSnaps = append(tableSnaps, snap)
	}

	out := struct {
		Timestamp string      `json:"timestamp"`
		Tables    []tableSnap `json:"tables"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tables:    tableSnaps,
	}
	return json.Marshal(out)
}

// tableFamilyName maps a nftables TableFamily constant to its canonical string name.
func tableFamilyName(f nftables.TableFamily) string {
	switch f {
	case nftables.TableFamilyINet:
		return "inet"
	case nftables.TableFamilyIPv4:
		return "ip"
	case nftables.TableFamilyIPv6:
		return "ip6"
	case nftables.TableFamilyARP:
		return "arp"
	case nftables.TableFamilyNetdev:
		return "netdev"
	case nftables.TableFamilyBridge:
		return "bridge"
	default:
		return "unspecified"
	}
}

// Enforcing reports whether the kernel is actually carrying easywall's rules:
// the table exists, it has an input chain, and that chain has rules in it.
//
// The dashboard used to derive this from the fact that the daemon was running,
// and told the operator "the core daemon is running and rules are live" on that
// basis alone. Those are different claims. After `nft delete table inet
// easywall`, or an apply that failed and whose rollback also failed, the daemon
// is up and nothing is being enforced — and the dashboard showed green.
//
// An error reading the state is reported as not enforcing. Being unable to
// confirm that a firewall is up is not the same as it being up, and the
// dashboard should say so.
func (m *NftablesManager) Enforcing() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn == nil {
		return false
	}

	table := &nftables.Table{Name: tableName, Family: nftables.TableFamilyINet}
	tables, err := m.conn.ListTables()
	if err != nil {
		slog.Warn("could not list nftables tables", "error", err)
		return false
	}
	found := false
	for _, tbl := range tables {
		if tbl.Name == tableName && tbl.Family == nftables.TableFamilyINet {
			table = tbl
			found = true
			break
		}
	}
	if !found {
		return false
	}

	chains, err := m.conn.ListChains()
	if err != nil {
		slog.Warn("could not list nftables chains", "error", err)
		return false
	}
	for _, ch := range chains {
		if ch.Table == nil || ch.Table.Name != tableName || ch.Name != inputChainName {
			continue
		}
		rules, err := m.conn.GetRules(table, ch)
		if err != nil {
			slog.Warn("could not read input chain rules", "error", err)
			return false
		}
		// An empty input chain means the table was recreated but never filled —
		// the policy would drop everything, which is not "live rules" either.
		return len(rules) > 0
	}
	return false
}

// RuleCounter is what one rule id's kernel rules have counted between them.
type RuleCounter struct {
	Packets uint64
	Bytes   uint64
}

// RuleCounters reads the packet counters off the input and forward chains and
// sums them by rule id.
//
// One UI rule with three sources is three kernel rules — addPortAccept builds
// one per source — so the figure for that port is their sum, and the id in each
// rule's comment is what makes summing them possible. A rule with no comment is
// skipped: the module rules, the blacklist, the whitelist and the Docker
// exceptions all carry no id and are not what this counts. A scope = "both"
// rule is the same arithmetic one chain wider: one kernel rule per chain, same
// id, summed the same way — the rule is one rule, and the packets it accepted
// are the packets it accepted, wherever they crossed.
//
// Only these two chains. addPortAccept and addForwardPortRules write there and
// nowhere else, and TestCollectionReadsInputAndForwardChains pins both halves
// of that so the collector cannot quietly start reading a chain nothing writes
// into — which would report "never" for every port for ever, with no error
// anywhere.
//
// addEstablishedAccept tags only the input chain's copy of the established-
// accept rule, precisely so that this function does not sum input and forward
// traffic under the one reserved id — see its doc comment. That the forward
// chain is now read here as well makes that decision load-bearing rather than
// moot: before this, a forward copy tagged by mistake went unread anyway; now
// it would double count.
//
// A missing table is an empty map and no error. Panic mode deletes the table on
// purpose, and a collector that treated that as a failure would log one line
// per tick for as long as the machine stays deliberately unfiltered.
func (m *NftablesManager) RuleCounters() (map[string]RuleCounter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn == nil {
		return nil, fmt.Errorf("nftables connection not available")
	}

	tables, err := m.conn.ListTables()
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	var table *nftables.Table
	for _, tbl := range tables {
		if tbl.Name == tableName && tbl.Family == nftables.TableFamilyINet {
			table = tbl
			break
		}
	}
	if table == nil {
		return map[string]RuleCounter{}, nil
	}

	chains, err := m.conn.ListChains()
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	out := map[string]RuleCounter{}
	for _, ch := range chains {
		if ch.Table == nil || ch.Table.Name != tableName ||
			ch.Table.Family != nftables.TableFamilyINet {
			continue
		}
		if ch.Name != inputChainName && ch.Name != forwardChainName {
			continue
		}
		rules, err := m.conn.GetRules(table, ch)
		if err != nil {
			return nil, fmt.Errorf("read the %s chain: %w", ch.Name, err)
		}
		for _, r := range rules {
			id, ok := idFromUserData(r.UserData)
			if !ok || id == "" {
				continue
			}
			c := out[id]
			for _, e := range r.Exprs {
				if counter, isCounter := e.(*expr.Counter); isCounter {
					c.Packets += counter.Packets
					c.Bytes += counter.Bytes
				}
			}
			out[id] = c
		}
	}
	return out, nil
}

// idFromUserData reads the rule-id comment out of a kernel rule's UserData,
// treating malformed TLV bytes as "no id" rather than letting them reach the
// caller as a panic.
//
// userdata.Get (nftables v0.3.0, userdata/userdata.go:62) slices
// udata[2:2+length] before it checks len(udata) < 2+length, so a truncated
// or corrupt TLV panics instead of returning an error. RuleCounters cannot
// assume every rule in the input chain is one of ours — Snapshot's own doc
// comment concedes a hand-written ruleset can share this table and chain
// namespace — and this runs in the root daemon, on a ticker and inside
// apply(), so a single bad comment must not be able to take it down.
func idFromUserData(userData []byte) (id string, ok bool) {
	defer func() {
		if recover() != nil {
			id, ok = "", false
		}
	}()
	return userdata.GetString(userData, userdata.TypeComment)
}

// Reset deletes and recreates the easywall table, giving us a clean slate.
// All other tables (filter, nat, docker, etc.) are untouched.
func (m *NftablesManager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reset()
}

// reset is Reset's body, split out so Apply can call it without taking mu a
// second time.
//
// Apply used to call Reset() directly. Once Apply itself took the lock for
// the whole cycle, that became Apply calling m.mu.Lock() and then, from
// inside the same goroutine, Reset() calling m.mu.Lock() again — a plain
// sync.Mutex is not reentrant, so the second Lock never returns. That is a
// worse failure than anything this mutex exists to prevent: a wedged apply
// does not time out, does not release beginApply's slot, and every method on
// this type that also takes mu — including the one the dashboard's Status
// polls every few seconds — hangs behind it forever. reset() assumes the
// caller already holds mu; only Reset() and Apply() may call it.
func (m *NftablesManager) reset() error {
	if m.conn == nil {
		return fmt.Errorf("nftables connection not available")
	}
	m.conn.DelTable(&nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyINet,
	})
	if err := m.conn.Flush(); err != nil {
		// Ignore "no such table" errors — table may not exist yet.
		_ = err
	}

	m.conn.AddTable(&nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyINet,
	})
	return m.conn.Flush()
}

// Apply translates the given RulesState and FirewallOptions into nftables
// rules and installs them atomically via a single netlink Flush call.
//
// netCfg carries the IPv6 disposition, Docker coexistence and the routing mode
// as one value. They used to arrive as separate parameters and a third was one
// parameter too many — the caller already holds them together, and the three
// of them describe one configuration that has to reach the kernel intact.
func (m *NftablesManager) Apply(state shared.RulesState, opts shared.FirewallOptions, netCfg shared.NetworkSettings) error {
	// Held for the whole cycle, deliberately including applyCustomRules'
	// subprocess below — up to NftTimeout (30s) — and not just the netlink
	// calls. The custom rules assume the table Apply just built is still
	// there; releasing the lock in between would let a concurrent Reset()
	// (Panic, from the console) delete it out from under the `nft -f -` that
	// is still writing to it.
	//
	// The cost is on the read side: Enforcing(), which Status() calls, and
	// Snapshot() both block for as long as this holds the lock — worst case a
	// little over 30s, when a custom rule set is slow or the process has to be
	// killed and waited out.
	//
	// Two commands reach this manager from the dispatch switch while an apply
	// holds the lock, and they want opposite things:
	//
	//   - GET_STATUS, through Status() -> Enforcing(), which asks the kernel
	//     whether the rules are live rather than inferring it from anything in
	//     memory. It has the short shared.CommandTimeout (5s), so it is not
	//     "the dashboard waits 30 seconds": the web process's read deadline
	//     expires first and the operator is told the core is unreachable, until
	//     the custom-rules step finishes and the next poll gets through.
	//   - PANIC, through Firewall.Panic -> Reset(). It deliberately bypasses
	//     beginApply so that it outranks a running apply rather than failing
	//     fast, which means it *will* queue on this lock for up to NftTimeout.
	//     That is why CommandTimeout puts it on the long deadline — three
	//     commands are on it now, IMPORT_RULES, VALIDATE_CUSTOM and PANIC, not
	//     the two nft-backed ones this comment used to name.
	//
	// APPLY_RULES and RESUME also end at this manager, but never behind this
	// lock: both go through beginApply, which refuses immediately with
	// ErrApplyInProgress while a cycle is running. Every other command touches
	// only the rules store, the config or the audit log, on its own connection's
	// goroutine, so nothing else is dragged behind this lock.
	//
	// One consequence for anyone tidying this up: Firewall.panicLandedDuringWrite
	// calls Reset() straight after this method returns, which is safe only
	// because the deferred Unlock below has already run. Moving that check inside
	// Apply would take mu twice from one goroutine and wedge the daemon — the
	// same non-reentrancy that produced reset() (see its comment).
	//
	// Correctness is not optional here; a status page that reports "unreachable"
	// for a few seconds during a slow custom-rules apply is a cost worth paying
	// for it — the alternative is Reset() deleting a table a subprocess is still
	// writing to.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Here rather than in reset(), which Reset() also calls on its own: this is
	// the start of one transaction's worth of rules, and it is the only place
	// that is true of.
	m.built = nil

	ipv6, docker, routing := netCfg.IPv6, netCfg.Docker, netCfg.Routing
	// Check the rules before Reset, not after: Reset deletes the table, so a
	// failure past this point costs the working ruleset. The builders below
	// each guard their own parsing and return quietly when an address will not
	// parse — which used to mean a malformed entry was listed in the interface
	// as blocked while no rule for it ever existed. Refusing here makes that
	// impossible to reach, and leaves the previous rules in place.
	if err := shared.ValidateRules(state.Current); err != nil {
		return fmt.Errorf("refusing to apply: %w", err)
	}

	// A zero-valued IPv6Config must mean "filter", not "some fourth thing".
	// Config.Validate fills the mode in for the daemon, but Apply is also
	// reachable with a struct built by hand, and an unset mode skipping the
	// ICMPv6 rules while every other rule still applied to IPv6 is precisely
	// the behaviour the mode was introduced to remove.
	if !ipv6.Mode.Valid() {
		if ipv6.Mode != "" {
			slog.Warn("unknown ipv6 mode; filtering", "mode", ipv6.Mode)
		}
		ipv6.Mode = shared.IPv6Filter
	}

	// And a zero-valued RoutingConfig must mean "closed", for the same reason.
	// Falling through to "open" on an unset field would hand a host the one
	// disposition nobody asked for.
	if !routing.Mode.Valid() {
		if routing.Mode != "" {
			slog.Warn("unknown routing mode; routing nothing", "mode", routing.Mode)
		}
		routing.Mode = shared.RoutingClosed
	}

	if err := m.reset(); err != nil {
		return fmt.Errorf("reset table: %w", err)
	}

	table := &nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyINet,
	}

	// --- INPUT chain (base, default DROP) ---
	inputChain := m.conn.AddChain(&nftables.Chain{
		Name:     inputChainName,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityRef(prioFilter),
		Policy:   policyDrop(),
	})

	// --- OUTPUT chain (base, default ACCEPT) ---
	m.conn.AddChain(&nftables.Chain{
		Name:     "output",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityRef(prioFilter),
		Policy:   policyAccept(),
	})

	// --- FORWARD chain (base) ---
	//
	// An empty base chain at a hook is not "no opinion": the policy is the
	// verdict, so a drop here destroys everything the host would forward, and it
	// beats an accept another table's forward chain has already made. That is
	// what routing.mode is for. Under "open" the policy is accept and no rules
	// are needed; otherwise it drops and addForwardExceptions names what may
	// cross.
	forwardPolicy := policyDrop()
	if routing.Mode == shared.RoutingOpen {
		forwardPolicy = policyAccept()
	}
	forwardChain := m.conn.AddChain(&nftables.Chain{
		Name:     forwardChainName,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityRef(prioFilter),
		Policy:   forwardPolicy,
	})

	// Base INPUT rules
	m.addLoopbackAccept(table, inputChain)

	// IPv6 disposition comes immediately after loopback and before everything
	// else, because passthrough and block are statements about all IPv6 traffic
	// and a later rule would only see what earlier ones left. Loopback stays
	// first either way: dropping ::1 breaks local services, which is nobody's
	// idea of "block IPv6".
	switch ipv6.Mode {
	case shared.IPv6Passthrough:
		m.addFamilyVerdict(table, inputChain, unix.NFPROTO_IPV6, expr.VerdictAccept)
	case shared.IPv6Block:
		m.addFamilyVerdict(table, inputChain, unix.NFPROTO_IPV6, expr.VerdictDrop)
	}

	m.addEstablishedAccept(table, inputChain)
	m.addICMPRules(table, inputChain, ipv6)

	// Which Docker networks are allowed is settled before the modules run,
	// because the bogon filter has to know about them: it drops RFC-1918
	// sources, and a bridge network is one.
	var dockerCIDRs []string
	if docker.Enabled {
		if docker.AllowBridgeNetworks {
			dockerCIDRs = append(dockerCIDRs, detectDockerBridges()...)
		}
		dockerCIDRs = append(dockerCIDRs, docker.CustomNetworks...)
	}

	// The whole forward chain, in one call, because its order is the correctness
	// of 2.19 and nothing here can be unit-tested. See buildForwardChain.
	m.buildForwardChain(table, forwardChain, state.Current, docker, routing, dockerCIDRs)

	// Optional protection modules
	if opts.PortScan {
		m.addPortScanPrevention(table, inputChain, opts)
	}
	if opts.SYNFlood {
		m.addSYNFloodProtection(table, inputChain, opts)
	}
	if opts.InvalidPackets {
		m.addInvalidPacketDrop(table, inputChain, opts)
	}
	if opts.Fragments {
		m.addFragmentDrop(table, inputChain, opts)
	}
	if opts.Bogons {
		m.addBogonFilter(table, inputChain, opts,
			append(append([]string(nil), state.Current.Whitelist...), dockerCIDRs...))
	}
	if opts.ICMPFlood {
		m.addICMPFloodProtection(table, inputChain, opts)
	}
	if opts.SSHBruteForce {
		m.addSSHBruteForce(table, inputChain, state.Current, opts)
	}
	if opts.TCPRSTFlood {
		m.addTCPRSTFlood(table, inputChain, opts)
	}
	if opts.ConnectionLimit {
		m.addConnectionLimit(table, inputChain, opts)
	}
	if opts.DropBroadcast {
		m.addBroadcastDrop(table, inputChain)
	}
	if opts.DropMulticast {
		m.addMulticastDrop(table, inputChain)
	}
	if opts.DropAnycast {
		m.addAnycastDrop(table, inputChain)
	}

	// Docker bridge whitelisting
	for _, cidr := range dockerCIDRs {
		m.addCIDRAccept(table, inputChain, cidr)
	}

	// Blacklist (DROP before whitelist)
	for _, ip := range state.Current.Blacklist {
		m.addBlacklistRule(table, inputChain, ip, opts)
	}

	// Whitelist (ACCEPT specific sources)
	for _, ip := range state.Current.Whitelist {
		m.addWhitelistRule(table, inputChain, ip)
	}

	// Open TCP / UDP ports.
	//
	// FiltersHost and FiltersForwarded are the two halves of one decision — a
	// rule's scope says which chain or chains it belongs in, and this is the
	// input chain's half of asking it. Skipping this the way
	// addForwardPortRules already asks FiltersForwarded was the gap: without
	// it, every rule reached the input chain regardless of scope, so a
	// scope = "forwarded" rule meant for a container was also opened on the
	// host directly — the opposite of what the operator asked for, and
	// invisible in the diff because the forward chain still looked right. A
	// rule reaching neither chain, or both when it asked for one, is a rule
	// whose behaviour does not match what the interface shows for it.
	for _, rule := range state.Current.TCP {
		if !rule.FiltersHost() {
			continue
		}
		m.addPortAccept(table, inputChain, "tcp", rule)
	}
	for _, rule := range state.Current.UDP {
		if !rule.FiltersHost() {
			continue
		}
		m.addPortAccept(table, inputChain, "udp", rule)
	}

	// Port forwarding (NAT)
	if len(state.Current.Forwarding) > 0 {
		m.addForwardingRules(table, state.Current.Forwarding)
	}

	m.checkBuilt()
	if err := m.conn.Flush(); err != nil {
		return err
	}
	// Apply custom rules via nft subprocess after all typed rules are committed.
	if len(state.Current.Custom) > 0 {
		if err := m.applyCustomRules(state.Current.Custom); err != nil {
			slog.Warn("custom rules apply warning", "error", err)
			return fmt.Errorf("apply custom rules: %w", err)
		}
	}

	// The log of what the policy drops goes last, in its own flush, because the
	// custom rules above are appended by the nft CLI after everything netlink
	// wrote. Adding it before them put it in front of rules that accept: a
	// packet a custom rule let in was written to the kernel log as
	// "easywall drop:" first and then accepted, so the line an operator greps
	// for named traffic that was never dropped. filters.md describes this as
	// "everything the final policy drops", and now it is.
	if opts.LogBlocked {
		m.addFinalLog(table, inputChain, opts)
		// The second check, and it is not ceremony. This rule is added after
		// the first flush has already gone out, so a check at that flush alone
		// would let it reach the kernel unread — one builder's output exempt
		// from the guard, which is precisely how three reversed masks lived for
		// five releases. TestIntegration_TheBuiltTableHasNoFindings asserts
		// checksRun == 2 so that collapsing the two is not silent.
		m.checkBuilt()
		if err := m.conn.Flush(); err != nil {
			return fmt.Errorf("add final log rule: %w", err)
		}
	}
	return nil
}

// applyCustomRules appends raw nftables expression strings to the input chain.
// The Go netlink library accepts only typed expressions, so custom rules are
// applied via the nft CLI using the existing table/chain created by Apply().
func (m *NftablesManager) applyCustomRules(rules []string) error {
	var cmds []string
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue
		}
		cmds = append(cmds, "add rule inet "+tableName+" input "+rule)
	}
	if len(cmds) == 0 {
		return nil
	}
	// Bounded. This runs inside Firewall.Apply, which holds the apply mutex for
	// the whole cycle — so an nft that never returns does not just fail this
	// apply, it wedges every future one, and Stop waits on the same goroutine.
	// A firewall manager that can never change the firewall again, and cannot be
	// shut down either, is a worse outcome than a failed apply.
	ctx, cancel := context.WithTimeout(context.Background(), nftTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, nftBinary, "-f", "-")
	// Killing the process is not enough on its own: CombinedOutput waits for the
	// output pipes to close, and anything the child spawned still holds them.
	// WaitDelay bounds that too, so a cancelled command really does return.
	cmd.WaitDelay = nftWaitDelay
	cmd.Stdin = strings.NewReader(strings.Join(cmds, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("nft custom rules: timed out after %s", nftTimeout)
	}
	if err != nil {
		return fmt.Errorf("nft custom rules: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// There is deliberately no Restore here. One existed until 2.5.0: it took a
// snapshot argument, ignored it, and returned nil — a function shaped exactly
// like a recovery path that recovered nothing. Rollback is
// rules.Rollback() followed by Apply(previous), in Firewall.rollback, and a
// snapshot is written to the log directory for post-incident reading only.

// --- Logging ---

// Log prefixes. Every one starts with "easywall " so that a single
// `journalctl -k | grep easywall` catches all of them, which is what the
// documentation tells operators to run.
const (
	logPrefixInvalid   = "easywall invalid: "
	logPrefixFragment  = "easywall fragment: "
	logPrefixBogon     = "easywall bogon: "
	logPrefixPortScan  = "easywall portscan: "
	logPrefixSYNFlood  = "easywall syn-flood: "
	logPrefixICMPFlood = "easywall icmp-flood: "
	logPrefixSSH       = "easywall ssh: "
	logPrefixTCPRST    = "easywall tcp-rst: "
	logPrefixBlacklist = "easywall blacklist: "
	logPrefixDrop      = "easywall drop: "
)

// logSpec describes the optional log rule that precedes a module's action.
type logSpec struct {
	enabled   bool
	prefix    string
	perMinute int
}

// logExprs builds a rate-limited log expression pair.
//
// expr.Log.Key is a bitmask over the NFTA_LOG_* attribute indices, not an
// attribute number. Setting it to unix.NFTA_LOG_PREFIX (2) sets the bit for
// NFTA_LOG_GROUP (1<<1) and leaves the prefix bit (1<<2) clear, so the kernel
// received an empty log group and no prefix at all. That is how every logged
// packet reached the kernel log unlabelled, while the documentation told
// operators to grep for a prefix that was never written.
func logExprs(prefix string, perMinute int) []expr.Any {
	if perMinute <= 0 {
		perMinute = 60
	}
	return []expr.Any{
		// Rate-limits the log line, not the verdict: this rule carries no
		// verdict of its own and falls through to the one that acts. A flood
		// must not be able to fill the disk, and must not escape the drop
		// either.
		&expr.Limit{
			Type:  expr.LimitTypePkts,
			Rate:  uint64(perMinute),
			Over:  false,
			Unit:  expr.LimitTimeMinute,
			Burst: uint32(perMinute),
		},
		&expr.Log{
			Key:  1 << unix.NFTA_LOG_PREFIX,
			Data: []byte(prefix),
		},
	}
}

// addFiltered installs a module's rule as up to two rules sharing one match:
// an optional rate-limited log rule that falls through, followed by the rule
// that acts. Both carry the same match, so what is logged is exactly what is
// dropped.
func (m *NftablesManager) addFiltered(t *nftables.Table, c *nftables.Chain, match []expr.Any, action expr.Any, lg logSpec) {
	if lg.enabled {
		logged := make([]expr.Any, 0, len(match)+2)
		logged = append(logged, match...)
		logged = append(logged, logExprs(lg.prefix, lg.perMinute)...)
		m.adder.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: logged})
	}
	acted := make([]expr.Any, 0, len(match)+1)
	acted = append(acted, match...)
	acted = append(acted, action)
	m.adder.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: acted})
}

// --- Helper builders ---

// addFamilyVerdict accepts or drops an entire address family in one rule.
// Used for the IPv6 passthrough and block modes, where the point is that no
// later rule gets a say.
func (m *NftablesManager) addFamilyVerdict(t *nftables.Table, c *nftables.Chain, family byte, kind expr.VerdictKind) {
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{family}},
			&expr.Verdict{Kind: kind},
		},
	})
}

func (m *NftablesManager) addLoopbackAccept(t *nftables.Table, c *nftables.Chain) {
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte("lo\x00")},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

// ── Conntrack state matching ────────────────────────────────────────────────
//
// The kernel writes ct state into a register as a **native** u32, so the mask
// a bitwise expression compares it against has to be in native byte order too.
// nft's own code generation is the reference:
//
//	$ nft --debug=netlink -c -f - <<<'... ct state established,related accept ...'
//	[ bitwise reg 1 = ( reg 1 & 0x00000006 ) ^ 0x00000000 ]
//
// All three masks in this file were written byte-reversed instead, as
// []byte{0x00, 0x00, 0x00, 0x06}. The kernel renders that back as
// `ct state 0x2000000,0x4000000` — bits no conntrack state ever sets — so the
// rule matched nothing at all, silently, while `nft list ruleset` showed a rule
// that looked present.
//
// What the established/related one cost, which is the whole input chain's
// stateful half: a reply packet had no rule to match, so an installation with a
// live table could not complete a single outbound connection — no DNS, no
// `apt update`, no version check — and applying the table dropped whatever SSH
// session was already open, because the only rules still matching were the
// stateless `dport N accept` ones. Measured in a veth pair against an HTTP
// server: 0 packets on the reversed rule and 6 on the corrected one, for the
// same request. addInvalidPacketDrop and addSSHBruteForce were the same
// mistake failing open — two protection modules that reported themselves on
// and enforced nothing.
//
// binaryutil.NativeEndian is what the rest of this file already uses for a
// register-width constant (see addAnycastDrop); these three were the outliers.
const (
	ctStateInvalid     = 0x01
	ctStateEstablished = 0x02
	ctStateRelated     = 0x04
	ctStateNew         = 0x08
)

// ctStateMask returns the register mask that matches any of the given ct state
// bits, in the byte order the kernel actually compares against.
func ctStateMask(bits uint32) []byte {
	return binaryutil.NativeEndian.PutUint32(bits)
}

// ctStateNoMatch is the zero a masked ct state is compared against. Byte order
// does not enter into it — every byte is zero — so it is named rather than
// repeated as a literal beside three masks where order is the whole point.
var ctStateNoMatch = []byte{0x00, 0x00, 0x00, 0x00}

// Reserved rule ids name a rule easywall builds for its own accounting rather
// than one an operator wrote. They are the only ids RuleCounters reports that
// no rules file contains, and the prefix is what keeps them out of usage.json:
// UsageStore.Collect books only ids liveRuleIDs names, and liveRuleIDs reads
// the rules file. The prefix makes that separation stateable rather than
// emergent.
const ReservedIDEstablished = "_established"

// IsReservedRuleID reports whether id is one of easywall's own.
func IsReservedRuleID(id string) bool {
	return strings.HasPrefix(id, "_")
}

// addEstablishedAccept is the stateful half of a chain, and in the input chain
// it is the one rule whose counter is worth reading on its own.
//
// It carried neither a counter nor a comment until 2.17, so RuleCounters — which
// skips every rule without an id — never saw it, and the number that would have
// caught the byte-reversed mask described above was the one number nothing in
// this daemon could read. On a host with any traffic at all the counter can only
// be zero if this rule matches nothing, which is exactly the signature of that
// defect: a table that looks right in `nft list ruleset` and completes no
// outbound connection.
//
// **Only the input chain's copy is tagged.** There are two callers: Apply adds
// this to the input chain, and buildForwardChain adds it at the top of the
// forward chain — so that a reply is not re-tested against the routed networks,
// and, since 2.19, so that nothing below it can deny one. A container's
// outbound connection comes back with the bridge address as its *destination*
// once conntrack has undone Docker's masquerade, which is exactly what
// addForwardPortRules' deny matches; behind that deny, every container on the
// host loses the network at the next apply. Tagging both
// would put one id on two different rules, and RuleCounters — which reads both
// the input and the forward chain — would sum them and report input and
// forward traffic as one figure. An id that is unique only because its one
// reader happens to filter to a chain nothing forward-scoped ever reached is
// unique by luck, and today RuleCounters does reach the forward chain: a
// metrics endpoint or outbound rules would each break it silently otherwise,
// which is the class of defect this release exists to prevent.
//
// So do not add the tag to the forward copy for symmetry. Untagged puts it in
// exactly the category it belongs to, beside the module rules, the blacklist,
// the whitelist and the Docker exceptions: rules with a counter, no id, and
// nothing reading them by name. The counter is on both, because it costs
// nothing and `nft list ruleset` is where an operator looks.
//
// The id is a reserved one, so nothing books it as a port rule. See
// ReservedIDEstablished.
func (m *NftablesManager) addEstablishedAccept(t *nftables.Table, c *nftables.Chain) {
	var tag []byte
	if c != nil && c.Name == inputChainName {
		tag = userdata.AppendString(nil, userdata.TypeComment, ReservedIDEstablished)
	}
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Ct{Register: 1, SourceRegister: false, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           ctStateMask(ctStateEstablished | ctStateRelated),
				Xor:            ctStateNoMatch,
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
			// After the match, before the verdict — the position portAcceptRules
			// documents and for the same reason. Ahead of the match this counts
			// every packet that reaches the rule rather than every packet it
			// accepts, which rises steadily on a host whose stateful half matches
			// nothing and is indistinguishable from a healthy figure.
			&expr.Counter{},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
		UserData: tag,
	})
}

func (m *NftablesManager) addICMPRules(t *nftables.Table, c *nftables.Chain, ipv6 shared.IPv6Config) {
	// ICMPv4 types to accept
	icmpv4Types := []byte{0, 3, 11, 12}
	for _, icmpType := range icmpv4Types {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_ICMP}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseTransportHeader,
					Offset:       0,
					Len:          1,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{icmpType}},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
		})
	}

	// Only filter mode consults these: under passthrough IPv6 was already
	// accepted, under block it was already dropped, and either way this rule
	// would never be reached.
	if ipv6.Mode != shared.IPv6Filter {
		return
	}

	// ICMPv6 types to accept
	icmpv6Types := []byte{1, 2, 3, 4, 128, 129}
	if ipv6.ICMPAllowRouterAdvertisement {
		icmpv6Types = append(icmpv6Types, 133, 134)
	}
	if ipv6.ICMPAllowNeighborAdvertisement {
		icmpv6Types = append(icmpv6Types, 135, 136)
	}

	for _, icmpType := range icmpv6Types {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_ICMPV6}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseTransportHeader,
					Offset:       0,
					Len:          1,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{icmpType}},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
		})
	}
}

func (m *NftablesManager) addInvalidPacketDrop(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	match := []expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           ctStateMask(ctStateInvalid),
			Xor:            ctStateNoMatch,
		},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
	}
	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop},
		logSpec{enabled: opts.InvalidPacketsLog, prefix: logPrefixInvalid})
}

func (m *NftablesManager) addFragmentDrop(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	// Drop fragmented IPv4 packets (offset > 0 or MF flag set)
	match := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       6, // fragment offset field
			Len:          2,
		},
		// Check if fragment offset bits are non-zero (fragmented packet)
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            2,
			Mask:           []byte{0x3f, 0xff},
			Xor:            []byte{0x00, 0x00},
		},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0x00, 0x00}},
	}
	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop},
		logSpec{enabled: opts.FragmentsLog, prefix: logPrefixFragment})
}

// addBogonFilter drops sources that cannot legitimately reach a public
// interface — with an exception for the ones the operator has said can.
//
// exempt holds the whitelist and the Docker bridge networks. They are needed
// here because both are lists of RFC-1918 addresses, which is exactly what this
// module drops, and it runs first: with the filter on, whitelisting 192.168.1.0/24
// or letting Docker's 172.17.0.0/16 through did nothing at all, because the
// packet was already gone by the time either rule was reached. Measured against
// a kernel — the drop for 172.16.0.0/12 sat at position 17 and the accept for
// 172.17.0.0/16 at 23.
//
// The exceptions go in front of the drops rather than the whole module moving
// after the whitelist, because the order the rest of the chain runs in is
// documented on four pages and is right: a protection module *should* see a
// packet before an accept rule does. What was wrong is narrower than that. This
// module's premise is "nothing legitimately has this source address", and an
// operator who whitelists a private network has just said otherwise about part
// of it. Everything else in the range is still dropped.
//
// The drops live in their own chain so an exception can `return` from it and
// carry on down the input chain; a `return` in a base chain would fall through
// to the drop policy instead, which is the opposite of an exception.
func (m *NftablesManager) addBogonFilter(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions, exempt []string) {
	bogonChain := m.conn.AddChain(&nftables.Chain{Name: "bogon", Table: t})

	// The exceptions come first, so a source the operator has allowed leaves
	// this chain before any drop can see it and carries on down the input chain.
	for _, cidr := range exempt {
		match := ipv4SourceMatch(cidr)
		if match == nil {
			continue // a comment, an IPv6 address, or unparseable — not for this filter
		}
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: bogonChain,
			Exprs: append(match, &expr.Verdict{Kind: expr.VerdictReturn}),
		})
	}

	// The eleven ranges are shared.BogonRanges — one list, so the rules this
	// builds and the verdict internal/shared reasons about cannot drift. See the
	// comment on that variable.
	for _, cidr := range shared.BogonRanges {
		match := ipv4SourceMatch(cidr)
		if match == nil {
			continue
		}
		m.addFiltered(t, bogonChain, match, &expr.Verdict{Kind: expr.VerdictDrop},
			logSpec{enabled: opts.BogonsLog, prefix: logPrefixBogon})
	}

	// The family and interface tests sit on the jump rather than on each of the
	// eleven drops. They have to be somewhere: the source-address offset below
	// is only correct for IPv4, and loopback legitimately carries 127.0.0.0/8.
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte("lo\x00")},
			&expr.Verdict{Kind: expr.VerdictJump, Chain: bogonChain.Name},
		},
	})
}

// ipv4SourceMatch matches an IPv4 source address or network. A bare address is
// treated as a /32. Returns nil for anything that is not an IPv4 entry —
// comments, IPv6, and text that does not parse — so callers can skip it.
func ipv4SourceMatch(entry string) []expr.Any {
	if shared.IsListComment(entry) {
		return nil
	}
	entry = strings.TrimSpace(entry)

	var ipNet *net.IPNet
	if ip := net.ParseIP(entry); ip != nil {
		ip4 := ip.To4()
		if ip4 == nil {
			return nil
		}
		ipNet = &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
	} else {
		_, parsed, err := net.ParseCIDR(entry)
		if err != nil || parsed.IP.To4() == nil {
			return nil
		}
		ipNet = &net.IPNet{IP: parsed.IP.To4(), Mask: parsed.Mask}
	}

	return []expr.Any{
		// The family test is redundant inside a chain only reached for IPv4, and
		// it stays because nft needs it to print the rule as `ip saddr
		// 10.0.0.0/8` rather than `@nh,96,32 & 0xff000000 == 0xa000000`. An
		// operator checking the firewall reads `nft list ruleset`, and a rule
		// they cannot read is one they cannot check.
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       12, // src IP offset in IPv4 header
			Len:          4,
		},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           []byte(ipNet.Mask),
			Xor:            []byte{0, 0, 0, 0},
		},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ipNet.IP},
	}
}

func (m *NftablesManager) addPortScanPrevention(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	// Named chain for port scan drops
	scanChain := m.conn.AddChain(&nftables.Chain{
		Name:  "portscan",
		Table: t,
	})
	// The log goes inside the chain, before its drop: everything that jumped
	// here is a scan by definition, so one rule covers all seven flag combos.
	if opts.PortScanLog {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: scanChain,
			Exprs: logExprs(logPrefixPortScan, 0),
		})
	}
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: scanChain,
		Exprs: []expr.Any{&expr.Verdict{Kind: expr.VerdictDrop}},
	})

	// Various illegal TCP flag combinations used in port scanning
	scanCombos := []struct {
		flags uint8
		mask  uint8
	}{
		{0x00, 0xff}, // NULL scan
		{0x01, 0xff}, // FIN only
		{0x03, 0x03}, // SYN+FIN
		{0x05, 0x05}, // RST+FIN
		{0x06, 0x06}, // SYN+RST
		{0x29, 0x29}, // FIN+PSH+URG (Xmas)
		{0xff, 0xff}, // ALL flags
	}

	for _, combo := range scanCombos {
		flags := combo
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseTransportHeader,
					Offset:       13, // TCP flags byte
					Len:          1,
				},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            1,
					Mask:           []byte{flags.mask},
					Xor:            []byte{0x00},
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{flags.flags}},
				&expr.Verdict{Kind: expr.VerdictJump, Chain: "portscan"},
			},
		})
	}
}

func (m *NftablesManager) addSYNFloodProtection(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.SYNFloodLimit
	if limit <= 0 {
		limit = 100
	}

	// The TCP header is identical in both families, so this match does not vary.
	match := func(addrFamily) []expr.Any {
		return []expr.Any{
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
			// SYN flag set, ACK not set (new connection)
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       13,
				Len:          1,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            1,
				Mask:           []byte{0x17}, // SYN+ACK+RST+FIN mask
				Xor:            []byte{0x00},
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{0x02}}, // SYN only
		}
	}

	over := m.addOverRateChain(t, "synflood-over",
		logSpec{enabled: opts.SYNFloodLog, prefix: logPrefixSYNFlood})
	m.addPerSourceRateLimit(t, c, match, perSourceRate{
		setPrefix: "synflood",
		rate:      uint64(limit),
		unit:      expr.LimitTimeSecond,
		burst:     uint32(limit * 2),
		timeout:   time.Minute,
	}, over)
}

func (m *NftablesManager) addICMPFloodProtection(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.ICMPFloodConnectionLimit
	if limit <= 0 {
		limit = 10
	}

	// Rate-limit echo requests. The protocol number and the echo-request type
	// both differ by family: ICMP type 8 on IPv4, ICMPv6 type 128 on IPv6. The
	// rule used to be written for IPv4 only, so a ping flood over IPv6 passed
	// the module entirely.
	match := func(f addrFamily) []expr.Any {
		proto, echo := byte(unix.IPPROTO_ICMP), byte(8)
		if f.nfproto == unix.NFPROTO_IPV6 {
			proto, echo = unix.IPPROTO_ICMPV6, 128
		}
		return []expr.Any{
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{proto}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       0,
				Len:          1,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{echo}},
		}
	}

	over := m.addOverRateChain(t, "icmpflood-over", logSpec{
		enabled:   opts.ICMPFloodLog,
		prefix:    logPrefixICMPFlood,
		perMinute: opts.ICMPFloodLogLimit,
	})
	m.addPerSourceRateLimit(t, c, match, perSourceRate{
		setPrefix: "icmpflood",
		rate:      uint64(limit),
		unit:      expr.LimitTimeSecond,
		burst:     uint32(limit),
		timeout:   time.Minute,
	}, over)
}

// sshMeteredPorts is which ports the SSH limiter meters, and it is a question
// about the input chain: addSSHBruteForce writes into it, so only a rule that
// reaches it may name a port here.
//
// Scope was the gap. A rule with scope = "forwarded" and ssh = true never
// reaches the input chain, so metering its port rate-limits a port the host
// does not open — inert on its own. The fallback below is what makes it worse
// than inert: one such rule fills the list, the list is no longer empty, and
// the default protection on port 22 silently goes away. An operator flagging
// SSH on a forwarded rule would lose brute-force protection on their real SSH
// port, which is the opposite of what the flag asks for. FiltersHost is the
// same question every other input-chain consumer of .TCP asks; this one was
// missed.
func sshMeteredPorts(rules shared.Rules) []string {
	var ports []string
	for _, rule := range rules.TCP {
		if rule.SSH && rule.FiltersHost() {
			ports = append(ports, rule.Port)
		}
	}

	// Also protect port 22 by default if no explicit SSH rule exists
	if len(ports) == 0 {
		return []string{"22"}
	}
	return ports
}

func (m *NftablesManager) addSSHBruteForce(t *nftables.Table, c *nftables.Chain, rules shared.Rules, opts shared.FirewallOptions) {
	limit := opts.SSHBruteForceConnectionLimit
	if limit <= 0 {
		limit = 5
	}

	sshPorts := sshMeteredPorts(rules)

	sshChain := m.conn.AddChain(&nftables.Chain{
		Name:  "sshbrute",
		Table: t,
	})

	// The rate is per source address. It used to be one counter for the chain,
	// which made the module the attack: five connection attempts a minute from
	// anywhere exhausted the budget, and every further SSH connection was
	// dropped — the administrator's included. A protection against being locked
	// out that locks you out is worse than none, because it is trusted.
	over := m.addOverRateChain(t, "sshbrute-over", logSpec{
		enabled:   opts.SSHBruteForceLog,
		prefix:    logPrefixSSH,
		perMinute: opts.SSHBruteForceLogLimit,
	})
	m.addPerSourceRateLimit(t, sshChain, func(addrFamily) []expr.Any { return nil }, perSourceRate{
		setPrefix: "sshbrute",
		rate:      uint64(limit),
		unit:      expr.LimitTimeMinute,
		burst:     uint32(limit),
		// Long enough that a slow brute force cannot reset its budget by
		// pausing, short enough that the set does not accumulate.
		timeout: 10 * time.Minute,
	}, over)

	// Anything that did not exceed its own rate is ordinary traffic, and
	// ordinary traffic is not this chain's decision to make.
	//
	// This used to accept, and Apply adds the jump to the input chain *before*
	// the blacklist — so a blacklisted address could SSH in as long as it stayed
	// under the limit, and port 22 was accepted outright whenever the module was
	// on and no rule opened it (sshPorts falls back to {"22"} above). A
	// protection module that opens a port and overrules the blacklist is doing
	// the opposite of its name.
	//
	// Returning puts the packet back where it came from: blacklist, then
	// whitelist, then the port rules. Over-rate still drops in sshbrute-over, so
	// nothing about the metering changes.
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: sshChain,
		Exprs: []expr.Any{&expr.Verdict{Kind: expr.VerdictReturn}},
	})

	// Meter new connections to each SSH port.
	//
	// buildPortExprs rather than a single-port match, because a port marked as
	// SSH may be a range. The old code parsed it with parsePort, got 0 back for
	// anything containing a colon, and skipped it — the module reported itself
	// enabled and metered nothing.
	for _, port := range sshPorts {
		exprs := []expr.Any{
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
		}
		exprs = append(exprs, buildPortExprs(port)...)
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: append(exprs,
				&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            4,
					Mask:           ctStateMask(ctStateNew),
					Xor:            ctStateNoMatch,
				},
				&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
				&expr.Verdict{Kind: expr.VerdictJump, Chain: "sshbrute"},
			),
		})
	}
}

// addTCPRSTFlood rate-limits inbound TCP RST packets per second.
//
// A reset flood is cheap to send and forces the receiver to tear down state,
// so the cap is on the packet rate rather than on connections.
func (m *NftablesManager) addTCPRSTFlood(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.TCPRSTFloodLimit
	if limit <= 0 {
		limit = 100
	}

	match := func(addrFamily) []expr.Any {
		return []expr.Any{
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
			// TCP flags live in the octet at offset 13 of the transport header.
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       13,
				Len:          1,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            1,
				Mask:           []byte{0x04}, // RST
				Xor:            []byte{0x00},
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{0x04}},
		}
	}

	over := m.addOverRateChain(t, "tcprst-over", logSpec{
		enabled: opts.TCPRSTFloodLog,
		prefix:  logPrefixTCPRST,
	})
	m.addPerSourceRateLimit(t, c, match, perSourceRate{
		setPrefix: "tcprst",
		rate:      uint64(limit),
		unit:      expr.LimitTimeSecond,
		burst:     uint32(limit),
		timeout:   time.Minute,
	}, over)
}

// addAnycastDrop drops traffic addressed to an anycast address.
//
// Anycast is not a bit in the packet — it is a property of the destination
// address as this host resolves it, so the check goes through the FIB. That is
// also why it is off by default: on a host with no anycast address configured
// the rule matches nothing, and on one that has, it will match traffic that may
// well be wanted.
func (m *NftablesManager) addAnycastDrop(t *nftables.Table, c *nftables.Chain) {
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Fib{
				Register:       1,
				ResultADDRTYPE: true,
				FlagDADDR:      true,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(unix.RTN_ANYCAST),
			},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

// addrFamily describes where the source address lives, per address family.
// An inet table sees both, and the key is a different width in each, so
// anything keyed on the source address needs one set and one rule per family.
type addrFamily struct {
	name    string
	nfproto byte
	keyType nftables.SetDatatype
	offset  uint32 // source address offset in the network header
	keyLen  uint32
}

var addrFamilies = []addrFamily{
	{"ipv4", unix.NFPROTO_IPV4, nftables.TypeIPAddr, 12, 4},
	{"ipv6", unix.NFPROTO_IPV6, nftables.TypeIP6Addr, 8, 16},
}

// srcAddrExprs loads the source address of family f into register 1.
func srcAddrExprs(f addrFamily) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{f.nfproto}},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       f.offset,
			Len:          f.keyLen,
		},
	}
}

// addOverRateChain builds the chain a packet lands in once its source has
// exceeded its rate: log it if asked, then drop.
//
// The log lives here rather than beside the match because the match now carries
// a stateful expression. addFiltered emits the match twice — once for the log
// rule, once for the action — and evaluating a meter twice per packet would
// count every packet twice, halving the rate the operator configured. One
// evaluation, one jump, and the log sits with the drop it explains.
func (m *NftablesManager) addOverRateChain(t *nftables.Table, name string, lg logSpec) *nftables.Chain {
	ch := m.conn.AddChain(&nftables.Chain{Name: name, Table: t})
	if lg.enabled {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: ch,
			Exprs: logExprs(lg.prefix, lg.perMinute),
		})
	}
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: ch,
		Exprs: []expr.Any{&expr.Verdict{Kind: expr.VerdictDrop}},
	})
	return ch
}

// perSourceRate is a rate cap applied to each source address separately.
type perSourceRate struct {
	setPrefix string // set names become <prefix>-v4 and <prefix>-v6
	rate      uint64
	unit      expr.LimitTime
	burst     uint32
	timeout   time.Duration // how long an idle source stays in the set
}

// addPerSourceRateLimit sends packets matching `match` whose source is over its
// own rate to `target`.
//
// A bare expr.Limit on a rule is one counter for the whole rule, not one per
// source — so the first host to exhaust it starves every other, and an attacker
// who can spend the budget can keep everyone else out. That is the opposite of
// what a flood protection is for, and it is what "per source address" in the
// interface, the documentation and the schema described for four modules that
// did not do it.
//
// The kernel spells "per source" as a dynamic set keyed by the address with a
// limit attached to each element — `meter` in nft's vocabulary. Elements carry a
// timeout so a set cannot grow without bound from spoofed sources.
// match is a function of the family because some modules match a different
// protocol in each — ICMP echo is protocol 1 type 8 on IPv4 and protocol 58
// type 128 on IPv6, and a rule written for one silently matches nothing in the
// other.
func (m *NftablesManager) addPerSourceRateLimit(t *nftables.Table, c *nftables.Chain,
	match func(addrFamily) []expr.Any, r perSourceRate, target *nftables.Chain) {
	for _, f := range addrFamilies {
		set := &nftables.Set{
			Table:      t,
			Name:       fmt.Sprintf("%s-%s", r.setPrefix, familySuffix(f)),
			KeyType:    f.keyType,
			Dynamic:    true,
			HasTimeout: true,
			Timeout:    r.timeout,
		}
		if err := m.conn.AddSet(set, nil); err != nil {
			slog.Warn("per-source rate limit: could not create set",
				"set", set.Name, "family", f.name, "error", err)
			continue
		}

		// The family test comes first: the source address offset below is only
		// correct for that family, and so is the module's own match.
		exprs := []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{f.nfproto}},
		}
		exprs = append(exprs, match(f)...)
		exprs = append(exprs,
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       f.offset,
				Len:          f.keyLen,
			},
			&expr.Dynset{
				SrcRegKey: 1,
				SetName:   set.Name,
				Operation: uint32(unix.NFT_DYNSET_OP_UPDATE),
				Timeout:   r.timeout,
				Exprs: []expr.Any{
					&expr.Limit{
						Type:  expr.LimitTypePkts,
						Rate:  r.rate,
						Over:  true, // the rule matches once the source is over its rate
						Unit:  r.unit,
						Burst: r.burst,
					},
				},
			},
			&expr.Verdict{Kind: expr.VerdictJump, Chain: target.Name},
		)

		m.adder.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: exprs})
	}
}

func familySuffix(f addrFamily) string {
	if f.nfproto == unix.NFPROTO_IPV6 {
		return "v6"
	}
	return "v4"
}

// addConnectionLimit caps simultaneous connections per source address.
//
// The cap has to be per source: a single global counter would let one host
// exhaust the limit and lock every other client out, which is the opposite of
// what the option promises. That is expressed as a dynamic set keyed by source
// address carrying a connlimit — `ct count over N` inside a meter, in nft's own
// vocabulary. inet needs one set per family, because the address key is a
// different width in each.
func (m *NftablesManager) addConnectionLimit(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	max := opts.ConnectionLimitMax
	if max <= 0 {
		max = 100
	}

	for _, f := range addrFamilies {
		set := &nftables.Set{
			Table:   t,
			Name:    "connlimit-" + familySuffix(f),
			KeyType: f.keyType,
			Dynamic: true,
		}
		if err := m.conn.AddSet(set, nil); err != nil {
			slog.Warn("connection limit: could not create set", "family", f.name, "error", err)
			continue
		}

		exprs := srcAddrExprs(f)
		exprs = append(exprs,
			&expr.Dynset{
				SrcRegKey: 1,
				SetName:   set.Name,
				Operation: uint32(unix.NFT_DYNSET_OP_ADD),
				Exprs: []expr.Any{
					&expr.Connlimit{
						Count: uint32(max),
						Flags: nftConnlimitInvert, // over, not under
					},
				},
			},
			&expr.Verdict{Kind: expr.VerdictDrop},
		)
		m.adder.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: exprs})
	}
}

// Packet types, as the kernel numbers them in include/uapi/linux/if_packet.h and
// as `nft` compiles its own keywords.
//
// Named here because the value that used to be inline was wrong and the comment
// beside it said otherwise: broadcast was written as 0x03, labelled
// NFT_PKTTYPE_BROADCAST. 0x03 is PACKET_OTHERHOST. Asked of nft directly —
// `nft --debug=netlink` on rules it built itself — broadcast compiles to
// 0x00000001, multicast to 0x00000002, other to 0x00000003.
const (
	pktTypeBroadcast = 0x01 // PACKET_BROADCAST
	pktTypeMulticast = 0x02 // PACKET_MULTICAST
)

// addBroadcastDrop drops traffic addressed to the link's broadcast address.
//
// This matched PACKET_OTHERHOST until 2.5.0, so the option did nothing it said:
// broadcast traffic passed untouched, and what it dropped instead was traffic
// addressed to a different host — which an interface does not receive unless it
// is in promiscuous mode. Read back from the kernel before the fix:
//
//	meta pkttype other drop
//
// It shipped because the test asserted the rule *count*. A count cannot see what
// a rule matches, so the wrong packet type passed for as long as exactly one rule
// was added. The test now reads the rule back and asks nft to name it.
func (m *NftablesManager) addBroadcastDrop(t *nftables.Table, c *nftables.Chain) {
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyPKTTYPE, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{pktTypeBroadcast}},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

func (m *NftablesManager) addMulticastDrop(t *nftables.Table, c *nftables.Chain) {
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyPKTTYPE, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{pktTypeMulticast}},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

// addrPos gives the offset of an address within the network header, per family.
// IPv4 carries the source at 12 and the destination at 16; IPv6 at 8 and 24.
type addrPos struct{ v4, v6 uint32 }

var (
	posSrcAddr = addrPos{v4: 12, v6: 8}
	posDstAddr = addrPos{v4: 16, v6: 24}
)

// cidrMatch matches an address or a network — either family, source or
// destination. Returns nil for a comment or for text that will not parse, so a
// caller can skip the entry.
//
// The mask test is omitted for a single address, so that `nft list ruleset`
// prints `ip daddr 172.17.0.2` rather than the same thing anded with
// 255.255.255.255. An operator checking a firewall reads that output.
func cidrMatch(entry string, pos addrPos) []expr.Any {
	return cidrMatchOp(entry, pos, expr.CmpOpEq)
}

// cidrMatchNegated is cidrMatch inverted: an address *outside* the network.
//
// Only the address comparison flips. The family test in front of it stays an
// equality — negating that would match every packet of the other family, which
// in an inet table is half the traffic on the host.
func cidrMatchNegated(entry string, pos addrPos) []expr.Any {
	return cidrMatchOp(entry, pos, expr.CmpOpNeq)
}

// withoutFamilyTest drops the leading `meta nfproto` comparison from a
// cidrMatch, for the second and later matches in one rule: the family has
// already been established by the first, and both come from the same entry.
// Type-checked rather than sliced blind, so a change to cidrMatch's shape
// leaves the match intact instead of beheading it.
func withoutFamilyTest(match []expr.Any) []expr.Any {
	if len(match) < 2 {
		return match
	}
	if _, isMeta := match[0].(*expr.Meta); !isMeta {
		return match
	}
	if _, isCmp := match[1].(*expr.Cmp); !isCmp {
		return match
	}
	return match[2:]
}

func cidrMatchOp(entry string, pos addrPos, op expr.CmpOp) []expr.Any {
	if shared.IsListComment(entry) {
		return nil
	}
	entry = strings.TrimSpace(entry)

	var ipNet *net.IPNet
	if ip := net.ParseIP(entry); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			ipNet = &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
		} else {
			ipNet = &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(128, 128)}
		}
	} else {
		_, parsed, err := net.ParseCIDR(entry)
		if err != nil {
			return nil
		}
		ipNet = parsed
	}

	family, offset, length, addr := byte(unix.NFPROTO_IPV6), pos.v6, uint32(16), ipNet.IP.To16()
	if ip4 := ipNet.IP.To4(); ip4 != nil {
		family, offset, length, addr = unix.NFPROTO_IPV4, pos.v4, 4, ip4
	}
	mask := []byte(ipNet.Mask)
	if len(mask) != int(length) || addr == nil {
		return nil
	}

	exprs := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{family}},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       offset,
			Len:          length,
		},
	}
	if ones, bits := ipNet.Mask.Size(); ones != bits {
		exprs = append(exprs, &expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            length,
			Mask:           mask,
			Xor:            make([]byte, length),
		})
	}
	return append(exprs, &expr.Cmp{Op: op, Register: 1, Data: addr})
}

// addForwardExceptions lets the Docker networks the operator has allowed cross
// the forward chain.
//
// The chain is a base chain at the forward hook with policy drop, and until
// 2.5.0 it had no rules in it at all. That is not the same as taking no
// interest in routed traffic: a base chain whose rules issue no verdict falls
// through to its policy, and a drop there is final no matter what another
// table's forward chain has already accepted. So every packet the host would
// have routed was destroyed — measured with a router between two namespaces, a
// second table accepting everything at the same hook, and its counter showing
// the accept had already matched:
//
//	no firewall                                       REACHABLE
//	+ another table's forward chain accepting all     REACHABLE
//	+ easywall's empty forward chain                  DROPPED
//
// Every Docker container's traffic goes that way: out of the bridge, through
// the forward hook, on to the world — and back the same way for a published
// port, which Docker DNATs before this chain sees it. So all three of the
// arrangements docker.md offers were dead, including the two whose whole point
// is that containers keep working. It went unseen because nothing routes on a
// test host and because the check on this chain asserted its policy, which was
// exactly the part that was right.
//
// Only what the operator has allowed is let through, and only in the two
// directions a container needs: a source inside one of those networks, or a
// destination inside one. Docker's own DOCKER-USER chain still gets its say —
// an accept here ends this chain, not the hook. With Docker coexistence off and
// routing.mode at its default, nothing is added and the chain still drops,
// which is the correct default for a host that does not route.
//
// matches comes from forwardExceptionMatches, over the Docker networks plus,
// under routing.mode = "networks", the networks named there. The two are
// deliberately one list: they are the same statement — "this host routes for
// these" — arrived at by two routes.
//
// The established accept that used to open this function is now added by
// buildForwardChain, ahead of the default-deny 2.19 introduced. It has to
// precede a rule that can say no, and this one can only say yes.
func (m *NftablesManager) addForwardExceptions(t *nftables.Table, c *nftables.Chain, matches [][]expr.Any) {
	for _, match := range matches {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: append(match, &expr.Verdict{Kind: expr.VerdictAccept}),
		})
	}
}

// forwardExceptionMatches builds the address tests addForwardExceptions turns
// into accepts, and returns nothing for a list that names no usable network.
//
// It is separate from the adding because buildForwardChain has to know whether
// anything will be added here before it adds the rule that comes first: with no
// network allowed to cross, 2.18 put nothing at all in this chain, and an
// established accept on its own would be a new rule on every host that does not
// route.
func forwardExceptionMatches(cidrs []string) [][]expr.Any {
	var matches [][]expr.Any
	for _, cidr := range cidrs {
		for _, pos := range []addrPos{posSrcAddr, posDstAddr} {
			if match := cidrMatch(cidr, pos); match != nil {
				matches = append(matches, match)
			}
		}
	}
	return matches
}

// buildForwardChain writes every rule in the forward chain, in the one order
// that makes them mean anything.
//
// It exists as a function rather than as a run of calls inside Apply because the
// order is the correctness of this release and Apply cannot be unit-tested — it
// needs a netlink connection. TestForwardChain_PortRulesComeBeforeTheExceptions
// drives this directly through the recording adder, and it is the only thing
// standing between this release and the defect it was written to fix.
//
//	ct state established,related     accept   ← replies, before anything can deny one
//	<forwarded port rules>           accept   ← only under published_ports = "filtered"
//	dst in bridge, src not in bridge drop     ← likewise; the rule that gives them meaning
//	<bridge / routing CIDR matches>  accept
//	                                 policy drop
//
// With docker.published_ports at its default the middle two render nothing and
// this is byte-for-byte the chain 2.18 built.
func (m *NftablesManager) buildForwardChain(
	t *nftables.Table, c *nftables.Chain, rules shared.Rules,
	docker shared.DockerConfig, routing shared.RoutingConfig, dockerCIDRs []string,
) {
	// What may cross: the Docker networks, always — the coexistence above
	// reaches no container otherwise, and that must not depend on a key nobody
	// has set yet — plus whatever routing.networks names. Under "open" the
	// policy already accepts and these would be dead weight.
	var exceptions [][]expr.Any
	if routing.Mode != shared.RoutingOpen {
		forwardCIDRs := dockerCIDRs
		if routing.Mode == shared.RoutingNetworks {
			forwardCIDRs = append(append([]string(nil), dockerCIDRs...), routing.Networks...)
		}
		exceptions = forwardExceptionMatches(forwardCIDRs)
	}

	// A forwarded deny is not dead weight under "open": it is the only thing in
	// this chain that says no, and dropping it because another key is set is
	// exactly the silent no-op this release exists to end.
	//
	// The question is whether addForwardPortRules will render anything, not
	// whether it was asked to. Filtering with no container network detected
	// renders nothing — see there — and an established accept on its own would
	// be a new rule in a chain 2.18 left empty, accepting forwarded traffic that
	// 2.18's policy dropped.
	if !forwardPortRulesRender(docker, dockerCIDRs) && len(exceptions) == 0 {
		return
	}

	// Return traffic first, so a reply is not re-tested against the networks —
	// and, since 2.19, so that nothing below can deny one. A container's
	// outbound connection comes back with the bridge address as its
	// *destination* once conntrack has undone Docker's masquerade, which is
	// exactly what the deny below matches. Behind that deny, every container on
	// the host loses the network at the next apply — and the acceptance window
	// cannot see it, because SSH arrives on input.
	m.addEstablishedAccept(t, c)
	m.addForwardPortRules(t, c, rules, docker, dockerCIDRs)
	m.addForwardExceptions(t, c, exceptions)
}

// forwardPortRulesRender is the question buildForwardChain has to answer before
// it adds the rule that comes first: will addForwardPortRules put anything in
// this chain? It is deliberately the conjunction of that function's two guards
// and not a third opinion — the two must agree, and three mutations hold them
// together: deleting either guard, or widening this, turns a different test red.
func forwardPortRulesRender(docker shared.DockerConfig, cidrs []string) bool {
	return docker.FiltersPublishedPorts() && len(cidrs) > 0
}

// addForwardPortRules renders the rules that let a named port reach a container,
// and the deny that gives them meaning.
//
// It does nothing at all unless docker.published_ports is "filtered". That is
// not a convenience: the deny below closes every published port that has no
// forwarded rule, and arriving switched on would take every container host's
// services off the network at its next apply — 2.5.0 with a different cause.
//
// ct state is deliberately not tested here. buildForwardChain adds the
// established accept at the top of this chain, so anything reaching these rules
// is new or invalid already, and a second state test would be a second place for
// the two to disagree.
func (m *NftablesManager) addForwardPortRules(
	t *nftables.Table, c *nftables.Chain, rules shared.Rules,
	docker shared.DockerConfig, cidrs []string,
) {
	if !docker.FiltersPublishedPorts() {
		return
	}

	// No container network was detected or configured, so there is no deny to
	// render — the loop at the foot of this function has nothing to iterate.
	// The accepts alone would open, in the forward chain, ports 2.18 kept shut,
	// and the interface would report them enforced: this release's own thesis,
	// reproduced by the feature. Rendering nothing leaves the chain
	// byte-identical to 2.18, which is the safe direction to fail in.
	//
	// Not a config error, which is why Validate does not cover this one:
	// detection runs here, at apply, not at load. A host whose containers have
	// not started yet legitimately has docker.enabled = true and no bridge, and
	// reconcileDockerBridges re-applies when one appears.
	if len(cidrs) == 0 {
		slog.Warn("docker.published_ports is \"filtered\" but no container network was found; " +
			"the forwarded port rules are not in the ruleset. They render as soon as a bridge " +
			"network is detected, or as soon as docker.custom_networks names one")
		return
	}

	for _, proto := range []struct {
		name  string
		rules []shared.PortRule
	}{{"tcp", rules.TCP}, {"udp", rules.UDP}} {
		for _, rule := range proto.rules {
			if !rule.FiltersForwarded() {
				continue
			}
			for _, r := range portAcceptRules(t, c, proto.name, rule) {
				r.Exprs = forwardFamilyPin(r.Exprs)
				m.adder.AddRule(r)
			}
		}
	}

	// The deny, once per bridge network, after the accepts and before the
	// exceptions. A container's outbound traffic has its *source* in the bridge
	// range and container-to-container has both ends inside, so neither matches;
	// only traffic arriving from outside for a container address does, which is
	// exactly a published port.
	for _, cidr := range cidrs {
		dst := cidrMatch(cidr, posDstAddr)
		src := cidrMatchNegated(cidr, posSrcAddr)
		if dst == nil || src == nil {
			continue
		}
		// The family test travels with each match, and one rule needs it once:
		// `meta nfproto ipv4` printed twice in the same line of
		// `nft list ruleset` is output an operator has to read past, and
		// cidrMatch's own comment is about that output.
		exprs := append(append([]expr.Any(nil), dst...), withoutFamilyTest(src)...)
		m.adder.AddRule(&nftables.Rule{
			Table: t, Chain: c,
			Exprs: append(exprs, &expr.Counter{}, &expr.Verdict{Kind: expr.VerdictDrop}),
		})
	}
}

// forwardFamilyPin pins a forwarded port accept to IPv4 unless it already names
// an address family.
//
// portAcceptRules builds a rule with no sources as `meta l4proto tcp dport N
// accept` and no address test at all, which in an inet table is both families.
// In the input chain that is right: ipv6.Mode has already had its say above it,
// and an open port is open on the addresses the host answers to. In the forward
// chain it is not. The deny this accept is paired with comes from
// detectDockerBridges, which returns IPv4 CIDRs only, so an unpinned accept
// opens the named port for forwarded *IPv6* to anything the host routes — far
// past the containers the rule is about, and traffic 2.18's policy drop
// refused. The deny cannot take it back: it is IPv4-only, and it sits below.
//
// A rule that already carries a family test got it from cidrMatch over a source
// the operator named, and keeps it. Naming an address is naming a family.
func forwardFamilyPin(exprs []expr.Any) []expr.Any {
	if len(exprs) > 1 {
		if meta, ok := exprs[0].(*expr.Meta); ok && meta.Key == expr.MetaKeyNFPROTO {
			return exprs
		}
	}
	return append([]expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
	}, exprs...)
}

func (m *NftablesManager) addCIDRAccept(t *nftables.Table, c *nftables.Chain, cidr string) {
	if shared.IsListComment(cidr) {
		return // a note or a spacer, not an address
	}
	cidr = strings.TrimSpace(cidr)
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}

	ip4 := ipNet.IP.To4()
	if ip4 != nil {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       12, // src IP
					Len:          4,
				},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            4,
					Mask:           []byte(ipNet.Mask),
					Xor:            []byte{0, 0, 0, 0},
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip4},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
		})
		return
	}
	// IPv6 CIDR
	ip6 := ipNet.IP.To16()
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV6}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       8, // src IP in IPv6 header
				Len:          16,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            16,
				Mask:           []byte(ipNet.Mask),
				Xor:            make([]byte, 16),
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip6},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func (m *NftablesManager) addBlacklistRule(t *nftables.Table, c *nftables.Chain, ip string, opts shared.FirewallOptions) {
	if shared.IsListComment(ip) {
		return // a note or a spacer, not an address
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		// Try CIDR
		m.addCIDRDrop(t, c, ip)
		return
	}

	// opts was accepted and ignored here until 2.5.0, which is why the
	// log_blacklist_connections switch produced nothing.
	lg := logSpec{
		enabled:   opts.LogBlacklist,
		prefix:    logPrefixBlacklist,
		perMinute: opts.LogBlacklistLimit,
	}

	var match []expr.Any
	if ip4 := parsed.To4(); ip4 != nil {
		match = []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       12, // src IP in IPv4 header
				Len:          4,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip4},
		}
	} else {
		match = []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV6}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       8, // src IP in IPv6 header
				Len:          16,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: parsed.To16()},
		}
	}

	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop}, lg)
}

func (m *NftablesManager) addCIDRDrop(t *nftables.Table, c *nftables.Chain, cidr string) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}
	ip4 := ipNet.IP.To4()
	if ip4 != nil {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       12,
					Len:          4,
				},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            4,
					Mask:           []byte(ipNet.Mask),
					Xor:            []byte{0, 0, 0, 0},
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip4},
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
		})
		return
	}
	// IPv6 CIDR
	ip6 := ipNet.IP.To16()
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV6}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       8, // src IP in IPv6 header
				Len:          16,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            16,
				Mask:           []byte(ipNet.Mask),
				Xor:            make([]byte, 16),
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip6},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

func (m *NftablesManager) addWhitelistRule(t *nftables.Table, c *nftables.Chain, ip string) {
	if shared.IsListComment(ip) {
		return // a note or a spacer, not an address
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		m.addCIDRAccept(t, c, ip)
		return
	}
	ip4 := parsed.To4()
	if ip4 != nil {
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       12,
					Len:          4,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip4},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
		})
		return
	}
	// IPv6 single address
	ip6 := parsed.To16()
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV6}},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       8, // src IP in IPv6 header
				Len:          16,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip6},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

// addPortAccept opens one port.
//
// It accepts, and only accepts. It used to send a port marked as SSH to the
// sshbrute chain instead — a chain that exists only while the SSH brute-force
// module is switched on. Switching that module off therefore produced a rule
// pointing at nothing: the apply failed, the rollback failed for the same
// reason, and the table was left with no chains and no policy at all. One
// checkbox on the options page turned the firewall off completely, and the
// audit log recorded it as `rollback_failed` without saying what had happened.
//
// The metering is not lost. addSSHBruteForce installs its own rule for every
// SSH port, matching new connections only, and it runs earlier in the chain —
// so a new connection is metered before it ever reaches this rule.
//
// A source restriction is one rule per source, matched on the address before the
// port is tested. cidrMatch is the same builder the forward exceptions use: both
// families, a bare address or a network, nil for a comment or a spacer — so an
// operator's note inside the source list is skipped here exactly as it is
// everywhere else. No sources means no address match and one rule, which is
// byte-identical to what every rule written before 2.11 produced.
//
// The building is in portAcceptRules, which takes no connection, because the
// two things this release added — the counter and the rule id — are exactly the
// things no test could see. addPortAccept cannot be called without netlink, so
// what it builds had never been asserted on at all.
func (m *NftablesManager) addPortAccept(t *nftables.Table, c *nftables.Chain, proto string, rule shared.PortRule) {
	for _, r := range portAcceptRules(t, c, proto, rule) {
		m.adder.AddRule(r)
	}
}

// portAcceptRules returns the kernel rules one port rule becomes: one per usable
// source, or exactly one when there are none.
//
// Each carries an expr.Counter placed after the matches and immediately before
// the verdict. The order is the whole correctness of the feature. Before the
// matches, the counter counts every packet that *reaches* the rule rather than
// every packet it *accepts* — a number that measures the rules above it and
// nothing else, rises steadily on a port nobody has ever connected to, and is
// indistinguishable from a real figure. TestPortAcceptRules_TheCounterSitsAfterTheMatch
// is what holds it.
//
// UserData is the rule id as an ordinary nftables comment. TypeComment rather
// than a private TLV type, so `nft list ruleset` shows an operator the same key
// the interface uses, and a rule read back through Conn.GetRules can be summed
// with its siblings — a UI rule with three sources is three kernel rules, and
// the counter for that port is their sum.
func portAcceptRules(t *nftables.Table, c *nftables.Chain, proto string, rule shared.PortRule) []*nftables.Rule {
	// A byte from the start rather than an int converted at use: both values are
	// untyped constants that fit, so this is a compile-time conversion and gosec
	// has no runtime narrowing to warn about (G115).
	var protoNum byte = unix.IPPROTO_TCP
	if proto == "udp" {
		protoNum = unix.IPPROTO_UDP
	}

	portMatch := func(prefix []expr.Any) []expr.Any {
		exprs := append([]expr.Any(nil), prefix...)
		exprs = append(exprs,
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{protoNum}},
		)
		exprs = append(exprs, buildPortExprs(rule.Port)...)
		// After the match, before the verdict. Read the comment above before
		// moving this line.
		exprs = append(exprs, &expr.Counter{})
		return append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})
	}

	// The id travels with every kernel rule this UI rule produces. Built once:
	// userdata.AppendString appends to the slice it is given, so a single shared
	// value handed to several rules would be fine here but is a trap the moment
	// a second TLV is ever added.
	tag := func() []byte {
		if rule.ID == "" {
			// A rule with no id is not tagged and has no counter history. It can
			// only happen to a set built in code — every path that writes the
			// rules file fills the ids in — and the interface renders an em dash
			// for it rather than "never", because "not observed" and "observed
			// to be idle" are different claims.
			return nil
		}
		return userdata.AppendString(nil, userdata.TypeComment, rule.ID)
	}

	var matches [][]expr.Any
	for _, src := range rule.Sources {
		if match := cidrMatch(src, posSrcAddr); match != nil {
			matches = append(matches, match)
		} else if !shared.IsListComment(src) {
			// ValidateRules accepts a little more than cidrMatch can build into a
			// rule (e.g. an IPv4-mapped IPv6 CIDR whose mask length cidrMatch's
			// family check rejects). The gap fails closed — the source is
			// dropped, never opened — but silently, so it is logged here.
			slog.Warn("port rule source accepted by validation but not usable in a kernel rule",
				"port", rule.Port, "source", src)
		}
	}

	// A source list that holds nothing usable — all comments, or all dropped by
	// the gap logged above — must not silently become "anywhere". A list of
	// comments alone is an operator who has not finished typing; opening the
	// port to the world would be the one wrong answer available.
	if len(rule.Sources) > 0 && len(matches) == 0 {
		return nil
	}

	if len(matches) == 0 {
		return []*nftables.Rule{{Table: t, Chain: c, Exprs: portMatch(nil), UserData: tag()}}
	}
	out := make([]*nftables.Rule, 0, len(matches))
	for _, match := range matches {
		out = append(out, &nftables.Rule{Table: t, Chain: c, Exprs: portMatch(match), UserData: tag()})
	}
	return out
}

func (m *NftablesManager) addForwardingRules(t *nftables.Table, rules []shared.ForwardingRule) {
	// NAT PREROUTING chain for port forwarding
	preChain := m.conn.AddChain(&nftables.Chain{
		Name:     "prerouting",
		Table:    t,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityRef(prioNAT),
	})

	for _, rule := range rules {
		protoNum := unix.IPPROTO_TCP
		if rule.Protocol == "udp" {
			protoNum = unix.IPPROTO_UDP
		}

		// Match the port the packet arrived on, redirect to the port that
		// serves it. These were the wrong way round until 2.5.0: the rule
		// matched DestPort and redirected to SourcePort, so the documented
		// example {source_port: 2222, dest_port: 22} produced
		// `tcp dport 22 redirect to :2222` — it captured SSH on 22 and sent it
		// somewhere nothing was listening, while 2222 did nothing at all.
		m.adder.AddRule(&nftables.Rule{
			Table: t,
			Chain: preChain,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{byte(protoNum)}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseTransportHeader,
					Offset:       2, // dest port
					Len:          2,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: portBytes(rule.SourcePort)}, // the incoming port
				&expr.Immediate{
					Register: 1,
					Data:     portBytes(rule.DestPort), // the port that serves it
				},
				&expr.Redir{
					RegisterProtoMin: 1,
				},
			},
		})
	}
}

func (m *NftablesManager) addFinalLog(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.LogBlockedLimit
	if limit <= 0 {
		limit = 60
	}
	m.adder.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: logExprs(logPrefixDrop, limit),
	})
}

// --- Utility functions ---

func policyDrop() *nftables.ChainPolicy {
	p := nftables.ChainPolicyDrop
	return &p
}

func policyAccept() *nftables.ChainPolicy {
	p := nftables.ChainPolicyAccept
	return &p
}

// parsePort returns the port number, or 0 if s is not one.
//
// Strict, like the validation upstream: fmt.Sscanf stops at the first character
// it cannot read and reports success for the part it got, so "80abc" parsed as
// 80 and "80 90" as 80. SaveStaged and Apply both reject those now, but the
// last mile should not be the one place that would still accept them.
func parsePort(s string) uint16 {
	p, err := shared.ParsePortNumber(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	// #nosec G115 -- ParsePortNumber returned without error, which it only does
	// for 1–65535. The bound is the line above.
	return uint16(p)
}

// buildPortExprs returns nftables expressions matching a port or port range.
// Port ranges use the "start:end" format.
func buildPortExprs(port string) []expr.Any {
	if lo, hi, ok := strings.Cut(port, ":"); ok {
		start, errLo := shared.ParsePortNumber(strings.TrimSpace(lo))
		end, errHi := shared.ParsePortNumber(strings.TrimSpace(hi))
		if errLo != nil || errHi != nil || end < start {
			// Unreachable through the daemon — validateRules runs first — and a
			// match on port 0 is better than a match on whatever half of a
			// malformed range happened to parse.
			slog.Warn("ignoring unparseable port range", "port", port)
			return matchPortEq(0)
		}
		// Range match
		return []expr.Any{
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2, // dest port
				Len:          2,
			},
			&expr.Cmp{Op: expr.CmpOpGte, Register: 1, Data: portBytes(start)},
			&expr.Cmp{Op: expr.CmpOpLte, Register: 1, Data: portBytes(end)},
		}
	}

	return matchPortEq(parsePort(port))
}

// portBytes packs a port into the two bytes a netlink comparison expects, most
// significant first — the wire order of a TCP or UDP header.
//
// It takes an int and bounds it *here* rather than trusting that something
// upstream did. Apply refuses any rule set ValidateRules rejects and that bounds
// every port to 1–65535, so nothing out of range reaches this today — but that
// is a guarantee held three functions away from the conversion, and a silent
// truncation is how port 70000 would become port 4464 in the kernel while the
// interface still said 70000. Out of range yields port 0, which matches no real
// packet: the same choice buildPortExprs already makes for a malformed range.
//
// One helper because this split was written out at seven call sites, and each
// one was a conversion someone had to reason about separately.
func portBytes(p int) []byte {
	if p < 1 || p > 65535 {
		return []byte{0, 0}
	}
	// #nosec G115 -- bounded three lines up, and both halves of the uint16 are
	// written: this is the split the wire format asks for, not a truncation.
	v := uint16(p)
	// #nosec G115 -- the two halves of v, high byte first.
	return []byte{byte(v >> 8), byte(v)}
}

// matchPortEq matches a single destination port.
func matchPortEq(p uint16) []expr.Any {
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       2, // dest port
			Len:          2,
		},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: portBytes(int(p))},
	}
}

// SaveSnapshot writes a nftables backup snapshot to disk.
// snapshotPrefix and snapshotSuffix bracket the only files rotation may delete.
const (
	snapshotPrefix = "nftables_"
	snapshotSuffix = ".json"
	snapshotsKept  = 10
)

// SaveSnapshot writes an nftables backup snapshot to disk and prunes old ones.
//
// The timestamp carries milliseconds. At one-second resolution two snapshots
// from the same second landed on the same filename, so the second overwrote the
// first and "the last ten" could quietly be fewer.
func SaveSnapshot(dir string, data []byte) error {
	if err := rotateSnapshots(dir, snapshotsKept); err != nil {
		slog.Warn("could not rotate nftables snapshots", "dir", dir, "error", err)
	}

	return os.WriteFile(snapshotPath(dir, time.Now().UTC()), data, 0600)
}

// snapshotPath builds a name that does not collide with one already there.
//
// The timestamp alone was not enough: at one-second resolution two snapshots
// from the same second landed on the same filename and the second overwrote the
// first, so "the last ten" could quietly be fewer. Milliseconds narrow that and
// do not close it — the suffix does. It is "_N" rather than "-N" so the
// disambiguated name still sorts after the plain one, and name order stays age
// order for rotation.
func snapshotPath(dir string, t time.Time) string {
	ts := t.Format("2006-01-02_15-04-05.000")
	path := filepath.Join(dir, snapshotPrefix+ts+snapshotSuffix)
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s%s_%d%s", snapshotPrefix, ts, i, snapshotSuffix))
	}
	return path
}

// rotateSnapshots keeps the newest `keep` snapshots and removes the rest.
//
// It considers only files this package wrote. It used to take every
// non-directory entry in the directory — and the directory it is called with is
// log_dir, which also holds audit.log. "audit.log" sorts before "nftables_…",
// so it was the first thing deleted: on the eleventh apply, easywall removed the
// security record that audit-log.md describes as append-only and never
// truncated by easywall. Anything logrotate had put beside it went the same way.
//
// The names embed a UTC timestamp in a format that sorts lexicographically, so
// name order is age order.
func rotateSnapshots(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var snapshots []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() ||
			!strings.HasPrefix(name, snapshotPrefix) ||
			!strings.HasSuffix(name, snapshotSuffix) {
			continue
		}
		snapshots = append(snapshots, name)
	}
	sort.Strings(snapshots)

	// One is about to be written, so make room for it: keep-1 survive here.
	for len(snapshots) >= keep {
		if err := os.Remove(filepath.Join(dir, snapshots[0])); err != nil {
			return err
		}
		snapshots = snapshots[1:]
	}
	return nil
}
