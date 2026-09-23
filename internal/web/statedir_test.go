package web

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
)

// TestEveryWebStatePathIsInTheStateDir holds the web's half of the 2.22 split:
// every file this process keeps under data_dir is in data_dir/web — its own
// directory — and is on stateFiles, so an upgrade brings it along. Walks every
// *Path method on Config, so a path added later is covered without anybody
// remembering to list it here.
func TestEveryWebStatePathIsInTheStateDir(t *testing.T) {
	c := &Config{}
	c.DataDir = "/d"
	c.SSLDir = "/s"
	v := reflect.ValueOf(c)
	seen := 0
	for i := 0; i < v.NumMethod(); i++ {
		m := v.Type().Method(i)
		if !strings.HasSuffix(m.Name, "Path") || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 ||
			m.Type.Out(0).Kind() != reflect.String {
			continue
		}
		p := v.Method(i).Call(nil)[0].String()
		if !strings.HasPrefix(p, "/d/") {
			continue
		}
		seen++
		if filepath.Dir(p) != "/d/web" {
			t.Errorf("%s is %s: in data_dir itself, which is root's since 2.22 — this process "+
				"cannot write there", m.Name, p)
		}
		if !slices.Contains(stateFiles, filepath.Base(p)) {
			t.Errorf("%s names %s, which stateFiles does not list: an upgrade leaves it behind",
				m.Name, filepath.Base(p))
		}
	}
	if seen < 4 {
		t.Fatalf("found %d *Path methods under data_dir, want at least 4: the walk is reading nothing", seen)
	}
}

// TestAnUpgradeBringsTheStateAlongAndFollowsNoLink: a file from before 2.22
// moves into data_dir/web; a link in the old layout — the thing that layout let
// the web user plant — is neither followed nor moved.
func TestAnUpgradeBringsTheStateAlongAndFollowsNoLink(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "totp_replay.json"), []byte(`{"step":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dataDir, "victim")
	if err := os.WriteFile(victim, []byte("not state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dataDir, "telemetry.json")); err != nil {
		t.Fatal(err)
	}

	prepareStateDir(dataDir)

	web := filepath.Join(dataDir, "web")
	if info, err := os.Stat(web); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("data_dir/web is %v (%v), want a 0700 directory", info, err)
	}
	if got, _ := os.ReadFile(filepath.Join(web, "totp_replay.json")); string(got) != `{"step":7}` { // #nosec G304 -- the test's own file
		t.Errorf("the TOTP replay step did not move: web/totp_replay.json reads %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "totp_replay.json")); err == nil {
		t.Error("the original is still in data_dir after a move that could remove it")
	}
	if _, err := os.Lstat(filepath.Join(web, "telemetry.json")); err == nil {
		t.Error("a link in data_dir was followed into the state directory")
	}
}

// TestAnUpgradeIntoADataDirItCannotWriteStillKeepsThePasskeys is the order a
// manual install can take: data_dir already 0750, the file still in it. A
// rename fails there; the copy must not, or the passkeys read as none and a
// passkey-only account signs in on its password.
func TestAnUpgradeIntoADataDirItCannotWriteStillKeepsThePasskeys(t *testing.T) {
	dataDir := t.TempDir()
	old := newPasskeyStore(filepath.Join(dataDir, "passkeys.json"))
	if err := old.add("YubiKey", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dataDir, "web"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dataDir, 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o700) })

	prepareStateDir(dataDir)

	if got := newPasskeyStore(filepath.Join(dataDir, "web", "passkeys.json")).all(); len(got) != 1 {
		t.Fatalf("the state directory holds %d passkeys after the upgrade, want 1", len(got))
	}
}

// TestAStateFileAlreadyMovedIsNotReplaced: the newer file wins, so a second
// start — or a package that moved it first — never rolls it back.
func TestAStateFileAlreadyMovedIsNotReplaced(t *testing.T) {
	dataDir := t.TempDir()
	web := filepath.Join(dataDir, "web")
	if err := os.Mkdir(web, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "totp_replay.json"), []byte(`{"step":9}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "totp_replay.json"), []byte(`{"step":3}`), 0600); err != nil {
		t.Fatal(err)
	}

	prepareStateDir(dataDir)

	if got := newTOTPReplay(filepath.Join(web, "totp_replay.json")).last(); got != 9 {
		t.Errorf("the replay step is %d after a second start, want 9: an old file rolled a newer one back", got)
	}
}

// TestTheServerReadsThePasskeysAnUpgradeLeftInDataDir: NewServer is what runs
// the move, before the stores are opened. Built through NewServer for that
// reason, not newTestServer.
func TestTheServerReadsThePasskeysAnUpgradeLeftInDataDir(t *testing.T) {
	dir := t.TempDir()
	sslDir := filepath.Join(dir, "ssl")
	old := newPasskeyStore(filepath.Join(dir, "passkeys.json"))
	if err := old.add("YubiKey", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "web.toml")
	if err := os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:19877"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
demo_mode = true
update_check = false
username = "admin"
password = ""
[tls]
cert = ""
key  = ""
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(s.Stop)
	if n := len(s.passkeys.all()); n != 1 {
		t.Errorf("the server sees %d passkeys, want the 1 enrolled before the upgrade", n)
	}
}
