package shared

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// An easywall.toml written before 2.19 has no published_ports key at all, and a
// container host that upgrades must not lose a published port at its next
// apply. That is 2.5.0's defect, and this is the guard against repeating it.
func TestDockerConfig_AnAbsentKeyLeavesPublishedPortsOpen(t *testing.T) {
	var c DockerConfig
	if _, err := toml.Decode("enabled = true\nallow_bridge_networks = true\n", &c); err != nil {
		t.Fatal(err)
	}
	if c.FiltersPublishedPorts() {
		t.Error("a config with no published_ports key filtered published ports")
	}
}

func TestDockerConfig_FilteredIsRead(t *testing.T) {
	var c DockerConfig
	if _, err := toml.Decode(`enabled = true`+"\n"+`published_ports = "filtered"`+"\n", &c); err != nil {
		t.Fatal(err)
	}
	if !c.FiltersPublishedPorts() {
		t.Error(`published_ports = "filtered" did not switch filtering on`)
	}
}

// Anything else is open, and says so once in the log rather than silently. A
// typo that quietly filters nothing is the same class of lie as a rule that
// reports itself enforced and is not.
func TestDockerConfig_AnUnknownValueIsOpen(t *testing.T) {
	var c DockerConfig
	//nolint:misspell // the misspelling is the test: a typo must read as "open".
	if _, err := toml.Decode(`published_ports = "filterd"`+"\n", &c); err != nil {
		t.Fatal(err)
	}
	if c.FiltersPublishedPorts() {
		t.Error("an unrecognised published_ports value switched filtering on")
	}
}

// The shipped sample documents the key, and the guard that every toml tag is
// documented lives in config_docs_test.go. This asserts the narrower thing that
// test cannot: that the sample ships the safe default rather than the new one.
func TestTheShippedSampleLeavesPublishedPortsOpen(t *testing.T) {
	sample := repoFile(t, "config", "easywall.toml")
	if !strings.Contains(sample, `published_ports       = "open"`) {
		t.Error("config/easywall.toml does not ship published_ports = \"open\"")
	}
}
