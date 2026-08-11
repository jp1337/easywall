//go:build integration

package core

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The three "drop by address type" options must match the address type they name.
//
// Broadcast was written as pkttype 0x03, which is PACKET_OTHERHOST, under a
// comment claiming it was PACKET_BROADCAST. So the option dropped traffic
// addressed to a different host — which an interface does not receive at all
// unless it is in promiscuous mode — and let every broadcast through. Read back
// from the kernel before the fix:
//
//	meta pkttype other drop
//
// nft is the oracle rather than a constant repeated in the test, because a
// constant repeated in the test is how the original passed: the existing check
// (TestIntegration_Apply_BroadcastDrop_AddsRule) asserted the rule *count*, and a
// count cannot see what a rule matches. Asked of nft on rules it built itself,
// `nft --debug=netlink` gives broadcast 0x00000001, multicast 0x00000002 and
// other 0x00000003.
func TestIntegration_AddressTypeDropsMatchWhatTheyName(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts shared.FirewallOptions
		want string
	}{
		{"drop_broadcast", shared.FirewallOptions{DropBroadcast: true}, "meta pkttype broadcast drop"},
		{"drop_multicast", shared.FirewallOptions{DropMulticast: true}, "meta pkttype multicast drop"},
		{"drop_anycast", shared.FirewallOptions{DropAnycast: true}, "fib daddr type anycast drop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			applyEmpty(t, m, tc.opts)

			out, err := exec.Command("nft", "list", "chain", "inet", tableName, "input").CombinedOutput()
			if err != nil {
				t.Fatalf("nft list chain: %v: %s", err, out)
			}
			chain := string(out)
			if !strings.Contains(chain, tc.want) {
				t.Errorf("%s did not produce %q.\nThe rule the kernel actually holds:\n%s",
					tc.name, tc.want, addressTypeRules(chain))
			}
		})
	}
}

// addressTypeRules pulls out just the lines that match on an address type, so a
// failure names the rule that is wrong instead of printing the whole chain.
func addressTypeRules(chain string) string {
	re := regexp.MustCompile(`(?m)^.*(pkttype|fib daddr type).*$`)
	found := re.FindAllString(chain, -1)
	if len(found) == 0 {
		return "  (no address-type rule at all)"
	}
	return strings.Join(found, "\n")
}
