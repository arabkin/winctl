package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds the runtime configuration.
// PasswordB64 is the base64-encoded password stored on disk.
type Config struct {
	Port        int    `json:"port"`
	Username    string `json:"username"`
	PasswordB64 string `json:"password"`

	SessionTimeoutMinutes int `json:"session_timeout_minutes"`

	RestartMinMinutes  int    `json:"restart_min_minutes"`
	RestartMaxMinutes  int    `json:"restart_max_minutes"`
	LockMinMinutes     int    `json:"lock_min_minutes"`
	LockMaxMinutes     int    `json:"lock_max_minutes"`
	UpdateCheckMinutes int    `json:"update_check_minutes"`
	LogLevel           string `json:"log_level"`

	// Decoded password, not serialized to JSON.
	password string
}

func (c *Config) Password() string {
	return c.password
}

func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		slog.Warn("could not determine executable path, using relative config.json", "error", err)
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func defaults() *Config {
	plain := "changeme"
	return &Config{
		Port:                  8443,
		Username:              "admin",
		PasswordB64:           base64.StdEncoding.EncodeToString([]byte(plain)),
		SessionTimeoutMinutes: 30,
		RestartMinMinutes:     5,
		RestartMaxMinutes:     15,
		LockMinMinutes:        5,
		LockMaxMinutes:        15,
		UpdateCheckMinutes:    360,
		LogLevel:              "info",
		password:              plain,
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read config %s: %w", path, err)
		}
		// File doesn't exist — first run, create it with defaults.
		cfg := defaults()
		if writeErr := Save(cfg, path); writeErr != nil {
			slog.Warn("could not write default config", "path", path, "error", writeErr)
		} else {
			slog.Info("created default config", "path", path)
		}
		return cfg, nil
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config file %s: %w", path, err)
	}

	// Decode the base64 password.
	decoded, err := base64.StdEncoding.DecodeString(cfg.PasswordB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 password in config %s: %w", path, err)
	}
	cfg.password = string(decoded)

	// Validate session timeout.
	if cfg.SessionTimeoutMinutes <= 0 {
		slog.Warn("invalid session_timeout_minutes, defaulting to 30", "value", cfg.SessionTimeoutMinutes)
		cfg.SessionTimeoutMinutes = 30
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d in config %s: must be 1-65535", cfg.Port, path)
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("username must not be empty in config %s", path)
	}
	if cfg.password == "" {
		return nil, fmt.Errorf("password must not be empty in config %s", path)
	}
	const maxIntervalMinutes = 1440 // 24 hours
	if cfg.RestartMinMinutes < 1 || cfg.RestartMinMinutes > maxIntervalMinutes {
		return nil, fmt.Errorf("restart_min_minutes must be 1-%d in config %s", maxIntervalMinutes, path)
	}
	if cfg.RestartMaxMinutes < cfg.RestartMinMinutes || cfg.RestartMaxMinutes > maxIntervalMinutes {
		return nil, fmt.Errorf("restart_max_minutes must be %d-%d in config %s", cfg.RestartMinMinutes, maxIntervalMinutes, path)
	}
	if cfg.LockMinMinutes < 1 || cfg.LockMinMinutes > maxIntervalMinutes {
		return nil, fmt.Errorf("lock_min_minutes must be 1-%d in config %s", maxIntervalMinutes, path)
	}
	if cfg.LockMaxMinutes < cfg.LockMinMinutes || cfg.LockMaxMinutes > maxIntervalMinutes {
		return nil, fmt.Errorf("lock_max_minutes must be %d-%d in config %s", cfg.LockMinMinutes, maxIntervalMinutes, path)
	}
	if cfg.UpdateCheckMinutes <= 0 {
		slog.Warn("invalid update_check_minutes, defaulting to 360", "value", cfg.UpdateCheckMinutes)
		cfg.UpdateCheckMinutes = 360
	}
	switch cfg.LogLevel {
	case "debug", "info", "error":
		// valid
	default:
		if cfg.LogLevel != "" {
			slog.Warn("invalid log_level, defaulting to info", "value", cfg.LogLevel)
		}
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
