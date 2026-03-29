//go:build windows

package cmd

import (
	"fmt"
	"log"
	"os"
	"winctl/service"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func installService() {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to get executable path: %v", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.CreateService(service.ServiceName, exePath, mgr.Config{
		DisplayName: service.DisplayName,
		Description: service.Description,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		log.Fatalf("failed to create service: %v", err)
	}
	defer s.Close()
	fmt.Printf("Service %q installed successfully.\n", service.ServiceName)
}

func uninstallService() {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(service.ServiceName)
	if err != nil {
		log.Fatalf("service %q not found: %v", service.ServiceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		log.Fatalf("failed to delete service: %v", err)
	}
	fmt.Printf("Service %q removed successfully.\n", service.ServiceName)
}

func startService() {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(service.ServiceName)
	if err != nil {
		log.Fatalf("service %q not found: %v", service.ServiceName, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		log.Fatalf("failed to start service: %v", err)
	}
	fmt.Printf("Service %q started.\n", service.ServiceName)
}

func stopService() {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(service.ServiceName)
	if err != nil {
		log.Fatalf("service %q not found: %v", service.ServiceName, err)
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	if err != nil {
		log.Fatalf("failed to stop service: %v", err)
	}
	fmt.Printf("Service %q stopped.\n", service.ServiceName)
}
