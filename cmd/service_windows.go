//go:build windows

package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

func upgradeService() {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(service.ServiceName)
	if err != nil {
		log.Fatalf("service %q not found — is it installed?: %v", service.ServiceName, err)
	}

	// Get the installed binary path from the service config.
	cfg, err := s.Config()
	if err != nil {
		s.Close()
		log.Fatalf("failed to read service config: %v", err)
	}
	installedPath := parseBinaryPath(cfg.BinaryPathName)

	newPath, err := os.Executable()
	if err != nil {
		s.Close()
		log.Fatalf("failed to get current executable path: %v", err)
	}
	newPath, _ = filepath.Abs(newPath)
	installedPath, _ = filepath.Abs(installedPath)

	if newPath == installedPath {
		s.Close()
		log.Fatalf("new binary is the same as installed binary — run upgrade from a different location")
	}

	// Stop the service.
	fmt.Println("Stopping service...")
	_, err = s.Control(svc.Stop)
	s.Close()
	if err != nil {
		log.Printf("warning: could not stop service (may already be stopped): %v", err)
	}
	// Wait for the process to release the file.
	time.Sleep(2 * time.Second)

	// Back up the old binary.
	backupPath := installedPath + ".bak"
	if err := copyFile(installedPath, backupPath); err != nil {
		log.Printf("warning: could not create backup at %s: %v", backupPath, err)
	} else {
		fmt.Printf("Backed up old binary to %s\n", backupPath)
	}

	// Copy new binary over the installed one.
	if err := copyFile(newPath, installedPath); err != nil {
		log.Fatalf("failed to copy new binary to %s: %v\nRestore from backup: %s", installedPath, err, backupPath)
	}
	fmt.Printf("Updated binary at %s\n", installedPath)

	// Re-open service and start it.
	m2, err := mgr.Connect()
	if err != nil {
		log.Fatalf("binary replaced but failed to reconnect to service manager: %v — start manually", err)
	}
	defer m2.Disconnect()

	s2, err := m2.OpenService(service.ServiceName)
	if err != nil {
		log.Fatalf("binary replaced but failed to open service: %v — start manually", err)
	}
	defer s2.Close()

	if err := s2.Start(); err != nil {
		log.Fatalf("binary replaced but failed to start service: %v — start manually", err)
	}
	fmt.Printf("Service %q upgraded and started.\n", service.ServiceName)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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

// parseBinaryPath extracts the executable path from a BinaryPathName that may
// include arguments. Handles quoted paths like `"C:\path\winctl.exe" run`.
func parseBinaryPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, `"`) {
		if end := strings.Index(raw[1:], `"`); end >= 0 {
			return raw[1 : end+1]
		}
	}
	if i := strings.IndexByte(raw, ' '); i >= 0 {
		return raw[:i]
	}
	return raw
}
