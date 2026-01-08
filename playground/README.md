# Devir Playground

Test environment for testing daemon mode and service types.

## Service Types

This playground demonstrates 4 different service types:

| Type | Description | Example |
|------|-------------|---------|
| `service` (default) | Long-running process | `web` - Python HTTP server |
| `oneshot` | Runs once and exits | `setup` - Startup script |
| `interval` | Runs at regular intervals | `health` - Health check every 5 seconds |
| `http` | Makes HTTP requests | `api-check` - httpbin.org GET request |

## Status Symbols

- `●` Running - Service is actively running
- `✓` Completed - Oneshot completed successfully
- `✗` Failed - Service encountered an error
- `◐` Waiting - Interval service waiting for next run
- `○` Stopped - Service is stopped

## Setup

```bash
# 1. Build Devir
cd ..
make build

# 2. Create MCP config (for Claude Code)
cd playground
cp .mcp.json.example .mcp.json

# 3. Edit the path in .mcp.json
```

## Usage

```bash
# Start TUI
../devir

# Connect in MCP mode (for Claude Code)
../devir --mcp
```

## devir.yaml Example

```yaml
services:
  # Long-running (default)
  web:
    dir: service1
    cmd: python3 -m http.server 8080
    port: 8080
    color: blue

  # Oneshot
  setup:
    type: oneshot
    dir: .
    cmd: echo "Setup complete!"
    color: yellow

  # Interval
  health:
    type: interval
    interval: 5s
    dir: .
    cmd: curl -sf http://localhost:8080 && echo "OK"
    color: green

  # HTTP
  api-check:
    type: http
    url: https://httpbin.org/get
    method: GET
    color: magenta
```

## Test Scenarios

### Scenario 1: Different Service Types
1. Start `../devir`
2. See `setup` oneshot complete with `✓`
3. See `web` service running with `●`
4. See `health` service waiting with `◐`, running every 5 seconds
5. See `api-check` HTTP service complete with `✓`

### Scenario 2: TUI → TUI
1. Terminal 1: `../devir`
2. Terminal 2: `../devir`
3. Both terminals should show the same logs and statuses

### Scenario 3: MCP Status
1. Call `devir_status` in Claude Code
2. See type, status, runCount info for all services
