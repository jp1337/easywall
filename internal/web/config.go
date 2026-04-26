package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/jpylypiw/easywall/internal/shared"
)

// Config is the runtime configuration for easywall-web.
type Config struct {
	shared.WebConfig
	configPath string
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
	if c.SocketPath == "" {
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
	if c.CSRFKey == "" {
		return fmt.Errorf("csrf_key is required")
	}
	if len(c.CSRFKey) < 32 {
		return fmt.Errorf("csrf_key must be at least 32 characters")
	}
	if c.Language == "" {
		c.Language = "en"
	}
	return nil
}

// IsFirstRun returns true when no password hash is set (first-run wizard needed).
func (c *Config) IsFirstRun() bool {
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

// VersionCachePath returns the path for the version check cache file.
func (c *Config) VersionCachePath() string {
	if c.DataDir != "" {
		return c.DataDir + "/version_cache.json"
	}
	return c.SSLDir + "/../version_cache.json"
}

// SaveCredentials persists updated username and password hash to the config file.
func (c *Config) SaveCredentials(username, passwordHash string) error {
	c.Username = username
	c.Password = passwordHash
	return c.save()
}

func (c *Config) save() error {
	dir := filepath.Dir(c.configPath)
	tmp, err := os.CreateTemp(dir, "web-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(c.WebConfig); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
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
	csrfKey, err := generateSecret(32)
	if err != nil {
		return err
	}

	content := fmt.Sprintf(`# easywall web configuration
# See documentation at https://jpylypiw.github.io/easywall/configuration

bind_addr    = "0.0.0.0:12227"
socket_path  = "/run/easywall/core.sock"
ssl_dir      = "/etc/easywall/ssl"
language     = "en"

# Auto-generated secrets — keep these private!
session_key = %q
csrf_key    = %q

# Set via first-run wizard — do not edit manually
username = ""
password = ""

[tls]
# Leave empty to use auto-generated self-signed certificate in ssl_dir.
# Set to paths of your own certificate/key for custom TLS (e.g. Let's Encrypt).
cert = ""
key  = ""
`, sessionKey, csrfKey)

	return os.WriteFile(path, []byte(content), 0640)
}

// generateSecret generates a cryptographically random hex string of byteLen bytes.
func generateSecret(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
