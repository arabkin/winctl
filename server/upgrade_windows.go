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
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

const detachedProcess = 0x00000008 // DETACHED_PROCESS creation flag

// ServiceName is set by the service package before the server starts.
// Avoids import cycle (service imports server).
var ServiceName = "WinCtlSvc"

var upgradeInProgress atomic.Bool

// applyUpgrade replaces the installed service binary and restarts the service.
// Must be called in a goroutine — blocks until the restart script is launched.
func applyUpgrade(tmpPath string) {
	if !upgradeInProgress.CompareAndSwap(false, true) {
		slog.Warn("upgrade: already in progress, ignoring duplicate request")
		return
	}

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
	scriptContent := "@echo off\r\n" +
		"timeout /t 3 /nobreak >nul\r\n" +
		"echo Stopping service... >> \"" + logFile + "\"\r\n" +
		"net stop " + ServiceName + " >> \"" + logFile + "\" 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo WARNING: net stop failed, continuing anyway >> \"" + logFile + "\"\r\n" +
		")\r\n" +
		"echo Copying new binary... >> \"" + logFile + "\"\r\n" +
		"copy /y \"" + tmpPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo ERROR: copy failed, restoring backup >> \"" + logFile + "\"\r\n" +
		"  copy /y \"" + backupPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"  net start " + ServiceName + " >> \"" + logFile + "\" 2>&1\r\n" +
		"  goto :cleanup\r\n" +
		")\r\n" +
		"echo Starting service with new binary... >> \"" + logFile + "\"\r\n" +
		"net start " + ServiceName + " >> \"" + logFile + "\" 2>&1\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo ERROR: start failed, restoring backup >> \"" + logFile + "\"\r\n" +
		"  copy /y \"" + backupPath + "\" \"" + installedPath + "\" >> \"" + logFile + "\" 2>&1\r\n" +
		"  net start " + ServiceName + " >> \"" + logFile + "\" 2>&1\r\n" +
		")\r\n" +
		":cleanup\r\n" +
		"del \"" + tmpPath + "\" >nul 2>&1\r\n" +
		"echo Upgrade complete >> \"" + logFile + "\"\r\n" +
		"del \"%~f0\" >nul 2>&1\r\n" // bat deletes itself

	batPath := filepath.Join(os.TempDir(), "winctl-upgrade.bat")
	if err := os.WriteFile(batPath, []byte(scriptContent), 0600); err != nil {
		slog.Error("upgrade: failed to write upgrade script", "error", err)
		cleanup()
		return
	}

	cmd := exec.Command("cmd", "/c", batPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
	if err := cmd.Start(); err != nil {
		slog.Error("upgrade: failed to start upgrade script", "error", err)
		_ = os.Remove(batPath)
		cleanup()
		return
	}
	slog.Info("upgrade: script launched", "script", batPath, "log", logFile)

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
