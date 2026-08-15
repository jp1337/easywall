package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// writeCoreConfig writes a minimal valid easywall.toml with the given extra
// sections appended, and returns its path.
func writeCoreConfig(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"/tmp/probe.sock\"\ndata_dir = \"" + dir +
		"\"\nlog_dir = \"" + dir + "\"\n\n[acceptance]\nduration = 120\n\n[ipv6]\nmode = \"filter\"\n\n" + extra
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A limit above its range must not reach the rule builders.
//
// These numbers are handed to nftables expressions whose fields are 32 bits, so
// too large does not fail — it wraps. Measured against a real kernel before this
// bound existed, with the daemon logging nothing at all:
//
//	connection_limit_max = 5000000000  ->  ct count over 705032704
//	connection_limit_max = 4294967296  ->  ct count over 0
//	syn_flood_limit      = 3000000000  ->  limit rate over 3000000000/second burst 1705032704
//
// `ct count over 0` matches every connection from every source and drops it: one
// number, and the host is behind a total block. The options page advertised
// max="9999" and the schema said 100000, but an HTML attribute is a browser hint
// and a schema is an editor's, so neither was in force anywhere.
func TestAnOutOfRangeLimitIsRefusedOnTheWayIn(t *testing.T) {
	cfg, err := LoadConfig(writeCoreConfig(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	opts := shared.FirewallOptions{
		ConnectionLimit:    true,
		ConnectionLimitMax: 4294967296, // uint32(…) == 0
	}
	err = cfg.SaveFirewallOptions(opts)
	if err == nil {
		t.Fatal("SaveFirewallOptions accepted a limit that becomes `ct count over 0` — " +
			"a rule that drops every connection from every source")
	}
	if !strings.Contains(err.Error(), "connection_limit_max") {
		t.Errorf("the message does not name the key: %v", err)
	}

	// And the value that is merely large, not wrapping.
	opts.ConnectionLimitMax = 5000000000
	if err := cfg.SaveFirewallOptions(opts); err == nil {
		t.Error("SaveFirewallOptions accepted 5000000000, which reaches the kernel as 705032704")
	}

	// The top of the range still goes through.
	opts.ConnectionLimitMax = 100000
	if err := cfg.SaveFirewallOptions(opts); err != nil {
		t.Errorf("the documented maximum was refused: %v", err)
	}
}

// The file path clamps rather than refusing, because a firewall daemon that will
// not start is worse than one running a documented value — the same trade
// acceptance.duration already makes. What it must not do is stay silent.
func TestAnOutOfRangeLimitInTheFileIsClampedAndSaidOutLoud(t *testing.T) {
	cfg, err := LoadConfig(writeCoreConfig(t,
		"[firewall]\nconnection_limit_per_ip = true\nconnection_limit_max = 4294967296\n"+
			"syn_flood = true\nsyn_flood_limit = 3000000000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the daemon refused to start over a limit it can clamp: %v", err)
	}

	opts := cfg.FirewallOptions()
	if opts.ConnectionLimitMax != 100000 {
		t.Errorf("connection_limit_max is %d, want it clamped to 100000", opts.ConnectionLimitMax)
	}
	if opts.SYNFloodLimit != 10000 {
		t.Errorf("syn_flood_limit is %d, want it clamped to 10000", opts.SYNFloodLimit)
	}
}

// Every limit the daemon enforces has to be reachable through the table, or a
// bound is declared and applied to nothing.
func TestEveryFirewallLimitIsWiredToItsOwnField(t *testing.T) {
	for _, l := range shared.FirewallLimits {
		var opts shared.FirewallOptions
		*l.Enabled(&opts) = true
		*l.Value(&opts) = l.Max + 1

		cfg, err := LoadConfig(writeCoreConfig(t, ""))
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveFirewallOptions(opts); err == nil {
			t.Errorf("%s: %d was accepted, one past its maximum of %d", l.Key, l.Max+1, l.Max)
		} else if !strings.Contains(err.Error(), l.Key) {
			t.Errorf("%s: the refusal names something else: %v", l.Key, err)
		}

		// And each one is a distinct field: setting this limit out of range must
		// not be reported against a different key.
		*l.Value(&opts) = l.Max
		if err := cfg.SaveFirewallOptions(opts); err != nil {
			t.Errorf("%s: its own maximum %d was refused: %v", l.Key, l.Max, err)
		}
	}
}
