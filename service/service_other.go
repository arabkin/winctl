//go:build !windows

package service

const ServiceName = "WinCtlSvc"
const DisplayName = "WinCtl Service"
const Description = "Machine control web dashboard — restart and lock scheduling"

// Version is set by cmd before RunService() is called.
// Used in service_windows.go; declared here so non-Windows builds compile.
var Version = "0.0.0"

func RunService() error {
	_ = Version // used on Windows; set by cmd
	return nil
}

func IsWindowsService() bool {
	return false
}
