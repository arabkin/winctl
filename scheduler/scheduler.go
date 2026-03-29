package scheduler

import (
	"context"
	"log"
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
	restartOnceCancel     context.CancelFunc
	lockScheduleCancel    context.CancelFunc
	lockOnceCancel        context.CancelFunc
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

	go func() {
		for {
			interval := randomInterval(s.restartInterval)
			next := time.Now().Add(interval)
			s.state.SetRestartSchedule(true, &next)
			log.Printf("restart scheduled in %v (at %s)", interval, next.Format(time.RFC3339))

			select {
			case <-time.After(interval):
				log.Println("executing scheduled restart")
				if err := s.exec.Restart(); err != nil {
					log.Printf("restart failed: %v", err)
				}
			case <-ctx.Done():
				s.state.SetRestartSchedule(false, nil)
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
		s.state.SetRestartSchedule(false, nil)
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

	at := time.Now().Add(60 * time.Second)
	s.state.SetRestartOnce(true, &at)
	log.Printf("one-shot restart in 60s (at %s)", at.Format(time.RFC3339))

	go func() {
		select {
		case <-time.After(60 * time.Second):
			log.Println("executing one-shot restart")
			if err := s.exec.Restart(); err != nil {
				log.Printf("restart failed: %v", err)
			}
			s.mu.Lock()
			s.restartOnceCancel = nil
			s.mu.Unlock()
			s.state.SetRestartOnce(false, nil)
		case <-ctx.Done():
			s.state.SetRestartOnce(false, nil)
			return
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

	go func() {
		for {
			interval := randomInterval(s.lockInterval)
			next := time.Now().Add(interval)
			s.state.SetLockSchedule(true, &next)
			log.Printf("lock scheduled in %v (at %s)", interval, next.Format(time.RFC3339))

			select {
			case <-time.After(interval):
				log.Println("executing scheduled lock")
				if err := s.exec.LockScreen(); err != nil {
					log.Printf("lock failed: %v", err)
				}
			case <-ctx.Done():
				s.state.SetLockSchedule(false, nil)
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
		s.state.SetLockSchedule(false, nil)
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

	at := time.Now().Add(60 * time.Second)
	s.state.SetLockOnce(true, &at)
	log.Printf("one-shot lock in 60s (at %s)", at.Format(time.RFC3339))

	go func() {
		select {
		case <-time.After(60 * time.Second):
			log.Println("executing one-shot lock")
			if err := s.exec.LockScreen(); err != nil {
				log.Printf("lock failed: %v", err)
			}
			s.mu.Lock()
			s.lockOnceCancel = nil
			s.mu.Unlock()
			s.state.SetLockOnce(false, nil)
		case <-ctx.Done():
			s.state.SetLockOnce(false, nil)
			return
		}
	}()
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
	log.Println("all schedules and pending actions reset")
}
