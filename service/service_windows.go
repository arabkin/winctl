//go:build windows

package service

import (
	"context"
	"log"
	"winctl/config"
	"winctl/scheduler"
	"winctl/server"
	"winctl/state"

	"golang.org/x/sys/windows/svc"
)

const ServiceName = "WinCtlSvc"
const DisplayName = "WinCtl Service"
const Description = "Machine control web dashboard — restart and lock scheduling"

type WinCtlService struct{}

func (s *WinCtlService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("config load error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := state.New()
	sched := scheduler.New(ctx, st)
	srv := server.New(cfg, st, sched)

	go func() {
		if err := server.Run(srv, ctx); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	status <- svc.Status{State: svc.Running, Accepts: accepted}
	log.Println("service running")

	for {
		c := <-req
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Println("service stopping")
			status <- svc.Status{State: svc.StopPending}
			sched.Stop()
			cancel()
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
