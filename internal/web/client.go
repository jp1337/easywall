package web

import (
	"encoding/json"
	"fmt"

	"github.com/jp1337/easywall/internal/shared"
)

// CoreClient communicates with the easywall-core daemon over a Unix socket,
// or — when demo is non-nil — with an in-memory mock for the public demo
// deployment. Callers don't see the difference: every method on CoreClient
// goes through Send, which transparently picks the right backend.
type CoreClient struct {
	socketPath string
	demo       *demoState
}

// NewCoreClient creates a client targeting the given Unix socket path.
func NewCoreClient(socketPath string) *CoreClient {
	return &CoreClient{socketPath: socketPath}
}

// NewDemoClient creates a client that dispatches every command to an
// in-memory state machine (no socket, no core, no nftables). Used by the
// public demo deployment configured with `demo_mode = true`.
func NewDemoClient() *CoreClient {
	return &CoreClient{demo: newDemoState()}
}

// IsDemo reports whether this client is backed by the in-memory demo.
func (c *CoreClient) IsDemo() bool {
	return c.demo != nil
}

// Send sends a typed command to the core daemon and returns its response.
// In demo mode, the command is dispatched to the in-memory state machine
// instead of the Unix socket — same return shape, no network I/O.
//
// The transport itself is shared.SendCommand: the console subcommands on
// easywall-core need the same thing, and the privileged binary must not import
// this package to get it.
func (c *CoreClient) Send(cmd shared.Command) (shared.Response, error) {
	if c.demo != nil {
		return c.demo.Send(cmd), nil
	}
	return shared.SendCommand(c.socketPath, cmd)
}

// GetStatus returns the current firewall status.
func (c *CoreClient) GetStatus() (*shared.FirewallStatus, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetStatus})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var status shared.FirewallStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}
	return &status, nil
}

// GetRules returns the full three-state rules document.
func (c *CoreClient) GetRules() (*shared.RulesState, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetRules})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var state shared.RulesState
	if err := json.Unmarshal(resp.Data, &state); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	return &state, nil
}

// SaveRules saves staged rules of the given type.
// ruleType: "tcp", "udp", "blacklist", "whitelist", "forwarding", "custom"
func (c *CoreClient) SaveRules(ruleType string, rules interface{}) error {
	payload, err := json.Marshal(shared.SaveRulesPayload{
		RuleType: ruleType,
		Rules:    rules,
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	resp, err := c.Send(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// ApplyRules triggers an asynchronous rule application on the core.
func (c *CoreClient) ApplyRules() error {
	resp, err := c.Send(shared.Command{Type: shared.CmdApplyRules})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// Accept confirms the applied rules. The bool reports whether an acceptance
// window was actually open: false means the confirmation came too late and the
// rules have already been rolled back.
func (c *CoreClient) Accept() (bool, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdAccept})
	if err != nil {
		return false, err
	}
	if !resp.Success {
		return false, fmt.Errorf("core error: %s", resp.Error)
	}
	var result shared.AcceptResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return false, fmt.Errorf("decode accept result: %w", err)
		}
	}
	return result.Accepted, nil
}

// Panic takes the firewall down through the core. The web interface does not
// offer this — `easywall-core panic` is the console tool — but the demo needs a
// way to reach the state, and a client method that exists is easier to keep
// honest than one that does not.
func (c *CoreClient) Panic() error {
	resp, err := c.Send(shared.Command{Type: shared.CmdPanic})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// Resume ends panic mode through the core.
func (c *CoreClient) Resume() error {
	resp, err := c.Send(shared.Command{Type: shared.CmdResume})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// GetOptions returns the current firewall options from the core config.
func (c *CoreClient) GetOptions() (*shared.FirewallOptions, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetOptions})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var opts shared.FirewallOptions
	if err := json.Unmarshal(resp.Data, &opts); err != nil {
		return nil, fmt.Errorf("parse options: %w", err)
	}
	return &opts, nil
}

// SaveOptions persists updated firewall options to the core config.
func (c *CoreClient) SaveOptions(opts shared.FirewallOptions) error {
	payload, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	resp, err := c.Send(shared.Command{Type: shared.CmdSaveOptions, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// GetSettings returns the current IPv6 and Docker configuration.
func (c *CoreClient) GetSettings() (*shared.NetworkSettings, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetSettings})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var s shared.NetworkSettings
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	return &s, nil
}

// SaveSettings persists updated IPv6 and Docker network settings to the core config.
func (c *CoreClient) SaveSettings(s shared.NetworkSettings) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	resp, err := c.Send(shared.Command{Type: shared.CmdSaveSettings, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// GetSystem returns the current acceptance window configuration.
func (c *CoreClient) GetSystem() (*shared.SystemSettings, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetSystem})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var s shared.SystemSettings
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		return nil, fmt.Errorf("parse system settings: %w", err)
	}
	return &s, nil
}

// SaveSystem persists updated system settings (acceptance window) to the core config.
func (c *CoreClient) SaveSystem(s shared.SystemSettings) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	resp, err := c.Send(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// GetLog returns the most recent audit log entries (newest first).
func (c *CoreClient) GetLog() ([]shared.AuditLogEntry, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdGetLog})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	var entries []shared.AuditLogEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		return nil, fmt.Errorf("parse log entries: %w", err)
	}
	return entries, nil
}

// ExportRules returns the current rule set as pretty-printed JSON bytes.
func (c *CoreClient) ExportRules() ([]byte, error) {
	resp, err := c.Send(shared.Command{Type: shared.CmdExportRules})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("core error: %s", resp.Error)
	}
	// resp.Data is already the pretty JSON (encoded as JSON string)
	var raw json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return resp.Data, nil // return as-is if not double-encoded
	}
	return raw, nil
}

// ImportRules validates and imports rules from raw JSON bytes.
func (c *CoreClient) ImportRules(data []byte) error {
	resp, err := c.Send(shared.Command{Type: shared.CmdImportRules, Payload: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// LogEvent hands the core one login event for the audit log.
//
// Fire-and-forget from the caller's point of view — auditevents.go is what calls
// this, from its own goroutine, so a slow or absent core never delays a login.
func (c *CoreClient) LogEvent(ev shared.LoginEvent, addr string, left int) error {
	payload, err := json.Marshal(shared.LogEventPayload{Event: ev, Addr: addr, Left: left})
	if err != nil {
		return fmt.Errorf("encode login event: %w", err)
	}
	resp, err := c.Send(shared.Command{Type: shared.CmdLogEvent, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("core error: %s", resp.Error)
	}
	return nil
}

// ValidateCustom validates custom nftables rules before saving.
// Returns a map of line-index to error string; empty map means all valid.
func (c *CoreClient) ValidateCustom(rules []string) (map[int]string, error) {
	payload, err := json.Marshal(shared.ValidateCustomPayload{Rules: rules})
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	resp, err := c.Send(shared.Command{
		Type:    shared.CmdValidateCustom,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	var result shared.ValidateCustomResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Errors, nil
}
