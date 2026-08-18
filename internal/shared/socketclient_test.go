package shared

import (
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

// echoServer answers one command with the given response and records what it saw.
func echoServer(t *testing.T, resp Response) (socketPath string, received *Command) {
	t.Helper()
	socketPath = filepath.Join(t.TempDir(), "core.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	received = &Command{}
	done := make(chan struct{})
	t.Cleanup(func() { <-done })

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
		if err != nil {
			return
		}
		_ = json.Unmarshal(data, received)
		out, _ := json.Marshal(resp)
		_, _ = conn.Write(out)
	}()
	return socketPath, received
}

func TestSendCommand_RoundTrip(t *testing.T) {
	socketPath, received := echoServer(t, Response{Success: true, Data: json.RawMessage(`{"ok":true}`)})

	resp, err := SendCommand(socketPath, Command{Type: CmdGetStatus})
	if err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, error %q", resp.Error)
	}
	if string(resp.Data) != `{"ok":true}` {
		t.Errorf("Data = %s", resp.Data)
	}
	if received.Type != CmdGetStatus {
		t.Errorf("the server saw %q, want %q", received.Type, CmdGetStatus)
	}
}

// The error a console user is most likely to meet: the daemon is not running.
// It has to be recognisable, because the CLI branches on it to decide whether it
// may touch nftables itself.
func TestSendCommand_AbsentSocket(t *testing.T) {
	_, err := SendCommand(filepath.Join(t.TempDir(), "nope.sock"), Command{Type: CmdGetStatus})
	if err == nil {
		t.Fatal("want an error for a socket that is not there")
	}
	if !strings.Contains(err.Error(), "connect to core") {
		t.Errorf("error should name the failure to connect, got %v", err)
	}
}
