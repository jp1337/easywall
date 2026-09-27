package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// An applied-config.json written before 2.25 has no ICMPAllowEchoRequest and
// reads as off — what that kernel held for IPv4, which accepted no echo
// request. Until the boot restore or the next apply records the new value,
// /blocked reads such a row as ping_off and the apply preview names the
// switch as pending, which is true (2.25 Review Focus 2).
func TestAPre225AppliedConfigReadsPingsAsOff(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "upgrade-2.22", "applied-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var applied AppliedConfig
	if err := json.Unmarshal(raw, &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Firewall.ICMPAllowEchoRequest {
		t.Fatal("a record from before the key reads pings as on")
	}
	live := applied
	live.Firewall.ICMPAllowEchoRequest = true
	d := DiffConfig(applied, live)
	if len(d) != 1 || d[0].Key != "icmp_allow_echo_request" {
		t.Errorf("the preview says %+v; want exactly icmp_allow_echo_request", d)
	}
}
