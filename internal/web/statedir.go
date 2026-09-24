package web

import (
	"log/slog"
	"os"
	"path/filepath"
)

// The web process keeps its state in <data_dir>/web, a directory only it can
// write, and not in data_dir itself.
//
// Until 2.22 it shared data_dir with the root core — root:easywall 0770, no
// sticky bit — and in a directory you can write you can replace any file and
// plant a link where one was. The web user could swap the core's rules.json,
// which is restored at boot with no acceptance window, and point last_apply at
// any file for root to write the next time an apply was accepted. The design
// says the network-facing process has no path to the kernel; that was one. Now
// data_dir is root:easywall 0750 and this subdirectory easywall:easywall 0700.
// docs-tech/threat-model.md, "data_dir is root's".
const stateSubdir = "web"

// stateFiles is every name this process has kept directly in data_dir: the
// four Config paths, and the copy passkeyStore sets aside when the store will
// not parse. TestEveryWebStatePathIsInTheStateDir holds the list to the paths.
var stateFiles = []string{
	"totp_replay.json",
	"passkeys.json",
	"passkeys.json.corrupt",
	"version_cache.json",
	"telemetry.json",
}

// prepareStateDir creates <data_dir>/web and brings in any state file still
// lying in data_dir from before 2.22.
//
// The package and the image move them as root before this process starts.
// This covers everything else — a manual install, the demo, a data_dir of the
// operator's own — in whichever order the upgrade happens. It matters because
// passkeys.json left behind reads as no passkeys at all, and for an account
// whose only second factor is a passkey that is the password alone.
//
// Copied, then removed, never renamed: once data_dir is 0750 this process can
// no longer unlink in it, so a rename fails exactly where the copy still
// works. An original that cannot be removed is harmless — nothing reads those
// names any more — and is said once. A link is never followed: links are what
// the old layout let be planted.
func prepareStateDir(dataDir string) {
	if dataDir == "" {
		return
	}
	dir := filepath.Join(dataDir, stateSubdir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		slog.Warn("could not create the web state directory; passkeys, the TOTP replay "+
			"guard and the version cache cannot be kept", "path", dir, "error", err)
		return
	}
	for _, name := range stateFiles {
		old, cur := filepath.Join(dataDir, name), filepath.Join(dir, name)
		if info, err := os.Lstat(old); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if _, err := os.Lstat(cur); err == nil {
			continue
		}
		data, err := os.ReadFile(old) // #nosec G304 -- a fixed name in this process's own data_dir
		if err == nil {
			err = writeFileAtomic(cur, data, 0600)
		}
		if err != nil {
			slog.Warn("could not move a state file into the web state directory",
				"from", old, "to", cur, "error", err)
			continue
		}
		if err := os.Remove(old); err != nil {
			slog.Info("moved a state file into the web state directory; the original "+
				"could not be removed and nothing reads it any more", "path", old, "error", err)
		}
	}
}
