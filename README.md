# Devir - Dev Runner CLI

A terminal UI for managing multiple dev services with colored logs, filtering, and MCP integration.

[![CI](https://github.com/productdevbook/devir/actions/workflows/ci.yml/badge.svg)](https://github.com/productdevbook/devir/actions/workflows/ci.yml)
[![Release](https://github.com/productdevbook/devir/actions/workflows/release.yml/badge.svg)](https://github.com/productdevbook/devir/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/productdevbook/devir)](https://goreportcard.com/report/github.com/productdevbook/devir)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/productdevbook/devir)](https://go.dev/)

## Features

- **Bubble Tea TUI** - Interactive terminal UI with tabs, viewport, and status bar
- **Colored Logs** - Each service has its own color for easy identification
- **Service Filtering** - View logs from all services or filter by specific service
- **Search** - Filter logs by text pattern
- **Port Management** - Detects ports in use and offers to kill them
- **MCP Server** - Integrate with Claude Code via Model Context Protocol

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/productdevbook/devir/master/install.sh | bash
```

### Homebrew (macOS/Linux)

```bash
brew install productdevbook/tap/devir
```

### Manual Download

Download from [Releases](https://github.com/productdevbook/devir/releases)

| Platform | Download |
|----------|----------|
| macOS (Apple Silicon) | [devir-darwin-arm64.tar.gz](https://github.com/productdevbook/devir/releases/latest/download/devir-darwin-arm64.tar.gz) |
| macOS (Intel) | [devir-darwin-amd64.tar.gz](https://github.com/productdevbook/devir/releases/latest/download/devir-darwin-amd64.tar.gz) |
| Linux (x64) | [devir-linux-amd64.tar.gz](https://github.com/productdevbook/devir/releases/latest/download/devir-linux-amd64.tar.gz) |
| Linux (ARM64) | [devir-linux-arm64.tar.gz](https://github.com/productdevbook/devir/releases/latest/download/devir-linux-arm64.tar.gz) |
| Windows (x64) | [devir-windows-amd64.zip](https://github.com/productdevbook/devir/releases/latest/download/devir-windows-amd64.zip) |

### From Source

```bash
go install github.com/productdevbook/devir@latest
```

Or build manually:

```bash
git clone https://github.com/productdevbook/devir.git
cd devir
make build
```

## Usage

### TUI Mode (default)

```bash
# Start all default services
devir

# Start specific services
devir admin server

# With filters
devir --filter "error"
devir --exclude "hmr"
```

### MCP Server Mode

```bash
devir --mcp
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
      "command": "devir",
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
| `devir_check_ports` | Check if ports are in use |
| `devir_kill_ports` | Kill processes on ports |

## Development

```bash
# Build
make build

# Build for all platforms
make build-all

# Run tests
make test

# Lint
make lint
```

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) - MCP server

## License

MIT
