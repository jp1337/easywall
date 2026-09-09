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
	Chain string // the chain the rule was being added to

	// Index is the position within that chain, 0-based, or noIndex when the
	// finding is about the chain rather than about one rule in it — a chain
	// that accepts has no single position to blame.
	//
	// It was a plain zero for both per-chain checks, so a jump at input rule 3
	// to a missing chain was reported as "input rule 0". A wrong number in an
	// audit entry is worse than no number: it sends whoever reads it to the
	// wrong builder.
	Index  int
	Reason string
}

// noIndex marks a finding that has no position within its chain. -1 rather than
// a second bool field, because String is the only reader and a bool would have
// to be kept in step with the number it qualifies.
const noIndex = -1

func (f Finding) String() string {
	if f.Index == noIndex {
		return fmt.Sprintf("%s: %s", f.Chain, f.Reason)
	}
	return fmt.Sprintf("%s rule %d: %s", f.Chain, f.Index, f.Reason)
}

// auditBuildFindings records what the expression check found about the rules an
// apply just wrote, or writes nothing when it found nothing.
//
// checkBuilt never refuses a write — a layer that stops filtering when it is
// unsure is the failure everConfigured exists to avoid (restore.go:218) — so it
// logs to the journal and degrades the health state. Until this entry the audit
// log, which is the record an operator greps after an incident, said the apply
// had simply succeeded. `health_degraded` carries a tone in the interface
// because it is a statement about what the firewall is doing: a rule is in the
// kernel and may match nothing, which is the class that let `ct state
// established,related accept` enforce nothing for five releases.
//
// The first finding and a count, not all of them. Every Reason is a fixed
// sentence with nothing an operator typed interpolated into it, so the text is
// safe to record verbatim — but a malformed ruleset can produce one finding per
// rule, and a 30 kB line in a log rendered on a web page is a page nobody can
// read. checkBuilt has already written every one of them to the journal.
//
// A function rather than four lines at the call site, and the reason is
// testability: Firewall.nft is a concrete *NftablesManager, and Apply cannot
// reach its success path without a kernel to flush to, so the formatting here
// would otherwise be provable only under `-tags integration`.
func auditBuildFindings(logPath string, findings []Finding, user string) {
	if len(findings) == 0 {
		return
	}
	detail := findings[0].String()
	if len(findings) > 1 {
		detail += fmt.Sprintf(" (and %d more; see the journal)", len(findings)-1)
	}
	WriteAuditLog(logPath, "health_degraded", "all", detail, user)
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
// The returned slice is not ordered. Two of the checks are per-chain rather
// than per-rule and are collected in maps, so callers must not depend on
// position; the tests assert on counts and on substrings.
//
// **It reads top-level Exprs only.** A verdict or a bitwise nested inside an
// expr.Dynset is invisible to it. No builder produces one today —
// addPerSourceRateLimit is the only Dynset in the tree and it carries a single
// expr.Limit — so recursing would be scaffolding for a case that does not
// exist. Whoever puts a verdict or a mask inside a Dynset has to extend this,
// and will not be told by a failing test that they need to.
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
	//
	// Every jump site, not the last one seen: this was a map to a single chain
	// name, so two chains jumping to the same missing target produced one
	// finding naming whichever of them was added last. Both jumps are wrong and
	// both have to be named, or a maintainer is sent to one builder to fix a
	// rule in another.
	accepts := map[string]bool{}
	jumped := map[string][]jumpSite{}

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
					jumped[v.Chain] = append(jumped[v.Chain], jumpSite{chain, idx})
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

	for target, sites := range jumped {
		if !created[target] {
			// One per jump site, each naming the rule that jumps.
			for _, s := range sites {
				findings = append(findings, Finding{
					Chain:  s.from,
					Index:  s.index,
					Reason: "the rule jumps to a chain this transaction does not create",
				})
			}
		}
		// Once per target, not once per jump site: the fault is in the target
		// chain, and it is the same fault however many rules reach it.
		if accepts[target] && !acceptingChains[target] {
			findings = append(findings, Finding{
				Chain: target,
				Index: noIndex,
				Reason: "a jumped-to chain accepts, so it outranks every rule " +
					"after the jump; it must drop or return",
			})
		}
	}
	return findings
}

// jumpSite is one rule that jumps, and where it is.
type jumpSite struct {
	from  string // the chain the jumping rule is in
	index int    // its position in that chain
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
