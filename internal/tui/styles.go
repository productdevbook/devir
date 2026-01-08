package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors for services
	ServiceColors = map[string]lipgloss.Color{
		"blue":    lipgloss.Color("12"),
		"green":   lipgloss.Color("10"),
		"yellow":  lipgloss.Color("11"),
		"magenta": lipgloss.Color("13"),
		"cyan":    lipgloss.Color("14"),
		"red":     lipgloss.Color("9"),
		"white":   lipgloss.Color("15"),
	}

	// Tab styles
	TabStyle = lipgloss.NewStyle().
			Padding(0, 2)

	ActiveTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true).
			Background(lipgloss.Color("236"))

	TabBarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("240"))

	// Log level styles
	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	WarnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	DebugStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	// Status bar styles
	StatusBarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	StatusRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	StatusStopped = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	StatusCompleted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	StatusFailed = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	StatusWaiting = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	// Help style
	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Viewport style
	ViewportStyle = lipgloss.NewStyle()

	// Service prefix style
	PrefixStyle = lipgloss.NewStyle().
			Bold(true)
)

// GetServiceStyle returns a styled prefix for a service
func GetServiceStyle(color string) lipgloss.Style {
	c, ok := ServiceColors[color]
	if !ok {
		c = ServiceColors["white"]
	}
	return PrefixStyle.Foreground(c)
}
