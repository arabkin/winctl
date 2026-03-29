package state

import (
	"sync"
	"testing"
	"time"
)

func TestNewStateIsIdle(t *testing.T) {
	s := New(false)
	st := s.Status()

	if st.RestartScheduleActive {
		t.Error("restart schedule should be inactive on new state")
	}
	if st.LockScheduleActive {
		t.Error("lock schedule should be inactive on new state")
	}
	if st.RestartPendingOnce {
		t.Error("restart once should not be pending on new state")
	}
	if st.LockPendingOnce {
		t.Error("lock once should not be pending on new state")
	}
	if st.RestartNextAt != nil || st.LockNextAt != nil || st.RestartOnceAt != nil || st.LockOnceAt != nil {
		t.Error("all time fields should be nil on new state")
	}
}

func TestSetRestartSchedule(t *testing.T) {
	s := New(false)
	now := time.Now()
	s.SetRestartSchedule(true, &now)

	st := s.Status()
	if !st.RestartScheduleActive {
		t.Error("restart schedule should be active")
	}
	if st.RestartNextAt == nil || !st.RestartNextAt.Equal(now) {
		t.Error("restart next_at should match set time")
	}

	s.SetRestartSchedule(false, nil)
	st = s.Status()
	if st.RestartScheduleActive {
		t.Error("restart schedule should be inactive after disable")
	}
	if st.RestartNextAt != nil {
		t.Error("restart next_at should be nil after disable")
	}
}

func TestSetLockSchedule(t *testing.T) {
	s := New(false)
	now := time.Now()
	s.SetLockSchedule(true, &now)

	st := s.Status()
	if !st.LockScheduleActive {
		t.Error("lock schedule should be active")
	}
	if st.LockNextAt == nil || !st.LockNextAt.Equal(now) {
		t.Error("lock next_at should match set time")
	}
}

func TestSetRestartOnce(t *testing.T) {
	s := New(false)
	at := time.Now().Add(60 * time.Second)
	s.SetRestartOnce(true, &at)

	st := s.Status()
	if !st.RestartPendingOnce {
		t.Error("restart once should be pending")
	}
	if st.RestartOnceAt == nil || !st.RestartOnceAt.Equal(at) {
		t.Error("restart once_at should match set time")
	}
}

func TestSetLockOnce(t *testing.T) {
	s := New(false)
	s.SetLockOnce(true, new(time.Now().Add(60*time.Second)))

	st := s.Status()
	if !st.LockPendingOnce {
		t.Error("lock once should be pending")
	}
}

func TestReset(t *testing.T) {
	s := New(false)
	now := time.Now()
	s.SetRestartSchedule(true, &now)
	s.SetLockSchedule(true, &now)
	s.SetRestartOnce(true, &now)
	s.SetLockOnce(true, &now)

	s.Reset()
	st := s.Status()

	if st.RestartScheduleActive || st.LockScheduleActive {
		t.Error("schedules should be inactive after reset")
	}
	if st.RestartPendingOnce || st.LockPendingOnce {
		t.Error("pending once should be false after reset")
	}
	if st.RestartNextAt != nil || st.LockNextAt != nil || st.RestartOnceAt != nil || st.LockOnceAt != nil {
		t.Error("all time fields should be nil after reset")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(false)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			now := time.Now()
			s.SetRestartSchedule(true, &now)
		}()
		go func() {
			defer wg.Done()
			s.Status()
		}()
		go func() {
			defer wg.Done()
			s.Reset()
		}()
	}
	wg.Wait()
}
