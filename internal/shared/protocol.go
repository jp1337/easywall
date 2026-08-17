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
)

// AllCommandTypes is the complete list of every command the protocol declares.
// It is exported so other packages and tests can verify that the commands they
// handle match what the documentation claims. The protocol's caller — the web
// process — and its documentation must agree on what commands exist, and this
// list is the authoritative answer.
var AllCommandTypes = []CommandType{
	CmdGetRules, CmdSaveRules, CmdApplyRules, CmdAccept,
	CmdGetStatus, CmdGetOptions, CmdSaveOptions,
	CmdGetSettings, CmdSaveSettings, CmdGetSystem,
	CmdSaveSystem, CmdGetLog, CmdExportRules,
	CmdImportRules, CmdValidateCustom,
	CmdPanic, CmdResume,
}

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
