//go:build windows

package service

import (
	"context"
	"log/slog"
	"winctl/config"
	"winctl/logging"
	"winctl/scheduler"
	"winctl/server"
	"winctl/state"
	"winctl/updater"

	"golang.org/x/sys/windows/svc"
)

// configPath is used by both service startup and state persistence.
var configPath = config.DefaultPath()

const ServiceName = "WinCtlSvc"
const DisplayName = "WinCtl Service"
const Description = "Machine control web dashboard — restart and lock scheduling"

// Version is set by cmd before RunService() is called.
var Version = "0.0.0"

type WinCtlService struct{}

func (s *WinCtlService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	server.ServiceName = ServiceName

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config load error, service cannot start", "error", err)
		return false, 1
	}

	logging.Setup(cfg.LogLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := state.New(false)
	statePath := state.StatePath(configPath)
	st.SetOnChange(func(intent state.Intent) {
		if err := state.SaveIntent(statePath, intent); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	})
	restartIvl := scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes}
	lockIvl := scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes}
	sched := scheduler.New(ctx, st, false, restartIvl, lockIvl)

	// Restore previously active schedules.
	intent := state.LoadIntent(statePath)
	if intent.RestartScheduleEnabled {
		slog.Info("restoring restart schedule from saved state")
		sched.StartRestartSchedule()
	}
	if intent.LockScheduleEnabled {
		slog.Info("restoring lock schedule from saved state")
		sched.StartLockSchedule()
	}

	upd := updater.New(Version, "")
	srv := server.New(cfg, configPath, st, sched, upd, Version)

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		if err := server.Run(srv, ctx); err != nil {
			slog.Error("server error", "error", err)
		}
	}()

	go updater.BackgroundCheck(upd, ctx, cfg.UpdateCheckMinutes)

	status <- svc.Status{State: svc.Running, Accepts: accepted}
	slog.Info("service running")

	for {
		c := <-req
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			slog.Info("service stopping")
			status <- svc.Status{State: svc.StopPending}
			st.SetOnChange(nil) // disconnect persistence before stopping scheduler
			sched.Stop()
			cancel()
			<-serverDone
			return false, 0
		}
	}
}

func RunService() error {
	return svc.Run(ServiceName, &WinCtlService{})
}

func IsWindowsService() bool {
	is, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return is
}
