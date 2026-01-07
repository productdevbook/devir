package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// LogEntry represents a single log line
type LogEntry struct {
	Time    time.Time
	Level   string // info, warn, error, debug
	Service string
	Message string
}

// Model is the Bubble Tea model
type Model struct {
	runner     *Runner
	services   []string
	activeTab  int // -1 = all, 0+ = specific service
	viewport   viewport.Model
	logs       []LogEntry
	width      int
	height     int
	ready      bool
	quitting   bool
	searching  bool
	searchInput textinput.Model
	searchQuery string
	autoScroll bool
}

// tickMsg is sent periodically to update logs
type tickMsg time.Time

// logMsg is sent when new logs arrive
type logMsg []LogEntry

// NewModel creates a new Model
func NewModel(runner *Runner) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	return Model{
		runner:      runner,
		services:    runner.ServiceOrder,
		activeTab:   -1, // All
		logs:        make([]LogEntry, 0, 1000),
		searchInput: ti,
		autoScroll:  true,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// Start services
	m.runner.StartWithChannel()

	return tea.Batch(
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.searchQuery = ""
				m.searchInput.SetValue("")
			case "enter":
				m.searchQuery = m.searchInput.Value()
				m.searching = false
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
			}
		} else {
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				m.runner.Stop()
				return m, tea.Quit

			case "tab":
				m.activeTab++
				if m.activeTab >= len(m.services) {
					m.activeTab = -1
				}
				m.updateViewport()

			case "shift+tab":
				m.activeTab--
				if m.activeTab < -1 {
					m.activeTab = len(m.services) - 1
				}
				m.updateViewport()

			case "a":
				m.activeTab = -1
				m.updateViewport()

			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				idx := int(msg.String()[0] - '1')
				if idx < len(m.services) {
					m.activeTab = idx
					m.updateViewport()
				}

			case "/":
				m.searching = true
				m.searchInput.Focus()
				cmds = append(cmds, textinput.Blink)

			case "r":
				if m.activeTab >= 0 && m.activeTab < len(m.services) {
					m.runner.RestartService(m.services[m.activeTab])
				}

			case "up", "k":
				m.viewport.LineUp(1)
				m.autoScroll = false

			case "down", "j":
				m.viewport.LineDown(1)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				}

			case "pgup":
				m.viewport.HalfViewUp()
				m.autoScroll = false

			case "pgdown":
				m.viewport.HalfViewDown()
				if m.viewport.AtBottom() {
					m.autoScroll = true
				}

			case "home", "g":
				m.viewport.GotoTop()
				m.autoScroll = false

			case "end", "G":
				m.viewport.GotoBottom()
				m.autoScroll = true
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 2  // Tabs
		footerHeight := 3  // Status + help
		viewportHeight := m.height - headerHeight - footerHeight

		if !m.ready {
			m.viewport = viewport.New(m.width, viewportHeight)
			m.viewport.SetContent("")
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = viewportHeight
		}

	case tickMsg:
		// Collect new logs from runner
		m.collectLogs()
		m.updateViewport()
		cmds = append(cmds, tickCmd())
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) collectLogs() {
	// Non-blocking read from log channel
	for {
		select {
		case entry := <-m.runner.LogEntryChan:
			m.logs = append(m.logs, entry)
			// Keep only last 2000 entries
			if len(m.logs) > 2000 {
				m.logs = m.logs[len(m.logs)-2000:]
			}
		default:
			return
		}
	}
}

func (m *Model) updateViewport() {
	content := m.renderLogs()
	m.viewport.SetContent(content)

	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *Model) getFilteredLogs() []LogEntry {
	var filtered []LogEntry

	for _, entry := range m.logs {
		// Filter by active tab
		if m.activeTab >= 0 {
			if entry.Service != m.services[m.activeTab] {
				continue
			}
		}

		// Filter by search query
		if m.searchQuery != "" {
			// Simple case-insensitive search
			if !containsIgnoreCase(entry.Message, m.searchQuery) &&
				!containsIgnoreCase(entry.Service, m.searchQuery) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
		 len(substr) == 0 ||
		 findIgnoreCase(s, substr))
}

func findIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
