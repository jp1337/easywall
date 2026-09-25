package shared

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

// echoServer answers one command with the given response and records what it saw.
func echoServer(t *testing.T, resp Response) (socketPath string, received *Command) {
	t.Helper()
	out, _ := json.Marshal(resp)
	return rawServer(t, out)
}

// rawServer answers one command with exactly these bytes.
func rawServer(t *testing.T, out []byte) (socketPath string, received *Command) {
	t.Helper()
	socketPath = filepath.Join(t.TempDir(), "core.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	received = &Command{}
	done := make(chan struct{})
	// Cleanups run last-first: the listener closes before the wait, so a test
	// whose client never connects fails instead of hanging in Accept.
	t.Cleanup(func() { <-done })
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, err := io.ReadAll(io.LimitReader(conn, MaxMessageBytes+1))
		if err != nil {
			return
		}
		_ = json.Unmarshal(data, received)
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

// commandOfSize is a command whose marshalled form is exactly n bytes.
func commandOfSize(t *testing.T, n int) Command {
	t.Helper()
	cmd := Command{Type: CmdUpdateFeed, Payload: json.RawMessage(`""`)}
	base, _ := json.Marshal(cmd)
	cmd.Payload = json.RawMessage(`"` + strings.Repeat("a", n-len(base)) + `"`)
	if out, _ := json.Marshal(cmd); len(out) != n {
		t.Fatalf("built %d bytes, want %d", len(out), n)
	}
	return cmd
}

// 2.23: the limit is MaxMessageBytes, and a command over it is refused before
// anything is sent — named, and recognisable with errors.Is, so a feed refresh
// can be recorded as too large instead of as a broken pipe.
func TestSendCommand_RequestLimitIsExact(t *testing.T) {
	socketPath, received := echoServer(t, Response{Success: true})
	if _, err := SendCommand(socketPath, commandOfSize(t, MaxMessageBytes)); err != nil {
		t.Fatalf("a command of exactly MaxMessageBytes was refused: %v", err)
	}
	if received.Type != CmdUpdateFeed {
		t.Errorf("the server saw %q", received.Type)
	}

	// No server behind this path: a refusal must not need one.
	_, err := SendCommand(filepath.Join(t.TempDir(), "nope.sock"), commandOfSize(t, MaxMessageBytes+1))
	if !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("one byte over the limit: got %v, want ErrRequestTooLarge", err)
	}
	if !strings.Contains(err.Error(), ErrRequestTooLargeText) {
		t.Errorf("the error does not say %q: %v", ErrRequestTooLargeText, err)
	}
}

// A reply longer than the limit is an error that says so. It used to be cut at
// 1 MiB and reported as "parse response: unexpected end of JSON input".
func TestSendCommand_ResponseLimitIsExact(t *testing.T) {
	head := []byte(`{"success":true}`)
	exact := append(head, []byte(strings.Repeat(" ", MaxMessageBytes-len(head)))...)
	socketPath, _ := rawServer(t, exact)
	resp, err := SendCommand(socketPath, Command{Type: CmdGetFeeds})
	if err != nil || !resp.Success {
		t.Fatalf("a reply of exactly MaxMessageBytes: %+v, %v", resp, err)
	}

	socketPath, _ = rawServer(t, append(exact, ' '))
	_, err = SendCommand(socketPath, Command{Type: CmdGetFeeds})
	if err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Errorf("one byte over the limit: got %v, want an error naming the length", err)
	}
}
