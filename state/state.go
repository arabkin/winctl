package state

import (
	"sync"
	"time"
)

type StatusDTO struct {
	DryRun                bool       `json:"dry_run"`
	RestartScheduleActive bool       `json:"restart_schedule_active"`
	RestartNextAt         *time.Time `json:"restart_next_at"`
	RestartPendingOnce    bool       `json:"restart_pending_once"`
	RestartOnceAt         *time.Time `json:"restart_once_at"`
	LockScheduleActive    bool       `json:"lock_schedule_active"`
	LockNextAt            *time.Time `json:"lock_next_at"`
	LockPendingOnce       bool       `json:"lock_pending_once"`
	LockOnceAt            *time.Time `json:"lock_once_at"`
}

type State struct {
	mu                 sync.RWMutex
	dryRun             bool
	restartScheduleOn  bool
	restartNextAt      *time.Time
	restartPendingOnce bool
	restartOnceAt      *time.Time
	lockScheduleOn     bool
	lockNextAt         *time.Time
	lockPendingOnce    bool
	lockOnceAt         *time.Time
	onChange           func(Intent)
}

func New(dryRun bool) *State {
	return &State{dryRun: dryRun}
}

// SetOnChange registers a callback invoked whenever schedule intent changes.
func (s *State) SetOnChange(fn func(Intent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// intent returns the current schedule intent. Must be called with lock held.
func (s *State) intent() Intent {
	return Intent{
		RestartScheduleEnabled: s.restartScheduleOn,
		LockScheduleEnabled:    s.lockScheduleOn,
	}
}

// notifyChange calls the onChange callback if set. Must be called without lock held.
func (s *State) notifyChange() {
	s.mu.RLock()
	fn := s.onChange
	intent := s.intent()
	s.mu.RUnlock()
	if fn != nil {
		fn(intent)
	}
}

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func (s *State) Status() StatusDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusDTO{
		DryRun:                s.dryRun,
		RestartScheduleActive: s.restartScheduleOn,
		RestartNextAt:         copyTime(s.restartNextAt),
		RestartPendingOnce:    s.restartPendingOnce,
		RestartOnceAt:         copyTime(s.restartOnceAt),
		LockScheduleActive:    s.lockScheduleOn,
		LockNextAt:            copyTime(s.lockNextAt),
		LockPendingOnce:       s.lockPendingOnce,
		LockOnceAt:            copyTime(s.lockOnceAt),
	}
}

func (s *State) SetRestartSchedule(on bool, next *time.Time) {
	s.mu.Lock()
	prev := s.restartScheduleOn
	s.restartScheduleOn = on
	s.restartNextAt = copyTime(next)
	s.mu.Unlock()
	if on != prev {
		s.notifyChange()
	}
}

func (s *State) SetRestartOnce(pending bool, at *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartPendingOnce = pending
	s.restartOnceAt = copyTime(at)
}

func (s *State) SetLockSchedule(on bool, next *time.Time) {
	s.mu.Lock()
	prev := s.lockScheduleOn
	s.lockScheduleOn = on
	s.lockNextAt = copyTime(next)
	s.mu.Unlock()
	if on != prev {
		s.notifyChange()
	}
}

func (s *State) SetLockOnce(pending bool, at *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockPendingOnce = pending
	s.lockOnceAt = copyTime(at)
}

func (s *State) Reset() {
	s.mu.Lock()
	hadSchedules := s.restartScheduleOn || s.lockScheduleOn
	s.restartScheduleOn = false
	s.restartNextAt = nil
	s.restartPendingOnce = false
	s.restartOnceAt = nil
	s.lockScheduleOn = false
	s.lockNextAt = nil
	s.lockPendingOnce = false
	s.lockOnceAt = nil
	s.mu.Unlock()
	if hadSchedules {
		s.notifyChange()
	}
}
