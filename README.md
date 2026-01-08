# Devir - Dev Runner CLI

A terminal UI for managing multiple dev services with colored logs, filtering, and MCP integration.

## Features

- **Bubble Tea TUI** - Interactive terminal UI with tabs, viewport, and status bar
- **Colored Logs** - Each service has its own color for easy identification
- **Service Filtering** - View logs from all services or filter by specific service
- **Search** - Filter logs by text pattern
- **Port Management** - Detects ports in use and offers to kill them
- **MCP Server** - Integrate with Claude Code via Model Context Protocol

## Installation

```bash
cd services/devir
go build -o ../../devir .
```

## Usage

### TUI Mode (default)

```bash
# Start all default services
./devir

# Start specific services
./devir admin server

# With filters
./devir --filter "error"
./devir --exclude "hmr"
```

### MCP Server Mode

```bash
./devir --mcp
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Cycle through services |
| `1-9` | Select specific service |
| `a` | Show all services |
| `/` | Search logs |
| `r` | Restart current service |
| `j/k` | Scroll up/down |
| `q` | Quit |

## Configuration

Create `devir.yaml` in your project root:

```yaml
services:
  admin:
    dir: apps/admin
    cmd: bun run dev
    port: 3000
    color: blue

  server:
    dir: server
    cmd: bun run dev
    port: 3123
    color: magenta

defaults:
  - admin
  - server
```

### Service Options

| Field | Description |
|-------|-------------|
| `dir` | Working directory (relative to config file) |
| `cmd` | Command to run |
| `port` | Port number (for status display) |
| `color` | Log prefix color: `blue`, `green`, `yellow`, `magenta`, `cyan`, `red`, `white` |

## MCP Integration

Add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "devir": {
      "command": "/path/to/devir",
      "args": ["--mcp", "-c", "/path/to/devir.yaml"]
    }
  }
}
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `devir_start` | Start services |
| `devir_stop` | Stop all services |
| `devir_status` | Get service status |
| `devir_logs` | Get recent logs |
| `devir_restart` | Restart a service |

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) - MCP server

## License

MIT
