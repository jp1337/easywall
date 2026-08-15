package shared

import "testing"

// The two network lists on the Network page — docker.custom_networks and
// routing.networks — are typed into a textarea, so they arrive with whatever an
// operator puts between the entries. The core used to run net.ParseCIDR over
// every element, blanks and comments included, while the editor validating that
// same textarea skipped both. A blank separator line was therefore accepted by
// the page and refused by the core, and the operator was shown "Failed to save
// changes. Check core connection." with a core that was answering perfectly.
//
// Measured before the fix, through the real socket:
//
//	POST /settings (routing_networks = "10.8.0.0/24\n\n10.9.0.0/24")
//	  web:  save settings error error="core error: routing network \"\": not a CIDR network"
//	  page: Failed to save changes. Check core connection.
//	  file: [routing] mode = "closed"   ← unchanged
func TestNetworkListsKeepCommentsAndBlanksTheEditorAllows(t *testing.T) {
	entries := []string{
		"172.17.0.0/16",
		"",                 // the blank line between two groups
		"# the office VPN", // the comment every other list editor documents
		"  ",               // whitespace only
		"  10.8.0.0/24  ",  // padded, as a paste leaves it
		"2001:db8::/32",
	}
	if err := ValidateNetworkList("docker custom network", entries); err != nil {
		t.Errorf("a list the editor accepts was refused: %v", err)
	}
}

// The other half: what the editor must refuse, because the apply step turns it
// into no rule at all. A bare address is the one that catches people — it is a
// valid IP, so the blacklist's validator waves it through.
func TestNetworkListsRefuseWhatWouldProduceNoRule(t *testing.T) {
	for _, entry := range []string{
		"192.168.1.5", // an address, not a network
		"10.9.0.0-24", // a hyphen where the slash belongs
		"10.0.0.0/33", // prefix out of range
		"not a network",
	} {
		if err := ValidateNetworkList("routing network", []string{entry}); err == nil {
			t.Errorf("%q was accepted; it produces no rule and would be listed as allowed anyway", entry)
		}
	}
}
