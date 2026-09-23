package core

import (
	"log/slog"
	"os"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Every installation upgrading from 2.20 has no [packet_log]. Unset must mean
// the default group, not group 0 — which is ulogd2's.
func TestPacketLog_UnsetIsTheDefault(t *testing.T) {
	c := &Config{}
	if got := c.PacketLogGroup(); got != shared.PacketLogGroupDefault {
		t.Errorf("PacketLogGroup() = %d, want %d", got, shared.PacketLogGroupDefault)
	}
	if got := c.PacketLogEntries(); got != shared.PacketLogEntriesDefault {
		t.Errorf("PacketLogEntries() = %d, want %d", got, shared.PacketLogEntriesDefault)
	}
	if c.PacketLogPersist() {
		t.Error("persist is on with no [packet_log] section; the file holds IP addresses")
	}
}

func TestValidate_ClampsPacketLogEntries(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{10, shared.PacketLogEntriesMin}, {10_000_000, shared.PacketLogEntriesMax}} {
		c := newTestConfig(t)
		c.IPv6.Mode = shared.IPv6Filter
		in := tc.in
		c.PacketLog.Entries = &in
		var n int
		prev := slog.Default()
		slog.SetDefault(slog.New(countingHandler{n: &n, substr: "packet_log.entries"}))
		err := c.Validate()
		slog.SetDefault(prev)
		if err != nil {
			t.Fatalf("Validate refused entries = %d instead of clamping: %v", tc.in, err)
		}
		if got := c.PacketLogEntries(); got != tc.want {
			t.Errorf("entries %d → %d, want %d", tc.in, got, tc.want)
		}
		if n == 0 {
			t.Errorf("entries %d was clamped without a warning naming the key", tc.in)
		}
	}
}

// A group outside uint16 is not a group; stopping with the key named is the
// rule configuration.md states for a value that cannot be interpreted.
func TestValidate_RefusesAGroupThatIsNotOne(t *testing.T) {
	for _, g := range []int{-1, 65536} {
		c := newTestConfig(t)
		c.IPv6.Mode = shared.IPv6Filter
		g := g
		c.PacketLog.Group = &g
		if err := c.Validate(); err == nil {
			t.Errorf("Validate accepted nflog_group = %d", g)
		}
	}
}

// saveLocked used to encode the running c.PacketLog into the file on every
// save from Options, Network or System — the paths are protected from this
// by c.fileConfig; [packet_log] was not. An operator who edits it directly
// for the next restart (the group is bound for the process's life, so
// editing the running config is never an option), and then something
// reloads before that restart happens, must not have the edit erased the
// next time anyone presses Save anywhere.
func TestSavePreservesAPacketLogEditMadeDirectlyInTheFile(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PacketLogGroup() != shared.PacketLogGroupDefault {
		t.Fatalf("fixture already sets a group; the edit below would prove nothing")
	}

	// The operator edits the file directly, for the next restart.
	if err := os.WriteFile(path, []byte(validCoreConfig+"\n[packet_log]\nnflog_group = 5000\n"), 0640); err != nil {
		t.Fatal(err)
	}

	// Something reloads before that restart. The running group must not
	// move (it is bound), but the edit on disk must survive.
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cfg.PacketLogGroup() != shared.PacketLogGroupDefault {
		t.Errorf("the running group changed before a restart: %d", cfg.PacketLogGroup())
	}

	if err := cfg.SaveFirewallOptions(shared.FirewallOptions{}); err != nil {
		t.Fatalf("SaveFirewallOptions: %v", err)
	}

	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if cfg2.PacketLogGroup() != 5000 {
		t.Errorf("nflog_group on disk = %d after a Save, want the operator's edit (5000) kept", cfg2.PacketLogGroup())
	}
}
