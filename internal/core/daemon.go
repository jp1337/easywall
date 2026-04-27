package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
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
	listener net.Listener
	wg       sync.WaitGroup
	quit     chan struct{}
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

	var err error
	d.listener, err = net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.cfg.SocketPath, err)
	}

	// Set socket permissions: root:easywall 0660
	if err := os.Chmod(d.cfg.SocketPath, 0660); err != nil {
		slog.Warn("could not chmod socket", "error", err)
	}
	if gid, err := lookupGroup("easywall"); err == nil {
		if err := os.Chown(d.cfg.SocketPath, 0, gid); err != nil {
			slog.Warn("could not chown socket to easywall group", "error", err)
		}
	}

	slog.Info("daemon listening", "socket", d.cfg.SocketPath)

	for {
		conn, err := d.listener.Accept()
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

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() {
	close(d.quit)
	if d.listener != nil {
		_ = d.listener.Close()
	}
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
		if err := d.firewall.RulesStore().SaveStaged(payload.RuleType, payload.Rules); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "rules_saved", payload.RuleType, "", "web")
		return shared.Response{Success: true}

	case shared.CmdApplyRules:
		// Apply runs asynchronously to avoid blocking the socket.
		// The caller must poll CmdGetStatus to track acceptance progress.
		go func() {
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
		d.firewall.Accept()
		return shared.Response{Success: true}

	case shared.CmdGetOptions:
		opts := d.firewall.Options()
		data, _ := json.Marshal(opts)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveOptions:
		var opts shared.FirewallOptions
		if err := json.Unmarshal(cmd.Payload, &opts); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		if err := d.cfg.SaveFirewallOptions(opts); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "options_saved", "", "", "web")
		return shared.Response{Success: true}

	case shared.CmdGetSettings:
		s := shared.NetworkSettings{IPv6: d.cfg.IPv6, Docker: d.cfg.Docker}
		data, _ := json.Marshal(s)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveSettings:
		var s shared.NetworkSettings
		if err := json.Unmarshal(cmd.Payload, &s); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		if err := d.cfg.SaveNetworkSettings(s); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "settings_saved", "", "", "web")
		return shared.Response{Success: true}

	case shared.CmdGetSystem:
		s := shared.SystemSettings{Acceptance: d.cfg.Acceptance}
		data, _ := json.Marshal(s)
		return shared.Response{Success: true, Data: data}

	case shared.CmdSaveSystem:
		var s shared.SystemSettings
		if err := json.Unmarshal(cmd.Payload, &s); err != nil {
			return errResp(fmt.Errorf("invalid payload: %w", err))
		}
		if s.Acceptance.Duration <= 0 {
			return errResp(fmt.Errorf("acceptance.duration must be > 0"))
		}
		if err := d.cfg.SaveSystemSettings(s); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "system_saved", "", "", "web")
		return shared.Response{Success: true}

	case shared.CmdGetLog:
		entries, err := readAuditLog(d.cfg.AuditLogPath(), 200)
		if err != nil && !os.IsNotExist(err) {
			return errResp(err)
		}
		data, _ := json.Marshal(entries)
		return shared.Response{Success: true, Data: data}

	case shared.CmdExportRules:
		data, err := d.firewall.RulesStore().ExportCurrent()
		if err != nil {
			return errResp(err)
		}
		return shared.Response{Success: true, Data: data}

	case shared.CmdImportRules:
		if err := d.firewall.RulesStore().ImportRules(cmd.Payload); err != nil {
			return errResp(err)
		}
		WriteAuditLog(d.cfg.AuditLogPath(), "rules_imported", "all", "", "web")
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

// readAuditLog reads the last n entries from the audit log file (most-recent first).
func readAuditLog(path string, n int) ([]shared.AuditLogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
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
