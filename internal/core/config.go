package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jpylypiw/easywall/internal/shared"
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
	return nil
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

// VersionCachePath returns the path for the version check cache file.
func (c *Config) VersionCachePath() string {
	return c.DataDir + "/version_cache.json"
}

// WriteDefaultCoreConfig writes a default easywall.toml to path.
// Used during installation / first run.
func WriteDefaultCoreConfig(path string) error {
	const defaultConfig = `# easywall core configuration
# See documentation at https://jpylypiw.github.io/easywall/configuration

socket_path = "/run/easywall/core.sock"
data_dir    = "/var/lib/easywall"
log_dir     = "/var/log/easywall"

[acceptance]
enabled  = true
duration = 120  # seconds before auto-rollback

[ipv6]
enabled                          = true
icmp_allow_router_advertisement  = true
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
	return os.WriteFile(path, []byte(defaultConfig), 0640)
}

// generateSecret generates a cryptographically random hex string of byteLen bytes.
func generateSecret(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
