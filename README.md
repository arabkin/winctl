# WinCtl

A Go CLI application that runs as a Windows service and provides a web dashboard + REST API for controlling machine behavior — scheduled restarts and screen locks with randomized intervals.

The service runs silently in the background (no system tray icon) and is only visible in `services.msc` or Task Manager.

## Features

- **Restart control** — trigger a one-shot restart (60s delay) or enable recurring restarts at configurable random intervals
- **Screen lock control** — same options as restart: one-shot with 60s delay or random recurring schedule
- **Disable/enable** each behavior independently
- **Reset all** — cancel everything and return the machine to normal
- **Session-based auth** with Basic Auth login, session cookies, and logout support
- **Auto-creates config** on first run with sensible defaults; validated on load
- **Persistent schedules** — active schedules survive service restarts (saved to `state.json`)
- **Hot config reload** — reload credentials and intervals via API without restarting
- **Web dashboard** with live status polling (2s interval), mode indicator (dry-run / real), and config viewer
- **REST API** for programmatic control
- **Dry-run mode** (`-d` / `--dry-run`) — simulates all actions without executing them; visible in dashboard
- **Toast notifications** in the web dashboard for action confirmations and errors
- **Windows Firewall rule** — install command creates an inbound rule for private (home) networks only

## How actions work

**Restart** has a two-stage countdown:

1. **App countdown (60s)** — shown in the dashboard as a pending timer. During this stage the restart can still be cancelled via Reset All.
2. **Windows countdown (60s)** — once the app timer fires, `shutdown /r /t 60` is issued. Windows shows a "You're about to be signed out" notification with a 1-minute warning. This stage cannot be cancelled from the dashboard (use `shutdown /a` in a terminal to abort).

Total time from button press to actual restart: **~2 minutes**.

**Screen lock** has a single stage:

1. **App countdown (60s)** — same dashboard timer, cancellable via Reset All.
2. **Immediate lock** — when the timer fires, the screen locks instantly (`rundll32 LockWorkStation`). No additional delay or notification.

For **scheduled** (recurring) actions, the app picks a random interval within the configured min/max range. When the interval elapses, the action fires immediately — no additional app countdown. For restart, the Windows 60-second shutdown notice still applies.

## Prerequisites

- Go 1.26+ ([download](https://go.dev/dl/))
- Windows (for service install/lock screen features)
- Node.js 18+ (for Playwright E2E tests only)

## Build

```bash
# Clone or download the project
cd winctl

# Fetch dependencies
go mod tidy

# Build for current platform (macOS/Linux — foreground mode only)
go build -o bin/winctl .

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o bin/winctl.exe .
```

## Configuration

On first run, WinCtl creates a `config.json` next to the executable with these defaults:

```json
{
    "port": 8443,
    "username": "admin",
    "password": "Y2hhbmdlbWU=",
    "session_timeout_minutes": 30,
    "restart_min_minutes": 5,
    "restart_max_minutes": 15,
    "lock_min_minutes": 5,
    "lock_max_minutes": 15
}
```

| Field | Description | Validation |
|-------|-------------|------------|
| `port` | HTTP listen port | 1–65535 |
| `username` | Basic Auth username | Non-empty |
| `password` | Base64-encoded password | Non-empty after decode |
| `session_timeout_minutes` | Session cookie lifetime | Defaults to 30 if <= 0 |
| `restart_min_minutes` | Minimum restart interval | >= 1 |
| `restart_max_minutes` | Maximum restart interval | >= min |
| `lock_min_minutes` | Minimum lock interval | >= 1 |
| `lock_max_minutes` | Maximum lock interval | >= min |

The `password` field is base64-encoded. The default decodes to `changeme`.

To set a new password:

```bash
# Encode your password
echo -n "mysecretpassword" | base64
# Output: bXlzZWNyZXRwYXNzd29yZA==

# Put the result in config.json
```

The config file is written with `0600` permissions (owner read/write only).

**Hot reload:** Config can be reloaded without restarting via `POST /api/config/reload` or the dashboard's "Reload Configuration" button. This updates credentials and scheduler intervals. Port changes still require a restart.

## Usage

### Foreground mode (development / any OS)

```bash
./bin/winctl run                # normal mode
./bin/winctl run -d             # dry-run mode (simulates actions)
./bin/winctl run --dry-run      # same as -d
```

Opens the HTTP server on `0.0.0.0:<port>` (all interfaces). Stop with `Ctrl+C`.
Visit `http://localhost:8443` (or the machine's IP from another device) and enter your credentials when prompted.

In dry-run mode, all restart and lock actions are simulated — the app logs what it would do but does not execute any OS commands:

```
[DRY RUN] simulating restart (shutdown /r /t 60)
[DRY RUN] simulating screen lock (rundll32 LockWorkStation)
```

### Windows service

All service commands require an **elevated (Administrator)** command prompt.

```bash
# Install, start, and configure firewall (one command)
winctl.exe install

# Upgrade: download new winctl.exe, then run from the download location
new-winctl.exe upgrade    # stops service → replaces binary → starts service

# Start/stop manually if needed
winctl.exe start
winctl.exe stop

# Remove the service (also removes firewall rule)
winctl.exe uninstall
```

The install command does three things: creates the service (auto-start on boot), starts it immediately, and adds a Windows Firewall inbound rule for the configured port on **private networks only**. The service appears as **WinCtl Service** in `services.msc`. The uninstall command removes both the service and the firewall rule.

The **upgrade** command replaces the installed binary without touching the service registration, config, or firewall rule. Run the new binary from a different location (e.g. Downloads) — it looks up the installed path from the service manager, stops the service, copies itself over, creates a `.bak` backup, and restarts.

#### Screen lock note

The `LockWorkStation` command only works when the service runs under an interactive user account, not `SYSTEM`. To configure this:

```cmd
sc.exe config WinCtlSvc obj= ".\yourusername" password= "yourpassword"
```

Restart the service after changing the account.
#### Network access (home network only)

The server listens on `0.0.0.0:<port>` (all interfaces). The install command automatically creates a Windows Firewall rule that allows inbound connections on the configured port for **private (home) networks only** — public network connections are blocked.

If you need to configure the firewall rule manually:

```cmd
:: Add rule (private networks only)
netsh advfirewall firewall add rule name="WinCtl Dashboard" dir=in action=allow protocol=TCP localport=8443 profile=private

:: Verify the rule exists
netsh advfirewall firewall show rule name="WinCtl Dashboard"

:: Remove rule
netsh advfirewall firewall delete rule name="WinCtl Dashboard"
```

**Important:** Your home network must be set to "Private" profile in Windows for this to work. Verify and change it at:

```
Settings > Network & Internet > your connection > Network profile type > Private
```

You can verify the current profile via PowerShell:

```powershell
Get-NetConnectionProfile
```

Look for `NetworkCategory: Private` on your home adapter. If it shows `Public`, change it:

```powershell
Set-NetConnectionProfile -InterfaceAlias "Wi-Fi" -NetworkCategory Private
```

### Dry-run mode for E2E testing

Dry-run is useful when running E2E or Playwright tests — the server behaves normally (schedules fire, state updates) but no real restarts or screen locks occur:

```bash
./bin/winctl run -d    # safe for testing
```

## Web Dashboard

Navigate to `http://localhost:8443` (or your configured port). The browser prompts for Basic Auth credentials.

The dashboard shows a mode badge (Real / Dry Run), connection status, and a Logout button in the header.

| Section | Controls |
|---------|----------|
| **Restart** | Restart Now (60s delay), Schedule On/Off |
| **Screen Lock** | Lock Now (60s delay), Schedule On/Off |
| **Configuration** | View current config values, Reload Configuration |
| **Global** | Reset All |

Status updates automatically every 2 seconds, showing active schedules with countdown timers.

## REST API

All endpoints require Basic Auth. Responses are JSON.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/status` | Current state of all schedules and pending actions |
| `POST` | `/api/restart/once` | Trigger one-shot restart with 60s delay |
| `POST` | `/api/restart/schedule` | Enable recurring restart (random configurable intervals) |
| `DELETE` | `/api/restart/schedule` | Disable recurring restart |
| `POST` | `/api/lock/once` | Trigger one-shot screen lock with 60s delay |
| `POST` | `/api/lock/schedule` | Enable recurring lock (random configurable intervals) |
| `DELETE` | `/api/lock/schedule` | Disable recurring lock |
| `POST` | `/api/reset` | Cancel all schedules and pending actions |
| `GET` | `/api/config` | Current configuration (excludes password) |
| `POST` | `/api/config/reload` | Reload config from disk (updates auth + intervals) |
| `POST` | `/api/logout` | Invalidate session and force re-authentication |

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
    "dry_run": false,
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

### Config response

```json
{
    "port": 8443,
    "username": "admin",
    "session_timeout_minutes": 30,
    "restart_min_minutes": 5,
    "restart_max_minutes": 15,
    "lock_min_minutes": 5,
    "lock_max_minutes": 15
}
```

## Testing

### Go unit/integration tests

```bash
go test ./... -v
```

Runs 80 tests across 4 packages:

| Package | Tests | What's covered |
|---------|-------|----------------|
| `config` | 12 | Defaults, save/load, file permissions, invalid JSON, invalid base64, port/username/interval validation |
| `state` | 14 | State operations, reset, concurrent access, intent persistence (save/load, missing file, invalid JSON, file permissions), onChange callback |
| `scheduler` | 14 | Start/stop schedules, one-shots, idempotency, reset, cancellation, random interval range |
| `server` | 40 | Auth (accept/reject), sessions (creation, expiry, concurrency), logout (cookie, re-auth flow), all API endpoints, method validation, JSON shape, static files, config get/reload, idempotent schedule enable/disable |

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

Runs 25 tests covering:

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
├── main.go                  # Entry point, delegates to cmd.Run()
├── go.mod / go.sum
├── Makefile                 # Build, test, run, e2e targets
├── CLAUDE.md                # Project conventions for AI assistants
├── config.json              # Auto-created on first run (not committed)
├── cmd/
│   ├── root.go              # CLI dispatch, foreground mode, signal handling
│   ├── service_windows.go   # Windows service, firewall, and upgrade management
│   └── service_other.go     # Stub for non-Windows platforms
├── service/
│   ├── service_windows.go   # Windows svc.Handler implementation
│   └── service_other.go     # Stub for non-Windows platforms
├── server/
│   ├── server.go            # HTTP server setup and routing
│   ├── auth.go              # Basic auth + session cookie middleware
│   ├── handlers.go          # REST API handlers
│   └── server_test.go       # API, auth, session, and config tests (40 tests)
├── scheduler/
│   ├── scheduler.go         # Timer goroutines for scheduled actions
│   └── scheduler_test.go    # Scheduler tests with mock executors (14 tests)
├── executor/
│   └── executor.go          # OS command wrappers (shutdown, lock, dry-run variants)
├── config/
│   ├── config.go            # JSON config loader with base64 password, validation
│   ├── config_test.go       # Config tests (12 tests)
│   └── testing.go           # NewForTest() helper
├── state/
│   ├── state.go             # Thread-safe in-memory state with RWMutex
│   ├── persist.go           # Schedule intent persistence (state.json)
│   ├── persist_test.go      # Persistence tests
│   └── state_test.go        # State tests (14 tests)
├── web/
│   ├── embed.go             # go:embed directive
│   └── static/
│       ├── index.html       # Dashboard UI
│       ├── style.css        # Styles
│       └── app.js           # Status polling, UI updates, toast notifications
├── docs/
│   └── plan.md              # Implementation plan
└── e2e/
    ├── package.json
    ├── playwright.config.ts
    └── tests/
        └── dashboard.spec.ts # Playwright E2E tests (25 tests)
```
