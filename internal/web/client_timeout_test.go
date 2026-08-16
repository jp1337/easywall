package web

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// slowCore answers every command after delay, like a core whose nft is busy.
func slowCore(t *testing.T, delay time.Duration) string {
	t.Helper()
	// Short path: sun_path is 108 bytes and t.TempDir() is not short.
	dir, err := os.MkdirTemp("", "ew")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "c.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := make([]byte, 4096)
				n, _ := conn.Read(buf)
				var cmd shared.Command
				_ = json.Unmarshal(buf[:n], &cmd)

				time.Sleep(delay)

				resp := shared.Response{Success: true}
				// Answer with the shape each command's decoder expects, so a
				// failure here is about the deadline and nothing else.
				switch cmd.Type {
				case shared.CmdValidateCustom:
					resp.Data, _ = json.Marshal(shared.ValidateCustomResult{Errors: map[int]string{}})
				case shared.CmdGetStatus:
					resp.Data, _ = json.Marshal(shared.FirewallStatus{})
				}
				out, _ := json.Marshal(resp)
				_, _ = conn.Write(out)
			}()
		}
	}()
	return sock
}

// An import must not be reported as failed while the core is still doing it.
//
// The client had one deadline for all fifteen commands, five seconds, and
// IMPORT_RULES runs every custom rule past `nft --check` first — which the core
// bounds at shared.NftTimeout, six times that. Measured through a real socket
// with an nft that takes eight seconds:
//
//	POST /import      -> HTTP 303 after 5.007s
//	web log           -> import rules error: read response: i/o timeout
//	the operator sees -> the import failed
//	the audit log     -> rules_imported
//	staged custom     -> [] before, ["tcp dport 8443 accept"] after
//
// The staged rule set had been replaced and the interface said it had not, so
// the operator's next move — try again, or apply — happens on top of a set that
// is not the one on screen.
func TestAnImportIsNotAbandonedWhileTheCoreIsStillValidatingIt(t *testing.T) {
	// Past the old flat deadline, comfortably inside the new one.
	c := NewCoreClient(slowCore(t, 6*time.Second))

	if err := c.ImportRules([]byte(`{"custom":["tcp dport 8443 accept"]}`)); err != nil {
		t.Errorf("the client gave up on an import the core was still working on: %v", err)
	}
	if _, err := c.ValidateCustom([]string{"tcp dport 8443 accept"}); err != nil {
		t.Errorf("the client gave up on a syntax check the core was still running: %v", err)
	}
}

// The other half: a command that only touches files keeps the short deadline,
// so a status poll cannot hang the dashboard for half a minute.
func TestAStatusPollStillGivesUpQuickly(t *testing.T) {
	c := NewCoreClient(slowCore(t, shared.NftTimeout))

	start := time.Now()
	_, err := c.GetStatus()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("GET_STATUS waited out an nft-length delay; the dashboard polls this every 2s")
	}
	if elapsed > shared.NftTimeout {
		t.Errorf("GET_STATUS waited %s, longer than the nft bound it has nothing to do with", elapsed)
	}
}

// And the coupling itself: whatever the two deadlines become, the client's has
// to outlast what the core is allowed to spend, or the same bug returns.
func TestTheClientOutwaitsWhatTheCoreMaySpendOnNft(t *testing.T) {
	for _, cmd := range []shared.CommandType{shared.CmdImportRules, shared.CmdValidateCustom} {
		if got := shared.CommandTimeout(cmd); got <= shared.NftTimeout {
			t.Errorf("%s: the client waits %s but the core may spend %s in nft — "+
				"it would report a failure for work that then completes",
				cmd, got, shared.NftTimeout)
		}
	}
	if shared.CommandTimeout(shared.CmdGetStatus) >= shared.NftTimeout {
		t.Error("GET_STATUS runs no subprocess; it should not carry the nft deadline")
	}
}

// The HTTP server's own WriteTimeout is a third deadline in the same chain,
// and nothing before this test tied it to the other two: POST /import and
// POST /validate call the core synchronously, so a WriteTimeout shorter than
// shared.CommandTimeout(CmdImportRules) cuts the connection before a reply
// the core is still writing can reach the client — the same failure mode
// TestAnImportIsNotAbandonedWhileTheCoreIsStillValidatingIt covers one layer
// down, reappearing at the HTTP layer where that test cannot see it.
func TestTheHTTPWriteDeadlineOutwaitsTheLongestCommand(t *testing.T) {
	longest := shared.CommandTimeout(shared.CmdImportRules)
	if v := shared.CommandTimeout(shared.CmdValidateCustom); v > longest {
		longest = v
	}
	if got := writeTimeout(); got <= longest {
		t.Errorf("http.Server.WriteTimeout = %s, but the client may wait %s for a reply "+
			"the handler is blocked on — the server would cut the connection first",
			got, longest)
	}
}
