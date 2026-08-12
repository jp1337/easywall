package shared

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDefaultConfig_WritesTheContentReadableOnlyByItsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easywall.toml")

	if err := WriteDefaultConfig(path, []byte("socket_path = \"/run/x.sock\"\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "socket_path = \"/run/x.sock\"\n" {
		t.Errorf("wrote %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// 0600 because the same flag writes web.toml, which carries the key that
	// signs session cookies and the password hash.
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode is %04o, want 0600 — a configuration holding the session "+
			"key must not be readable by everyone on the host", mode)
	}
}

// The flag exists to create a file that is missing, and both paths it is
// documented with — /etc/easywall/easywall.toml and /etc/easywall/web.toml —
// hold a working firewall's settings once easywall is running. Replacing one
// would discard the account, the session key and every option, in a command
// whose name sounds like it prints something.
func TestWriteDefaultConfig_RefusesToOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.toml")
	const original = "username = \"admin\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteDefaultConfig(path, []byte("username = \"\"\n"))
	if !errors.Is(err, ErrConfigExists) {
		t.Fatalf("error is %v, want ErrConfigExists", err)
	}

	got, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("the file was modified despite the refusal: %q", got)
	}
}

// A missing directory is said out loud rather than created. Whoever creates
// /etc/easywall decides its ownership, and getting that wrong is what left the
// web process unable to reach its own configuration in the 2.5.0 package.
func TestWriteDefaultConfig_DoesNotCreateTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "easywall.toml")

	err := WriteDefaultConfig(path, []byte("x = 1\n"))
	if err == nil {
		t.Fatal("no error for a missing parent directory")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error is %v, want it to wrap os.ErrNotExist so the message names "+
			"the directory", err)
	}
	if _, statErr := os.Stat(filepath.Dir(path)); statErr == nil {
		t.Error("the directory was created")
	}
}
