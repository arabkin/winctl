//go:build windows

package executor

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32                    = windows.NewLazySystemDLL("wtsapi32.dll")
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	advapi32                    = windows.NewLazySystemDLL("advapi32.dll")
	procWTSGetActiveConsoleSessionID = kernel32.NewProc("WTSGetActiveConsoleSessionId")
	procWTSQueryUserToken       = wtsapi32.NewProc("WTSQueryUserToken")
	procCreateProcessAsUserW    = advapi32.NewProc("CreateProcessAsUserW")
)

func Restart() error {
	return exec.Command("shutdown", "/r", "/t", "60").Run()
}

func LockScreen() error {
	// Get the active console session (the physical screen).
	sessionID, _, _ := procWTSGetActiveConsoleSessionID.Call()
	if sessionID == 0xFFFFFFFF {
		return fmt.Errorf("no active console session found")
	}

	// Get the logged-in user's token for that session.
	var token windows.Token
	r, _, err := procWTSQueryUserToken.Call(sessionID, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return fmt.Errorf("WTSQueryUserToken failed: %w", err)
	}
	defer token.Close()

	// Duplicate the token for CreateProcessAsUser.
	var dupToken windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&dupToken,
	); err != nil {
		return fmt.Errorf("DuplicateTokenEx failed: %w", err)
	}
	defer dupToken.Close()

	// Launch rundll32 in the user's session to lock the workstation.
	cmdLine, _ := windows.UTF16PtrFromString("rundll32.exe user32.dll,LockWorkStation")
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation

	r, _, err = procCreateProcessAsUserW.Call(
		uintptr(dupToken),
		0,
		uintptr(unsafe.Pointer(cmdLine)),
		0, 0,
		0, // don't inherit handles
		0, // creation flags
		0, // environment (inherit)
		0, // current directory (inherit)
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		return fmt.Errorf("CreateProcessAsUser failed: %w", err)
	}

	windows.CloseHandle(pi.Process)
	windows.CloseHandle(pi.Thread)
	return nil
}
