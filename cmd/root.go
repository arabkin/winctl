package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"winctl/config"
	"winctl/scheduler"
	"winctl/server"
	"winctl/service"
	"winctl/state"
)

func Run() {
	if len(os.Args) < 2 {
		if service.IsWindowsService() {
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
		runFlags.Parse(os.Args[2:])
		runForeground(*dryRun, *configFile)
	default:
		printUsage()
	}
}

func runForeground(dryRun bool, configFile string) {
	cfg, err := config.Load(configFile)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := state.New(dryRun)
	restartIvl := scheduler.IntervalRange{MinMinutes: cfg.RestartMinMinutes, MaxMinutes: cfg.RestartMaxMinutes}
	lockIvl := scheduler.IntervalRange{MinMinutes: cfg.LockMinMinutes, MaxMinutes: cfg.LockMaxMinutes}
	sched := scheduler.New(ctx, st, dryRun, restartIvl, lockIvl)
	srv := server.New(cfg, configFile, st, sched)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		sched.Stop()
		cancel()
		// Second signal force-exits immediately.
		<-sigCh
		log.Println("forced shutdown")
		os.Exit(1)
	}()

	mode := ""
	if dryRun {
		mode = " [DRY RUN]"
	}
	log.Printf("WinCtl running on http://0.0.0.0:%d (user: %s)%s", cfg.Port, cfg.Username, mode)
	if err := server.Run(srv, ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func printUsage() {
	fmt.Println("Usage: winctl <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install    Install as Windows service (starts it and creates firewall rule)")
	fmt.Println("  uninstall  Remove Windows service and firewall rule")
	fmt.Println("  upgrade    Replace installed binary with this one (stop → copy → start)")
	fmt.Println("  start      Start the Windows service")
	fmt.Println("  stop       Stop the Windows service")
	fmt.Println("  run        Run in foreground (debug mode)")
	fmt.Println()
	fmt.Println("Run flags:")
	fmt.Println("  -d, --dry-run    Simulate actions without executing them")
	fmt.Println("  -f <path>        Path to config file (default: next to executable)")
}
