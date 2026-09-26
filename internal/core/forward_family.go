package core

import (
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// cidrFamilies reports which address families the container networks cover.
// A comment, a blank line or an entry that does not parse covers neither —
// cidrMatch renders nothing for them either. An IPv4-mapped IPv6 network is
// IPv4: shared.ParseNetwork unmaps it, and cidrMatch writes it as `ip saddr`.
func cidrFamilies(cidrs []string) (v4, v6 bool) {
	for _, c := range cidrs {
		if shared.IsListComment(c) {
			continue
		}
		p, err := shared.ParseNetwork(c)
		if err != nil {
			continue
		}
		if p.Addr().Is4() {
			v4 = true
		} else {
			v6 = true
		}
	}
	return v4, v6
}

// containerNetworks assembles the container networks every consumer in Apply
// reads — the input chain's bridge accepts, the bogon exemptions, the forward
// chain, the published-port check and the record of what is in force: the
// detected bridges when bridge detection is on, then docker.custom_networks,
// with IPv6 removed under ipv6.mode = "block" (spec D3). Nothing when Docker
// coexistence is off. One owner, so D3 cannot hold for one consumer and not
// another (plan review #4).
func containerNetworks(docker shared.DockerConfig, detected []string, mode shared.IPv6Mode) []string {
	if !docker.Enabled {
		return nil
	}
	var cidrs []string
	if docker.AllowBridgeNetworks {
		cidrs = append(cidrs, detected...)
	}
	cidrs = append(cidrs, docker.CustomNetworks...)
	if mode == shared.IPv6Block {
		cidrs = withoutIPv6(cidrs)
	}
	return cidrs
}

// withoutIPv6 is cidrs with every IPv6 network removed — what ipv6.mode =
// "block" leaves of the container networks (spec D3). Comments and entries
// that do not parse are kept: they render nothing either way, and dropping
// them would make this function decide something it was not asked.
func withoutIPv6(cidrs []string) []string {
	var out []string
	for _, c := range cidrs {
		if !shared.IsListComment(c) {
			if p, err := shared.ParseNetwork(c); err == nil && !p.Addr().Is4() {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// ruleFamily reports the family a rule's leading `meta nfproto` test names.
func ruleFamily(exprs []expr.Any) (byte, bool) {
	if len(exprs) < 2 {
		return 0, false
	}
	meta, ok := exprs[0].(*expr.Meta)
	if !ok || meta.Key != expr.MetaKeyNFPROTO {
		return 0, false
	}
	cmp, ok := exprs[1].(*expr.Cmp)
	if !ok || len(cmp.Data) != 1 {
		return 0, false
	}
	return cmp.Data[0], true
}

// forwardFamilyPin pins a forwarded port accept to one address family unless
// it already names one.
//
// portAcceptRules builds a rule with no sources as `meta l4proto tcp dport N
// accept` and no address test at all, which in an inet table is both families.
// In the input chain that is right: ipv6.Mode has already had its say above it.
// In the forward chain it is not: the deny this accept is paired with exists
// only for the families that have a container network, so an unpinned accept
// opens the port for forwarded traffic of a family with no deny — to anything
// the host routes. addForwardPortRules therefore emits one pinned copy per
// family that has a container network (spec D1).
//
// A rule that already carries a family test got it from cidrMatch over a source
// the operator named, and keeps it. Naming an address is naming a family.
func forwardFamilyPin(exprs []expr.Any, family byte) []expr.Any {
	if _, named := ruleFamily(exprs); named {
		return exprs
	}
	return append([]expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{family}},
	}, exprs...)
}

// the two family bytes, named once for the callers above.
const (
	familyIPv4 = byte(unix.NFPROTO_IPV4)
	familyIPv6 = byte(unix.NFPROTO_IPV6)
)
