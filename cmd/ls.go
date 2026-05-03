package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/logging"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
	"github.com/spf13/cobra"
)

type lsEntry struct {
	Worktree  string `json:"worktree"`
	Service   string `json:"service"`
	URL       string `json:"url,omitempty"`
	Reachable bool   `json:"reachable,omitempty"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	DirectURL string `json:"direct_url,omitempty"`
}

const probeTimeout = 500 * time.Millisecond

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all worktrees and their services",
	Long: `List all git worktrees and the status of each configured service.

Displays a table with worktree branch, service name, allocated port,
running status, and PID for each service.

Use --json to output the result as a JSON array for scripting and automation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}

		trees, err := git.ListWorktrees(cwd)
		if err != nil {
			return fmt.Errorf("listing worktrees: %w", err)
		}

		// Load state for runtime info.
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
		if st == nil {
			st = &state.State{
				Services:        map[string]map[string]*state.ServiceState{},
				PortAssignments: map[string]int{},
			}
		}

		// Sort service names for consistent output.
		serviceNames := make([]string, 0, len(cfg.Services))
		for name := range cfg.Services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)

		entries := buildLsEntries(trees, serviceNames, st, cfg, &st.Proxy)

		// Detect orphaned branches: in state but not in worktree list.
		activeBranches := make(map[string]bool, len(trees))
		for _, t := range trees {
			if !t.IsBare {
				activeBranches[t.Branch] = true
			}
		}
		orphanBranches := state.OrphanedBranches(st, activeBranches)
		sort.Strings(orphanBranches)
		for _, branch := range orphanBranches {
			for _, svcName := range serviceNames {
				entries = append(entries, lsEntry{
					Worktree: branch + " (orphaned)",
					Service:  svcName,
					Status:   state.StatusStopped,
				})
			}
		}

		probeReachability(entries, probeTimeout)

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			return json.NewEncoder(os.Stdout).Encode(entries)
		}

		return printLsTable(entries)
	},
}

func buildLsEntries(trees []git.Worktree, serviceNames []string, st *state.State, c *config.Config, proxy *state.ProxyState) []lsEntry {
	// Determine proxy scheme and whether proxy is available.
	proxyRunning := proxy != nil && proxy.Status == state.StatusRunning && proxy.PID > 0
	scheme := "http"
	if proxy != nil && proxy.HTTPS {
		scheme = "https"
	}

	entries := make([]lsEntry, 0)
	for _, tree := range trees {
		if tree.IsBare {
			continue
		}
		branch := tree.Branch
		if branch == "" {
			branch = "(detached)"
		}

		slug := tree.Slug()

		for _, svcName := range serviceNames {
			e := lsEntry{
				Worktree: branch,
				Service:  svcName,
				Status:   state.StatusStopped,
			}

			ss := state.GetServiceState(st, tree.Branch, svcName)
			if ss != nil {
				e.Port = ss.Port
				switch {
				case ss.PID > 0 && process.IsProcessRunning(ss.PID):
					e.Status = state.StatusRunning
					e.PID = ss.PID
				case ss.Status == state.StatusRunning && ss.PID > 0:
					e.Status = state.StatusStopped // stale
				default:
					e.Status = ss.Status
				}
			}

			// Build URLs.
			if proxyRunning && c != nil {
				if svc, ok := c.Services[svcName]; ok {
					e.URL = fmt.Sprintf("%s://%s.localhost:%d", scheme, slug, svc.ProxyPort)
				}
			}
			if e.Port > 0 {
				e.DirectURL = fmt.Sprintf("http://localhost:%d", e.Port)
			}

			entries = append(entries, e)
		}
	}
	return entries
}

// probeReachability issues parallel HEAD requests against each entry's proxy
// URL with a short timeout. An entry is marked reachable when the proxy
// returns a status code below 500 — anything 5xx (including 502 Bad Gateway
// from the proxy when the upstream service is dead) leaves it unreachable.
func probeReachability(entries []lsEntry, timeout time.Duration) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// Probes target dev servers on `*.localhost` addresses with
			// auto-generated self-signed certs (per ADR-003). The probe is
			// loopback-only and never reaches the network — InsecureSkipVerify
			// is intentional. //nolint:gosec // G402: dev-server probe over loopback
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	var wg sync.WaitGroup
	for i := range entries {
		if entries[i].URL == "" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodHead, entries[i].URL, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil && resp.StatusCode < 500 {
				entries[i].Reachable = true
			}
		}(i)
	}
	wg.Wait()
}

func printLsTable(entries []lsEntry) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	hasURLs := false
	for _, e := range entries {
		if e.URL != "" {
			hasURLs = true
			break
		}
	}

	if hasURLs {
		_, _ = fmt.Fprintln(w, "WORKTREE\tSERVICE\tURL\tPORT\tSTATUS\tPID")
	} else {
		_, _ = fmt.Fprintln(w, "WORKTREE\tSERVICE\tPORT\tSTATUS\tPID")
	}

	for _, e := range entries {
		portStr := "—"
		pidStr := "—"
		if e.Port > 0 {
			portStr = fmt.Sprintf("%d", e.Port)
		}
		if e.PID > 0 {
			pidStr = fmt.Sprintf("%d", e.PID)
		}

		if hasURLs {
			urlStr := "—"
			if e.URL != "" {
				if e.Reachable {
					urlStr = e.URL
				} else {
					urlStr = e.URL + " (unreachable)"
				}
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Worktree, e.Service, urlStr, portStr, e.Status, pidStr)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Worktree, e.Service, portStr, e.Status, pidStr)
		}
	}

	return w.Flush()
}

func init() {
	lsCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(lsCmd)
}
