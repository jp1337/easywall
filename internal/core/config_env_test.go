package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_EnvOverridesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	if err := os.WriteFile(path, []byte("socket_path = \"/from/file.sock\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EASYWALL_CORE_SOCKET_PATH", "/from/env.sock")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SocketPath != "/from/env.sock" {
		t.Errorf("SocketPath = %q, want the environment's /from/env.sock", cfg.SocketPath)
	}
}
