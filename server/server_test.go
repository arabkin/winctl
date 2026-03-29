package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"winctl/config"
	"winctl/scheduler"
	"winctl/state"
)

func setupTestServer(t *testing.T) (*http.Server, *state.State, *scheduler.Scheduler) {
	t.Helper()
	cfg := config.NewForTest(8443, "admin", "testpass")
	st := state.New()
	ctx := context.Background()
	noopExec := scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	sched := scheduler.NewWithExec(ctx, st, noopExec, restartIvl, lockIvl)
	srv := New(cfg, st, sched)
	return srv, st, sched
}

func authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:testpass"))
}

func doRequest(handler http.Handler, method, path, auth string) *httptest.ResponseRecorder {
	return doRequestWithCookie(handler, method, path, auth, nil)
}

func doRequestWithCookie(handler http.Handler, method, path, auth string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func extractSessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == "winctl_session" {
			return c
		}
	}
	return nil
}

// --- Auth tests ---

func TestAuthRejectsNoCredentials(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/status", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestAuthRejectsWrongPassword(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	badAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
	w := doRequest(srv.Handler, "GET", "/api/status", badAuth)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthRejectsWrongUsername(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	badAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("nobody:testpass"))
	w := doRequest(srv.Handler, "GET", "/api/status", badAuth)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthAcceptsCorrectCredentials(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/status", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Session tests ---

func TestAuthSetsSessionCookie(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/status", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cookie := extractSessionCookie(w)
	if cookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if cookie.Value == "" {
		t.Error("session cookie should not be empty")
	}
}

func TestSessionCookieAllowsAccessWithoutBasicAuth(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// First request with Basic Auth to get session cookie.
	w := doRequest(srv.Handler, "GET", "/api/status", authHeader())
	cookie := extractSessionCookie(w)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}

	// Second request with only the cookie, no Basic Auth.
	w2 := doRequestWithCookie(srv.Handler, "GET", "/api/status", "", []*http.Cookie{cookie})
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with session cookie, got %d", w2.Code)
	}
}

func TestExpiredSessionRequiresReauth(t *testing.T) {
	// Use 0-minute timeout so sessions expire immediately.
	cfg := config.NewForTestWithTimeout(8443, "admin", "testpass", 0)
	st := state.New()
	ctx := context.Background()
	noopExec := scheduler.ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}
	restartIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	lockIvl := scheduler.IntervalRange{MinMinutes: 5, MaxMinutes: 15}
	sched := scheduler.NewWithExec(ctx, st, noopExec, restartIvl, lockIvl)
	srv := New(cfg, st, sched)

	// Get a session cookie.
	w := doRequest(srv.Handler, "GET", "/api/status", authHeader())
	cookie := extractSessionCookie(w)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}

	// Session is already expired (0-minute timeout). Cookie-only request should fail.
	time.Sleep(1 * time.Millisecond)
	w2 := doRequestWithCookie(srv.Handler, "GET", "/api/status", "", []*http.Cookie{cookie})
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired session, got %d", w2.Code)
	}
}

func TestInvalidSessionCookieRequiresAuth(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	fakeCookie := &http.Cookie{Name: "winctl_session", Value: "invalid-token"}
	w := doRequestWithCookie(srv.Handler, "GET", "/api/status", "", []*http.Cookie{fakeCookie})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid cookie, got %d", w.Code)
	}
}

// --- Status endpoint ---

func TestStatusReturnsJSON(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/status", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %s, want application/json", ct)
	}
	var status state.StatusDTO
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode status JSON: %v", err)
	}
	if status.RestartScheduleActive {
		t.Error("restart schedule should be inactive initially")
	}
}

func TestStatusRejectsPost(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "POST", "/api/status", authHeader())
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- Restart Once ---

func TestRestartOnce(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "POST", "/api/restart/once", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Give goroutine a moment to update state
	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if !status.RestartPendingOnce {
		t.Error("restart should be pending after POST /api/restart/once")
	}
	if status.RestartOnceAt == nil {
		t.Error("restart_once_at should be set")
	}
}

func TestRestartOnceRejectsGet(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/restart/once", authHeader())
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- Restart Schedule ---

func TestRestartScheduleEnable(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "POST", "/api/restart/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if !status.RestartScheduleActive {
		t.Error("restart schedule should be active after POST")
	}
}

func TestRestartScheduleDisable(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	// Enable first
	doRequest(srv.Handler, "POST", "/api/restart/schedule", authHeader())
	time.Sleep(10 * time.Millisecond)

	// Disable
	w := doRequest(srv.Handler, "DELETE", "/api/restart/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if status.RestartScheduleActive {
		t.Error("restart schedule should be inactive after DELETE")
	}
}

func TestRestartScheduleRejectsGet(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/restart/schedule", authHeader())
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- Lock Once ---

func TestLockOnce(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "POST", "/api/lock/once", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if !status.LockPendingOnce {
		t.Error("lock should be pending after POST /api/lock/once")
	}
}

// --- Lock Schedule ---

func TestLockScheduleEnable(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "POST", "/api/lock/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if !status.LockScheduleActive {
		t.Error("lock schedule should be active after POST")
	}
}

func TestLockScheduleDisable(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	doRequest(srv.Handler, "POST", "/api/lock/schedule", authHeader())
	time.Sleep(10 * time.Millisecond)

	w := doRequest(srv.Handler, "DELETE", "/api/lock/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if status.LockScheduleActive {
		t.Error("lock schedule should be inactive after DELETE")
	}
}

// --- Reset ---

func TestReset(t *testing.T) {
	srv, st, _ := setupTestServer(t)

	// Enable everything
	doRequest(srv.Handler, "POST", "/api/restart/schedule", authHeader())
	doRequest(srv.Handler, "POST", "/api/lock/schedule", authHeader())
	doRequest(srv.Handler, "POST", "/api/restart/once", authHeader())
	doRequest(srv.Handler, "POST", "/api/lock/once", authHeader())
	time.Sleep(10 * time.Millisecond)

	// Reset
	w := doRequest(srv.Handler, "POST", "/api/reset", authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	status := st.Status()
	if status.RestartScheduleActive || status.LockScheduleActive {
		t.Error("schedules should be inactive after reset")
	}
	if status.RestartPendingOnce || status.LockPendingOnce {
		t.Error("pending once should be false after reset")
	}
}

func TestResetRejectsGet(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := doRequest(srv.Handler, "GET", "/api/reset", authHeader())
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- Static files ---

func TestStaticFilesServed(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Root path serves index.html
	w := doRequest(srv.Handler, "GET", "/", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", w.Code)
	}
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty body for /")
	}

	// CSS file
	w = doRequest(srv.Handler, "GET", "/style.css", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /style.css, got %d", w.Code)
	}

	// JS file
	w = doRequest(srv.Handler, "GET", "/app.js", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /app.js, got %d", w.Code)
	}
}

// --- JSON response shape ---

func TestAPIResponsesAreJSON(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/restart/once"},
		{"POST", "/api/restart/schedule"},
		{"POST", "/api/lock/once"},
		{"POST", "/api/lock/schedule"},
		{"POST", "/api/reset"},
	}

	for _, ep := range endpoints {
		w := doRequest(srv.Handler, ep.method, ep.path, authHeader())
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %s, want application/json", ep.method, ep.path, ct)
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("%s %s: invalid JSON response: %v", ep.method, ep.path, err)
		}
		if _, ok := resp["status"]; !ok {
			t.Errorf("%s %s: response missing 'status' field", ep.method, ep.path)
		}
	}
}

// --- Idempotency ---

func TestScheduleEnableIdempotent(t *testing.T) {
	srv, st, _ := setupTestServer(t)

	// Enable restart schedule twice
	doRequest(srv.Handler, "POST", "/api/restart/schedule", authHeader())
	doRequest(srv.Handler, "POST", "/api/restart/schedule", authHeader())
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.RestartScheduleActive {
		t.Error("restart schedule should still be active")
	}

	// Disable should still work
	doRequest(srv.Handler, "DELETE", "/api/restart/schedule", authHeader())
	time.Sleep(10 * time.Millisecond)

	status = st.Status()
	if status.RestartScheduleActive {
		t.Error("restart schedule should be inactive after single disable")
	}
}

func TestDisableWhenAlreadyDisabledIsNoop(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Disable without enabling — should not panic
	w := doRequest(srv.Handler, "DELETE", "/api/restart/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for noop disable, got %d", w.Code)
	}

	w = doRequest(srv.Handler, "DELETE", "/api/lock/schedule", authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for noop disable, got %d", w.Code)
	}
}
