package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
	"winctl/config"
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

func (h *handlers) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req scheduler.CancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid JSON"})
		return
	}
	if !req.RestartOnce && !req.RestartSchedule && !req.LockOnce && !req.LockSchedule {
		writeJSON(w, map[string]string{"status": "nothing to cancel"})
		return
	}
	h.scheduler.Cancel(req)
	slog.Info("activities cancelled", "restart_once", req.RestartOnce, "restart_schedule", req.RestartSchedule, "lock_once", req.LockOnce, "lock_schedule", req.LockSchedule)
	writeJSON(w, map[string]string{"status": "cancelled"})
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

func (h *handlers) configUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionTimeoutMinutes int    `json:"session_timeout_minutes"`
		RestartMinMinutes     int    `json:"restart_min_minutes"`
		RestartMaxMinutes     int    `json:"restart_max_minutes"`
		LockMinMinutes        int    `json:"lock_min_minutes"`
		LockMaxMinutes        int    `json:"lock_max_minutes"`
		UpdateCheckMinutes    int    `json:"update_check_minutes"`
		LogLevel              string `json:"log_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid JSON"})
		return
	}

	var errors []string
	if req.SessionTimeoutMinutes < 1 {
		errors = append(errors, "session timeout must be at least 1 minute")
	}
	if req.RestartMinMinutes < 1 || req.RestartMinMinutes > 1440 {
		errors = append(errors, "restart min must be 1-1440 minutes")
	}
	if req.RestartMaxMinutes < req.RestartMinMinutes || req.RestartMaxMinutes > 1440 {
		errors = append(errors, "restart max must be >= min and <= 1440")
	}
	if req.LockMinMinutes < 1 || req.LockMinMinutes > 1440 {
		errors = append(errors, "lock min must be 1-1440 minutes")
	}
	if req.LockMaxMinutes < req.LockMinMinutes || req.LockMaxMinutes > 1440 {
		errors = append(errors, "lock max must be >= min and <= 1440")
	}
	if req.UpdateCheckMinutes < 1 {
		errors = append(errors, "update check must be at least 1 minute")
	}
	switch req.LogLevel {
	case "debug", "info", "error":
	default:
		errors = append(errors, "log level must be debug, info, or error")
	}
	if len(errors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "validation failed", "details": errors})
		return
	}

	h.config.mu.Lock()
	h.config.cfg.SessionTimeoutMinutes = req.SessionTimeoutMinutes
	h.config.cfg.RestartMinMinutes = req.RestartMinMinutes
	h.config.cfg.RestartMaxMinutes = req.RestartMaxMinutes
	h.config.cfg.LockMinMinutes = req.LockMinMinutes
	h.config.cfg.LockMaxMinutes = req.LockMaxMinutes
	h.config.cfg.UpdateCheckMinutes = req.UpdateCheckMinutes
	h.config.cfg.LogLevel = req.LogLevel
	err := config.Save(h.config.cfg, h.config.path)
	h.config.mu.Unlock()
	if err != nil {
		slog.Error("config save failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "failed to save config"})
		return
	}

	cfg, err := h.config.reload()
	if err != nil {
		slog.Error("config reload after save failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "saved but reload failed"})
		return
	}
	h.scheduler.UpdateIntervals(
		scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes},
		scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes},
	)
	h.sessions.updateTimeout(cfg.SessionTimeoutMinutes)
	logging.Setup(cfg.LogLevel)
	slog.Info("configuration updated",
		"session_timeout", cfg.SessionTimeoutMinutes,
		"restart_min", cfg.RestartMinMinutes,
		"restart_max", cfg.RestartMaxMinutes,
		"lock_min", cfg.LockMinMinutes,
		"lock_max", cfg.LockMaxMinutes,
		"update_check", cfg.UpdateCheckMinutes,
		"log_level", cfg.LogLevel,
	)
	writeJSON(w, map[string]string{"status": "configuration saved"})
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
	slog.Info("configuration reloaded",
		"session_timeout", cfg.SessionTimeoutMinutes,
		"restart_min", cfg.RestartMinMinutes,
		"restart_max", cfg.RestartMaxMinutes,
		"lock_min", cfg.LockMinMinutes,
		"lock_max", cfg.LockMaxMinutes,
		"update_check", cfg.UpdateCheckMinutes,
		"log_level", cfg.LogLevel,
	)
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid level: must be debug, info, or error"})
		return
	}
	if err := h.config.setLogLevel(level); err != nil {
		slog.Error("failed to save log level", "error", err)
		w.Header().Set("Content-Type", "application/json")
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
		w.Header().Set("Content-Type", "application/json")
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
	// Check upgrade not already in progress before downloading.
	if !upgradeInProgress.CompareAndSwap(false, true) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"error": "upgrade already in progress"})
		return
	}

	// Extend deadline — download may take longer than the default 10s WriteTimeout.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		slog.Warn("upgrade: could not extend write deadline, large downloads may timeout", "error", err)
	}

	// Pre-flight: verify we can reach the service manager and find the service.
	if errMsg := preflightUpgradeCheck(); errMsg != "" {
		upgradeInProgress.Store(false)
		slog.Error("upgrade preflight failed", "error", errMsg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": errMsg})
		return
	}

	info := h.updater.Cached()
	if !info.Available {
		upgradeInProgress.Store(false)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"status": "no update available"})
		return
	}
	tmpPath, err := h.updater.Download(info)
	if err != nil {
		upgradeInProgress.Store(false)
		slog.Error("update download failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "download failed: " + err.Error()})
		return
	}
	slog.Info("update downloaded", "path", tmpPath, "version", info.Version)
	writeJSON(w, map[string]string{
		"status":  "upgrading",
		"version": info.Version,
		"message": "Service will restart in approximately 5 seconds",
	})

	// Flush response before triggering upgrade (service will be killed).
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Trigger in-place upgrade asynchronously (Windows: stop → replace → restart service).
	go applyUpgrade(tmpPath)
}
