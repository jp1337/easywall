package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	if err := c.ensureSessionKey(); err != nil {
		return err
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

// sessionKeyPlaceholder is the value config/web.toml ships with. It is a
// placeholder in the sense that a reader is expected to replace it — and it was
// not enforced, so `docker compose up`, which bind-mounts that very file,
// started a firewall administration interface whose session cookies were signed
// with a key published in the repository. A forged cookie built from it was
// accepted on /dashboard with no password at all.
const sessionKeyPlaceholder = "CHANGE_ME"

// minSessionKeyLen is the shortest key worth signing with. The documented
// recipe is `openssl rand -hex 32`, which produces 64 characters.
const minSessionKeyLen = 32

// ensureSessionKey guarantees the cookie signing key is one nobody else knows.
//
// An unusable key — empty, the shipped placeholder, or too short to be worth
// forging against — is replaced with a fresh one and written back to the config
// file, so the next start keeps the same sessions. Refusing to start instead
// would be defensible, but it would also break `docker compose up` out of the
// box, and an operator who has just been told to edit a file inside a container
// image is an operator who reaches for the fastest way past the message.
func (c *Config) ensureSessionKey() error {
	if c.SessionKey != "" &&
		!strings.Contains(c.SessionKey, sessionKeyPlaceholder) &&
		len(c.SessionKey) >= minSessionKeyLen {
		return nil
	}

	key, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("generate session key: %w", err)
	}
	c.SessionKey = key

	if c.configPath == "" {
		return nil // constructed in memory; nothing to persist to
	}
	if err := c.saveLocked(); err != nil {
		return fmt.Errorf("no usable session_key, and the generated one could not be "+
			"saved to %s (%w). Set session_key yourself: openssl rand -hex 32",
			c.configPath, err)
	}
	slog.Warn("session_key was missing or still the shipped placeholder; " +
		"generated a new one and saved it. Existing sessions are invalidated.")
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
//
// Written atomically where the directory permits it, and in place where it does
// not. The packaged layout is the second case on purpose: /etc/easywall belongs
// to root and holds easywall.toml, the configuration the *root* daemon reads.
// Making that directory writable by the unprivileged web user so it could
// create a temp file there would hand a network-facing process the ability to
// rewrite what root loads — the one thing the two-process split exists to
// prevent. web.toml itself belongs to the web user, so an in-place rewrite
// works and nothing else in that directory is reachable.
func (c *Config) saveLocked() error {
	data, err := c.encode()
	if err != nil {
		return err
	}

	dir := filepath.Dir(c.configPath)
	tmp, err := os.CreateTemp(dir, "web-*.toml.tmp")
	if err != nil {
		if !errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("create temp config: %w", err)
		}
		// No write access to the directory: rewrite the file itself. It is a few
		// hundred bytes and rewritten only when credentials change.
		if err := os.WriteFile(c.configPath, data, 0600); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		return nil
	}

	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, c.configPath)
}

// encode renders the configuration as TOML. c.mu must be held.
func (c *Config) encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c.WebConfig); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

// There is deliberately no WriteDefaultWebConfig here, for the same reason as
// on the core side: config/web.toml is what the package installs, and a second
// default in the binary only drifts from it.

// generateSecret generates a cryptographically random hex string of byteLen bytes.
func generateSecret(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
