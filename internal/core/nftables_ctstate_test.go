package core

import (
	"testing"

	"github.com/google/nftables/binaryutil"
)

// The ct state mask has to be in the byte order the kernel compares against,
// and for a long time none of the three in nftables.go was.
//
// The expected bytes below are not derived from ctStateMask — they are what nft
// itself emits, read off its own code generator:
//
//	$ nft --debug=netlink -c -f - <<'NFT'
//	table inet t { chain c { type filter hook input priority filter; policy drop;
//	  ct state established,related accept
//	  tcp dport 22 ct state new accept } }
//	NFT
//	[ bitwise reg 1 = ( reg 1 & 0x00000006 ) ^ 0x00000000 ]
//	[ bitwise reg 1 = ( reg 1 & 0x00000008 ) ^ 0x00000000 ]
//
// Written the other way round the kernel renders the rule as
// `ct state 0x2000000,0x4000000` and it matches no packet ever sent. That is not
// a theory: loaded into a veth pair against an HTTP server, the reversed
// established/related rule finished the run with `counter packets 0` and the
// request failed, while the corrected one reported `packets 6` and the same
// request returned 200.
//
// Verify this test by mutation, not by reading it: swap ctStateMask for
// binaryutil.BigEndian.PutUint32 and every case below must go red.
func TestCtStateMaskIsTheByteOrderTheKernelCompares(t *testing.T) {
	for _, tc := range []struct {
		name string
		bits uint32
		want []byte
	}{
		{"established|related", ctStateEstablished | ctStateRelated, []byte{0x06, 0x00, 0x00, 0x00}},
		{"new", ctStateNew, []byte{0x08, 0x00, 0x00, 0x00}},
		{"invalid", ctStateInvalid, []byte{0x01, 0x00, 0x00, 0x00}},
	} {
		got := ctStateMask(tc.bits)
		if len(got) != 4 {
			t.Fatalf("%s: mask is %d bytes, the register compare is 4", tc.name, len(got))
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s: ctStateMask(%#x) = % x, want % x\n"+
					"  reversed, the kernel reads bits no conntrack state sets and the rule "+
					"matches nothing — for established|related that is every reply packet "+
					"on the machine", tc.name, tc.bits, got, tc.want)
				break
			}
		}
	}
}

// The state bits themselves, against the values nft's own ct_state_tbl carries.
// A mask in the right byte order over the wrong bit is the same defect one step
// along, and it would render as a plausible-looking `ct state new` on a rule
// that was supposed to say `established`.
func TestCtStateBitsMatchConntrack(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  uint32
		want uint32
	}{
		{"invalid", ctStateInvalid, 0x01},
		{"established", ctStateEstablished, 0x02},
		{"related", ctStateRelated, 0x04},
		{"new", ctStateNew, 0x08},
	} {
		if tc.got != tc.want {
			t.Errorf("ct state %s is %#x, conntrack sets %#x", tc.name, tc.got, tc.want)
		}
	}
}

// ctStateNoMatch is compared against a masked register, so it must be the same
// width as the mask. A short slice here is a silent no-match on every ct rule.
func TestCtStateNoMatchIsRegisterWidth(t *testing.T) {
	if len(ctStateNoMatch) != len(binaryutil.NativeEndian.PutUint32(0)) {
		t.Errorf("ctStateNoMatch is %d bytes, the masked register is 4", len(ctStateNoMatch))
	}
	for i, b := range ctStateNoMatch {
		if b != 0 {
			t.Errorf("ctStateNoMatch[%d] = %#x, want 0", i, b)
		}
	}
}
