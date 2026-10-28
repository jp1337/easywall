package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A key the file does not name is the shipped default, for every key — not
// only for the ones somebody remembered to backfill. LoadConfig decoded into a
// zero Config, so a hand-written or templated easywall.toml without
// acceptance.enabled had no rollback window, without docker.enabled lost
// container networking, and without the ICMPv6 keys broke IPv6 — while the
// schema and configuration.md said each defaults to true.
func TestLoadConfig_AnAbsentKeyIsTheShippedDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "core.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := shared.CoreDefault()
	want.SocketPath, want.DataDir, want.LogDir = cfg.SocketPath, cfg.DataDir, cfg.LogDir
	got := cfg.CoreConfig
	// An empty list is empty whether it decoded as [] or was cloned to nil.
	for _, l := range []*[]string{&got.Docker.CustomNetworks, &got.Routing.Networks,
		&want.Docker.CustomNetworks, &want.Routing.Networks} {
		if len(*l) == 0 {
			*l = nil
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("absent keys must read as the shipped defaults\n got  %+v\n want %+v", got, want)
	}
}

// A list the file empties stays empty: decoding over the defaults must replace
// a slice, never merge into it.
func TestLoadConfig_AnEmptiedListStaysEmpty(t *testing.T) {
	if len(shared.CoreDefault().Docker.CustomNetworks) == 0 && len(shared.CoreDefault().Routing.Networks) == 0 {
		t.Skip("no list ships non-empty; nothing to empty")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "core.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\nlog_dir = \"" + dir + "\"\n" +
		"[docker]\ncustom_networks = []\n[routing]\nnetworks = []\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Docker.CustomNetworks) != 0 || len(cfg.Routing.Networks) != 0 {
		t.Errorf("emptied lists came back: %v %v", cfg.Docker.CustomNetworks, cfg.Routing.Networks)
	}
}
