package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
	"winctl/executor"
	"winctl/state"
)

type ExecFuncs struct {
	Restart    func() error
	LockScreen func() error
}

type IntervalRange struct {
	MinMinutes int
	MaxMinutes int
}

type Scheduler struct {
	ctx    context.Context
	cancel context.CancelFunc
	state  *state.State
	exec   ExecFuncs

	restartInterval IntervalRange
	lockInterval    IntervalRange

	mu sync.Mutex

	restartScheduleCancel context.CancelFunc
	restartScheduleGen    uint64
	restartOnceCancel     context.CancelFunc
	restartOnceGen        uint64
	lockScheduleCancel    context.CancelFunc
	lockScheduleGen       uint64
	lockOnceCancel        context.CancelFunc
	lockOnceGen           uint64
}

func New(ctx context.Context, st *state.State, dryRun bool, restartInterval, lockInterval IntervalRange) *Scheduler {
	exec := ExecFuncs{
		Restart:    executor.Restart,
		LockScreen: executor.LockScreen,
	}
	if dryRun {
		exec = ExecFuncs{
			Restart:    executor.DryRestart,
			LockScreen: executor.DryLockScreen,
		}
	}
	return NewWithExec(ctx, st, exec, restartInterval, lockInterval)
}

func NewWithExec(ctx context.Context, st *state.State, exec ExecFuncs, restartInterval, lockInterval IntervalRange) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		ctx:             ctx,
		cancel:          cancel,
		state:           st,
		exec:            exec,
		restartInterval: restartInterval,
		lockInterval:    lockInterval,
	}
}

func (s *Scheduler) Stop() {
	s.cancel()
}

func randomInterval(ir IntervalRange) time.Duration {
	spread := ir.MaxMinutes - ir.MinMinutes
	if spread <= 0 {
		return time.Duration(ir.MinMinutes) * time.Minute
	}
	minutes := rand.IntN(spread+1) + ir.MinMinutes
	return time.Duration(minutes) * time.Minute
}

func (s *Scheduler) StartRestartSchedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restartScheduleCancel != nil {
		return // already running
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.restartScheduleCancel = cancel
	s.restartScheduleGen++
	gen := s.restartScheduleGen

	go func() {
		defer func() {
			s.mu.Lock()
			if s.restartScheduleGen == gen {
				s.restartScheduleCancel = nil
				s.mu.Unlock()
				s.state.SetRestartSchedule(false, nil)
			} else {
				s.mu.Unlock()
			}
		}()

		for {
			s.mu.Lock()
			ivl := s.restartInterval
			s.mu.Unlock()
			interval := randomInterval(ivl)
			next := time.Now().Add(interval)
			s.state.SetRestartSchedule(true, &next)
			slog.Info("restart scheduled", "interval", interval, "next_at", next.Format(time.RFC3339))

			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				slog.Info("executing scheduled restart")
				if err := s.exec.Restart(); err != nil {
					slog.Error("restart failed", "error", err)
				}
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) StopRestartSchedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restartScheduleCancel != nil {
		s.restartScheduleCancel()
		s.restartScheduleCancel = nil
	}
}

func (s *Scheduler) RestartOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restartOnceCancel != nil {
		return // already pending
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.restartOnceCancel = cancel
	s.restartOnceGen++
	gen := s.restartOnceGen

	at := time.Now().Add(60 * time.Second)
	s.state.SetRestartOnce(true, &at)
	slog.Info("one-shot restart scheduled", "at", at.Format(time.RFC3339))

	go func() {
		timer := time.NewTimer(60 * time.Second)
		select {
		case <-timer.C:
			slog.Info("executing one-shot restart")
			if err := s.exec.Restart(); err != nil {
				slog.Error("restart failed", "error", err)
			}
		case <-ctx.Done():
			timer.Stop()
		}
		cancel()
		s.mu.Lock()
		if s.restartOnceGen == gen {
			s.restartOnceCancel = nil
			s.mu.Unlock()
			s.state.SetRestartOnce(false, nil)
		} else {
			s.mu.Unlock()
		}
	}()
}

func (s *Scheduler) StartLockSchedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockScheduleCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.lockScheduleCancel = cancel
	s.lockScheduleGen++
	gen := s.lockScheduleGen

	go func() {
		defer func() {
			s.mu.Lock()
			if s.lockScheduleGen == gen {
				s.lockScheduleCancel = nil
				s.mu.Unlock()
				s.state.SetLockSchedule(false, nil)
			} else {
				s.mu.Unlock()
			}
		}()

		for {
			s.mu.Lock()
			ivl := s.lockInterval
			s.mu.Unlock()
			interval := randomInterval(ivl)
			next := time.Now().Add(interval)
			s.state.SetLockSchedule(true, &next)
			slog.Info("lock scheduled", "interval", interval, "next_at", next.Format(time.RFC3339))

			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				slog.Info("executing scheduled lock")
				if err := s.exec.LockScreen(); err != nil {
					slog.Error("lock failed", "error", err)
				}
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) StopLockSchedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockScheduleCancel != nil {
		s.lockScheduleCancel()
		s.lockScheduleCancel = nil
	}
}

func (s *Scheduler) LockOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockOnceCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.lockOnceCancel = cancel
	s.lockOnceGen++
	gen := s.lockOnceGen

	at := time.Now().Add(60 * time.Second)
	s.state.SetLockOnce(true, &at)
	slog.Info("one-shot lock scheduled", "at", at.Format(time.RFC3339))

	go func() {
		timer := time.NewTimer(60 * time.Second)
		select {
		case <-timer.C:
			slog.Info("executing one-shot lock")
			if err := s.exec.LockScreen(); err != nil {
				slog.Error("lock failed", "error", err)
			}
		case <-ctx.Done():
			timer.Stop()
		}
		cancel()
		s.mu.Lock()
		if s.lockOnceGen == gen {
			s.lockOnceCancel = nil
			s.mu.Unlock()
			s.state.SetLockOnce(false, nil)
		} else {
			s.mu.Unlock()
		}
	}()
}

func (s *Scheduler) UpdateIntervals(restart, lock IntervalRange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartInterval = restart
	s.lockInterval = lock
	slog.Info("scheduler intervals updated", "restart_min", restart.MinMinutes, "restart_max", restart.MaxMinutes, "lock_min", lock.MinMinutes, "lock_max", lock.MaxMinutes)
}

func (s *Scheduler) ResetAll() {
	s.StopRestartSchedule()
	s.StopLockSchedule()

	s.mu.Lock()
	if s.restartOnceCancel != nil {
		s.restartOnceCancel()
		s.restartOnceCancel = nil
	}
	if s.lockOnceCancel != nil {
		s.lockOnceCancel()
		s.lockOnceCancel = nil
	}
	s.mu.Unlock()

	s.state.Reset()
	slog.Info("all schedules and pending actions reset")
}
