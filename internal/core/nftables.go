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
	"time"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
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
	conn *nftables.Conn
}

// NewNftablesManager creates a new manager and verifies netlink connectivity.
func NewNftablesManager() (*NftablesManager, error) {
	conn, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("open netlink connection: %w", err)
	}
	return &NftablesManager{conn: conn}, nil
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
		if ch.Table == nil || ch.Table.Name != tableName || ch.Name != "input" {
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

// Reset deletes and recreates the easywall table, giving us a clean slate.
// All other tables (filter, nat, docker, etc.) are untouched.
func (m *NftablesManager) Reset() error {
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

	if err := m.Reset(); err != nil {
		return fmt.Errorf("reset table: %w", err)
	}

	table := &nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyINet,
	}

	// --- INPUT chain (base, default DROP) ---
	inputChain := m.conn.AddChain(&nftables.Chain{
		Name:     "input",
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
		Name:     "forward",
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

	// What may cross the forward chain: the Docker networks, always — the
	// coexistence above reaches no container otherwise, and that must not depend
	// on a key nobody has set yet — plus whatever routing.networks names. Under
	// "open" the policy already accepts and these rules would be dead weight.
	if routing.Mode != shared.RoutingOpen {
		forwardCIDRs := dockerCIDRs
		if routing.Mode == shared.RoutingNetworks {
			forwardCIDRs = append(append([]string(nil), dockerCIDRs...), routing.Networks...)
		}
		m.addForwardExceptions(table, forwardChain, forwardCIDRs)
	}

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

	// Open TCP / UDP ports
	for _, rule := range state.Current.TCP {
		m.addPortAccept(table, inputChain, "tcp", rule)
	}
	for _, rule := range state.Current.UDP {
		m.addPortAccept(table, inputChain, "udp", rule)
	}

	// Port forwarding (NAT)
	if len(state.Current.Forwarding) > 0 {
		m.addForwardingRules(table, state.Current.Forwarding)
	}

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
		m.conn.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: logged})
	}
	acted := make([]expr.Any, 0, len(match)+1)
	acted = append(acted, match...)
	acted = append(acted, action)
	m.conn.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: acted})
}

// --- Helper builders ---

// addFamilyVerdict accepts or drops an entire address family in one rule.
// Used for the IPv6 passthrough and block modes, where the point is that no
// later rule gets a say.
func (m *NftablesManager) addFamilyVerdict(t *nftables.Table, c *nftables.Chain, family byte, kind expr.VerdictKind) {
	m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte("lo\x00")},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func (m *NftablesManager) addEstablishedAccept(t *nftables.Table, c *nftables.Chain) {
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Ct{Register: 1, SourceRegister: false, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           []byte{0x00, 0x00, 0x00, 0x06}, // ESTABLISHED | RELATED
				Xor:            []byte{0x00, 0x00, 0x00, 0x00},
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0x00, 0x00, 0x00, 0x00}},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func (m *NftablesManager) addICMPRules(t *nftables.Table, c *nftables.Chain, ipv6 shared.IPv6Config) {
	// ICMPv4 types to accept
	icmpv4Types := []byte{0, 3, 11, 12}
	for _, icmpType := range icmpv4Types {
		m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
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
			Mask:           []byte{0x00, 0x00, 0x00, 0x01}, // INVALID state
			Xor:            []byte{0x00, 0x00, 0x00, 0x00},
		},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0x00, 0x00, 0x00, 0x00}},
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
	// Drop packets from RFC-1918 and special ranges arriving from non-loopback interfaces.
	// These are "impossible" sources on the public internet.
	// filters.md listed "this network" and loopback among the ranges this drops,
	// and neither was here — while TEST-NET-3 and the reserved space were here
	// and not in the table. The list and the documentation now name the same
	// eleven ranges.
	//
	// IPv4 only, deliberately: fe80::/10 is link-local, and IPv6 needs neighbour
	// discovery on it to work at all, so the IPv6 equivalents are not a
	// symmetric translation. filters.md says so rather than implying coverage
	// that is not here.
	bogons := []string{
		"0.0.0.0/8",       // "this network"
		"10.0.0.0/8",      // private
		"100.64.0.0/10",   // carrier-grade NAT
		"127.0.0.0/8",     // loopback, which cannot arrive on a real interface
		"169.254.0.0/16",  // link-local
		"172.16.0.0/12",   // private
		"192.0.2.0/24",    // TEST-NET-1
		"192.168.0.0/16",  // private
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved
	}

	bogonChain := m.conn.AddChain(&nftables.Chain{Name: "bogon", Table: t})

	// The exceptions come first, so a source the operator has allowed leaves
	// this chain before any drop can see it and carries on down the input chain.
	for _, cidr := range exempt {
		match := ipv4SourceMatch(cidr)
		if match == nil {
			continue // a comment, an IPv6 address, or unparseable — not for this filter
		}
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: bogonChain,
			Exprs: append(match, &expr.Verdict{Kind: expr.VerdictReturn}),
		})
	}

	for _, cidr := range bogons {
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
	m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: scanChain,
			Exprs: logExprs(logPrefixPortScan, 0),
		})
	}
	m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
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

func (m *NftablesManager) addSSHBruteForce(t *nftables.Table, c *nftables.Chain, rules shared.Rules, opts shared.FirewallOptions) {
	limit := opts.SSHBruteForceConnectionLimit
	if limit <= 0 {
		limit = 5
	}

	// Find SSH ports from the TCP rules
	var sshPorts []string
	for _, rule := range rules.TCP {
		if rule.SSH {
			sshPorts = append(sshPorts, rule.Port)
		}
	}

	// Also protect port 22 by default if no explicit SSH rule exists
	if len(sshPorts) == 0 {
		sshPorts = []string{"22"}
	}

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

	// Anything that did not exceed its own rate is ordinary traffic.
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: sshChain,
		Exprs: []expr.Any{&expr.Verdict{Kind: expr.VerdictAccept}},
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
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: append(exprs,
				&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            4,
					Mask:           []byte{0x00, 0x00, 0x00, 0x08}, // NEW state
					Xor:            []byte{0x00, 0x00, 0x00, 0x00},
				},
				&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0x00, 0x00, 0x00, 0x00}},
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
	m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: ch,
			Exprs: logExprs(lg.prefix, lg.perMinute),
		})
	}
	m.conn.AddRule(&nftables.Rule{
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

		m.conn.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: exprs})
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
		m.conn.AddRule(&nftables.Rule{Table: t, Chain: c, Exprs: exprs})
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
	m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
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
	return append(exprs, &expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: addr})
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
// cidrs is the Docker networks plus, under routing.mode = "networks", the
// networks named there. The two are deliberately one list: they are the same
// statement — "this host routes for these" — arrived at by two routes.
func (m *NftablesManager) addForwardExceptions(t *nftables.Table, c *nftables.Chain, cidrs []string) {
	var matches [][]expr.Any
	for _, cidr := range cidrs {
		for _, pos := range []addrPos{posSrcAddr, posDstAddr} {
			if match := cidrMatch(cidr, pos); match != nil {
				matches = append(matches, match)
			}
		}
	}
	if len(matches) == 0 {
		return
	}

	// Return traffic first, so a reply is not re-tested against the networks:
	// a connection out of a container to the internet comes back with neither
	// address inside the bridge range once Docker has un-NATed it.
	m.addEstablishedAccept(t, c)
	for _, match := range matches {
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: append(match, &expr.Verdict{Kind: expr.VerdictAccept}),
		})
	}
}

func (m *NftablesManager) addCIDRAccept(t *nftables.Table, c *nftables.Chain, cidr string) {
	if shared.IsListComment(cidr) {
		return // a note or a spacer, not an address
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}

	ip4 := ipNet.IP.To4()
	if ip4 != nil {
		m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
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
		m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
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
func (m *NftablesManager) addPortAccept(t *nftables.Table, c *nftables.Chain, proto string, rule shared.PortRule) {
	protoNum := unix.IPPROTO_TCP
	if proto == "udp" {
		protoNum = unix.IPPROTO_UDP
	}

	exprs := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{byte(protoNum)}},
	}
	exprs = append(exprs, buildPortExprs(rule.Port)...)
	exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})

	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: exprs,
	})
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
		m.conn.AddRule(&nftables.Rule{
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
	m.conn.AddRule(&nftables.Rule{
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
