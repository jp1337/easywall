package shared

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrConfigExists is returned when the target of --write-config is already
// there. Nothing is written and nothing is read.
var ErrConfigExists = errors.New("the file already exists")

// WriteDefaultConfig writes a commented default configuration to path.
//
// It refuses to overwrite. The whole point of the flag is to produce a file on
// a host that has none, and the paths people pass it — /etc/easywall/*.toml —
// are the ones holding a working firewall's settings and, for web.toml, the
// key that signs its session cookies. A flag that silently replaced either
// would be a footgun in the shape of a convenience.
//
// The mode is 0600 for both files, which is what the package installs: one is
// read by root and must not be writable by the web process, and the other
// carries the session secret and the password hash.
func WriteDefaultConfig(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s: %w", path, ErrConfigExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, err)
	}

	// A missing directory is reported as itself rather than created. Writing
	// /etc/easywall into existence with the wrong owner is how the web process
	// came to be unable to reach its own configuration; whoever creates that
	// directory has to decide its ownership deliberately.
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	// O_EXCL rather than a second existence check: between the Stat above and
	// this call another process may have created the file, and the kernel is
	// the only thing that can answer that without a race.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s: %w", path, ErrConfigExists)
		}
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
