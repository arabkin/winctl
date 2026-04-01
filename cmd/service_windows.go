//go:build windows

package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"winctl/config"
	"winctl/service"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const firewallRuleName = "WinCtl Dashboard"

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

	if err := s.Start(); err != nil {
		log.Printf("warning: service installed but failed to start: %v", err)
		fmt.Println("Run 'winctl.exe start' to start the service manually.")
	} else {
		fmt.Printf("Service %q started.\n", service.ServiceName)
	}

	addFirewallRule()
}

func addFirewallRule() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Printf("warning: could not load config to read port, using default 8443: %v", err)
		cfg = &config.Config{Port: 8443}
	}
	port := fmt.Sprintf("%d", cfg.Port)

	// Remove any existing rule first to avoid duplicates.
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		fmt.Sprintf("name=%s", firewallRuleName)).Run()

	out, err := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		fmt.Sprintf("name=%s", firewallRuleName),
		"dir=in",
		"action=allow",
		"protocol=TCP",
		fmt.Sprintf("localport=%s", port),
		"profile=private",
	).CombinedOutput()
	if err != nil {
		log.Printf("warning: failed to add firewall rule: %v\n%s", err, out)
		fmt.Println("Could not create firewall rule. You may need to add it manually:")
		fmt.Printf("  netsh advfirewall firewall add rule name=\"%s\" dir=in action=allow protocol=TCP localport=%s profile=private\n", firewallRuleName, port)
		return
	}
	fmt.Printf("Firewall rule %q added (port %s, private networks only).\n", firewallRuleName, port)
	fmt.Println("NOTE: Ensure your home network is set to \"Private\" in Windows:")
	fmt.Println("  Settings > Network & Internet > your connection > Network profile type > Private")
}

func removeFirewallRule() {
	out, err := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		fmt.Sprintf("name=%s", firewallRuleName)).CombinedOutput()
	if err != nil {
		log.Printf("warning: failed to remove firewall rule: %v\n%s", err, out)
		return
	}
	fmt.Printf("Firewall rule %q removed.\n", firewallRuleName)
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

	removeFirewallRule()
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
