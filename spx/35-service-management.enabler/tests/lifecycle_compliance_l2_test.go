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

// TestSIGTERMBeforeSIGKILL verifies that StopServices eventually kills a process
// that ignores SIGTERM, proving that SIGKILL is sent after the graceful-shutdown
// timeout. The test is L2 because the runner's SIGTERM-to-SIGKILL timeout is
// hardcoded at 10 seconds, making this test inherently slow.
func TestSIGTERMBeforeSIGKILL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SIGTERM→SIGKILL test in short mode (requires ~10s)")
	}

	dir := t.TempDir()
	worktreeDir := filepath.Join(dir, "wt")
	if err := os.MkdirAll(worktreeDir, 0700); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	store, err := state.NewFileStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// Command ignores SIGTERM; only SIGKILL will stop it.
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "sh -c 'trap \"\" TERM; sleep 120'",
				PortRange: config.PortRange{Min: 19700, Max: 19799},
				ProxyPort: 19200,
			},
		},
		Env:       map[string]string{},
		Worktrees: map[string]config.WTOverride{},
	}
	reg := port.NewRegistry(store, cfg)
	mgr := process.NewManager(cfg, store, reg)

	tree := &git.Worktree{Path: worktreeDir, Branch: "main"}
	results := mgr.StartServices(tree, "")
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("StartServices() error: %v", r.Err)
		}
	}
	// Give the process a moment to fully start.
	time.Sleep(100 * time.Millisecond)

	// StopServices must complete within 15s (runner timeout is 10s).
	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.StopServices(tree, "")
	}()

	select {
	case <-done:
		// StopServices returned; process was killed.
	case <-time.After(15 * time.Second):
		t.Error("StopServices() did not complete within 15s for a SIGTERM-ignoring process; SIGKILL may not have been sent")
	}
}
