package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/logging"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
	"github.com/spf13/cobra"
)

var resetAll bool

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Hunt down and kill any process bound to the current worktree's allocated ports",
	Long: `Reset finds any process listening on a port allocated to the current worktree's
configured services and terminates it. This is the escape hatch when a previous
dev server crashed or escaped its process group, leaving an orphan that holds
the port.

Use --all to reset every worktree's allocated ports in one invocation.

After reset, the affected services are marked stopped in state. The next
'portree up' starts cleanly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}

		stateDir := filepath.Join(commonRoot, ".portree")
		store, err := state.NewFileStore(stateDir)
		if err != nil {
			return fmt.Errorf("creating state store: %w", err)
		}
		registry := port.NewRegistry(store, cfg)

		var trees []git.Worktree
		if resetAll {
			trees, err = git.ListWorktrees(cwd)
			if err != nil {
				return fmt.Errorf("listing worktrees: %w", err)
			}
		} else {
			tree, err := git.CurrentWorktree(cwd)
			if err != nil {
				return fmt.Errorf("detecting worktree: %w", err)
			}
			trees = []git.Worktree{*tree}
		}

		serviceNames := make([]string, 0, len(cfg.Services))
		for name := range cfg.Services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)

		killed := 0
		for _, tree := range trees {
			if tree.IsBare {
				continue
			}
			for _, svcName := range serviceNames {
				p, err := registry.GetPort(tree.Branch, svcName)
				if err != nil || p == 0 {
					// No port allocated yet — nothing to reset.
					continue
				}
				pids := pidsListeningOn(p)
				for _, pid := range pids {
					logging.Info("Killing PID %d holding %s/%s port %d", pid, tree.Branch, svcName, p)
					terminatePID(pid)
					killed++
				}
				// Mark the service stopped in state regardless — even if no PID
				// was holding the port, the recorded state may be inconsistent.
				if err := store.WithLock(func() error {
					st, e := store.Load()
					if e != nil {
						return e
					}
					ss := state.GetServiceState(st, tree.Branch, svcName)
					portVal := p
					if ss != nil {
						portVal = ss.Port
					}
					state.SetServiceState(st, tree.Branch, svcName, state.StoppedServiceState(portVal))
					return store.Save(st)
				}); err != nil {
					logging.Warn("failed to update state for %s/%s: %v", tree.Branch, svcName, err)
				}
			}
		}

		if killed == 0 {
			logging.Info("No processes were holding allocated ports.")
		} else {
			noun := "processes"
			if killed == 1 {
				noun = "process"
			}
			logging.Info("✓ %d %s killed", killed, noun)
		}
		return nil
	},
}

// pidsListeningOn returns PIDs of processes listening on the given TCP port.
// Uses lsof which is universally available on macOS and Linux.
func pidsListeningOn(p int) []int {
	cmd := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(p), "-sTCP:LISTEN", "-t")
	out, err := cmd.Output()
	if err != nil {
		// lsof exits non-zero when nothing matches. Treat as empty.
		return nil
	}
	var pids []int
	for _, line := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(line); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// terminatePID sends SIGTERM to the process group; if still alive after the
// grace window, sends SIGKILL. Falls back to single-PID signals if the
// process is not a process-group leader.
func terminatePID(pid int) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// Process already gone.
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !process.IsProcessRunning(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	// Brief wait for kernel to reap.
	for i := 0; i < 10; i++ {
		if !process.IsProcessRunning(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func init() {
	resetCmd.Flags().BoolVar(&resetAll, "all", false, "Reset every worktree's allocated ports")
	rootCmd.AddCommand(resetCmd)
}
