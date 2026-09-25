package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A reader that stops early — grep -q, grep -m, head — closes the pipe while the
// command on its left may still be writing. That command then dies of SIGPIPE
// (exit 141), and under `set -o pipefail` the pipeline fails however well the
// match went. Whether it happens depends on how many writes the output takes,
// so it is a coin toss on a runner.
//
// It cost four red Build runs on main, from the 2.22 merge to 2.23.1, all with
// "/etc/easywall/easywall.toml was not seeded with [docker] enabled = true" —
// about a file that was correct. `compose exec … sed … | grep -Eq` delivered the
// [docker] section in two frames often enough; the real docker compose binary
// exits 141 on the second one. Capture the output first, then match it.
var earlyExitingReader = regexp.MustCompile(`\|\s*(head\b|grep\b[^|#]*\s(-[A-Za-z]*[qm][A-Za-z0-9]*|--quiet|--silent|--max-count)\b)`)

func TestNoWorkflowPipesIntoAReaderThatStopsEarly(t *testing.T) {
	dir := filepath.Join("..", "..", ".github", "workflows")
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows under %s: %v", dir, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if m := earlyExitingReader.FindString(line); m != "" {
				t.Errorf("%s:%d pipes into %q, which can kill the writer with SIGPIPE under pipefail — capture the output, then match it:\n  %s",
					filepath.Base(f), i+1, strings.TrimSpace(m), strings.TrimSpace(line))
			}
		}
	}
}

// The pattern above is the guard; a pattern that matches nothing guards
// nothing.
func TestTheEarlyExitPatternCatchesTheShapesThatBit(t *testing.T) {
	for _, bad := range []string{
		`  | grep -Eq '^enabled[[:space:]]*=[[:space:]]*true' \`,
		`compose exec -T easywall id easywall | grep -q 'uid=100'`,
		`amd64) file /tmp/core | grep -q "x86-64" ;;`,
		`journalctl | grep -m1 started`,
		`docker logs x | grep --quiet ready`,
		`ls | head -1`,
	} {
		if !earlyExitingReader.MatchString(bad) {
			t.Errorf("not caught: %s", bad)
		}
	}
	for _, fine := range []string{
		`grep -q ready <<<"$out"`,
		`compose logs | grep 'setup token' | tail -1`,
		`ls | grep -v README`,
		`grep -Eq '^enabled' "$f"`,
	} {
		if earlyExitingReader.MatchString(fine) {
			t.Errorf("caught, but reads its whole input: %s", fine)
		}
	}
}
