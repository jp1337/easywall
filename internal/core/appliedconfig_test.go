package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

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
