package web

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path"
	"slices"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// The country column on /blocked (2.27, spec D4): which country each address
// block is registered to, from ipverse's whole dataset, held by this process
// in memory. A lookup is a label on a row, never a rule — the core never sees
// this table.

// countryArchiveMaxUnpacked bounds what one archive may decompress to: 8 657 778
// bytes on 2026-10-14, so 64 MiB is 7.7 times that. A body is capped at
// feedMaxBody before it gets here, and deflate expands at most about 1 032:1,
// so without this an 8 MiB bomb could make the reader inflate gigabytes.
// A var so a test can prove the bound without a 64 MiB fixture.
var countryArchiveMaxUnpacked int64 = 64 << 20

var (
	errCountryArchiveEmpty    = errors.New("country archive: not one country's aggregated.json")
	errCountryArchiveTooLong  = errors.New("country archive: larger than it may unpack to")
	errCountryArchiveUnsorted = errors.New("country archive: ranges out of order")
)

// countryArchiveMaxPrefixes bounds how many prefixes one archive may hold:
// 248 507 on 2026-10-14, so 2.1 times that. Without it, seven files of 8 MiB each repeating one
// prefix — 117 KB compressed, inside every size bound — drove the heap to
// 310 MB (pre-flight review, 2.27).
const countryArchiveMaxPrefixes = 1 << 19

// countryRanges is one family's table: sorted, disjoint ranges, each with the
// index of its code in shared.CountryCodes. IPv4 is the address as a number;
// IPv6 is its first 64 bits, because no delegation is longer than a /64 (the
// longest across all 69 614 on 2026-10-14 is /64).
//
// Three parallel slices, not a slice of structs with a netip.Prefix: measured
// on the whole dataset, 17 bytes a range against 64 (the plan's D4 numbers).
type countryRanges struct {
	start, end []uint64
	cc         []uint8
}

type countryTable struct {
	v4, v6    countryRanges
	countries int
}

// lookup is the country addr's block is registered to, lower case, or "" when
// none is — a private or reserved address, one between two ranges, or no table.
func (t *countryTable) lookup(addr netip.Addr) string {
	addr = addr.Unmap()
	if t == nil || !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return ""
	}
	rs, k := &t.v6, addrKey(addr)
	if addr.Is4() {
		rs = &t.v4
	}
	i, found := slices.BinarySearch(rs.start, k)
	if !found {
		i--
	}
	if i < 0 || k > rs.end[i] {
		return ""
	}
	return shared.CountryCodes[rs.cc[i]]
}

// ranges is how many ranges the table holds, both families.
func (t *countryTable) ranges() int {
	if t == nil {
		return 0
	}
	return len(t.v4.start) + len(t.v6.start)
}

// addrKey is an IPv4 address as a number, or an IPv6 address's first 64 bits.
func addrKey(a netip.Addr) uint64 {
	if a.Is4() {
		b := a.As4()
		return uint64(binary.BigEndian.Uint32(b[:]))
	}
	b := a.As16()
	return binary.BigEndian.Uint64(b[:8])
}

// prefixRange is p as the first and last key it covers. An IPv6 prefix longer
// than /64 covers its whole /64.
// shortcut: a /65../128 labels its whole /64; none exists in the data (2026-10-14).
func prefixRange(p netip.Prefix) (uint64, uint64) {
	width := 64
	if p.Addr().Is4() {
		width = 32
	}
	host := width - min(p.Bits(), width)
	start := addrKey(p.Addr())
	if host == 64 {
		return start, ^uint64(0)
	}
	return start, start | (uint64(1)<<host - 1)
}

// countryPrefix is one prefix of the archive, already a range: 24 bytes
// where a netip.Prefix and its code took 40.
type countryPrefix struct {
	start, end uint64
	bits, cc   uint8
}

func newCountryPrefix(p netip.Prefix, cc uint8) countryPrefix {
	s, e := prefixRange(p)
	return countryPrefix{start: s, end: e, bits: uint8(p.Bits()), cc: cc} // #nosec G115 -- 0..128
}

// buildCountryRanges sorts and merges one family. Two prefixes are either
// disjoint or one inside the other; inside, the narrower one wins — ipverse
// lists 123.100.242.0/24 under DE inside SG's 123.100.240.0/22 (2026-10-14).
// Neighbours of one country merge into one range.
//
// A sweep over the prefixes sorted by start, wider first, with a stack of the
// ones that enclose the current address: each address is emitted once, under
// the innermost prefix that holds it, at any depth of nesting.
func buildCountryRanges(ps []countryPrefix) countryRanges {
	slices.SortFunc(ps, func(a, b countryPrefix) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.bits, b.bits), cmp.Compare(a.cc, b.cc))
	})
	// A nested prefix adds at most two ranges; most merge. Sized for the
	// common case, so the table's slices are not twice what they hold.
	rs := countryRanges{
		start: make([]uint64, 0, len(ps)), end: make([]uint64, 0, len(ps)), cc: make([]uint8, 0, len(ps)),
	}
	add := func(s, e uint64, cc uint8) {
		if n := len(rs.start); n > 0 && rs.cc[n-1] == cc && rs.end[n-1] != ^uint64(0) && rs.end[n-1]+1 == s {
			rs.end[n-1] = e
			return
		}
		rs.start, rs.end, rs.cc = append(rs.start, s), append(rs.end, e), append(rs.cc, cc)
	}
	var stack []countryPrefix
	var next uint64 // the first address not yet emitted
	done := false   // an emitted range reached the last address; next would wrap
	// pop closes the innermost open prefix: what of it is not yet emitted is its own.
	pop := func() {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !done && next <= top.end {
			add(next, top.end, top.cc)
		}
		if top.end == ^uint64(0) {
			done = true
		} else {
			next = top.end + 1
		}
	}
	for _, p := range ps {
		for len(stack) > 0 && stack[len(stack)-1].end < p.start {
			pop()
		}
		if n := len(stack); n > 0 && next < p.start {
			add(next, p.start-1, stack[n-1].cc)
		}
		stack, next = append(stack, p), p.start
	}
	for len(stack) > 0 {
		pop()
	}
	rs.start, rs.end, rs.cc = slices.Clip(rs.start), slices.Clip(rs.end), slices.Clip(rs.cc)
	return rs
}

// check is the order lookup's binary search relies on: every range starts no
// later than it ends, and ends before the next one starts. O(n), after every
// build, so a merge that goes wrong refuses the archive instead of labelling
// addresses with the wrong country.
func (rs countryRanges) check() error {
	for i := range rs.start {
		if rs.start[i] > rs.end[i] || i > 0 && rs.end[i-1] >= rs.start[i] {
			return fmt.Errorf("%w: range %d is [%d,%d]", errCountryArchiveUnsorted, i, rs.start[i], rs.end[i])
		}
	}
	return nil
}

// parseCountryArchive reads ipverse's release archive: every
// country/<cc>/aggregated.json, nothing else. A code not in
// shared.CountryCodes is skipped; a prefix that does not parse is skipped, as
// parseFeed counts one. Any error — a truncated archive, a file that is not
// JSON, more than it may unpack to, not one country — returns no table, and
// the caller keeps the one it has.
func parseCountryArchive(r io.Reader) (*countryTable, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("country archive: %w", err)
	}
	lr := &io.LimitedReader{R: gz, N: countryArchiveMaxUnpacked}
	tr := tar.NewReader(lr)
	// Sized for the real archive, so the slices do not double their way there.
	v4, v6 := make([]countryPrefix, 0, 1<<18), make([]countryPrefix, 0, 1<<17)
	seen := map[uint8]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("country archive: %w", err)
		}
		if ok, _ := path.Match("country/*/aggregated.json", hdr.Name); !ok || hdr.Typeflag != tar.TypeReg {
			continue
		}
		idx := slices.Index(shared.CountryCodes, strings.Split(hdr.Name, "/")[1])
		if idx < 0 {
			continue
		}
		if hdr.Size > feedMaxBody {
			return nil, errCountryArchiveTooLong
		}
		var doc struct {
			Prefixes struct {
				IPv4 []string `json:"ipv4"`
				IPv6 []string `json:"ipv6"`
			} `json:"prefixes"`
		}
		if err := json.NewDecoder(tr).Decode(&doc); err != nil {
			return nil, fmt.Errorf("country archive: %s: %w", hdr.Name, err)
		}
		cc := uint8(idx) // #nosec G115 -- TestCountryCodesFitAByte
		for _, list := range [][]string{doc.Prefixes.IPv4, doc.Prefixes.IPv6} {
			for _, s := range list {
				p, ok := parseEntry(s)
				if !ok {
					continue
				}
				if len(v4)+len(v6) >= countryArchiveMaxPrefixes {
					return nil, fmt.Errorf("%w: more than %d prefixes", errCountryArchiveTooLong, countryArchiveMaxPrefixes)
				}
				seen[cc] = true
				if p.Addr().Is4() {
					v4 = append(v4, newCountryPrefix(p, cc))
				} else {
					v6 = append(v6, newCountryPrefix(p, cc))
				}
			}
		}
	}
	if lr.N <= 0 {
		return nil, errCountryArchiveTooLong
	}
	if len(seen) == 0 {
		return nil, errCountryArchiveEmpty
	}
	t := &countryTable{v4: buildCountryRanges(v4), v6: buildCountryRanges(v6), countries: len(seen)}
	if err := errors.Join(t.v4.check(), t.v6.check()); err != nil {
		return nil, err
	}
	return t, nil
}
