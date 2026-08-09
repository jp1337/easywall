package shared

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TelemetryEndpoint is where a counted installation reports in. It is a var so
// tests can point it at an httptest server; nothing else may change it.
//
// Named in configuration.md, because "we send a random identifier and the
// version" is only checkable if the operator also knows where.
var TelemetryEndpoint = "https://telemetry.wdkro.de/v1/count"

const (
	// telemetryInterval is how long a successful report is good for.
	telemetryInterval = 24 * time.Hour

	// telemetryTick is how often the reporter wakes up to ask whether the
	// interval has elapsed. Coarse on purpose: this is a counter, not a metric.
	telemetryTick = time.Hour

	// telemetryMaxSpread is the largest random delay before the first report.
	// Without it, every installation started by the same package upgrade would
	// arrive within the same minute forever after.
	telemetryMaxSpread = time.Hour
)

// telemetryTimeout bounds a single report. A var so a test can prove the bound
// exists without waiting for it.
var telemetryTimeout = 10 * time.Second

// telemetryState is what survives a restart: who this installation is, and
// when it last managed to say so.
type telemetryState struct {
	ID       string `json:"id"`
	LastSent string `json:"last_sent,omitempty"` // RFC3339, only after a success
}

// Reporter reports that this installation exists, once a day, if the operator
// said it may.
//
// It answers one question — how many machines run easywall, and on which
// version — and it is built so that answering it cannot cost the operator
// anything. It never blocks a page, never retries in a tight loop, and a
// failure is a debug line rather than an error the operator has to care about.
//
// Consent is read through Enabled on every attempt rather than captured once,
// so switching the toggle off stops the next report without a restart. That is
// the difference between a setting and a promise.
type Reporter struct {
	// StatePath is the file holding the identifier and the last-sent stamp.
	StatePath string

	// Enabled reports whether the operator currently consents. Required; a nil
	// Enabled means nothing is ever sent.
	Enabled func() bool

	mu    sync.Mutex
	state *telemetryState

	// now and spread exist so tests do not have to wait for real time.
	now    func() time.Time
	spread func() time.Duration
}

// NewReporter returns a Reporter storing its state at statePath.
func NewReporter(statePath string, enabled func() bool) *Reporter {
	return &Reporter{
		StatePath: statePath,
		Enabled:   enabled,
		now:       time.Now,
		spread:    randomSpread,
	}
}

// Run reports for as long as done is open, and returns when it closes.
//
// The first report waits a random part of an hour. Nothing depends on it
// having happened, so there is no reason to make a freshly started machine
// spend its first second on it.
func (r *Reporter) Run(done <-chan struct{}) {
	select {
	case <-time.After(r.spread()):
	case <-done:
		return
	}

	r.reportIfDue()

	ticker := time.NewTicker(telemetryTick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.reportIfDue()
		case <-done:
			return
		}
	}
}

// reportIfDue sends one report if consent is in place and the interval has
// elapsed. Exported behaviour is tested through this.
func (r *Reporter) reportIfDue() {
	if r.Enabled == nil || !r.Enabled() {
		return
	}

	state, err := r.load()
	if err != nil {
		slog.Debug("telemetry: cannot read state", "path", r.StatePath, "error", err)
		return
	}
	if !r.due(state) {
		return
	}

	if err := r.send(state.ID); err != nil {
		// Deliberately not recorded as a send. A host that is offline for a
		// week should report on the day it comes back, not a day later.
		slog.Debug("telemetry: report failed", "error", err)
		return
	}

	state.LastSent = r.now().UTC().Format(time.RFC3339)
	if err := r.save(state); err != nil {
		slog.Debug("telemetry: cannot record the report", "path", r.StatePath, "error", err)
	}
}

func (r *Reporter) due(state *telemetryState) bool {
	if state.LastSent == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, state.LastSent)
	if err != nil {
		return true
	}
	return r.now().Sub(last) >= telemetryInterval
}

// send performs the request: two parameters, nothing else.
//
// Redirects are refused. A counter that follows a redirect can be pointed at a
// third party by whoever controls the endpoint's DNS, and the documentation
// names exactly one destination.
func (r *Reporter) send(id string) error {
	q := url.Values{}
	q.Set("id", id)
	q.Set("v", CurrentVersion)

	req, err := http.NewRequest(http.MethodGet, TelemetryEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("easywall/%s", CurrentVersion))

	client := &http.Client{
		Timeout: telemetryTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("telemetry endpoint redirected")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry endpoint returned %s", resp.Status)
	}
	return nil
}

// load returns the stored state, creating the identifier on first use.
func (r *Reporter) load() (*telemetryState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != nil {
		copied := *r.state
		return &copied, nil
	}

	data, err := os.ReadFile(r.StatePath) // #nosec G304 -- path comes from the daemon's own config
	switch {
	case err == nil:
		var state telemetryState
		if jsonErr := json.Unmarshal(data, &state); jsonErr == nil && state.ID != "" {
			r.state = &state
			copied := state
			return &copied, nil
		}
		// A corrupt file means a new identifier, not a failure. Losing the old
		// one costs one double-count and nothing else.
		slog.Debug("telemetry: unreadable state, starting a new identifier", "path", r.StatePath)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	id, err := newInstallationID()
	if err != nil {
		return nil, err
	}
	state := &telemetryState{ID: id}
	r.state = state
	if err := r.saveLocked(state); err != nil {
		return nil, err
	}
	copied := *state
	return &copied, nil
}

func (r *Reporter) save(state *telemetryState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	return r.saveLocked(state)
}

func (r *Reporter) saveLocked(state *telemetryState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(r.StatePath); dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	// 0600: the identifier is not a secret, but it is the one thing that ties
	// a series of reports together, and nothing else needs to read it.
	return os.WriteFile(r.StatePath, data, 0600)
}

// newInstallationID returns 16 random bytes as hex.
//
// Random, not derived. A hash of the hostname or the machine-id would be
// stable without a file to keep — and would also be reproducible by anyone who
// knows the host, which turns a count into a lookup.
func newInstallationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate installation id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func randomSpread() time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(telemetryMaxSpread)))
	if err != nil {
		return telemetryMaxSpread / 2
	}
	return time.Duration(n.Int64())
}
