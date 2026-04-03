package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
	"winctl/config"
)

const sessionCookieName = "winctl_session"
const loggedOutCookieName = "winctl_logged_out"

type configHolder struct {
	mu   sync.RWMutex
	cfg  *config.Config
	path string
}

func newConfigHolder(cfg *config.Config, path string) *configHolder {
	return &configHolder{cfg: cfg, path: path}
}

func (ch *configHolder) get() *config.Config {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.cfg
}

func (ch *configHolder) reload() (*config.Config, error) {
	cfg, err := config.Load(ch.path)
	if err != nil {
		return nil, err
	}
	ch.mu.Lock()
	ch.cfg = cfg
	ch.mu.Unlock()
	return cfg, nil
}

func (ch *configHolder) setLogLevel(level string) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	prev := ch.cfg.LogLevel
	ch.cfg.LogLevel = level
	if err := config.Save(ch.cfg, ch.path); err != nil {
		ch.cfg.LogLevel = prev
		return err
	}
	return nil
}

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
	slog.Error("login failed", "remote_addr", remoteAddr, "attempt", lt.failures, "max_attempts", maxFailedAttempts)
	if lt.failures >= maxFailedAttempts {
		lt.locked = true
		slog.Error("authentication locked", "failed_attempts", maxFailedAttempts)
	}
}

func (lt *loginTracker) recordSuccess(remoteAddr string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	slog.Info("login successful", "remote_addr", remoteAddr)
	lt.failures = 0
}

func (lt *loginTracker) isLocked() bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.locked
}

type session struct {
	expiresAt time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	timeout  time.Duration
}

func newSessionStore(timeoutMinutes int) *sessionStore {
	return &sessionStore{
		sessions: make(map[string]session),
		timeout:  time.Duration(timeoutMinutes) * time.Minute,
	}
}

func (s *sessionStore) updateTimeout(minutes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = time.Duration(minutes) * time.Minute
}

func (s *sessionStore) create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Lazy sweep of expired sessions to prevent unbounded growth.
	now := time.Now()
	for k, v := range s.sessions {
		if now.After(v.expiresAt) {
			delete(s.sessions, k)
		}
	}

	s.sessions[token] = session{expiresAt: now.Add(s.timeout)}
	return token, nil
}

func (s *sessionStore) valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(sess.expiresAt) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func (s *sessionStore) remove(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func basicAuth(ch *configHolder, store *sessionStore, tracker *loginTracker, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tracker.isLocked() {
			http.Error(w, "Too many failed login attempts. Restart the service to unlock.", http.StatusForbidden)
			return
		}

		// Check logged-out cookie first — must take priority over valid sessions
		// so the browser forgets the cached Authorization header after logout.
		if _, err := r.Cookie(loggedOutCookieName); err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:   loggedOutCookieName,
				MaxAge: -1,
				Path:   "/",
			})
			w.Header().Set("WWW-Authenticate", `Basic realm="winctl"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check existing session cookie.
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			if store.valid(cookie.Value) {
				next.ServeHTTP(w, r)
				return
			}
			// Expired — clear the cookie.
			http.SetCookie(w, &http.Cookie{
				Name:   sessionCookieName,
				MaxAge: -1,
				Path:   "/",
			})
		}

		// No valid session — require Basic Auth.
		cfg := ch.get()
		user, pass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username))
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.Password()))
		if !ok || (userOK&passOK) != 1 {
			tracker.recordFailure(r.RemoteAddr)
			w.Header().Set("WWW-Authenticate", `Basic realm="winctl"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tracker.recordSuccess(r.RemoteAddr)

		// Credentials valid — create session.
		token, err := store.create()
		if err != nil {
			slog.Error("session token generation failed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})

		next.ServeHTTP(w, r)
	})
}
