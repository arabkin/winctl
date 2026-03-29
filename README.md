# WinCtl

A Go CLI application that runs as a Windows service and provides a web dashboard + REST API for controlling machine behavior — scheduled restarts and screen locks with randomized intervals.

The service runs silently in the background (no system tray icon) and is only visible in `services.msc` or Task Manager.

## Features

- **Restart control** — trigger a one-shot restart (60s delay) or enable recurring restarts at random 1–10 minute intervals
- **Screen lock control** — same options as restart: one-shot with 60s delay or random recurring schedule
- **Disable/enable** each behavior independently
- **Reset all** — cancel everything and return the machine to normal
- **Basic HTTP auth** with base64-encoded password in config
- **Auto-creates config** on first run with sensible defaults
- **Web dashboard** with live status polling (2s interval)
- **REST API** for programmatic control

## Prerequisites

- Go 1.22+ ([download](https://go.dev/dl/))
- Windows (for service install/lock screen features)
- Node.js 18+ (for Playwright E2E tests only)

## Build

```bash
# Clone or download the project
cd winctl

# Fetch dependencies
go mod tidy

# Build for current platform (macOS/Linux — foreground mode only)
go build -o winctl .

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o winctl.exe .
```

## Configuration

On first run, WinCtl creates a `config.json` next to the executable with these defaults:

```json
{
    "port": 8443,
    "username": "admin",
    "password": "Y2hhbmdlbWU="
}
```

The `password` field is base64-encoded. The default decodes to `changeme`.

To set a new password:

```bash
# Encode your password
echo -n "mysecretpassword" | base64
# Output: bXlzZWNyZXRwYXNzd29yZA==

# Put the result in config.json
```

The config file is written with `0600` permissions (owner read/write only).

## Usage

### Foreground mode (development / any OS)

```bash
./winctl run
```

Opens the HTTP server on the configured port. Stop with `Ctrl+C`.
Visit `http://localhost:8443` and enter your credentials when prompted.

### Windows service

All service commands require an **elevated (Administrator)** command prompt.

```bash
# Install as auto-start Windows service
winctl.exe install

# Start the service
winctl.exe start

# Stop the service
winctl.exe stop

# Remove the service
winctl.exe uninstall
```

After install, the service starts automatically on boot. It appears as **WinCtl Service** in `services.msc`.

#### Screen lock note

The `LockWorkStation` command only works when the service runs under an interactive user account, not `SYSTEM`. To configure this:

```cmd
sc config WinCtlSvc obj= ".\yourusername" password= "yourpassword"
```

Restart the service after changing the account.

## Web Dashboard

Navigate to `http://localhost:8443` (or your configured port). The browser prompts for Basic Auth credentials.

The dashboard has three sections:

| Section | Controls |
|---------|----------|
| **Restart** | Restart Now (60s delay), Schedule On/Off |
| **Screen Lock** | Lock Now (60s delay), Schedule On/Off |
| **Global** | Reset All |

Status updates automatically every 2 seconds, showing active schedules with countdown timers.

## REST API

All endpoints require Basic Auth. Responses are JSON.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/status` | Current state of all schedules and pending actions |
| `POST` | `/api/restart/once` | Trigger one-shot restart with 60s delay |
| `POST` | `/api/restart/schedule` | Enable recurring restart (random 1–10 min intervals) |
| `DELETE` | `/api/restart/schedule` | Disable recurring restart |
| `POST` | `/api/lock/once` | Trigger one-shot screen lock with 60s delay |
| `POST` | `/api/lock/schedule` | Enable recurring lock (random 1–10 min intervals) |
| `DELETE` | `/api/lock/schedule` | Disable recurring lock |
| `POST` | `/api/reset` | Cancel all schedules and pending actions |

### Example

```bash
# Check status
curl -u admin:changeme http://localhost:8443/api/status

# Enable restart schedule
curl -u admin:changeme -X POST http://localhost:8443/api/restart/schedule

# Reset everything
curl -u admin:changeme -X POST http://localhost:8443/api/reset
```

### Status response

```json
{
    "restart_schedule_active": true,
    "restart_next_at": "2026-03-29T14:32:00Z",
    "restart_pending_once": false,
    "restart_once_at": null,
    "lock_schedule_active": false,
    "lock_next_at": null,
    "lock_pending_once": false,
    "lock_once_at": null
}
```

## Testing

### Go unit/integration tests

```bash
go test ./... -v
```

Runs 33 tests across 4 packages:

| Package | Tests | What's covered |
|---------|-------|----------------|
| `config` | 5 | Defaults, save/load, file permissions, invalid JSON, invalid base64 |
| `state` | 7 | State operations, reset, concurrent access |
| `scheduler` | 12 | Start/stop schedules, one-shots, idempotency, reset, cancellation, random interval range |
| `server` | 19 | Auth (accept/reject), all API endpoints, method validation, JSON shape, static files |

The scheduler tests use mock executor functions — no real `shutdown` or `rundll32` commands are executed.

### Playwright E2E tests

These tests require the WinCtl server to be running.

```bash
# Terminal 1: start the server
go run . run

# Terminal 2: run Playwright tests
cd e2e
npm install
npx playwright install chromium
npx playwright test
```

Runs 27 tests covering:

- Dashboard loading, title, sections, buttons
- Authentication (401 without credentials)
- Initial idle state
- Restart controls (one-shot, schedule on/off)
- Lock controls (one-shot, schedule on/off)
- Reset All clears everything
- Live status polling and countdown display
- Direct API endpoint tests

Configure with environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `WINCTL_PORT` | `8443` | Server port |
| `WINCTL_USER` | `admin` | Auth username |
| `WINCTL_PASS` | `changeme` | Auth password (plaintext) |

## Project Structure

```
winctl/
├── main.go                  # Entry point
├── go.mod
├── config.json              # Default config (auto-created on first run)
├── cmd/
│   ├── root.go              # CLI dispatch and foreground mode
│   ├── service_windows.go   # Windows service management (install/start/stop/uninstall)
│   └── service_other.go     # Stub for non-Windows platforms
├── service/
│   ├── service_windows.go   # Windows svc.Handler implementation
│   └── service_other.go     # Stub for non-Windows platforms
├── server/
│   ├── server.go            # HTTP server setup and routing
│   ├── auth.go              # Basic auth middleware
│   ├── handlers.go          # REST API handlers
│   └── server_test.go       # API and auth tests
├── scheduler/
│   ├── scheduler.go         # Timer goroutines for scheduled actions
│   └── scheduler_test.go    # Scheduler tests with mock executors
├── executor/
│   └── executor.go          # OS command wrappers (shutdown, lock)
├── config/
│   ├── config.go            # Config loading with base64 password
│   ├── config_test.go       # Config tests
│   └── testing.go           # Test helper for creating configs
├── state/
│   ├── state.go             # Thread-safe in-memory state
│   └── state_test.go        # State tests
├── web/
│   ├── embed.go             # go:embed directive
│   └── static/
│       ├── index.html       # Dashboard
│       ├── style.css         # Styles
│       └── app.js           # Status polling and UI updates
└── e2e/
    ├── package.json
    ├── playwright.config.ts
    └── tests/
        └── dashboard.spec.ts # Playwright E2E tests
```
