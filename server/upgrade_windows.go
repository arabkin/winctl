//go:build windows

package server

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "WinCtlSvc"

// applyUpgrade replaces the installed service binary and restarts the service.
// Must be called in a goroutine — blocks until the restart script is launched.
func applyUpgrade(tmpPath string) {
	time.Sleep(1 * time.Second) // let the HTTP response reach the client

	m, err := mgr.Connect()
	if err != nil {
		slog.Error("upgrade: failed to connect to service manager", "error", err)
		return
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		slog.Error("upgrade: service not found", "error", err)
		return
	}

	cfg, err := s.Config()
	s.Close()
	if err != nil {
		slog.Error("upgrade: failed to read service config", "error", err)
		return
	}
	installedPath := cfg.BinaryPathName
	slog.Info("upgrade: replacing binary", "installed", installedPath, "new", tmpPath)

	// Back up old binary.
	backupPath := installedPath + ".bak"
	if err := copyFileForUpgrade(installedPath, backupPath); err != nil {
		slog.Warn("upgrade: could not create backup", "path", backupPath, "error", err)
	} else {
		slog.Info("upgrade: backed up old binary", "path", backupPath)
	}

	// Spawn a detached process to: wait → stop service → replace binary → start service.
	// This must be external because our own process will be killed by the stop.
	script := fmt.Sprintf(
		`timeout /t 3 /nobreak >nul & net stop %s & copy /y "%s" "%s" & net start %s`,
		serviceName, tmpPath, installedPath, serviceName,
	)
	cmd := exec.Command("cmd", "/c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		slog.Error("upgrade: failed to start upgrade script", "error", err)
		return
	}
	slog.Info("upgrade: restart script launched, service will restart shortly")
}

func copyFileForUpgrade(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
