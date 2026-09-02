package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Daemon listens on a Unix socket and dispatches typed commands to the Firewall.
// The socket is owned by root:easywall with mode 0660 so the web process
// (running as user easywall) can connect without root privileges.
type Daemon struct {
	cfg      *Config
	firewall *Firewall
	wg       sync.WaitGroup
	quit     chan struct{}
	quitOnce sync.Once

	// mu guards listener only. Start writes it and Stop reads it, and the two
	// are called from different goroutines — Start blocks for the process
	// lifetime, so Stop necessarily runs on another one. Without this, a
	// SIGTERM arriving while the socket is still being set up races the write.
	mu       sync.Mutex
	listener net.Listener

	// login folds bursts of stranger-triggerable login events into two lines.
	// Built on first use so a Daemon constructed by hand in a test still records.
	loginOnce sync.Once
	login     *loginEvents
}

// NewDaemon initialises the daemon. Call Start() to begin accepting connections.
func NewDaemon(cfg *Config) (*Daemon, error) {
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	fw, err := NewFirewall(cfg)
	if err != nil {
		return nil, fmt.Errorf("init firewall: %w", err)
	}

	return &Daemon{
		cfg:      cfg,
		firewall: fw,
		quit:     make(chan struct{}),
	}, nil
}

// Start creates the Unix socket and begins accepting connections.
// Blocks until Stop() is called.
func (d *Daemon) Start() error {
	// If Stop already ran before the restore could start, do not restore.
	// A daemon told to stop before it began must not start writing to the kernel.
	d.mu.Lock()
	select {
	case <-d.quit:
		d.mu.Unlock()
		return nil
	default:
		d.mu.Unlock()
	}

	// One loud entry, once, if the panic marker cannot be read at all.
	//
	// PanicEngaged answers an unreadable marker with "engaged", so the restore
	// below will decline to filter and the machine comes up open. That default
	// is right — the alternative is filtering a machine somebody deliberately
	// unfiltered — but it is indistinguishable from real panic mode everywhere
	// an operator looks: the banner, `easywall-core status` and the status reply
	// all read one boolean. Worse, the same unreadable marker reaches
	// Firewall.rollback, which now goes ahead when the state is unknown
	// precisely because it must not withdraw the acceptance window's undo on a
	// permission fault. So the fault is reported here, in the audit log, with
	// the path and the errno — the one place a fault this systemic is worth a
	// line — rather than by every caller that trips over it.
	//
	// boot_enforce_failed, not an action of its own: the consequence is exactly
	// what that action already means. The stored rules are not going into the
	// kernel at this start, and it is already registered in auditActionLabels,
	// auditActionTones, both locales and the documented colour table.
	if _, known, markerErr := PanicState(d.cfg.PanicMarkerPath()); !known {
		WriteAuditLog(d.cfg.AuditLogPath(), "boot_enforce_failed", "all",
			fmt.Sprintf("%s: cannot read the panic marker %s: %v — the stored rules are "+
				"not being restored, and this machine is not filtering",
				RestoreReasonBoot, d.cfg.PanicMarkerPath(), markerErr), "core")
	}

	// Restore the stored rules before the listener is created. The socket is the
	// only thing that makes this process observable, so putting the kernel work
	// first ensures no client can observe a half-restored firewall by construction.
	//
	// There is a brief window of a few instructions between the quit check above
	// and Add(1) below, but it is tolerable: a restore that installs the already-
	// stored rules on a daemon that is shutting down leaves the kernel holding
	// exactly what the machine is supposed to have, and the next start would do
	// the same thing anyway.
	//
	// The WaitGroup exists so Stop() waits for kernel work, not for client connections.
	// Its scope is the restore only, not the accept loop, so the counter is released
	// the moment the restore returns. Extending it over the loop would couple
	// shutdown correctness to Stop() closing the listener first, a dependency that
	// is not obvious and should not exist.
	d.wg.Add(1)
	func() {
		defer d.wg.Done()
		if err := d.firewall.RestoreCurrent(RestoreReasonBoot); err != nil {
			slog.Error("could not put the stored rules back at startup; this machine "+
				"is not filtering — open the interface and apply, or run "+
				"`easywall-core status` to see why", "error", err)
		}
	}()

	// Tracked in wg so Stop waits for it rather than leaving it polling into a
	// closed daemon. Deliberately outside the closure above: that one releases
	// its wg slot the instant the boot restore returns, specifically so d.wg
	// does not span the whole daemon lifetime (see the comment on it). This
	// goroutine's job only starts once the boot restore has already run, and
	// it can poll for up to reconcileWait — Stop has to wait for it to notice
	// d.quit, not for the boot restore that came before it.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.firewall.reconcileDockerBridges(d.quit)
	}()

	// Remove stale socket file if it exists
	_ = os.Remove(d.cfg.SocketPath)

	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.cfg.SocketPath, err)
	}

	d.mu.Lock()
	select {
	case <-d.quit:
		// Stop already ran: it saw no listener and closed nothing, so this one
		// would leak and keep accepting. Close it here instead.
		d.mu.Unlock()
		_ = ln.Close()
		return nil
	default:
		d.listener = ln
		d.mu.Unlock()
	}

	// Set socket permissions: root:easywall 0660.
	//
	// The group is the entire link to the web process: it runs as easywall, and
	// a socket left owned by root:root is one it cannot connect to at all. That
	// is what the packaged unit produced — CapabilityBoundingSet=CAP_NET_ADMIN
	// leaves a root process without CAP_CHOWN, the chown failed, and this was a
	// warning nobody reads on a daemon that then reported "daemon listening" as
	// though the interface would work. It never did.
	//
	// Passing -1 for the owner asks only for the group to change, which the
	// owner of a file may do for a group it belongs to without any capability.
	// The unit puts the daemon in the easywall group for exactly that reason.
	// #nosec G302 -- 0660 is the point of the socket, not an oversight: the web
	// process runs as easywall and connecting needs write. Read the note above for
	// what the group ownership is doing, and what happened when it was missing.
	if err := os.Chmod(d.cfg.SocketPath, 0660); err != nil {
		slog.Warn("could not chmod socket", "error", err)
	}
	gid, err := lookupGroup("easywall")
	switch {
	case err != nil:
		// No such group: a manual installation that runs both processes as the
		// same user. Nothing to hand over.
		slog.Warn("no easywall group; leaving the socket's group as it is", "error", err)
	default:
		if err := os.Chown(d.cfg.SocketPath, -1, gid); err != nil {
			slog.Error("could not give the socket to the easywall group — "+
				"easywall-web runs as that user and will not be able to connect, "+
				"so the whole interface will report the core as unreachable. "+
				"Run the core as a member of the easywall group (Group=easywall in "+
				"easywall-core.service) or grant it CAP_CHOWN",
				"socket", d.cfg.SocketPath, "error", err)
		}
	}

	slog.Info("daemon listening", "socket", d.cfg.SocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-d.quit:
				return nil
			default:
				slog.Error("accept error", "error", err)
				continue
			}
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConn(conn)
		}()
	}
}

// currentListener returns the active listener, or nil once Stop has run.
// Anything outside Start must go through this rather than reading the field:
// Start writes it from its own goroutine, so an unguarded read is a race.
func (d *Daemon) currentListener() net.Listener {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.listener
}

// setListener installs a listener. Used by Start and by tests that supply
// their own socket.
func (d *Daemon) setListener(ln net.Listener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listener = ln
}

// Stop gracefully shuts down the daemon. Safe to call more than once, and safe
// to call before or during Start.
func (d *Daemon) Stop() {
	d.quitOnce.Do(func() { close(d.quit) })

	d.mu.Lock()
	ln := d.listener
	d.listener = nil
	d.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}

	// End an open acceptance window before waiting, or Stop blocks for as long
	// as that window has left — up to an hour. Cancelling it counts as "not
	// confirmed", so the rules roll back, which is what the window promises: the
	// operator did not confirm, and the reason they did not is that the machine
	// was told to stop.
	//
	// The latching variant, because the window may not be open yet. An apply
	// that is between beginApply and Start — a rules read, a backup write, an
	// nft snapshot subprocess and a promote — would otherwise swallow this
	// cancel and open a full-length window with Stop already waiting on it.
	d.firewall.ShutdownAcceptance()

	d.wg.Wait()
	_ = os.Remove(d.cfg.SocketPath)
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()

	data, err := io.ReadAll(io.LimitReader(conn, 1<<20)) // 1MB max
	if err != nil {
		slog.Warn("read command error", "error", err)
		return
	}

	var cmd shared.Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		d.sendError(conn, "invalid JSON command")
		return
	}

	resp := d.dispatch(cmd)
	out, _ := json.Marshal(resp)
	_, _ = conn.Write(out)
}

func (d *Daemon) dispatch(cmd shared.Command) shared.Response {
	switch cmd.Type {
	case shared.CmdGetStatus:
		status := d.firewall.Status()
		data, _ := json.Marshal(status)
		return shared.Response{Success: true, Data: data}

	case shared.CmdGetRules:
		state, err := d.firewall.RulesStore().GetState()
		if err != nil {
			return errResp(err)
		}
		data, _ := json.Marshal(state)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveRules:
		var payload shared.SaveRulesPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		// Read the set being replaced first, so the audit entry can say what
		// changed rather than only that something did.
		before, _ := d.firewall.RulesStore().GetState()
		if err := d.firewall.RulesStore().SaveStaged(payload.RuleType, payload.Rules); err != nil {
			return errResp(err)
		}
		after, _ := d.firewall.RulesStore().GetState()
		WriteAuditLog(d.cfg.AuditLogPath(), "rules_saved", payload.RuleType,
			shared.DescribeRuleChange(payload.RuleType, before.Staged, after.Staged), "web")
		return shared.Response{Success: true}

	case shared.CmdApplyRules:
		// Checked synchronously, before the slot and before the goroutine, for
		// the same reason ErrApplyInProgress is checked synchronously below:
		// the answer the caller needs is "did this start", and apply() already
		// refuses this case on its own — but apply() runs inside the goroutine
		// below, where its error is only logged. Without this, a browser
		// clicking Apply while the console has run `panic` got back
		// {"status":"started"} and never anything else; the operator had no
		// way to learn their apply did nothing short of reading the daemon's
		// own log. This check can still race a `panic` that lands after it and
		// before beginApply — apply()'s own check is what closes that window;
		// this one only makes the ordinary case visible.
		if d.firewall.PanicEngaged() {
			return errResp(ErrPanicEngaged)
		}

		// The slot is claimed here, synchronously, before anything is reported.
		// This used to start the goroutine unconditionally and answer "started"
		// every time, which was untrue for the second request: it did not start,
		// it queued inside Apply's mutex until the open acceptance window closed,
		// and then ran on its own. See ErrApplyInProgress for what that did to an
		// operator, and to shutdown.
		if !d.firewall.beginApply() {
			return errResp(ErrApplyInProgress)
		}

		// Apply runs asynchronously to avoid blocking the socket.
		// The caller must poll CmdGetStatus to track acceptance progress.
		//
		// Tracked in d.wg so Stop waits for it. Until it was, stopping in the
		// middle of an acceptance window — a package upgrade, systemctl
		// restart, a SIGTERM — abandoned the goroutine holding that window: the
		// unconfirmed rules stayed live and the rollback never ran.
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			defer d.firewall.endApply()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("apply panic recovered", "error", r)
				}
			}()
			if err := d.firewall.apply("web"); err != nil {
				slog.Error("apply error", "error", err)
			}
		}()
		data, _ := json.Marshal(map[string]string{"status": "started"})
		return shared.Response{Success: true, Data: data}

	case shared.CmdAccept:
		// Whether it landed is the answer the caller needs: a confirmation that
		// arrives after the window closed must not be reported as success.
		data, _ := json.Marshal(shared.AcceptResult{Accepted: d.firewall.Accept()})
		return shared.Response{Success: true, Data: data}

	case shared.CmdGetOptions:
		opts := d.firewall.Options()
		data, _ := json.Marshal(opts)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveOptions:
		var opts shared.FirewallOptions
		if err := json.Unmarshal(cmd.Payload, &opts); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		changed := shared.DescribeStructChange(d.firewall.Options(), opts)
		if err := d.cfg.SaveFirewallOptions(opts); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "options_saved", "", changed, "web")
		return shared.Response{Success: true}

	case shared.CmdGetSettings:
		s := d.cfg.NetworkSettings()
		data, _ := json.Marshal(s)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveSettings:
		var s shared.NetworkSettings
		if err := json.Unmarshal(cmd.Payload, &s); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		changed := shared.DescribeStructChange(d.cfg.NetworkSettings(), s)
		if err := d.cfg.SaveNetworkSettings(s); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "settings_saved", "", changed, "web")
		return shared.Response{Success: true}

	case shared.CmdGetSystem:
		s := d.cfg.SystemSettings()
		data, _ := json.Marshal(s)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveSystem:
		var s shared.SystemSettings
		if err := json.Unmarshal(cmd.Payload, &s); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		changed := shared.DescribeStructChange(d.cfg.SystemSettings(), s)
		if err := d.cfg.SaveSystemSettings(s); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "system_saved", "", changed, "web")
		return shared.Response{Success: true}

	case shared.CmdGetLog:
		entries, err := readAuditLog(d.cfg.AuditLogPath(), 200)
		if err != nil && !os.IsNotExist(err) {
			return errResp(err)
		}
		data, _ := json.Marshal(entries)
		return shared.Response{Success: true, Data: data}

	case shared.CmdExportRules:
		data, err := d.firewall.RulesStore().ExportStaged()
		if err != nil {
			return errResp(err)
		}
		return shared.Response{Success: true, Data: data}

	case shared.CmdImportRules:
		// The editor runs every custom rule past `nft --check` before saving it;
		// import did not, so a file could carry a rule the editor would have
		// refused. Same check, same path.
		var incoming shared.Rules
		if err := json.Unmarshal(cmd.Payload, &incoming); err != nil {
			return errResp(fmt.Errorf("invalid import data: %w", err))
		}
		errs, err := validateCustomRules(incoming.Custom)
		if err != nil {
			return errResp(err)
		}
		if len(errs) > 0 {
			lines := make([]string, 0, len(errs))
			for i, msg := range errs {
				lines = append(lines, fmt.Sprintf("custom rule %d: %s", i+1, msg))
			}
			sort.Strings(lines)
			return errResp(fmt.Errorf("import validation failed: %s", strings.Join(lines, "; ")))
		}
		if err := d.firewall.RulesStore().ImportRules(cmd.Payload); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "rules_imported", "all",
			fmt.Sprintf("%d tcp, %d udp, %d blacklist, %d whitelist",
				len(incoming.TCP), len(incoming.UDP),
				len(incoming.Blacklist), len(incoming.Whitelist)), "web")
		return shared.Response{Success: true}

	case shared.CmdValidateCustom:
		var payload shared.ValidateCustomPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		errs, err := validateCustomRules(payload.Rules)
		if err != nil {
			return errResp(err)
		}
		result := shared.ValidateCustomResult{Errors: errs}
		data, _ := json.Marshal(result)
		return shared.Response{Success: true, Data: data}

	case shared.CmdGetAppliedConfig:
		res, err := readAppliedConfig(d.cfg.AppliedConfigPath())
		if err != nil {
			return errResp(err)
		}
		data, _ := json.Marshal(res)
		return shared.Response{Success: true, Data: data}

	case shared.CmdPanic:
		if err := d.firewall.Panic("console"); err != nil {
			return errResp(err)
		}
		return shared.Response{Success: true}

	case shared.CmdResume:
		if err := d.firewall.Resume("console"); err != nil {
			return errResp(err)
		}
		return shared.Response{Success: true}

	case shared.CmdLogEvent:
		var p shared.LogEventPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		if err := d.recordLoginEvent(p); err != nil {
			return errResp(err)
		}
		return shared.Response{Success: true}

	default:
		return shared.Response{Success: false, Error: fmt.Sprintf("unknown command: %s", cmd.Type)}
	}
}

func (d *Daemon) sendError(conn net.Conn, msg string) {
	resp := shared.Response{Success: false, Error: msg}
	out, _ := json.Marshal(resp)
	_, _ = conn.Write(out)
}

func errResp(err error) shared.Response {
	return shared.Response{Success: false, Error: err.Error()}
}

// recordLoginEvent hands one login event to the debouncer.
func (d *Daemon) recordLoginEvent(p shared.LogEventPayload) error {
	return d.loginEventLog().record(p, time.Now())
}

// loginEventLog builds the debouncer on first use and starts its sweeper.
func (d *Daemon) loginEventLog() *loginEvents {
	d.loginOnce.Do(func() {
		d.login = newLoginEvents(func(action, ruleType, detail, user string) {
			WriteAuditLog(d.cfg.AuditLogPath(), action, ruleType, detail, user)
		})
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.login.run(d.quit)
		}()
	})
	return d.login
}

// auditTailBytes is how much of the end of the audit log is read to satisfy a
// request for the most recent entries.
//
// An entry is around 150 bytes, so this holds well over a thousand — several
// times the 200 the viewer asks for. The file is append-only and rotated by
// logrotate, which the Debian package configures but a manual install may not;
// reading all of it to show the newest 200 meant the dashboard, which reads the
// log on every load, grew slower for the lifetime of the host.
const auditTailBytes = 256 * 1024

// readAuditLog reads the last n entries from the audit log file (most-recent first).
func readAuditLog(path string, n int) ([]shared.AuditLogEntry, error) {
	data, truncated, err := tailFile(path, auditTailBytes)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	// The first line of a window that does not start at the beginning of the
	// file is almost certainly half an entry. Dropping it costs one record out
	// of a thousand and keeps a mangled one out of the log view.
	if truncated && len(lines) > 0 {
		lines = lines[1:]
	}
	// Reverse so most-recent first
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	entries := make([]shared.AuditLogEntry, 0, min(n, len(lines)))
	for _, line := range lines {
		if len(entries) >= n {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e shared.AuditLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// tailFile returns at most max bytes from the end of the file, and whether it
// had to skip anything to do so.
func tailFile(path string, max int64) ([]byte, bool, error) {
	// #nosec G304 -- the only caller passes cfg.AuditLogPath(), built from log_dir
	// in the daemon's own config. No request names a file to read.
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close() //nolint:errcheck // read-only

	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}

	size := info.Size()
	if size <= max {
		data, err := io.ReadAll(f)
		return data, false, err
	}

	if _, err := f.Seek(size-max, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(f)
	return data, true, err
}

// maxCustomRules bounds how many statements one request may ask the daemon to
// check. Well above any real rule set — the editor is a textarea and a long one
// holds a few dozen lines — and far below what a 64 KB form body can carry,
// which is about three thousand.
const maxCustomRules = 256

// validateCustomRules checks the rules with "nft --check" and returns a map of
// line-index to error string; an empty map means all of them are valid.
//
// Two passes, because one fork is cheap and N is not. Every statement goes into
// a single script first, which answers the ordinary case — a rule set that is
// valid — with one subprocess. Only when that script is rejected does it fall
// back to checking each statement on its own, which is the only way to say
// *which* line is wrong.
//
// It matters because this runs on the root daemon, once per keystroke pause in
// the editor, and there was no bound on it at all: measured at ~15 ms per
// statement, a 64 KB body of one-line rules asked for roughly 50 seconds of
// forking, serially, per request — with nothing cancelling it when the web
// process gave up after five. The count is capped and the whole validation
// shares one deadline rather than granting each statement its own.
func validateCustomRules(rules []string) (map[int]string, error) {
	if len(rules) > maxCustomRules {
		return nil, fmt.Errorf("too many custom rules: %d, the limit is %d", len(rules), maxCustomRules)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nftTimeout)
	defer cancel()

	if checkCustomScript(ctx, rules) == nil {
		return map[int]string{}, nil
	}
	return validateCustomRulesEach(ctx, rules), nil
}

// checkCustomScript checks every statement in one table, in one nft run.
func checkCustomScript(ctx context.Context, rules []string) error {
	var body strings.Builder
	found := false
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue
		}
		body.WriteString("    " + rule + "\n")
		found = true
	}
	if !found {
		return nil
	}
	_, err := runNftCheck(ctx,
		"table inet easywall_validate {\n  chain test_input {\n    type filter hook input priority 0;\n"+
			body.String()+"  }\n}\n")
	return err
}

// validateCustomRulesEach attributes failures to individual lines.
func validateCustomRulesEach(ctx context.Context, rules []string) map[int]string {
	errs := make(map[int]string)
	for i, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue // skip blanks and comments
		}
		// Wrap in a test table context for nft -c
		script := "table inet easywall_validate {\n  chain test_input {\n    type filter hook input priority 0;\n    " + rule + "\n  }\n}\n"
		out, err := runNftCheck(ctx, script)
		// The deadline is shared by the whole validation rather than granted to
		// each statement: one request must not be able to hold the daemon for
		// the timeout times the number of lines.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			errs[i] = fmt.Sprintf("syntax check timed out after %s", nftTimeout)
			continue
		}
		if err != nil {
			msg := strings.TrimSpace(out)
			if msg == "" {
				msg = err.Error()
			}
			// Strip the internal table name from error messages for cleaner output
			msg = strings.ReplaceAll(msg, "easywall_validate", "easywall")
			errs[i] = msg
		}
	}
	return errs
}

// runNftCheck passes a script to "nft --check --file -" and returns its output.
func runNftCheck(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, nftBinary, "--check", "--file", "-")
	// Killing the process is not enough on its own: CombinedOutput waits for the
	// output pipes to close, and anything the child spawned still holds them.
	// WaitDelay bounds that too, so a cancelled command really does return.
	cmd.WaitDelay = nftWaitDelay
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// groupFilePath is the path to the system group file; overridden in tests.
var groupFilePath = "/etc/group"

// lookupGroup returns the numeric GID for a group name by parsing /etc/group.
// Uses bufio.Scanner to avoid loading the entire file into memory.
func lookupGroup(name string) (int, error) {
	f, err := os.Open(groupFilePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// /etc/group format: name:password:gid:members
		fields := strings.SplitN(scanner.Text(), ":", 4)
		if len(fields) >= 3 && fields[0] == name {
			var gid int
			if _, err := fmt.Sscanf(fields[2], "%d", &gid); err == nil {
				return gid, nil
			}
		}
	}
	return 0, fmt.Errorf("group %q not found", name)
}
