//go:build windows

package executor

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWtsapi32          = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQueryUserToken = modWtsapi32.NewProc("WTSQueryUserToken")
)

func Restart() error {
	return exec.Command("shutdown", "/r", "/t", "60").Run()
}

func LockScreen() error {
	// Get the active console session (the physical screen).
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		return fmt.Errorf("no active console session found")
	}

	// Get the logged-in user's token for that session.
	var token windows.Token
	r, _, err := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return fmt.Errorf("WTSQueryUserToken for session %d: %v", sessionID, err)
	}
	defer token.Close()

	// Duplicate as a primary token for CreateProcessAsUser.
	var dupToken windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&dupToken,
	); err != nil {
		return fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer dupToken.Close()

	// Launch rundll32 in the user's interactive desktop to lock the workstation.
	cmdLine, _ := windows.UTF16PtrFromString("rundll32.exe user32.dll,LockWorkStation")
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktop
	var pi windows.ProcessInformation

	err2 := windows.CreateProcessAsUser(
		dupToken,
		nil,
		cmdLine,
		nil,
		nil,
		false,
		0,
		nil,
		nil,
		&si,
		&pi,
	)
	if err2 != nil {
		return fmt.Errorf("CreateProcessAsUser: %w", err2)
	}

	_ = windows.CloseHandle(pi.Process)
	_ = windows.CloseHandle(pi.Thread)
	return nil
}
