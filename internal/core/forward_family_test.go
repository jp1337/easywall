package core

import (
	"slices"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

func TestCidrFamilies(t *testing.T) {
	for _, tc := range []struct {
		cidrs  []string
		v4, v6 bool
	}{
		{nil, false, false},
		{[]string{"172.17.0.0/16"}, true, false},
		{[]string{"fd00:ea5e::/64"}, false, true},
		{[]string{"172.17.0.0/16", "fd00:ea5e::/64"}, true, true},
		{[]string{"# a comment", "", "fd00:ea5e::/64"}, false, true},
	} {
		v4, v6 := cidrFamilies(tc.cidrs)
		if v4 != tc.v4 || v6 != tc.v6 {
			t.Errorf("cidrFamilies(%v) = %v, %v; want %v, %v", tc.cidrs, v4, v6, tc.v4, tc.v6)
		}
	}
}

// Review Focus 3: an IPv4-mapped network is IPv4. shared.ParseNetwork unmaps
// it, and the forward chain renders it as `ip saddr`, so treating it as IPv6
// would strip an IPv4 network under block and invent an IPv6 container host.
func TestFamiliesOfAMappedNetworkIsIPv4(t *testing.T) {
	mapped := []string{"::ffff:10.0.0.0/104"}
	if v4, v6 := cidrFamilies(mapped); !v4 || v6 {
		t.Errorf("cidrFamilies(%v) = %v, %v; a mapped network is IPv4", mapped, v4, v6)
	}
	if got := withoutIPv6(mapped); !slices.Equal(got, mapped) {
		t.Errorf("withoutIPv6 stripped an IPv4-mapped network: %v", got)
	}
}

// D3 has one owner: Apply assembles the container networks once, and every
// consumer — input accepts, forward chain, published-port check, the record —
// reads that list.
func TestContainerNetworks(t *testing.T) {
	detected := []string{"172.17.0.0/16", "fd00:ea5e::/64"}
	docker := shared.DockerConfig{Enabled: true, AllowBridgeNetworks: true,
		CustomNetworks: []string{"10.20.0.0/16", "2001:db8:c0::/64"}}
	for _, tc := range []struct {
		name   string
		docker shared.DockerConfig
		mode   shared.IPv6Mode
		want   []string
	}{
		{"filter keeps both families", docker, shared.IPv6Filter,
			[]string{"172.17.0.0/16", "fd00:ea5e::/64", "10.20.0.0/16", "2001:db8:c0::/64"}},
		{"block strips IPv6, detected and custom", docker, shared.IPv6Block,
			[]string{"172.17.0.0/16", "10.20.0.0/16"}},
		{"no detection: custom only", shared.DockerConfig{Enabled: true, CustomNetworks: []string{"10.20.0.0/16"}},
			shared.IPv6Filter, []string{"10.20.0.0/16"}},
		{"coexistence off: nothing", shared.DockerConfig{CustomNetworks: []string{"10.20.0.0/16"}},
			shared.IPv6Filter, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := containerNetworks(tc.docker, detected, tc.mode); !slices.Equal(got, tc.want) {
				t.Errorf("containerNetworks = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWithoutIPv6(t *testing.T) {
	got := withoutIPv6([]string{"172.17.0.0/16", "fd00:ea5e::/64", "2001:db8::/32", "10.0.0.0/8"})
	if want := []string{"172.17.0.0/16", "10.0.0.0/8"}; !slices.Equal(got, want) {
		t.Errorf("withoutIPv6 = %v, want %v", got, want)
	}
}

func TestForwardFamilyPin(t *testing.T) {
	bare := []expr.Any{&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1}}
	for _, fam := range []byte{unix.NFPROTO_IPV4, unix.NFPROTO_IPV6} {
		pinned := forwardFamilyPin(bare, fam)
		got, ok := ruleFamily(pinned)
		if !ok || got != fam {
			t.Errorf("forwardFamilyPin(_, %d) reads back as %d, %v", fam, got, ok)
		}
		// A rule that already names its family already keeps it.
		if again := forwardFamilyPin(pinned, unix.NFPROTO_IPV4+unix.NFPROTO_IPV6-fam); len(again) != len(pinned) {
			t.Errorf("a rule that already names a family was pinned a second time")
		}
	}
	if _, ok := ruleFamily(bare); ok {
		t.Error("a rule with no family test reads as having one")
	}
}
