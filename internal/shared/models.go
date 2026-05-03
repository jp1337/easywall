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

// AcceptanceConfig controls the two-step activation safety mechanism.
type AcceptanceConfig struct {
	Enabled  bool `toml:"enabled"`
	Duration int  `toml:"duration"` // seconds before auto-rollback
}

// IPv6Config controls IPv6 support.
type IPv6Config struct {
	Enabled                        bool `toml:"enabled"`
	ICMPAllowRouterAdvertisement   bool `toml:"icmp_allow_router_advertisement"`
	ICMPAllowNeighborAdvertisement bool `toml:"icmp_allow_neighbor_advertisement"`
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

	// DemoMode runs the web binary against an in-memory mock instead of the
	// Unix socket — no easywall-core required. Used by the public demo
	// deployment so visitors can explore every page without affecting any
	// real firewall. State resets when the process restarts.
	DemoMode bool `toml:"demo_mode"`
}

// NetworkSettings groups IPv6 and Docker configuration for IPC transport.
type NetworkSettings struct {
	IPv6   IPv6Config   `json:"ipv6"`
	Docker DockerConfig `json:"docker"`
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
}
