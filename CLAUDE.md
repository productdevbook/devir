# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # Build binary to ./devir
make build-all    # Build for all platforms (dist/)
make test         # Run tests
make lint         # Run golangci-lint
make install      # Build and copy to /usr/local/bin
make clean        # Remove build artifacts
```

## Architecture

Devir is a terminal UI for managing multiple dev services. Written in Go using Bubble Tea TUI framework.

### Core Components

```
cmd/devir/main.go     # Entry point, CLI flags, mode selection (TUI vs MCP)
internal/
  config/             # YAML config loading, service validation
  types/              # Shared types (LogLine, LogEntry, ServiceInfo)
  runner/             # Service lifecycle management, log processing
  tui/                # Bubble Tea model, view rendering, keyboard handling
  mcp/                # MCP server for Claude Code integration
```

### Key Patterns

**Runner** (`internal/runner/runner.go`):
- Manages service processes with platform-specific handling (Unix/Windows)
- Channels for log streaming: `LogChan` (simple mode), `LogEntryChan` (TUI mode)
- Log filtering via compiled regex (`filter`, `exclude` patterns)
- Port detection and process killing via `process_unix.go`/`process_windows.go`

**TUI Model** (`internal/tui/`):
- Bubble Tea architecture: Model → Update → View cycle
- Tab-based service filtering (`activeTab`: -1=all, 0+=specific)
- Auto-scroll with manual override on user scroll
- 50ms tick for log collection

**MCP Server** (`internal/mcp/mcp.go`):
- Exposes 7 tools: `devir_start`, `devir_stop`, `devir_status`, `devir_logs`, `devir_restart`, `devir_check_ports`, `devir_kill_ports`
- Uses `go-sdk/mcp` with stdio transport
- Same runner infrastructure as TUI mode

### Configuration

Config file: `devir.yaml` (searched in current dir and parents)

```yaml
services:
  name:
    dir: relative/path    # Working directory
    cmd: command to run   # Command to execute
    port: 3000            # Port for status display
    color: blue           # Log prefix color

defaults:
  - name                  # Services to start by default
```

### Dependencies

- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/bubbles` - TUI components (viewport, textinput)
- `github.com/charmbracelet/lipgloss` - Styling
- `github.com/modelcontextprotocol/go-sdk` - MCP server
- `gopkg.in/yaml.v3` - Config parsing
