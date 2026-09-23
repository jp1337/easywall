package core

import (
	"os"
	"path/filepath"

	"github.com/jp1337/easywall/internal/shared"
)

// logDirLooksLost reports the one combination that means the log directory
// did not survive while the data directory did: rules are configured, the
// audit log is missing or empty, and nothing has rotated it away.
//
// The case it exists for: a container recreated without /var/log/easywall
// mounted. The image's VOLUME directive gives the new container an empty
// anonymous volume, the daemon starts normally, and the audit page simply shows
// a short history — the worst way for a security record to go. Detectable
// without knowing anything about Docker.
func logDirLooksLost(auditPath string, rules shared.Rules) bool {
	if rules.IsEmpty() {
		return false // nothing was ever applied, so there was nothing to log
	}
	if info, err := os.Stat(auditPath); err == nil && info.Size() > 0 {
		return false
	}
	// logrotate's `create` leaves an empty audit.log beside audit.log.1 or a
	// compressed older one; that is a rotation, not a loss.
	rotated, _ := filepath.Glob(auditPath + ".*")
	return len(rotated) == 0
}
