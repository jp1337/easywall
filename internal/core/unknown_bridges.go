package core

import (
	"slices"

	"github.com/jp1337/easywall/internal/shared"
)

// unknownBridges returns every network in now that baked does not hold,
// grouped by bridge in detection order. Under ipv6.mode = "block" an IPv6
// network is left out: the forward chain keeps it out on purpose (spec D3),
// so an apply would not add it and naming it would ask for one that changes
// nothing. A network in baked that is gone from now is not reported: rules
// naming a vanished network cost nothing.
func unknownBridges(now []bridgeNet, baked []string, mode shared.IPv6Mode) []shared.UnknownBridge {
	var out []shared.UnknownBridge
	for _, n := range now {
		if slices.Contains(baked, n.CIDR) {
			continue
		}
		if mode == shared.IPv6Block {
			if _, v6 := cidrFamilies([]string{n.CIDR}); v6 {
				continue
			}
		}
		if i := len(out) - 1; i >= 0 && out[i].Interface == n.Iface {
			out[i].CIDRs = append(out[i].CIDRs, n.CIDR)
			continue
		}
		out = append(out, shared.UnknownBridge{Interface: n.Iface, CIDRs: []string{n.CIDR}})
	}
	return out
}

// unknownBridgesWhen is unknownBridges behind the three conditions under
// which the question means anything: rules are enforced, Docker coexistence
// is on, and bridge detection is on. Split out so the conditions are tested
// without a Firewall.
func unknownBridgesWhen(enforcing, dockerOn, detectOn bool, now []bridgeNet, baked []string, mode shared.IPv6Mode) []shared.UnknownBridge {
	if !enforcing || !dockerOn || !detectOn {
		return nil
	}
	return unknownBridges(now, baked, mode)
}

// unknownBridges is the status field: what exists now and is not in force.
// Panic mode is not enforcing, so it reports nothing either.
func (f *Firewall) unknownBridges(enforcing bool) []shared.UnknownBridge {
	n := f.cfg.NetworkSettings()
	return unknownBridgesWhen(enforcing && !f.PanicEngaged(), n.Docker.Enabled, n.Docker.AllowBridgeNetworks,
		detectDockerBridgeNets(), f.nft.BakedBridges(), n.IPv6.Mode)
}
