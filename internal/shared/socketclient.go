package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// DialTimeout bounds only reaching the socket. Connecting to a local Unix socket
// is immediate or it is not going to happen, whatever the command is.
const DialTimeout = 5 * time.Second

// SendCommand sends one command to a core daemon socket and returns its reply.
//
// It lives in shared, not in internal/web, because there are two callers now: the
// web process and the `easywall-core` console subcommands. The privileged binary
// must not import the web package to reach its own daemon.
func SendCommand(socketPath string, cmd Command) (Response, error) {
	out, err := json.Marshal(cmd)
	if err != nil {
		return Response{}, fmt.Errorf("marshal command: %w", err)
	}
	// Refused here rather than sent: the core stops reading at the limit, so
	// the rest of the write would fail with a broken pipe and the caller would
	// never see the core's own "request too large".
	if len(out) > MaxMessageBytes {
		return Response{}, fmt.Errorf("send %s: %w: %d bytes, the limit is %d",
			cmd.Type, ErrRequestTooLarge, len(out), MaxMessageBytes)
	}

	conn, err := net.DialTimeout("unix", socketPath, DialTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("connect to core: %w", err)
	}
	defer conn.Close() //nolint:errcheck // the read below is what matters

	// Per command, not one number for all of them. Two run nft while the caller
	// waits and the core bounds that at NftTimeout, so a flat five seconds meant
	// giving up on work the core went on to finish — see CommandTimeout.
	_ = conn.SetDeadline(time.Now().Add(CommandTimeout(cmd.Type)))

	if _, err := conn.Write(out); err != nil {
		return Response{}, fmt.Errorf("send command: %w", err)
	}

	// Signal EOF so the daemon's io.ReadAll returns.
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}

	// One byte past the limit, so a reply that is too long is an error that
	// says so rather than JSON cut short and reported as unparseable.
	data, err := io.ReadAll(io.LimitReader(conn, MaxMessageBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if len(data) > MaxMessageBytes {
		return Response{}, fmt.Errorf("read response: longer than %d bytes", MaxMessageBytes)
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return Response{}, fmt.Errorf("parse response: %w", err)
	}
	return resp, nil
}
