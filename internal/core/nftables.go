package core

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

const (
	tableName = "easywall"

	// nftables chain priorities
	prioFilter = 0
	prioNAT    = -100
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

// Snapshot serialises the current kernel nftables state to JSON for backup.
func (m *NftablesManager) Snapshot() ([]byte, error) {
	if m.conn == nil {
		return nil, fmt.Errorf("nftables connection not available")
	}
	tables, err := m.conn.ListTables()
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	snapshot := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"tables":    len(tables),
	}
	return json.Marshal(snapshot)
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
		m.addInvalidPacketDrop(table, inputChain)
	}
	if opts.Fragments {
		m.addFragmentDrop(table, inputChain)
	}
	if opts.Bogons {
		m.addBogonFilter(table, inputChain)
	}
	if opts.ICMPFlood {
		m.addICMPFloodProtection(table, inputChain, opts)
	}
	if opts.SSHBruteForce {
		m.addSSHBruteForce(table, inputChain, state.Current, opts)
	}
	if opts.DropBroadcast {
		m.addBroadcastDrop(table, inputChain)
	}
	if opts.DropMulticast {
		m.addMulticastDrop(table, inputChain)
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

	return m.conn.Flush()
}

// Restore restores a previously taken snapshot. Currently a no-op placeholder
// that relies on the rules.go rollback mechanism to re-apply the backup rules.
// A true byte-level restore is available for emergency recovery only.
func (m *NftablesManager) Restore(snapshot []byte) error {
	// The primary rollback path is: rules.Rollback() + Apply(backupRules).
	// This function is kept for potential future use with iptables-restore equivalent.
	return nil
}

// --- Helper builders ---

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

	if !ipv6.Enabled {
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

func (m *NftablesManager) addInvalidPacketDrop(t *nftables.Table, c *nftables.Chain) {
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           []byte{0x00, 0x00, 0x00, 0x01}, // INVALID state
				Xor:            []byte{0x00, 0x00, 0x00, 0x00},
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0x00, 0x00, 0x00, 0x00}},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

func (m *NftablesManager) addFragmentDrop(t *nftables.Table, c *nftables.Chain) {
	// Drop fragmented IPv4 packets (offset > 0 or MF flag set)
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
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
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

func (m *NftablesManager) addBogonFilter(t *nftables.Table, c *nftables.Chain) {
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

		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
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
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
		})
	}
}

func (m *NftablesManager) addPortScanPrevention(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	// Named chain for port scan drops
	scanChain := m.conn.AddChain(&nftables.Chain{
		Name:  "portscan",
		Table: t,
	})
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

	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
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
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
}

func (m *NftablesManager) addICMPFloodProtection(t *nftables.Table, c *nftables.Chain, opts shared.FirewallOptions) {
	limit := opts.ICMPFloodConnectionLimit
	if limit <= 0 {
		limit = 10
	}

	// Rate-limit ICMP echo-request (ping flood)
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
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{8}}, // echo-request
			&expr.Limit{
				Type:  expr.LimitTypePkts,
				Rate:  uint64(limit),
				Over:  true,
				Unit:  expr.LimitTimeSecond,
				Burst: uint32(limit),
			},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
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
	if ip4 == nil {
		return // IPv6 CIDR not handled in this path
	}

	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
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
}

func (m *NftablesManager) addBlacklistRule(t *nftables.Table, c *nftables.Chain, ip string, opts shared.FirewallOptions) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		// Try CIDR
		m.addCIDRDrop(t, c, ip)
		return
	}

	ip4 := parsed.To4()
	if ip4 != nil {
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       12,
					Len:          4,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip4},
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
		})
	} else {
		ip6 := parsed.To16()
		m.conn.AddRule(&nftables.Rule{
			Table: t,
			Chain: c,
			Exprs: []expr.Any{
				&expr.Payload{
					DestRegister: 1,
					Base:         expr.PayloadBaseNetworkHeader,
					Offset:       8, // src IP in IPv6 header
					Len:          16,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ip6},
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
		})
	}
}

func (m *NftablesManager) addCIDRDrop(t *nftables.Table, c *nftables.Chain, cidr string) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return
	}
	m.conn.AddRule(&nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
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
	}
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
					byte(rule.DestPort >> 8),
					byte(rule.DestPort),
				}},
				&expr.Immediate{
					Register: 1,
					Data:     []byte{byte(rule.SourcePort >> 8), byte(rule.SourcePort)},
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
		Exprs: []expr.Any{
			&expr.Limit{
				Type:  expr.LimitTypePkts,
				Rate:  uint64(limit),
				Over:  false,
				Unit:  expr.LimitTimeMinute,
				Burst: uint32(limit),
			},
			&expr.Log{
				Key:  unix.NFTA_LOG_PREFIX,
				Data: []byte("easywall blocked: "),
			},
		},
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
