package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
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

func basicAuth(ch *configHolder, store *sessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("WWW-Authenticate", `Basic realm="winctl"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Credentials valid — create session.
		token, err := store.create()
		if err != nil {
			log.Printf("error: session token generation failed: %v", err)
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
