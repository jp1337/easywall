package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// feedSampleLines and feedSampleBytes bound what a feed's row shows of the
// lines it could not read: the first five, each cut to 80 bytes. Enough to see
// "that is an error page, not a list" without the page carrying the body.
const (
	feedSampleLines = 5
	feedSampleBytes = 80
)

// parseFeed reads one body in one of the three catalogue formats. It returns
// the entries canonical and deduplicated — a full-length prefix as a bare
// address (plan P12), so 100 000 IPv6 entries fit the socket — the number of
// lines that were neither blank, a comment, nor an entry, and the first five
// of those.
//
// It keeps whatever parses, including what the core then drops as not globally
// routable (Hagezi TIF carries three 240/4 addresses): stripping is the
// core's, done once, where nothing can skip it (spec D4).
func parseFeed(format shared.FeedFormat, body []byte) (entries []string, rejected int, sample []string, err error) {
	var entry func(line string) (netip.Prefix, bool, bool) // prefix, skip, ok
	switch format {
	case shared.FeedFormatPlain:
		entry = plainEntry
	case shared.FeedFormatSpamhaus:
		entry = spamhausEntry
	case shared.FeedFormatDShield:
		entry = dshieldEntry
	default:
		return nil, 0, nil, fmt.Errorf("unknown feed format %q", format)
	}

	seen := map[netip.Prefix]struct{}{}
	for raw := range bytes.Lines(body) {
		line := strings.TrimSpace(string(raw))
		p, skip, ok := entry(line)
		switch {
		case skip:
		case ok:
			seen[p] = struct{}{}
		default:
			rejected++
			if len(sample) < feedSampleLines {
				sample = append(sample, sampleLine(line))
			}
		}
	}

	ps := make([]netip.Prefix, 0, len(seen))
	for p := range seen {
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return a.Bits() - b.Bits()
	})
	entries = make([]string, len(ps))
	for i, p := range ps {
		entries[i] = canonicalEntry(p)
	}
	return entries, rejected, sample, nil
}

// plainEntry: one address or prefix per line; '#' and ';' start a comment,
// on a line of their own or after the entry. A line with anything else on it —
// a hosts file's "0.0.0.0 example.com" — is not an entry.
func plainEntry(line string) (netip.Prefix, bool, bool) {
	if i := strings.IndexAny(line, "#;"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if line == "" {
		return netip.Prefix{}, true, false
	}
	p, ok := parseEntry(line)
	return p, false, ok
}

// spamhausEntry: {"cidr":"…",…} per line; the {"type":"metadata",…} record
// that ends each file is skipped, not rejected.
func spamhausEntry(line string) (netip.Prefix, bool, bool) {
	if line == "" {
		return netip.Prefix{}, true, false
	}
	var rec struct {
		CIDR string `json:"cidr"`
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil {
		return netip.Prefix{}, false, false
	}
	if rec.Type == "metadata" && rec.CIDR == "" {
		return netip.Prefix{}, true, false
	}
	p, ok := parseEntry(rec.CIDR)
	return p, false, ok
}

// dshieldEntry: start<TAB>end<TAB>prefixlen<TAB>…, '#' comments. The start
// must be the network's own address.
func dshieldEntry(line string) (netip.Prefix, bool, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return netip.Prefix{}, true, false
	}
	f := strings.Split(line, "\t")
	if len(f) < 3 {
		return netip.Prefix{}, false, false
	}
	start, err := netip.ParseAddr(strings.TrimSpace(f[0]))
	if err != nil || start.Zone() != "" {
		return netip.Prefix{}, false, false
	}
	bits, err := strconv.Atoi(strings.TrimSpace(f[2]))
	if err != nil {
		return netip.Prefix{}, false, false
	}
	p, err := start.Unmap().Prefix(bits)
	if err != nil || p.Addr() != start.Unmap() {
		return netip.Prefix{}, false, false
	}
	return p, false, true
}

// parseEntry reads a bare address or a prefix, masked, with ::ffff: unmapped
// so an IPv4 address written in IPv6 lands in the IPv4 set. A zone is refused:
// "fe80::1%eth0" names an interface on the publisher's host.
func parseEntry(s string) (netip.Prefix, bool) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, false
		}
		a, bits := p.Addr(), p.Bits()
		if a.Is4In6() {
			if bits < 96 {
				return netip.Prefix{}, false
			}
			a, bits = a.Unmap(), bits-96
		}
		p, err = a.Prefix(bits)
		return p, err == nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Prefix{}, false
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), true
}

// canonicalEntry is what goes on the wire: a host prefix as its bare address.
func canonicalEntry(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}

// sampleLine cuts a rejected line for display: at most feedSampleBytes, never
// half a character, control characters shown as '?'.
func sampleLine(s string) string {
	s = strings.ToValidUTF8(s, "?")
	if len(s) > feedSampleBytes {
		// The cut can split a character; its remaining bytes are dropped.
		s = strings.ToValidUTF8(s[:feedSampleBytes], "")
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, s)
}
