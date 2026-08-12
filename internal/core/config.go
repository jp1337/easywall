package core

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/internal/shared"
)

// Config is the runtime configuration for easywall-core.
//
// It is read and written from different goroutines: an apply runs
// asynchronously so the socket stays responsive, and while its acceptance
// window is open the operator can save a setting on another page. Apply reads
// the firewall options, the IPv6 mode and the Docker section; the save writes
// them. Without the lock below that is a data race on a slice header and a
// string — and the race detector never saw it, because no test applied and
// saved at the same time until one dispatched every command in a row.
type Config struct {
	shared.CoreConfig

	// mu guards CoreConfig. Take it for writing in the Save* methods and for
	// reading through the accessors; nothing outside this file should touch the
	// embedded struct directly.
	mu sync.RWMutex

	// Derived fields (not in TOML)
	configPath string
}

// FirewallOptions returns a copy of the [firewall] section.
func (c *Config) FirewallOptions() shared.FirewallOptions {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Firewall
}

// NetworkSettings returns a copy of the [ipv6] and [docker] sections.
//
// CustomNetworks is copied rather than shared: handing out the backing array
// would put the race straight back, one indirection further away.
func (c *Config) NetworkSettings() shared.NetworkSettings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	docker := c.Docker
	docker.CustomNetworks = append([]string(nil), c.Docker.CustomNetworks...)
	routing := c.Routing
	routing.Networks = append([]string(nil), c.Routing.Networks...)
	return shared.NetworkSettings{IPv6: c.IPv6, Docker: docker, Routing: routing}
}

// SystemSettings returns a copy of the [acceptance] section.
func (c *Config) SystemSettings() shared.SystemSettings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return shared.SystemSettings{Acceptance: c.Acceptance}
}

// LoadConfig reads and parses the TOML config at path.
// Returns a descriptive error if the file cannot be read or parsed.
func LoadConfig(path string) (*Config, error) {
	// #nosec G304 -- path is the --config argument this daemon was started with.
	// Nothing from a request reaches it; there is no other way to name a file here.
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
	// A file written by hand, or by an older version that accepted anything, is
	// brought into range rather than kept from starting: the daemon refusing to
	// come up is a worse outcome than a window of a different length.
	if !shared.ValidAcceptanceDuration(c.Acceptance.Duration) {
		clamped := min(max(c.Acceptance.Duration, shared.AcceptanceDurationMin), shared.AcceptanceDurationMax)
		slog.Warn("acceptance.duration is out of range; using the nearest permitted value",
			"configured", c.Acceptance.Duration, "using", clamped,
			"min", shared.AcceptanceDurationMin, "max", shared.AcceptanceDurationMax)
		c.Acceptance.Duration = clamped
	}

	for _, l := range firewallLimits(&c.Firewall) {
		if *l.value > 0 || !*l.enabled {
			continue
		}
		// Substituted rather than refused, for the same reason the acceptance
		// duration is clamped: a firewall daemon that will not start is worse
		// than one running a documented default. But it is said out loud —
		// configuration.md promised "never a silent fallback", and this was five
		// of them.
		slog.Warn("firewall limit is not a positive number; using the default",
			"key", l.key, "configured", *l.value, "using", l.fallback)
		*l.value = l.fallback
	}

	c.migrateIPv6Mode()
	if !c.IPv6.Mode.Valid() {
		return fmt.Errorf("ipv6.mode must be one of %q, %q or %q, got %q",
			shared.IPv6Filter, shared.IPv6Passthrough, shared.IPv6Block, c.IPv6.Mode)
	}

	// An absent [routing] section means closed, which is what every config
	// written before this key existed was already getting. Unset is filled in;
	// set-and-wrong is refused, because the three answers open and close a
	// router and guessing between them is not something to do quietly.
	if c.Routing.Mode == "" {
		c.Routing.Mode = shared.RoutingClosed
	}
	if !c.Routing.Mode.Valid() {
		return fmt.Errorf("routing.mode must be one of %q, %q or %q, got %q",
			shared.RoutingClosed, shared.RoutingNetworks, shared.RoutingOpen, c.Routing.Mode)
	}
	return nil
}

// firewallLimit ties a numeric limit to the module that uses it, so validation
// and the save path cannot disagree about which values matter.
type firewallLimit struct {
	key      string
	enabled  *bool
	value    *int
	fallback int
}

func firewallLimits(o *shared.FirewallOptions) []firewallLimit {
	return []firewallLimit{
		{"ssh_brute_force_connection_limit", &o.SSHBruteForce, &o.SSHBruteForceConnectionLimit, 5},
		{"icmp_flood_connection_limit", &o.ICMPFlood, &o.ICMPFloodConnectionLimit, 10},
		{"syn_flood_limit", &o.SYNFlood, &o.SYNFloodLimit, 100},
		{"tcp_rst_flood_limit", &o.TCPRSTFlood, &o.TCPRSTFloodLimit, 100},
		{"connection_limit_max", &o.ConnectionLimit, &o.ConnectionLimitMax, 100},
		{"log_blocked_connections_limit", &o.LogBlocked, &o.LogBlockedLimit, 60},
		{"log_blacklist_connections_limit", &o.LogBlacklist, &o.LogBlacklistLimit, 60},
	}
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

// Reload re-reads the config file and adopts the sections that can change
// while the daemon runs: [firewall], [acceptance], [ipv6] and [docker].
//
// features/system-settings.md has always told operators to edit easywall.toml
// and send SIGHUP. Nothing handled that signal, and an unhandled SIGHUP
// terminates the process — so following the documentation shut the firewall
// manager down.
//
// The paths are deliberately not reloaded: socket_path, data_dir and log_dir
// are bound at startup, and swapping them under a running daemon would leave
// the socket and the rules file pointing at different places. A change to one
// is reported and ignored.
//
// A file that does not parse or does not validate leaves the running
// configuration exactly as it was. A typo must never disarm anything.
func (c *Config) Reload() error {
	fresh, err := LoadConfig(c.configPath)
	if err != nil {
		return err
	}
	if err := fresh.Validate(); err != nil {
		return fmt.Errorf("refusing to reload: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, p := range []struct{ name, old, new string }{
		{"socket_path", c.SocketPath, fresh.SocketPath},
		{"data_dir", c.DataDir, fresh.DataDir},
		{"log_dir", c.LogDir, fresh.LogDir},
	} {
		if p.old != p.new {
			slog.Warn("ignoring changed path on reload; it takes effect on restart",
				"key", p.name, "running", p.old, "in_file", p.new)
		}
	}

	c.Firewall = fresh.Firewall
	c.Acceptance = fresh.Acceptance
	c.IPv6 = fresh.IPv6
	c.Docker = fresh.Docker
	c.Routing = fresh.Routing
	return nil
}

// AcceptanceDuration returns the acceptance window as a time.Duration.
func (c *Config) AcceptanceDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
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
	if s.Routing.Mode == "" {
		s.Routing.Mode = shared.RoutingClosed
	}
	if !s.Routing.Mode.Valid() {
		return fmt.Errorf("unknown routing mode %q", s.Routing.Mode)
	}
	// Every entry has to be a network the apply step can turn into a rule.
	// addCIDRAccept returns quietly on anything it cannot parse, so an unchecked
	// entry was listed here as whitelisted and never reached the kernel — the
	// same silent skip the blacklist had, in the direction where the operator
	// finds out because something they expected to work does not.
	if err := checkCIDRList("docker custom network", s.Docker.CustomNetworks); err != nil {
		return err
	}
	if err := checkCIDRList("routing network", s.Routing.Networks); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IPv6 = s.IPv6
	c.Docker = s.Docker
	c.Routing = s.Routing
	return c.saveLocked()
}

// checkCIDRList refuses a list holding anything the apply step could not turn
// into a rule. Shared by the two lists on the Network page so they cannot come
// to disagree about what a network is.
func checkCIDRList(what string, entries []string) error {
	for _, cidr := range entries {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("%s %q: not a CIDR network", what, cidr)
		}
	}
	return nil
}

// SaveFirewallOptions updates the [firewall] section and atomically persists the config.
// SaveFirewallOptions updates the [firewall] section and atomically persists the config.
//
// A limit arriving here is being chosen now, so a value that cannot work is
// refused rather than replaced. Storing 100 when the operator asked for 0 leaves
// the file and the interface disagreeing about what the firewall is doing.
func (c *Config) SaveFirewallOptions(opts shared.FirewallOptions) error {
	for _, l := range firewallLimits(&opts) {
		if *l.enabled && *l.value <= 0 {
			return fmt.Errorf("%s must be a positive number, got %d", l.key, *l.value)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.Firewall = opts
	return c.saveLocked()
}

// SaveSystemSettings updates the [acceptance] section and atomically persists the config.
//
// Unlike Validate, a value arriving here is being chosen right now, so it is
// rejected rather than adjusted — silently storing something other than what
// was asked for is how a setting comes to disagree with what it says.
func (c *Config) SaveSystemSettings(s shared.SystemSettings) error {
	if !shared.ValidAcceptanceDuration(s.Acceptance.Duration) {
		return fmt.Errorf("acceptance duration %d is outside %d–%d seconds",
			s.Acceptance.Duration, shared.AcceptanceDurationMin, shared.AcceptanceDurationMax)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Acceptance = s.Acceptance
	return c.saveLocked()
}

// saveLocked persists the configuration. c.mu must be held for writing.
//
// The write happens under the same lock as the field update, and deliberately
// so. Taking a snapshot under a read lock and writing it afterwards left the
// two saves free to reorder: an older snapshot could reach the file after a
// newer one and undo it. Measured at 20 of 100 concurrent saves of two
// different sections. The file is small and saves are rare; holding the lock
// across the write costs nothing worth having.
func (c *Config) saveLocked() error {
	snapshot := c.CoreConfig
	snapshot.Docker.CustomNetworks = append([]string(nil), c.Docker.CustomNetworks...)
	snapshot.Routing.Networks = append([]string(nil), c.Routing.Networks...)

	dir := filepath.Dir(c.configPath)
	tmp, err := os.CreateTemp(dir, "core-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(snapshot); err != nil {
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

// There is deliberately no WriteDefaultCoreConfig here. One existed until this
// release and nothing but its own test ever called it: the configuration that
// actually ships is config/easywall.toml, installed by the package. Two
// definitions of "the default" is one too many, and they had already drifted —
// the dead one carried ipv6.mode while the shipped one still had the obsolete
// ipv6.enabled.
