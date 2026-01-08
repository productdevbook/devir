package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"devir/internal/config"
	"devir/internal/daemon"
	"devir/internal/runner"
	"devir/internal/types"
)

// Model is the Bubble Tea model
type Model struct {
	// Legacy runner mode
	Runner *runner.Runner

	// Client mode
	client   *daemon.Client
	cfg      *config.Config
	statuses map[string]daemon.ServiceStatus

	services    []string
	activeTab   int // -1 = all, 0+ = specific service
	viewport    viewport.Model
	logs        []types.LogEntry
	width       int
	height      int
	ready       bool
	quitting    bool
	searching   bool
	searchInput textinput.Model
	searchQuery string
	autoScroll  bool
	clientMode  bool
}

// tickMsg is sent periodically to update logs
type tickMsg time.Time

// New creates a new Model with runner (legacy mode)
func New(r *runner.Runner) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	return Model{
		Runner:      r,
		services:    r.ServiceOrder,
		activeTab:   -1, // All
		logs:        make([]types.LogEntry, 0, 1000),
		searchInput: ti,
		autoScroll:  true,
		clientMode:  false,
	}
}

// NewWithClient creates a new Model with daemon client
func NewWithClient(client *daemon.Client, services []string, cfg *config.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	return Model{
		client:      client,
		cfg:         cfg,
		services:    services,
		statuses:    make(map[string]daemon.ServiceStatus),
		activeTab:   -1, // All
		logs:        make([]types.LogEntry, 0, 1000),
		searchInput: ti,
		autoScroll:  true,
		clientMode:  true,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	if m.clientMode {
		// Request initial status
		m.client.Status()
		return tickCmd()
	}

	// Legacy runner mode
	m.Runner.StartWithChannel()
	return tickCmd()
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
				if m.clientMode {
					m.client.Stop()
				} else {
					m.Runner.Stop()
				}
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
					if m.clientMode {
						m.client.Restart(m.services[m.activeTab])
					} else {
						m.Runner.RestartService(m.services[m.activeTab])
					}
				}

			case "up", "k":
				m.viewport.ScrollUp(1)
				m.autoScroll = false

			case "down", "j":
				m.viewport.ScrollDown(1)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				}

			case "pgup":
				m.viewport.HalfPageUp()
				m.autoScroll = false

			case "pgdown":
				m.viewport.HalfPageDown()
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

		headerHeight := 2
		footerHeight := 3
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
		if m.clientMode {
			m.collectClientLogs()
			// Periodically request status
			m.client.Status()
		} else {
			m.collectLogs()
		}
		m.updateViewport()
		cmds = append(cmds, tickCmd())
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) collectLogs() {
	for {
		select {
		case entry := <-m.Runner.LogEntryChan:
			m.logs = append(m.logs, entry)
			if len(m.logs) > 2000 {
				m.logs = m.logs[len(m.logs)-2000:]
			}
		default:
			return
		}
	}
}

func (m *Model) collectClientLogs() {
	// Process any pending messages from client
	for {
		select {
		case msg := <-m.client.Receive():
			if msg.Type == daemon.MsgLogEntry {
				logData, err := daemon.ParsePayload[daemon.LogEntryData](msg)
				if err == nil {
					m.logs = append(m.logs, types.LogEntry{
						Time:    logData.Time,
						Level:   logData.Level,
						Service: logData.Service,
						Message: logData.Message,
					})
					if len(m.logs) > 2000 {
						m.logs = m.logs[len(m.logs)-2000:]
					}
				}
			} else if msg.Type == daemon.MsgStatusResponse {
				resp, _ := daemon.ParsePayload[daemon.StatusResponse](msg)
				for _, s := range resp.Services {
					m.statuses[s.Name] = s
				}
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

// GetFilteredLogs returns logs filtered by active tab and search query
func (m *Model) GetFilteredLogs() []types.LogEntry {
	var filtered []types.LogEntry

	for _, entry := range m.logs {
		if m.activeTab >= 0 {
			if entry.Service != m.services[m.activeTab] {
				continue
			}
		}

		if m.searchQuery != "" {
			if !containsIgnoreCase(entry.Message, m.searchQuery) &&
				!containsIgnoreCase(entry.Service, m.searchQuery) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// GetServiceStatus returns service status (works in both modes)
func (m *Model) GetServiceStatus(name string) (running bool, port int, color string) {
	if m.clientMode {
		if s, ok := m.statuses[name]; ok {
			return s.Running, s.Port, s.Color
		}
		// Get color from config
		if svc, ok := m.cfg.Services[name]; ok {
			return false, svc.Port, svc.Color
		}
		return false, 0, "white"
	}

	// Legacy mode
	if state, ok := m.Runner.Services[name]; ok {
		return state.Running, state.Service.Port, state.Service.Color
	}
	return false, 0, "white"
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
