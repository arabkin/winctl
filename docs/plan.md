# WinCtl Design & Implementation Plan

## 1. Problem Statement

A Windows machine needs to be controlled remotely via a web interface — specifically, the ability to restart the machine and lock the screen on demand or on a randomized recurring schedule. The controlling application must run as a background daemon (Windows service), invisible to the logged-in user except through `services.msc` or Task Manager.

## 2. Design Decisions

### 2.1 Technology: Go CLI with embedded web UI

**Decision:** Single Go binary with embedded static files.

**Rationale:**
- Go compiles to a single static binary — no runtime dependencies on the target Windows machine
- `go:embed` eliminates the need to ship separate HTML/CSS/JS files
- `golang.org/x/sys/windows/svc` provides first-class Windows service integration
- Stdlib `net/http` is sufficient for this scope — no framework overhead
- Cross-compilation (`GOOS=windows`) works from any development platform

**Alternatives considered:**
- Python + Flask/FastAPI: requires Python runtime installed on target, more complex service setup via `pywin32`
- Node.js + Express: requires Node runtime, service management via `node-windows` is fragile
- C# / ASP.NET: strongest Windows integration, but heavier toolchain and overkill for this scope

### 2.2 Windows Service vs. Startup Task

**Decision:** Proper Windows service via `golang.org/x/sys/windows/svc`.

**Rationale:**
- Services start before user login — the machine is controllable even if nobody is logged in
- Managed through `services.msc` and `sc.exe` — familiar to Windows admins
- Auto-start on boot via `mgr.StartAutomatic`
- No system tray icon, no user-visible UI beyond the web dashboard

**Trade-off:** Screen lock (`LockWorkStation`) only works when the service runs under an interactive user session. Documented as a configuration step (`sc config WinCtlSvc obj=`).

### 2.3 Authentication: Session-based auth with Basic Auth login

**Decision:** HTTP Basic Auth for initial login, with `crypto/subtle.ConstantTimeCompare`. Successful auth creates a session cookie; subsequent requests use the cookie without re-sending credentials.

**Rationale:**
- Browser-native — the browser prompts automatically on 401
- Timing-safe comparison prevents credential guessing via response time analysis
- Session cookies avoid sending credentials on every request
- Logout support via session invalidation + logged-out cookie to force browser to forget cached Basic Auth credentials
- Base64-encoded password in config prevents casual shoulder-surfing but is not encryption — this is intentional for a local-network tool

**Not chosen:**
- TLS client certificates: correct but operationally heavy for this use case
- OAuth: overkill

### 2.4 Scheduler: goroutine-per-behavior with context cancellation

**Decision:** Each schedule (restart, lock) runs in its own goroutine loop. Cancellation is handled via `context.WithCancel`.

**Rationale:**
- Simple, predictable concurrency model — one goroutine per active schedule
- `context.Cancel` provides clean shutdown without leaked goroutines
- `time.NewTimer` in a select loop allows cancellation mid-wait with proper cleanup via `timer.Stop()`
- Random interval is re-rolled each cycle, not fixed at enable time

**Idempotency:** Starting an already-running schedule is a no-op (checked via nil cancel function). Stopping an inactive schedule is safe. This prevents duplicate goroutines from careless API calls.

### 2.5 Executor: injectable functions for testability and dry-run

**Decision:** The scheduler accepts an `ExecFuncs` struct with `Restart` and `LockScreen` function fields. The executor package provides both real and dry-run implementations.

**Rationale:**
- Production code passes `executor.Restart` and `executor.LockScreen`
- Dry-run mode passes `executor.DryRestart` and `executor.DryLockScreen`, which log `[DRY RUN] simulating ...` and return nil
- Tests pass no-op or counting functions — no real `shutdown` or `rundll32` in CI
- `New(ctx, state, dryRun)` selects real or dry-run executors; `NewWithExec()` allows full test injection
- Avoids interface overhead for two simple functions
- Dry-run is only available in foreground mode (`run -d` / `run --dry-run`); the Windows service always uses real executors

### 2.6 State: in-memory with mutex, no persistence

**Decision:** `sync.RWMutex`-guarded struct, no disk persistence.

**Rationale:**
- Schedules are transient — if the service restarts, starting clean is safer than resuming a stale schedule
- No database dependency, no file corruption risk
- `Status()` returns a deep copy (DTO struct with cloned `*time.Time` values), not a reference — safe for concurrent JSON serialization

**Future option:** Adding JSON persistence would be trivial — `Save()`/`LoadState()` methods are a natural extension if needed.

### 2.7 Config: auto-create with os.Executable() path resolution

**Decision:** Config file lives next to the executable, auto-created with defaults on first run.

**Rationale:**
- Windows services run with `C:\Windows\system32` as cwd — using `os.Executable()` ensures the config is always found
- Auto-creation means zero setup for the default case
- File permissions set to `0600` (owner-only) for security
- Falls back to hardcoded defaults if config is missing or malformed — the service always starts
- Config validation rejects invalid port, empty credentials, and misconfigured interval ranges at load time
- Hot reload via `POST /api/config/reload` updates credentials and scheduler intervals without restart; port changes require restart

### 2.8 Web UI: plain HTML/CSS/JS, no build step

**Decision:** Vanilla HTML with fetch-based polling, no framework or bundler.

**Rationale:**
- Three files (HTML, CSS, JS) embedded directly — no `node_modules`, no webpack, no build step
- Browser-native Basic Auth dialog handles login — no custom auth UI
- 2-second polling via `setInterval` + `fetch("/api/status")` is simple and sufficient for this refresh rate
- Countdown formatting is ~15 lines of JS

**Not chosen:**
- React/Vue/Svelte: massive overhead for 6 buttons and 4 status fields
- WebSocket: lower latency but more complex server code for negligible benefit at 2s refresh

### 2.9 Build tags for cross-platform development

**Decision:** `//go:build windows` for service/SCM code, `//go:build !windows` stubs for everything else.

**Rationale:**
- Developers on macOS/Linux can build and test the HTTP layer, config, scheduler, and state without Windows
- Only `cmd/service_windows.go` and `service/service_windows.go` require Windows
- The `run` subcommand (foreground mode) works on all platforms
- CI can run all Go tests on Linux

## 3. Architecture

```
┌─────────────┐     ┌──────────────┐     ┌────────────┐
│   Browser    │────▶│  HTTP Server │────▶│  Handlers   │
│  (Basic Auth)│◀────│  (net/http)  │◀────│  (JSON API) │
└─────────────┘     └──────────────┘     └─────┬──────┘
                                               │
                    ┌──────────────┐     ┌──────▼──────┐
                    │   Executor   │◀────│  Scheduler   │
                    │ (shutdown,   │     │ (goroutines, │
                    │  rundll32)   │     │  timers)     │
                    └──────────────┘     └──────┬──────┘
                                               │
                                        ┌──────▼──────┐
                                        │    State     │
                                        │ (RWMutex,   │
                                        │  in-memory)  │
                                        └─────────────┘
```

**Request flow:**
1. Browser sends request with Basic Auth header (first request) or session cookie (subsequent)
2. Auth middleware validates session cookie or credentials (constant-time compare), creates session on success
3. Handler dispatches to scheduler method (start/stop/reset)
4. Scheduler updates state and manages timer goroutines
5. Timer fires → executor runs OS command (or logs simulation in dry-run mode)
6. Browser polls `/api/status` every 2s → handler reads state → returns JSON (includes `dry_run` flag)

**Service lifecycle:**
1. Windows SCM calls `svc.Run()` → `Execute()` method
2. Reports `StartPending`, loads config, creates state/scheduler/server
3. Reports `Running`, enters select loop waiting for SCM commands
4. On `Stop`/`Shutdown`: cancels root context → scheduler stops goroutines → HTTP server drains connections → reports `Stopped`

## 4. API Design

All endpoints are under `/api/` and return JSON. The UI is served from `/` via embedded static files.

Action endpoints return `{"status": "descriptive message"}`.

The status endpoint returns the full state snapshot — active schedules, next fire times, pending one-shots with their target times. Null fields indicate inactive/not-pending.

Method enforcement is explicit: wrong methods return `405 Method Not Allowed`. This was chosen over a catch-all because each endpoint has specific semantics (POST to enable, DELETE to disable).

## 5. Testing Strategy

### Go unit/integration tests (64 tests)

- **config** (12): save/load roundtrip, base64 encoding, file permissions, error handling for invalid JSON and invalid base64, port/username/password/interval validation
- **state** (7): all setters/getters, reset, concurrent access stress test (100 goroutines)
- **scheduler** (14): start/stop for each behavior, idempotency (double-start, stop-when-idle), `ResetAll`, `Stop()` cancels all goroutines, random interval bounds, mock executor wiring
- **server** (31): auth reject (no creds, wrong user, wrong pass), auth accept, session cookies, session expiry, logout, all endpoints with correct methods, method rejection (405), JSON response shape validation, static file serving, idempotent schedule toggle, disable-when-inactive is safe, concurrent session creation

Tests use `httptest.NewRecorder` for API tests and `scheduler.NewWithExec` with no-op functions to avoid executing real OS commands.

### Playwright E2E tests (27 tests)

Run against a live server instance. Cover:
- Page structure (title, header, sections, buttons, connection badge)
- Authentication (401 without credentials)
- Initial idle state
- All button interactions (restart/lock one-shot, schedule on/off, reset)
- Live status polling (API-triggered state change reflected in UI within polling interval)
- Countdown text format validation
- Direct API endpoint tests (status shape, toggle state, reset, method rejection)

## 6. File Inventory

| File | Purpose |
|------|---------|
| `main.go` | Entry point → `cmd.Run()` |
| `cmd/root.go` | CLI dispatch, foreground runner, `-d`/`--dry-run` flag parsing |
| `cmd/service_windows.go` | SCM install/uninstall/start/stop |
| `cmd/service_other.go` | Stub for non-Windows |
| `service/service_windows.go` | `svc.Handler` implementation |
| `service/service_other.go` | Stub for non-Windows |
| `server/server.go` | HTTP server, mux, static file serving |
| `server/auth.go` | Session-based auth middleware with Basic Auth login, config holder |
| `server/handlers.go` | REST endpoint handlers (status, schedules, config, logout) |
| `server/server_test.go` | API + auth tests |
| `scheduler/scheduler.go` | Schedule/one-shot timer management |
| `scheduler/scheduler_test.go` | Scheduler tests with mock executors |
| `executor/executor.go` | `shutdown` and `rundll32` wrappers + dry-run variants |
| `config/config.go` | Config loader with base64 password |
| `config/config_test.go` | Config tests |
| `config/testing.go` | `NewForTest()` helper |
| `state/state.go` | Thread-safe state |
| `state/state_test.go` | State tests |
| `web/embed.go` | `go:embed` directive |
| `web/static/index.html` | Dashboard HTML |
| `web/static/style.css` | Dashboard styles |
| `web/static/app.js` | Polling + UI updates |
| `e2e/package.json` | Playwright dependencies |
| `e2e/playwright.config.ts` | Playwright config |
| `e2e/tests/dashboard.spec.ts` | E2E test suite |
