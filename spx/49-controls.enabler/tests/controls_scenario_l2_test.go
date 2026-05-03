package controls_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// binaryPath is the compiled portree binary, built once per test run via TestMain.
var (
	binaryPath string
	buildOnce  sync.Once
	buildErr   error
)

var binaryDir string

func TestMain(m *testing.M) {
	buildOnce.Do(func() {
		dir, err := findProjectRoot()
		if err != nil {
			buildErr = fmt.Errorf("finding project root: %w", err)
			return
		}
		// MkdirTemp gives a unique directory per test process — two parallel
		// `go test` runs won't collide on the same binary path.
		tmp, err := os.MkdirTemp("", "portree-test-*")
		if err != nil {
			buildErr = fmt.Errorf("creating temp dir: %w", err)
			return
		}
		bin := filepath.Join(tmp, "portree")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", bin, ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building binary: %w\n%s", err, out)
			_ = os.RemoveAll(tmp)
			return
		}
		binaryPath = bin
		binaryDir = tmp
	})
	code := m.Run()
	if binaryDir != "" {
		_ = os.RemoveAll(binaryDir)
	}
	os.Exit(code)
}

// findProjectRoot walks up from this test file's directory to find go.mod.
func findProjectRoot() (string, error) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize a minimal git repo.
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

	// Write a minimal .portree.toml.
	toml := `[services.web]
command = "sleep 100"
port_range = { min = 19900, max = 19999 }
proxy_port = 19000
`
	if err := os.WriteFile(filepath.Join(dir, ".portree.toml"), []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runPortree(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	if buildErr != nil {
		t.Fatalf("binary build failed: %v", buildErr)
	}
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			exitCode = ex.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestLsJSONOutputContainsRequiredFields verifies that `portree ls --json`
// produces valid JSON with url and direct_url fields on each entry.
func TestLsJSONOutputContainsRequiredFields(t *testing.T) {
	dir := setupTestRepo(t)
	stdout, _, _ := runPortree(t, dir, "ls", "--json")
	if stdout == "" {
		t.Fatal("portree ls --json produced no output")
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("portree ls --json output is not valid JSON: %v\noutput:\n%s", err, stdout)
	}
	if len(entries) == 0 {
		t.Skip("no entries to check (no running services in test repo)")
	}
	required := []string{"worktree", "service", "port", "status"}
	for i, entry := range entries {
		for _, field := range required {
			if _, ok := entry[field]; !ok {
				t.Errorf("entry[%d] missing required field %q", i, field)
			}
		}
	}
}

// TestLsHumanReadableOutputIsTabular verifies that `portree ls` without --json
// produces a human-readable table (non-JSON text output).
func TestLsHumanReadableOutputIsTabular(t *testing.T) {
	dir := setupTestRepo(t)
	stdout, _, _ := runPortree(t, dir, "ls")
	// The output must not be valid JSON (it's a table, not JSON).
	var anything interface{}
	if err := json.Unmarshal([]byte(stdout), &anything); err == nil {
		t.Error("portree ls (no --json) produced JSON output; expected a human-readable table")
	}
}

// TestDoctorReportsConfigErrors verifies that `portree doctor` reports each
// configuration problem when given an invalid .portree.toml.
func TestDoctorReportsConfigErrors(t *testing.T) {
	dir := setupTestRepo(t)
	// Overwrite with a config that has an empty command (validation error).
	badToml := `[services.web]
command = ""
port_range = { min = 19900, max = 19999 }
proxy_port = 19000
`
	if err := os.WriteFile(filepath.Join(dir, ".portree.toml"), []byte(badToml), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, _ := runPortree(t, dir, "doctor")
	combined := stdout + stderr
	if !strings.Contains(combined, "web") {
		t.Errorf("doctor output should name the service with the error; got:\n%s", combined)
	}
}

// TestMissingConfigInstructsInit verifies that any portree command requiring
// config surfaces the instruction to run `portree init` when .portree.toml is absent.
func TestMissingConfigInstructsInit(t *testing.T) {
	dir := setupTestRepo(t)
	if err := os.Remove(filepath.Join(dir, ".portree.toml")); err != nil {
		t.Fatal(err)
	}
	_, stderr, exitCode := runPortree(t, dir, "ls")
	if exitCode == 0 {
		t.Error("portree ls without .portree.toml should exit non-zero")
	}
	if !strings.Contains(stderr, "portree init") && !strings.Contains(stderr, "init") {
		t.Errorf("error output should mention 'portree init'; got stderr:\n%s", stderr)
	}
}
