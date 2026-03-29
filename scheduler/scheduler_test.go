package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	"winctl/state"
)

func noopExec() ExecFuncs {
	return ExecFuncs{
		Restart:    func() error { return nil },
		LockScreen: func() error { return nil },
	}
}

func TestStartAndStopRestartSchedule(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.StartRestartSchedule()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.RestartScheduleActive {
		t.Error("restart schedule should be active")
	}
	if status.RestartNextAt == nil {
		t.Error("restart next_at should be set")
	}

	s.StopRestartSchedule()
	time.Sleep(10 * time.Millisecond)

	status = st.Status()
	if status.RestartScheduleActive {
		t.Error("restart schedule should be inactive after stop")
	}
}

func TestStartRestartScheduleIdempotent(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.StartRestartSchedule()
	s.StartRestartSchedule() // should not panic or create duplicate
	time.Sleep(10 * time.Millisecond)

	s.StopRestartSchedule()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if status.RestartScheduleActive {
		t.Error("should be inactive after single stop")
	}
}

func TestStopRestartScheduleWhenNotRunning(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	// Should not panic
	s.StopRestartSchedule()
}

func TestStartAndStopLockSchedule(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.StartLockSchedule()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.LockScheduleActive {
		t.Error("lock schedule should be active")
	}

	s.StopLockSchedule()
	time.Sleep(10 * time.Millisecond)

	status = st.Status()
	if status.LockScheduleActive {
		t.Error("lock schedule should be inactive after stop")
	}
}

func TestRestartOnce(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.RestartOnce()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.RestartPendingOnce {
		t.Error("restart should be pending")
	}
	if status.RestartOnceAt == nil {
		t.Error("restart once_at should be set")
	}
}

func TestRestartOnceIdempotent(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.RestartOnce()
	s.RestartOnce() // second call should be ignored
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.RestartPendingOnce {
		t.Error("restart should still be pending")
	}
}

func TestLockOnce(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.LockOnce()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if !status.LockPendingOnce {
		t.Error("lock should be pending")
	}
}

func TestResetAll(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	s.StartRestartSchedule()
	s.StartLockSchedule()
	s.RestartOnce()
	s.LockOnce()
	time.Sleep(10 * time.Millisecond)

	s.ResetAll()
	time.Sleep(10 * time.Millisecond)

	status := st.Status()
	if status.RestartScheduleActive || status.LockScheduleActive {
		t.Error("schedules should be inactive after reset")
	}
	if status.RestartPendingOnce || status.LockPendingOnce {
		t.Error("pending once should be false after reset")
	}
}

func TestResetAllWhenNothingActive(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())
	defer s.Stop()

	// Should not panic
	s.ResetAll()
}

func TestStopCancelsSchedules(t *testing.T) {
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, noopExec())

	s.StartRestartSchedule()
	s.StartLockSchedule()
	time.Sleep(10 * time.Millisecond)

	s.Stop()
	time.Sleep(50 * time.Millisecond)

	status := st.Status()
	if status.RestartScheduleActive {
		t.Error("restart schedule should be inactive after Stop()")
	}
	if status.LockScheduleActive {
		t.Error("lock schedule should be inactive after Stop()")
	}
}

func TestRandomIntervalRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		d := randomInterval()
		if d < 1*time.Minute || d > 10*time.Minute {
			t.Errorf("randomInterval() = %v, want [1m, 10m]", d)
		}
	}
}

func TestExecFuncsCalled(t *testing.T) {
	var restartCount atomic.Int32
	var lockCount atomic.Int32

	exec := ExecFuncs{
		Restart:    func() error { restartCount.Add(1); return nil },
		LockScreen: func() error { lockCount.Add(1); return nil },
	}

	// We can't easily test the scheduled execution without waiting minutes,
	// but we can verify the functions are wired correctly by checking the struct.
	st := state.New()
	ctx := context.Background()
	s := NewWithExec(ctx, st, exec)
	defer s.Stop()

	if s.exec.Restart == nil || s.exec.LockScreen == nil {
		t.Error("exec functions should be set")
	}

	// Call them directly to verify wiring
	s.exec.Restart()
	s.exec.LockScreen()

	if restartCount.Load() != 1 {
		t.Errorf("restart should have been called once, got %d", restartCount.Load())
	}
	if lockCount.Load() != 1 {
		t.Errorf("lock should have been called once, got %d", lockCount.Load())
	}
}
