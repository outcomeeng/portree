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

// spawnPortOrphan starts a python3 process that binds the given TCP port on
// 127.0.0.1, holds it for 120s, and prints "bound" to stdout once the bind
// succeeds. The test waits up to 3s for that confirmation. Cleanup kills the
// orphan's process group via t.Cleanup.
func spawnPortOrphan(t *testing.T, port int) {
	t.Helper()
	pyScript := fmt.Sprintf(
		"import socket, time, sys\n"+
			"s = socket.socket()\n"+
			"s.bind(('127.0.0.1', %d))\n"+
			"s.listen(1)\n"+
			"sys.stdout.write('bound\\n'); sys.stdout.flush()\n"+
			"time.sleep(120)\n",
		port)
	orphan := exec.Command("python3", "-c", pyScript)
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdoutPipe, _ := orphan.StdoutPipe()
	if err := orphan.Start(); err != nil {
		t.Skipf("could not start python3 orphan: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-orphan.Process.Pid, syscall.SIGKILL)
		_ = orphan.Wait()
	})
	buf := make([]byte, 32)
	readDone := make(chan struct{})
	go func() {
		_, _ = stdoutPipe.Read(buf)
		close(readDone)
	}()
	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("orphan never reported bound on port %d", port)
	}
}

// waitForPortFree polls until the given TCP port is available for binding.
func waitForPortFree(t *testing.T, port int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestResetProxyPortKillsOrphanOnProxyPort verifies that `portree reset
// --proxy-port` terminates a non-portree listener on the configured proxy
// port. Mirrors the v0.4.0 dev:kill-replacement use case: a stray Next.js (or
// other dev server) bound to 3000 needs to be cleaned up.
func TestResetProxyPortKillsOrphanOnProxyPort(t *testing.T) {
	mainDir := setupTestRepo(t)
	const proxyPort = 19000 // matches test config's proxy_port

	spawnPortOrphan(t, proxyPort)

	if _, stderr, code := runPortree(t, mainDir, "reset", "--proxy-port"); code != 0 {
		t.Fatalf("reset --proxy-port: %d, %s", code, stderr)
	}

	if !waitForPortFree(t, proxyPort, 2*time.Second) {
		t.Errorf("port %d still held after `reset --proxy-port`", proxyPort)
	}
}

// TestResetProxyPortLeavesLegitimateProxyAlone verifies that --proxy-port
// skips the PID recorded as the running portree proxy in state.
func TestResetProxyPortLeavesLegitimateProxyAlone(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("up --ensure-proxy: %d, %s", code, stderr)
	}
	t.Cleanup(func() {
		runPortree(t, mainDir, "proxy", "stop")
		runPortree(t, mainDir, "down", "--all")
	})

	proxyPID, ok := waitForProxyStatus(t, mainDir, "running", 3*time.Second)
	if !ok {
		t.Fatal("legitimate proxy did not start")
	}

	if _, stderr, code := runPortree(t, mainDir, "reset", "--proxy-port"); code != 0 {
		t.Fatalf("reset --proxy-port: %d, %s", code, stderr)
	}
	time.Sleep(300 * time.Millisecond)

	if err := syscall.Kill(proxyPID, 0); err != nil {
		t.Errorf("legitimate proxy PID %d killed by `reset --proxy-port`: %v", proxyPID, err)
	}
	if status, _ := readProxyState(t, mainDir); status != "running" {
		t.Errorf("proxy state changed to %q after `reset --proxy-port`; must remain running", status)
	}
}

// TestResetProxyPortComposesWithAll verifies that `reset --all --proxy-port`
// cleans both per-worktree service ports AND configured proxy ports in one
// invocation.
func TestResetProxyPortComposesWithAll(t *testing.T) {
	mainDir := setupTestRepo(t)
	const proxyPort = 19000

	// Determine an allocated service port via an up/down round-trip.
	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("up: %d, %s", code, stderr)
	}
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	_ = json.Unmarshal([]byte(stdout), &entries)
	servicePort := 0
	for _, e := range entries {
		if pf, ok := e["port"].(float64); ok && pf > 0 {
			servicePort = int(pf)
			break
		}
	}
	if servicePort == 0 {
		t.Fatalf("could not determine service port; ls:\n%s", stdout)
	}
	if _, stderr, code := runPortree(t, mainDir, "down"); code != 0 {
		t.Fatalf("down: %d, %s", code, stderr)
	}

	// Two orphans: one on a service port, one on the proxy port.
	spawnPortOrphan(t, servicePort)
	spawnPortOrphan(t, proxyPort)

	if _, stderr, code := runPortree(t, mainDir, "reset", "--all", "--proxy-port"); code != 0 {
		t.Fatalf("reset --all --proxy-port: %d, %s", code, stderr)
	}

	for _, p := range []int{servicePort, proxyPort} {
		if !waitForPortFree(t, p, 2*time.Second) {
			t.Errorf("port %d still held after `reset --all --proxy-port`", p)
		}
	}
}

// TestResetKillsOrphanProcessesOnCurrentWorktreePorts verifies that the
// `portree reset` command terminates any process listening on the ports
// allocated to the current worktree's services. The deterministic FNV-hash
// allocator means each branch:service has a stable port, so an orphan from
// a previous run can be hunted down by port without knowing its PID.
func TestResetKillsOrphanProcessesOnCurrentWorktreePorts(t *testing.T) {
	mainDir := setupTestRepo(t)

	// First, run portree up to determine which port the allocator hands out
	// for main:web. Then stop it and grab that port for the orphan test.
	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("up: %d, %s", code, stderr)
	}
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	_ = json.Unmarshal([]byte(stdout), &entries)
	allocatedPort := 0
	for _, e := range entries {
		if pf, ok := e["port"].(float64); ok && pf > 0 {
			allocatedPort = int(pf)
			break
		}
	}
	if allocatedPort == 0 {
		t.Fatalf("could not determine allocated port; ls:\n%s", stdout)
	}
	if _, stderr, code := runPortree(t, mainDir, "down"); code != 0 {
		t.Fatalf("down: %d, %s", code, stderr)
	}

	// Spawn an orphan process bound to that port — simulates the leftover
	// `next dev` (or any dev server) that survived an earlier crash. python3
	// is portable across macOS and CI Linux; the script exits if anything
	// signals it.
	pyScript := fmt.Sprintf(
		"import socket, time, sys\n"+
			"s = socket.socket()\n"+
			"s.bind(('127.0.0.1', %d))\n"+
			"s.listen(1)\n"+
			"sys.stdout.write('bound\\n'); sys.stdout.flush()\n"+
			"time.sleep(120)\n",
		allocatedPort)
	orphan := exec.Command("python3", "-c", pyScript)
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdoutPipe, _ := orphan.StdoutPipe()
	if err := orphan.Start(); err != nil {
		t.Skipf("could not start python3 orphan: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-orphan.Process.Pid, syscall.SIGKILL)
		_ = orphan.Wait()
	})
	// Wait for the script to print "bound" before continuing.
	buf := make([]byte, 32)
	_ = orphan.Process // keep linter quiet
	readDone := make(chan struct{})
	go func() {
		_, _ = stdoutPipe.Read(buf)
		close(readDone)
	}()
	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("orphan never reported bound on port %d", allocatedPort)
	}

	// portree reset must hunt down and kill the orphan on this worktree's port.
	if _, stderr, code := runPortree(t, mainDir, "reset"); code != 0 {
		t.Fatalf("reset: %d, %s", code, stderr)
	}

	// The port must now be available for binding.
	deadline := time.Now().Add(2 * time.Second)
	freed := false
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", "127.0.0.1:"+intToStr(allocatedPort))
		if err == nil {
			_ = ln.Close()
			freed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !freed {
		t.Errorf("port %d still held after `portree reset`", allocatedPort)
	}
}

func intToStr(i int) string {
	return fmt.Sprintf("%d", i)
}

// TestPortreeIgnoresConfigInLinkedWorktree verifies that a `.portree.toml`
// committed (or otherwise present) in a linked worktree's checkout is ignored
// — only the main worktree's config is authoritative. Per ADR-15, config and
// state both resolve from MainWorktreeRoot, so per-worktree `.portree.toml`
// files would silently fragment the model if honored.
func TestPortreeIgnoresConfigInLinkedWorktree(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	// Plant a different .portree.toml in the linked worktree's checkout.
	// Different service name than main's — if it's loaded, ls will show it.
	rogueConfig := `[services.rogue]
command = "sleep 100"
port_range = { min = 18000, max = 18099 }
proxy_port = 18000
`
	rogueConfigPath := filepath.Join(linkedDir, ".portree.toml")
	if err := os.WriteFile(rogueConfigPath, []byte(rogueConfig), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runPortree(t, linkedDir, "ls", "--json")
	if code != 0 {
		t.Fatalf("ls from linked exited %d; stderr:\n%s", code, stderr)
	}

	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}

	for _, e := range entries {
		if s, _ := e["service"].(string); s == "rogue" {
			t.Errorf("ls showed service %q from the linked worktree's .portree.toml; main's config must win", s)
		}
	}
	foundMainService := false
	for _, e := range entries {
		if s, _ := e["service"].(string); s == "web" {
			foundMainService = true
			break
		}
	}
	if !foundMainService {
		t.Errorf("ls did not show 'web' from main's config; entries:\n%s", stdout)
	}
}

// readProxyState returns (status, pid) from the state.json file at mainDir.
func readProxyState(t *testing.T, mainDir string) (string, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(mainDir, ".portree", "state.json"))
	if err != nil {
		return "", 0
	}
	var st map[string]interface{}
	if json.Unmarshal(raw, &st) != nil {
		return "", 0
	}
	proxy, _ := st["proxy"].(map[string]interface{})
	status, _ := proxy["status"].(string)
	pidF, _ := proxy["pid"].(float64)
	return status, int(pidF)
}

// waitForProxyStatus polls state.json until proxy.status matches `want` or the
// timeout elapses. Returns the matching PID and whether the wait succeeded.
func waitForProxyStatus(t *testing.T, mainDir, want string, timeout time.Duration) (int, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, pid := readProxyState(t, mainDir)
		if status == want {
			return pid, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, false
}

// TestUpEnsureProxyStartsProxyInBackground verifies that `portree up --ensure-proxy`
// daemonizes the proxy: the proxy keeps running after the up command returns,
// state records it, and the proxy port accepts TCP connections.
func TestUpEnsureProxyStartsProxyInBackground(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("up --ensure-proxy: %d, %s", code, stderr)
	}
	t.Cleanup(func() {
		runPortree(t, mainDir, "proxy", "stop")
		runPortree(t, mainDir, "down", "--all")
	})

	pid, ok := waitForProxyStatus(t, mainDir, "running", 3*time.Second)
	if !ok {
		t.Fatal("proxy never registered as running in state")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Errorf("proxy PID %d not alive after up --ensure-proxy: %v", pid, err)
	}

	// Proxy must accept connections on its configured port (19000 in test config).
	deadline := time.Now().Add(2 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:19000", 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			connected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !connected {
		t.Errorf("proxy not accepting connections on port 19000")
	}
}

// TestUpEnsureProxyIsIdempotent verifies that a second `up --ensure-proxy`
// invocation does NOT restart an already-running proxy.
func TestUpEnsureProxyIsIdempotent(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("first up --ensure-proxy: %d, %s", code, stderr)
	}
	t.Cleanup(func() {
		runPortree(t, mainDir, "proxy", "stop")
		runPortree(t, mainDir, "down", "--all")
	})

	firstPID, ok := waitForProxyStatus(t, mainDir, "running", 3*time.Second)
	if !ok {
		t.Fatal("first up did not start proxy")
	}

	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("second up --ensure-proxy: %d, %s", code, stderr)
	}
	_, secondPID := readProxyState(t, mainDir)
	if secondPID != firstPID {
		t.Errorf("second `up --ensure-proxy` restarted proxy: PID %d → %d", firstPID, secondPID)
	}
}

// TestDownReleaseProxyStopsProxyWhenLastOut verifies that `portree down --release-proxy`
// stops the proxy when no other worktree has running services.
func TestDownReleaseProxyStopsProxyWhenLastOut(t *testing.T) {
	mainDir := setupTestRepo(t)

	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("up --ensure-proxy: %d, %s", code, stderr)
	}
	t.Cleanup(func() {
		runPortree(t, mainDir, "proxy", "stop")
		runPortree(t, mainDir, "down", "--all")
	})

	if _, ok := waitForProxyStatus(t, mainDir, "running", 3*time.Second); !ok {
		t.Fatal("proxy did not start")
	}

	if _, stderr, code := runPortree(t, mainDir, "down", "--release-proxy"); code != 0 {
		t.Fatalf("down --release-proxy: %d, %s", code, stderr)
	}

	if _, ok := waitForProxyStatus(t, mainDir, "stopped", 3*time.Second); !ok {
		status, pid := readProxyState(t, mainDir)
		t.Errorf("proxy still running after `down --release-proxy` (last worktree); status=%q pid=%d", status, pid)
	}

	// Port 19000 must be free.
	ln, err := net.Listen("tcp", "127.0.0.1:19000")
	if err != nil {
		t.Errorf("port 19000 still bound after release-proxy: %v", err)
	} else {
		_ = ln.Close()
	}
}

// TestDownReleaseProxyKeepsProxyWhenOtherWorktreesRunning verifies that
// `portree down --release-proxy` leaves the proxy alone when at least one
// other worktree still has running services.
func TestDownReleaseProxyKeepsProxyWhenOtherWorktreesRunning(t *testing.T) {
	mainDir := setupTestRepo(t)
	commitConfig(t, mainDir)

	linkedDir := filepath.Join(t.TempDir(), "feature-worktree")
	addWorktree(t, mainDir, linkedDir, "feature")

	// Main starts services AND proxy.
	if _, stderr, code := runPortree(t, mainDir, "up", "--ensure-proxy"); code != 0 {
		t.Fatalf("up --ensure-proxy from main: %d, %s", code, stderr)
	}
	// Linked starts its services.
	if _, stderr, code := runPortree(t, linkedDir, "up"); code != 0 {
		t.Fatalf("up from linked: %d, %s", code, stderr)
	}
	t.Cleanup(func() {
		runPortree(t, mainDir, "proxy", "stop")
		runPortree(t, mainDir, "down", "--all")
	})

	proxyPID, ok := waitForProxyStatus(t, mainDir, "running", 3*time.Second)
	if !ok {
		t.Fatal("proxy did not start")
	}

	// Main releases — proxy must remain because feature worktree still has services.
	if _, stderr, code := runPortree(t, mainDir, "down", "--release-proxy"); code != 0 {
		t.Fatalf("down --release-proxy: %d, %s", code, stderr)
	}

	// Brief settling — give the release path time to (incorrectly) stop the proxy if buggy.
	time.Sleep(300 * time.Millisecond)

	if err := syscall.Kill(proxyPID, 0); err != nil {
		t.Errorf("proxy PID %d killed even though feature worktree still running: %v", proxyPID, err)
	}
	if status, _ := readProxyState(t, mainDir); status != "running" {
		t.Errorf("proxy state changed to %q; should remain running while feature worktree active", status)
	}
}

// TestLsShowsProxyURLAndReachability verifies that `portree ls` surfaces the
// proxy URL (http://{slug}.localhost:{proxy_port}) when the proxy is running
// and probes its reachability. Without the URL in the output, ls is unhelpful
// for users who route traffic through the shared proxy.
func TestLsShowsProxyURLAndReachability(t *testing.T) {
	mainDir := setupTestRepo(t)

	// Start a service so a port is allocated and recorded.
	if _, stderr, code := runPortree(t, mainDir, "up"); code != 0 {
		t.Fatalf("up exited %d; stderr:\n%s", code, stderr)
	}
	t.Cleanup(func() { runPortree(t, mainDir, "down") })

	// Stand up an HTTP server on the configured proxy_port (19000) responding
	// 200 to HEAD. We then mark the proxy as running in state.json so ls's
	// URL-rendering path is triggered, and the probe sees a real listener.
	listener, err := net.Listen("tcp", "127.0.0.1:19000")
	if err != nil {
		t.Skipf("could not bind 127.0.0.1:19000 (left over from another test?): %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })

	statePath := filepath.Join(mainDir, ".portree", "state.json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st map[string]interface{}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	st["proxy"] = map[string]interface{}{
		"pid":    os.Getpid(),
		"status": "running",
	}
	out, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(statePath, out, 0600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	// JSON output must include url and reachable=true.
	stdout, _, _ := runPortree(t, mainDir, "ls", "--json")
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("ls --json: %v\n%s", err, stdout)
	}
	foundReachable := false
	for _, e := range entries {
		url, _ := e["url"].(string)
		reachable, _ := e["reachable"].(bool)
		if strings.Contains(url, ".localhost:19000") && reachable {
			foundReachable = true
		}
	}
	if !foundReachable {
		t.Errorf("ls --json should surface a reachable URL on .localhost:19000; got:\n%s", stdout)
	}

	// Human-readable table must include the URL.
	tableStdout, _, _ := runPortree(t, mainDir, "ls")
	if !strings.Contains(tableStdout, ".localhost:19000") {
		t.Errorf("ls table should include URL on .localhost:19000; got:\n%s", tableStdout)
	}
	if !strings.Contains(tableStdout, "URL") {
		t.Errorf("ls table should include a URL column header when proxy is running; got:\n%s", tableStdout)
	}
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
