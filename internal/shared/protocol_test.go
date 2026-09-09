package shared

import (
	"testing"
	"time"
)

// CmdGetHealth is two netlink reads and one file read — nothing that queues
// behind the nft mutex the way IMPORT_RULES and VALIDATE_CUSTOM do — so it
// belongs in CommandTimeout's default branch, not its long one. Nothing
// currently stops a future edit from moving it into the long branch beside
// PANIC and RESUME, and a five-second poll that silently became thirty-five
// would be invisible to every caller: this test is what turns that drift into
// a failure.
func TestCommandTimeoutKeepsGetHealthShort(t *testing.T) {
	if got, want := CommandTimeout(CmdGetHealth), 5*time.Second; got != want {
		t.Errorf("CommandTimeout(CmdGetHealth) = %s, want %s", got, want)
	}
}
