package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg triggers periodic state refresh.
type TickMsg struct{}

// StatusUpdateMsg carries refreshed service status data.
type StatusUpdateMsg struct {
	Rows []ServiceRow
}

// ServiceRow represents a single row in the dashboard table.
//
// URL and Reachable are populated when the proxy is running — URL holds the
// fully formed proxy URL (`http://{slug}.localhost:{proxy_port}`), Reachable
// is the result of a HEAD probe against it (true iff the response status is
// below 500). When the proxy is stopped, URL is empty and Reachable is false.
type ServiceRow struct {
	Branch    string
	Slug      string
	Service   string
	Port      int
	Status    string // state.StatusRunning or state.StatusStopped
	PID       int
	URL       string
	Reachable bool
}

// ActionResultMsg carries the result of a user action (start/stop/restart).
type ActionResultMsg struct {
	Message string
	IsError bool
}

// ProxyStatusMsg carries the proxy status.
type ProxyStatusMsg struct {
	Running bool
	Ports   []int
}

// tickCmd returns a command that sends a TickMsg after 2 seconds.
func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(_ time.Time) tea.Msg {
		return TickMsg{}
	})
}
