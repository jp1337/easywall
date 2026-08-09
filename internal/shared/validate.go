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
		return nil
	}

	if _, err := ParsePortNumber(r.Port); err != nil {
		return err
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
