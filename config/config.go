package config

import (
	"encoding/base64"
	"encoding/json"
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
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func defaults() *Config {
	plain := "changeme"
	return &Config{
		Port:              8443,
		Username:          "admin",
		PasswordB64:           base64.StdEncoding.EncodeToString([]byte(plain)),
		SessionTimeoutMinutes: 30,
		RestartMinMinutes:     5,
		RestartMaxMinutes: 15,
		LockMinMinutes:    5,
		LockMaxMinutes:    15,
		password:          plain,
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist — create it with defaults.
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
		log.Printf("warning: invalid config, using defaults: %v", err)
		return defaults(), nil
	}

	// Decode the base64 password.
	decoded, err := base64.StdEncoding.DecodeString(cfg.PasswordB64)
	if err != nil {
		log.Printf("warning: invalid base64 password in config, using defaults")
		return defaults(), nil
	}
	cfg.password = string(decoded)

	return cfg, nil
}

func save(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
