package core

import (
	"fmt"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
)

// The check that runs in front of netlink.
//
// For five releases `ct state established,related accept` matched no packet.
// All three ct-state masks were written big-endian; the kernel writes the state
// into a register as a native u32 and compares it that way. The rule was
// present, reported itself present, and enforced nothing — the invalid-packet
// drop and the SSH brute-force meter with it. It was found because an
// operator's VPS went unreachable after `docker compose up -d` and they pasted
// the ruleset into Discord, and the unit test covering the rule had written the
// defect down as expected output.
//
// That is the shape of the problem this file exists for. A rule can be wrong in
// a way that no amount of reading the diff reveals, because the wrongness is in
// four bytes of a register mask and both the code and the kernel's own printout
// look exactly as intended. What cannot look right is the *decoded* mask, and
// this decodes it.
//
// It needs no kernel, so it runs on every Apply and — the half that actually
// kills the class — in CI on every build, over a table with every protection
// module switched on. In CI a finding is a failure. On a host it is a degraded
// health state and the table is written anyway: see the comment at the call
// site in Apply, and everConfigured in restore.go, whose reasoning is the same
// one. A layer that refuses to filter when it is unsure is a layer that can
// quietly stop enforcing rules somebody wrote.

// Finding is one thing wrong with one built rule.
//
// Reason is a fixed sentence. Nothing an operator typed is interpolated into
// it, because this text reaches the audit log, and the audit log takes no free
// text: an address or a port name arriving in a Reason would put attacker-chosen
// bytes into the record an operator greps after an incident.
type Finding struct {
	Chain  string // the chain the rule was being added to
	Index  int    // position within that chain, 0-based
	Reason string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s rule %d: %s", f.Chain, f.Index, f.Reason)
}

// ctStateAllBits is every bit a conntrack state can carry, as the four
// constants in nftables.go name them. A mask with a bit outside this set
// matches nothing at all — `ct state 0x8000000` is what the ruleset pasted into
// Discord contained, and it is what a big-endian 0x08 decodes to.
const ctStateAllBits = ctStateInvalid | ctStateEstablished | ctStateRelated | ctStateNew

// CheckRules reports what is wrong with rules, which is one nftables
// transaction's worth in the order they were added.
//
// acceptingChains names the jumped-to chains that are allowed to end in accept.
// Passing nil means none is, which is the state easywall's own table should be
// in: every module chain either drops or returns, so under-rate traffic falls
// back into the input chain and meets the blacklist. The SSH chain accepted
// until 2.16, which meant a blacklisted address could open SSH as long as it
// stayed under the rate limit — a protection module outranking the blacklist.
// The parameter exists so a future chain that legitimately accepts is declared
// in one list a reviewer reads, rather than by weakening the rule for all of
// them.
//
// The returned slice is not ordered. Two of the four checks are per-chain
// rather than per-rule and are collected in maps, so callers must not depend on
// position; the tests assert on counts and on substrings.
func CheckRules(rules []*nftables.Rule, acceptingChains map[string]bool) []Finding {
	var findings []Finding

	// A chain is "created" if this transaction puts a rule in one addressed at
	// it. Every chain easywall jumps to carries at least one unconditional rule
	// — bogon its drops, portscan and every -over chain their drop, sshbrute its
	// return — so a target missing from this set is a target that was never
	// built. The alternative, recording AddChain too, would need a second seam
	// through the manager for a case no builder produces.
	created := map[string]bool{}
	for _, r := range rules {
		if r.Chain != nil {
			created[r.Chain.Name] = true
		}
	}

	// Which chains accept, and which are jumped to. Collected in one pass so
	// the verdict is per-chain while the ct-state report stays per-rule.
	accepts := map[string]bool{}
	jumped := map[string]string{} // target chain -> a chain that jumps to it

	perChain := map[string]int{}
	for _, r := range rules {
		chain := "?"
		if r.Chain != nil {
			chain = r.Chain.Name
		}
		idx := perChain[chain]
		perChain[chain]++

		add := func(reason string) {
			findings = append(findings, Finding{Chain: chain, Index: idx, Reason: reason})
		}

		for i, e := range r.Exprs {
			switch v := e.(type) {
			case *expr.Bitwise:
				// Only a bitwise that follows a ct-state load is a ct-state
				// mask. Every other bitwise in this table is an address netmask
				// or a TCP flags mask, and this check has nothing to say about
				// either: a /8 netmask is 0xff000000, which names no conntrack
				// state, so a check without this test would report every
				// whitelisted network in the table.
				if !precededByCtState(r.Exprs, i, v) {
					continue
				}
				if len(v.Mask) != 4 {
					add("a conntrack state mask is not four bytes wide")
					continue
				}
				native := binaryutil.NativeEndian.Uint32(v.Mask)
				if native == 0 || native & ^uint32(ctStateAllBits) != 0 {
					add("the conntrack state mask names bits no conntrack " +
						"state carries; it is probably byte-reversed")
				}
			case *expr.Verdict:
				switch v.Kind {
				case expr.VerdictJump, expr.VerdictGoto:
					jumped[v.Chain] = chain
				case expr.VerdictAccept:
					accepts[chain] = true
				// The remaining eight kinds expr defines. All eleven are listed
				// rather than the handful easywall builds, because the default
				// below fails a build: reporting `stolen`, `repeat` or `stop` as
				// a verdict the kernel does not define would be a false positive
				// in a gate, and a gate that cries wolf is a gate somebody
				// switches off. See verdict.go in google/nftables, where the
				// constants run from VerdictReturn (-5) to VerdictStop (5).
				case expr.VerdictDrop, expr.VerdictReturn, expr.VerdictContinue,
					expr.VerdictQueue, expr.VerdictBreak, expr.VerdictStolen,
					expr.VerdictRepeat, expr.VerdictStop:
				default:
					add("the rule ends in a verdict this kernel does not define")
				}
			}
		}
	}

	for target, from := range jumped {
		if !created[target] {
			findings = append(findings, Finding{
				Chain:  from,
				Reason: "the rule jumps to a chain this transaction does not create",
			})
		}
		if accepts[target] && !acceptingChains[target] {
			findings = append(findings, Finding{
				Chain: target,
				Reason: "a jumped-to chain accepts, so it outranks every rule " +
					"after the jump; it must drop or return",
			})
		}
	}
	return findings
}

// precededByCtState reports whether bw, the expression at index i, masks a
// conntrack state that was loaded into the register it reads from.
//
// Walking backwards to the nearest expr.Ct rather than only to i-1: the
// builders put the load immediately before the mask today, and a rule that
// grows a counter or a payload test in between must not silently fall out of
// this check's scope.
func precededByCtState(exprs []expr.Any, i int, bw *expr.Bitwise) bool {
	for j := i - 1; j >= 0; j-- {
		if ct, is := exprs[j].(*expr.Ct); is {
			return ct.Key == expr.CtKeySTATE && ct.Register == bw.SourceRegister
		}
	}
	return false
}
