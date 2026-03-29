package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.Port != 8443 {
		t.Errorf("expected port 8443, got %d", cfg.Port)
	}
	if cfg.Username != "admin" {
		t.Errorf("expected username admin, got %s", cfg.Username)
	}
	if cfg.Password() != "changeme" {
		t.Errorf("expected password changeme, got %s", cfg.Password())
	}
	decoded, err := base64.StdEncoding.DecodeString(cfg.PasswordB64)
	if err != nil {
		t.Fatalf("PasswordB64 is not valid base64: %v", err)
	}
	if string(decoded) != "changeme" {
		t.Errorf("PasswordB64 decodes to %q, want changeme", decoded)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &Config{
		Port:        9090,
		Username:    "testuser",
		PasswordB64: base64.StdEncoding.EncodeToString([]byte("secret123")),
		password:    "secret123",
	}

	if err := save(original, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if loaded.Port != 9090 {
		t.Errorf("port: got %d, want 9090", loaded.Port)
	}
	if loaded.Username != "testuser" {
		t.Errorf("username: got %s, want testuser", loaded.Username)
	}

	decoded, err := base64.StdEncoding.DecodeString(loaded.PasswordB64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != "secret123" {
		t.Errorf("password decoded to %q, want secret123", decoded)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := save(defaults(), path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions: got %o, want 0600", perm)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("{invalid json"), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON config")
	}
}

func TestLoadInvalidBase64Password(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"port": 8443, "username": "admin", "password": "not-valid-base64!!!", "session_timeout_minutes": 30}`
	os.WriteFile(path, []byte(data), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid base64 password")
	}
}

func TestLoadCreatesDefaultOnMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Port != 8443 {
		t.Errorf("expected default port 8443, got %d", cfg.Port)
	}
	if cfg.SessionTimeoutMinutes != 30 {
		t.Errorf("expected default session timeout 30, got %d", cfg.SessionTimeoutMinutes)
	}
	// File should have been created.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected default config file to be created: %v", err)
	}
}

func TestLoadValidatesSessionTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"port": 8443, "username": "admin", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": 0, "restart_min_minutes": 5, "restart_max_minutes": 15, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTimeoutMinutes != 30 {
		t.Errorf("expected session timeout to default to 30 for zero value, got %d", cfg.SessionTimeoutMinutes)
	}
}

func TestLoadValidatesNegativeSessionTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"port": 8443, "username": "admin", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": -5, "restart_min_minutes": 5, "restart_max_minutes": 15, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTimeoutMinutes != 30 {
		t.Errorf("expected session timeout to default to 30 for negative value, got %d", cfg.SessionTimeoutMinutes)
	}
}

func TestLoadValidatesPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"port": 0, "username": "admin", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": 30, "restart_min_minutes": 5, "restart_max_minutes": 15, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for port 0")
	}
}

func TestLoadValidatesEmptyUsername(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"port": 8443, "username": "", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": 30, "restart_min_minutes": 5, "restart_max_minutes": 15, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestLoadValidatesIntervalRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"port": 8443, "username": "admin", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": 30, "restart_min_minutes": 0, "restart_max_minutes": 15, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for restart_min_minutes = 0")
	}
}

func TestLoadValidatesMaxLessThanMin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"port": 8443, "username": "admin", "password": "Y2hhbmdlbWU=", "session_timeout_minutes": 30, "restart_min_minutes": 15, "restart_max_minutes": 5, "lock_min_minutes": 5, "lock_max_minutes": 15}`
	os.WriteFile(path, []byte(data), 0600)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for restart_max_minutes < restart_min_minutes")
	}
}
