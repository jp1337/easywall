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
	"regexp"
	"strconv"
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

// TOTPSecret returns the enrolled second-factor secret, or "" when there is none.
func (c *Config) TOTPSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WebConfig.TOTPSecret
}

// TOTPEnabled reports whether a second factor is enrolled.
func (c *Config) TOTPEnabled() bool { return c.TOTPSecret() != "" }

// RecoveryCodes returns a copy of the stored hashes.
//
// A copy, because the caller consumes one and hands the rest back, and handing
// out the slice the config holds would let that edit land without the lock.
func (c *Config) RecoveryCodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.WebConfig.RecoveryCodes))
	copy(out, c.WebConfig.RecoveryCodes)
	return out
}

// LoadConfig reads and parses the TOML config at path.
func LoadConfig(path string) (*Config, error) {
	// #nosec G304 -- path is the --config argument this process was started with,
	// never anything a request supplied.
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

// TelemetryEnabled reports whether the operator agreed to be counted. Unset
// means no — consent is asked for, never assumed.
func (c *Config) TelemetryEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Telemetry != nil && *c.Telemetry
}

// VersionCachePath returns the path for the version check cache file.
func (c *Config) VersionCachePath() string {
	if c.DataDir != "" {
		return c.DataDir + "/version_cache.json"
	}
	return c.SSLDir + "/../version_cache.json"
}

// TelemetryStatePath returns the path for the installation identifier and the
// last-reported stamp.
func (c *Config) TelemetryStatePath() string {
	if c.DataDir != "" {
		return c.DataDir + "/telemetry.json"
	}
	return c.SSLDir + "/../telemetry.json"
}

// TOTPReplayPath returns the path for the last accepted TOTP step.
//
// In data_dir and not in web.toml: this changes once per login, and web.toml is
// rewritten in place on a directory the packaged layout does not let this
// process create a temp file in.
func (c *Config) TOTPReplayPath() string {
	if c.DataDir != "" {
		return c.DataDir + "/totp_replay.json"
	}
	return c.SSLDir + "/../totp_replay.json"
}

// SaveTelemetry records the operator's answer to being counted.
//
// Separate from every other save on purpose: withdrawing consent must work
// when nothing else does. It touches only web.toml, so it does not depend on
// the core being reachable.
//
// Rolled back on a failed write like every Save* below: otherwise a transient
// failure leaves consent looking withdrawn until the next restart quietly
// brings back whatever disk still holds.
func (c *Config) SaveTelemetry(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := c.Telemetry
	c.Telemetry = &enabled
	if err := c.saveLocked(); err != nil {
		c.Telemetry = prev
		return err
	}
	return nil
}

// SaveCredentials persists updated username and password hash to the config file.
//
// Rolled back on a failed write, the same shape as SaveTOTP: setting the
// fields first and saving second would make currentCredential() report the
// new password the instant a full disk refused the write meant to make it so.
// RequireAuth compares every request's session fingerprint against that
// value, so the operator's own session would be evicted, the internal_error
// flash would never reach them because the redirect it fires on is keyed to
// the credential that just changed under them, and the new password would
// work while the old one — the one actually on disk — was refused until a
// restart flipped the in-memory value back.
func (c *Config) SaveCredentials(username, passwordHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prevUsername, prevPassword := c.Username, c.Password
	c.Username = username
	c.Password = passwordHash
	if err := c.saveLocked(); err != nil {
		c.Username, c.Password = prevUsername, prevPassword
		return err
	}
	return nil
}

// SaveTOTP writes the second-factor secret and its recovery hashes in one write.
//
// One write and not two: between them sits a file with a secret and no way back,
// or eight codes for a factor that is not enrolled. Passing an empty secret and
// a nil slice switches the factor off, and that clears both — a secret that
// survived "turn it off" would be a factor still enforced after the interface
// said it was not.
//
// The in-memory fields are rolled back when the write fails. Setting them first
// and saving second — the shape every other Save* method here uses — would
// leave TOTPEnabled() reporting a factor is enrolled the instant a full disk
// refused the write that was supposed to make it so, before the operator's own
// request had even finished. Enrolment is the one path that checks its own
// write for exactly that reason: a pending secret is worth keeping only because
// nothing else believes it is confirmed yet.
func (c *Config) SaveTOTP(secret string, recoveryHashes []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prevSecret := c.WebConfig.TOTPSecret
	prevCodes := c.WebConfig.RecoveryCodes
	c.WebConfig.TOTPSecret = secret
	c.WebConfig.RecoveryCodes = append([]string(nil), recoveryHashes...)
	if err := c.saveLocked(); err != nil {
		c.WebConfig.TOTPSecret = prevSecret
		c.WebConfig.RecoveryCodes = prevCodes
		return err
	}
	return nil
}

// SaveRecoveryCodes replaces the stored hashes and leaves the secret alone. It
// is what consuming a code and regenerating the set both come down to.
//
// Rolled back on a failed write, the same shape as SaveTOTP: without it, the
// worst case is regenerating a set — the operator is holding eight new codes
// that were never written, the write fails, and the eight the process now
// believes in are ones nobody has ever seen. With a lost phone that is a
// lockout until a restart discards the unsaved in-memory copy.
//
// The snapshot is a plain slice-header copy, safe for the same reason
// SaveTOTP's is: the field below is always reassigned a freshly allocated
// slice (append([]string(nil), ...) never writes into the old backing array),
// so prevCodes keeps pointing at untouched data through the rollback.
func (c *Config) SaveRecoveryCodes(hashes []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prevCodes := c.WebConfig.RecoveryCodes
	c.WebConfig.RecoveryCodes = append([]string(nil), hashes...)
	if err := c.saveLocked(); err != nil {
		c.WebConfig.RecoveryCodes = prevCodes
		return err
	}
	return nil
}

// ErrAlreadySetUp is returned when the wizard is asked to create an account on
// an installation that already has one.
var ErrAlreadySetUp = errors.New("the account has already been created")

// FirstRunAccount is everything the wizard decides about the account, in one
// value, so the write that persists it cannot be called with the username and
// the hash swapped — they are adjacent strings, and two of the five fields are
// optional.
//
// It lives here beside Config rather than in shared: it is not protocol, and the
// core never sees any of it.
type FirstRunAccount struct {
	Username     string
	PasswordHash string
	Telemetry    bool

	// Empty and nil when no second factor was set up. They are written together
	// with the account or not at all — there is no path that stores one without
	// the other, which is what keeps a secret from existing for an account that
	// does not.
	TOTPSecret     string
	RecoveryHashes []string
}

// SaveFirstRun persists everything the setup wizard decides, in one write.
//
// One write rather than several, because a failure halfway through the first run
// is the worst moment to leave a half-configured file behind: the wizard closes
// as soon as a password exists, and whatever did not land cannot be asked again.
//
// The "is it still the first run" test happens here, under the same lock as the
// write. The handler checks too, but that check and this write are two moments:
// two POSTs arriving together both passed it, both wrote, and the second one
// decided who owns the firewall. The window is small and it sits on a machine
// that is, by definition, freshly exposed and not yet protected.
//
// The in-memory fields are restored when the write fails, for the reason the
// sibling savers record: a transient failure that left c.Password set would send
// the operator's retry into the ErrAlreadySetUp branch above, dead-ending the
// wizard with nothing on disk.
func (c *Config) SaveFirstRun(a FirstRunAccount) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Password != "" {
		return ErrAlreadySetUp
	}

	prevUser, prevPass := c.Username, c.Password
	prevTelemetry := c.Telemetry
	prevSecret, prevCodes := c.WebConfig.TOTPSecret, c.WebConfig.RecoveryCodes

	c.Username = a.Username
	c.Password = a.PasswordHash
	c.Telemetry = &a.Telemetry
	c.WebConfig.TOTPSecret = a.TOTPSecret
	c.WebConfig.RecoveryCodes = append([]string(nil), a.RecoveryHashes...)

	if err := c.saveLocked(); err != nil {
		c.Username, c.Password = prevUser, prevPass
		c.Telemetry = prevTelemetry
		c.WebConfig.TOTPSecret, c.WebConfig.RecoveryCodes = prevSecret, prevCodes
		return err
	}
	return nil
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
	// Read before write: the file on disk is what carries the comments, and
	// what this save has to preserve. A file that has gone missing or become
	// unreadable is not an error here — render falls back to the encoder.
	existing, readErr := os.ReadFile(c.configPath) // #nosec G304 -- the daemon's own config path
	if readErr != nil {
		existing = nil
	}
	data, err := c.render(existing)
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
		//
		// #nosec G703 -- configPath is the -config argument this process was
		// started with, and the read above is what makes gosec call it tainted.
		// Nothing from a request reaches it.
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

// configHeader is written above a config file easywall has had to rebuild from
// the struct, which happens only when the file on disk could not be edited in
// place — see mergeConfig.
const configHeader = `# easywall web configuration
#
# Rebuilt by easywall: the file it replaced could not be edited in place, so the
# comments that were in it are gone. Every key is documented at
# https://easywall-project.org/configuration/

`

// managedKeys are the only keys easywall ever writes. Everything else in
// web.toml is read and never touched, so an edit in place has to reach these
// six and no others.
//
// Ordered as config/web.toml orders them, so a key that has to be appended
// lands somewhere a reader expects it.
var managedKeys = []string{"session_key", "username", "password", "telemetry", "totp_secret", "recovery_codes"}

// encode renders the whole configuration as TOML, comments and all discarded.
// The fallback path — see mergeConfig for when it is taken. c.mu must be held.
//
// #nosec G117 -- the struct being encoded *is* web.toml, and session_key belongs
// in it: easywall generates one on first start and writes it back here, which is
// the only place it is kept. The file is 0600 and owned by the web user. There is
// nothing to redact from a value whose destination is the file it came from.
func (c *Config) encode() ([]byte, error) {
	buf := bytes.NewBufferString(configHeader)
	if err := toml.NewEncoder(buf).Encode(c.WebConfig); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

// render produces the bytes to write: the existing file with the four managed
// values replaced, or a fresh encoding when that cannot be done safely.
//
// The file the package installs is three kilobytes of comments explaining what
// each key does. The encoder serialises a struct, so the first save replaced all
// of it with fourteen bare lines — and on a container installation the first
// save is the first *start*, because the shipped session_key is a placeholder
// that ensureSessionKey replaces and writes back. configuration.md still sends
// operators to this file to configure easywall, and they arrived at one that no
// longer said anything.
//
// c.mu must be held.
func (c *Config) render(existing []byte) ([]byte, error) {
	if merged, ok := mergeConfig(existing, c.WebConfig); ok {
		return merged, nil
	}
	return c.encode()
}

// tomlValue renders a Go value as the TOML scalar for one of the managed keys.
func tomlValue(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return strconv.Quote(t), true
	case *bool:
		if t == nil {
			return "", false // unset: leave the file's line alone
		}
		return strconv.FormatBool(*t), true
	case []string:
		// Always rendered, including as [], because clearing the second factor
		// has to remove the previous codes rather than leave them in the file.
		parts := make([]string, len(t))
		for i, v := range t {
			parts[i] = strconv.Quote(v)
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	default:
		return "", false
	}
}

// managedValues pairs each managed key with the value to write, skipping the
// ones that are unset and therefore have nothing to say.
func managedValues(cfg shared.WebConfig) map[string]string {
	out := make(map[string]string, len(managedKeys))
	for key, v := range map[string]interface{}{
		"session_key":    cfg.SessionKey,
		"username":       cfg.Username,
		"password":       cfg.Password,
		"telemetry":      cfg.Telemetry,
		"totp_secret":    cfg.TOTPSecret,
		"recovery_codes": cfg.RecoveryCodes,
	} {
		if rendered, ok := tomlValue(v); ok {
			out[key] = rendered
		}
	}
	return out
}

// trailingComment returns whatever follows the value on an assignment line —
// a note the operator wrote beside the setting, which has no business being
// deleted because the value next to it changed.
//
// A quoted value is skipped to its closing quote first, so a "#" inside the
// string is not mistaken for the start of a comment.
func trailingComment(rest string) string {
	i := 0
	if strings.HasPrefix(rest, `"`) {
		for i = 1; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++
				continue
			}
			if rest[i] == '"' {
				i++
				break
			}
		}
	}
	if hash := strings.Index(rest[min(i, len(rest)):], "#"); hash >= 0 {
		return rest[min(i, len(rest))+hash-countSpacesBefore(rest, min(i, len(rest))+hash):]
	}
	return ""
}

// countSpacesBefore returns how many spaces or tabs immediately precede index i,
// so the gap between the value and its comment is kept as it was written.
func countSpacesBefore(s string, i int) int {
	n := 0
	for i-n-1 >= 0 && (s[i-n-1] == ' ' || s[i-n-1] == '\t') {
		n++
	}
	return n
}

// keyLineRe matches an assignment to one of the managed keys, capturing the
// indentation, the key, the spacing around "=" and anything trailing.
var keyLineRe = regexp.MustCompile(`^(\s*)(session_key|username|password|telemetry|totp_secret|recovery_codes)(\s*=\s*)(.*)$`)

// mergeConfig replaces the managed values inside the existing file text,
// keeping every comment, blank line and alignment around them. It reports false
// when it cannot be sure of the result, and the caller re-encodes instead.
//
// Only assignments above the first table header are considered: a key of the
// same name inside [tls] is a different key, and rewriting it would move a value
// into a section it does not belong to.
//
// What makes this safe enough to write credentials with is the last step. The
// merged text is decoded again and compared against the configuration it was
// supposed to express; any difference at all, and the whole attempt is thrown
// away in favour of the encoder. There is no path on which a file this function
// half understood reaches disk.
func mergeConfig(existing []byte, cfg shared.WebConfig) ([]byte, bool) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return nil, false
	}

	want := managedValues(cfg)
	lines := strings.Split(string(existing), "\n")
	seen := make(map[string]bool, len(want))
	inTable := false
	lastTopLevel := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inTable = true
			continue
		}
		if inTable {
			continue
		}
		m := keyLineRe.FindStringSubmatch(line)
		if m == nil {
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				lastTopLevel = i
			}
			continue
		}
		key := m[2]
		if seen[key] {
			return nil, false // said twice; which one wins is not ours to decide
		}
		seen[key] = true
		lastTopLevel = i
		if value, ok := want[key]; ok {
			lines[i] = m[1] + key + m[3] + value + trailingComment(m[4])
		}
	}

	// A key the file never had is appended after the last top-level assignment,
	// which keeps it out of whatever table follows.
	var missing []string
	for _, key := range managedKeys {
		if _, ok := want[key]; ok && !seen[key] {
			missing = append(missing, key+" = "+want[key])
		}
	}
	if len(missing) > 0 {
		if lastTopLevel < 0 {
			return nil, false
		}
		rest := append([]string(nil), lines[lastTopLevel+1:]...)
		lines = append(lines[:lastTopLevel+1], append(missing, rest...)...)
	}

	merged := []byte(strings.Join(lines, "\n"))

	// The guard. Decode what we are about to write and insist it says exactly
	// what the caller asked for.
	var check shared.WebConfig
	if _, err := toml.Decode(string(merged), &check); err != nil {
		return nil, false
	}
	if !sameManagedValues(check, cfg) {
		return nil, false
	}
	return merged, true
}

// sameManagedValues reports whether two configurations agree on every value
// easywall writes.
func sameManagedValues(a, b shared.WebConfig) bool {
	if a.SessionKey != b.SessionKey || a.Username != b.Username || a.Password != b.Password {
		return false
	}
	if a.TOTPSecret != b.TOTPSecret {
		return false
	}
	if len(a.RecoveryCodes) != len(b.RecoveryCodes) {
		return false
	}
	for i := range a.RecoveryCodes {
		if a.RecoveryCodes[i] != b.RecoveryCodes[i] {
			return false
		}
	}
	switch {
	case a.Telemetry == nil && b.Telemetry == nil:
		return true
	case a.Telemetry == nil || b.Telemetry == nil:
		return false
	default:
		return *a.Telemetry == *b.Telemetry
	}
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
