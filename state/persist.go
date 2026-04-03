package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Intent captures which schedules should be active across restarts.
// One-shot timers are not persisted (they're short-lived and would be stale).
type Intent struct {
	RestartScheduleEnabled bool `json:"restart_schedule_enabled"`
	LockScheduleEnabled    bool `json:"lock_schedule_enabled"`
}

// StatePath returns the state.json path next to the given config file.
func StatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "state.json")
}

// SaveIntent writes the current schedule intent to disk.
func SaveIntent(path string, intent Intent) error {
	data, err := json.MarshalIndent(intent, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("save state to %s: %w", path, err)
	}
	return nil
}

// LoadIntent reads the persisted schedule intent from disk.
// Returns zero Intent if the file doesn't exist or is invalid.
func LoadIntent(path string) Intent {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("could not read state file", "path", path, "error", err)
		}
		return Intent{}
	}
	var intent Intent
	if err := json.Unmarshal(data, &intent); err != nil {
		slog.Warn("invalid state file", "path", path, "error", err)
		return Intent{}
	}
	return intent
}
