package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestDescribeRuleChange_NamesTheAddresses(t *testing.T) {
	before := shared.Rules{Blacklist: []string{"192.0.2.1", "192.0.2.2"}}
	after := shared.Rules{Blacklist: []string{"192.0.2.2", "203.0.113.7"}}

	got := describeRuleChange("blacklist", before, after)
	for _, want := range []string{"added 203.0.113.7", "removed 192.0.2.1"} {
		if !strings.Contains(got, want) {
			t.Errorf("detail %q is missing %q", got, want)
		}
	}
	if strings.Contains(got, "192.0.2.2") {
		t.Errorf("detail %q mentions an entry that did not change", got)
	}
}

func TestDescribeRuleChange_CountsStructuredRules(t *testing.T) {
	before := shared.Rules{TCP: []shared.PortRule{{Port: "22"}}}
	after := shared.Rules{TCP: []shared.PortRule{{Port: "22"}, {Port: "443"}}}

	if got := describeRuleChange("tcp", before, after); !strings.Contains(got, "1 port added") {
		t.Errorf("expected an added-port count, got %q", got)
	}
}

func TestDescribeRuleChange_EditInPlaceIsNotSilent(t *testing.T) {
	before := shared.Rules{TCP: []shared.PortRule{{Port: "22", Description: "ssh"}}}
	after := shared.Rules{TCP: []shared.PortRule{{Port: "22", Description: "SSH access"}}}

	got := describeRuleChange("tcp", before, after)
	if got == "" || strings.Contains(got, "no change") {
		t.Errorf("a same-size edit still changed something; detail was %q", got)
	}
}

func TestDescribeStructChange_NamesTheSwitchThatMoved(t *testing.T) {
	before := shared.FirewallOptions{SYNFlood: true, PortScan: true}
	after := shared.FirewallOptions{SYNFlood: true, PortScan: false, TCPRSTFlood: true}

	got := describeStructChange(before, after)
	for _, want := range []string{"port_scan", "tcp_rst_flood"} {
		if !strings.Contains(got, want) {
			t.Errorf("detail %q is missing %q", got, want)
		}
	}
	if strings.Contains(got, "syn_flood") {
		t.Errorf("detail %q names a switch that did not move", got)
	}
}

func TestDescribeStructChange_RecursesIntoNestedSettings(t *testing.T) {
	before := shared.NetworkSettings{Docker: shared.DockerConfig{Enabled: false}}
	after := shared.NetworkSettings{Docker: shared.DockerConfig{Enabled: true}}

	if got := describeStructChange(before, after); !strings.Contains(got, "docker.enabled") {
		t.Errorf("expected the nested field path, got %q", got)
	}
}

func TestDescribeStructChange_SaysSoWhenNothingMoved(t *testing.T) {
	opts := shared.FirewallOptions{SYNFlood: true}
	if got := describeStructChange(opts, opts); got != "no change" {
		t.Errorf("expected %q, got %q", "no change", got)
	}
}

// An audit line that runs to a kilobyte does not get read.
func TestJoinCapped_BoundsTheLine(t *testing.T) {
	many := make([]string, 20)
	for i := range many {
		many[i] = "203.0.113." + string(rune('a'+i))
	}
	got := joinCapped(many)
	if !strings.Contains(got, "and 14 more") {
		t.Errorf("expected the tail to be summarised, got %q", got)
	}
	if strings.Count(got, ",") > maxDetailItems {
		t.Errorf("listed more than %d items: %q", maxDetailItems, got)
	}
}
