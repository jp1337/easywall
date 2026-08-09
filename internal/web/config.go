package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/internal/shared"
)

// Config is the runtime configuration for easywall-web.
//
// Read on every request — the language, the demo flag, and the password hash
// that every authenticated request compares its session against — and written
// when the operator changes their password or completes the first-run wizard.
// Those are different goroutines, and without the lock below that is a data
// race on a string header: the reader can observe the new length with the old
// pointer. The race detector never saw it because no test changed a password
// while another request was in flight.
type Config struct {
	shared.WebConfig

	// mu guards WebConfig. Use the accessors rather than the embedded fields.
	mu         sync.RWMutex
	configPath string
}

// Credentials returns the username and password hash in force right now.
func (c *Config) Credentials() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Username, c.Password
}

// PasswordHash returns the stored hash.
func (c *Config) PasswordHash() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Password
}

// LoadConfig reads and parses the TOML config at path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg.WebConfig); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.configPath = path
	return &cfg, nil
}

// Validate checks all required fields and returns a descriptive error if invalid.
func (c *Config) Validate() error {
	if c.BindAddr == "" {
		return fmt.Errorf("bind_addr is required")
	}
	// Demo mode runs without a core daemon, so the socket path is irrelevant.
	if c.SocketPath == "" && !c.DemoMode {
		return fmt.Errorf("socket_path is required")
	}
	if c.SSLDir == "" {
		return fmt.Errorf("ssl_dir is required")
	}
	if c.DataDir == "" {
		c.DataDir = "/var/lib/easywall"
	}
	if c.SessionKey == "" {
		return fmt.Errorf("session_key is required")
	}
	if c.Language == "" {
		c.Language = "en"
	}
	// Both halves of a custom certificate, or neither.
	//
	// With only one set, easywall generated its own pair into ssl_dir and then
	// served the generated certificate with the configured key — or the other
	// way round. TLS refuses the mismatch, and the operator got "private key
	// does not match public key" from a certificate they never configured.
	switch {
	case c.TLS.CertFile != "" && c.TLS.KeyFile == "":
		return fmt.Errorf("tls.cert is set but tls.key is not; set both, or neither for a self-signed certificate")
	case c.TLS.KeyFile != "" && c.TLS.CertFile == "":
		return fmt.Errorf("tls.key is set but tls.cert is not; set both, or neither for a self-signed certificate")
	}
	return nil
}

// IsFirstRun returns true when no password hash is set (first-run wizard needed).
func (c *Config) IsFirstRun() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Password == ""
}

// CertPath returns the effective TLS certificate path.
func (c *Config) CertPath() string {
	if c.TLS.CertFile != "" {
		return c.TLS.CertFile
	}
	return c.SSLDir + "/cert.pem"
}

// KeyPath returns the effective TLS key path.
func (c *Config) KeyPath() string {
	if c.TLS.KeyFile != "" {
		return c.TLS.KeyFile
	}
	return c.SSLDir + "/key.pem"
}

// LocalesDir returns the locales directory path.
func (c *Config) LocalesDir() string {
	return "locales"
}

// TemplatesDir returns the templates directory path.
func (c *Config) TemplatesDir() string {
	return "web/templates"
}

// StaticDir returns the static assets directory path.
func (c *Config) StaticDir() string {
	return "web/static"
}

// UpdateCheckEnabled reports whether the dashboard may contact the GitHub
// releases API. An absent key means yes, so an existing config keeps behaving
// as it did.
func (c *Config) UpdateCheckEnabled() bool {
	return c.UpdateCheck == nil || *c.UpdateCheck
}

// VersionCachePath returns the path for the version check cache file.
func (c *Config) VersionCachePath() string {
	if c.DataDir != "" {
		return c.DataDir + "/version_cache.json"
	}
	return c.SSLDir + "/../version_cache.json"
}

// SaveCredentials persists updated username and password hash to the config file.
func (c *Config) SaveCredentials(username, passwordHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Username = username
	c.Password = passwordHash
	return c.saveLocked()
}

// saveLocked persists the configuration. c.mu must be held for writing, so the
// file write cannot reorder against the field update.
func (c *Config) saveLocked() error {
	dir := filepath.Dir(c.configPath)
	tmp, err := os.CreateTemp(dir, "web-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(c.WebConfig); err != nil {
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

// WriteDefaultWebConfig writes a default web.toml to path.
func WriteDefaultWebConfig(path string) error {
	sessionKey, err := generateSecret(32)
	if err != nil {
		return err
	}

	content := fmt.Sprintf(`# easywall web configuration
# See documentation at https://jp1337.github.io/easywall/configuration

bind_addr    = "0.0.0.0:12227"
socket_path  = "/run/easywall/core.sock"
ssl_dir      = "/etc/easywall/ssl"
data_dir     = "/var/lib/easywall"
language     = "en"

# The dashboard checks github.com once a day for a newer release. This is the
# only outbound request easywall makes. Set to false to remove it entirely.
update_check = true

# Auto-generated secret — keep this private!
session_key = %q

# Set via first-run wizard — do not edit manually
username = ""
password = ""

[tls]
# Leave empty to use auto-generated self-signed certificate in ssl_dir.
# Set to paths of your own certificate/key for custom TLS (e.g. Let's Encrypt).
cert = ""
key  = ""
`, sessionKey)

	return os.WriteFile(path, []byte(content), 0600)
}

// generateSecret generates a cryptographically random hex string of byteLen bytes.
func generateSecret(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
