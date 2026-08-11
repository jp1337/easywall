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
	d.firewall.CancelAcceptance()

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
			defer func() {
				if r := recover(); r != nil {
					slog.Error("apply panic recovered", "error", r)
				}
			}()
			if err := d.firewall.Apply("web"); err != nil {
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
		if errs := validateCustomRules(incoming.Custom); len(errs) > 0 {
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
		errs := validateCustomRules(payload.Rules)
		result := shared.ValidateCustomResult{Errors: errs}
		data, _ := json.Marshal(result)
		return shared.Response{Success: true, Data: data}

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

// validateCustomRules checks each rule by wrapping it in a minimal nft table
// and passing it to "nft --check --file -". Returns a map of line-index to
// error string for any rule that fails; an empty map means all rules are valid.
func validateCustomRules(rules []string) map[int]string {
	errs := make(map[int]string)
	for i, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue // skip blanks and comments
		}
		// Wrap in a test table context for nft -c
		script := "table inet easywall_validate {\n  chain test_input {\n    type filter hook input priority 0;\n    " + rule + "\n  }\n}\n"
		// Bounded, like the apply path: the web client gives up after five
		// seconds, but an unbounded nft here would leave the goroutine behind
		// for the life of the process, once per validation request.
		ctx, cancel := context.WithTimeout(context.Background(), nftTimeout)
		cmd := exec.CommandContext(ctx, nftBinary, "--check", "--file", "-")
		// Killing the process is not enough on its own: CombinedOutput waits for
		// the output pipes to close, and anything the child spawned still holds
		// them. WaitDelay bounds that too, so a cancelled command really does
		// return.
		cmd.WaitDelay = nftWaitDelay
		cmd.Stdin = strings.NewReader(script)
		out, err := cmd.CombinedOutput()
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()
		// Specifically DeadlineExceeded: cancel() above makes ctx.Err() non-nil
		// either way, so a plain nil check reports every successful validation
		// as a timeout.
		if timedOut {
			errs[i] = fmt.Sprintf("syntax check timed out after %s", nftTimeout)
			continue
		}
		if err != nil {
			msg := strings.TrimSpace(string(out))
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
