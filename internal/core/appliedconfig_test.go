package core

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// countingHandler counts the records it is asked to log, without formatting
// them anywhere a test would need to parse.
type countingHandler struct{ n *int }

func (h countingHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (h countingHandler) Handle(context.Context, slog.Record) error { *h.n++; return nil }
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler        { return h }
func (h countingHandler) WithGroup(string) slog.Handler             { return h }

func TestAppliedConfig_AMissingFileIsNotRecorded(t *testing.T) {
	res, err := readAppliedConfig(filepath.Join(t.TempDir(), "applied-config.json"))
	if err != nil {
		t.Fatalf("a missing snapshot is a state, not an error: %v", err)
	}
	if res.Recorded {
		t.Error("a snapshot that was never written must not claim to be recorded")
	}
}

func TestAppliedConfig_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied-config.json")
	want := shared.AppliedConfig{
		Firewall: shared.FirewallOptions{Fragments: true, ConnectionLimitMax: 250},
		Network:  shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Block}},
	}
	if err := writeAppliedConfig(path, want); err != nil {
		t.Fatalf("writeAppliedConfig: %v", err)
	}

	res, err := readAppliedConfig(path)
	if err != nil {
		t.Fatalf("readAppliedConfig: %v", err)
	}
	if !res.Recorded {
		t.Fatal("a snapshot that was just written reports itself unrecorded")
	}
	if len(shared.DiffConfig(res.Config, want)) != 0 {
		t.Errorf("the snapshot came back different: %v", shared.DiffConfig(res.Config, want))
	}
}

// Status calls appliedConfig, and /apply polls GET_STATUS every 2s. A snapshot
// that exists and will not parse used to warn on every single read — about
// thirty identical journal lines a minute for as long as the page stayed
// open. It must warn once for a given error, stay quiet on repeats of the
// exact same one, and warn again both when the message changes and when a
// later read succeeds and then fails again.
func TestAppliedConfig_CorruptSnapshotWarnsOncePerDistinctError(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	cfg.DataDir = dir
	cfg.LogDir = dir
	f := &Firewall{cfg: cfg}

	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n}))
	defer slog.SetDefault(prev)

	path := cfg.AppliedConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ half a file"), 0600); err != nil {
		t.Fatal(err)
	}

	f.appliedConfig()
	f.appliedConfig()
	f.appliedConfig()
	if n != 1 {
		t.Errorf("three reads of the same corrupt file logged %d times, want 1", n)
	}

	// A different corruption is a different message and warns again.
	if err := os.WriteFile(path, []byte("{ a different half a file, longer"), 0600); err != nil {
		t.Fatal(err)
	}
	f.appliedConfig()
	if n != 2 {
		t.Errorf("a new error message did not produce a new warning; n=%d", n)
	}

	// A successful read clears the throttle, so a later failure warns again too.
	if err := writeAppliedConfig(path, shared.AppliedConfig{}); err != nil {
		t.Fatal(err)
	}
	f.appliedConfig()
	if err := os.WriteFile(path, []byte("{ half a file"), 0600); err != nil {
		t.Fatal(err)
	}
	f.appliedConfig()
	if n != 3 {
		t.Errorf("a failure recurring after a successful read did not warn again; n=%d", n)
	}
}

func TestAppliedConfig_UnreadableContentIsNotRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied-config.json")
	if err := os.WriteFile(path, []byte("{ half a file"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := readAppliedConfig(path)
	if err == nil {
		t.Error("a snapshot that will not parse should say so rather than be believed")
	}
	if res.Recorded {
		t.Error("an unparseable snapshot must not be reported as recorded")
	}
}

// An installation that has not applied or restarted under 2.10 has no snapshot,
// and that state is *unknown*, not *identical*. HasPending therefore keeps its
// 2.9 meaning — the rule diff alone — until a snapshot exists.
func TestStatus_ConfigDriftIsOnlyPendingOnceRecorded(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	cfg.DataDir = dir
	cfg.LogDir = dir
	cfg.Firewall.Fragments = true

	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatal(err)
	}
	f := &Firewall{cfg: cfg, rules: store, nft: &NftablesManager{}, acceptance: NewAcceptance(time.Second)}

	// No snapshot, and staged equals current: nothing is pending, whatever the
	// configuration says.
	if f.Status().HasPending {
		t.Error("an unrecorded configuration reported a pending change; every 2.9 " +
			"installation would light up on the first page load after the upgrade")
	}

	// A snapshot that disagrees with the live configuration is a pending change.
	if err := writeAppliedConfig(cfg.AppliedConfigPath(),
		shared.AppliedConfig{Firewall: shared.FirewallOptions{Fragments: false}}); err != nil {
		t.Fatal(err)
	}
	if !f.Status().HasPending {
		t.Error("drop_fragments was switched on and never applied, and the page with " +
			"the button says there is nothing to apply — this is the defect 2.10 exists for")
	}

	// And a snapshot that agrees is not.
	if err := writeAppliedConfig(cfg.AppliedConfigPath(),
		shared.AppliedConfig{Firewall: cfg.FirewallOptions(), Network: cfg.NetworkSettings()}); err != nil {
		t.Fatal(err)
	}
	if f.Status().HasPending {
		t.Error("a snapshot that matches the live configuration reported a drift")
	}
}
