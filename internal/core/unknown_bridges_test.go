package core

import (
	"slices"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Review Focus 4: a network the last apply did not bake in is named, grouped
// by its bridge, in the order detection found it.
func TestUnknownBridgesNamesANetworkTheRulesDoNotKnow(t *testing.T) {
	now := []bridgeNet{
		{"docker0", "172.17.0.0/16"},
		{"br-4f2a", "172.20.0.0/24"},
		{"br-4f2a", "fd00:ea5e:4f2a::/64"},
	}
	got := unknownBridges(now, []string{"172.17.0.0/16", "172.20.0.0/24"}, shared.IPv6Filter)
	want := []shared.UnknownBridge{{Interface: "br-4f2a", CIDRs: []string{"fd00:ea5e:4f2a::/64"}}}
	if len(got) != 1 || got[0].Interface != want[0].Interface || !slices.Equal(got[0].CIDRs, want[0].CIDRs) {
		t.Errorf("unknownBridges = %+v, want %+v", got, want)
	}
}

func TestUnknownBridgesIsEmptyWhenEverythingIsKnown(t *testing.T) {
	now := []bridgeNet{{"docker0", "172.17.0.0/16"}}
	if got := unknownBridges(now, []string{"172.17.0.0/16", "172.30.0.0/16"}, shared.IPv6Filter); len(got) != 0 {
		t.Errorf("a vanished network or a known one was reported: %+v", got)
	}
}

// D3 and D5 together: under block an IPv6 network is kept out on purpose, so
// it is not "unknown" — an apply would not add it.
func TestUnknownBridgesIgnoresIPv6UnderBlock(t *testing.T) {
	now := []bridgeNet{{"br-mail", "172.20.0.0/24"}, {"br-mail", "fd00:ea5e::/64"}}
	if got := unknownBridges(now, []string{"172.20.0.0/24"}, shared.IPv6Block); len(got) != 0 {
		t.Errorf("under block an IPv6 network was reported unknown: %+v", got)
	}
}

// Review Focus 5: before anything is enforced there is nothing to be missing
// from; the line would greet every fresh install.
func TestUnknownBridgesSaysNothingWhileNotEnforcing(t *testing.T) {
	if got := unknownBridgesWhen(false, true, true, []bridgeNet{{"docker0", "172.17.0.0/16"}}, nil, shared.IPv6Filter); got != nil {
		t.Errorf("reported unknown bridges while nothing is enforced: %+v", got)
	}
	if got := unknownBridgesWhen(true, false, true, []bridgeNet{{"docker0", "172.17.0.0/16"}}, nil, shared.IPv6Filter); got != nil {
		t.Errorf("reported unknown bridges with Docker coexistence off: %+v", got)
	}
	if got := unknownBridgesWhen(true, true, false, []bridgeNet{{"docker0", "172.17.0.0/16"}}, nil, shared.IPv6Filter); got != nil {
		t.Errorf("reported unknown bridges with bridge detection off: %+v", got)
	}
}
