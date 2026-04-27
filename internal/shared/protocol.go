package shared

import "encoding/json"

// CommandType identifies which operation the core daemon should perform.
type CommandType string

const (
	CmdGetRules    CommandType = "GET_RULES"
	CmdSaveRules   CommandType = "SAVE_RULES"
	CmdApplyRules  CommandType = "APPLY_RULES"
	CmdAccept      CommandType = "ACCEPT"
	CmdGetStatus   CommandType = "GET_STATUS"
	CmdGetOptions  CommandType = "GET_OPTIONS"
	CmdSaveOptions  CommandType = "SAVE_OPTIONS"
	CmdGetSettings  CommandType = "GET_SETTINGS"
	CmdSaveSettings CommandType = "SAVE_SETTINGS"
	CmdExportRules  CommandType = "EXPORT_RULES"
	CmdImportRules CommandType = "IMPORT_RULES"
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
