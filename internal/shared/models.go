package shared

// PortRule represents a TCP or UDP port to be opened.
type PortRule struct {
	Port        string `json:"port"`        // single port "80" or range "8000:9000"
	Description string `json:"description"` // human-readable label
	SSH         bool   `json:"ssh"`         // route through SSH brute-force chain
}

// ForwardingRule represents a port forwarding (NAT PREROUTING) entry.
type ForwardingRule struct {
	Protocol   string `json:"protocol"`    // "tcp" or "udp"
	SourcePort int    `json:"source_port"` // external port
	DestPort   int    `json:"dest_port"`   // internal port
}

// Rules holds all firewall rule sets.
type Rules struct {
	TCP        []PortRule       `json:"tcp"`
	UDP        []PortRule       `json:"udp"`
	Blacklist  []string         `json:"blacklist"`  // blocked source IPs / CIDRs
	Whitelist  []string         `json:"whitelist"`  // always-allowed source IPs / CIDRs
	Forwarding []ForwardingRule `json:"forwarding"` // NAT port forwards
	Custom     []string         `json:"custom"`     // raw nftables rule strings
}

// RulesState holds the three-state rules system preventing lockouts.
// current = applied to kernel, staged = pending user apply, backup = rollback target.
type RulesState struct {
	Current Rules `json:"current"`
	Staged  Rules `json:"staged"`
	Backup  Rules `json:"backup"`
}

// FirewallOptions controls which optional protection modules are enabled.
type FirewallOptions struct {
	// SSH brute-force prevention (rate-limits new connections to SSH port)
	SSHBruteForce                bool `toml:"ssh_brute_force"`
	SSHBruteForceLog             bool `toml:"ssh_brute_force_log"`
	SSHBruteForceConnectionLimit int  `toml:"ssh_brute_force_connection_limit"`
	SSHBruteForceLogLimit        int  `toml:"ssh_brute_force_log_limit"`

	// ICMP flood prevention
	ICMPFlood                bool `toml:"icmp_flood"`
	ICMPFloodLog             bool `toml:"icmp_flood_log"`
	ICMPFloodConnectionLimit int  `toml:"icmp_flood_connection_limit"`
	ICMPFloodLogLimit        int  `toml:"icmp_flood_log_limit"`

	// SYN flood prevention (rate-limits new TCP SYN packets per source IP)
	SYNFlood      bool `toml:"syn_flood"`
	SYNFloodLog   bool `toml:"syn_flood_log"`
	SYNFloodLimit int  `toml:"syn_flood_limit"`

	// Port scan prevention (drops TCP packets with suspicious flag combinations)
	PortScan    bool `toml:"port_scan"`
	PortScanLog bool `toml:"port_scan_log"`

	// Drop packets in INVALID conntrack state
	InvalidPackets    bool `toml:"drop_invalid_packets"`
	InvalidPacketsLog bool `toml:"drop_invalid_packets_log"`

	// Drop IP fragments (common in evasion attacks)
	Fragments    bool `toml:"drop_fragments"`
	FragmentsLog bool `toml:"drop_fragments_log"`

	// Bogon filter: drop packets from RFC-1918/loopback/link-local arriving on external interfaces
	Bogons    bool `toml:"bogon_filter"`
	BogonsLog bool `toml:"bogon_filter_log"`

	// Connection limit per source IP
	ConnectionLimit    bool `toml:"connection_limit_per_ip"`
	ConnectionLimitMax int  `toml:"connection_limit_max"`

	// TCP RST flood prevention
	TCPRSTFlood      bool `toml:"tcp_rst_flood"`
	TCPRSTFloodLog   bool `toml:"tcp_rst_flood_log"`
	TCPRSTFloodLimit int  `toml:"tcp_rst_flood_limit"`

	// Broadcast / multicast / anycast drop
	DropBroadcast bool `toml:"drop_broadcast"`
	DropMulticast bool `toml:"drop_multicast"`
	DropAnycast   bool `toml:"drop_anycast"`

	// General logging of blocked connections
	LogBlocked      bool `toml:"log_blocked_connections"`
	LogBlockedLimit int  `toml:"log_blocked_connections_limit"`

	// Logging of blacklisted connections
	LogBlacklist      bool `toml:"log_blacklist_connections"`
	LogBlacklistLimit int  `toml:"log_blacklist_connections_limit"`
}

// FirewallLimit describes one numeric option: what it is called, the range it
// may hold, the value used when it is absent, and how to reach it in a
// FirewallOptions.
//
// One table, because there were three and they disagreed. The options page
// offered `max="9999"` on all nine fields, the JSON Schemas said 100, 1000,
// 10000 and 100000 for five of them and nothing for the other four, and the
// daemon had no upper bound at all — it checked only that an enabled module's
// limit was positive. An HTML max is a hint to the browser and nothing more,
// which is the same discovery that produced AcceptanceDurationMin/Max; these
// nine were left behind by it.
//
// What the missing bound cost, measured against a real kernel. The limits are
// handed to nftables expressions whose fields are 32 bits, so a large value does
// not fail — it wraps:
//
//	connection_limit_max = 5000000000  ->  ct count over 705032704
//	connection_limit_max = 4294967296  ->  ct count over 0           ← drops everything
//	syn_flood_limit      = 3000000000  ->  limit rate over 3000000000/second burst 1705032704
//
// `ct count over 0` matches every connection from every source and drops it.
// One number, entered on the options page or edited into easywall.toml, turns
// the firewall into a total block — on a product whose first sentence is that it
// cannot lock you out. The daemon logged nothing on any of the three.
type FirewallLimit struct {
	Key     string
	Min     int
	Max     int
	Default int

	// Enabled is the module switch this limit belongs to. A limit only has to be
	// usable when its module is on.
	Enabled func(*FirewallOptions) *bool
	// Value is the limit itself.
	Value func(*FirewallOptions) *int
}

// FirewallLimits is the one description of every numeric firewall option.
//
// The maxima are the ones the JSON Schemas already published where they had an
// opinion, because those were considered and are what an operator's editor has
// been enforcing. The four log limits had none: they are lines per minute, and
// 10000 is 166 a second, which is far past what any disk wants and nowhere near
// where the 32-bit fields wrap.
var FirewallLimits = []FirewallLimit{
	{"ssh_brute_force_connection_limit", 1, 100, 5,
		func(o *FirewallOptions) *bool { return &o.SSHBruteForce },
		func(o *FirewallOptions) *int { return &o.SSHBruteForceConnectionLimit }},
	{"ssh_brute_force_log_limit", 1, 10000, 60,
		func(o *FirewallOptions) *bool { return &o.SSHBruteForceLog },
		func(o *FirewallOptions) *int { return &o.SSHBruteForceLogLimit }},
	{"icmp_flood_connection_limit", 1, 1000, 10,
		func(o *FirewallOptions) *bool { return &o.ICMPFlood },
		func(o *FirewallOptions) *int { return &o.ICMPFloodConnectionLimit }},
	{"icmp_flood_log_limit", 1, 10000, 60,
		func(o *FirewallOptions) *bool { return &o.ICMPFloodLog },
		func(o *FirewallOptions) *int { return &o.ICMPFloodLogLimit }},
	{"syn_flood_limit", 1, 10000, 100,
		func(o *FirewallOptions) *bool { return &o.SYNFlood },
		func(o *FirewallOptions) *int { return &o.SYNFloodLimit }},
	{"tcp_rst_flood_limit", 1, 10000, 100,
		func(o *FirewallOptions) *bool { return &o.TCPRSTFlood },
		func(o *FirewallOptions) *int { return &o.TCPRSTFloodLimit }},
	{"connection_limit_max", 1, 100000, 100,
		func(o *FirewallOptions) *bool { return &o.ConnectionLimit },
		func(o *FirewallOptions) *int { return &o.ConnectionLimitMax }},
	{"log_blocked_connections_limit", 1, 10000, 60,
		func(o *FirewallOptions) *bool { return &o.LogBlocked },
		func(o *FirewallOptions) *int { return &o.LogBlockedLimit }},
	{"log_blacklist_connections_limit", 1, 10000, 60,
		func(o *FirewallOptions) *bool { return &o.LogBlacklist },
		func(o *FirewallOptions) *int { return &o.LogBlacklistLimit }},
}

// InRange reports whether v is a value this limit may hold.
func (l FirewallLimit) InRange(v int) bool { return v >= l.Min && v <= l.Max }

// Clamp brings v into range, for the file path where refusing to start is the
// worse outcome.
func (l FirewallLimit) Clamp(v int) int { return min(max(v, l.Min), l.Max) }

// AcceptanceConfig controls the two-step activation safety mechanism.
type AcceptanceConfig struct {
	Enabled  bool `toml:"enabled"`
	Duration int  `toml:"duration"` // seconds before auto-rollback
}

// Bounds on the acceptance window. The settings page has advertised these as
// the permitted range from the beginning — as an HTML min/max, which is a hint
// to the browser and nothing more. The server took any positive number.
//
// Both ends matter. Below ten seconds the window closes before anyone can read
// the confirmation page, let alone click it, so every apply rolls back and the
// firewall can never be changed. Above an hour a lockout you have already
// noticed keeps you shut out for the rest of it.
const (
	AcceptanceDurationMin = 10
	AcceptanceDurationMax = 3600
)

// ValidAcceptanceDuration reports whether d is within the permitted range.
func ValidAcceptanceDuration(d int) bool {
	return d >= AcceptanceDurationMin && d <= AcceptanceDurationMax
}

// IPv6Mode says what the firewall does with IPv6 traffic.
//
// This replaces a boolean that could not express the question. `enabled = false`
// was documented — in the interface, in its warning, and in configuration.md —
// as "IPv6 traffic is not filtered at all". It did nothing of the sort: the
// table is `inet`, so every rule and the drop policy still applied to IPv6 and
// only the ICMPv6 exemptions were removed. IPv6 came out filtered *and* broken,
// which was neither of the two things an operator might have wanted.
type IPv6Mode string

const (
	// IPv6Filter applies every rule to IPv6 as well as IPv4, and permits the
	// ICMPv6 types IPv6 needs to function. The default, and what almost
	// everyone wants.
	IPv6Filter IPv6Mode = "filter"

	// IPv6Passthrough accepts all IPv6 before any other rule is consulted.
	// For hosts where IPv6 is somebody else's problem — an upstream firewall,
	// a cloud security group. easywall then protects IPv4 only.
	IPv6Passthrough IPv6Mode = "passthrough"

	// IPv6Block drops all IPv6 before any other rule is consulted, apart from
	// loopback. For hosts that have no business speaking IPv6 at all.
	IPv6Block IPv6Mode = "block"
)

// Valid reports whether m is a mode the core knows how to apply.
func (m IPv6Mode) Valid() bool {
	switch m {
	case IPv6Filter, IPv6Passthrough, IPv6Block:
		return true
	default:
		return false
	}
}

// IPv6Config controls IPv6 support.
type IPv6Config struct {
	Mode IPv6Mode `toml:"mode" json:"mode"`

	// Enabled is the pre-2.5.0 spelling, kept only so an existing config still
	// loads. Config.Normalise translates it into Mode and it is not written
	// back. Nothing should read it.
	Enabled bool `toml:"enabled" json:"enabled"`

	ICMPAllowRouterAdvertisement   bool `toml:"icmp_allow_router_advertisement" json:"icmp_allow_router_advertisement"`
	ICMPAllowNeighborAdvertisement bool `toml:"icmp_allow_neighbor_advertisement" json:"icmp_allow_neighbor_advertisement"`
}

// RoutingMode says what the firewall does with traffic this host would route
// rather than receive — between two interfaces, out of a container, into a
// published container port.
//
// A three-way choice for the same reason IPv6Mode is one: the two useful
// answers at the ends are "nothing is routed" and "routing is somebody else's
// business", and between them sits the case most hosts actually have, which is
// a named list. A boolean can express two of the three, and the one it drops is
// the one a VPN gateway needs.
//
// This exists because the forward chain used to be a base chain with policy
// drop and no rules in it, which is not neutrality: it destroyed every routed
// packet, including ones another table's forward chain had already accepted,
// and nothing in the configuration or the documentation mentioned it.
type RoutingMode string

const (
	// RoutingClosed routes nothing beyond what Docker coexistence has allowed.
	// The default, and what easywall has always done — a plain server routes
	// nothing and loses nothing by it.
	RoutingClosed RoutingMode = "closed"

	// RoutingNetworks additionally lets the networks in RoutingConfig.Networks
	// cross the forward chain, in either direction.
	RoutingNetworks RoutingMode = "networks"

	// RoutingOpen leaves routed traffic alone: the forward chain accepts, and
	// easywall filters what arrives for this host only. For a router whose
	// peers change — a VPN concentrator — where there is no list to write down.
	RoutingOpen RoutingMode = "open"
)

// Valid reports whether m is a mode the core knows how to apply.
func (m RoutingMode) Valid() bool {
	switch m {
	case RoutingClosed, RoutingNetworks, RoutingOpen:
		return true
	default:
		return false
	}
}

// RoutingConfig controls what crosses the forward chain.
type RoutingConfig struct {
	Mode RoutingMode `toml:"mode" json:"mode"`

	// Networks are the networks allowed across, consulted under
	// RoutingNetworks. Traffic with a source or a destination inside one of
	// them may cross; everything else still meets the drop policy.
	Networks []string `toml:"networks" json:"networks"`
}

// DockerConfig controls Docker coexistence mode.
type DockerConfig struct {
	Enabled             bool     `toml:"enabled"`               // auto-detect Docker bridges
	AllowBridgeNetworks bool     `toml:"allow_bridge_networks"` // whitelist detected bridge networks
	CustomNetworks      []string `toml:"custom_networks"`       // additional networks to whitelist
}

// CoreConfig is the full configuration for easywall-core.
type CoreConfig struct {
	Firewall   FirewallOptions  `toml:"firewall"`
	Acceptance AcceptanceConfig `toml:"acceptance"`
	IPv6       IPv6Config       `toml:"ipv6"`
	Docker     DockerConfig     `toml:"docker"`
	Routing    RoutingConfig    `toml:"routing"`
	SocketPath string           `toml:"socket_path"`
	DataDir    string           `toml:"data_dir"`
	LogDir     string           `toml:"log_dir"`
}

// TLSConfig controls TLS certificate settings for the web server.
type TLSConfig struct {
	CertFile string `toml:"cert"` // path to custom cert (empty = auto-generate)
	KeyFile  string `toml:"key"`  // path to custom key (empty = auto-generate)
}

// WebConfig is the full configuration for easywall-web.
type WebConfig struct {
	BindAddr   string    `toml:"bind_addr"` // e.g. "0.0.0.0:12227"
	SocketPath string    `toml:"socket_path"`
	SSLDir     string    `toml:"ssl_dir"`
	DataDir    string    `toml:"data_dir"` // writable dir for caches (e.g. version check)
	TLS        TLSConfig `toml:"tls"`
	Language   string    `toml:"language"`    // default locale, e.g. "en"
	SessionKey string    `toml:"session_key"` // HMAC key for gorilla/sessions
	Username   string    `toml:"username"`
	Password   string    `toml:"password"` // argon2id hash

	// UpdateCheck controls whether the dashboard asks the GitHub releases API
	// for the newest version. Unset means on, and on an isolated network it is a
	// request an operator may reasonably want gone entirely rather than merely
	// failing quietly.
	//
	// One of the two requests easywall can make. The other is Telemetry below,
	// which is off unless someone switches it on. docs/_docs/security.md lists both.
	UpdateCheck *bool `toml:"update_check"`

	// Telemetry records whether the operator agreed to easywall counting this
	// installation. Unset means no: consent is asked for, never assumed.
	//
	// When it is on, shared.Reporter sends one request a day carrying a random
	// identifier generated on the machine and the version, and nothing else. The
	// System page turns it off again without the core being reachable, because
	// consent you can only withdraw while another process is up is not consent.
	Telemetry *bool `toml:"telemetry" json:"telemetry"`

	// DemoMode runs the web binary against an in-memory mock instead of the
	// Unix socket — no easywall-core required. Used by the public demo
	// deployment so visitors can explore every page without affecting any
	// real firewall. State resets when the process restarts.
	DemoMode bool `toml:"demo_mode"`
}

// NetworkSettings groups the IPv6, Docker and routing configuration for IPC
// transport. These are the three questions the Network page asks, and an apply
// needs all of them at once.
type NetworkSettings struct {
	IPv6    IPv6Config    `json:"ipv6"`
	Docker  DockerConfig  `json:"docker"`
	Routing RoutingConfig `json:"routing"`
}

// SystemSettings groups the acceptance window configuration for IPC transport.
type SystemSettings struct {
	Acceptance AcceptanceConfig `json:"acceptance"`
}

// AuditLogEntry is a single parsed line from the audit log.
type AuditLogEntry struct {
	Time     string `json:"time"`
	Action   string `json:"action"`
	RuleType string `json:"rule_type"`
	Detail   string `json:"detail"`
	User     string `json:"user"`
}

// AcceptanceStatus represents the current state of a two-step activation cycle.
type AcceptanceStatus string

const (
	AcceptanceIdle       AcceptanceStatus = "idle"
	AcceptancePending    AcceptanceStatus = "pending"
	AcceptanceAccepted   AcceptanceStatus = "accepted"
	AcceptanceRolledBack AcceptanceStatus = "rolled_back"
)

// FirewallStatus is returned by the core daemon for dashboard display.
type FirewallStatus struct {
	Active     bool             `json:"active"`
	Acceptance AcceptanceStatus `json:"acceptance"`
	HasPending bool             `json:"has_pending"`
	LastApply  string           `json:"last_apply"` // RFC3339 timestamp or empty
	// Panic is true while this installation is deliberately unfiltered. It is
	// carried in the status rather than read from disk by the web process
	// because only the core knows where its data directory is — the two may be
	// configured apart — and because the interface has to show it on every page.
	Panic bool `json:"panic"`
}
