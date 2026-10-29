package core

import (
	"os"
	"path/filepath"
	"testing"
)

// A snapshot written before an option existed has no field for it. A restore
// now enforces the snapshot, so the field must read as the shipped default —
// read as zero, upgrading would switch the option off at the next boot.
func TestReadAppliedConfig_AFieldTheSnapshotPredatesIsTheDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied-config.json")
	// A 2.24-era snapshot: no ICMPAllowEchoRequest, which 2.25 shipped on.
	if err := os.WriteFile(path, []byte(`{"firewall":{"SSHBruteForce":true},"network":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := readAppliedConfig(path)
	if err != nil || !res.Recorded {
		t.Fatalf("recorded=%v err=%v", res.Recorded, err)
	}
	if !res.Config.Firewall.ICMPAllowEchoRequest {
		t.Error("an option the snapshot predates read as off; it must read as its shipped default")
	}
	if !res.Config.Network.Docker.Enabled {
		t.Error("docker.enabled, absent from the snapshot, read as off")
	}
}
