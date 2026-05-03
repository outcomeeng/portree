package controls_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// commitConfig commits .portree.toml so it appears in linked worktrees that
// branch from HEAD. setupTestRepo writes the file but does not commit it.
func commitConfig(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "add", ".portree.toml"},
		{"git", "commit", "-m", "add config"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

// addWorktree creates a linked worktree at path on a new branch.
func addWorktree(t *testing.T, mainDir, path, branch string) {
	t.Helper()
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
	cmd.Dir = mainDir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add %s: %v\n%s", path, err, out)
	}
}

// removeWorktree force-removes a linked worktree, leaving its state entry
// in state.json orphaned.
func removeWorktree(t *testing.T, mainDir, path string) {
	t.Helper()
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree remove %s: %v\n%s", path, err, out)
	}
}

// captureBranchPIDs records every running PID for the named branch from
// `portree ls --json` and registers a cleanup that SIGTERMs each process group.
// Used before removing a worktree so its services do not leak when prune
// removes them from state.
func captureBranchPIDs(t *testing.T, mainDir, branch string) {
	t.Helper()
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	_ = json.Unmarshal([]byte(stdout), &entries)
	var pids []int
	for _, e := range entries {
		if wt, _ := e["worktree"].(string); wt != branch {
			continue
		}
		if pidF, ok := e["pid"].(float64); ok && pidF > 0 {
			pids = append(pids, int(pidF))
		}
	}
	t.Cleanup(func() {
		for _, pid := range pids {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
		}
	})
}

// TestStateFileIsInMainWorktreeRoot verifies that portree commands invoked from
// a linked worktree write state to the main worktree's .portree/state.json,
// not the linked worktree's. The shared root must resolve via
// `git rev-parse --git-common-dir` so all worktrees converge on one state file.
func TestStateFileIsInMainWorktreeRoot(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	_, stderr, code := runPortree(t, linkedDir, "up")
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })
	if code != 0 {
		t.Fatalf("portree up from linked worktree exited %d; stderr:\n%s", code, stderr)
	}

	mainState := filepath.Join(mainDir, ".portree", "state.json")
	linkedState := filepath.Join(linkedDir, ".portree", "state.json")

	if _, err := os.Stat(mainState); os.IsNotExist(err) {
		t.Errorf("state.json missing in main worktree root %q — invocation from a linked worktree must write to the shared state", mainState)
	}
	if _, err := os.Stat(linkedState); err == nil {
		t.Errorf("state.json exists in linked worktree %q — state must live at the main worktree root, not the calling worktree", linkedState)
	}
}

// TestDownAllPruneComposesStopAndPrune verifies that `portree down --all --prune`
// stops services for every non-bare worktree AND removes orphaned state entries
// in one invocation. --prune must not short-circuit the stop loop.
func TestDownAllPruneComposesStopAndPrune(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	featureDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, featureDir, "feature")

	if _, stderr, code := runPortree(t, mainDir, "up", "--all"); code != 0 {
		t.Fatalf("portree up --all exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	captureBranchPIDs(t, mainDir, "feature")
	removeWorktree(t, mainDir, featureDir)

	if _, stderr, code := runPortree(t, mainDir, "down", "--all", "--prune"); code != 0 {
		t.Fatalf("portree down --all --prune exited %d; stderr:\n%s", code, stderr)
	}

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("portree ls --json output not parseable: %v\n%s", err, stdout)
	}

	for _, e := range entries {
		if status, _ := e["status"].(string); status == "running" {
			t.Errorf("service %v/%v still running after `down --all --prune` — stop loop did not execute (entry: %+v)",
				e["worktree"], e["service"], e)
		}
		if wt, _ := e["worktree"].(string); strings.Contains(wt, "orphaned") {
			t.Errorf("orphaned state entry %q remains after `down --all --prune` — prune step did not execute (entry: %+v)", wt, e)
		}
	}
}
