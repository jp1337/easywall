package core

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/internal/shared"
)

// Config is the runtime configuration for easywall-core.
type Config struct {
	shared.CoreConfig

	// Derived fields (not in TOML)
	configPath string
}

// LoadConfig reads and parses the TOML config at path.
// Returns a descriptive error if the file cannot be read or parsed.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg.CoreConfig); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.configPath = path
	return &cfg, nil
}

// Validate checks all required fields and returns a descriptive error if
// something is missing or invalid. No silent defaults for security-critical values.
func (c *Config) Validate() error {
	if c.SocketPath == "" {
		return fmt.Errorf("socket_path is required")
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.LogDir == "" {
		return fmt.Errorf("log_dir is required")
	}
	if c.Acceptance.Duration <= 0 {
		return fmt.Errorf("acceptance.duration must be > 0 (seconds)")
	}

	// SSH brute force limits
	if c.Firewall.SSHBruteForce {
		if c.Firewall.SSHBruteForceConnectionLimit <= 0 {
			c.Firewall.SSHBruteForceConnectionLimit = 5
		}
	}
	if c.Firewall.ICMPFlood {
		if c.Firewall.ICMPFloodConnectionLimit <= 0 {
			c.Firewall.ICMPFloodConnectionLimit = 10
		}
	}
	if c.Firewall.SYNFlood {
		if c.Firewall.SYNFloodLimit <= 0 {
			c.Firewall.SYNFloodLimit = 100
		}
	}
	if c.Firewall.ConnectionLimit {
		if c.Firewall.ConnectionLimitMax <= 0 {
			c.Firewall.ConnectionLimitMax = 100
		}
	}
	if c.Firewall.LogBlocked {
		if c.Firewall.LogBlockedLimit <= 0 {
			c.Firewall.LogBlockedLimit = 60
		}
	}

	c.migrateIPv6Mode()
	if !c.IPv6.Mode.Valid() {
		return fmt.Errorf("ipv6.mode must be one of %q, %q or %q, got %q",
			shared.IPv6Filter, shared.IPv6Passthrough, shared.IPv6Block, c.IPv6.Mode)
	}
	return nil
}

// migrateIPv6Mode fills in ipv6.mode for a config written before 2.5.0.
//
// The old key was ipv6.enabled, and it did not mean what it said. `false`
// removed the ICMPv6 exemptions and left every other rule applying to IPv6, so
// the effect was a filtered but non-functional IPv6 stack — not the
// "unfiltered" the documentation promised, and not "blocked" either. There is
// no faithful translation of that, so both old values map to `filter`: it is
// the safe direction, it is what the majority already had, and for anyone who
// set `false` it repairs an IPv6 stack that was quietly broken.
//
// Someone who genuinely wanted IPv6 out of the way now has two settings that
// actually do it, and the release notes point at them.
func (c *Config) migrateIPv6Mode() {
	if c.IPv6.Mode != "" {
		return
	}
	c.IPv6.Mode = shared.IPv6Filter
	if !c.IPv6.Enabled {
		slog.Warn("ipv6.enabled is obsolete and did not do what it said; " +
			"using ipv6.mode = \"filter\". Set ipv6.mode to \"passthrough\" or " +
			"\"block\" if you want IPv6 left alone or dropped")
	}
}

// AcceptanceDuration returns the acceptance window as a time.Duration.
func (c *Config) AcceptanceDuration() time.Duration {
	return time.Duration(c.Acceptance.Duration) * time.Second
}

// RulesPath returns the absolute path to rules.json.
func (c *Config) RulesPath() string {
	return c.DataDir + "/rules.json"
}

// AuditLogPath returns the absolute path to the audit log.
func (c *Config) AuditLogPath() string {
	return c.LogDir + "/audit.log"
}

// LastApplyPath returns the path of the file recording when rules were last
// applied and accepted. It is kept on disk because the dashboard shows it, and
// a value held only in memory reset to "never" on every daemon restart while
// the rules it referred to were still live.
func (c *Config) LastApplyPath() string {
	return c.DataDir + "/last_apply"
}

// VersionCachePath returns the path for the version check cache file.
func (c *Config) VersionCachePath() string {
	return c.DataDir + "/version_cache.json"
}

// SaveNetworkSettings updates the [ipv6] and [docker] sections and atomically persists the config.
func (c *Config) SaveNetworkSettings(s shared.NetworkSettings) error {
	// Unset means filter, the same as everywhere else — a caller that omits the
	// field gets the safe disposition rather than an error. A value that is set
	// and wrong is a different matter: guessing which of open or closed the
	// caller meant is not something to do quietly.
	if s.IPv6.Mode == "" {
		s.IPv6.Mode = shared.IPv6Filter
	}
	if !s.IPv6.Mode.Valid() {
		return fmt.Errorf("unknown ipv6 mode %q", s.IPv6.Mode)
	}
	// Every entry has to be a network the apply step can turn into a rule.
	// addCIDRAccept returns quietly on anything it cannot parse, so an unchecked
	// entry was listed here as whitelisted and never reached the kernel — the
	// same silent skip the blacklist had, in the direction where the operator
	// finds out because something they expected to work does not.
	for _, cidr := range s.Docker.CustomNetworks {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("docker custom network %q: not a CIDR network", cidr)
		}
	}
	c.IPv6 = s.IPv6
	c.Docker = s.Docker
	return c.save()
}

// SaveFirewallOptions updates the [firewall] section and atomically persists the config.
func (c *Config) SaveFirewallOptions(opts shared.FirewallOptions) error {
	c.Firewall = opts
	return c.save()
}

// SaveSystemSettings updates the [acceptance] section and atomically persists the config.
func (c *Config) SaveSystemSettings(s shared.SystemSettings) error {
	c.Acceptance = s.Acceptance
	return c.save()
}

func (c *Config) save() error {
	dir := filepath.Dir(c.configPath)
	tmp, err := os.CreateTemp(dir, "core-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(c.CoreConfig); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, c.configPath)
}

// WriteDefaultCoreConfig writes a default easywall.toml to path.
// Used during installation / first run.
func WriteDefaultCoreConfig(path string) error {
	const defaultConfig = `# easywall core configuration
# See documentation at https://jp1337.github.io/easywall/configuration

socket_path = "/run/easywall/core.sock"
data_dir    = "/var/lib/easywall"
log_dir     = "/var/log/easywall"

[acceptance]
enabled  = true
duration = 120  # seconds before auto-rollback

[ipv6]
# filter      — apply every rule to IPv6 as well as IPv4 (default)
# passthrough — accept all IPv6 before any rule; IPv6 is managed elsewhere
# block       — drop all IPv6 except loopback
mode                              = "filter"
icmp_allow_router_advertisement   = true
icmp_allow_neighbor_advertisement = true

[docker]
enabled              = false
allow_bridge_networks = true
custom_networks      = []

[firewall]
# --- Always active (no config needed) ---
# loopback accept, established/related accept, basic ICMP

# --- Optional protection modules ---
ssh_brute_force                      = true
ssh_brute_force_log                  = false
ssh_brute_force_connection_limit     = 5
ssh_brute_force_log_limit            = 60

icmp_flood                           = true
icmp_flood_log                       = false
icmp_flood_connection_limit          = 10
icmp_flood_log_limit                 = 60

syn_flood                            = true
syn_flood_log                        = false
syn_flood_limit                      = 100

port_scan                            = true
port_scan_log                        = false

drop_invalid_packets                 = true
drop_invalid_packets_log             = false

drop_fragments                       = false
drop_fragments_log                   = false

bogon_filter                         = false
bogon_filter_log                     = false

connection_limit_per_ip              = false
connection_limit_max                 = 100

tcp_rst_flood                        = false
tcp_rst_flood_log                    = false
tcp_rst_flood_limit                  = 100

drop_broadcast                       = false
drop_multicast                       = false
drop_anycast                         = false

log_blocked_connections              = false
log_blocked_connections_limit        = 60

log_blacklist_connections            = false
log_blacklist_connections_limit      = 60
`
	return os.WriteFile(path, []byte(defaultConfig), 0600)
}
