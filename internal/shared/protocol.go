package shared

import (
	"encoding/json"
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
// The nft-backed commands, plus PANIC, get NftTimeout plus room for the core's
// own work either side. PANIC can queue behind an apply's nft subprocess — it is
// designed to win rather than to fail fast — so the client must wait as long as
// the server might. RESUME restores through beginApply and returns ErrApplyInProgress
// immediately rather than blocking, so it belongs on the short deadline.
// Everything else keeps the short deadline, because a status poll that hangs for
// half a minute is its own problem.
func CommandTimeout(cmd CommandType) time.Duration {
	switch cmd {
	case CmdImportRules, CmdValidateCustom, CmdPanic:
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
	CmdImportRules, CmdValidateCustom, CmdGetAppliedConfig,
	CmdPanic, CmdResume, CmdLogEvent,
}

// LoginEvent is one of the nine things that can happen at the door. The type is
// closed on purpose: the core refuses anything not in AllLoginEvents and writes
// nothing, so the web process cannot compose a line of its own.
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
)

// AllLoginEvents is the complete list, and it is what four guards hang off:
// the core's dispatch accepts exactly these, features/audit-log.md documents
// each, and both locale files label each.
var AllLoginEvents = []LoginEvent{
	EvLoginOK, EvLoginFailed, Ev2FAFailed, EvRecoveryUsed, EvRateLimited,
	EvLogout, EvTOTPEnabled, EvTOTPDisabled, EvRecoveryRenewed,
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
	RuleType string      `json:"rule_type"` // "tcp", "udp", "blacklist", "whitelist", "forwarding", "custom"
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
