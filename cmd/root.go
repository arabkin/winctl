package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"winctl/config"
	"winctl/logging"
	"winctl/scheduler"
	"winctl/server"
	"winctl/service"
	"winctl/state"
	"winctl/updater"
)

var AppVersion = "1.1.7"

func Run() {
	if len(os.Args) < 2 {
		if service.IsWindowsService() {
			service.Version = AppVersion
			if err := service.RunService(); err != nil {
				log.Fatalf("service failed: %v", err)
			}
			return
		}
		printUsage()
		return
	}

	switch os.Args[1] {
	case "install":
		installService()
	case "uninstall":
		uninstallService()
	case "start":
		startService()
	case "stop":
		stopService()
	case "upgrade":
		upgradeService()
	case "run":
		runFlags := flag.NewFlagSet("run", flag.ExitOnError)
		dryRun := runFlags.Bool("dry-run", false, "simulate actions without executing them")
		runFlags.BoolVar(dryRun, "d", false, "simulate actions without executing them (shorthand)")
		configFile := runFlags.String("f", config.DefaultPath(), "path to config file")
		logLevel := runFlags.String("log", "", "log level: debug, info, error (overrides config)")
		if err := runFlags.Parse(os.Args[2:]); err != nil {
			log.Fatalf("invalid flags: %v", err)
		}
		runForeground(*dryRun, *configFile, *logLevel)
	default:
		printUsage()
	}
}

func runForeground(dryRun bool, configFile string, logLevelFlag string) {
	cfg, err := config.Load(configFile)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// CLI flag overrides config; config is the default.
	level := cfg.LogLevel
	if logLevelFlag != "" {
		switch logLevelFlag {
		case "debug", "info", "error":
			// valid
		default:
			log.Fatalf("invalid --log value %q: must be debug, info, or error", logLevelFlag)
		}
		if logLevelFlag != cfg.LogLevel {
			level = logLevelFlag
			cfg.LogLevel = level
			if saveErr := config.Save(cfg, configFile); saveErr != nil {
				log.Printf("warning: failed to persist log level to config: %v", saveErr)
			}
		}
	}
	logging.Setup(level)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := state.New(dryRun)
	statePath := state.StatePath(configFile)
	st.SetOnChange(func(intent state.Intent) {
		if err := state.SaveIntent(statePath, intent); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	})
	restartIvl := scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes}
	lockIvl := scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes}
	sched := scheduler.New(ctx, st, dryRun, restartIvl, lockIvl)

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

	upd := updater.New(AppVersion, "")
	srv := server.New(cfg, configFile, st, sched, upd, AppVersion)
	defer func() {
		if err := srv.Close(); err != nil {
			slog.Error("server close failed", "error", err)
		}
	}()

	go updater.BackgroundCheck(upd, ctx, cfg.UpdateCheckMinutes)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down")
		st.SetOnChange(nil) // disconnect persistence before stopping scheduler
		sched.Stop()
		cancel()
		// Second signal force-exits immediately.
		<-sigCh
		slog.Info("forced shutdown")
		os.Exit(1)
	}()

	mode := ""
	if dryRun {
		mode = " [DRY RUN]"
	}
	slog.Info("WinCtl running", "addr", fmt.Sprintf("http://0.0.0.0:%d", cfg.Port), "user", cfg.Username, "mode", mode)
	if err := server.Run(srv, ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func printUsage() {
	fmt.Print(`Usage: winctl <command>

Commands:
  install    Install as Windows service (starts it and creates firewall rule)
  uninstall  Remove Windows service and firewall rule
  upgrade    Replace installed binary with this one (stop -> copy -> start)
  start      Start the Windows service
  stop       Stop the Windows service
  run        Run in foreground (debug mode)

Run flags:
  -d, --dry-run    Simulate actions without executing them
  -f <path>        Path to config file (default: next to executable)
  --log <level>      Log level: debug, info, error (default: info)
`)
}
