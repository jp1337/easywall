package core

import (
	"strings"
	"testing"
)

// published_ports = "filtered" with docker.enabled = false is a contradiction,
// not a preference, and the daemon says so with both keys named.
//
// Nothing detects a container network with coexistence switched off, so the
// forward chain would be asked to filter traffic to addresses it has been told
// not to look for. What that rendered was worse than nothing: the accepts
// without the deny that gives them meaning, opening in the forward chain the
// ports 2.18 kept shut, with the interface reporting them enforced.
//
// configuration.md's rule is that a value which cannot be interpreted stops the
// daemon with the key named — the same register a custom_networks entry that is
// not a CIDR is refused in.
func TestFilteredPublishedPortsNeedsDockerEnabled(t *testing.T) {
	cfg, err := LoadConfig(writeCoreConfig(t,
		"[docker]\nenabled = false\npublished_ports = \"filtered\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted published_ports = \"filtered\" on a host with Docker " +
			"coexistence off, where it can only render accepts with no deny behind them")
	}
	for _, key := range []string{"docker.published_ports", "docker.enabled"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("the message does not name %s: %v", key, err)
		}
	}
}

// And the two legitimate arrangements still start. The second is the one that
// matters: "enabled, but no bridge detected yet" is a transient a container
// host passes through on every boot, it is settled at apply rather than at
// load, and refusing it here would stop the daemon on a host whose containers
// simply have not started.
func TestPublishedPortsAcceptsTheArrangementsThatCanWork(t *testing.T) {
	for _, tc := range []struct{ name, section string }{
		{"filtered with docker on", "[docker]\nenabled = true\npublished_ports = \"filtered\"\n"},
		{"open with docker off", "[docker]\nenabled = false\npublished_ports = \"open\"\n"},
		{"absent", "[docker]\nenabled = false\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(writeCoreConfig(t, tc.section))
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate stopped the daemon on a working configuration: %v", err)
			}
		})
	}
}
