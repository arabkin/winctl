package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

	RestartMinMinutes int `json:"restart_min_minutes"`
	RestartMaxMinutes int `json:"restart_max_minutes"`
	LockMinMinutes    int `json:"lock_min_minutes"`
	LockMaxMinutes    int `json:"lock_max_minutes"`

	// Decoded password, not serialized to JSON.
	password string
}

func (c *Config) Password() string {
	return c.password
}

func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("warning: could not determine executable path, using relative config.json: %v", err)
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
		if writeErr := save(cfg, path); writeErr != nil {
			log.Printf("warning: could not write default config to %s: %v", path, writeErr)
		} else {
			log.Printf("created default config at %s", path)
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
		log.Printf("warning: session_timeout_minutes is %d, defaulting to 30", cfg.SessionTimeoutMinutes)
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
	if cfg.RestartMinMinutes < 1 {
		return nil, fmt.Errorf("restart_min_minutes must be >= 1 in config %s", path)
	}
	if cfg.RestartMaxMinutes < cfg.RestartMinMinutes {
		return nil, fmt.Errorf("restart_max_minutes (%d) must be >= restart_min_minutes (%d) in config %s", cfg.RestartMaxMinutes, cfg.RestartMinMinutes, path)
	}
	if cfg.LockMinMinutes < 1 {
		return nil, fmt.Errorf("lock_min_minutes must be >= 1 in config %s", path)
	}
	if cfg.LockMaxMinutes < cfg.LockMinMinutes {
		return nil, fmt.Errorf("lock_max_minutes (%d) must be >= lock_min_minutes (%d) in config %s", cfg.LockMaxMinutes, cfg.LockMinMinutes, path)
	}

	return cfg, nil
}

func save(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
