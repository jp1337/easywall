package web

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A service that is up and cannot serve a single page is worse than one that
// refused to start: systemd reports it active, the port answers, and the failure
// only exists in a WARN line nobody read.
//
// Measured before the fix, with the assets missing from the working directory:
//
//	WARN  templates not loaded  dir=web/templates error="pattern matches no files"
//	INFO  easywall-web listening addr=127.0.0.1:12241
//	GET /login     503  "Web interface not ready (templates missing — run Phase 4)"
//	GET /dashboard 303  → /login → 503
//
// Start() already refuses to bind without a TLS certificate on exactly this
// reasoning. Templates are the same case.
func TestStartRefusesToServeWithoutTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/ssl", 0750); err != nil {
		t.Fatal(err)
	}
	cfgPath := dir + "/web.toml"
	if err := os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:0"
socket_path = "`+dir+`/core.sock"
ssl_dir = "`+dir+`/ssl"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
update_check = false
[tls]
cert = ""
key  = ""
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// The test runs in internal/web, where cfg.TemplatesDir() ("web/templates")
	// does not exist — the same shape as a unit with the wrong WorkingDirectory.
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if srv.tmpl != nil {
		t.Skip("templates were found; this test needs a working directory without them")
	}
	defer srv.Stop()

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Start returned nil; it served a firewall interface that has no pages")
		}
		for _, want := range []string{"templates", "working directory"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q — the operator has to be told what to fix",
					err, want)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start is still serving without templates; every page it answers is a 503")
	}
}
