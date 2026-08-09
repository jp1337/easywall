package core

import (
	"encoding/json"
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
	if err := validateRules(state.Current); err != nil {
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
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(strings.Join(cmds, "\n") + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
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
	bogons := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",   // CGNAT
		"169.254.0.0/16",  // link-local
		"192.0.2.0/24",    // TEST-NET-1
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

	match := []expr.Any{
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
		&expr.Limit{
			Type:  expr.LimitTypePkts,
			Rate:  uint64(limit),
			Over:  true, // drop when rate exceeded
			Unit:  expr.LimitTimeSecond,
			Burst: uint32(limit * 2),
		},
	}
	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop},
		logSpec{enabled: opts.SYNFloodLog, prefix: logPrefixSYNFlood})
}

func (m *NftablesManager) addICMPFloodProtection(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.ICMPFloodConnectionLimit
	if limit <= 0 {
		limit = 10
	}

	// Rate-limit ICMP echo-request (ping flood)
	match := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_ICMP}},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       0,
			Len:          1,
		},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{8}}, // echo-request
		&expr.Limit{
			Type:  expr.LimitTypePkts,
			Rate:  uint64(limit),
			Over:  true,
			Unit:  expr.LimitTimeSecond,
			Burst: uint32(limit),
		},
	}
	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop}, logSpec{
		enabled:   opts.ICMPFloodLog,
		prefix:    logPrefixICMPFlood,
		perMinute: opts.ICMPFloodLogLimit,
	})
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

	// Accept within rate limit, drop if exceeded
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: sshChain,
		Exprs: []expr.Any{
			&expr.Limit{
				Type:  expr.LimitTypePkts,
				Rate:  uint64(limit),
				Over:  false, // accept within limit
				Unit:  expr.LimitTimeMinute,
				Burst: uint32(limit),
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
	// Anything still here exceeded the rate, which is the event worth logging —
	// the accepted connections above are ordinary traffic.
	if opts.SSHBruteForceLog {
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: sshChain,
			Exprs: logExprs(logPrefixSSH, opts.SSHBruteForceLogLimit),
		})
	}
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: sshChain,
		Exprs: []expr.Any{&expr.Verdict{Kind: expr.VerdictDrop}},
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

	match := []expr.Any{
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
		&expr.Limit{
			Type:  expr.LimitTypePkts,
			Rate:  uint64(limit),
			Over:  true,
			Unit:  expr.LimitTimeSecond,
			Burst: uint32(limit),
		},
	}

	m.addFiltered(t, c, match, &expr.Verdict{Kind: expr.VerdictDrop}, logSpec{
		enabled:   opts.TCPRSTFloodLog,
		prefix:    logPrefixTCPRST,
		perMinute: 0,
	})
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

	families := []struct {
		name     string
		nfproto  byte
		keyType  nftables.SetDatatype
		offset   uint32
		keyLen   uint32
		setLabel string
	}{
		{"ipv4", unix.NFPROTO_IPV4, nftables.TypeIPAddr, 12, 4, "connlimit-v4"},
		{"ipv6", unix.NFPROTO_IPV6, nftables.TypeIP6Addr, 8, 16, "connlimit-v6"},
	}

	for _, f := range families {
		set := &nftables.Set{
			Table:   t,
			Name:    f.setLabel,
			KeyType: f.keyType,
			Dynamic: true,
		}
		if err := m.conn.AddSet(set, nil); err != nil {
			slog.Warn("connection limit: could not create set", "family", f.name, "error", err)
			continue
		}

		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{f.nfproto}},
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       f.offset,
					Len:          f.keyLen,
				},
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
			},
		})
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

func parsePort(s string) uint16 {
	var p int
	_, err := fmt.Sscanf(s, "%d", &p)
	if err != nil || p < 1 || p > 65535 {
		return 0
	}
	return uint16(p)
}

// buildPortExprs returns nftables expressions matching a port or port range.
// Port ranges use the "start:end" format.
func buildPortExprs(port string) []expr.Any {
	var start, end int
	if n, _ := fmt.Sscanf(port, "%d:%d", &start, &end); n == 2 {
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

	// Single port
	p := parsePort(port)
	return []expr.Any{
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       2,
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
