package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"winctl/logging"
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
	version   string
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
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
	s := h.state.Status()
	upd := h.updater.Cached()
	writeJSON(w, map[string]any{
		"version":                 h.version,
		"dry_run":                 s.DryRun,
		"restart_schedule_active": s.RestartScheduleActive,
		"restart_next_at":         s.RestartNextAt,
		"restart_pending_once":    s.RestartPendingOnce,
		"restart_once_at":         s.RestartOnceAt,
		"lock_schedule_active":    s.LockScheduleActive,
		"lock_next_at":            s.LockNextAt,
		"lock_pending_once":       s.LockPendingOnce,
		"lock_once_at":            s.LockOnceAt,
		"update_available":        upd.Available,
		"update_version":          upd.Version,
	})
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
		"update_check_minutes":    cfg.UpdateCheckMinutes,
		"log_level":               cfg.LogLevel,
	})
}

func (h *handlers) configReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.config.reload()
	if err != nil {
		slog.Error("config reload failed", "error", err)
		http.Error(w, "config reload failed", http.StatusInternalServerError)
		return
	}
	h.scheduler.UpdateIntervals(
		scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes},
		scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes},
	)
	h.sessions.updateTimeout(cfg.SessionTimeoutMinutes)
	logging.Setup(cfg.LogLevel)
	slog.Info("configuration reloaded")
	writeJSON(w, map[string]string{"status": "configuration reloaded"})
}

func (h *handlers) configSetLogLevel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	level := r.URL.Query().Get("level")
	switch level {
	case "debug", "info", "error":
		// valid
	default:
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid level: must be debug, info, or error"})
		return
	}
	if err := h.config.setLogLevel(level); err != nil {
		slog.Error("failed to save log level", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "failed to save config"})
		return
	}
	logging.Setup(level)
	slog.Info("log level changed", "level", level)
	writeJSON(w, map[string]string{"status": "log level set to " + level})
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
		slog.Error("update check failed", "error", err)
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
		slog.Error("update download failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "download failed: " + err.Error()})
		return
	}
	slog.Info("update downloaded", "path", tmpPath, "version", info.Version)
	writeJSON(w, map[string]string{"status": "downloaded", "version": info.Version})
}
