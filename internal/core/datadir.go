package core

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"
)

// dataDirIsShared reports whether data_dir lets anyone but its owner write in
// it, and says so in the log when it does.
//
// It did until 2.22: root:easywall 0770, shared with the web user. Anyone who
// can write a directory can replace the files in it and plant links where they
// were, so the network-facing process could swap rules.json — which the core
// restores at boot with no acceptance window — and point last_apply at any file
// root then wrote. The package and the image now set 0750 at every upgrade and
// every start and keep the web's state in data_dir/web. A manual install from
// before 2.22 keeps whatever mode it was given, and this line is the only thing
// that will tell it. A warning, not a refusal: a core that will not start is a
// machine with no firewall, which is worse than the finding.
//
// A data_dir the core does not own is shared too, whatever its mode: the owner
// of a directory can chmod it and then replace anything in it.
func dataDirIsShared(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid != uint32(os.Geteuid()) { // #nosec G115 -- a uid, never negative
		slog.Error("data_dir is not owned by the user easywall-core runs as, so its owner "+
			"can replace rules.json, which is restored at boot: chown root "+dir,
			"path", dir, "owner", st.Uid)
		return true
	}
	if info.Mode().Perm()&0o022 == 0 {
		return false
	}
	slog.Error("data_dir is writable by more than its owner, so whoever else can write it "+
		"can replace rules.json, which is restored at boot. The web process keeps its state "+
		"in data_dir/web since 2.22 and needs no write here: chmod 0750 "+dir,
		"path", dir, "mode", fmt.Sprintf("%04o", info.Mode().Perm()))
	return true
}
