package shared

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ValidateRules is the one definition of what a storable rule set is.
//
// It lives here rather than in the core because three places have to agree on
// it: the core, which must not trust what the web process sends; the web
// process, which owes the operator a message naming the line rather than a
// redirect; and the demo, which represents the product to everyone who has not
// installed it yet and used to accept input the real thing refuses.
func ValidateRules(r Rules) error {
	for _, rule := range r.TCP {
		if err := validatePortRule(rule); err != nil {
			return fmt.Errorf("tcp rule %q: %w", rule.Port, err)
		}
	}
	for _, rule := range r.UDP {
		if err := validatePortRule(rule); err != nil {
			return fmt.Errorf("udp rule %q: %w", rule.Port, err)
		}
	}
	for _, ip := range r.Blacklist {
		if IsListComment(ip) {
			continue
		}
		if err := validateIPOrCIDR(ip); err != nil {
			return fmt.Errorf("blacklist %q: %w", ip, err)
		}
	}
	for _, ip := range r.Whitelist {
		if IsListComment(ip) {
			continue
		}
		if err := validateIPOrCIDR(ip); err != nil {
			return fmt.Errorf("whitelist %q: %w", ip, err)
		}
	}
	for i, rule := range r.Custom {
		if err := validateCustomRule(rule); err != nil {
			return fmt.Errorf("custom rule %d: %w", i+1, err)
		}
	}
	for _, fwd := range r.Forwarding {
		if fwd.Protocol != "tcp" && fwd.Protocol != "udp" {
			return fmt.Errorf("forwarding: invalid protocol %q", fwd.Protocol)
		}
		if fwd.SourcePort < 1 || fwd.SourcePort > 65535 {
			return fmt.Errorf("forwarding: invalid source port %d", fwd.SourcePort)
		}
		if fwd.DestPort < 1 || fwd.DestPort > 65535 {
			return fmt.Errorf("forwarding: invalid dest port %d", fwd.DestPort)
		}
	}
	return nil
}

// validatePortRule accepts "80" or "8000:9000" and nothing else.
//
// It used to parse with fmt.Sscanf, which stops at the first thing it cannot
// read and reports success for what it got: "80abc" passed as port 80, and so
// did "80 90" — someone who meant to open two ports opened one, and the rule
// list showed a string the firewall was not enforcing. strconv.Atoi rejects any
// trailing character, so what is stored is what is applied.
func validatePortRule(r PortRule) error {
	if r.Port == "" {
		return fmt.Errorf("port is required")
	}

	if start, end, ok := strings.Cut(r.Port, ":"); ok {
		lo, err := ParsePortNumber(start)
		if err != nil {
			return fmt.Errorf("port range start: %w", err)
		}
		hi, err := ParsePortNumber(end)
		if err != nil {
			return fmt.Errorf("port range end: %w", err)
		}
		if hi < lo {
			return fmt.Errorf("port range %d:%d ends before it starts", lo, hi)
		}
	} else if _, err := ParsePortNumber(r.Port); err != nil {
		return err
	}

	// The source restriction is an address list like every other one in this
	// file: comments and blank spacers are part of what the operator typed and
	// are skipped rather than refused, exactly as cidrMatch skips them when it
	// builds the rule.
	for _, src := range r.Sources {
		if IsListComment(src) {
			continue
		}
		if err := validateIPOrCIDR(strings.TrimSpace(src)); err != nil {
			return fmt.Errorf("source %q: %w", src, err)
		}
	}
	return nil
}

// customRuleSeparators are the characters nft reads as the end of one command
// and the start of the next.
const customRuleSeparators = "\n\r;"

// validateCustomRule checks that a rule is one nft statement and not several.
//
// Custom rules are appended to a script as `add rule inet easywall input <rule>`,
// and nft takes both a newline and a semicolon as a command separator. A rule
// carrying either is not a rule: it is a second command, run by the root daemon,
// able to reach tables easywall does not own. Demonstrated against a real
// kernel — an imported rule containing a newline wrote into a neighbouring
// table, which is precisely what "easywall owns one table and never looks at
// another" says cannot happen.
//
// The textarea splits on newlines, so this was reachable through import, where
// a JSON string can hold anything. Structural rather than a parser check on
// purpose: it does not depend on nft's grammar, on a subprocess being available,
// or on the wrapper used for syntax checking happening to be unbalanced.
func validateCustomRule(rule string) error {
	if i := strings.IndexAny(rule, customRuleSeparators); i >= 0 {
		return fmt.Errorf("contains %q, which nft reads as the end of the command; "+
			"a custom rule is a single statement, so put each on its own line",
			string(rule[i]))
	}
	return nil
}

// ValidateNetworkList is the one definition of what the two network lists on the
// Network page hold: `docker.custom_networks` and `routing.networks`.
//
// It lives here for the same reason ValidateRules does — three places have to
// agree on it. The core must not trust what the web process sends; the web
// process owes the operator a message naming the entry rather than a generic
// "check core connection"; and the demo represents the product to everyone who
// has not installed it yet.
//
// They had three different answers. The editor validated with the blacklist's
// rules, which accept a bare address and skip comments and blanks; the core
// demanded net.ParseCIDR of every element including those; and the demo checked
// nothing. So a blank line between two networks, a `#` note, or a bare address
// was accepted by the page, refused by the core, and reported as "Failed to save
// changes. Check core connection." — with a working core.
//
// Comments and blank lines are skipped rather than refused, because that is what
// the rest of the product does with a list an operator types: the editors keep
// them, countEntries ignores them, and addCIDRAccept and cidrMatch in the core
// already skip them when building rules. This function was the only place that
// did not.
func ValidateNetworkList(what string, entries []string) error {
	for _, entry := range entries {
		if IsListComment(entry) {
			continue
		}
		if _, _, err := net.ParseCIDR(strings.TrimSpace(entry)); err != nil {
			return fmt.Errorf("%s %q: not a CIDR network", what, entry)
		}
	}
	return nil
}

// ParsePortNumber parses a complete port number, rejecting anything else.
func ParsePortNumber(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port %d is outside 1-65535", p)
	}
	return p, nil
}

// isListComment reports whether an address-list entry is a comment or a blank
// spacer. Both are part of the stored list — the editor keeps them so an
// operator's note about why an address is blocked survives a save — and neither
// is an address to validate or turn into a rule.
func IsListComment(entry string) bool {
	entry = strings.TrimSpace(entry)
	return entry == "" || strings.HasPrefix(entry, "#")
}

func validateIPOrCIDR(s string) error {
	if ip := net.ParseIP(s); ip != nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}
	return fmt.Errorf("invalid IP or CIDR: %s", s)
}
