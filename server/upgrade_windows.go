//go:build windows

package server

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceName is set by the service package before the server starts.
// Avoids import cycle (service imports server).
var ServiceName = "WinCtlSvc"

var upgradeInProgress atomic.Bool

// preflightUpgradeCheck verifies the service manager is reachable and the
// service exists. Returns empty string on success, error message on failure.
func preflightUpgradeCheck() string {
	m, err := mgr.Connect()
	if err != nil {
		return "cannot connect to service manager: " + err.Error()
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return "service " + ServiceName + " not found: " + err.Error()
	}

	cfg, err := s.Config()
	s.Close()
	if err != nil {
		return "cannot read service config: " + err.Error()
	}

	installedPath := parseBinaryPath(cfg.BinaryPathName)
	if _, err := os.Stat(installedPath); err != nil {
		return "installed binary not found at " + installedPath + ": " + err.Error()
	}

	slog.Info("upgrade preflight passed", "service", ServiceName, "binary", installedPath)
	return ""
}

// applyUpgrade replaces the installed service binary and restarts the service.
// Must be called in a goroutine. The caller (handler) already holds upgradeInProgress.
func applyUpgrade(tmpPath string) {
	cleanup := func() {
		_ = os.Remove(tmpPath)
		upgradeInProgress.Store(false)
	}

	time.Sleep(1 * time.Second) // let the HTTP response reach the client

	m, err := mgr.Connect()
	if err != nil {
		slog.Error("upgrade: failed to connect to service manager", "error", err)
		cleanup()
		return
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		slog.Error("upgrade: service not found", "error", err)
		cleanup()
		return
	}

	cfg, err := s.Config()
	s.Close()
	if err != nil {
		slog.Error("upgrade: failed to read service config", "error", err)
		cleanup()
		return
	}

	// Parse BinaryPathName — may contain arguments (e.g. "C:\path\winctl.exe" run).
	installedPath := parseBinaryPath(cfg.BinaryPathName)
	slog.Info("upgrade: replacing binary", "installed", installedPath, "new", tmpPath)

	// Back up old binary — abort if backup fails (no rollback possible without it).
	backupPath := installedPath + ".bak"
	if err := copyFileForUpgrade(installedPath, backupPath); err != nil {
		slog.Error("upgrade: aborting — could not create backup", "path", backupPath, "error", err)
		cleanup()
		return
	}
	slog.Info("upgrade: backed up old binary", "path", backupPath)

	// Write upgrade script to a temp .bat file for auditability.
	// Uses if-errorlevel checks so failures trigger rollback.
	// Logs output to a file next to the binary for post-mortem.
	logFile := installedPath + ".upgrade.log"
	// Use ping for delay (timeout command fails in non-interactive SYSTEM context).
	// Use sc instead of net for service control (more reliable from SYSTEM).
	sn := ServiceName
	scriptContent := "@echo off\r\n" +
		"echo Upgrade started %date% %time% >> \"" + logFile + "\"\r\n" +
		"ping -n 4 127.0.0.1 >nul\r\n" +
		"echo Stopping service... >> \"" + logFile + "\"\r\n" +
		"sc.exe stop " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		"echo Waiting for service to stop... >> \"" + logFile + "\"\r\n" +
		"ping -n 6 127.0.0.1 >nul\r\n" +
		"sc.exe query " + sn + " | find \"STOPPED\" >nul 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo ERROR: service did not stop, aborting upgrade >> \"" + logFile + "\"\r\n" +
		"  sc.exe start " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		"  goto :cleanup\r\n" +
		")\r\n" +
		"echo Service stopped. Copying new binary... >> \"" + logFile + "\"\r\n" +
		"copy /y \"" + tmpPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo ERROR: copy failed, restoring backup >> \"" + logFile + "\"\r\n" +
		"  copy /y \"" + backupPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"  if errorlevel 1 (\r\n" +
		"    echo FATAL: rollback copy failed — restore manually from .bak >> \"" + logFile + "\"\r\n" +
		"    goto :cleanup\r\n" +
		"  )\r\n" +
		"  sc.exe start " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		"  goto :cleanup\r\n" +
		")\r\n" +
		"echo Starting service with new binary... >> \"" + logFile + "\"\r\n" +
		"sc.exe start " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		"echo Verifying service started... >> \"" + logFile + "\"\r\n" +
		"ping -n 6 127.0.0.1 >nul\r\n" +
		"sc.exe query " + sn + " | find \"RUNNING\" >nul 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo ERROR: service not running after start, restoring backup >> \"" + logFile + "\"\r\n" +
		"  sc.exe stop " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		"  ping -n 3 127.0.0.1 >nul\r\n" +
		"  copy /y \"" + backupPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"  if errorlevel 1 (\r\n" +
		"    echo FATAL: rollback copy failed — restore manually from .bak >> \"" + logFile + "\"\r\n" +
		"    goto :cleanup\r\n" +
		"  )\r\n" +
		"  sc.exe start " + sn + " >> \"" + logFile + "\" 2>&1\r\n" +
		")\r\n" +
		":cleanup\r\n" +
		"del \"" + tmpPath + "\" >nul 2>&1\r\n" +
		"schtasks.exe /delete /tn WinCtlUpgrade /f >nul 2>&1\r\n" +
		"echo Upgrade finished %date% %time% >> \"" + logFile + "\"\r\n" +
		"del \"%~f0\" >nul 2>&1\r\n" // bat deletes itself

	batPath := filepath.Join(os.TempDir(), "winctl-upgrade.bat")
	if err := os.WriteFile(batPath, []byte(scriptContent), 0600); err != nil {
		slog.Error("upgrade: failed to write upgrade script", "error", err)
		cleanup()
		return
	}

	// Use schtasks to run the bat script as an independent SYSTEM task.
	// This survives the service process being killed (unlike child processes).
	taskName := "WinCtlUpgrade"
	// Delete any leftover task from a previous attempt.
	_ = exec.Command("schtasks.exe", "/delete", "/tn", taskName, "/f").Run()
	// Create a one-time task that runs immediately as SYSTEM.
	cmd := exec.Command("schtasks.exe", "/create",
		"/tn", taskName,
		"/tr", `cmd.exe /c "`+batPath+`"`,
		"/sc", "once",
		"/st", "00:00",
		"/ru", "SYSTEM",
		"/f",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("upgrade: failed to create scheduled task", "error", err, "output", string(out))
		_ = os.Remove(batPath)
		cleanup()
		return
	}
	// Run the task immediately.
	cmd2 := exec.Command("schtasks.exe", "/run", "/tn", taskName)
	if out, err := cmd2.CombinedOutput(); err != nil {
		slog.Error("upgrade: failed to run scheduled task", "error", err, "output", string(out))
		_ = exec.Command("schtasks.exe", "/delete", "/tn", taskName, "/f").Run()
		_ = os.Remove(batPath)
		cleanup()
		return
	}
	slog.Info("upgrade: scheduled task launched", "task", taskName, "script", batPath, "log", logFile)

	// The service should be killed by the bat script within ~10 seconds.
	// If we're still alive after 30 seconds, the script failed — reset the flag.
	go func() {
		time.Sleep(30 * time.Second)
		if upgradeInProgress.CompareAndSwap(true, false) {
			slog.Error("upgrade: script did not restart service within 30s — upgrade may have failed, check " + logFile)
		}
	}()
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

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
