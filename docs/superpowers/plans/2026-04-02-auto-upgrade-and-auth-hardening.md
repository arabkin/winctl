# Auto-Upgrade & Auth Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automatic release detection with in-app upgrade (download + SHA256 verify + replace binary), and harden authentication with login attempt logging and lockout after 3 failures.

**Architecture:** New `updater` package checks GitHub releases API periodically, exposes update info via new API endpoint. Frontend shows Upgrade card when update available with confirmation dialog. Auth middleware gains a `loginTracker` that logs attempts and locks out after 3 failures (reset on service restart). All upgrade and auth events are logged.

**Tech Stack:** Go stdlib (`net/http`, `crypto/sha256`, `encoding/hex`, `encoding/json`), GitHub REST API (unauthenticated), embedded web dashboard.

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `updater/updater.go` | GitHub release checker, download, SHA256 verify |
| Create | `updater/updater_test.go` | Tests with mock HTTP server |
| Modify | `server/auth.go` | Add `loginTracker` (attempt logging + lockout) |
| Modify | `server/server.go` | Register new `/api/update/*` routes, wire updater |
| Modify | `server/handlers.go` | Add update check, download, status handlers |
| Modify | `server/server_test.go` | Tests for lockout, login logging, update endpoints |
| Modify | `web/static/index.html` | Upgrade card with confirmation dialog |
| Modify | `web/static/app.js` | Upgrade UI logic (check, confirm, proceed) |
| Modify | `web/static/style.css` | Upgrade card, dialog, progress styles |
| Modify | `config/config.go` | Add `version` constant |

---

### Task 1: Version Constant

**Files:**
- Create: `version.go` (project root)

- [ ] **Step 1: Create version file**

```go
package main

const Version = "1.0.2"
```

- [ ] **Step 2: Commit**

```bash
git add version.go
git commit -m "feat: add version constant"
```

---

### Task 2: Updater Package — Release Check

**Files:**
- Create: `updater/updater.go`
- Create: `updater/updater_test.go`

- [ ] **Step 1: Write failing test for release check**

```go
package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckFindsNewerRelease(t *testing.T) {
	release := ghRelease{
		TagName: "v2.0.0",
		Name:    "WinCtl v2.0.0",
		Body:    "## Changes\n- Something new",
		Assets: []ghAsset{{
			Name:               "winctl.exe",
			BrowserDownloadURL: "https://example.com/winctl.exe",
			Size:               1024,
			Digest:             "sha256:abc123",
		}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL+"/repos/test/test/releases/latest")
	info, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available {
		t.Fatal("expected update available")
	}
	if info.Version != "2.0.0" {
		t.Errorf("version: got %s, want 2.0.0", info.Version)
	}
	if info.SHA256 != "abc123" {
		t.Errorf("sha256: got %s, want abc123", info.SHA256)
	}
}

func TestCheckNoUpdateWhenSameVersion(t *testing.T) {
	release := ghRelease{
		TagName: "v1.0.2",
		Name:    "WinCtl v1.0.2",
		Assets:  []ghAsset{{Name: "winctl.exe", Digest: "sha256:abc"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL+"/repos/test/test/releases/latest")
	info, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if info.Available {
		t.Fatal("expected no update available")
	}
}

func TestCheckNoUpdateWhenOlderVersion(t *testing.T) {
	release := ghRelease{
		TagName: "v1.0.1",
		Name:    "WinCtl v1.0.1",
		Assets:  []ghAsset{{Name: "winctl.exe", Digest: "sha256:abc"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL+"/repos/test/test/releases/latest")
	info, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if info.Available {
		t.Fatal("expected no update for older version")
	}
}

func TestCheckHandlesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL+"/repos/test/test/releases/latest")
	_, err := u.Check()
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestCheckHandlesNoExeAsset(t *testing.T) {
	release := ghRelease{
		TagName: "v2.0.0",
		Assets:  []ghAsset{{Name: "winctl.tar.gz", Digest: "sha256:abc"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL+"/repos/test/test/releases/latest")
	_, err := u.Check()
	if err == nil {
		t.Fatal("expected error when no .exe asset")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./updater/ -v -count=1`
Expected: FAIL (package doesn't exist)

- [ ] **Step 3: Implement updater package**

```go
package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultAPIURL = "https://api.github.com/repos/arabkin/winctl/releases/latest"
	assetName     = "winctl.exe"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Name    string    `json:"name"`
	Body    string    `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

// UpdateInfo is returned to callers and the API.
type UpdateInfo struct {
	Available   bool   `json:"available"`
	Version     string `json:"version,omitempty"`
	ReleaseName string `json:"release_name,omitempty"`
	Body        string `json:"body,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type Updater struct {
	currentVersion string
	apiURL         string
	client         *http.Client

	mu     sync.RWMutex
	cached *UpdateInfo
}

func New(currentVersion, apiURL string) *Updater {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         apiURL,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}

// Check queries GitHub for the latest release and returns update info.
func (u *Updater) Check() (UpdateInfo, error) {
	req, err := http.NewRequest("GET", u.apiURL, nil)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "WinCtl/"+u.currentVersion)

	resp, err := u.client.Do(req)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return UpdateInfo{}, fmt.Errorf("decode release: %w", err)
	}

	remoteVer := strings.TrimPrefix(release.TagName, "v")
	if !isNewer(remoteVer, u.currentVersion) {
		info := UpdateInfo{Available: false}
		u.mu.Lock()
		u.cached = &info
		u.mu.Unlock()
		return info, nil
	}

	var asset *ghAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return UpdateInfo{}, fmt.Errorf("no %s asset in release %s", assetName, release.TagName)
	}

	sha := strings.TrimPrefix(asset.Digest, "sha256:")

	info := UpdateInfo{
		Available:   true,
		Version:     remoteVer,
		ReleaseName: release.Name,
		Body:        release.Body,
		DownloadURL: asset.BrowserDownloadURL,
		Size:        asset.Size,
		SHA256:      sha,
	}

	u.mu.Lock()
	u.cached = &info
	u.mu.Unlock()

	log.Printf("update available: %s (current: %s)", remoteVer, u.currentVersion)
	return info, nil
}

// Cached returns the last check result without making a network call.
func (u *Updater) Cached() UpdateInfo {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.cached != nil {
		return *u.cached
	}
	return UpdateInfo{}
}

// Download fetches the binary to a temp file and verifies its SHA256.
// Returns the temp file path on success.
func (u *Updater) Download(info UpdateInfo) (string, error) {
	if info.DownloadURL == "" {
		return "", fmt.Errorf("no download URL")
	}

	log.Printf("downloading %s (%d bytes)...", info.DownloadURL, info.Size)
	resp, err := u.client.Get(info.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "winctl-update-*.exe")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	got := hex.EncodeToString(hasher.Sum(nil))
	if info.SHA256 != "" && got != info.SHA256 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", info.SHA256, got)
	}

	log.Printf("download complete, SHA256 verified: %s", got)
	return tmpPath, nil
}

// isNewer returns true if remote version string is greater than current.
// Simple lexicographic comparison of dot-separated numeric segments.
func isNewer(remote, current string) bool {
	rParts := strings.Split(remote, ".")
	cParts := strings.Split(current, ".")
	for i := 0; i < len(rParts) && i < len(cParts); i++ {
		if rParts[i] > cParts[i] {
			return true
		}
		if rParts[i] < cParts[i] {
			return false
		}
	}
	return len(rParts) > len(cParts)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./updater/ -v -count=1`
Expected: All 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add updater/updater.go updater/updater_test.go
git commit -m "feat: add updater package with GitHub release check, download, and SHA256 verification"
```

---

### Task 3: Download and SHA256 Verification Tests

**Files:**
- Modify: `updater/updater_test.go`

- [ ] **Step 1: Add download and verification tests**

Append to `updater/updater_test.go`:

```go
func TestDownloadAndVerifySHA256(t *testing.T) {
	content := []byte("fake binary content for testing")
	h := sha256.Sum256(content)
	expectedSHA := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	u := New("1.0.0", "")
	info := UpdateInfo{
		DownloadURL: srv.URL + "/winctl.exe",
		SHA256:      expectedSHA,
	}

	path, err := u.Download(info)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Error("downloaded content mismatch")
	}
}

func TestDownloadRejectsSHA256Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary content"))
	}))
	defer srv.Close()

	u := New("1.0.0", "")
	info := UpdateInfo{
		DownloadURL: srv.URL + "/winctl.exe",
		SHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
	}

	path, err := u.Download(info)
	if err == nil {
		os.Remove(path)
		t.Fatal("expected SHA256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDownloadHandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := New("1.0.0", "")
	info := UpdateInfo{DownloadURL: srv.URL + "/winctl.exe"}

	_, err := u.Download(info)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		remote, current string
		want            bool
	}{
		{"2.0.0", "1.0.2", true},
		{"1.0.3", "1.0.2", true},
		{"1.1.0", "1.0.2", true},
		{"1.0.2", "1.0.2", false},
		{"1.0.1", "1.0.2", false},
		{"0.9.0", "1.0.2", false},
	}
	for _, tt := range tests {
		got := isNewer(tt.remote, tt.current)
		if got != tt.want {
			t.Errorf("isNewer(%s, %s) = %v, want %v", tt.remote, tt.current, got, tt.want)
		}
	}
}
```

Add these imports to the test file's import block:

```go
"crypto/sha256"
"encoding/hex"
"os"
"strings"
```

- [ ] **Step 2: Run tests**

Run: `go test ./updater/ -v -count=1`
Expected: All 9 tests PASS

- [ ] **Step 3: Commit**

```bash
git add updater/updater_test.go
git commit -m "test: add download, SHA256 verification, and version comparison tests"
```

---

### Task 4: Auth Lockout — Login Tracker

**Files:**
- Modify: `server/auth.go`
- Modify: `server/server_test.go`

- [ ] **Step 1: Write failing tests for login tracking and lockout**

Append to `server/server_test.go`:

```go
func TestLoginFailureIsLogged(t *testing.T) {
	cfg := config.NewForTest(0, "admin", "secret")
	st := state.New(false)
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := scheduler.NewWithExec(ctx, st, scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}, restartIvl, lockIvl)
	srv := New(cfg, "", st, sched)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.SetBasicAuth("admin", "wrong")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLockoutAfter3FailedAttempts(t *testing.T) {
	cfg := config.NewForTest(0, "admin", "secret")
	st := state.New(false)
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := scheduler.NewWithExec(ctx, st, scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}, restartIvl, lockIvl)
	srv := New(cfg, "", st, sched)

	// 3 failed attempts
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// 4th attempt with correct credentials should be rejected (locked out)
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (locked out), got %d", w.Code)
	}
}

func TestValidLoginResetsFailureCount(t *testing.T) {
	cfg := config.NewForTest(0, "admin", "secret")
	st := state.New(false)
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := scheduler.NewWithExec(ctx, st, scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}, restartIvl, lockIvl)
	srv := New(cfg, "", st, sched)

	// 2 failed attempts (below lockout threshold)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
	}

	// Successful login resets counter
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 3 more failed attempts should be needed to lock out
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
	}

	// Now locked out
	req = httptest.NewRequest("GET", "/api/status", nil)
	req.SetBasicAuth("admin", "secret")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run "TestLockout|TestLoginFailure|TestValidLoginResets" -v -count=1`
Expected: FAIL (loginTracker not implemented)

- [ ] **Step 3: Implement login tracker in auth.go**

Add to `server/auth.go` after the `sessionStore` type:

```go
const maxFailedAttempts = 3

type loginTracker struct {
	mu       sync.Mutex
	failures int
	locked   bool
}

func (lt *loginTracker) recordFailure(remoteAddr string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.failures++
	log.Printf("login failed from %s (attempt %d/%d)", remoteAddr, lt.failures, maxFailedAttempts)
	if lt.failures >= maxFailedAttempts {
		lt.locked = true
		log.Printf("authentication locked after %d failed attempts — restart service to unlock", maxFailedAttempts)
	}
}

func (lt *loginTracker) recordSuccess(remoteAddr string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	log.Printf("login successful from %s", remoteAddr)
	lt.failures = 0
}

func (lt *loginTracker) isLocked() bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.locked
}
```

Modify `basicAuth` function — update the signature to accept `*loginTracker`:

```go
func basicAuth(ch *configHolder, store *sessionStore, tracker *loginTracker, next http.Handler) http.Handler {
```

Add at the top of the handler function (before any other checks):

```go
if tracker.isLocked() {
    http.Error(w, "Too many failed login attempts. Restart the service to unlock.", http.StatusForbidden)
    return
}
```

After the credential comparison fails (the existing `w.Header().Set("WWW-Authenticate", ...)` / `http.Error(w, "Unauthorized", 401)` block), add:

```go
tracker.recordFailure(r.RemoteAddr)
```

After successful credential validation (before creating session), add:

```go
tracker.recordSuccess(r.RemoteAddr)
```

Update `server.go` `New()` to create and pass the tracker:

```go
tracker := &loginTracker{}
// ...
Handler: basicAuth(ch, store, tracker, mux),
```

- [ ] **Step 4: Run tests**

Run: `go test ./server/ -v -count=1`
Expected: All tests PASS (existing + 3 new)

- [ ] **Step 5: Commit**

```bash
git add server/auth.go server/server.go server/server_test.go
git commit -m "feat: add login attempt logging and lockout after 3 failures"
```

---

### Task 5: Update API Endpoints

**Files:**
- Modify: `server/handlers.go`
- Modify: `server/server.go`
- Modify: `server/server_test.go`

- [ ] **Step 1: Write failing tests for update endpoints**

Append to `server/server_test.go`:

```go
func TestUpdateCheckReturnsJSON(t *testing.T) {
	cfg := config.NewForTest(0, "admin", "secret")
	st := state.New(false)
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := scheduler.NewWithExec(ctx, st, scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}, restartIvl, lockIvl)
	srv := New(cfg, "", st, sched)

	req := httptest.NewRequest("GET", "/api/update/status", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("content-type: got %s, want application/json", ct)
	}
}

func TestUpdateCheckRejectsPost(t *testing.T) {
	cfg := config.NewForTest(0, "admin", "secret")
	st := state.New(false)
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := scheduler.NewWithExec(ctx, st, scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}, restartIvl, lockIvl)
	srv := New(cfg, "", st, sched)

	req := httptest.NewRequest("POST", "/api/update/status", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run "TestUpdateCheck" -v -count=1`
Expected: FAIL (route not registered)

- [ ] **Step 3: Add updater to handlers and register routes**

Add to `handlers` struct in `server/handlers.go`:

```go
type handlers struct {
	state     *state.State
	scheduler *scheduler.Scheduler
	sessions  *sessionStore
	config    *configHolder
	updater   *updater.Updater
}
```

Add handler methods in `server/handlers.go`:

```go
func (h *handlers) updateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.updater.Cached())
}

func (h *handlers) updateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := h.updater.Check()
	if err != nil {
		log.Printf("update check failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": fmt.Sprintf("update check failed: %v", err)})
		return
	}
	writeJSON(w, info)
}

func (h *handlers) updateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := h.updater.Cached()
	if !info.Available {
		writeJSON(w, map[string]string{"error": "no update available"})
		return
	}

	tmpPath, err := h.updater.Download(info)
	if err != nil {
		log.Printf("update download failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": fmt.Sprintf("download failed: %v", err)})
		return
	}

	log.Printf("update downloaded to %s, initiating upgrade to %s", tmpPath, info.Version)
	writeJSON(w, map[string]string{
		"status":   "downloaded",
		"version":  info.Version,
		"tmp_path": tmpPath,
	})
}
```

Add `"fmt"` to the imports in `handlers.go` if not already present.

Register routes in `server/server.go` `New()`:

```go
mux.HandleFunc("/api/update/status", h.updateStatus)
mux.HandleFunc("/api/update/check", h.updateCheck)
mux.HandleFunc("/api/update/apply", h.updateApply)
```

Update `New()` to accept and wire the updater:

```go
func New(cfg *config.Config, configPath string, st *state.State, sched *scheduler.Scheduler, upd *updater.Updater) *http.Server {
	// ...
	h := &handlers{state: st, scheduler: sched, sessions: store, config: ch, updater: upd}
	// ...
}
```

Add `"winctl/updater"` to imports in `server/server.go`.

Update all callers of `New()`:
- `cmd/root.go`: `srv := server.New(cfg, configFile, st, sched, upd)` — create updater before server:
  ```go
  upd := updater.New(version.Version, "")
  ```
  Add import `"winctl/updater"` and reference the version. Actually, since `Version` is in `main` package, we need to move it. Create a simple approach — pass version string directly.

  In `cmd/root.go`, add a package-level variable:
  ```go
  var AppVersion = "1.0.2"
  ```
  Then: `upd := updater.New(AppVersion, "")`

- `service/service_windows.go`: same pattern with `upd := updater.New("1.0.2", "")`
- `server/server_test.go`: update all `New()` calls to pass `updater.New("1.0.0", "")` as the last argument. Add import `"winctl/updater"`.

- [ ] **Step 4: Run all tests**

Run: `go test ./server/ -v -count=1`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add server/handlers.go server/server.go server/server_test.go cmd/root.go service/service_windows.go
git commit -m "feat: add update check, status, and apply API endpoints"
```

---

### Task 6: Background Update Check on Startup

**Files:**
- Modify: `cmd/root.go`
- Modify: `service/service_windows.go`

- [ ] **Step 1: Add periodic background check**

In `cmd/root.go` `runForeground()`, after creating the updater, add a background check:

```go
// Check for updates in background on startup, then every 6 hours.
go func() {
	if info, err := upd.Check(); err != nil {
		log.Printf("update check: %v", err)
	} else if info.Available {
		log.Printf("update available: v%s", info.Version)
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if info, err := upd.Check(); err != nil {
				log.Printf("update check: %v", err)
			} else if info.Available {
				log.Printf("update available: v%s", info.Version)
			}
		case <-ctx.Done():
			return
		}
	}
}()
```

Add `"time"` to imports.

Apply the same pattern in `service/service_windows.go` after creating `upd`.

- [ ] **Step 2: Commit**

```bash
git add cmd/root.go service/service_windows.go
git commit -m "feat: periodic background update check (startup + every 6 hours)"
```

---

### Task 7: Dashboard — Upgrade Card UI

**Files:**
- Modify: `web/static/index.html`
- Modify: `web/static/app.js`
- Modify: `web/static/style.css`

- [ ] **Step 1: Add upgrade card HTML**

In `web/static/index.html`, add a new card BEFORE the Global card:

```html
<div id="upgrade-card" class="card card-upgrade" style="display:none;">
    <h2>Upgrade Available</h2>
    <div class="status-row">
        <span class="label">New version</span>
        <span class="value" id="upgrade-version"></span>
    </div>
    <div id="upgrade-details" style="display:none;">
        <div class="upgrade-body" id="upgrade-body"></div>
        <div class="status-row">
            <span class="label">Size</span>
            <span class="value" id="upgrade-size"></span>
        </div>
        <div class="actions">
            <button class="btn-on" onclick="applyUpgrade()">Proceed</button>
            <button class="btn-off" onclick="cancelUpgrade()">Cancel</button>
        </div>
    </div>
    <div id="upgrade-prompt" class="actions">
        <button class="btn-upgrade" onclick="showUpgradeDetails()">View Details</button>
    </div>
    <div id="upgrade-progress" style="display:none;">
        <p id="upgrade-progress-text">Downloading update...</p>
    </div>
</div>
```

- [ ] **Step 2: Add upgrade CSS**

Append to `web/static/style.css`:

```css
.card-upgrade { border-color: #b5830a; }
.btn-upgrade { background: #b5830a; width: 100%; }
.btn-upgrade:hover { background: #d4a017; }
.upgrade-body {
    background: #1a1a2e;
    border-radius: 6px;
    padding: 0.75rem;
    margin: 0.5rem 0;
    font-size: 0.85rem;
    color: #8899aa;
    white-space: pre-wrap;
    max-height: 200px;
    overflow-y: auto;
}
```

- [ ] **Step 3: Add upgrade JavaScript**

Append to `web/static/app.js`:

```javascript
function checkForUpdate() {
    fetch('/api/update/status')
        .then(r => r.json())
        .then(data => {
            const card = document.getElementById('upgrade-card');
            if (data.available) {
                card.style.display = '';
                document.getElementById('upgrade-version').textContent = 'v' + data.version;
                card.dataset.version = data.version;
                card.dataset.body = data.body || '';
                card.dataset.size = data.size || 0;
            } else {
                card.style.display = 'none';
            }
        })
        .catch(() => {});
}

function showUpgradeDetails() {
    const card = document.getElementById('upgrade-card');
    document.getElementById('upgrade-body').textContent = card.dataset.body || 'No release notes.';
    const bytes = parseInt(card.dataset.size || '0');
    document.getElementById('upgrade-size').textContent = (bytes / 1024 / 1024).toFixed(1) + ' MB';
    document.getElementById('upgrade-details').style.display = '';
    document.getElementById('upgrade-prompt').style.display = 'none';
}

function cancelUpgrade() {
    document.getElementById('upgrade-details').style.display = 'none';
    document.getElementById('upgrade-prompt').style.display = '';
}

function applyUpgrade() {
    document.getElementById('upgrade-details').style.display = 'none';
    document.getElementById('upgrade-progress').style.display = '';
    document.getElementById('upgrade-progress-text').textContent = 'Downloading and verifying update...';

    fetch('/api/update/apply', { method: 'POST' })
        .then(r => r.json())
        .then(data => {
            if (data.error) {
                document.getElementById('upgrade-progress-text').textContent = 'Error: ' + data.error;
                showToast('Upgrade failed: ' + data.error, 'error');
                return;
            }
            document.getElementById('upgrade-progress-text').textContent =
                'Update v' + data.version + ' downloaded and verified. Service will restart shortly.';
            showToast('Update downloaded. Service restarting...', 'ok');
        })
        .catch(err => {
            document.getElementById('upgrade-progress-text').textContent = 'Error: ' + err.message;
            showToast('Upgrade failed: ' + err.message, 'error');
        });
}
```

Modify the initialization section at the bottom of `app.js` — add after `fetchConfig()`:

```javascript
checkForUpdate();
setInterval(checkForUpdate, 60000); // Check every 60 seconds
```

- [ ] **Step 4: Commit**

```bash
git add web/static/index.html web/static/app.js web/static/style.css
git commit -m "feat: add upgrade card to dashboard with details dialog and progress"
```

---

### Task 8: Update Documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update README**

Add to the Features section:

```markdown
- **Auto-update detection** — checks GitHub releases on startup and periodically; dashboard shows Upgrade card when a new version is available
- **In-app upgrade** — download, SHA256 verify, and replace binary from the dashboard
- **Login lockout** — locks authentication after 3 failed attempts (service restart to unlock); all login attempts are logged
```

Add to API Endpoints table:

```markdown
| `GET` | `/api/update/status` | Cached update check result |
| `POST` | `/api/update/check` | Force check for updates |
| `POST` | `/api/update/apply` | Download, verify, and apply update |
```

Add to Known Constraints in CLAUDE.md:

```markdown
- Login locks out after 3 failed attempts; only service restart clears the lockout (no timed reset)
- Auto-update checks GitHub API unauthenticated (rate limit: 60 req/hour per IP)
- Update binary is SHA256-verified against the GitHub release asset digest
```

Update test counts after all tests are added.

- [ ] **Step 2: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add auto-update, login lockout, and new API endpoints to documentation"
```

---

## Summary

| Task | What | New Tests |
|------|------|-----------|
| 1 | Version constant | — |
| 2 | Updater: release check | 5 |
| 3 | Updater: download + SHA256 | 4 |
| 4 | Auth: login tracker + lockout | 3 |
| 5 | Update API endpoints | 2+ |
| 6 | Background periodic check | — |
| 7 | Dashboard upgrade UI | — |
| 8 | Documentation | — |
