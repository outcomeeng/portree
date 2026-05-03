package controls_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestStatusReportsServicesAndProxy verifies that `portree status` reports
// the calling worktree's name and slug, plus each configured service's
// runtime and reachability fields. Stops short of asserting actual
// reachability (no upstream is running in this test) — that's covered by
// TestStatusDirectAndProxyReachabilityAreIndependent below.
func TestStatusReportsServicesAndProxy(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("portree up: %d, %s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	stdout, stderr, code := runPortree(t, mainDir, "status")
	if code != 0 {
		t.Fatalf("portree status exited %d; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "Worktree") {
		t.Errorf("status output missing Worktree header; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Services") {
		t.Errorf("status output missing Services section; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Proxy") {
		t.Errorf("status output missing Proxy section; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "web") {
		t.Errorf("status output should name the configured 'web' service; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "direct") {
		t.Errorf("status output should label the direct URL row; got:\n%s", stdout)
	}
}

// TestStatusReportsProxyNotRunning verifies the Proxy block reports
// "not running" when no proxy is up.
func TestStatusReportsProxyNotRunning(t *testing.T) {
	mainDir := setupTestRepo(t)

	stdout, stderr, code := runPortree(t, mainDir, "status")
	if code != 0 {
		t.Fatalf("portree status: %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "Proxy") {
		t.Fatalf("status output missing Proxy section; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "not running") {
		t.Errorf("Proxy block should report 'not running' when no proxy is up; got:\n%s", stdout)
	}
}

// TestStatusJSONShape verifies the JSON output has the documented shape:
// top-level array with worktree, slug, services array, proxy object;
// each service entry contains the fields required by the spec.
func TestStatusJSONShape(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("portree up: %d, %s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	stdout, stderr, code := runPortree(t, mainDir, "status", "--json")
	if code != 0 {
		t.Fatalf("portree status --json: %d, %s", code, stderr)
	}

	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("status --json output is not a JSON array: %v\n%s", err, stdout)
	}
	if len(entries) == 0 {
		t.Fatal("status --json returned an empty array; expected at least the calling worktree")
	}
	ws := entries[0]
	for _, key := range []string{"worktree", "slug", "services", "proxy"} {
		if _, ok := ws[key]; !ok {
			t.Errorf("worktree entry missing required key %q; got: %v", key, ws)
		}
	}

	services, ok := ws["services"].([]interface{})
	if !ok || len(services) == 0 {
		t.Fatalf("worktree.services should be a non-empty array; got: %v", ws["services"])
	}
	svc, ok := services[0].(map[string]interface{})
	if !ok {
		t.Fatalf("service entry should be an object; got: %v", services[0])
	}
	for _, key := range []string{"name", "port", "status", "direct_reachable", "proxy_reachable"} {
		if _, ok := svc[key]; !ok {
			t.Errorf("service entry missing required key %q; got: %v", key, svc)
		}
	}

	proxy, ok := ws["proxy"].(map[string]interface{})
	if !ok {
		t.Fatalf("worktree.proxy should be an object; got: %v", ws["proxy"])
	}
	for _, key := range []string{"running", "ports", "https", "healthy"} {
		if _, ok := proxy[key]; !ok {
			t.Errorf("proxy entry missing required key %q; got: %v", key, proxy)
		}
	}
}

// TestStatusDirectAndProxyReachabilityAreIndependent verifies that the two
// reachability dimensions are reported independently. We bind a real HTTP
// server on the service's allocated port (so direct is reachable) but no
// proxy is running (so proxy is unreachable). Status must report
// direct_reachable=true, proxy_reachable=false.
func TestStatusDirectAndProxyReachabilityAreIndependent(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("portree up: %d, %s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down", "--all") })

	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	servicePort := 0
	for _, e := range entries {
		if pf, ok := e["port"].(float64); ok && pf > 0 {
			servicePort = int(pf)
			break
		}
	}
	if servicePort == 0 {
		t.Fatalf("could not determine service port")
	}

	// The "sleep 100" service from setupTestRepo holds the port at the TCP
	// level (it's a child of the wrapper sh -c, but only the wrapper holds
	// the port — actually neither holds it; sleep doesn't bind). To make
	// direct reachable, we need a real HTTP server. Stop the service first,
	// then bind our own listener.
	if _, stderr, code := runPortree(t, mainDir, "down"); code != 0 {
		t.Fatalf("portree down: %d, %s", code, stderr)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", servicePort))
	if err != nil {
		t.Skipf("cannot bind 127.0.0.1:%d (left over from earlier test?): %v", servicePort, err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
	// Give the server a moment to accept.
	time.Sleep(100 * time.Millisecond)

	// Manually mark the service running in state with a live PID so status
	// includes it. We reuse our own PID — IsProcessRunning accepts it.
	statePath := filepath.Join(mainDir, ".portree", "state.json")
	raw, _ := os.ReadFile(statePath)
	var st map[string]interface{}
	_ = json.Unmarshal(raw, &st)
	services, _ := st["services"].(map[string]interface{})
	if services == nil {
		services = map[string]interface{}{}
	}
	branch := mainBranchName(t, mainDir)
	branchSvcs, _ := services[branch].(map[string]interface{})
	if branchSvcs == nil {
		branchSvcs = map[string]interface{}{}
	}
	branchSvcs["web"] = map[string]interface{}{
		"port":   servicePort,
		"pid":    os.Getpid(),
		"status": "running",
	}
	services[branch] = branchSvcs
	st["services"] = services
	out, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(statePath, out, 0600)

	stdout, stderr, code := runPortree(t, mainDir, "status", "--json")
	if code != 0 {
		t.Fatalf("status --json: %d, %s", code, stderr)
	}
	var statuses []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &statuses); err != nil {
		t.Fatalf("status --json: %v\n%s", err, stdout)
	}
	if len(statuses) == 0 {
		t.Fatal("status returned no worktrees")
	}
	svcs, _ := statuses[0]["services"].([]interface{})
	if len(svcs) == 0 {
		t.Fatalf("status reports no services; raw:\n%s", stdout)
	}
	first, ok := svcs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first service entry is not an object: %v", svcs[0])
	}
	directReachable, _ := first["direct_reachable"].(bool)
	proxyReachable, _ := first["proxy_reachable"].(bool)
	if !directReachable {
		t.Errorf("direct_reachable should be true (real HTTP server bound); got false")
	}
	if proxyReachable {
		t.Errorf("proxy_reachable should be false (no proxy running); got true")
	}

	// Cleanup state we wrote so other tests aren't confused by the bogus
	// PID. Killing our own PID is bad — instead, blank the state.
	_ = os.WriteFile(statePath, raw, 0600)
}

// TestStatusAllCoversEveryWorktree verifies that --all reports every
// non-bare worktree using the per-worktree shape.
func TestStatusAllCoversEveryWorktree(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	stdout, stderr, code := runPortree(t, mainDir, "status", "--all", "--json")
	if code != 0 {
		t.Fatalf("status --all --json: %d, %s", code, stderr)
	}

	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if len(entries) < 2 {
		t.Errorf("status --all should return >= 2 worktrees; got %d:\n%s", len(entries), stdout)
	}
}

// mainBranchName returns the branch name of the main worktree at dir
// (typically "main" or "master").
func mainBranchName(t *testing.T, dir string) string {
	t.Helper()
	cmd := newGitCmd(dir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func newGitCmd(dir string, args ...string) *exec.Cmd {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	return c
}

// suppress unused-import lint when syscall/exec aren't needed for some build
// path (they're used by helpers above).
var _ = syscall.SIGTERM
