package shared

import (
	"encoding/json"
	"errors"
	"time"
)

// NftTimeout bounds every call the core makes to the nft binary.
//
// It lives here because the web process has to know it. Two commands run nft
// while the caller waits — VALIDATE_CUSTOM and IMPORT_RULES — so the client's
// deadline has to be longer than this one, or the client gives up on work the
// core then finishes. See CommandTimeout.
const NftTimeout = 30 * time.Second

// defaultCommandTimeout is how long the web process waits for a command that
// only touches files. Generous for a local socket and a few kilobytes of JSON.
const defaultCommandTimeout = 5 * time.Second

// CommandTimeout is how long easywall-web waits for a reply to cmd.
//
// One number for all fifteen commands was wrong, and it was wrong in the
// direction that loses data. IMPORT_RULES runs every custom rule past
// `nft --check` before storing anything, which the core bounds at NftTimeout —
// six times the five seconds the client allowed. Measured against a real socket
// with an nft that takes eight seconds:
//
//	POST /import      -> HTTP 303 after 5.007s
//	web log           -> import rules error: read response: i/o timeout
//	the operator sees -> the import failed
//	the audit log     -> rules_imported
//	staged custom     -> [] before, ["tcp dport 8443 accept"] after
//
// So the import succeeded, the staged rule set was replaced, and the interface
// said it had not been — and the obvious next move after "import failed" is to
// try again or to apply, on top of a rule set that is not the one on screen.
//
// The nft-backed commands, plus PANIC and RESUME, get NftTimeout plus room for
// the core's own work either side. PANIC can queue behind an apply's nft
// subprocess — it is designed to win rather than to fail fast — so the client
// must wait as long as the server might.
//
// RESUME used to be classified with the short deadline, on the reasoning that
// RestoreCurrent's beginApply() fails fast with ErrApplyInProgress rather than
// blocking. That stopped being true the moment Firewall.Panic and
// Firewall.Resume started sharing panicMu (see restore.go): RESUME now
// acquires that lock before it ever reaches beginApply, and a PANIC already
// queued behind the nft mutex — up to NftTimeout, per NftablesManager's own
// comment — holds panicMu for that whole window. A RESUME landing there blocks
// on the lock, not on the file operation the old reasoning was about, and a
// five-second deadline sitting in front of a thirty-second wait is exactly the
// failure IMPORT_RULES used to produce: the client gives up and reports a
// failure for work the core goes on to finish. RESUME now shares PANIC's
// deadline because it shares PANIC's queue.
//
// UPDATE_FEED joins them for PANIC's reason: it takes the nft mutex to replace
// a live set, so it can queue behind an apply's nft subprocess for up to
// NftTimeout, and it writes up to 100 000 elements while holding it.
//
// Everything else keeps the short deadline, because a status poll that hangs
// for half a minute is its own problem.
func CommandTimeout(cmd CommandType) time.Duration {
	switch cmd {
	case CmdImportRules, CmdValidateCustom, CmdPanic, CmdResume, CmdUpdateFeed:
		return NftTimeout + defaultCommandTimeout
	default:
		return defaultCommandTimeout
	}
}

// CommandType identifies which operation the core daemon should perform.
type CommandType string

const (
	CmdGetRules       CommandType = "GET_RULES"
	CmdSaveRules      CommandType = "SAVE_RULES"
	CmdApplyRules     CommandType = "APPLY_RULES"
	CmdAccept         CommandType = "ACCEPT"
	CmdGetStatus      CommandType = "GET_STATUS"
	CmdGetOptions     CommandType = "GET_OPTIONS"
	CmdSaveOptions    CommandType = "SAVE_OPTIONS"
	CmdGetSettings    CommandType = "GET_SETTINGS"
	CmdSaveSettings   CommandType = "SAVE_SETTINGS"
	CmdGetSystem      CommandType = "GET_SYSTEM"
	CmdSaveSystem     CommandType = "SAVE_SYSTEM"
	CmdGetLog         CommandType = "GET_LOG"
	CmdExportRules    CommandType = "EXPORT_RULES"
	CmdImportRules    CommandType = "IMPORT_RULES"
	CmdValidateCustom CommandType = "VALIDATE_CUSTOM"

	// CmdPanic tears the firewall down and records that it was deliberate;
	// CmdResume ends that and puts the stored rules back. Both are sent by the
	// `easywall-core` console subcommands rather than by the web process, so
	// that there is one writer to the table even while somebody is standing at
	// the machine — see internal/core/restore.go.
	CmdPanic  CommandType = "PANIC"
	CmdResume CommandType = "RESUME"

	// CmdGetAppliedConfig returns the configuration that went into the kernel
	// with the rules that are in it, and whether it was recorded at all.
	//
	// Read-only, one file, so it keeps the short deadline. It exists because
	// RulesState answers only half of "what changes": the options and the network
	// settings live in the core's config and take effect at the next apply, and
	// nothing on either side could see the difference between the two.
	CmdGetAppliedConfig CommandType = "GET_APPLIED_CONFIG"

	// CmdGetUsage returns what each port rule has carried and when it last
	// carried anything, keyed by rule id, with the time the figures were read.
	//
	// Read-only, one file, so it keeps the short deadline: the core answers it
	// out of usage.json and never touches netlink. Collecting here would queue
	// behind the nft mutex, which an apply holds for up to NftTimeout — six
	// times what this command's client is willing to wait.
	//
	// No audit entry. Reading a counter is not an event, which is the same
	// reasoning CmdGetStatus already runs on.
	CmdGetUsage CommandType = "GET_USAGE"

	// CmdGetHealth returns whether this firewall is doing what it says: three
	// facts, evaluated in order, plus the identity of the last self-test.
	//
	// It keeps the short deadline, and *not* because it is non-blocking. Both
	// of its netlink reads take the nft mutex — NftablesManager.Enforcing and
	// RuleCounters each open with m.mu.Lock() — and Apply holds that lock
	// across applyCustomRules' nft subprocess for up to NftTimeout. The
	// deadline is short anyway, for the consumer: Dockerfile's HEALTHCHECK is
	// --timeout=5s, so docker abandons the probe at five seconds whatever this
	// says, and a longer deadline would only make every other caller wait.
	//
	// The honest scope: during a slow custom-rules apply, /healthz answers 503
	// and the image's --interval=10s --retries=3 reaches its third failure
	// inside that window. Nothing restarts on unhealthy, so the cost is a
	// wrong word in `docker ps` — see features/health.md, which says so to
	// operators. CmdGetUsage avoids this by never touching netlink at all;
	// GET_HEALTH cannot, because whether the kernel is enforcing is the
	// question it exists to answer. Closing it properly needs a second source
	// of truth — a cached snapshot the way Server.statusForRender does it —
	// which is a decision about what GET_HEALTH is, not a fix to a wrong line.
	//
	// No audit entry: reading a counter is not an event, which is
	// CmdGetStatus's and CmdGetUsage's reasoning already.
	//
	// The reply carries no rule detail and no counter values. It is rendered by
	// /healthz, which is unauthenticated so that an orchestrator holding no
	// session can ask.
	CmdGetHealth CommandType = "GET_HEALTH"

	// CmdLogEvent hands the core a login event to record.
	//
	// It exists because the audit log had no logins in it at all —
	// features/audit-log.md sent an operator to `journalctl -u easywall-web` for
	// them — and because the entry has to be written by the process that owns
	// the record. The web process is network-facing; a failed login is
	// unauthenticated input, and the payload is therefore a fixed enum with no
	// free-text field anywhere in it. See LoginEvent below.
	CmdLogEvent CommandType = "LOG_EVENT"

	// CmdCancelAcceptance ends an open acceptance window at the operator's
	// request, so the rules roll back now instead of in two minutes.
	//
	// It is not the panic button in reverse. The panic banner deliberately
	// carries no control, because the network-facing process may not *re-arm* a
	// firewall a human disarmed at the console — a stolen session would then be
	// able to. This runs the other way: it restores the last *confirmed* rule
	// set, the state the operator already approved, and a stolen session reaches
	// the identical outcome today by doing nothing for 120 seconds. It grants no
	// capability; it saves the wait.
	CmdCancelAcceptance CommandType = "CANCEL_ACCEPTANCE"

	// CmdGetPacketLog returns what the firewall refused: the packets the eleven
	// log rules sent to easywall's NFLOG group, decoded by the core, filtered
	// by PacketLogFilter, newest first.
	//
	// Read-only and answered out of memory, so it keeps the short deadline.
	// No audit entry: reading the log is not an event, CmdGetLog's reasoning.
	CmdGetPacketLog CommandType = "GET_PACKET_LOG"

	// CmdUpdateFeed hands the core a new version of one enabled feed — the
	// entries the web process fetched and parsed, or not_modified for a 304.
	// The core trusts none of it: it re-parses every entry, drops what is not
	// globally routable, refuses a prefix broader than /8 or /16 and a copy
	// under 70 % of the stored one, stores what is left and, if the feed is in
	// Current, replaces the kernel set's contents without an apply (spec §3).
	// The long deadline — see CommandTimeout.
	CmdUpdateFeed CommandType = "UPDATE_FEED"

	// CmdGetFeeds returns what the core holds for each feed: counts,
	// timestamps and counters, never entries. With an address it also says
	// which copies contain it, which is how Reachable learns of a feed hit
	// without a third command. Short deadline; no audit entry.
	CmdGetFeeds CommandType = "GET_FEEDS"
)

// AllCommandTypes is the complete list of every command the protocol declares.
// It is exported so other packages and tests can verify that the commands they
// handle match what the documentation claims. The protocol's caller — the web
// process — and its documentation must agree on what commands exist, and this
// list is the authoritative answer.
var AllCommandTypes = []CommandType{
	CmdGetRules, CmdSaveRules, CmdApplyRules, CmdAccept, CmdCancelAcceptance,
	CmdGetStatus, CmdGetOptions, CmdSaveOptions,
	CmdGetSettings, CmdSaveSettings, CmdGetSystem,
	CmdSaveSystem, CmdGetLog, CmdExportRules,
	CmdImportRules, CmdValidateCustom, CmdGetAppliedConfig, CmdGetUsage,
	CmdGetHealth, CmdPanic, CmdResume, CmdLogEvent, CmdGetPacketLog,
	CmdUpdateFeed, CmdGetFeeds,
}

// LoginEvent is one of the thirteen things that can happen at the door. The
// type is closed on purpose: the core refuses anything not in AllLoginEvents
// and writes nothing, so the web process cannot compose a line of its own.
type LoginEvent string

const (
	EvLoginOK         LoginEvent = "login_ok"
	EvLoginFailed     LoginEvent = "login_failed"
	Ev2FAFailed       LoginEvent = "login_2fa_failed"
	EvRecoveryUsed    LoginEvent = "login_recovery_used"
	EvRateLimited     LoginEvent = "login_ratelimited"
	EvLogout          LoginEvent = "logout"
	EvTOTPEnabled     LoginEvent = "totp_enabled"
	EvTOTPDisabled    LoginEvent = "totp_disabled"
	EvRecoveryRenewed LoginEvent = "recovery_codes_regenerated"
	EvPasskeyUsed     LoginEvent = "passkey_used"
	EvPasskeyEnrolled LoginEvent = "passkey_enrolled"
	EvPasskeyRemoved  LoginEvent = "passkey_removed"

	// EvPasskeyCloneSuspected is a passkey assertion that verified — the
	// signature checks out — but whose signature counter did not advance past
	// what this credential last reported. go-webauthn's own UpdateCounter
	// calls that a CloneWarning and still returns success from ValidateLogin;
	// this is the event that exists because that flag is otherwise stored and
	// never read. Distinct from Ev2FAFailed on purpose: "the assertion did not
	// verify" and "this credential's counter went backwards" are different
	// facts, and an operator reading the log needs to tell them apart.
	EvPasskeyCloneSuspected LoginEvent = "passkey_clone_suspected" // #nosec G101 -- an event name, not a credential; gosec's pattern matches "pass" inside "passkey"
)

// AllLoginEvents is the complete list, and it is what four guards hang off:
// the core's dispatch accepts exactly these, features/audit-log.md documents
// each, and both locale files label each.
var AllLoginEvents = []LoginEvent{
	EvLoginOK, EvLoginFailed, Ev2FAFailed, EvRecoveryUsed, EvRateLimited,
	EvLogout, EvTOTPEnabled, EvTOTPDisabled, EvRecoveryRenewed,
	EvPasskeyUsed, EvPasskeyEnrolled, EvPasskeyRemoved,
	EvPasskeyCloneSuspected,
}

// ValidLoginEvent reports whether ev is one this protocol declares.
func ValidLoginEvent(ev LoginEvent) bool {
	for _, known := range AllLoginEvents {
		if known == ev {
			return true
		}
	}
	return false
}

// LogEventPayload is the payload for CmdLogEvent.
//
// Four fields and not one of them is free text. Addr is run through
// netip.ParseAddr in the core and normalised there; if that fails the entry is
// written *without* an address rather than dropped, because the entry is the
// record and the address is the annotation. Left is an integer and can smuggle
// nothing. Proxied is a boolean: false only when the resolved-client walk named
// a client, true whenever it fell back to the peer — a derivation shaped by a
// header's value, but a bool still smuggles no free text into the log. See
// docs-tech/threat-model.md for why reading a trusted header is acceptable at
// all. The submitted username is
// deliberately absent: it would be foreign text in the record, and with exactly
// one account it says nothing.
type LogEventPayload struct {
	Event   LoginEvent `json:"event"`
	Addr    string     `json:"addr"`
	Left    int        `json:"left"` // login_recovery_used only
	Proxied bool       `json:"proxied"`
}

// ProxyToken is what an address recorded through a reverse proxy carries in the
// detail string. One fixed English word, appended right after the address —
// `grep 'via-proxy' audit.log` finds every proxied login regardless of what
// follows it in the same detail (a debounce summary, a recovery-codes count).
// The interface strips it, wherever it sits, and renders a chip in the
// operator's language in its place.
//
// One constant, three packages: the core writes it (loginevents.go), the web
// process strips it in two places (server.go's detailLabel, democlient.go's
// demo echo of the same shape). A second literal drifting out of step with
// this one would silently stop the chip from rendering for whichever caller
// still had the old spelling.
const ProxyToken = " via-proxy"

// Command is sent from easywall-web to easywall-core over the Unix socket.
type Command struct {
	Type    CommandType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is returned from easywall-core to easywall-web.
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// SaveRulesPayload is the payload for CmdSaveRules.
type SaveRulesPayload struct {
	RuleType string      `json:"rule_type"` // "tcp", "udp", "blocklist", "allowlist", "feeds", "forwarding", "custom"
	Rules    interface{} `json:"rules"`
}

// AcceptResult is returned for CmdAccept. Accepted is false when no acceptance
// window was open — the confirmation arrived after the window had already
// closed and the rules had been rolled back.
type AcceptResult struct {
	Accepted bool `json:"accepted"`
}

// CancelResult is returned for CmdCancelAcceptance. Cancelled is false when no
// window was open — the rollback arrived after it had already closed, and the
// previous rules came back on their own.
type CancelResult struct {
	Cancelled bool `json:"cancelled"`
}

// ErrApplyInProgressText is the exact Response.Error the core returns when
// APPLY_RULES arrives while a cycle is already running.
//
// It lives here because both sides have to agree on it: the core writes it, and
// the web process has to recognise it to say "an apply is already running"
// rather than reporting a generic failure. Response carries no error code, and
// adding one for a single case is more protocol than this needs.
const ErrApplyInProgressText = "an apply is already in progress"

// ErrPanicEngagedText is the exact Response.Error the core returns when
// APPLY_RULES arrives while panic mode is engaged.
//
// It lives here for the same reason as ErrApplyInProgressText: the core
// writes it and the web process has to recognise it, to say plainly that the
// firewall was taken down at the console rather than reporting a generic
// failure. The case this guards is a browser tab left open across a `panic`
// run at the console — the maintainer has ruled that the web interface may
// not be the thing that re-arms a firewall someone disarmed by hand, and this
// is the string that lets the interface explain the refusal instead of just
// showing it.
const ErrPanicEngagedText = "panic mode is engaged"

// ValidateCustomPayload is the payload for CmdValidateCustom.
type ValidateCustomPayload struct {
	Rules []string `json:"rules"`
}

// ValidateCustomResult holds per-rule validation errors (empty = all valid).
type ValidateCustomResult struct {
	Errors map[int]string `json:"errors"`
}

// MaxMessageBytes bounds one request and one reply on the socket, each way.
// Eight MiB since 2.23: one UPDATE_FEED carries up to FeedMaxEntries entries,
// and 100 000 of the longest IPv6 addresses measure 4 200 059 bytes bare and
// 4 600 059 as /128 prefixes — over four MiB either way (plan P12,
// TestMaxMessageBytesCarriesAFullFeed). It was 1 MiB, and a longer request
// was cut at the limit and answered "invalid JSON command" — a truncation
// indistinguishable from a malformed message.
const MaxMessageBytes = 8 << 20

// ErrRequestTooLargeText is the exact Response.Error the core returns for a
// request longer than MaxMessageBytes. SendCommand refuses to send one in the
// first place, with ErrRequestTooLarge, so the web process can record a feed
// refresh as too large without a round trip.
const ErrRequestTooLargeText = "request too large"

// ErrRequestTooLarge is what SendCommand returns for a command it will not
// send. errors.Is finds it through the wrapping.
var ErrRequestTooLarge = errors.New(ErrRequestTooLargeText)

// ErrFeedShrankText is the exact Response.Error the core returns when an update
// for a feed enabled in Current would leave fewer than FeedShrinkPercent of the
// stored entries. The web process matches it to tell the operator how to accept
// the smaller list: switch the feed off and apply, then on and apply — a feed
// that is only staged is not held to the guard.
const ErrFeedShrankText = "feed shrank below 70 % of the stored copy"

// UpdateFeedPayload is the payload for CmdUpdateFeed.
type UpdateFeedPayload struct {
	ID string `json:"id"`
	// Entries are addresses or prefixes, canonical and deduplicated by the web
	// process. The core parses each one again.
	Entries []string `json:"entries,omitempty"`
	// NotModified is a 304: the core moves checked_at and nothing else (P8).
	NotModified bool `json:"not_modified,omitempty"`
}

// UpdateFeedResult is the reply to CmdUpdateFeed.
type UpdateFeedResult struct {
	Before  int  `json:"before"`  // stored entry count before
	After   int  `json:"after"`   // after the core's own filtering
	Dropped int  `json:"dropped"` // not globally routable, removed by the core
	Changed bool `json:"changed"` // the stored prefix set differs from the previous one
	Loaded  bool `json:"loaded"`  // the kernel set was replaced (the feed is in Current)
}

// GetFeedsPayload is the payload for CmdGetFeeds. Empty is allowed.
type GetFeedsPayload struct {
	// Addr, when set, makes each FeedStatus say whether its copy contains it.
	Addr string `json:"addr,omitempty"`
}

// FeedStatus is what the core holds for one feed. Never its entries.
type FeedStatus struct {
	ID        string    `json:"id"`
	Stored    bool      `json:"stored"` // a copy is in feeds.json
	Entries   int       `json:"entries"`
	Dropped   int       `json:"dropped"`
	ChangedAt time.Time `json:"changed_at"`
	CheckedAt time.Time `json:"checked_at"`
	InKernel  bool      `json:"in_kernel"` // enabled in Current
	// Packets is the _feed-<id> counters since the last apply; 0 and
	// CountersRead false when they could not be read (P16).
	Packets      uint64 `json:"packets"`
	CountersRead bool   `json:"counters_read"`
	// AllowlistOverlap is how many staged allowlist entries this copy covers:
	// those stay reachable, because the allowlist is evaluated first.
	AllowlistOverlap int  `json:"allowlist_overlap"`
	ContainsAddr     bool `json:"contains_addr"`
}

// GetFeedsResult is the reply to CmdGetFeeds.
type GetFeedsResult struct {
	Feeds []FeedStatus `json:"feeds"` // one per id stored or enabled in Current ∪ Staged
}
