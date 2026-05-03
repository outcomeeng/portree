package servicemanagement_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
)

func newTestStack(t *testing.T) (*process.Manager, *state.FileStore, string) {
	t.Helper()
	dir := t.TempDir()
	worktreeDir := filepath.Join(dir, "worktree")
	if err := os.MkdirAll(worktreeDir, 0700); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	store, err := state.NewFileStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "sleep 60",
				PortRange: config.PortRange{Min: 19500, Max: 19599},
				ProxyPort: 19000,
			},
		},
		Env:       map[string]string{},
		Worktrees: map[string]config.WTOverride{},
	}
	reg := port.NewRegistry(store, cfg)
	mgr := process.NewManager(cfg, store, reg)
	return mgr, store, worktreeDir
}

func TestStartServicesLaunchesOnAssignedPort(t *testing.T) {
	mgr, _, worktreeDir := newTestStack(t)
	tree := &git.Worktree{Path: worktreeDir, Branch: "main"}

	results := mgr.StartServices(tree, "")
	if len(results) == 0 {
		t.Fatal("StartServices() returned no results")
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("StartServices() service %q error: %v", r.Service, r.Err)
		}
		if r.Port < 19500 || r.Port > 19599 {
			t.Errorf("service %q: port %d not in range [19500, 19599]", r.Service, r.Port)
		}
	}
	t.Cleanup(func() { mgr.StopServices(tree, "") })
}

func TestWorktreeCommandOverrideIsUsed(t *testing.T) {
	// Verify that CommandForBranch applies per-worktree overrides.
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {Command: "default-command"},
		},
		Worktrees: map[string]config.WTOverride{
			"feature": {
				Services: map[string]config.WTServiceOverride{
					"web": {Command: "override-command"},
				},
			},
		},
	}
	if got := cfg.CommandForBranch("web", "feature"); got != "override-command" {
		t.Errorf("CommandForBranch(web, feature) = %q, want 'override-command'", got)
	}
	if got := cfg.CommandForBranch("web", "main"); got != "default-command" {
		t.Errorf("CommandForBranch(web, main) = %q, want 'default-command' (no override)", got)
	}
}

func TestEnvMergeWorktreeOverridesGlobal(t *testing.T) {
	cfg := &config.Config{
		Services:  map[string]config.ServiceConfig{"web": {Command: "x"}},
		Env:       map[string]string{"KEY": "global"},
		Worktrees: map[string]config.WTOverride{"feat": {Services: map[string]config.WTServiceOverride{"web": {Env: map[string]string{"KEY": "local"}}}}},
	}
	merged := cfg.EnvForBranch("web", "feat")
	if merged["KEY"] != "local" {
		t.Errorf("EnvForBranch: KEY = %q, want 'local' (worktree should override global)", merged["KEY"])
	}
}

func TestStopServicesClears(t *testing.T) {
	mgr, store, worktreeDir := newTestStack(t)
	tree := &git.Worktree{Path: worktreeDir, Branch: "main"}

	results := mgr.StartServices(tree, "")
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("StartServices() error: %v", r.Err)
		}
	}

	// Give the process a moment to start.
	time.Sleep(50 * time.Millisecond)

	stopResults := mgr.StopServices(tree, "")
	for _, r := range stopResults {
		if r.Err != nil {
			t.Errorf("StopServices() service %q error: %v", r.Service, r.Err)
		}
	}

	// State should reflect stopped status.
	var st *state.State
	if err := store.WithLock(func() error {
		var e error
		st, e = store.Load()
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if ss := state.GetServiceState(st, "main", "web"); ss != nil {
		if ss.Status == state.StatusRunning {
			t.Error("state still shows 'running' after StopServices()")
		}
	}
}
