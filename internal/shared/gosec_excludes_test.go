package shared

import (
	"strings"
	"testing"
)

// TestG115IsNotGloballyExcluded keeps the two gosec runs telling one story.
//
// .golangci.yml's exclusions do not apply to the standalone gosec in
// security.yml, so a rule excluded here is a rule that only ever reaches a
// human through code scanning, after a push, on a check run named gosec. That
// divergence let 2.18 convert a pid to uint32 with no bound and pass Lint from
// the line being written through three reviews.
//
// Measured when the exclusion was removed: zero G115 findings in production
// code. All four sites the exclusion's comment defended carry their own
// #nosec with the bound beside it, which is the form that survives both runs.
func TestG115IsNotGloballyExcluded(t *testing.T) {
	cfg := repoFile(t, ".golangci.yml")
	for _, line := range strings.Split(cfg, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- G115") {
			t.Errorf("G115 is excluded in .golangci.yml (%q) — Lint then cannot see an "+
				"integer narrowing that standalone gosec in security.yml will report "+
				"after the push. Use a #nosec G115 comment with the bound beside the "+
				"line instead, the way nftables.go:2389 and totp.go:75 do", trimmed)
		}
	}
}
