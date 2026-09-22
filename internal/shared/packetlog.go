package shared

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
)

// A refused packet, as easywall-core decoded it from NFLOG.
//
// Typed on purpose. The kernel log is text, and anything reading text is a
// parser in the root process — which 2.24's row rejects in as many words. NFLOG
// hands the core a binary header of fixed structure and the first bytes of the
// packet, so every field below is read out of a known offset rather than out of
// a line somebody could shape.

// PacketLogEntry is one refused packet.
type PacketLogEntry struct {
	// Seq is assigned by the core, strictly increasing, and survives a restart
	// through the spill file. It is the row's identity in the page — the live
	// tail preserves an open drill-down by it.
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`

	// Rule is one of PacketLogRules, or PacketLogRuleOther.
	Rule string `json:"rule"`

	// Hook is "input" or "forward", or empty when NFLOG did not say. Only an
	// input packet can be let in by opening a port.
	Hook  string `json:"hook,omitempty"`
	InDev string `json:"in,omitempty"`

	Family  uint8      `json:"family"` // 4 or 6
	Src     netip.Addr `json:"src"`
	Dst     netip.Addr `json:"dst"`
	Proto   string     `json:"proto"` // tcp, udp, icmp, icmpv6, or the protocol number
	SrcPort uint16     `json:"sport,omitempty"`
	// DstPort is zero when the packet carries no transport header this code
	// read: ICMP, a later fragment, a protocol without ports.
	DstPort  uint16 `json:"dport,omitempty"`
	TCPFlags string `json:"flags,omitempty"` // "SYN", "SYN,ACK", …
	TTL      uint8  `json:"ttl"`             // hop limit for IPv6
	Length   uint16 `json:"len"`
	CtState  string `json:"ct,omitempty"` // new, established, related, untracked
	// Mark is the skb's nfmark (NFULA_MARK), or zero when the kernel set none —
	// which most packets carry as-is, so zero is not itself informative.
	Mark uint32 `json:"mark,omitempty"`
}

// PacketLogRules are the ten log switches, by the name their prefix carries.
// filters.md lists them in this order.
var PacketLogRules = []string{
	"ssh", "icmp_flood", "syn_flood", "tcp_rst", "portscan",
	"invalid", "fragment", "bogon", "blacklist", "drop",
}

// PacketLogRuleOther is a packet some other rule logged into easywall's group —
// a custom rule, usually. Shown, and labelled as not one of the ten.
const PacketLogRuleOther = "other"

// PacketLogProtos are the protocols the filter offers by name.
var PacketLogProtos = []string{"tcp", "udp", "icmp", "icmpv6"}

// logPrefixStem is how every easywall log prefix begins; the core's constants
// in nftables.go are "easywall <name>: ".
const logPrefixStem = "easywall "

// RuleFromPrefix names the rule behind an NFLOG prefix.
func RuleFromPrefix(prefix string) string {
	name, ok := strings.CutPrefix(prefix, logPrefixStem)
	if !ok {
		return PacketLogRuleOther
	}
	name = strings.TrimRight(name, "\x00 ")
	name = strings.ReplaceAll(strings.TrimSuffix(name, ":"), "-", "_")
	if slices.Contains(PacketLogRules, name) {
		return name
	}
	return PacketLogRuleOther
}

const (
	packetLogDefaultLimit = 200 // what the page shows; the audit log shows as many
	packetLogMaxLimit     = 1000
)

// PacketLogFilter narrows GET_PACKET_LOG. Every field is optional and they
// combine with AND.
type PacketLogFilter struct {
	Src   string `json:"src,omitempty"`  // an address or a CIDR
	Dst   string `json:"dst,omitempty"`  // an address or a CIDR
	Port  uint16 `json:"port,omitempty"` // the destination port — a source port is ephemeral noise
	Proto string `json:"proto,omitempty"`
	Rule  string `json:"rule,omitempty"`
	InDev string `json:"in,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// ifaceName is what Linux allows in an interface name, and no more: IFNAMSIZ
// is sixteen including the NUL.
var ifaceName = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,15}$`)

// Validate refuses a filter this code cannot interpret. Called by the web
// process at its trust boundary and again by the core.
func (f PacketLogFilter) Validate() error {
	for _, v := range []struct{ name, val string }{{"src", f.Src}, {"dst", f.Dst}} {
		if v.val == "" {
			continue
		}
		if _, err := addrOrPrefix(v.val); err != nil {
			return fmt.Errorf("%s: %w", v.name, err)
		}
	}
	if f.Proto != "" && !slices.Contains(PacketLogProtos, f.Proto) {
		return fmt.Errorf("proto: %q is not one of %v", f.Proto, PacketLogProtos)
	}
	if f.Rule != "" && f.Rule != PacketLogRuleOther && !slices.Contains(PacketLogRules, f.Rule) {
		return fmt.Errorf("rule: %q is not a rule easywall logs", f.Rule)
	}
	if f.InDev != "" && !ifaceName.MatchString(f.InDev) {
		return fmt.Errorf("in: %q is not an interface name", f.InDev)
	}
	if f.Limit < 0 {
		return fmt.Errorf("limit: %d is negative", f.Limit)
	}
	return nil
}

// IsZero reports whether no field narrows anything.
func (f PacketLogFilter) IsZero() bool { return f == PacketLogFilter{} }

// EffectiveLimit is how many entries a query returns at most.
func (f PacketLogFilter) EffectiveLimit() int {
	if f.Limit <= 0 {
		return packetLogDefaultLimit
	}
	return min(f.Limit, packetLogMaxLimit)
}

// Matcher compiles the filter once, so a query over twenty thousand entries
// parses the addresses once rather than twenty thousand times.
func (f PacketLogFilter) Matcher() (func(PacketLogEntry) bool, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	var src, dst netip.Prefix
	if f.Src != "" {
		src, _ = addrOrPrefix(f.Src)
	}
	if f.Dst != "" {
		dst, _ = addrOrPrefix(f.Dst)
	}
	return func(e PacketLogEntry) bool {
		return (!src.IsValid() || src.Contains(e.Src.Unmap())) &&
			(!dst.IsValid() || dst.Contains(e.Dst.Unmap())) &&
			(f.Port == 0 || e.DstPort == f.Port) &&
			(f.Proto == "" || e.Proto == f.Proto) &&
			(f.Rule == "" || e.Rule == f.Rule) &&
			(f.InDev == "" || e.InDev == f.InDev)
	}, nil
}

// addrOrPrefix reads "203.0.113.9" as 203.0.113.9/32, and unmaps and unzones
// first so that ::ffff:203.0.113.9 is the IPv4 address it is.
func addrOrPrefix(s string) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is neither an address nor a network", s)
	}
	a = a.Unmap().WithZone("")
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// PacketLogResult is the reply to GET_PACKET_LOG.
type PacketLogResult struct {
	Entries []PacketLogEntry `json:"entries"` // newest first, at most EffectiveLimit
	Matched int              `json:"matched"` // how many in the ring matched, shown or not
	Held    int              `json:"held"`    // how many the ring holds

	// Listening is whether the core holds its NFLOG group. False with a Reason
	// means the bind failed and the log rules write to the kernel log instead.
	Listening bool      `json:"listening"`
	Group     uint16    `json:"group"`
	Reason    string    `json:"reason,omitempty"`
	Since     time.Time `json:"since,omitempty"` // when the ring began: start, or the oldest replayed entry

	Discarded uint64 `json:"discarded"` // packets too short to decode, and torn lines in the file
	Lost      uint64 `json:"lost"`      // times the socket overran and the kernel dropped messages
	Persisted bool   `json:"persisted"`
}
