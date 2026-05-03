package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fairy-pitta/portree/internal/state"
)

// Fixed column widths for SERVICE, PORT, STATUS, PID, and the optional URL
// column rendered when at least one row carries a non-empty URL.
const (
	colServiceWidth = 12
	colPortWidth    = 8
	colStatusWidth  = 14
	colPIDWidth     = 10
	colURLWidth     = 38
	colSeparators   = 4 * 2 // 4 separators × 2 chars ("  ")
	colCursorPrefix = 2     // "▸ " or "  "
	colMinWorktree  = 18
)

// fixedColumnsWidth is the sum of all non-WORKTREE columns plus separators and cursor.
const fixedColumnsWidth = colServiceWidth + colPortWidth + colStatusWidth + colPIDWidth + colSeparators + colCursorPrefix

// worktreeColumnWidth computes the dynamic WORKTREE column width. When the
// URL column is rendered, hasURL adjusts the budget accordingly.
func worktreeColumnWidth(termWidth int, hasURL bool) int {
	// borderOverhead accounts for borderStyle: RoundedBorder (1 char each side) + Padding(1,2) (2 chars each side) = ~6.
	// Update this if borderStyle in styles.go changes.
	const borderOverhead = 6
	extra := 0
	if hasURL {
		extra = colURLWidth + 2 // column + one separator
	}
	available := termWidth - fixedColumnsWidth - extra - borderOverhead
	if available < colMinWorktree {
		return colMinWorktree
	}
	return available
}

// renderTable renders the dashboard table with the given rows and cursor
// position. When the proxy is running and at least one row carries a URL,
// an additional URL column is inserted (showing the proxy URL with a
// reachability indicator).
func renderTable(rows []ServiceRow, cursor int, termWidth int) string {
	hasURL := false
	for _, row := range rows {
		if row.URL != "" {
			hasURL = true
			break
		}
	}
	wtWidth := worktreeColumnWidth(termWidth, hasURL)

	var b strings.Builder

	// Header
	headerCells := []string{
		lipgloss.NewStyle().Width(wtWidth).Bold(true).Foreground(colorWhite).Render("WORKTREE"),
		lipgloss.NewStyle().Width(colServiceWidth).Bold(true).Foreground(colorWhite).Render("SERVICE"),
	}
	if hasURL {
		headerCells = append(headerCells,
			lipgloss.NewStyle().Width(colURLWidth).Bold(true).Foreground(colorWhite).Render("URL"))
	}
	headerCells = append(headerCells,
		lipgloss.NewStyle().Width(colPortWidth).Bold(true).Foreground(colorWhite).Render("PORT"),
		lipgloss.NewStyle().Width(colStatusWidth).Bold(true).Foreground(colorWhite).Render("STATUS"),
		lipgloss.NewStyle().Width(colPIDWidth).Bold(true).Foreground(colorWhite).Render("PID"),
	)
	header := "  " + strings.Join(headerCells, "  ") // leading spaces align with cursor prefix on data rows
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Rows
	for i, row := range rows {
		portStr := "—"
		if row.Port > 0 {
			portStr = fmt.Sprintf("%d", row.Port)
		}

		statusStr := statusStopped
		if row.Status == state.StatusRunning {
			statusStr = statusRunning
		}

		pidStr := "—"
		if row.PID > 0 {
			pidStr = fmt.Sprintf("%d", row.PID)
		}

		cells := []string{
			lipgloss.NewStyle().Width(wtWidth).Render(row.Branch),
			lipgloss.NewStyle().Width(colServiceWidth).Render(row.Service),
		}
		if hasURL {
			cells = append(cells,
				lipgloss.NewStyle().Width(colURLWidth).Render(formatURLCell(row.URL, row.Reachable)))
		}
		cells = append(cells,
			lipgloss.NewStyle().Width(colPortWidth).Render(portStr),
			lipgloss.NewStyle().Width(colStatusWidth).Render(statusStr),
			lipgloss.NewStyle().Width(colPIDWidth).Render(pidStr),
		)

		line := strings.Join(cells, "  ")

		if i == cursor {
			// Prepend cursor indicator
			line = "▸ " + line
			b.WriteString(selectedRowStyle.Render(line))
		} else {
			line = "  " + line
			b.WriteString(rowStyle.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// formatURLCell composes the URL column value: the URL itself plus a small
// "✓" or "✗" suffix indicating reachability. When the row has no URL (e.g.
// the proxy isn't running for this service), an em-dash placeholder is used.
func formatURLCell(url string, reachable bool) string {
	if url == "" {
		return "—"
	}
	marker := "✗"
	if reachable {
		marker = "✓"
	}
	return fmt.Sprintf("%s %s", marker, url)
}

// renderProxyStatus renders the proxy status line.
func renderProxyStatus(running bool, ports []int) string {
	if running {
		portStrs := make([]string, len(ports))
		for i, p := range ports {
			portStrs[i] = fmt.Sprintf(":%d", p)
		}
		return proxyRunningStyle.Render(
			fmt.Sprintf("Proxy: ● running (%s)", strings.Join(portStrs, ", ")))
	}
	return proxyStoppedStyle.Render("Proxy: ○ stopped")
}

// renderHelp renders the key binding help bar with automatic wrapping.
func renderHelp(keys KeyMap, width int) string {
	items := []string{
		"[s] start", "[x] stop", "[r] restart", "[o] open", "[l] logs",
		"[a] all start", "[X] all stop", "[p] proxy", "[q] quit",
	}

	// Account for border padding (~6 chars)
	maxWidth := width - 6
	if maxWidth < 40 {
		maxWidth = 40
	}

	var lines []string
	var currentLine []string
	currentLen := 0
	separator := "  "

	for _, item := range items {
		itemLen := len(item)
		newLen := currentLen + itemLen
		if len(currentLine) > 0 {
			newLen += len(separator)
		}

		if newLen > maxWidth && len(currentLine) > 0 {
			lines = append(lines, helpStyle.Render(strings.Join(currentLine, separator)))
			currentLine = []string{item}
			currentLen = itemLen
		} else {
			currentLine = append(currentLine, item)
			currentLen = newLen
		}
	}

	if len(currentLine) > 0 {
		lines = append(lines, helpStyle.Render(strings.Join(currentLine, separator)))
	}

	return strings.Join(lines, "\n")
}
