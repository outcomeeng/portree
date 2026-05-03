package controls_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpAllStartsServicesForAllWorktrees verifies `portree up --all` starts
// services for every non-bare worktree in a single invocation.
func TestUpAllStartsServicesForAllWorktrees(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	if _, stderr, code := runPortree(t, mainDir, "up", "--all"); code != 0 {
		t.Fatalf("portree up --all exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("portree ls --json output not parseable: %v\n%s", err, stdout)
	}

	runningWorktrees := map[string]bool{}
	for _, e := range entries {
		if status, _ := e["status"].(string); status == "running" {
			if wt, _ := e["worktree"].(string); wt != "" {
				runningWorktrees[wt] = true
			}
		}
	}
	if len(runningWorktrees) < 2 {
		t.Errorf("`up --all` should start services for every non-bare worktree; saw running services in %d worktree(s), want >= 2; ls output:\n%s",
			len(runningWorktrees), stdout)
	}
}

// TestUpServiceFilterStartsOnlyNamedService verifies `portree up --service <name>`
// starts only the named service.
func TestUpServiceFilterStartsOnlyNamedService(t *testing.T) {
	dir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, dir, "up", "--service", "web"); code != 0 {
		t.Fatalf("portree up --service web exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, dir, "down") })

	stdout, _, _ := runPortree(t, dir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("portree ls --json output not parseable: %v\n%s", err, stdout)
	}

	for _, e := range entries {
		status, _ := e["status"].(string)
		service, _ := e["service"].(string)
		if status == "running" && service != "web" {
			t.Errorf("`up --service web` started unexpected service %q (entry: %+v)", service, e)
		}
	}
}

// TestDownPruneRemovesOrphanedBranchEntries verifies that `portree down --prune`
// removes state entries for branches whose worktrees have been removed.
func TestDownPruneRemovesOrphanedBranchEntries(t *testing.T) {
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

	if _, stderr, code := runPortree(t, mainDir, "down", "--prune"); code != 0 {
		t.Fatalf("portree down --prune exited %d; stderr:\n%s", code, stderr)
	}

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("portree ls --json output not parseable: %v\n%s", err, stdout)
	}

	for _, e := range entries {
		if wt, _ := e["worktree"].(string); strings.Contains(wt, "orphaned") {
			t.Errorf("orphaned state entry %q remains after `down --prune`: %+v", wt, e)
		}
	}
}
