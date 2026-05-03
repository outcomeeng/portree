package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fairy-pitta/portree/internal/logging"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
)

// EnsureRunning starts the proxy as a detached daemon if it isn't already
// running. Idempotent: a second call while the proxy is alive is a no-op.
//
// The daemon is spawned via re-exec of the portree binary with `proxy start`.
// The child detaches into a new session (Setsid) so it survives the calling
// shell. stdout/stderr are redirected to .portree/logs/proxy.log so any
// startup error is recoverable post-mortem.
//
// commonRoot must be the canonical state-and-config root (typically resolved
// via git.MainWorktreeRoot).
func EnsureRunning(commonRoot string, https bool) error {
	stateDir := filepath.Join(commonRoot, ".portree")
	store, err := state.NewFileStore(stateDir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	// Check current proxy state.
	var current state.ProxyState
	if err := store.WithLock(func() error {
		st, err := store.Load()
		if err != nil {
			return err
		}
		current = st.Proxy
		return nil
	}); err != nil {
		return err
	}
	if current.Status == state.StatusRunning && current.PID > 0 && process.IsProcessRunning(current.PID) {
		logging.Info("Proxy already running (PID %d)", current.PID)
		return nil
	}

	// Prepare log file before spawning the daemon.
	logsDir := filepath.Join(commonRoot, ".portree", "logs")
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return fmt.Errorf("creating logs dir: %w", err)
	}
	logPath := filepath.Join(logsDir, "proxy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening proxy log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	args := []string{"proxy", "start"}
	if https {
		args = append(args, "--https")
	}

	daemon := exec.Command(exe, args...)
	daemon.Dir = commonRoot
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	daemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := daemon.Start(); err != nil {
		return fmt.Errorf("spawning proxy daemon: %w", err)
	}

	// Watch for early daemon exit so a crash (bad flag, port already bound,
	// missing dependency) surfaces immediately instead of waiting out the
	// 3-second registration deadline.
	exited := make(chan error, 1)
	go func() { exited <- daemon.Wait() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case waitErr := <-exited:
			return fmt.Errorf("proxy daemon exited before registering: %w; check %s", waitErr, logPath)
		default:
		}
		var ready bool
		var pid int
		if err := store.WithLock(func() error {
			st, err := store.Load()
			if err != nil {
				return err
			}
			if st.Proxy.Status == state.StatusRunning && st.Proxy.PID > 0 && process.IsProcessRunning(st.Proxy.PID) {
				ready = true
				pid = st.Proxy.PID
			}
			return nil
		}); err == nil && ready {
			logging.Info("Proxy started (PID %d) — log: %s", pid, logPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("proxy did not start within timeout; check %s", logPath)
}

// ReleaseIfUnused stops the proxy iff no worktree currently has running
// services. The calling site is responsible for stopping its own services
// before invoking this — only services that remain alive after that count
// toward "still in use".
func ReleaseIfUnused(commonRoot string) error {
	stateDir := filepath.Join(commonRoot, ".portree")
	store, err := state.NewFileStore(stateDir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	var active []string
	if err := store.WithLock(func() error {
		st, err := store.Load()
		if err != nil {
			return err
		}
		for branch, services := range st.Services {
			for svcName, ss := range services {
				if ss.Status == state.StatusRunning && ss.PID > 0 && process.IsProcessRunning(ss.PID) {
					active = append(active, fmt.Sprintf("%s/%s", branch, svcName))
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if len(active) > 0 {
		logging.Info("Proxy still in use by: %s", strings.Join(active, ", "))
		return nil
	}

	return StopDaemon(store)
}

// StopDaemon SIGTERMs the proxy PID recorded in state, with a SIGKILL
// fallback after a grace window, then writes a stopped record.
func StopDaemon(store *state.FileStore) error {
	var proxyPID int
	if err := store.WithLock(func() error {
		st, err := store.Load()
		if err != nil {
			return err
		}
		proxyPID = st.Proxy.PID
		return nil
	}); err != nil {
		return err
	}
	if proxyPID == 0 || !process.IsProcessRunning(proxyPID) {
		logging.Info("Proxy not running.")
		return nil
	}

	_ = syscall.Kill(proxyPID, syscall.SIGTERM)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !process.IsProcessRunning(proxyPID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if process.IsProcessRunning(proxyPID) {
		_ = syscall.Kill(proxyPID, syscall.SIGKILL)
	}

	if err := store.WithLock(func() error {
		st, err := store.Load()
		if err != nil {
			return err
		}
		st.Proxy = state.ProxyState{Status: state.StatusStopped}
		return store.Save(st)
	}); err != nil {
		return err
	}
	logging.Info("Proxy stopped.")
	return nil
}
