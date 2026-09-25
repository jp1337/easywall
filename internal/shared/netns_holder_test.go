package shared

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A namespace held by `unshare -n sleep` is not there when its pid is: the
// holder is forked in this namespace and switches only once unshare runs, so
// /proc/<pid>/ns/net answers first with ours. Measured 200 of 200 at the moment
// the harnesses called it ready; a veth moved into it by pid landed here, and
// `Cannot find device` failed or skipped a test on main twice (2026-09-16,
// 2026-09-25). internal/core/netns.go clones with CLONE_NEWNET, and so must a
// test that holds one.
func TestNoTestHoldsANamespaceWithUnshare(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	needle := `"unshare"` + `, "-n"` // split, or this file would match itself
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, needle) {
				t.Errorf("%s:%d holds a namespace with unshare -n — clone it with SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}", path, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
