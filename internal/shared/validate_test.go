package shared

import (
	"strings"
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

// A custom rule is one nft statement. nft reads a newline or a semicolon as the
// end of a command, so a rule carrying either is a second command — run by the
// root daemon, and able to reach tables easywall does not own.
//
// Reachable through import, where a JSON string can hold anything the textarea
// would have split apart. Demonstrated against a real kernel before this check
// existed: an imported rule with a newline in it wrote a drop into a
// neighbouring table, which is exactly what "easywall owns one table and never
// looks at another" says cannot happen.
func TestValidateRules_RejectsCommandSeparatorsInCustomRules(t *testing.T) {
	payloads := []string{
		"accept\nadd rule inet bystander c drop",
		"accept\r\nflush ruleset",
		"accept; add rule inet bystander c drop",
		"accept;",
		"tcp dport 22 accept\n}\ntable inet evil { chain c { type filter hook prerouting priority -300; accept } }",
	}
	for _, rule := range payloads {
		err := ValidateRules(Rules{Custom: []string{rule}})
		if err == nil {
			t.Errorf("a rule carrying a command separator must be refused: %q", rule)
		}
	}
}

// And the rules operators actually write must keep working, braces and all.
func TestValidateRules_AcceptsOrdinaryCustomRules(t *testing.T) {
	ordinary := []string{
		"ip saddr 192.0.2.50 tcp dport 9100 accept",
		"tcp dport { 80, 443 } accept",
		"udp dport 53 limit rate 50/second accept",
		"ct state { new, established } accept",
		`tcp dport 10000 log prefix "legacy-admin: " drop`,
		"iif eth0 ip protocol udp udp dport 1194 accept",
		"# a comment line",
		"",
	}
	if err := ValidateRules(Rules{Custom: ordinary}); err != nil {
		t.Errorf("ordinary custom rules must be accepted: %v", err)
	}
}

// The message has to name the rule, or an operator with forty of them has to
// find it by bisection.
func TestValidateRules_NamesTheOffendingCustomRule(t *testing.T) {
	err := ValidateRules(Rules{Custom: []string{
		"tcp dport 80 accept",
		"tcp dport 443 accept",
		"accept\nflush ruleset",
	}})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "custom rule 3") {
		t.Errorf("the error should name rule 3, got: %v", err)
	}
}

func TestValidateRules_PortSources(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		sources []string
		wantErr bool
	}{
		{name: "no sources is anywhere", port: "443", sources: nil},
		{name: "empty list is anywhere", port: "443", sources: []string{}},
		{name: "a bare address", port: "443", sources: []string{"192.168.1.10"}},
		{name: "a network", port: "443", sources: []string{"10.0.0.0/8"}},
		{name: "an IPv6 network", port: "443", sources: []string{"fc00::/7"}},
		{name: "several", port: "443", sources: []string{"10.0.0.0/8", "172.16.0.0/12"}},
		{name: "a comment and a blank are kept, not rules",
			port: "443", sources: []string{"# the LAN", "", "192.168.0.0/16"}},
		{name: "not an address", port: "443", sources: []string{"the-lan"}, wantErr: true},
		{name: "a hostname is not an address", port: "443", sources: []string{"nas.local"}, wantErr: true},
		{name: "a port is not an address", port: "443", sources: []string{"192.168.1.10:443"}, wantErr: true},
		{name: "a range port with a valid network source", port: "8000:9000", sources: []string{"10.0.0.0/8"}},
		{name: "a range port with an invalid source is rejected", port: "8000:9000", sources: []string{"not-an-address"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRules(Rules{TCP: []PortRule{{Port: tc.port, Sources: tc.sources}}})
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateRules accepted sources %v", tc.sources)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateRules rejected sources %v: %v", tc.sources, err)
			}
		})
	}
}
