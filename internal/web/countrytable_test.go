package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"math/rand/v2"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// countryArchive is a release archive holding files, in order, the way
// ipverse's tarball lays them out: country/<cc>/aggregated.json beside the two
// .txt files.
func countryArchive(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	return gzipped(t, tarOf(t, files...))
}

func tarOf(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0644, Size: int64(len(f[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gzipped(t *testing.T, raw []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func aggregated(v4, v6 string) string {
	return `{"countryCode":"XX","prefixes":{"ipv4":[` + v4 + `],"ipv6":[` + v6 + `]}}`
}

// sampleCountryFiles: three countries on documentation addresses (RFC 5737,
// RFC 3849) — feed_parse_test.go's rule for every fixture — one of them also
// listing 10.0.0.0/8, which no lookup may answer: a private address is
// nobody's. The .txt beside DE's JSON is never read.
var sampleCountryFiles = [][2]string{
	{"country/de/aggregated.json", aggregated(`"192.0.2.0/24","10.0.0.0/8"`, `"2001:db8:1::/48"`)},
	{"country/de/ipv4-aggregated.txt", "198.18.0.0/15\n"},
	{"country/ie/aggregated.json", aggregated(`"198.51.100.0/25"`, `"2001:db8:2::/48"`)},
	{"country/ru/aggregated.json", aggregated(`"203.0.113.128/25"`, ``)},
}

func sampleCountryTable(t *testing.T) *countryTable {
	t.Helper()
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t, sampleCountryFiles...)))
	if err != nil {
		t.Fatalf("parseCountryArchive: %v", err)
	}
	return tab
}

// Spec §3: a v4 and a v6 address resolve to the right country; a private
// address, one between two ranges and an empty table give none.
func TestACountryIsLookedUpForBothFamilies(t *testing.T) {
	tab := sampleCountryTable(t)
	for addr, want := range map[string]string{
		"192.0.2.80":         "de",
		"::ffff:192.0.2.80":  "de", // an IPv4 address written in IPv6
		"2001:db8:1:4200::1": "de",
		"198.51.100.127":     "ie", // the last address of the /25
		"2001:db8:2::1":      "ie",
		"203.0.113.200":      "ru",
		"198.51.100.128":     "", // one past the /25
		"203.0.113.9":        "", // between two ranges
		"1.1.1.1":            "", // below every IPv4 range
		"2001:db8::1":        "", // below every IPv6 range
		"10.1.2.3":           "", // private, though the archive lists it
		"fe80::1":            "", // link-local
		"198.18.0.1":         "", // only in a .txt the parser does not read
	} {
		if got := tab.lookup(netip.MustParseAddr(addr)); got != want {
			t.Errorf("lookup(%s) = %q, want %q", addr, got, want)
		}
	}
	if tab.countries != 3 {
		t.Errorf("countries = %d, want 3", tab.countries)
	}
	var none *countryTable
	if got := none.lookup(netip.MustParseAddr("192.0.2.80")); got != "" {
		t.Errorf("no table answered %q", got)
	}
}

// ipverse lists 123.100.242.0/24 under DE inside SG's 123.100.240.0/22
// (2026-10-14): the narrower one names the addresses it holds, the wider one
// keeps the rest on both sides of it. The same shape on documentation
// addresses, with a neighbour of SG's after it.
func TestTheNarrowerPrefixWins(t *testing.T) {
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t,
		[2]string{"country/de/aggregated.json", aggregated(`"192.0.2.32/27"`, ``)},
		[2]string{"country/sg/aggregated.json", aggregated(`"192.0.2.0/25","192.0.2.128/26"`, ``)},
	)))
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{
		"192.0.2.31": "sg", "192.0.2.32": "de", "192.0.2.63": "de", "192.0.2.64": "sg",
		"192.0.2.191": "sg", "192.0.2.192": "",
	} {
		if got := tab.lookup(netip.MustParseAddr(addr)); got != want {
			t.Errorf("lookup(%s) = %q, want %q", addr, got, want)
		}
	}
	// sg .0–.31, de .32–.63, then sg .64–.127 and its neighbour .128–.191 as one.
	if got := tab.ranges(); got != 3 {
		t.Errorf("%d ranges, want 3 — neighbours of one country merge, the hole splits", got)
	}
}

func TestAnArchiveThatCannotBeReadGivesNoTable(t *testing.T) {
	// Cut in the middle of a file the parser skips: the tar reader meets the
	// broken gzip stream while skipping it, after one country was already read.
	r := rand.New(rand.NewPCG(1, 2))
	filler := make([]byte, 64<<10)
	for i := range filler {
		filler[i] = "0123456789abcdef"[r.IntN(16)]
	}
	long := countryArchive(t, sampleCountryFiles[0], [2]string{"country/de/ipv6-aggregated.txt", string(filler)}, sampleCountryFiles[2])
	for name, body := range map[string][]byte{
		"truncated":  long[:len(long)/2],
		"not gzip":   []byte("<html>a captive portal</html>"),
		"not json":   countryArchive(t, [2]string{"country/de/aggregated.json", `{"prefixes":`}),
		"no country": countryArchive(t, [2]string{"country/zz/aggregated.json", aggregated(`"192.0.2.0/24"`, ``)}, [2]string{"README.md", "x"}),
	} {
		if tab, err := parseCountryArchive(bytes.NewReader(body)); err == nil {
			t.Errorf("%s: read as a table of %d ranges", name, tab.ranges())
		}
	}
}

// The unpack bound: an archive that reaches it is refused even when it ends on
// a file boundary, where the tar reader alone sees a clean end.
func TestAnArchiveLargerThanItMayUnpackToIsRefused(t *testing.T) {
	raw := tarOf(t, sampleCountryFiles...)
	defer func(n int64) { countryArchiveMaxUnpacked = n }(countryArchiveMaxUnpacked)
	countryArchiveMaxUnpacked = int64(len(raw) - 1024) // everything but the two zero blocks that end it
	if _, err := parseCountryArchive(bytes.NewReader(gzipped(t, raw))); !errors.Is(err, errCountryArchiveTooLong) {
		t.Errorf("err = %v, want errCountryArchiveTooLong", err)
	}
	countryArchiveMaxUnpacked = int64(len(raw)) + 1
	if _, err := parseCountryArchive(bytes.NewReader(gzipped(t, raw))); err != nil {
		t.Errorf("an archive inside the bound: %v", err)
	}
}

// The table stores a country as its index in CountryCodes, in a byte.
func TestCountryCodesFitAByte(t *testing.T) {
	if n := len(shared.CountryCodes); n > 256 {
		t.Fatalf("%d country codes; countryRanges.cc is a uint8", n)
	}
}

// tableOf is a table of one country per prefix, read through the archive.
func tableOf(t *testing.T, prefixes map[string]string) *countryTable {
	t.Helper()
	var files [][2]string
	for cc, p := range prefixes {
		files = append(files, [2]string{"country/" + cc + "/aggregated.json", aggregated(`"`+p+`"`, ``)})
	}
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t, files...)))
	if err != nil {
		t.Fatalf("parseCountryArchive: %v", err)
	}
	return tab
}

// checkEveryAddress looks up all 256 addresses of 192.0.2.0/24 and compares
// each with the narrowest of prefixes that holds it, found by brute force.
func checkEveryAddress(t *testing.T, tab *countryTable, prefixes map[string]string) {
	t.Helper()
	wrong := 0
	for i := range 256 {
		a := netip.AddrFrom4([4]byte{192, 0, 2, byte(i)})
		want, bits := "", -1
		for cc, s := range prefixes {
			if p := netip.MustParsePrefix(s); p.Contains(a) && p.Bits() > bits {
				want, bits = cc, p.Bits()
			}
		}
		if got := tab.lookup(a); got != want {
			if wrong++; wrong <= 5 {
				t.Errorf("lookup(%s) = %q, want %q", a, got, want)
			}
		}
	}
	if wrong > 0 {
		t.Errorf("%d of 256 addresses labelled wrong", wrong)
	}
}

// Pre-flight review, 2.27: SG /24 ⊃ DE /25 ⊃ IE /26 ⊃ RU /27, and FR /32
// inside DE. A merge that only knew the last range made [128,63] and labelled
// 87 of 256 addresses wrong.
func TestNestingThreeLevelsDeepLabelsEveryAddress(t *testing.T) {
	ps := map[string]string{
		"sg": "192.0.2.0/24", "de": "192.0.2.0/25", "ie": "192.0.2.64/26",
		"ru": "192.0.2.64/27", "fr": "192.0.2.10/32",
	}
	checkEveryAddress(t, tableOf(t, ps), ps)
}

// Siblings inside one parent: the parent keeps every gap between them, and
// the last address of the /24. IE's /26 closes RU's /27 and DE's /26 at once.
func TestSiblingPrefixesInsideOneParent(t *testing.T) {
	ps := map[string]string{
		"sg": "192.0.2.0/24", "de": "192.0.2.0/26", "ru": "192.0.2.32/27",
		"ie": "192.0.2.128/26", "fr": "192.0.2.255/32",
	}
	checkEveryAddress(t, tableOf(t, ps), ps)
}

// The same comparison on random nestings: up to eight prefixes of /24 to /32
// inside 192.0.2.0/24, each its own country.
func TestRandomNestingsLabelEveryAddress(t *testing.T) {
	codes := []string{"de", "fr", "ie", "ru", "sg", "nl", "pl", "se"}
	r := rand.New(rand.NewPCG(5, 6))
	for range 300 {
		ps, seen := map[string]string{}, map[string]bool{}
		for _, cc := range codes[:1+r.IntN(len(codes))] {
			p := netip.PrefixFrom(netip.AddrFrom4([4]byte{192, 0, 2, byte(r.IntN(256))}), 24+r.IntN(9)).Masked().String()
			if !seen[p] { // one prefix under two countries has no narrower one to win
				ps[cc], seen[p] = p, true
			}
		}
		checkEveryAddress(t, tableOf(t, ps), ps)
		if t.Failed() {
			t.Fatalf("prefixes %v", ps)
		}
	}
}

// The post-check refuses what lookup's binary search cannot read: the range
// the pre-flight review's merge made, starts out of order, an overlap.
// Neighbours of one country that did not merge are still in order.
func TestRangesOutOfOrderAreRefused(t *testing.T) {
	for name, rs := range map[string]countryRanges{
		"start after end": {start: []uint64{0, 128}, end: []uint64{127, 63}, cc: []uint8{1, 2}},
		"starts unsorted": {start: []uint64{64, 0}, end: []uint64{127, 63}, cc: []uint8{1, 2}},
		"overlap":         {start: []uint64{0, 64}, end: []uint64{127, 255}, cc: []uint8{1, 2}},
	} {
		if err := rs.check(); !errors.Is(err, errCountryArchiveUnsorted) {
			t.Errorf("%s: check() = %v, want errCountryArchiveUnsorted", name, err)
		}
	}
	ok := countryRanges{start: []uint64{0, 64, 200}, end: []uint64{63, 127, 200}, cc: []uint8{1, 1, 2}}
	if err := ok.check(); err != nil {
		t.Errorf("sorted ranges: %v", err)
	}
}

// The prefix bound: files within countryFileMax repeating one prefix are
// read up to countryArchiveMaxPrefixes and refused one past it.
func TestAnArchiveOfTooManyPrefixesIsRefused(t *testing.T) {
	const perFile = 130_000 // 1.95 MB of JSON
	for n, want := range map[int]error{countryArchiveMaxPrefixes: nil, countryArchiveMaxPrefixes + 1: errCountryArchiveTooLong} {
		var files [][2]string
		for i, cc := range []string{"de", "fr", "ie", "ru", "sg"} {
			k := min(perFile, n-i*perFile)
			list := strings.TrimSuffix(strings.Repeat(`"192.0.2.0/24",`, k), ",")
			files = append(files, [2]string{"country/" + cc + "/aggregated.json", aggregated(list, ``)})
		}
		if _, err := parseCountryArchive(bytes.NewReader(countryArchive(t, files...))); !errors.Is(err, want) {
			t.Errorf("%d prefixes: err = %v, want %v", n, err, want)
		}
	}
}

// BenchmarkCountryLookup: a synthetic table the size of ipverse's on
// 2026-10-14 — 143 265 IPv4 and 69 135 IPv6 ranges — and random addresses of
// both families, most of them in no range or between two.
func BenchmarkCountryLookup(b *testing.B) {
	r := rand.New(rand.NewPCG(7, 8))
	synth := func(n int, width uint) []countryPrefix {
		ps := make([]countryPrefix, n)
		step := uint64(1) << width / uint64(n)
		for i := range ps {
			s := uint64(i) * step
			ps[i] = countryPrefix{start: s, end: s + step/2, bits: 24, cc: uint8(r.IntN(len(shared.CountryCodes)))} // #nosec G115 -- < 256
		}
		return ps
	}
	tab := &countryTable{v4: buildCountryRanges(synth(143265, 32)), v6: buildCountryRanges(synth(69135, 64))}
	addrs := make([]netip.Addr, 1<<16)
	for i := range addrs {
		if i%2 == 0 {
			addrs[i] = netip.AddrFrom4([4]byte{byte(1 + r.IntN(222)), byte(r.IntN(256)), byte(r.IntN(256)), byte(r.IntN(256))})
		} else {
			var a [16]byte
			for j := range 8 {
				a[j] = byte(r.IntN(256))
			}
			a[0] = 0x20 | a[0]&0x1f // 2000::/3, global unicast
			addrs[i] = netip.AddrFrom16(a)
		}
	}
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		tab.lookup(addrs[i&(len(addrs)-1)])
	}
}

// The last IPv6 key and a tie. US holds ffff::/16 and the last /64 of the key
// space, FR the same /64; on IPv4 US and DE both hold 198.51.100.0/24. The
// range ending at ^uint64(0) must not wrap the sweep back to 0, and the same
// prefix under two countries goes to the higher CountryCodes index: US. US is
// listed first, so the archive's order alone would give the tie to DE and FR.
func TestTheLastKeyAndATieBuildExactly(t *testing.T) {
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t,
		[2]string{"country/us/aggregated.json", aggregated(`"198.51.100.0/24"`, `"ffff::/16","ffff:ffff:ffff:ffff::/64"`)},
		[2]string{"country/de/aggregated.json", aggregated(`"198.51.100.0/24"`, ``)},
		[2]string{"country/fr/aggregated.json", aggregated(``, `"ffff:ffff:ffff:ffff::/64"`)},
	)))
	if err != nil {
		t.Fatalf("parseCountryArchive: %v", err)
	}
	if de, us := slices.Index(shared.CountryCodes, "de"), slices.Index(shared.CountryCodes, "us"); de >= us || us > 255 {
		t.Fatalf("the fixture needs us after de in CountryCodes: %d, %d", de, us)
	}
	us := uint8(slices.Index(shared.CountryCodes, "us")) // #nosec G115 -- checked above
	for name, c := range map[string]struct{ got, want countryRanges }{
		"v4": {tab.v4, countryRanges{start: []uint64{0xc6336400}, end: []uint64{0xc63364ff}, cc: []uint8{us}}},
		"v6": {tab.v6, countryRanges{start: []uint64{0xffff << 48}, end: []uint64{^uint64(0)}, cc: []uint8{us}}},
	} {
		if !slices.Equal(c.got.start, c.want.start) || !slices.Equal(c.got.end, c.want.end) || !slices.Equal(c.got.cc, c.want.cc) {
			t.Errorf("%s = %x %x %v, want %x %x %v", name, c.got.start, c.got.end, c.got.cc, c.want.start, c.want.end, c.want.cc)
		}
	}
}

// parseEntry, not netip.ParsePrefix: an unmasked prefix is masked, and an
// IPv4 prefix written in IPv6 lands in the IPv4 table.
func TestPrefixesAreReadAsTheFeedsReadThem(t *testing.T) {
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t,
		[2]string{"country/de/aggregated.json", aggregated(`"192.0.2.77/24"`, `"::ffff:198.51.100.0/120"`)},
	)))
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{"192.0.2.5": "de", "198.51.100.1": "de"} {
		if got := tab.lookup(netip.MustParseAddr(addr)); got != want {
			t.Errorf("lookup(%s) = %q, want %q", addr, got, want)
		}
	}
}

// A symlink named like a country's file is skipped, not followed or decoded.
func TestASymlinkInTheArchiveIsSkipped(t *testing.T) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: "country/fr/aggregated.json", Linkname: "../de/aggregated.json", Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	for _, f := range sampleCountryFiles {
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0o644, Size: int64(len(f[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	tab, err := parseCountryArchive(bytes.NewReader(gzipped(t, b.Bytes())))
	if err != nil {
		t.Fatalf("parseCountryArchive: %v", err)
	}
	if tab.countries != 3 {
		t.Errorf("countries = %d, want 3", tab.countries)
	}
}

// One country's file larger than countryFileMax is refused, even when it is
// valid JSON padded with spaces.
func TestACountryFileLargerThanItMayBeIsRefused(t *testing.T) {
	doc := aggregated(`"192.0.2.0/24"`, ``)
	for size, want := range map[int]error{countryFileMax: nil, countryFileMax + 1: errCountryArchiveTooLong} {
		body := doc + strings.Repeat(" ", size-len(doc))
		if _, err := parseCountryArchive(bytes.NewReader(countryArchive(t, [2]string{"country/de/aggregated.json", body}))); !errors.Is(err, want) {
			t.Errorf("%d bytes: err = %v, want %v", size, err, want)
		}
	}
}

// A prefix broader than a country's breadth (2.26: /4 and /16) is skipped:
// it would label every unassigned address it covers. The US's broadest,
// 16.0.0.0/6 and 2630::/16 (2026-10-14), are read.
func TestAPrefixBroaderThanACountryIsSkipped(t *testing.T) {
	tab, err := parseCountryArchive(bytes.NewReader(countryArchive(t,
		[2]string{"country/us/aggregated.json", aggregated(`"0.0.0.0/3","16.0.0.0/6"`, `"2000::/15","2630::/16"`)},
	)))
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{
		"1.1.1.1": "", "17.0.0.1": "us", "2001:db8::1": "", "2630::1": "us",
	} {
		if got := tab.lookup(netip.MustParseAddr(addr)); got != want {
			t.Errorf("lookup(%s) = %q, want %q", addr, got, want)
		}
	}
}
