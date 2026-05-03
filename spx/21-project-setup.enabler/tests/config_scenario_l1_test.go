package projectsetup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
)

func TestLoadWithValidConfig(t *testing.T) {
	dir := t.TempDir()
	toml := `
[services.web]
command = "npm start"
port_range = { min = 3100, max = 3199 }
proxy_port = 3000

[env]
NODE_ENV = "development"

[worktrees.main]
[worktrees.main.services.web]
`
	if err := os.WriteFile(filepath.Join(dir, ".portree.toml"), []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if _, ok := cfg.Services["web"]; !ok {
		t.Error("expected service 'web' to be loaded")
	}
	if cfg.Env["NODE_ENV"] != "development" {
		t.Errorf("env NODE_ENV = %q, want 'development'", cfg.Env["NODE_ENV"])
	}
	if _, ok := cfg.Worktrees["main"]; !ok {
		t.Error("expected worktree 'main' to be present")
	}
}

func TestLoadMissingConfigReturnsInstructiveError(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for missing .portree.toml, got nil")
	}
	if !strings.Contains(err.Error(), ".portree.toml") {
		t.Errorf("error should name the missing file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "portree init") {
		t.Errorf("error should mention 'portree init', got: %v", err)
	}
}
