package shared

import (
	"testing"
)

func TestValidateIPOrCIDR(t *testing.T) {
	valid := []string{"192.168.1.1", "10.0.0.0/8", "::1", "2001:db8::/32"}
	for _, s := range valid {
		if err := validateIPOrCIDR(s); err != nil {
			t.Errorf("expected %q to be valid: %v", s, err)
		}
	}
	invalid := []string{"", "notanip", "256.1.2.3", "10.0.0.0/33"}
	for _, s := range invalid {
		if err := validateIPOrCIDR(s); err == nil {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestValidatePortRule(t *testing.T) {
	valid := []PortRule{
		{Port: "1"}, {Port: "65535"}, {Port: "80"}, {Port: "8000:9000"},
	}
	for _, r := range valid {
		if err := validatePortRule(r); err != nil {
			t.Errorf("expected port %q to be valid: %v", r.Port, err)
		}
	}
	invalid := []PortRule{
		{Port: ""},
		{Port: "0"},
		{Port: "65536"},
		{Port: "abc"},
		{Port: "9000:8000"}, // end < start
		{Port: "0:100"},
	}
	for _, r := range invalid {
		if err := validatePortRule(r); err == nil {
			t.Errorf("expected port %q to be invalid", r.Port)
		}
	}
}

func TestValidateRules_InvalidForwardingProtocol(t *testing.T) {
	r := Rules{
		TCP: []PortRule{}, UDP: []PortRule{},
		Blacklist: []string{}, Whitelist: []string{}, Custom: []string{},
		Forwarding: []ForwardingRule{{Protocol: "icmp", SourcePort: 80, DestPort: 8080}},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for invalid forwarding protocol")
	}
}

func TestValidateRules_InvalidForwardingPort(t *testing.T) {
	r := Rules{
		TCP: []PortRule{}, UDP: []PortRule{},
		Blacklist: []string{}, Whitelist: []string{}, Custom: []string{},
		Forwarding: []ForwardingRule{{Protocol: "tcp", SourcePort: 0, DestPort: 80}},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for source port 0")
	}
}

func TestValidateRules_InvalidDestPort(t *testing.T) {
	r := Rules{
		TCP: []PortRule{}, UDP: []PortRule{},
		Blacklist: []string{}, Whitelist: []string{}, Custom: []string{},
		Forwarding: []ForwardingRule{{Protocol: "tcp", SourcePort: 8080, DestPort: 0}},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for dest port 0")
	}
}

func TestValidateRules_InvalidTCPPort(t *testing.T) {
	r := Rules{
		TCP:        []PortRule{{Port: "99999"}},
		UDP:        []PortRule{},
		Blacklist:  []string{},
		Whitelist:  []string{},
		Custom:     []string{},
		Forwarding: []ForwardingRule{},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for invalid TCP port")
	}
}

func TestValidateRules_InvalidUDPPort(t *testing.T) {
	r := Rules{
		TCP:        []PortRule{},
		UDP:        []PortRule{{Port: "0"}},
		Blacklist:  []string{},
		Whitelist:  []string{},
		Custom:     []string{},
		Forwarding: []ForwardingRule{},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for invalid UDP port 0")
	}
}

func TestValidateRules_InvalidBlacklistIP(t *testing.T) {
	r := Rules{
		TCP:        []PortRule{},
		UDP:        []PortRule{},
		Blacklist:  []string{"not-an-ip"},
		Whitelist:  []string{},
		Custom:     []string{},
		Forwarding: []ForwardingRule{},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for invalid blacklist IP")
	}
}

func TestValidateRules_InvalidWhitelistCIDR(t *testing.T) {
	r := Rules{
		TCP:        []PortRule{},
		UDP:        []PortRule{},
		Blacklist:  []string{},
		Whitelist:  []string{"300.300.300.300"},
		Custom:     []string{},
		Forwarding: []ForwardingRule{},
	}
	if err := ValidateRules(r); err == nil {
		t.Error("expected error for invalid whitelist IP")
	}
}

func TestValidatePortRule_RejectsTrailingGarbage(t *testing.T) {
	bad := []string{
		"80abc", // read as 80
		"80 90", // someone meaning two ports opens one
		" 80",   // leading space
		"80:",   // half a range
		":90",   // the other half
		"80:90:100",
		"",
		"http",
		"-1",
		"0",
		"65536",
		"8000:7000", // ends before it starts
	}
	for _, port := range bad {
		t.Run(port, func(t *testing.T) {
			if err := validatePortRule(PortRule{Port: port}); err == nil {
				t.Errorf("accepted %q", port)
			}
		})
	}
}

func TestValidatePortRule_AcceptsWhatItShould(t *testing.T) {
	for _, port := range []string{"1", "22", "8080", "65535", "8000:9000", "1:65535"} {
		t.Run(port, func(t *testing.T) {
			if err := validatePortRule(PortRule{Port: port}); err != nil {
				t.Errorf("rejected %q: %v", port, err)
			}
		})
	}
}
