package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/logging"
	"github.com/fairy-pitta/portree/internal/state"
	"github.com/fairy-pitta/portree/internal/status"
	"github.com/spf13/cobra"
)

var (
	statusAll  bool
	statusJSON bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show health and reachability for the calling worktree's services and the shared proxy",
	Long: `Status reports each configured service's runtime state, allocated port,
direct URL (http://localhost:<port>), and proxy URL (when the proxy is
running) — plus an independent reachability indicator for each.

The Proxy block reports whether the shared proxy is running, on which
ports, and whether it's accepting connections.

Output is hierarchical and one-glance for humans by default; pass --json
for a structured form designed for automation (Playwright, scripts).

By default only the calling worktree is reported. Pass --all to cover
every non-bare worktree of the repository.`,
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

		var st *state.State
		if err := store.WithLock(func() error {
			var e error
			st, e = store.Load()
			return e
		}); err != nil {
			logging.Warn("failed to load state: %v", err)
		}

		var trees []git.Worktree
		if statusAll {
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

		statuses := status.Build(trees, cfg, st)
		status.Probe(statuses, 500*time.Millisecond)

		if statusJSON {
			return emitStatusJSON(statuses)
		}
		return emitStatusHuman(statuses)
	},
}

// emitStatusJSON writes the status report as a JSON array to stdout. With
// only one worktree the array contains a single element — consumers can
// always treat the output as an array regardless of `--all` to keep parsing
// uniform.
func emitStatusJSON(statuses []status.WorktreeStatus) error {
	if statuses == nil {
		statuses = []status.WorktreeStatus{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(statuses)
}

// emitStatusHuman writes a per-worktree hierarchical report. One block per
// worktree: Worktree header, Services list, Proxy block.
func emitStatusHuman(statuses []status.WorktreeStatus) error {
	if len(statuses) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}
	for i, ws := range statuses {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Worktree  %s  (slug: %s)\n", ws.Worktree, ws.Slug)
		fmt.Println()

		fmt.Println("Services")
		if len(ws.Services) == 0 {
			fmt.Println("  (none configured)")
		} else {
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, svc := range ws.Services {
				marker := "✗"
				if svc.Status == state.StatusRunning {
					marker = "✓"
				}
				portCell := "—"
				if svc.Port > 0 {
					portCell = fmt.Sprintf("port %d", svc.Port)
				}
				pidCell := svc.Status
				if svc.PID > 0 {
					pidCell = fmt.Sprintf("%s (PID %d)", svc.Status, svc.PID)
				}
				_, _ = fmt.Fprintf(tw, "  %s %s\t%s\t%s\n", marker, svc.Name, portCell, pidCell)
				if svc.DirectURL != "" {
					_, _ = fmt.Fprintf(tw, "    direct\t%s\t%s\n", svc.DirectURL, reachLabel(svc.DirectReachable))
				}
				if svc.ProxyURL != "" {
					_, _ = fmt.Fprintf(tw, "    via proxy\t%s\t%s\n", svc.ProxyURL, reachLabel(svc.ProxyReachable))
				}
			}
			_ = tw.Flush()
		}

		fmt.Println()
		fmt.Println("Proxy")
		if !ws.Proxy.Running {
			fmt.Println("  ✗ not running")
			continue
		}
		ports := make([]string, len(ws.Proxy.Ports))
		for i, p := range ws.Proxy.Ports {
			ports[i] = fmt.Sprintf("%d", p)
		}
		fmt.Printf("  ✓ running (PID %d)\n", ws.Proxy.PID)
		fmt.Printf("    listening  %s\n", strings.Join(ports, ", "))
		fmt.Printf("    healthy    %s\n", yesNo(ws.Proxy.Healthy))
	}
	return nil
}

func reachLabel(reachable bool) string {
	if reachable {
		return "reachable"
	}
	return "unreachable"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func init() {
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "Report status for every non-bare worktree")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Emit JSON for scripts and automation")
	rootCmd.AddCommand(statusCmd)
}
