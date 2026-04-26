package web

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestNewCoreClient(t *testing.T) {
	c := NewCoreClient("/run/easywall/core.sock")
	if c == nil {
		t.Fatal("NewCoreClient returned nil")
	}
	if c.socketPath != "/run/easywall/core.sock" {
		t.Errorf("unexpected socket path: %s", c.socketPath)
	}
}

func TestCoreClient_Send_Success(t *testing.T) {
	fc := newFakeCore(t)
	expected := shared.Response{Success: true}
	fc.SetResponse(shared.CmdGetStatus, expected)

	client := NewCoreClient(fc.socketPath)
	resp, err := client.Send(shared.Command{Type: shared.CmdGetStatus})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !resp.Success {
		t.Error("expected success response")
	}
}

func TestCoreClient_Send_ConnectError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	_, err := client.Send(shared.Command{Type: shared.CmdGetStatus})
	if err == nil {
		t.Error("expected error when connecting to nonexistent socket")
	}
}

func TestCoreClient_GetStatus_Success(t *testing.T) {
	fc := newFakeCore(t)
	status := shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptanceIdle,
		HasPending: true,
		LastApply:  "2026-01-01T12:00:00Z",
	}
	data, _ := json.Marshal(status)
	fc.SetResponse(shared.CmdGetStatus, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	got, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !got.Active {
		t.Error("expected Active=true")
	}
	if got.Acceptance != shared.AcceptanceIdle {
		t.Errorf("unexpected acceptance: %s", got.Acceptance)
	}
	if !got.HasPending {
		t.Error("expected HasPending=true")
	}
}

func TestCoreClient_GetStatus_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, shared.Response{Success: false, Error: "internal error"})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("expected error on core error response")
	}
}

func TestCoreClient_GetStatus_BadJSON(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetStatus, shared.Response{Success: true, Data: json.RawMessage(`not-json`)})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("expected error on invalid JSON data")
	}
}

// TestCoreClient_GetStatus_BadDataType sends valid JSON that isn't a FirewallStatus.
// This covers the inner json.Unmarshal error path for the Data field.
func TestCoreClient_GetStatus_BadDataType(t *testing.T) {
	fc := newFakeCore(t)
	// Valid JSON array can't be unmarshaled into FirewallStatus struct
	data, _ := json.Marshal([]string{"not", "a", "status"})
	fc.SetResponse(shared.CmdGetStatus, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("expected error when data is wrong type")
	}
}

func TestCoreClient_GetRules_Success(t *testing.T) {
	fc := newFakeCore(t)
	state := shared.RulesState{
		Staged: shared.Rules{
			TCP: []shared.PortRule{{Port: "80", Description: "HTTP"}},
		},
	}
	data, _ := json.Marshal(state)
	fc.SetResponse(shared.CmdGetRules, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	got, err := client.GetRules()
	if err != nil {
		t.Fatalf("GetRules: %v", err)
	}
	if len(got.Staged.TCP) != 1 {
		t.Errorf("expected 1 TCP rule, got %d", len(got.Staged.TCP))
	}
}

func TestCoreClient_GetRules_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetRules, shared.Response{Success: false, Error: "rules error"})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetRules()
	if err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_GetRules_BadJSON(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetRules, shared.Response{Success: true, Data: json.RawMessage(`{bad}`)})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetRules()
	if err == nil {
		t.Error("expected error on invalid JSON")
	}
}

// TestCoreClient_GetRules_BadDataType sends valid JSON that isn't a RulesState.
func TestCoreClient_GetRules_BadDataType(t *testing.T) {
	fc := newFakeCore(t)
	data, _ := json.Marshal("just a string, not a rules state")
	fc.SetResponse(shared.CmdGetRules, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetRules()
	if err == nil {
		t.Error("expected error when data is wrong type")
	}
}

func TestCoreClient_SaveRules_Success(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	client := NewCoreClient(fc.socketPath)
	rules := []shared.PortRule{{Port: "443", Description: "HTTPS"}}
	if err := client.SaveRules("tcp", rules); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
}

func TestCoreClient_SaveRules_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: false, Error: "save failed"})

	client := NewCoreClient(fc.socketPath)
	if err := client.SaveRules("tcp", []shared.PortRule{}); err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_ApplyRules_Success(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdApplyRules, shared.Response{Success: true})

	client := NewCoreClient(fc.socketPath)
	if err := client.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}
}

func TestCoreClient_ApplyRules_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdApplyRules, shared.Response{Success: false, Error: "apply failed"})

	client := NewCoreClient(fc.socketPath)
	if err := client.ApplyRules(); err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_Accept_Success(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdAccept, shared.Response{Success: true})

	client := NewCoreClient(fc.socketPath)
	if err := client.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

func TestCoreClient_Accept_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdAccept, shared.Response{Success: false, Error: "accept failed"})

	client := NewCoreClient(fc.socketPath)
	if err := client.Accept(); err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_GetOptions_Success(t *testing.T) {
	fc := newFakeCore(t)
	opts := shared.FirewallOptions{SSHBruteForce: true, ICMPFlood: false}
	data, _ := json.Marshal(opts)
	fc.SetResponse(shared.CmdGetOptions, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	got, err := client.GetOptions()
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}
	if !got.SSHBruteForce {
		t.Error("expected SSHBruteForce=true")
	}
}

func TestCoreClient_GetOptions_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetOptions, shared.Response{Success: false, Error: "options error"})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetOptions()
	if err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_GetOptions_BadJSON(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetOptions, shared.Response{Success: true, Data: json.RawMessage(`notjson`)})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetOptions()
	if err == nil {
		t.Error("expected error on invalid JSON")
	}
}

// TestCoreClient_GetOptions_BadDataType sends valid JSON that isn't FirewallOptions.
func TestCoreClient_GetOptions_BadDataType(t *testing.T) {
	fc := newFakeCore(t)
	data, _ := json.Marshal([]int{1, 2, 3})
	fc.SetResponse(shared.CmdGetOptions, shared.Response{Success: true, Data: data})

	client := NewCoreClient(fc.socketPath)
	_, err := client.GetOptions()
	if err == nil {
		t.Error("expected error when data is wrong type")
	}
}

func TestCoreClient_ExportRules_Success(t *testing.T) {
	fc := newFakeCore(t)
	exportData := json.RawMessage(`{"tcp":[],"udp":[]}`)
	wrappedData, _ := json.Marshal(exportData)
	fc.SetResponse(shared.CmdExportRules, shared.Response{Success: true, Data: wrappedData})

	client := NewCoreClient(fc.socketPath)
	got, err := client.ExportRules()
	if err != nil {
		t.Fatalf("ExportRules: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil export data")
	}
}

func TestCoreClient_ExportRules_NotDoubleEncoded(t *testing.T) {
	fc := newFakeCore(t)
	// Simulate data that is already raw JSON (not double-encoded)
	rawData := json.RawMessage(`{"tcp":[]}`)
	fc.SetResponse(shared.CmdExportRules, shared.Response{Success: true, Data: rawData})

	client := NewCoreClient(fc.socketPath)
	_, err := client.ExportRules()
	if err != nil {
		t.Fatalf("ExportRules: %v", err)
	}
}

func TestCoreClient_ExportRules_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdExportRules, shared.Response{Success: false, Error: "export error"})

	client := NewCoreClient(fc.socketPath)
	_, err := client.ExportRules()
	if err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_ImportRules_Success(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdImportRules, shared.Response{Success: true})

	client := NewCoreClient(fc.socketPath)
	payload := []byte(`{"tcp":[],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`)
	if err := client.ImportRules(payload); err != nil {
		t.Fatalf("ImportRules: %v", err)
	}
}

func TestCoreClient_ImportRules_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdImportRules, shared.Response{Success: false, Error: "import failed"})

	client := NewCoreClient(fc.socketPath)
	if err := client.ImportRules([]byte(`{}`)); err == nil {
		t.Error("expected error on core error")
	}
}

func TestCoreClient_Send_InvalidJSONResponse(t *testing.T) {
	dir := t.TempDir()
	socketPath := dir + "/bad.sock"

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Drain the request first
		_, _ = io.ReadAll(io.LimitReader(conn, 1<<20))
		// Return non-JSON bytes
		_, _ = conn.Write([]byte("this is not valid json!!!"))
	}()

	client := NewCoreClient(socketPath)
	_, err = client.Send(shared.Command{Type: shared.CmdGetStatus})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("expected 'parse response' in error, got: %v", err)
	}
}

func TestCoreClient_ExportRules_NilData(t *testing.T) {
	fc := newFakeCore(t)
	// Success with nil Data — triggers the "return as-is" fallback in ExportRules
	fc.SetResponse(shared.CmdExportRules, shared.Response{Success: true, Data: nil})

	client := NewCoreClient(fc.socketPath)
	// Should not error — returns the raw (nil) data
	_, err := client.ExportRules()
	if err != nil {
		t.Fatalf("ExportRules with nil data should not error: %v", err)
	}
}

func TestCoreClient_ApplyRules_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	err := client.ApplyRules()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_Accept_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	err := client.Accept()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_ImportRules_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	err := client.ImportRules([]byte(`{}`))
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_GetStatus_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	_, err := client.GetStatus()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_GetRules_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	_, err := client.GetRules()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_GetOptions_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	_, err := client.GetOptions()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_SaveRules_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	err := client.SaveRules("tcp", []shared.PortRule{})
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}

func TestCoreClient_ExportRules_SendError(t *testing.T) {
	client := NewCoreClient("/nonexistent/socket.sock")
	_, err := client.ExportRules()
	if err == nil {
		t.Error("expected error when socket doesn't exist")
	}
}
