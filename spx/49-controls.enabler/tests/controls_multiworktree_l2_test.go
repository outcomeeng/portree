package controls_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

// TestAllCommandsFindMainWorktreeFromLinkedWorktree exercises every portree
// subcommand that touches state or config from a linked worktree that has no
// local .portree.toml. Each must succeed (or fail with a non-config-related
// error) — none may fail with "config not found in <linked-worktree>".
func TestAllCommandsFindMainWorktreeFromLinkedWorktree(t *testing.T) {
	mainDir := setupTestRepo(t)
	// Note: do NOT commit .portree.toml — main has it, linked branches do not.

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	cases := []struct {
		name string
		args []string
	}{
		{"ls", []string{"ls"}},
		{"ls --json", []string{"ls", "--json"}},
		{"doctor", []string{"doctor"}},
		{"down", []string{"down"}},
		{"down --all", []string{"down", "--all"}},
		{"down --prune", []string{"down", "--prune"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runPortree(t, linkedDir, tc.args...)
			combined := stdout + stderr
			if strings.Contains(combined, "not found in "+linkedDir) {
				t.Errorf("portree %s looked for config/state in linked worktree %q instead of main worktree %q\nstdout:\n%s\nstderr:\n%s",
					strings.Join(tc.args, " "), linkedDir, mainDir, stdout, stderr)
			}
			if code != 0 {
				t.Errorf("portree %s from linked worktree exited %d (config/state must resolve via main worktree)\nstdout:\n%s\nstderr:\n%s",
					strings.Join(tc.args, " "), code, stdout, stderr)
			}
		})
	}
}

// TestInitFromLinkedWorktreeCreatesConfigInMainWorktree verifies that
// `portree init` invoked from a linked worktree creates .portree.toml in the
// main worktree. Anything else makes the file invisible to all other commands.
func TestInitFromLinkedWorktreeCreatesConfigInMainWorktree(t *testing.T) {
	mainDir := initBareRepo(t) // empty repo, no .portree.toml anywhere

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	if _, stderr, code := runPortree(t, linkedDir, "init"); code != 0 {
		t.Fatalf("portree init from linked worktree exited %d; stderr:\n%s", code, stderr)
	}

	mainConfig := filepath.Join(mainDir, ".portree.toml")
	linkedConfig := filepath.Join(linkedDir, ".portree.toml")
	if _, err := os.Stat(mainConfig); os.IsNotExist(err) {
		t.Errorf(".portree.toml not created in main worktree %q", mainConfig)
	}
	if _, err := os.Stat(linkedConfig); err == nil {
		t.Errorf(".portree.toml was created in the linked worktree %q — must live at main worktree root", linkedConfig)
	}
}

// initBareRepo creates a temp git repo with one empty commit, no .portree.toml.
func initBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "commit", "--allow-empty", "-m", "init"},
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
	return dir
}

// TestUpWithoutAllAffectsOnlyCallingWorktree verifies the canonical multi-
// developer workflow: each developer runs `portree up` from their own worktree
// and only that worktree's services start. Another developer running `portree
// up` from a different worktree must not start, stop, or otherwise touch the
// first worktree's services.
//
// This is the default portree workflow. `--all` is the exception, not the rule.
func TestUpWithoutAllAffectsOnlyCallingWorktree(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	// Developer A: `portree up` from mainDir. Only main's service should start.
	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("up from main exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	running := map[string]int{}
	for _, e := range entries {
		if s, _ := e["status"].(string); s != "running" {
			continue
		}
		wt, _ := e["worktree"].(string)
		if pidF, ok := e["pid"].(float64); ok && pidF > 0 {
			running[wt] = int(pidF)
		}
	}
	if len(running) != 1 {
		t.Fatalf("up from main should start exactly 1 worktree's services; got %d running:\n%s", len(running), stdout)
	}
	var mainPID int
	for _, pid := range running {
		mainPID = pid
	}

	// Developer B: `portree up` from linkedDir. Only feature's service should
	// start. Main's PID must remain untouched.
	if _, stderr, code := runPortree(t, linkedDir, "up"); code != 0 {
		t.Fatalf("up from linked exited %d; stderr:\n%s", code, stderr)
	}
	time.Sleep(200 * time.Millisecond)

	if err := syscall.Kill(mainPID, 0); err != nil {
		t.Errorf("main worktree's PID %d was killed by `portree up` from a different worktree: %v", mainPID, err)
	}

	stdout, _, _ = runPortree(t, mainDir, "ls", "--json")
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	runningCount := 0
	for _, e := range entries {
		if s, _ := e["status"].(string); s == "running" {
			runningCount++
		}
	}
	if runningCount != 2 {
		t.Errorf("after each developer ran their own `up`, expected 2 running services; got %d:\n%s", runningCount, stdout)
	}
}

// TestSecondUpFromAnotherWorktreeIsIdempotent verifies the canonical multi-
// developer scenario: two `portree up --all` invocations from different
// worktrees against the same shared state must NOT kill or overwrite the
// services started by the first invocation. The second invocation is a no-op
// for already-running services.
//
// This is THE basic scenario portree was built for. The original PIDs must
// survive, and `portree ls` must continue to show them.
func TestSecondUpFromAnotherWorktreeIsIdempotent(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	// First up: from mainDir, start services for both worktrees.
	if _, stderr, code := runPortree(t, mainDir, "up", "--all"); code != 0 {
		t.Fatalf("first portree up --all exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	// Capture original PIDs.
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	originalPIDs := map[string]int{}
	for _, e := range entries {
		if status, _ := e["status"].(string); status != "running" {
			continue
		}
		wt, _ := e["worktree"].(string)
		if pidF, ok := e["pid"].(float64); ok && pidF > 0 {
			originalPIDs[wt] = int(pidF)
		}
	}
	if len(originalPIDs) < 2 {
		t.Fatalf("expected 2 running services after first up; got %d (entries: %s)", len(originalPIDs), stdout)
	}

	// Second up from a DIFFERENT worktree. Must be idempotent — original PIDs
	// must remain untouched.
	if _, stderr, code := runPortree(t, linkedDir, "up", "--all"); code != 0 {
		t.Fatalf("second portree up --all exited %d; stderr:\n%s", code, stderr)
	}

	// Brief settling window — the bug manifests immediately when the new
	// wrapper exits, not seconds later.
	time.Sleep(500 * time.Millisecond)

	// Original PIDs must still be signalable (alive).
	for wt, pid := range originalPIDs {
		if err := syscall.Kill(pid, 0); err != nil {
			t.Errorf("original %q PID %d was killed by the second `up --all`: %v", wt, pid, err)
		}
	}

	// State must still reflect the original PIDs (not be overwritten with
	// new wrapper PIDs that have since died).
	stdout2, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries2 []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout2), &entries2); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout2)
	}
	runningInState := 0
	for _, e := range entries2 {
		status, _ := e["status"].(string)
		if status != "running" {
			continue
		}
		runningInState++
		wt, _ := e["worktree"].(string)
		pidF, _ := e["pid"].(float64)
		if originalPIDs[wt] != 0 && int(pidF) != originalPIDs[wt] {
			t.Errorf("state for %q now shows PID %d; original (still alive) was %d — second `up` overwrote state",
				wt, int(pidF), originalPIDs[wt])
		}
	}
	if runningInState < len(originalPIDs) {
		t.Errorf("after second up, expected %d running services in state; got %d\nls output:\n%s",
			len(originalPIDs), runningInState, stdout2)
	}
}

// TestDownPruneReapsStaleEntries verifies that `portree down --prune` clears
// state entries whose recorded PID is no longer alive. It must do so without
// touching any actually-running services (no SIGTERM to live processes).
func TestDownPruneReapsStaleEntries(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	liveDir := filepath.Join(t.TempDir(), "live-worktree")
	addWorktree(t, mainDir, liveDir, "live")

	if _, stderr, code := runPortree(t, mainDir, "up", "--all"); code != 0 {
		t.Fatalf("portree up --all exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	// Capture both PIDs and SIGKILL only the main branch's, simulating a
	// crashed service whose state entry was left as "running".
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	var stalePID, livePID int
	for _, e := range entries {
		wt, _ := e["worktree"].(string)
		pidF, _ := e["pid"].(float64)
		switch {
		case wt != "live" && stalePID == 0 && pidF > 0:
			stalePID = int(pidF)
		case wt == "live" && livePID == 0 && pidF > 0:
			livePID = int(pidF)
		}
	}
	if stalePID == 0 || livePID == 0 {
		t.Fatalf("expected one PID each for stale-target and live worktree; entries:\n%s", stdout)
	}
	if err := syscall.Kill(-stalePID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill -%d: %v", stalePID, err)
	}
	// Wait for the killed PID to be reaped.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(stalePID, 0); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, stderr, code := runPortree(t, mainDir, "down", "--prune"); code != 0 {
		t.Fatalf("portree down --prune exited %d; stderr:\n%s", code, stderr)
	}

	// The live service must still be running (--prune must not kill anything).
	if err := syscall.Kill(livePID, 0); err != nil {
		t.Errorf("live service PID %d was killed by `down --prune`: %v", livePID, err)
	}

	// Doctor's stale check must now pass.
	stdout, stderr, _ := runPortree(t, mainDir, "doctor")
	if strings.Contains(stdout+stderr, "stale") {
		t.Errorf("doctor still reports stale entries after `down --prune`:\n%s\n%s", stdout, stderr)
	}
}

// TestDoctorStaleHintsAtPrune verifies that doctor's stale-state row names
// the command that clears the entries.
func TestDoctorStaleHintsAtPrune(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("portree up exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down") })

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v", err)
	}
	var pid int
	for _, e := range entries {
		if pf, ok := e["pid"].(float64); ok && pf > 0 {
			pid = int(pf)
			break
		}
	}
	if pid == 0 {
		t.Fatal("no running PID after up")
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill -%d: %v", pid, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	stdout, stderr, _ := runPortree(t, mainDir, "doctor")
	combined := stdout + stderr
	if !strings.Contains(combined, "stale") {
		t.Fatalf("doctor should report stale entries; got:\n%s", combined)
	}
	if !strings.Contains(combined, "down --prune") {
		t.Errorf("doctor's stale-state row should name `portree down --prune`; got:\n%s", combined)
	}
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
