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
		running, _, color := m.GetServiceStatus(name)
		status := "○"
		if running {
			status = "●"
		}

		tabText := fmt.Sprintf("%s%s", name, status)
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

	for _, name := range m.services {
		running, port, color := m.GetServiceStatus(name)

		var status string
		if running {
			status = StatusRunning.Render("●")
		} else {
			status = StatusStopped.Render("○")
		}

		serviceStyle := GetServiceStyle(color)
		portStr := ""
		if port > 0 {
			portStr = fmt.Sprintf(":%d", port)
		}

		parts = append(parts, fmt.Sprintf("%s %s%s", status, serviceStyle.Render(name), portStr))
	}

	statusContent := strings.Join(parts, "  │  ")

	if m.searchQuery != "" {
		statusContent += fmt.Sprintf("  │  Filter: %s", m.searchQuery)
	}

	return StatusBarStyle.Width(m.width).Render(statusContent)
}

func (m Model) renderHelp() string {
	if m.searching {
		return "Search: " + m.searchInput.View()
	}

	help := "Tab: switch │ 1-9: select │ a: all │ /: search │ r: restart │ q: quit"
	return HelpStyle.Render(help)
}
