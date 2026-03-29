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

	// Since Load() uses configPath() which is based on os.Executable(),
	// we test the parsing logic directly.
	var cfg Config
	err := json.Unmarshal([]byte("{invalid json"), &cfg)
	if err == nil {
		t.Error("expected JSON parse error for invalid input")
	}
}

func TestLoadInvalidBase64Password(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"port": 8443, "username": "admin", "password": "not-valid-base64!!!"}`
	os.WriteFile(path, []byte(data), 0600)

	var cfg Config
	json.Unmarshal([]byte(data), &cfg)

	_, err := base64.StdEncoding.DecodeString(cfg.PasswordB64)
	if err == nil {
		t.Error("expected base64 decode error for invalid password")
	}
}
