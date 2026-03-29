package state

import (
	"sync"
	"time"
)

type StatusDTO struct {
	RestartScheduleActive bool       `json:"restart_schedule_active"`
	RestartNextAt         *time.Time `json:"restart_next_at"`
	RestartPendingOnce    bool       `json:"restart_pending_once"`
	RestartOnceAt         *time.Time `json:"restart_once_at"`
	LockScheduleActive   bool       `json:"lock_schedule_active"`
	LockNextAt           *time.Time `json:"lock_next_at"`
	LockPendingOnce      bool       `json:"lock_pending_once"`
	LockOnceAt           *time.Time `json:"lock_once_at"`
}

type State struct {
	mu                  sync.RWMutex
	restartScheduleOn   bool
	restartNextAt       *time.Time
	restartPendingOnce  bool
	restartOnceAt       *time.Time
	lockScheduleOn      bool
	lockNextAt          *time.Time
	lockPendingOnce     bool
	lockOnceAt          *time.Time
}

func New() *State {
	return &State{}
}

func (s *State) Status() StatusDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusDTO{
		RestartScheduleActive: s.restartScheduleOn,
		RestartNextAt:         s.restartNextAt,
		RestartPendingOnce:    s.restartPendingOnce,
		RestartOnceAt:         s.restartOnceAt,
		LockScheduleActive:   s.lockScheduleOn,
		LockNextAt:           s.lockNextAt,
		LockPendingOnce:      s.lockPendingOnce,
		LockOnceAt:           s.lockOnceAt,
	}
}

func (s *State) SetRestartSchedule(on bool, next *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartScheduleOn = on
	s.restartNextAt = next
}

func (s *State) SetRestartOnce(pending bool, at *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartPendingOnce = pending
	s.restartOnceAt = at
}

func (s *State) SetLockSchedule(on bool, next *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockScheduleOn = on
	s.lockNextAt = next
}

func (s *State) SetLockOnce(pending bool, at *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockPendingOnce = pending
	s.lockOnceAt = at
}

func (s *State) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartScheduleOn = false
	s.restartNextAt = nil
	s.restartPendingOnce = false
	s.restartOnceAt = nil
	s.lockScheduleOn = false
	s.lockNextAt = nil
	s.lockPendingOnce = false
	s.lockOnceAt = nil
}
