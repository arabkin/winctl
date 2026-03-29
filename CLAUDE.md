# WinCtl

Windows machine control daemon — Go CLI with embedded web dashboard and REST API.

## Build & Run

```bash
go mod tidy
go build -o bin/winctl .                                     # current platform
GOOS=windows GOARCH=amd64 go build -o bin/winctl.exe .       # Windows cross-compile
./bin/winctl run                                              # foreground mode
./bin/winctl run -d                                           # dry-run mode (no real OS commands)
```

## Test

```bash
# Go tests (64 tests across config, state, scheduler, server packages)
go test ./... -v

# Playwright E2E (requires server running on localhost:8443)
cd e2e && npm install && npx playwright install chromium && npx playwright test
```

## Project Structure

- `main.go` — entry point, delegates to `cmd.Run()`
- `cmd/` — CLI subcommands (install, uninstall, start, stop, run) with `//go:build windows` and `//go:build !windows` variants
- `service/` — Windows `svc.Handler` implementation with build-tag stubs for non-Windows
- `server/` — HTTP server (`server.go`), session-based auth with Basic Auth middleware (`auth.go`), REST handlers (`handlers.go`)
- `scheduler/` — timer goroutines for scheduled/one-shot restart and lock actions; accepts injectable `ExecFuncs` for testability
- `executor/` — OS command wrappers (`shutdown /r /t 60`, `rundll32 LockWorkStation`) plus `DryRestart()` / `DryLockScreen()` variants that log instead of executing
- `config/` — JSON config loader with base64 password, auto-creates defaults on first run; `testing.go` exports `NewForTest()` helper
- `state/` — thread-safe in-memory state with `sync.RWMutex`
- `web/` — `go:embed` static files (HTML/CSS/JS dashboard)
- `e2e/` — Playwright test suite

## Code Conventions

- **Go stdlib only** plus `golang.org/x/sys` for Windows service integration — no frameworks
- **Build tags**: Windows-specific code in `*_windows.go`, stubs in `*_other.go`
- HTTP routing uses `http.ServeMux` with `HandleFunc`; method dispatch inside handlers
- Auth uses `crypto/subtle.ConstantTimeCompare` — no timing side-channels
- Scheduler is testable via `NewWithExec(ctx, state, ExecFuncs)` — pass mock functions in tests; `UpdateIntervals()` allows hot-reloading interval ranges
- Config password is stored as base64 in `config.json`, decoded at load time
- Config resolves path relative to `os.Executable()`, not working directory (important for Windows services)
- All API responses are `application/json` with a `"status"` field for action endpoints
- State mutations are guarded by `sync.RWMutex`; `Status()` returns a deep copy (DTO with cloned `*time.Time` pointers)
- Schedule operations are idempotent — double-start is a no-op, stop-when-idle is safe
- `run` subcommand accepts `-d` / `--dry-run` flag via `flag.FlagSet`; when active, `scheduler.New()` wires dry-run executor functions that log `[DRY RUN]` instead of running OS commands
- Dry-run is foreground-only; the Windows service always runs in real mode

## API Endpoints

All require Basic Auth (session cookie established on first auth). UI at `/`.

| Method | Path | Action |
|--------|------|--------|
| GET | `/api/status` | Current state (includes `dry_run` flag) |
| POST | `/api/restart/once` | One-shot restart (60s) |
| POST/DELETE | `/api/restart/schedule` | Toggle recurring restart |
| POST | `/api/lock/once` | One-shot lock (60s) |
| POST/DELETE | `/api/lock/schedule` | Toggle recurring lock |
| POST | `/api/reset` | Cancel everything |
| GET | `/api/config` | Current config values (excludes password) |
| POST | `/api/config/reload` | Reload config from disk (updates auth, intervals) |
| POST | `/api/logout` | Invalidate session and force re-auth |

## Known Constraints

- Screen lock (`rundll32 LockWorkStation`) requires service to run under a user account, not SYSTEM
- `shutdown /r /t 60` works from any session including SYSTEM
- Config reload updates credentials and intervals; port changes require restart
- State is in-memory only — schedules do not persist across service restarts
- Config validation rejects: port outside 1-65535, empty username/password, interval min < 1, max < min
