package servicemanagement_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
)

// TestEnvVarsInjected verifies that service processes receive $PORT, $PT_BRANCH,
// and $PT_{SERVICE}_URL when started by the process manager.
func TestEnvVarsInjected(t *testing.T) {
	dir := t.TempDir()
	worktreeDir := filepath.Join(dir, "wt")
	if err := os.MkdirAll(worktreeDir, 0700); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(dir, "env.txt")

	// Service command writes the relevant env vars to a file, then exits.
	cmd := fmt.Sprintf(
		`sh -c 'printf "PORT=%%s\nPT_BRANCH=%%s\nPT_WEB_URL=%%s\n" "$PORT" "$PT_BRANCH" "$PT_WEB_URL" > %s'`,
		outFile,
	)

	stateDir := filepath.Join(dir, "state")
	store, err := state.NewFileStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   cmd,
				PortRange: config.PortRange{Min: 19600, Max: 19699},
				ProxyPort: 19100,
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

	// Wait for the short-lived command to write the file.
	deadline := time.Now().Add(3 * time.Second)
	var content []byte
	for time.Now().Before(deadline) {
		content, err = os.ReadFile(outFile)
		if err == nil && len(content) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(content) == 0 {
		t.Fatal("service process did not write env vars to output file within 3s")
	}

	out := string(content)
	if !strings.Contains(out, "PORT=") || strings.Contains(out, "PORT=\n") {
		t.Errorf("$PORT not injected or empty; output:\n%s", out)
	}
	if !strings.Contains(out, "PT_BRANCH=main") {
		t.Errorf("$PT_BRANCH not injected as 'main'; output:\n%s", out)
	}
	if !strings.Contains(out, "PT_WEB_URL=") || strings.Contains(out, "PT_WEB_URL=\n") {
		t.Errorf("$PT_WEB_URL not injected or empty; output:\n%s", out)
	}
}
