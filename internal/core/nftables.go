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
	nftBinary  = "nft"
	nftTimeout = 30 * time.Second
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
func (m *NftablesManager) Snapshot() ([]byte, error) {
	if m.conn == nil {
		return nil, fmt.Errorf("nftables connection not available")
	}
	tables, err := m.conn.ListTables()
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	type chainSnap struct {
		Name  string `json:"name"`
		Rules int    `json:"rules"`
	}
	type tableSnap struct {
		Name   string      `json:"name"`
		Family string      `json:"family"`
		Chains []chainSnap `json:"chains"`
	}

	tableSnaps := make([]tableSnap, 0, len(tables))
	for _, tbl := range tables {
		var chainSnaps []chainSnap
		if chains, err := m.conn.ListChains(); err == nil {
			for _, ch := range chains {
				if ch.Table == nil || ch.Table.Name != tbl.Name {
					continue
				}
				ruleCount := 0
				if rules, err := m.conn.GetRules(tbl, ch); err == nil {
					ruleCount = len(rules)
				}
				chainSnaps = append(chainSnaps, chainSnap{Name: ch.Name, Rules: ruleCount})
			}
		}
		tableSnaps = append(tableSnaps, tableSnap{
			Name:   tbl.Name,
			Family: tableFamilyName(tbl.Family),
			Chains: chainSnaps,
		})
	}

	snap := struct {
		Timestamp string      `json:"timestamp"`
		Tables    []tableSnap `json:"tables"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tables:    tableSnaps,
	}
	return json.Marshal(snap)
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
func (m *NftablesManager) Apply(state shared.RulesState, opts shared.FirewallOptions, ipv6 shared.IPv6Config, docker shared.DockerConfig) error {
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

	// --- FORWARD chain (base, default DROP) ---
	m.conn.AddChain(&nftables.Chain{
		Name:     "forward",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityRef(prioFilter),
		Policy:   policyDrop(),
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
		m.addBogonFilter(table, inputChain, opts)
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
	if docker.Enabled {
		var cidrs []string
		if docker.AllowBridgeNetworks {
			cidrs = append(cidrs, detectDockerBridges()...)
		}
		cidrs = append(cidrs, docker.CustomNetworks...)
		for _, cidr := range cidrs {
			m.addCIDRAccept(table, inputChain, cidr)
		}
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

	// Final logging + DROP (already set via chain policy, add log rule if requested)
	if opts.LogBlocked {
		m.addFinalLog(table, inputChain, opts)
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

func (m *NftablesManager) addBogonFilter(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
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

	for _, cidr := range bogons {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		ip := ipNet.IP.To4()
		mask := []byte(ipNet.Mask)
		if ip == nil {
			continue
		}

		match := []expr.Any{
			// Only for IPv4
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.NFPROTO_IPV4}},
			// Not loopback interface
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte("lo\x00")},
			// Match source IP in bogon range
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
				Mask:           mask,
				Xor:            []byte{0, 0, 0, 0},
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip},
		}
		m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop},
			logSpec{enabled: opts.BogonsLog, prefix: logPrefixBogon})
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

	// Jump to sshbrute chain for each SSH port
	for _, port := range sshPorts {
		portNum := parsePort(port)
		if portNum == 0 {
			continue
		}
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseTransportHeader,
					Offset:       2, // dest port
					Len:          2,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{byte(portNum >> 8), byte(portNum)}},
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
			},
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

func (m *NftablesManager) addBroadcastDrop(t *nftables.Table, c *nftables.Chain) {
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyPKTTYPE, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{0x03}}, // NFT_PKTTYPE_BROADCAST
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
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{0x02}}, // NFT_PKTTYPE_MULTICAST
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
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

func (m *NftablesManager) addPortAccept(t *nftables.Table, c *nftables.Chain, proto string, rule shared.PortRule) {
	protoNum := unix.IPPROTO_TCP
	if proto == "udp" {
		protoNum = unix.IPPROTO_UDP
	}

	targetChain := ""
	if rule.SSH {
		targetChain = "sshbrute"
	}

	exprs := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{byte(protoNum)}},
	}

	// Port range or single port
	portExprs := buildPortExprs(rule.Port)
	exprs = append(exprs, portExprs...)

	if targetChain != "" {
		exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictJump, Chain: targetChain})
	} else {
		exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})
	}

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
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{
					byte(rule.SourcePort >> 8), // the incoming port
					byte(rule.SourcePort),
				}},
				&expr.Immediate{
					Register: 1,
					Data: []byte{
						byte(rule.DestPort >> 8), // the port that serves it
						byte(rule.DestPort),
					},
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
			&expr.Cmp{Op: expr.CmpOpGte, Register: 1, Data: []byte{byte(start >> 8), byte(start)}},
			&expr.Cmp{Op: expr.CmpOpLte, Register: 1, Data: []byte{byte(end >> 8), byte(end)}},
		}
	}

	return matchPortEq(parsePort(port))
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
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{byte(p >> 8), byte(p)}},
	}
}

// SaveSnapshot writes a nftables backup snapshot to disk.
func SaveSnapshot(dir string, data []byte) error {
	ts := time.Now().UTC().Format("2006-01-02_15-04-05")
	path := fmt.Sprintf("%s/nftables_%s.json", dir, ts)

	// Rotate: keep only last 10 snapshots
	_ = rotateSnapshots(dir, 10)

	return os.WriteFile(path, data, 0600)
}

func rotateSnapshots(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var snapshots []string
	for _, e := range entries {
		if !e.IsDir() {
			snapshots = append(snapshots, e.Name())
		}
	}

	for len(snapshots) >= keep {
		_ = os.Remove(dir + "/" + snapshots[0])
		snapshots = snapshots[1:]
	}
	return nil
}
