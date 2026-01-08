package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	if !m.ready {
		return "Loading...\n"
	}

	var b strings.Builder

	// Header - Tabs
	b.WriteString(m.renderTabs())
	b.WriteString("\n")

	// Viewport - Logs
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Footer - Status bar + Help
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderTabs() string {
	var tabs []string

	// All tab
	allTab := "All"
	if m.activeTab == -1 {
		tabs = append(tabs, ActiveTabStyle.Render(allTab))
	} else {
		tabs = append(tabs, TabStyle.Render(allTab))
	}

	// Service tabs
	for i, name := range m.services {
		_, _, color, icon, _, status := m.GetFullServiceStatus(name)
		statusSymbol := getStatusSymbol(status)

		// Use custom icon if defined, otherwise just name
		displayName := name
		if icon != "" {
			displayName = icon + " " + name
		}

		tabText := fmt.Sprintf("%s%s", displayName, statusSymbol)
		style := GetServiceStyle(color)

		if i == m.activeTab {
			tabs = append(tabs, ActiveTabStyle.Inherit(style).Render(tabText))
		} else {
			tabs = append(tabs, TabStyle.Inherit(style).Render(tabText))
		}
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return TabBarStyle.Width(m.width).Render(tabBar)
}

// getStatusSymbol returns appropriate symbol based on status
func getStatusSymbol(status string) string {
	switch status {
	case "running":
		return "●"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "waiting":
		return "◐"
	default:
		return "○"
	}
}

func (m Model) renderLogs() string {
	var b strings.Builder
	logs := m.GetFilteredLogs()

	for _, entry := range logs {
		_, _, color := m.GetServiceStatus(entry.Service)
		serviceStyle := GetServiceStyle(color)

		var levelStyle lipgloss.Style
		switch entry.Level {
		case "error":
			levelStyle = ErrorStyle
		case "warn":
			levelStyle = WarnStyle
		case "debug":
			levelStyle = DebugStyle
		default:
			levelStyle = InfoStyle
		}

		level := levelStyle.Render(fmt.Sprintf("%-5s", strings.ToUpper(entry.Level)))
		service := serviceStyle.Render(fmt.Sprintf("[%s]", entry.Service))
		line := fmt.Sprintf("%s %s %s\n", level, service, entry.Message)

		b.WriteString(line)
	}

	return b.String()
}

func (m Model) renderStatusBar() string {
	var parts []string
	var totalCPU float64
	var totalMemory uint64

	for _, name := range m.services {
		running, port, color, icon, svcType, status := m.GetFullServiceStatus(name)

		statusStr := getStyledStatus(status)

		serviceStyle := GetServiceStyle(color)
		portStr := ""
		if port > 0 {
			portStr = fmt.Sprintf(":%d", port)
		}

		// Use icon if defined, otherwise show type indicator
		displayName := name
		if icon != "" {
			displayName = icon + " " + name
		} else {
			// Show service type indicator only if no icon
			switch svcType {
			case "oneshot":
				displayName = name + "[1]"
			case "interval":
				displayName = name + "[∞]"
			case "http":
				displayName = name + "[H]"
			}
		}

		// Get metrics for running services
		metricsStr := ""
		if running {
			cpu, memory := m.GetServiceMetrics(name)
			totalCPU += cpu
			totalMemory += memory
			if cpu > 0 || memory > 0 {
				metricsStr = fmt.Sprintf(" %s %s", formatCPU(cpu), formatMemory(memory))
			}
		}

		parts = append(parts, fmt.Sprintf("%s %s%s%s", statusStr, serviceStyle.Render(displayName), portStr, metricsStr))
	}

	statusContent := strings.Join(parts, "  │  ")

	// Add totals when on "All" tab and there are running services
	if m.activeTab == -1 && (totalCPU > 0 || totalMemory > 0) {
		statusContent = fmt.Sprintf("Σ %s %s  │  ", formatCPU(totalCPU), formatMemory(totalMemory)) + statusContent
	}

	if m.searchQuery != "" {
		statusContent += fmt.Sprintf("  │  Filter: %s", m.searchQuery)
	}

	return StatusBarStyle.Width(m.width).Render(statusContent)
}

// formatCPU formats CPU percentage
func formatCPU(cpu float64) string {
	if cpu < 0.1 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", cpu)
}

// formatMemory formats memory in human-readable format
func formatMemory(bytes uint64) string {
	if bytes == 0 {
		return "0MB"
	}
	mb := float64(bytes) / (1024 * 1024)
	if mb >= 1024 {
		return fmt.Sprintf("%.1fGB", mb/1024)
	}
	return fmt.Sprintf("%.0fMB", mb)
}

// getStyledStatus returns styled status symbol
func getStyledStatus(status string) string {
	switch status {
	case "running":
		return StatusRunning.Render("●")
	case "completed":
		return StatusCompleted.Render("✓")
	case "failed":
		return StatusFailed.Render("✗")
	case "waiting":
		return StatusWaiting.Render("◐")
	default:
		return StatusStopped.Render("○")
	}
}

func (m Model) renderHelp() string {
	if m.searching {
		return "Search: " + m.searchInput.View()
	}

	// Show status message if present
	if m.statusMsg != "" {
		return HelpStyle.Render(m.statusMsg)
	}

	help := "Tab: switch │ 1-9: select │ a: all │ /: search │ c: copy │ x: clear │ r: restart │ q: quit"
	return HelpStyle.Render(help)
}
