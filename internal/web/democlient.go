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

	// appliedConfig is what the demo's kernel would be holding: the options and
	// settings as they were at the last apply. Seeded one switch away from
	// d.options so the apply screen has a configuration drift to show — see
	// seed().
	appliedConfig shared.AppliedConfig

	auditLog []shared.AuditLogEntry

	acceptance shared.AcceptanceStatus
	lastApply  string // RFC3339, empty when never applied

	// Pending acceptance timer; if it fires before CmdAccept arrives,
	// state rolls back to .Backup and acceptance becomes "rolled_back".
	acceptanceTimer *time.Timer

	// User identity recorded in audit log entries — overridable but
	// "demo" by default since we don't have a real session here.
	actor string

	// panicMode mirrors shared.FirewallStatus.Panic. The demo has no kernel and
	// no marker file, so this bool is the whole of panic mode here: CmdPanic
	// sets it, CmdResume clears it, and statusLocked reports it the way the
	// real core reports whatever EngagePanic/ClearPanic left on disk. Named
	// panicMode rather than panic so nothing here reads like a call to the
	// builtin.
	panicMode bool
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
		Fragments:                    true,
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
		Routing: shared.RoutingConfig{
			Mode:     shared.RoutingClosed,
			Networks: []string{},
		},
	}
	// One switch away from what is configured now, so a visitor opening /apply
	// finds a configuration drift as well as a rule diff — the half of "what
	// changes" that nothing could see before 2.10. drop_fragments because it is a
	// plain on/off with no numeric partner to explain, and off -> on because that
	// is the direction an operator reads as "I turned something on and have not
	// applied it yet".
	applied := d.options
	applied.Fragments = false
	d.appliedConfig = shared.AppliedConfig{Firewall: applied, Network: d.settings}
	d.system = shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 120},
	}
	// Audit log: 18 entries spanning the last ~30 hours to simulate
	// realistic operator activity (apply cycles, individual rule edits,
	// option toggles, an import, a rollback). Newest first.
	//
	// UTC, exactly as the core writes it — core/rules.go:319 and
	// core/firewall.go:286. The demo used to store local time, which made it the
	// only installation whose stamps carried an offset other than Z, and
	// therefore the one place shortTime's offset-preserving behaviour looked
	// correct. A demo that does not behave like the product cannot be used to
	// check the product.
	now := time.Now().UTC()
	d.auditLog = buildSeedAuditLog(now)

	// Recent, not two hours stale. The demo is reset every few hours and this is
	// the first number a visitor reads; opening on a last apply from two hours
	// ago reads as an installation nobody is looking after.
	d.lastApply = now.Add(-4 * time.Minute).Format(time.RFC3339)
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
		{-4 * time.Minute, "apply_accepted", "", "", "demo"},
		{-4*time.Minute - 30*time.Second, "rules_saved", "tcp", "+8443", "demo"},
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
		// The re-apply happened two minutes after the rollback it followed, which
		// makes it the more recent of the pair — so in a newest-first list it goes
		// above the rollback, not below. Listed the other way round for a while:
		// same two offsets, swapped positions, and the log read as a rollback that
		// happened after the apply it was rolling back, which is a sequence the
		// real core cannot produce.
		{-18*time.Hour + 2*time.Minute, "apply_accepted", "", "", "demo"},
		{-18 * time.Hour, "apply_rolledback", "", "timeout", "demo"},
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
	case shared.CmdGetAppliedConfig:
		return demoOK(shared.AppliedConfigResult{Recorded: true, Config: d.appliedConfig})
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
		// Staged, like the core: import replaces staged, so this is the half
		// that makes the pair lossless.
		raw, err := json.Marshal(d.rules.Staged)
		if err != nil {
			return demoErr(err)
		}
		return shared.Response{Success: true, Data: raw}
	case shared.CmdImportRules:
		return d.handleImportRules(cmd.Payload)
	case shared.CmdPanic:
		return d.handlePanic()
	case shared.CmdResume:
		return d.handleResume()
	case shared.CmdLogEvent:
		return d.handleLogEvent(cmd.Payload)
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
// across slices and works for arbitrary nested types in shared.Rules — and then
// the configuration against what the last apply put in the kernel, exactly as
// core.Firewall.Status does. A demo whose pending state is computed differently
// from the product's is a demo that teaches the wrong thing.
func (d *demoState) hasPendingLocked() bool {
	a, _ := json.Marshal(d.rules.Current)
	b, _ := json.Marshal(d.rules.Staged)
	if string(a) != string(b) {
		return true
	}
	live := shared.AppliedConfig{Firewall: d.options, Network: d.settings}
	return len(shared.DiffConfig(d.appliedConfig, live)) > 0
}

func (d *demoState) statusLocked() shared.FirewallStatus {
	return shared.FirewallStatus{
		// The real core reports Active false the moment Panic tears the table
		// down; there is no kernel here to ask, so panicMode is the only signal
		// this mock has and Active follows its negation the same way the rest
		// of this struct's derived fields (HasPending) follow the state
		// underneath them rather than being tracked independently.
		Active:     !d.panicMode,
		Panic:      d.panicMode,
		Acceptance: d.acceptance,
		HasPending: d.hasPendingLocked(),
		LastApply:  d.lastApply,
	}
}

// audit appends a newest-first entry to the log, capped at 200 to mirror
// the real core's behavior, attributed to the operator driving the demo.
func (d *demoState) audit(action, ruleType, detail string) {
	d.auditAs(action, ruleType, detail, d.actor)
}

// auditAs is audit with an explicit user. Its one caller other than audit
// itself is handleLogEvent, which — like the real core — attributes a login
// event to "web" rather than to whichever operator is driving the demo.
func (d *demoState) auditAs(action, ruleType, detail, user string) {
	e := shared.AuditLogEntry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Action:   action,
		RuleType: ruleType,
		Detail:   detail,
		User:     user,
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

	// Kept so the audit entry can say what changed, exactly as the core does.
	before := d.rules.Staged

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

	// The same check the core runs. Without it the demo accepted input the real
	// thing refuses — and the demo is what people judge the product by before
	// they install it.
	if err := shared.ValidateRules(d.rules.Staged); err != nil {
		d.rules.Staged = before
		return demoErr(err)
	}

	d.audit("rules_saved", generic.RuleType, shared.DescribeRuleChange(generic.RuleType, before, d.rules.Staged))
	return shared.Response{Success: true}
}

func (d *demoState) handleApplyRules() shared.Response {
	// Refused for the same reason the real core refuses it: a human at the
	// console took the firewall down on purpose, and the web interface does
	// not get to override that by pushing a new apply through. Checked before
	// the acceptance-window refusal below because panic mode outranks it —
	// the marker, not any in-flight window, is what decides whether an apply
	// may run at all.
	if d.panicMode {
		return shared.Response{Success: false, Error: shared.ErrPanicEngagedText}
	}

	// Refused while a window is open, exactly as the core refuses it. The demo
	// used to accept it and silently restart the window instead — a third
	// behaviour, in the one place where visitors form their idea of what the
	// product does. The core's reason is in core.ErrApplyInProgress.
	if d.acceptance == shared.AcceptancePending {
		return shared.Response{Success: false, Error: shared.ErrApplyInProgressText}
	}

	// Cancel any in-flight pending timer so we don't roll back twice.
	if d.acceptanceTimer != nil {
		d.acceptanceTimer.Stop()
		d.acceptanceTimer = nil
	}

	// Snapshot the current rules as backup; promote staged → current.
	d.rules.Backup = d.rules.Current
	d.rules.Current = d.rules.Staged

	// The configuration goes into the kernel with the rules, so the demo records
	// it here for the same reason Firewall.apply does.
	d.appliedConfig = shared.AppliedConfig{Firewall: d.options, Network: d.settings}

	// The order the core writes these in, because the audit log is the thing a
	// visitor reads to understand what easywall records.
	//
	// This used to write apply_accepted here, before the confirmation window had
	// even opened, and stamp the last-apply time with it. An apply nobody
	// confirmed therefore produced "Rules accepted" immediately followed by
	// "Rules rolled back" for the same apply — a pair the real product cannot
	// produce — and left the dashboard reporting a successful apply that had been
	// undone. audit-log.md teaches operators to read exactly those two lines.
	if d.system.Acceptance.Enabled {
		d.audit("apply_started", "all", "")
		d.acceptance = shared.AcceptancePending
		dur := time.Duration(d.system.Acceptance.Duration) * time.Second
		d.acceptanceTimer = time.AfterFunc(dur, d.rollback)
	} else {
		d.audit("apply_started", "all", "acceptance window disabled — applied without confirmation")
		d.lastApply = time.Now().UTC().Format(time.RFC3339)
		d.audit("apply_accepted", "all", "no confirmation required")
		d.acceptance = shared.AcceptanceAccepted
		// Match the real core: brief "accepted" pulse, then drop back to idle.
		go d.delayedReset()
	}

	raw, _ := json.Marshal(map[string]string{"status": "started"})
	return shared.Response{Success: true, Data: raw}
}

func (d *demoState) handleAccept() shared.Response {
	// Nothing to accept unless a window is open. The core reports that, and so
	// must the demo: a confirmation arriving after the window closed changes
	// nothing, and saying otherwise is the one lie the apply page must not tell.
	if d.acceptance != shared.AcceptancePending {
		return demoOK(shared.AcceptResult{Accepted: false})
	}

	if d.acceptanceTimer != nil {
		d.acceptanceTimer.Stop()
		d.acceptanceTimer = nil
	}
	// Confirmation is what makes an apply final, so this is where the log records
	// it and where the dashboard's "last apply" is stamped.
	d.lastApply = time.Now().UTC().Format(time.RFC3339)
	d.audit("apply_accepted", "all", "")
	d.acceptance = shared.AcceptanceAccepted
	go d.delayedReset()
	return demoOK(shared.AcceptResult{Accepted: true})
}

// rollback fires from a time.AfterFunc when the acceptance window expires
// without an Accept. It must lock independently because it runs on its own
// goroutine.
func (d *demoState) rollback() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rules.Current = d.rules.Backup
	// The rules just reverted; record the configuration alongside them for the
	// same reason Firewall.rollback does.
	d.appliedConfig = shared.AppliedConfig{Firewall: d.options, Network: d.settings}
	d.acceptance = shared.AcceptanceRolledBack
	d.audit("apply_rolledback", "all", "timeout") // the token the real core writes
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
	before := d.options
	d.options = opts
	d.audit("options_saved", "", shared.DescribeStructChange(before, opts))
	return shared.Response{Success: true}
}

func (d *demoState) handleSaveSettings(payload []byte) shared.Response {
	var s shared.NetworkSettings
	if err := json.Unmarshal(payload, &s); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	// The same check the core runs, for the same reason the custom-rule page
	// reports its checker as unavailable rather than answering "no errors": the
	// demo is the product to everyone who has not installed it. It accepted every
	// network here, including ones a real installation refuses.
	if err := shared.ValidateNetworkList("docker custom network", s.Docker.CustomNetworks); err != nil {
		return demoErr(err)
	}
	if err := shared.ValidateNetworkList("routing network", s.Routing.Networks); err != nil {
		return demoErr(err)
	}
	before := d.settings
	d.settings = s
	d.audit("settings_saved", "", shared.DescribeStructChange(before, s))
	return shared.Response{Success: true}
}

func (d *demoState) handleSaveSystem(payload []byte) shared.Response {
	var s shared.SystemSettings
	if err := json.Unmarshal(payload, &s); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	if !shared.ValidAcceptanceDuration(s.Acceptance.Duration) {
		return demoErr(fmt.Errorf("acceptance duration %d is outside %d–%d seconds",
			s.Acceptance.Duration, shared.AcceptanceDurationMin, shared.AcceptanceDurationMax))
	}
	before := d.system
	d.system = s
	d.audit("system_saved", "", shared.DescribeStructChange(before, s))
	return shared.Response{Success: true}
}

func (d *demoState) handleImportRules(payload []byte) shared.Response {
	// The export endpoint returns the Current rules as JSON. Import accepts
	// the same shape and replaces Staged so the user can review + apply.
	var imported shared.Rules
	if err := json.Unmarshal(payload, &imported); err != nil {
		return demoErr(fmt.Errorf("invalid rules: %w", err))
	}
	if err := shared.ValidateRules(imported); err != nil {
		return demoErr(fmt.Errorf("import validation failed: %w", err))
	}
	d.rules.Staged = imported
	d.audit("rules_imported", "", fmt.Sprintf("%d tcp, %d udp, %d blacklist, %d whitelist",
		len(imported.TCP), len(imported.UDP), len(imported.Blacklist), len(imported.Whitelist)))
	return shared.Response{Success: true}
}

// handlePanic mirrors core.Firewall.Panic for a visitor with no kernel behind
// them: there is no table to tear down, so setting panicMode and writing the
// same audit action the real core writes is the whole of it.
func (d *demoState) handlePanic() shared.Response {
	d.panicMode = true
	d.audit("panic_engaged", "all", "the firewall was taken down from the console")
	return shared.Response{Success: true}
}

// handleResume mirrors core.Firewall.Resume. The real core restores the
// stored rules from disk here; the demo has no disk-backed "current" separate
// from what panicMode already suppresses, so clearing the flag is the
// restore.
func (d *demoState) handleResume() shared.Response {
	d.panicMode = false
	d.audit("panic_resumed", "all", "panic mode was ended from the console")
	return shared.Response{Success: true}
}

// handleLogEvent records a login event on the demo's log — refusing one the
// protocol does not declare, because a demo that accepts what production
// refuses is a demo that hides the refusal.
//
// It does not record one the way the core does: the core parses p.Addr with
// netip.ParseAddr and normalises it (loginevents.go), folds bursts of the
// stranger-triggerable events into a debounced summary, and never repeats one
// address twice running. This handler skips all of that and echoes p.Addr
// verbatim. That is not a security problem — p.Addr is r.RemoteAddr, not
// attacker-controlled input, and detailLabel escapes everything it did not
// write itself — but it is not what the core does, so the comment must not
// claim it is.
//
// What it does share with the core is the log's cap: d.audit() below caps at
// 200 entries, and a handler that appended straight to d.auditLog without that
// cap turned 300 login attempts on the public demo into a /log page that grew
// without bound — a slow leak, and a page that buries what the demo exists to
// show.
func (d *demoState) handleLogEvent(payload []byte) shared.Response {
	var p shared.LogEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return demoErr(fmt.Errorf("invalid payload: %w", err))
	}
	if !shared.ValidLoginEvent(p.Event) {
		return demoErr(fmt.Errorf("unknown login event: %q", p.Event))
	}
	detail := ""
	if p.Addr != "" {
		detail = "from " + p.Addr
		if p.Proxied {
			// The same token the core writes. The public demo is behind nginx,
			// which is where this was noticed, so the demo has to show it.
			detail += shared.ProxyToken
		}
	}
	d.auditAs(string(p.Event), "", detail, "web")
	return shared.Response{Success: true}
}
