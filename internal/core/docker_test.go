package core

import (
	"fmt"
	"net"
	"os"
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

func TestIsDockerRunning_SocketExists(t *testing.T) {
	dir := t.TempDir()
	sockPath := dir + "/docker.sock"
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	old := dockerSocketPaths
	dockerSocketPaths = []string{sockPath}
	defer func() { dockerSocketPaths = old }()

	if !isDockerRunning() {
		t.Error("expected isDockerRunning=true when socket file exists")
	}
}

func TestIsDockerRunning_NoSocket(t *testing.T) {
	old := dockerSocketPaths
	dockerSocketPaths = []string{"/nonexistent/docker.sock"}
	defer func() { dockerSocketPaths = old }()

	if isDockerRunning() {
		t.Error("expected isDockerRunning=false when socket does not exist")
	}
}

func TestReadProcNetDev_NotFound(t *testing.T) {
	old := procNetDevPath
	procNetDevPath = "/nonexistent/proc/net/dev"
	defer func() { procNetDevPath = old }()

	names := readProcNetDev()
	if names != nil {
		t.Errorf("expected nil when file not found, got %v", names)
	}
}

func TestDetectDockerBridges_InterfaceError(t *testing.T) {
	old := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return nil, fmt.Errorf("mock error")
	}
	defer func() { netInterfacesFn = old }()

	cidrs := detectDockerBridges()
	if cidrs != nil {
		t.Errorf("expected nil on interfaces error, got %v", cidrs)
	}
}

func TestDetectDockerBridges_WithIPv4(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("172.17.0.1/16")
	fakeIface := net.Interface{Name: "docker0", Index: 99}

	oldIfaces := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{fakeIface}, nil
	}
	defer func() { netInterfacesFn = oldIfaces }()

	oldAddrs := ifaceAddrsFn
	ifaceAddrsFn = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{ipnet}, nil
	}
	defer func() { ifaceAddrsFn = oldAddrs }()

	cidrs := detectDockerBridges()
	if len(cidrs) != 1 {
		t.Fatalf("expected 1 CIDR, got %d: %v", len(cidrs), cidrs)
	}
	if cidrs[0] != "172.17.0.0/16" {
		t.Errorf("unexpected CIDR: %s", cidrs[0])
	}
}

func TestDetectDockerBridges_IPv6Skipped(t *testing.T) {
	_, ipnet6, _ := net.ParseCIDR("fe80::1/64")
	fakeIface := net.Interface{Name: "br-abc123", Index: 99}

	oldIfaces := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{fakeIface}, nil
	}
	defer func() { netInterfacesFn = oldIfaces }()

	oldAddrs := ifaceAddrsFn
	ifaceAddrsFn = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{ipnet6}, nil
	}
	defer func() { ifaceAddrsFn = oldAddrs }()

	cidrs := detectDockerBridges()
	if len(cidrs) != 0 {
		t.Errorf("expected no CIDRs for IPv6-only interface, got %v", cidrs)
	}
}

func TestDetectDockerBridges_AddrsError(t *testing.T) {
	fakeIface := net.Interface{Name: "docker0", Index: 99}

	oldIfaces := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{fakeIface}, nil
	}
	defer func() { netInterfacesFn = oldIfaces }()

	oldAddrs := ifaceAddrsFn
	ifaceAddrsFn = func(_ net.Interface) ([]net.Addr, error) {
		return nil, fmt.Errorf("addrs error")
	}
	defer func() { ifaceAddrsFn = oldAddrs }()

	cidrs := detectDockerBridges()
	if len(cidrs) != 0 {
		t.Errorf("expected empty CIDRs on Addrs error, got %v", cidrs)
	}
}

func TestDetectDockerBridges_NonDockerSkipped(t *testing.T) {
	fakeIface := net.Interface{Name: "eth0", Index: 1}

	oldIfaces := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{fakeIface}, nil
	}
	defer func() { netInterfacesFn = oldIfaces }()

	called := false
	oldAddrs := ifaceAddrsFn
	ifaceAddrsFn = func(_ net.Interface) ([]net.Addr, error) {
		called = true
		return nil, nil
	}
	defer func() { ifaceAddrsFn = oldAddrs }()

	detectDockerBridges()
	if called {
		t.Error("ifaceAddrsFn should not be called for non-Docker interface")
	}
}

func TestDetectDockerBridges_NonIPNetAddr(t *testing.T) {
	fakeIface := net.Interface{Name: "docker0", Index: 99}

	oldIfaces := netInterfacesFn
	netInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{fakeIface}, nil
	}
	defer func() { netInterfacesFn = oldIfaces }()

	oldAddrs := ifaceAddrsFn
	ifaceAddrsFn = func(_ net.Interface) ([]net.Addr, error) {
		// Return a *net.TCPAddr (not *net.IPNet) — should be silently skipped
		addr, _ := net.ResolveTCPAddr("tcp", "172.17.0.1:80")
		return []net.Addr{addr}, nil
	}
	defer func() { ifaceAddrsFn = oldAddrs }()

	cidrs := detectDockerBridges()
	if len(cidrs) != 0 {
		t.Errorf("expected no CIDRs for non-IPNet addr, got %v", cidrs)
	}
}
