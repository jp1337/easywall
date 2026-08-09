package web

import (
	"encoding/json"
	"errors"
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
	// Seed every list with the kind of entries an experienced operator
	// would actually have on a small-to-mid-sized internet-facing host.
	// All IPs/CIDRs use RFC 5737 / RFC 3849 documentation prefixes so
	// nothing here points at a real network.
	example := shared.Rules{
		TCP: []shared.PortRule{
			{Port: "22", Description: "SSH (admin)", SSH: true},
			{Port: "80", Description: "HTTP — redirect to HTTPS"},
			{Port: "443", Description: "HTTPS — main web"},
			{Port: "8443", Description: "HTTPS — staging / backend"},
			{Port: "25", Description: "SMTP — incoming mail"},
			{Port: "587", Description: "SMTP submission (STARTTLS)"},
			{Port: "993", Description: "IMAPS"},
			{Port: "5432", Description: "PostgreSQL — replication peer"},
		},
		UDP: []shared.PortRule{
			{Port: "53", Description: "DNS — authoritative"},
			{Port: "123", Description: "NTP"},
			{Port: "51820", Description: "WireGuard VPN"},
		},
		Blacklist: []string{
			"# scanner ranges observed in fail2ban logs over the last 30d",
			"192.0.2.42",
			"192.0.2.118",
			"198.51.100.0/24",
			"203.0.113.0/24",
			"",
			"# IPv6 — block known ranges from compromised cloud tenant",
			"2001:db8:bad::/48",
		},
		Whitelist: []string{
			"# always-allow management — never lock these out",
			"203.0.113.10",
			"203.0.113.11",
			"",
			"# office / VPN subnet",
			"198.51.100.0/24",
			"",
			"# monitoring — Prometheus scraper",
			"192.0.2.50/32",
			"",
			"# IPv6 admin range",
			"2001:db8:1::/64",
		},
		Forwarding: []shared.ForwardingRule{
			{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
			{Protocol: "tcp", SourcePort: 8443, DestPort: 443},
			{Protocol: "tcp", SourcePort: 2222, DestPort: 22},
			{Protocol: "udp", SourcePort: 51821, DestPort: 51820},
		},
		Custom: []string{
			"# allow Prometheus node-exporter only from the monitoring host",
			"ip saddr 192.0.2.50 tcp dport 9100 accept",
			"",
			"# rate-limit inbound DNS queries to mitigate amplification",
			"udp dport 53 limit rate 50/second accept",
			"",
			"# log + drop traffic to legacy admin port",
			"tcp dport 10000 log prefix \"legacy-admin: \" drop",
		},
	}
	d.rules = shared.RulesState{
		Current: example,
		Staged:  example,
		Backup:  example,
	}
	d.options = shared.FirewallOptions{
		SSHBruteForce:                true,
		SSHBruteForceLog:             true,
		SSHBruteForceConnectionLimit: 5,
		SSHBruteForceLogLimit:        60,
		ICMPFlood:                    true,
		ICMPFloodConnectionLimit:     10,
		ICMPFloodLogLimit:            60,
		SYNFlood:                     true,
		SYNFloodLog:                  true,
		SYNFloodLimit:                100,
		PortScan:                     true,
		PortScanLog:                  true,
		InvalidPackets:               true,
		Bogons:                       true,
		ConnectionLimit:              true,
		ConnectionLimitMax:           100,
		TCPRSTFloodLimit:             100,
		LogBlocked:                   true,
		LogBlockedLimit:              60,
		LogBlacklist:                 true,
		LogBlacklistLimit:            60,
	}
	d.settings = shared.NetworkSettings{
		IPv6: shared.IPv6Config{
			Mode:                           shared.IPv6Filter,
			ICMPAllowRouterAdvertisement:   true,
			ICMPAllowNeighborAdvertisement: true,
		},
		Docker: shared.DockerConfig{
			Enabled:             true,
			AllowBridgeNetworks: true,
			CustomNetworks:      []string{"172.20.0.0/16"},
		},
	}
	d.system = shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 120},
	}
	// Audit log: 18 entries spanning the last ~30 hours to simulate
	// realistic operator activity (apply cycles, individual rule edits,
	// option toggles, an import, a rollback). Newest first.
	now := time.Now()
	d.auditLog = buildSeedAuditLog(now)
	d.lastApply = now.Add(-2 * time.Hour).Format(time.RFC3339)
}

// buildSeedAuditLog returns ~18 plausible audit entries, newest first.
// Times are spread across the last day with variable gaps so the table
// looks like a real working log, not a demo stub.
//
// Details are written the way the core writes them — data, not prose. They used
// to read "added 8443 (staging)" and "acceptance window expired", which was
// English text no locale could reach, and it also promised a level of detail the
// core does not record (see Known Gaps in DESIGN.md). "timeout" is the one token
// the core does emit, and it is translated through auditDetailKeys.
func buildSeedAuditLog(now time.Time) []shared.AuditLogEntry {
	type e struct {
		offset   time.Duration
		action   string
		ruleType string
		detail   string
		user     string
	}
	entries := []e{
		{-2 * time.Hour, "apply_accepted", "", "", "demo"},
		{-2*time.Hour - 30*time.Second, "rules_saved", "tcp", "+8443", "demo"},
		{-3 * time.Hour, "options_saved", "", "ssh_brute_force_log", "demo"},
		{-4 * time.Hour, "rules_saved", "blacklist", "+192.0.2.42", "demo"},
		{-4*time.Hour - 12*time.Second, "rules_saved", "blacklist", "+192.0.2.118", "demo"},
		{-5 * time.Hour, "settings_saved", "", "docker_enabled", "demo"},
		{-6 * time.Hour, "system_saved", "", "acceptance_duration=120", "demo"},
		{-8 * time.Hour, "apply_accepted", "", "", "demo"},
		{-8*time.Hour - 45*time.Second, "rules_saved", "forwarding", "+8443→443/tcp", "demo"},
		{-9 * time.Hour, "rules_saved", "whitelist", "+203.0.113.10/32", "demo"},
		{-12 * time.Hour, "rules_saved", "udp", "+51820", "demo"},
		{-14 * time.Hour, "rules_imported", "", "rules-2026-05-02.json", "demo"},
		{-18 * time.Hour, "apply_rolledback", "", "timeout", "demo"},
		{-18*time.Hour + 2*time.Minute, "apply_accepted", "", "", "demo"},
		{-20 * time.Hour, "options_saved", "", "syn_flood_limit=100", "demo"},
		{-24 * time.Hour, "rules_saved", "custom", "+1", "demo"},
		{-26 * time.Hour, "apply_accepted", "", "", "demo"},
		{-30 * time.Hour, "rules_saved", "tcp", "+22 +80 +443", "demo"},
	}
	out := make([]shared.AuditLogEntry, 0, len(entries))
	for _, x := range entries {
		out = append(out, shared.AuditLogEntry{
			Time:     now.Add(x.offset).Format(time.RFC3339),
			Action:   x.action,
			RuleType: x.ruleType,
			Detail:   x.detail,
			User:     x.user,
		})
	}
	return out
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
		// There is no nft binary behind the demo, so it cannot judge syntax. It
		// used to answer "no errors", which told every visitor their rules were
		// valid whatever they typed — a false green on the one page where being
		// wrong locks you out. Reporting the checker as unavailable is the truth,
		// and the interface already has a state for exactly that.
		return demoErr(errors.New("syntax checking needs the core daemon"))
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
	d.audit("apply_accepted", "", "")

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
	d.audit("apply_rolledback", "", "timeout") // the token the real core writes
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
