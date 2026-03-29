package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
	"winctl/config"
)

const sessionCookieName = "winctl_session"

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

func (s *sessionStore) create() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = session{expiresAt: time.Now().Add(s.timeout)}
	return token
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

func basicAuth(cfg *config.Config, next http.Handler) http.Handler {
	store := newSessionStore(cfg.SessionTimeoutMinutes)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.Password())) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="winctl"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Credentials valid — create session.
		token := store.create()
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
