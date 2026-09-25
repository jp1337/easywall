package core

import (
	"encoding/binary"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nflog "github.com/florianl/go-nflog/v2"
	"github.com/jp1337/easywall/internal/shared"
)

// ipv4 builds a 20-byte IPv4 header followed by l4.
func ipv4(proto byte, src, dst string, fragOff uint16, l4 []byte) []byte {
	b := make([]byte, 20, 20+len(l4))
	b[0] = 0x45 // version 4, IHL 5
	binary.BigEndian.PutUint16(b[2:4], uint16(20+len(l4)))
	binary.BigEndian.PutUint16(b[6:8], fragOff&0x1fff)
	b[8] = 57 // TTL
	b[9] = proto
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(b[12:16], s[:])
	copy(b[16:20], d[:])
	return append(b, l4...)
}

// ipv6 builds a 40-byte IPv6 header with next header nh, followed by rest.
func ipv6(nh byte, src, dst string, rest []byte) []byte {
	b := make([]byte, 40, 40+len(rest))
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:6], uint16(len(rest)))
	b[6] = nh
	b[7] = 64 // hop limit
	s, d := netip.MustParseAddr(src).As16(), netip.MustParseAddr(dst).As16()
	copy(b[8:24], s[:])
	copy(b[24:40], d[:])
	return append(b, rest...)
}

func tcp(sport, dport uint16, flags byte) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b[0:2], sport)
	binary.BigEndian.PutUint16(b[2:4], dport)
	b[12] = 5 << 4
	b[13] = flags
	return b
}

func udp(sport, dport uint16) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint16(b[0:2], sport)
	binary.BigEndian.PutUint16(b[2:4], dport)
	return b
}

func attr(prefix string, payload []byte) nflog.Attribute {
	return nflog.Attribute{Prefix: &prefix, Payload: &payload}
}

func noNames(uint32) string { return "" }

func TestDecode_IPv4TCP(t *testing.T) {
	e, ok := entryFromAttribute(attr("easywall drop: ",
		ipv4(6, "203.0.113.9", "198.51.100.1", 0, tcp(51514, 22, 0x02))), noNames, time.Unix(0, 0))
	if !ok {
		t.Fatal("a whole IPv4/TCP header was discarded")
	}
	want := shared.PacketLogEntry{
		Time: time.Unix(0, 0).UTC(), Rule: "drop", Family: 4,
		Src: netip.MustParseAddr("203.0.113.9"), Dst: netip.MustParseAddr("198.51.100.1"),
		Proto: "tcp", SrcPort: 51514, DstPort: 22, TCPFlags: "SYN", TTL: 57, Length: 40,
	}
	if e != want {
		t.Errorf("decoded\n  %+v\nwant\n  %+v", e, want)
	}
}

// 2.23: a feed's rule names the feed after the stem, and the entry carries it.
func TestDecode_AFeedRowNamesTheFeed(t *testing.T) {
	e, ok := entryFromAttribute(attr(shared.FeedLogPrefix("spamhaus-drop")+"\x00",
		ipv4(6, "203.0.113.9", "198.51.100.1", 0, tcp(51514, 22, 0x02))), noNames, time.Now())
	if !ok || e.Rule != "feed" || e.Feed != "spamhaus-drop" {
		t.Errorf("got rule %q feed %q (ok=%v)", e.Rule, e.Feed, ok)
	}
}

func TestDecode_IPv6UDP(t *testing.T) {
	e, ok := entryFromAttribute(attr("easywall bogon: ",
		ipv6(17, "2001:db8::9", "2001:db8::1", udp(5353, 53))), noNames, time.Now())
	if !ok || e.Family != 6 || e.Proto != "udp" || e.DstPort != 53 || e.TTL != 64 ||
		e.Src != netip.MustParseAddr("2001:db8::9") || e.Rule != "bogon" {
		t.Errorf("got %+v (ok=%v)", e, ok)
	}
}

// An extension header between the IPv6 header and TCP is walked, not mistaken
// for the transport header.
func TestDecode_IPv6WalksAnExtensionHeader(t *testing.T) {
	hop := make([]byte, 8)
	hop[0] = 6 // next: TCP
	e, ok := entryFromAttribute(attr("easywall drop: ",
		ipv6(0, "2001:db8::9", "2001:db8::1", append(hop, tcp(40000, 443, 0x12)...))), noNames, time.Now())
	if !ok || e.Proto != "tcp" || e.DstPort != 443 || e.TCPFlags != "SYN,ACK" {
		t.Errorf("got %+v (ok=%v)", e, ok)
	}
}

func TestDecode_ICMPHasNoPorts(t *testing.T) {
	e, ok := entryFromAttribute(attr("easywall icmp-flood: ",
		ipv4(1, "203.0.113.9", "198.51.100.1", 0, []byte{8, 0, 0, 0})), noNames, time.Now())
	if !ok || e.Proto != "icmp" || e.DstPort != 0 || e.SrcPort != 0 {
		t.Errorf("got %+v (ok=%v)", e, ok)
	}
}

// A later fragment carries no transport header; the bytes after the IP header
// are the middle of somebody's payload, and reading ports out of them would
// invent a port the page then offers to open.
func TestDecode_LaterFragmentHasNoPorts(t *testing.T) {
	e, ok := entryFromAttribute(attr("easywall fragment: ",
		ipv4(6, "203.0.113.9", "198.51.100.1", 185, tcp(1, 2, 0x02))), noNames, time.Now())
	if !ok {
		t.Fatal("a later fragment is a real packet and must be shown")
	}
	if e.DstPort != 0 || e.SrcPort != 0 || e.TCPFlags != "" {
		t.Errorf("ports were read out of a fragment's payload: %+v", e)
	}
	if e.Proto != "tcp" {
		t.Errorf("proto = %q, want tcp — the header still says what it is", e.Proto)
	}
}

// Spec §8: a payload shorter than its own header is discarded and counted,
// never rendered half-decoded.
func TestDecode_TruncatedIsDiscarded(t *testing.T) {
	whole := ipv4(6, "203.0.113.9", "198.51.100.1", 0, tcp(1, 22, 0x02))
	for name, b := range map[string][]byte{
		"no bytes":             {},
		"half an IPv4 header":  whole[:12],
		"IHL past the end":     append([]byte{0x4f}, whole[1:20]...),
		"half a TCP header":    whole[:30],
		"half an IPv6 header":  ipv6(6, "2001:db8::1", "2001:db8::2", nil)[:30],
		"not IP at all":        {0x00, 0x01, 0x02},
		"a one-byte ICMP body": ipv4(1, "203.0.113.9", "198.51.100.1", 0, []byte{8}),
	} {
		if e, ok := entryFromAttribute(attr("easywall drop: ", b), noNames, time.Now()); ok {
			t.Errorf("%s: decoded to %+v", name, e)
		}
	}
	if _, ok := entryFromAttribute(nflog.Attribute{}, noNames, time.Now()); ok {
		t.Error("an attribute with no payload was decoded")
	}
}

func TestDecode_MetadataFromTheAttribute(t *testing.T) {
	a := attr("easywall ssh: ", ipv4(6, "203.0.113.9", "198.51.100.1", 0, tcp(1, 22, 0x02)))
	idx, hook, ct, mark := uint32(3), uint8(1), uint32(2), uint32(4242)
	a.InDev, a.Hook, a.CtInfo, a.Mark = &idx, &hook, &ct, &mark
	e, _ := entryFromAttribute(a, func(i uint32) string {
		if i == 3 {
			return "eth0"
		}
		return ""
	}, time.Now())
	if e.InDev != "eth0" || e.Hook != "input" || e.CtState != "new" || e.Rule != "ssh" || e.Mark != 4242 {
		t.Errorf("got %+v", e)
	}
}

func entry(src string, port uint16) shared.PacketLogEntry {
	return shared.PacketLogEntry{Rule: "drop", Family: 4, Proto: "tcp",
		Src: netip.MustParseAddr(src), Dst: netip.MustParseAddr("198.51.100.1"), DstPort: port}
}

func TestRing_NewestFirstAndWraps(t *testing.T) {
	p := NewPacketLog(3)
	for i := range 5 {
		p.Add(entry("203.0.113.9", uint16(1000+i)))
	}
	res, err := p.Query(shared.PacketLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Held != 3 || res.Matched != 3 || len(res.Entries) != 3 {
		t.Fatalf("held %d matched %d shown %d, want 3/3/3", res.Held, res.Matched, len(res.Entries))
	}
	for i, want := range []uint16{1004, 1003, 1002} {
		if res.Entries[i].DstPort != want {
			t.Errorf("entry %d is port %d, want %d — newest first, oldest overwritten",
				i, res.Entries[i].DstPort, want)
		}
	}
	if res.Entries[0].Seq != 5 {
		t.Errorf("seq = %d, want 5", res.Entries[0].Seq)
	}
}

func TestRing_FilterAndLimit(t *testing.T) {
	p := NewPacketLog(100)
	for i := range 10 {
		p.Add(entry("203.0.113.9", 22))
		p.Add(entry("192.0.2.7", uint16(i+1)))
	}
	res, _ := p.Query(shared.PacketLogFilter{Src: "203.0.113.0/24", Limit: 4})
	if res.Matched != 10 || len(res.Entries) != 4 {
		t.Errorf("matched %d shown %d, want 10 and 4", res.Matched, len(res.Entries))
	}
	if _, err := p.Query(shared.PacketLogFilter{Src: "nonsense"}); err == nil {
		t.Error("the core accepted a filter the web process should never have sent")
	}
}

// An empty ring answers [] and not null: the page ranges over it.
func TestRing_EmptyIsAnEmptyList(t *testing.T) {
	res, _ := NewPacketLog(10).Query(shared.PacketLogFilter{})
	if res.Entries == nil {
		t.Error("Entries is nil; JSON would carry null")
	}
}

func TestPersist_SurvivesARestartIntoASmallerRing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packets.log")
	a := NewPacketLog(1000)
	if err := a.Persist(path); err != nil {
		t.Fatal(err)
	}
	for i := range 50 {
		a.Add(entry("203.0.113.9", uint16(i+1)))
	}

	b := NewPacketLog(10) // entries lowered between the two starts
	if err := b.Persist(path); err != nil {
		t.Fatal(err)
	}
	res, _ := b.Query(shared.PacketLogFilter{})
	if res.Held != 10 || res.Entries[0].DstPort != 50 || res.Entries[9].DstPort != 41 {
		t.Errorf("after replay: held %d, newest %d, oldest %d — want the newest ten, 50..41",
			res.Held, res.Entries[0].DstPort, res.Entries[res.Held-1].DstPort)
	}
	// Seq continues rather than restarting at 1: the page keys rows on it.
	b.Add(entry("203.0.113.9", 999))
	res, _ = b.Query(shared.PacketLogFilter{Limit: 1})
	if res.Entries[0].Seq != 51 {
		t.Errorf("seq after replay = %d, want 51", res.Entries[0].Seq)
	}
	if !res.Persisted {
		t.Error("Persisted is false on a log that is writing its file")
	}
}

// Spec §4: compacted in place once it holds more than twice the ring. Never
// depends on logrotate — daemon.go already says of the audit log that a manual
// install may not have it.
func TestPersist_CompactsAtTwiceTheRing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packets.log")
	p := NewPacketLog(10)
	if err := p.Persist(path); err != nil {
		t.Fatal(err)
	}
	for i := range 21 {
		p.Add(entry("203.0.113.9", uint16(i+1)))
	}
	if n := countLines(t, path); n != 10 {
		t.Errorf("the file holds %d lines after the 21st entry, want 10 — compacted to the ring", n)
	}
	for i := range 5 {
		p.Add(entry("203.0.113.9", uint16(100+i)))
	}
	if n := countLines(t, path); n != 15 {
		t.Errorf("the file holds %d lines, want 15 — appending again after compaction", n)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("packets.log is %v, want 0600 — it holds IP addresses", info.Mode().Perm())
	}
}

// Review Focus 4: a power cut mid-write leaves half a line.
func TestReplaySkipsATornLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packets.log")
	good, _ := json.Marshal(shared.PacketLogEntry{Seq: 7, Rule: "drop", Proto: "tcp",
		Src: netip.MustParseAddr("203.0.113.9"), DstPort: 22})
	if err := os.WriteFile(path, append(append(good, '\n'), []byte(`{"seq":8,"rule":"dr`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewPacketLog(10)
	if err := p.Persist(path); err != nil {
		t.Fatalf("a torn line stopped the replay: %v", err)
	}
	res, _ := p.Query(shared.PacketLogFilter{})
	if res.Held != 1 || res.Entries[0].Seq != 7 {
		t.Errorf("held %d, want the one good entry", res.Held)
	}
	if res.Discarded != 1 {
		t.Errorf("discarded = %d, want 1 — a line skipped silently is a line nobody knows about", res.Discarded)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

// Spec §8: a payload too short to decode is discarded and counted, not merely
// skipped. entryFromAttribute reports ok=false; hook is what NFLOG actually
// calls, and it is hook's job to turn that into a count the page can show.
func TestHookCountsWhatItCannotDecode(t *testing.T) {
	p := NewPacketLog(10)
	p.hook(attr("easywall drop: ", []byte{0x45})) // a version/IHL byte, nothing after it
	res, _ := p.Query(shared.PacketLogFilter{})
	if res.Discarded != 1 || res.Held != 0 {
		t.Errorf("discarded=%d held=%d, want 1 and 0", res.Discarded, res.Held)
	}
}

// 2.23 G3: an ICMP row can say it was a ping only if the entry carries the
// type. Read from the first two bytes of the ICMP header, for both families,
// and only when the protocol belongs to the family.
func TestDecode_ICMPRecordsTypeAndCode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		want    *shared.PacketICMP
	}{
		{"IPv4 echo request", ipv4(1, "203.0.113.9", "198.51.100.1", 0, []byte{8, 0, 0, 0}), &shared.PacketICMP{Type: 8}},
		{"IPv4 unreachable, host", ipv4(1, "203.0.113.9", "198.51.100.1", 0, []byte{3, 1, 0, 0}), &shared.PacketICMP{Type: 3, Code: 1}},
		{"IPv4 echo reply is type 0, not absent", ipv4(1, "203.0.113.9", "198.51.100.1", 0, []byte{0, 0, 0, 0}), &shared.PacketICMP{}},
		{"ICMPv6 echo request", ipv6(58, "2001:db8::9", "2001:db8::1", []byte{128, 0, 0, 0}), &shared.PacketICMP{Type: 128}},
		{"ICMPv6 router solicitation", ipv6(58, "2001:db8::9", "ff02::2", []byte{133, 0, 0, 0}), &shared.PacketICMP{Type: 133}},
		{"TCP carries none", ipv4(6, "203.0.113.9", "198.51.100.1", 0, tcp(1, 22, 0x02)), nil},
		{"ICMPv6 inside IPv4 is not read", ipv4(58, "203.0.113.9", "198.51.100.1", 0, []byte{128, 0, 0, 0}), nil},
		{"ICMPv4 inside IPv6 is not read", ipv6(1, "2001:db8::9", "2001:db8::1", []byte{8, 0, 0, 0}), nil},
		{"a later fragment carries none", ipv4(1, "203.0.113.9", "198.51.100.1", 185, []byte{8, 0, 0, 0}), nil},
	} {
		e, ok := entryFromAttribute(attr("easywall drop: ", tc.payload), noNames, time.Now())
		if !ok {
			t.Errorf("%s: discarded", tc.name)
			continue
		}
		switch {
		case tc.want == nil && e.ICMP != nil:
			t.Errorf("%s: ICMP = %+v, want none", tc.name, *e.ICMP)
		case tc.want != nil && (e.ICMP == nil || *e.ICMP != *tc.want):
			t.Errorf("%s: ICMP = %+v, want %+v", tc.name, e.ICMP, *tc.want)
		}
	}
}

// The spill file outlives an upgrade. A line a 2.21 core wrote has no icmp
// field and must replay as "not recorded", and an entry without one must not
// grow the key — a jq filter written against 2.21's lines keeps matching.
func TestReplayReadsLinesWithoutTheICMPField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packets.log")
	old := `{"seq":3,"time":"2026-09-20T10:00:00Z","rule":"drop","family":4,"src":"203.0.113.9","dst":"198.51.100.1","proto":"icmp","ttl":57,"len":84}`
	if err := os.WriteFile(path, []byte(old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewPacketLog(10)
	if err := p.Persist(path); err != nil {
		t.Fatal(err)
	}
	p.Add(shared.PacketLogEntry{Rule: "drop", Family: 4, Proto: "icmp",
		Src: netip.MustParseAddr("203.0.113.9"), ICMP: &shared.PacketICMP{Type: 13}})
	res, _ := p.Query(shared.PacketLogFilter{})
	if res.Discarded != 0 || len(res.Entries) != 2 {
		t.Fatalf("held %d, discarded %d — the 2.21 line must replay", len(res.Entries), res.Discarded)
	}
	if res.Entries[1].ICMP != nil {
		t.Errorf("the 2.21 line replayed with ICMP %+v; it recorded none", *res.Entries[1].ICMP)
	}
	if got := res.Entries[0].ICMP; got == nil || got.Type != 13 {
		t.Errorf("the new entry lost its type across the spill file: %+v", got)
	}
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if strings.Contains(lines[0], `"icmp":`) {
		t.Errorf("an entry with no ICMP header was written with the key: %s", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], `"icmp":{"type":13,"code":0}`) {
		t.Errorf("the ICMP type is not in the file: %s", lines[len(lines)-1])
	}
}
