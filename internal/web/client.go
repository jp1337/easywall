package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

const clientTimeout = 5 * time.Second

// CoreClient communicates with the easywall-core daemon over a Unix socket.
type CoreClient struct {
	socketPath string
}

// NewCoreClient creates a client targeting the given Unix socket path.
func NewCoreClient(socketPath string) *CoreClient {
	return &CoreClient{socketPath: socketPath}
}

// Send sends a typed command to the core daemon and returns its response.
func (c *CoreClient) Send(cmd shared.Command) (shared.Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, clientTimeout)
	if err != nil {
		return shared.Response{}, fmt.Errorf("connect to core: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(clientTimeout))

	out, err := json.Marshal(cmd)
	if err != nil {
		return shared.Response{}, fmt.Errorf("marshal command: %w", err)
	}
	if _, err := conn.Write(out); err != nil {
		return shared.Response{}, fmt.Errorf("send command: %w", err)
	}

	// Signal EOF so the daemon's io.ReadAll returns
	if tc, ok := conn.(*net.UnixConn); ok {
		_ = tc.CloseWrite()
	}

	data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return shared.Response{}, fmt.Errorf("read response: %w", err)
	}

	var resp shared.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return shared.Response{}, fmt.Errorf("parse response: %w", err)
	}
	return resp, nil
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

// Accept signals that the admin confirmed the new rules work.
func (c *CoreClient) Accept() error {
	resp, err := c.Send(shared.Command{Type: shared.CmdAccept})
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
