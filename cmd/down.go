package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/logging"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
	"github.com/spf13/cobra"
)

var (
	downAll     bool
	downService string
	downPrune   bool
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop dev servers for the current worktree",
	Long:  "Stops all running services (or a specific one) for the current worktree, or all worktrees with --all.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}

		stateDir := filepath.Join(stateRoot, ".portree")
		store, err := state.NewFileStore(stateDir)
		if err != nil {
			return fmt.Errorf("creating state store: %w", err)
		}

		// Standalone --prune (no --all, no --service): only prune. With other
		// flags, --prune composes additively and runs after the stop loop.
		if downPrune && !downAll && downService == "" {
			return pruneOrphanedState(store, cwd)
		}

		if downService != "" {
			if _, ok := cfg.Services[downService]; !ok {
				return fmt.Errorf("unknown service %q", downService)
			}
		}

		registry := port.NewRegistry(store, cfg)
		mgr := process.NewManager(cfg, store, registry)

		var trees []git.Worktree
		if downAll {
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

		totalStopped := 0
		for _, tree := range trees {
			if tree.IsBare {
				continue
			}
			results := mgr.StopServices(&tree, downService)
			for _, r := range results {
				if r.Err != nil {
					logging.Error("stopping %s/%s: %v", r.Branch, r.Service, r.Err)
				} else {
					logging.Info("Stopping %s for %s ...", r.Service, r.Branch)
					totalStopped++
				}
			}
		}

		if totalStopped > 0 {
			noun := "services"
			if totalStopped == 1 {
				noun = "service"
			}
			if downAll {
				logging.Info("✓ %d %s stopped", totalStopped, noun)
			} else {
				logging.Info("✓ %d %s stopped for %s", totalStopped, noun, trees[0].Branch)
			}
		}

		if downPrune {
			return pruneOrphanedState(store, cwd)
		}

		return nil
	},
}

// pruneOrphanedState removes state entries for branches whose worktrees no
// longer exist and reaps stale entries whose recorded PID is no longer alive.
// Both kinds are non-destructive — no live processes are signalled.
func pruneOrphanedState(store *state.FileStore, cwd string) error {
	trees, err := git.ListWorktrees(cwd)
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	activeBranches := make(map[string]bool, len(trees))
	for _, t := range trees {
		if !t.IsBare {
			activeBranches[t.Branch] = true
		}
	}

	var pruned, reaped []string
	if err := store.WithLock(func() error {
		st, e := store.Load()
		if e != nil {
			return e
		}

		for _, branch := range state.OrphanedBranches(st, activeBranches) {
			pruned = append(pruned, branch)
			delete(st.Services, branch)
		}

		// Clean up port assignments for pruned branches.
		for key := range st.PortAssignments {
			branch, _ := state.ParsePortKey(key)
			if !activeBranches[branch] {
				delete(st.PortAssignments, key)
			}
		}

		// Reap stale entries: status=running but PID is no longer alive.
		for branch, services := range st.Services {
			for svcName, ss := range services {
				if ss.Status == state.StatusRunning && ss.PID > 0 && !process.IsProcessRunning(ss.PID) {
					reaped = append(reaped, fmt.Sprintf("%s/%s", branch, svcName))
					state.SetServiceState(st, branch, svcName, state.StoppedServiceState(ss.Port))
				}
			}
		}

		return store.Save(st)
	}); err != nil {
		return fmt.Errorf("pruning state: %w", err)
	}

	if len(pruned) > 0 {
		sort.Strings(pruned)
		logging.Info("Pruned %d orphaned branch(es): %s", len(pruned), strings.Join(pruned, ", "))
	}
	if len(reaped) > 0 {
		sort.Strings(reaped)
		logging.Info("Reaped %d stale entry/entries: %s", len(reaped), strings.Join(reaped, ", "))
	}
	if len(pruned) == 0 && len(reaped) == 0 {
		logging.Info("No orphaned or stale state entries found.")
	}

	return nil
}

func init() {
	downCmd.Flags().BoolVar(&downAll, "all", false, "Stop services for all worktrees")
	downCmd.Flags().StringVar(&downService, "service", "", "Stop only a specific service")
	downCmd.Flags().BoolVar(&downPrune, "prune", false, "Remove state entries for deleted worktrees")
	rootCmd.AddCommand(downCmd)
}
