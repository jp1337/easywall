package shared

import "testing"

func port(p, desc string) PortRule { return PortRule{Port: p, Description: desc} }

func TestDiffRules_PortsAreKeyedByPort(t *testing.T) {
	cur := Rules{TCP: []PortRule{port("22", "SSH"), port("80", "HTTP")}}
	// Reordered, one added, one removed, one description edited.
	staged := Rules{TCP: []PortRule{port("80", "HTTP/1.1"), port("8443", "Nextcloud")}}

	got := DiffRules(cur, staged)
	want := []RuleDelta{
		{Set: "tcp", Kind: DeltaChanged, Key: "80", Label: "HTTP/1.1", From: "HTTP", To: "HTTP/1.1"},
		{Set: "tcp", Kind: DeltaAdded, Key: "8443", Label: "Nextcloud"},
		{Set: "tcp", Kind: DeltaRemoved, Key: "22", Label: "SSH"},
	}
	assertDeltas(t, got, want)
}

func TestDiffRules_ReorderingPortsIsNotAChange(t *testing.T) {
	cur := Rules{UDP: []PortRule{port("53", "DNS"), port("123", "NTP")}}
	staged := Rules{UDP: []PortRule{port("123", "NTP"), port("53", "DNS")}}
	if got := DiffRules(cur, staged); len(got) != 0 {
		t.Errorf("order does not change what is accepted, got %v", got)
	}
}

func TestDiffRules_ListsSkipCommentsAndBlanks(t *testing.T) {
	cur := Rules{Blacklist: []string{"# scanners", "192.0.2.42", "", "192.0.2.118"}}
	staged := Rules{Blacklist: []string{"192.0.2.118", "# a different note", "203.0.113.9"}}

	got := DiffRules(cur, staged)
	want := []RuleDelta{
		{Set: "blacklist", Kind: DeltaAdded, Key: "203.0.113.9"},
		{Set: "blacklist", Kind: DeltaRemoved, Key: "192.0.2.42"},
	}
	assertDeltas(t, got, want)
}

func TestDiffRules_ForwardingIsKeyedByProtocolAndSourcePort(t *testing.T) {
	cur := Rules{Forwarding: []ForwardingRule{
		{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
		{Protocol: "udp", SourcePort: 51821, DestPort: 51820},
	}}
	staged := Rules{Forwarding: []ForwardingRule{
		{Protocol: "tcp", SourcePort: 8080, DestPort: 8081},
	}}

	got := DiffRules(cur, staged)
	want := []RuleDelta{
		{Set: "forwarding", Kind: DeltaChanged, Key: "8080/tcp",
			From: "8080->80/tcp", To: "8080->8081/tcp"},
		{Set: "forwarding", Kind: DeltaRemoved, Key: "51821->51820/udp"},
	}
	assertDeltas(t, got, want)
}

func TestDiffRules_AMovedCustomRuleIsAChange(t *testing.T) {
	cur := Rules{Custom: []string{"# note", "tcp dport 9100 accept", "udp dport 53 accept"}}
	staged := Rules{Custom: []string{"udp dport 53 accept", "tcp dport 9100 accept"}}

	got := DiffRules(cur, staged)
	want := []RuleDelta{
		{Set: "custom", Kind: DeltaChanged, Key: "#1",
			From: "tcp dport 9100 accept", To: "udp dport 53 accept"},
		{Set: "custom", Kind: DeltaChanged, Key: "#2",
			From: "udp dport 53 accept", To: "tcp dport 9100 accept"},
	}
	assertDeltas(t, got, want)
}

func TestDiffConfig_NamesTheTomlKeyAndBothValues(t *testing.T) {
	applied := AppliedConfig{
		Firewall: FirewallOptions{Fragments: false, ConnectionLimitMax: 100},
		Network:  NetworkSettings{IPv6: IPv6Config{Mode: IPv6Filter}},
	}
	live := AppliedConfig{
		Firewall: FirewallOptions{Fragments: true, ConnectionLimitMax: 250},
		Network:  NetworkSettings{IPv6: IPv6Config{Mode: IPv6Block}},
	}

	got := DiffConfig(applied, live)
	want := []ConfigDelta{
		{Key: "drop_fragments", From: "off", To: "on"},
		{Key: "connection_limit_max", From: "100", To: "250"},
		{Key: "ipv6.mode", From: "filter", To: "block"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d deltas, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("delta %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDiffConfig_NoDriftIsNoDeltas(t *testing.T) {
	c := AppliedConfig{Firewall: FirewallOptions{SYNFlood: true, SYNFloodLimit: 100}}
	if got := DiffConfig(c, c); len(got) != 0 {
		t.Errorf("identical configs produced %v", got)
	}
}

// assertDeltas compares in order: the page lists what arrived before what left,
// and a preview whose order shifts between two loads is one nobody trusts.
func assertDeltas(t *testing.T, got, want []RuleDelta) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d deltas, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("delta %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
