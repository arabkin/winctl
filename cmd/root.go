package cmd

import (
	"context"
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
	case "run":
		runForeground()
	default:
		printUsage()
	}
}

func runForeground() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("config warning: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := state.New()
	sched := scheduler.New(ctx, st)
	srv := server.New(cfg, st, sched)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		sched.Stop()
		cancel()
	}()

	log.Printf("WinCtl running on http://localhost:%d (user: %s)", cfg.Port, cfg.Username)
	if err := server.Run(srv, ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func printUsage() {
	fmt.Println("Usage: winctl <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install    Install as Windows service")
	fmt.Println("  uninstall  Remove Windows service")
	fmt.Println("  start      Start the Windows service")
	fmt.Println("  stop       Stop the Windows service")
	fmt.Println("  run        Run in foreground (debug mode)")
}
