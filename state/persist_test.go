package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadIntent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	intent := Intent{RestartScheduleEnabled: true, LockScheduleEnabled: false}
	if err := SaveIntent(path, intent); err != nil {
		t.Fatal(err)
	}

	loaded := LoadIntent(path)
	if loaded.RestartScheduleEnabled != true {
		t.Errorf("restart: got %v, want true", loaded.RestartScheduleEnabled)
	}
	if loaded.LockScheduleEnabled != false {
		t.Errorf("lock: got %v, want false", loaded.LockScheduleEnabled)
	}
}

func TestLoadIntentMissingFile(t *testing.T) {
	intent := LoadIntent(filepath.Join(t.TempDir(), "nonexistent.json"))
	if intent.RestartScheduleEnabled || intent.LockScheduleEnabled {
		t.Error("expected zero intent for missing file")
	}
}

func TestLoadIntentInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatal(err)
	}

	intent := LoadIntent(path)
	if intent.RestartScheduleEnabled || intent.LockScheduleEnabled {
		t.Error("expected zero intent for invalid JSON")
	}
}

func TestStatePath(t *testing.T) {
	got := StatePath("/some/dir/config.json")
	want := filepath.Join("/some/dir", "state.json")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestOnChangeCalledOnScheduleToggle(t *testing.T) {
	st := New(false)
	var called int
	var lastIntent Intent
	st.SetOnChange(func(intent Intent) {
		called++
		lastIntent = intent
	})

	// Enable restart schedule.
	st.SetRestartSchedule(true, nil)
	if called != 1 {
		t.Fatalf("expected 1 call, got %d", called)
	}
	if !lastIntent.RestartScheduleEnabled {
		t.Error("expected restart enabled in intent")
	}

	// Setting same value should not trigger.
	st.SetRestartSchedule(true, nil)
	if called != 1 {
		t.Fatalf("expected 1 call (no change), got %d", called)
	}

	// Disable restart.
	st.SetRestartSchedule(false, nil)
	if called != 2 {
		t.Fatalf("expected 2 calls, got %d", called)
	}

	// Enable lock schedule.
	st.SetLockSchedule(true, nil)
	if called != 3 {
		t.Fatalf("expected 3 calls, got %d", called)
	}
	if !lastIntent.LockScheduleEnabled {
		t.Error("expected lock enabled in intent")
	}
}

func TestOnChangeCalledOnReset(t *testing.T) {
	st := New(false)
	var called int
	st.SetOnChange(func(intent Intent) {
		called++
	})

	// Reset with no schedules active should not trigger.
	st.Reset()
	if called != 0 {
		t.Fatalf("expected 0 calls (nothing to reset), got %d", called)
	}

	// Enable a schedule, then reset.
	st.SetRestartSchedule(true, nil) // triggers once
	called = 0
	st.Reset()
	if called != 1 {
		t.Fatalf("expected 1 call from reset, got %d", called)
	}
}

func TestSaveIntentReturnsErrorForBadPath(t *testing.T) {
	err := SaveIntent("/nonexistent/dir/state.json", Intent{RestartScheduleEnabled: true})
	if err == nil {
		t.Fatal("expected non-nil error for bad path")
	}
}

func TestSaveIntentFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := SaveIntent(path, Intent{RestartScheduleEnabled: true}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("permissions: got %o, want 0600", perm)
	}
}
