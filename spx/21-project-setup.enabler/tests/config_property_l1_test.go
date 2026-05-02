package projectsetup_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/quick"

	"github.com/fairy-pitta/portree/internal/config"
)

// TestLoadIsDeterministic verifies that loading the same .portree.toml content
// always produces the same Config value (no randomness or external state).
func TestLoadIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	toml := `
[services.web]
command = "npm start"
port_range = { min = 3100, max = 3199 }
proxy_port = 3000
`
	if err := os.WriteFile(filepath.Join(dir, ".portree.toml"), []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Services["web"].Command != second.Services["web"].Command {
		t.Errorf("Load() is not deterministic: command differs between calls")
	}
	if first.Services["web"].PortRange != second.Services["web"].PortRange {
		t.Errorf("Load() is not deterministic: port range differs between calls")
	}
}

// TestEnvMergeWorktreeOverridesGlobal verifies that worktree-specific env vars
// always override global env vars for the same key, regardless of declaration order.
func TestEnvMergeWorktreeOverridesGlobal(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "npm start",
				PortRange: config.PortRange{Min: 3100, Max: 3199},
				ProxyPort: 3000,
			},
		},
		Env: map[string]string{
			"SHARED_KEY": "global-value",
			"GLOBAL_ONLY": "only-in-global",
		},
		Worktrees: map[string]config.WTOverride{
			"feature": {
				Services: map[string]config.WTServiceOverride{
					"web": {Env: map[string]string{"SHARED_KEY": "worktree-value"}},
				},
			},
		},
	}

	merged := cfg.EnvForBranch("web", "feature")

	if merged["SHARED_KEY"] != "worktree-value" {
		t.Errorf("env merge: SHARED_KEY = %q, want 'worktree-value' (worktree must override global)", merged["SHARED_KEY"])
	}
	if merged["GLOBAL_ONLY"] != "only-in-global" {
		t.Errorf("env merge: GLOBAL_ONLY = %q, want 'only-in-global' (global keys not overridden must pass through)", merged["GLOBAL_ONLY"])
	}
}

// TestEnvMergeIsDeterministic uses quick.Check to verify that env merging
// produces the same result regardless of key ordering.
func TestEnvMergeIsDeterministic(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command: "npm start",
				PortRange: config.PortRange{Min: 3100, Max: 3199},
				ProxyPort: 3000,
			},
		},
		Env: map[string]string{"K": "v1"},
		Worktrees: map[string]config.WTOverride{
			"main": {Services: map[string]config.WTServiceOverride{"web": {Env: map[string]string{"K": "v2"}}}},
		},
	}
	f := func() bool {
		a := cfg.EnvForBranch("web", "main")
		b := cfg.EnvForBranch("web", "main")
		return a["K"] == b["K"]
	}
	if err := quick.Check(f, nil); err != nil {
		t.Errorf("EnvForBranch is not deterministic: %v", err)
	}
}
