package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	nflog "github.com/florianl/go-nflog/v2"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// The packet log: what the ten log rules refused, read from NFLOG as binary
// headers of fixed structure, never from a line of text. See
// docs-tech/specs/2026-09-22-2.21-you-can-see-what-it-refuses.md §2 for why the
// kernel log cannot be the source.

// packetSnaplen is how much of each packet the log rule copies to NFLOG: an
// IPv6 header, a short extension chain and a TCP header with options fit.
const packetSnaplen = 128

// entryFromAttribute turns one NFLOG message into an entry, or reports that the
// payload was too short to decode — which the caller counts, never shows.
func entryFromAttribute(a nflog.Attribute, ifname func(uint32) string, now time.Time) (shared.PacketLogEntry, bool) {
	var e shared.PacketLogEntry
	if a.Payload == nil {
		return e, false
	}
	e.Rule = shared.PacketLogRuleOther
	if a.Prefix != nil {
		e.Rule = shared.RuleFromPrefix(*a.Prefix)
	}
	// NFULA_TIMESTAMP is present only when the skb carried one; most do not.
	e.Time = now.UTC()
	if a.Timestamp != nil {
		e.Time = a.Timestamp.UTC()
	}
	if a.InDev != nil {
		e.InDev = ifname(*a.InDev)
	}
	if a.Hook != nil {
		e.Hook = hookName(*a.Hook)
	}
	if a.Mark != nil {
		e.Mark = *a.Mark
	}
	e.CtState = ctState(a.CtInfo)
	if !decodeHeaders(&e, *a.Payload) {
		return shared.PacketLogEntry{}, false
	}
	return e, true
}

// Attribute.Hook is *uint8 in go-nflog v2.3.0 (types.go) — the byte after
// hw_protocol in NFULA_PACKET_HDR.
func hookName(h uint8) string {
	switch h {
	case unix.NF_INET_LOCAL_IN:
		return "input"
	case unix.NF_INET_FORWARD:
		return "forward"
	}
	return ""
}

// ctState names enum ip_conntrack_info. Absent means the packet had no
// conntrack entry at all — which is what an INVALID drop looks like.
func ctState(info *uint32) string {
	if info == nil {
		return ""
	}
	switch *info {
	case 0, 3: // IP_CT_ESTABLISHED, IP_CT_ESTABLISHED_REPLY
		return "established"
	case 1, 4: // IP_CT_RELATED, IP_CT_RELATED_REPLY
		return "related"
	case 2: // IP_CT_NEW
		return "new"
	case 7: // IP_CT_UNTRACKED
		return "untracked"
	}
	return ""
}

func decodeHeaders(e *shared.PacketLogEntry, b []byte) bool {
	if len(b) == 0 {
		return false
	}
	switch b[0] >> 4 {
	case 4:
		return decodeIPv4(e, b)
	case 6:
		return decodeIPv6(e, b)
	}
	return false
}

func decodeIPv4(e *shared.PacketLogEntry, b []byte) bool {
	if len(b) < 20 {
		return false
	}
	ihl := int(b[0]&0x0f) * 4
	if ihl < 20 || len(b) < ihl {
		return false
	}
	e.Family = 4
	e.Length = binary.BigEndian.Uint16(b[2:4])
	e.TTL = b[8]
	e.Src = netip.AddrFrom4([4]byte(b[12:16]))
	e.Dst = netip.AddrFrom4([4]byte(b[16:20]))
	if binary.BigEndian.Uint16(b[6:8])&0x1fff != 0 {
		e.Proto = ipProtoName(b[9], 4) // a later fragment: no transport header here
		return true
	}
	return decodeTransport(e, b[9], 4, b[ihl:])
}

func decodeIPv6(e *shared.PacketLogEntry, b []byte) bool {
	if len(b) < 40 {
		return false
	}
	e.Family = 6
	e.Length = uint16(min(40+int(binary.BigEndian.Uint16(b[4:6])), 65535)) // #nosec G115 -- clamped on this line
	e.TTL = b[7]
	e.Src = netip.AddrFrom16([16]byte(b[8:24]))
	e.Dst = netip.AddrFrom16([16]byte(b[24:40]))
	next, rest := b[6], b[40:]
	// Bounded: eight extension headers is more than any real packet carries,
	// and a loop an attacker's packet could steer must end.
	for range 8 {
		switch next {
		case 0, 43, 60: // hop-by-hop, routing, destination options
			if len(rest) < 8 {
				return false
			}
			n := (int(rest[1]) + 1) * 8
			if len(rest) < n {
				return false
			}
			next, rest = rest[0], rest[n:]
		case 44: // fragment
			if len(rest) < 8 {
				return false
			}
			later := binary.BigEndian.Uint16(rest[2:4])>>3 != 0
			next, rest = rest[0], rest[8:]
			if later {
				e.Proto = ipProtoName(next, 6)
				return true
			}
		default:
			return decodeTransport(e, next, 6, rest)
		}
	}
	return false
}

func decodeTransport(e *shared.PacketLogEntry, proto byte, family int, b []byte) bool {
	e.Proto = ipProtoName(proto, family)
	switch proto {
	case unix.IPPROTO_TCP:
		if len(b) < 20 {
			return false
		}
		e.SrcPort = binary.BigEndian.Uint16(b[0:2])
		e.DstPort = binary.BigEndian.Uint16(b[2:4])
		e.TCPFlags = tcpFlags(b[13])
	case unix.IPPROTO_UDP:
		if len(b) < 8 {
			return false
		}
		e.SrcPort = binary.BigEndian.Uint16(b[0:2])
		e.DstPort = binary.BigEndian.Uint16(b[2:4])
	case unix.IPPROTO_ICMP, unix.IPPROTO_ICMPV6:
		if len(b) < 2 {
			return false
		}
		// Only when the protocol belongs to the family: ICMPv6 inside IPv4 is
		// named by its number above, and its bytes are not a type this page
		// can read.
		if e.Proto == "icmp" || e.Proto == "icmpv6" {
			e.ICMP = &shared.PacketICMP{Type: b[0], Code: b[1]}
		}
	}
	return true
}

// Named ipProtoName, not protoName: docker.go already declares a
// package-level protoName(n byte) string, and the two would collide.
func ipProtoName(p byte, family int) string {
	switch {
	case p == unix.IPPROTO_TCP:
		return "tcp"
	case p == unix.IPPROTO_UDP:
		return "udp"
	case p == unix.IPPROTO_ICMP && family == 4:
		return "icmp"
	case p == unix.IPPROTO_ICMPV6 && family == 6:
		return "icmpv6"
	}
	return strconv.Itoa(int(p))
}

var tcpFlagNames = [8]string{"FIN", "SYN", "RST", "PSH", "ACK", "URG", "ECE", "CWR"}

func tcpFlags(b byte) string {
	var out []string
	for i, name := range tcpFlagNames {
		if b&(1<<i) != 0 {
			out = append(out, name)
		}
	}
	return strings.Join(out, ",")
}

// PacketLog holds the most recent refused packets, newest first on the way
// out. It is the only read path; the spill file exists to be replayed into it.
type PacketLog struct {
	mu        sync.Mutex
	buf       []shared.PacketLogEntry
	next      int // where the next entry is written
	n         int // how many are held
	seq       uint64
	since     time.Time
	discarded uint64
	lost      uint64

	listening bool
	// everListened is sticky: once a bind has succeeded it stays true, which is
	// how setListening (and Query, from it) tells a bind that never succeeded
	// apart from a listener that ran and then died — the two states the page
	// must not describe with the same sentence.
	everListened bool
	group        uint16
	reason       string

	spill *spillFile // nil when persist is off, or after the file failed

	namesMu sync.Mutex
	names   map[uint32]string
	namesAt time.Time
}

// NewPacketLog makes a ring of the given capacity.
func NewPacketLog(capacity int) *PacketLog {
	return &PacketLog{buf: make([]shared.PacketLogEntry, max(capacity, 1)), since: time.Now().UTC()}
}

// Add numbers e and stores it, and appends it to the file when there is one.
// ponytail: spill.append holds p.mu through its own rewrite, which fires
// once the file grows past 2×entries lines and rewrites it down to the ring's
// held entries — at the 200000 maximum, up to 200,000 lines written, in one
// call. That stalls Query (the page's 5-second poll) and the receive
// goroutine (→ ENOBUFS → Lost) for the duration. Bounded and rare (once per
// `entries` packets written); move the rewrite off p.mu (snapshot
// oldestFirst, rewrite outside the lock, swap the file in) if that stall ever
// measures.
func (p *PacketLog) Add(e shared.PacketLogEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	e.Seq = p.seq
	p.push(e)
	if p.spill != nil {
		if err := p.spill.append(e, p.oldestFirst); err != nil {
			// Once, not per packet: a full disk must not become a log flood.
			slog.Error("the packet log stopped writing its file; the page keeps "+
				"working and starts empty after the next restart", "path", p.spill.path, "error", err)
			p.spill.close()
			p.spill = nil
		}
	}
}

func (p *PacketLog) push(e shared.PacketLogEntry) {
	p.buf[p.next] = e
	p.next = (p.next + 1) % len(p.buf)
	if p.n < len(p.buf) {
		p.n++
	}
}

// at returns the i-th newest entry. Caller holds p.mu.
func (p *PacketLog) at(i int) shared.PacketLogEntry {
	l := len(p.buf)
	return p.buf[((p.next-1-i)%l+l)%l]
}

// oldestFirst is what a compaction writes. Caller holds p.mu.
func (p *PacketLog) oldestFirst() []shared.PacketLogEntry {
	out := make([]shared.PacketLogEntry, 0, p.n)
	for i := p.n - 1; i >= 0; i-- {
		out = append(out, p.at(i))
	}
	return out
}

// Query answers GET_PACKET_LOG. Linear over the ring: twenty thousand struct
// comparisons per five-second poll is nothing.
func (p *PacketLog) Query(f shared.PacketLogFilter) (shared.PacketLogResult, error) {
	match, err := f.Matcher()
	if err != nil {
		return shared.PacketLogResult{}, err
	}
	limit := f.EffectiveLimit()

	p.mu.Lock()
	defer p.mu.Unlock()
	res := shared.PacketLogResult{
		Entries: []shared.PacketLogEntry{}, Held: p.n,
		Listening: p.listening, Stopped: !p.listening && p.everListened,
		Group: p.group, Reason: p.reason, Since: p.since,
		Discarded: p.discarded, Lost: p.lost, Persisted: p.spill != nil,
	}
	for i := range p.n {
		e := p.at(i)
		if !match(e) {
			continue
		}
		res.Matched++
		if len(res.Entries) < limit {
			res.Entries = append(res.Entries, e)
		}
	}
	return res, nil
}

func (p *PacketLog) setListening(group uint16, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.group, p.listening, p.reason = group, err == nil, ""
	if err != nil {
		p.reason = err.Error()
	} else {
		p.everListened = true
	}
}

func (p *PacketLog) countDiscarded() { p.mu.Lock(); p.discarded++; p.mu.Unlock() }
func (p *PacketLog) countLost()      { p.mu.Lock(); p.lost++; p.mu.Unlock() }

// Persist replays path into the ring, rewrites it to exactly what the ring now
// holds, and appends every later entry to it. A file that does not exist yet
// is not an error.
func (p *PacketLog) Persist(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// #nosec G304 -- path is cfg.PacketLogPath(), built from log_dir
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var e shared.PacketLogEntry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				p.discarded++
				continue
			}
			p.push(e)
			p.seq = max(p.seq, e.Seq)
		}
		_ = f.Close()
		if err := sc.Err(); err != nil {
			return fmt.Errorf("replay %s: %w", path, err)
		}
		if p.n > 0 {
			p.since = p.at(p.n - 1).Time
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replay %s: %w", path, err)
	}

	s := &spillFile{path: path, limit: 2 * len(p.buf)}
	if err := s.rewrite(p.oldestFirst()); err != nil {
		return err
	}
	p.spill = s
	return nil
}

// spillFile is packets.log: one JSON entry per line, appended, and rewritten
// from the ring once it holds more than twice the ring's capacity. Rotating
// itself is the point — at a hundred times the audit log's volume, a missing
// logrotate is a full disk on the one machine that must not fall over.
type spillFile struct {
	path  string
	f     *os.File
	lines int
	limit int
}

func (s *spillFile) append(e shared.PacketLogEntry, ring func() []shared.PacketLogEntry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return err
	}
	s.lines++
	if s.lines > s.limit {
		return s.rewrite(ring())
	}
	return nil
}

// rewrite replaces the file with entries, atomically, and reopens it for
// appending.
func (s *spillFile) rewrite(entries []shared.PacketLogEntry) error {
	s.close()
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".packets-*.tmp")
	if err != nil {
		return fmt.Errorf("compact %s: %w", s.path, err)
	}
	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	// CreateTemp makes 0600 already; said out loud because the file holds addresses.
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	// #nosec G304 -- s.path is cfg.PacketLogPath()
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	s.f, s.lines = f, len(entries)
	return nil
}

func (s *spillFile) close() {
	if s.f != nil {
		_ = s.f.Close()
		s.f = nil
	}
}

// listen binds group and hands every packet to p. It returns once the bind
// has succeeded or failed, together with a stop func that closes the socket.
//
// stop is not a hard barrier. go-nflog v2.3.0 keeps its receive goroutine out
// of the WaitGroup that Close waits on, so hook may still run once after stop
// returns, and the kernel may hold the group for a moment until that goroutine
// has let go of the socket. A caller that rebinds the same group straight
// after stop can therefore meet -EPERM; the integration tests use a group
// each for that reason.
//
// Conntrack state first, and without it if the kernel refuses the flag — an
// entry without its ct state is worth more than no entry. Twice with it,
// because nfnetlink_log answers -EAGAIN when it has just loaded
// nf_conntrack_netlink on demand (nfulnl_recv_config), and the second try then
// succeeds.
//
// A group another program holds answers -EPERM, not -EBUSY: the portid check
// in nfulnl_recv_config runs before the bind. A process without CAP_NET_ADMIN
// gets the same errno, so the journal line in startPacketLog names both
// causes.
func (p *PacketLog) listen(group uint16) (stop func(), err error) {
	var lastErr error
	for _, flags := range []uint16{nflog.FlagConntrack, nflog.FlagConntrack, 0} {
		var nf *nflog.Nflog
		nf, lastErr = nflog.Open(&nflog.Config{Group: group, Copymode: nflog.CopyPacket, Flags: flags})
		if lastErr != nil {
			continue
		}
		// Ten modules at up to sixty lines a minute each, all bursting at once,
		// is six hundred messages in one breath; the default buffer is ~200 KB.
		_ = nf.Con.SetReadBuffer(1 << 20)

		ctx, cancel := context.WithCancel(context.Background())
		// A closure over this call's own ctx, not the p.onError method: cancelling
		// ctx (from stop, below) is how a clean shutdown surfaces here — go-nflog
		// turns it into a read error — and that must return quietly rather than
		// being logged as the listener having failed.
		onError := func(err error) int {
			if ctx.Err() != nil {
				return 1
			}
			return p.onError(err)
		}
		if lastErr = nf.RegisterWithErrorFunc(ctx, p.hook, onError); lastErr != nil {
			cancel()
			_ = nf.Close()
			continue
		}
		p.setListening(group, nil)
		return func() {
			cancel()
			_ = nf.Close() // closes the socket; see above for what it does not wait on
		}, nil
	}
	// Not wrapped with the group number: every caller already has it (Query's
	// Group field, and startPacketLog's own log line), and blocked_not_listening
	// prefixes it again — wrapping it here too rendered it three times.
	p.setListening(group, lastErr)
	return nil, lastErr
}

func (p *PacketLog) hook(a nflog.Attribute) int {
	e, ok := entryFromAttribute(a, p.ifname, time.Now())
	if !ok {
		p.countDiscarded()
		return 0
	}
	p.Add(e)
	return 0
}

// onError keeps reading through an overrun — the kernel dropped messages the
// socket could not take, which is counted and shown — and stops on anything
// else, which the page then reports as "not listening". listen's closure has
// already filtered out a clean shutdown before this is called.
func (p *PacketLog) onError(err error) int {
	// *netlink.OpError has Unwrap (mdlayher/netlink errors.go:113), so the
	// errno is reachable.
	if errors.Is(err, unix.ENOBUFS) {
		p.countLost()
		return 0
	}
	slog.Error("the packet log stopped reading its NFLOG group; the Blocked page "+
		"shows nothing new until easywall-core restarts", "error", err)
	p.mu.Lock()
	group := p.group
	p.mu.Unlock()
	p.setListening(group, err)
	return 1
}

// ifname resolves an interface index. Cached, because an index lookup is a full
// RTM_GETLINK dump and this runs per packet.
// ponytail: whole-map expiry once a minute, so a recycled index (a container's
// veth coming and going) is mislabelled for at most that long; per-entry
// invalidation on RTM_DELLINK if it ever matters.
func (p *PacketLog) ifname(idx uint32) string {
	p.namesMu.Lock()
	defer p.namesMu.Unlock()
	if p.names == nil || time.Since(p.namesAt) > time.Minute {
		p.names, p.namesAt = map[uint32]string{}, time.Now()
	}
	if name, ok := p.names[idx]; ok {
		return name
	}
	name := ""
	if iface, err := net.InterfaceByIndex(int(idx)); err == nil {
		name = iface.Name
	}
	p.names[idx] = name
	return name
}
