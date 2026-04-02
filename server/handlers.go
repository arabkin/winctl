package server

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"winctl/scheduler"
	"winctl/state"
	"winctl/updater"
)

type handlers struct {
	state     *state.State
	scheduler *scheduler.Scheduler
	sessions  *sessionStore
	config    *configHolder
	updater   *updater.Updater
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Printf("error: failed to encode JSON response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buf.Bytes())
}

func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.state.Status())
}

func (h *handlers) restartOnce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.scheduler.RestartOnce()
	writeJSON(w, map[string]string{"status": "restart scheduled in 60s"})
}

func (h *handlers) restartSchedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.scheduler.StartRestartSchedule()
		writeJSON(w, map[string]string{"status": "restart schedule enabled"})
	case http.MethodDelete:
		h.scheduler.StopRestartSchedule()
		writeJSON(w, map[string]string{"status": "restart schedule disabled"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) lockOnce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.scheduler.LockOnce()
	writeJSON(w, map[string]string{"status": "lock scheduled in 60s"})
}

func (h *handlers) lockSchedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.scheduler.StartLockSchedule()
		writeJSON(w, map[string]string{"status": "lock schedule enabled"})
	case http.MethodDelete:
		h.scheduler.StopLockSchedule()
		writeJSON(w, map[string]string{"status": "lock schedule disabled"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) reset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.scheduler.ResetAll()
	writeJSON(w, map[string]string{"status": "all settings reset"})
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		h.sessions.remove(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookieName,
		MaxAge: -1,
		Path:   "/",
	})
	http.SetCookie(w, &http.Cookie{
		Name:     loggedOutCookieName,
		Value:    "1",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   10,
	})
	writeJSON(w, map[string]string{"status": "logged out"})
}

func (h *handlers) configGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := h.config.get()
	writeJSON(w, map[string]any{
		"port":                    cfg.Port,
		"username":                cfg.Username,
		"session_timeout_minutes": cfg.SessionTimeoutMinutes,
		"restart_min_minutes":     cfg.RestartMinMinutes,
		"restart_max_minutes":     cfg.RestartMaxMinutes,
		"lock_min_minutes":        cfg.LockMinMinutes,
		"lock_max_minutes":        cfg.LockMaxMinutes,
	})
}

func (h *handlers) configReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.config.reload()
	if err != nil {
		log.Printf("config reload failed: %v", err)
		http.Error(w, "config reload failed", http.StatusInternalServerError)
		return
	}
	h.scheduler.UpdateIntervals(
		scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes},
		scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes},
	)
	h.sessions.updateTimeout(cfg.SessionTimeoutMinutes)
	log.Println("configuration reloaded successfully")
	writeJSON(w, map[string]string{"status": "configuration reloaded"})
}

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
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "update check failed: " + err.Error()})
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
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"status": "no update available"})
		return
	}
	tmpPath, err := h.updater.Download(info)
	if err != nil {
		log.Printf("update download failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "download failed: " + err.Error()})
		return
	}
	log.Printf("update downloaded to %s, initiating upgrade to %s", tmpPath, info.Version)
	writeJSON(w, map[string]string{"status": "downloaded", "version": info.Version})
}
