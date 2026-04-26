package core

import (
	"testing"
)

func TestIsDockerInterface(t *testing.T) {
	docker := []string{"docker0", "docker_gwbridge", "br-abc123def456"}
	for _, name := range docker {
		if !isDockerInterface(name) {
			t.Errorf("expected %q to be a Docker interface", name)
		}
	}

	notDocker := []string{"eth0", "lo", "wlan0", "enp3s0", "veth1234", "ens3"}
	for _, name := range notDocker {
		if isDockerInterface(name) {
			t.Errorf("expected %q NOT to be a Docker interface", name)
		}
	}
}

func TestIsDockerInterface_EdgeCases(t *testing.T) {
	if isDockerInterface("") {
		t.Error("empty string should not be a Docker interface")
	}
	// "br" without dash is not a docker bridge
	if isDockerInterface("br") {
		t.Error("bare 'br' without dash should not match")
	}
	// "docker" prefix matches
	if !isDockerInterface("docker1") {
		t.Error("docker1 should match docker prefix")
	}
}

func TestReadProcNetDev(t *testing.T) {
	// /proc/net/dev is available on Linux; this is a smoke test.
	names := readProcNetDev()
	// On Linux this should return at least "lo"
	// On non-Linux (CI), it may return nil — just ensure no panic.
	_ = names
}

func TestDetectDockerBridges_NoDocker(t *testing.T) {
	// If Docker is not running, should return nil or empty slice — no panic.
	cidrs := detectDockerBridges()
	_ = cidrs
}

func TestIsDockerRunning(t *testing.T) {
	// Result depends on test environment — just ensure no panic and returns bool.
	running := isDockerRunning()
	_ = running
}
