//go:build integration

package core

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

// TestMain re-executes the test binary inside a fresh, isolated network
// namespace so that all nftables operations are completely isolated from
// the host firewall. The technique is identical to the one used by the
// github.com/google/nftables library itself.
//
// When run without CAP_NET_ADMIN / root the child process will fail to
// create the namespace and the tests are skipped with a clear message.
func TestMain(m *testing.M) {
	// The Harness peer, and it has to be checked first. Under "go test"
	// /proc/self/exe is this test binary rather than easywall-core, so the hook
	// in cmd/easywall-core/main.go never runs for a peer the harness starts
	// here — and the child inherits EASYWALL_IN_NETNS from its parent, so
	// without this branch the peer would fall into the case below and run the
	// entire integration suite a second time inside its own namespace.
	if os.Getenv(PeerEnvVar) != "" {
		os.Exit(RunPeer(os.Stdin, os.Stdout))
	}

	if os.Getenv("EASYWALL_IN_NETNS") != "" {
		// We are already inside the isolated namespace — run all tests.
		os.Exit(m.Run())
	}

	// Re-exec this binary in a new network namespace.
	cmd := exec.Command("/proc/self/exe", os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "EASYWALL_IN_NETNS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNET,
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		// CLONE_NEWNET requires CAP_SYS_ADMIN / root.
		os.Exit(1)
	}
}
