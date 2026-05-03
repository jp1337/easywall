package web

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// demoState is an in-memory mock of easywall-core that powers the public
// demo deployment. All commands sent through CoreClient.Send are dispatched
// here when the client was constructed via NewDemoClient — no nftables, no
// root, no Unix socket. State resets to seed values whenever the process
// restarts (systemd-friendly for periodic reset on a public host).
type demoState struct {
	mu sync.Mutex

	rules    shared.RulesState
	options  shared.FirewallOptions
	settings shared.NetworkSettings
	system   shared.SystemSettings

	auditLog []shared.AuditLogEntry

	acceptance shared.AcceptanceStatus
	lastApply  string // RFC3339, empty when never applied

	// Pending acceptance timer; if it fires before CmdAccept arrives,
	// state rolls back to .Backup and acceptance becomes "rolled_back".
	acceptanceTimer *time.Timer

	// User identity recorded in audit log entries — overridable but
	// "demo" by default since we don't have a real session here.
	actor string
}

// newDemoState constructs the demo state machine and seeds it with a
// realistic example rule set so the UI looks alive on first visit.
func newDemoState() *demoState {
	d := &demoState{
		actor:      "demo",
		acceptance: shared.AcceptanceIdle,
	}
	d.seed()
	return d
}

func (d *demoState) seed() {
	example := shared.Rules{
		TCP: []shared.PortRule{
			{Port: "22", Description: "SSH", SSH: true},
			{Port: "80", Description: "HTTP"},
			{Port: "443", Description: "HTTPS"},
		},
		UDP: []shared.PortRule{
			{Port: "53", Description: "DNS"},
		},
		Whitelist:  []string{"192.168.0.0/24"},
		Blacklist:  []string{"198.51.100.42"},
		Forwarding: []shared.ForwardingRule{},
		Custom:     []string{},
	}
	d.rules = shared.RulesState{
		Current: example,
		Staged:  example,
		Backup:  example,
	}
	d.options = shared.FirewallOptions{
		SSHBruteForce:                true,
		SSHBruteForceConnectionLimit: 5,
		SSHBruteForceLogLimit:        60,
		ICMPFlood:                    true,
		ICMPFloodConnectionLimit:     10,
		ICMPFloodLogLimit:            60,
		SYNFlood:                     true,
		SYNFloodLimit:                100,
		PortScan:                     true,
		InvalidPackets:               true,
		ConnectionLimitMax:           100,
		TCPRSTFloodLimit:             100,
		LogBlockedLimit:              60,
		LogBlacklistLimit:            60,
	}
	d.settings = shared.NetworkSettings{
		IPv6: shared.IPv6Config{
			Enabled:                        true,
			ICMPAllowRouterAdvertisement:   true,
			ICMPAllowNeighborAdvertisement: true,
		},
	}
	d.system = shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 60},
	}
	// Seed a few audit log entries from "yesterday" so the page is not empty.
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	d.auditLog = []shared.AuditLogEntry{
		{Time: yesterday.Format(time.RFC3339), Action: "rules_applied", User: "demo"},
		{Time: yesterday.Add(-1 * time.Minute).Format(time.RFC3339), Action: "rules_saved", RuleType: "tcp", User: "demo"},
		{Time: yesterday.Add(-5 * time.Minute).Format(time.RFC3339), Action: "options_saved", User: "demo"},
		{Time: yesterday.Add(-10 * time.Minute).Format(time.RFC3339), Action: "rules_saved", RuleType: "blacklist", User: "demo"},
	}
	d.lastApply = yesterday.Format(time.RFC3339)
}

// Send dispatches a typed command to the in-memory handler.
func (d *demoState) Send(cmd shared.Command) shared.Response {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch cmd.Type {
	case shared.CmdGetStatus:
		return demoOK(d.statusLocked())
	case shared.CmdGetRules:
		return demoOK(d.rules)
	case shared.CmdSaveRules:
		return d.handleSaveRules(cmd.Payload)
	case shared.CmdApplyRules:
		return d.handleApplyRules()
	case shared.CmdAccept:
		return d.handleAccept()
	case shared.CmdGetOptions:
		return demoOK(d.options)
	case shared.CmdSaveOptions:
		return d.handleSaveOptions(cmd.Payload)
	case shared.CmdGetSettings:
		return demoOK(d.settings)
	case shared.CmdSaveSettings:
		return d.handleSaveSettings(cmd.Payload)
	case shared.CmdGetSystem:
		return demoOK(d.system)
	case shared.CmdSaveSystem:
		return d.handleSaveSystem(cmd.Payload)
	case shared.CmdGetLog:
		return demoOK(d.auditLog)
	case shared.CmdValidateCustom:
		// Demo can't run nft — accept everything as valid.
		return demoOK(shared.ValidateCustomResult{Errors: map[int]string{}})
	case shared.CmdExportRules:
		raw, err := json.Marshal(d.rules.Current)
		if err != nil {
			return demoErr(err)
		}
		return shared.Response{Success: true, Data: raw}
	case shared.CmdImportRules:
		return d.handleImportRules(cmd.Payload)
	}
	return demoErr(fmt.Errorf("unknown command %q", cmd.Type))
}

// ── Helpers ──────────────────────────────────────────────────────────────

func demoOK(data interface{}) shared.Response {
	raw, err := json.Marshal(data)
	if err != nil {
		return demoErr(err)
	}
	return shared.Response{Success: true, Data: raw}
}

func demoErr(err error) shared.Response {
	return shared.Response{Success: false, Error: err.Error()}
}

// hasPendingLocked compares Current to Staged via JSON marshal — order-stable
// across slices and works for arbitrary nested types in shared.Rules.
func (d *demoState) hasPendingLocked() bool {
	a, _ := json.Marshal(d.rules.Current)
	b, _ := json.Marshal(d.rules.Staged)
	return string(a) != string(b)
}

func (d *demoState) statusLocked() shared.FirewallStatus {
	return shared.FirewallStatus{
		Active:     true,
		Acceptance: d.acceptance,
		HasPending: d.hasPendingLocked(),
		LastApply:  d.lastApply,
	}
}

// audit appends a newest-first entry to the log, capped at 200 to mirror
// the real core's behavior.
func (d *demoState) audit(action, ruleType, detail string) {
	e := shared.AuditLogEntry{
		Time:     time.Now().Format(time.RFC3339),
		Action:   action,
		RuleType: ruleType,
		Detail:   detail,
		User:     d.actor,
	}
	d.auditLog = append([]shared.AuditLogEntry{e}, d.auditLog...)
	if len(d.auditLog) > 200 {
		d.auditLog = d.auditLog[:200]
	}
}

// ── Command handlers ─────────────────────────────────────────────────────

func (d *demoState) handleSaveRules(payload []byte) shared.Response {
	// Decode as raw map first, then dispatch by RuleType to the correct
	// concrete type. The core does the same thing on its side.
	var generic struct {
		RuleType string          `json:"rule_type"`
		Rules    json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(payload, &generic); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}

	switch generic.RuleType {
	case "tcp":
		var rs []shared.PortRule
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid tcp rules: %w", err))
		}
		d.rules.Staged.TCP = rs
	case "udp":
		var rs []shared.PortRule
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid udp rules: %w", err))
		}
		d.rules.Staged.UDP = rs
	case "blacklist":
		var rs []string
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid blacklist: %w", err))
		}
		d.rules.Staged.Blacklist = rs
	case "whitelist":
		var rs []string
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid whitelist: %w", err))
		}
		d.rules.Staged.Whitelist = rs
	case "custom":
		var rs []string
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid custom: %w", err))
		}
		d.rules.Staged.Custom = rs
	case "forwarding":
		var rs []shared.ForwardingRule
		if err := json.Unmarshal(generic.Rules, &rs); err != nil {
			return demoErr(fmt.Errorf("invalid forwarding rules: %w", err))
		}
		d.rules.Staged.Forwarding = rs
	default:
		return demoErr(fmt.Errorf("unknown rule type %q", generic.RuleType))
	}

	d.audit("rules_saved", generic.RuleType, "")
	return shared.Response{Success: true}
}

func (d *demoState) handleApplyRules() shared.Response {
	// Cancel any in-flight pending timer so we don't roll back twice.
	if d.acceptanceTimer != nil {
		d.acceptanceTimer.Stop()
		d.acceptanceTimer = nil
	}

	// Snapshot the current rules as backup; promote staged → current.
	d.rules.Backup = d.rules.Current
	d.rules.Current = d.rules.Staged
	d.lastApply = time.Now().Format(time.RFC3339)
	d.audit("rules_applied", "", "")

	if d.system.Acceptance.Enabled {
		d.acceptance = shared.AcceptancePending
		dur := time.Duration(d.system.Acceptance.Duration) * time.Second
		d.acceptanceTimer = time.AfterFunc(dur, d.rollback)
	} else {
		d.acceptance = shared.AcceptanceAccepted
		// Match the real core: brief "accepted" pulse, then drop back to idle.
		go d.delayedReset()
	}

	raw, _ := json.Marshal(map[string]string{"status": "started"})
	return shared.Response{Success: true, Data: raw}
}

func (d *demoState) handleAccept() shared.Response {
	if d.acceptanceTimer != nil {
		d.acceptanceTimer.Stop()
		d.acceptanceTimer = nil
	}
	d.acceptance = shared.AcceptanceAccepted
	go d.delayedReset()
	return shared.Response{Success: true}
}

// rollback fires from a time.AfterFunc when the acceptance window expires
// without an Accept. It must lock independently because it runs on its own
// goroutine.
func (d *demoState) rollback() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rules.Current = d.rules.Backup
	d.acceptance = shared.AcceptanceRolledBack
	d.audit("rules_rolled_back", "", "acceptance window expired")
	d.acceptanceTimer = nil
}

// delayedReset matches the real core's behavior: after Accept, the status
// pulses "accepted" briefly before returning to idle so the dashboard can
// reflect the success and then quiet down.
func (d *demoState) delayedReset() {
	time.Sleep(2 * time.Second)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acceptance == shared.AcceptanceAccepted {
		d.acceptance = shared.AcceptanceIdle
	}
}

func (d *demoState) handleSaveOptions(payload []byte) shared.Response {
	var opts shared.FirewallOptions
	if err := json.Unmarshal(payload, &opts); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	d.options = opts
	d.audit("options_saved", "", "")
	return shared.Response{Success: true}
}

func (d *demoState) handleSaveSettings(payload []byte) shared.Response {
	var s shared.NetworkSettings
	if err := json.Unmarshal(payload, &s); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	d.settings = s
	d.audit("settings_saved", "", "")
	return shared.Response{Success: true}
}

func (d *demoState) handleSaveSystem(payload []byte) shared.Response {
	var s shared.SystemSettings
	if err := json.Unmarshal(payload, &s); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	if s.Acceptance.Duration <= 0 {
		return demoErr(fmt.Errorf("acceptance.duration must be > 0"))
	}
	d.system = s
	d.audit("system_saved", "", "")
	return shared.Response{Success: true}
}

func (d *demoState) handleImportRules(payload []byte) shared.Response {
	// The export endpoint returns the Current rules as JSON. Import accepts
	// the same shape and replaces Staged so the user can review + apply.
	var imported shared.Rules
	if err := json.Unmarshal(payload, &imported); err != nil {
		return demoErr(fmt.Errorf("invalid rules: %w", err))
	}
	d.rules.Staged = imported
	d.audit("rules_imported", "", "")
	return shared.Response{Success: true}
}
