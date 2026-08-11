package shared

import "encoding/json"

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
)

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

// ValidateCustomPayload is the payload for CmdValidateCustom.
type ValidateCustomPayload struct {
	Rules []string `json:"rules"`
}

// ValidateCustomResult holds per-rule validation errors (empty = all valid).
type ValidateCustomResult struct {
	Errors map[int]string `json:"errors"`
}
