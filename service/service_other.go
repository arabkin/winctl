//go:build !windows

package service

const ServiceName = "WinCtlSvc"
const DisplayName = "WinCtl Service"
const Description = "Machine control web dashboard — restart and lock scheduling"

func RunService() error {
	return nil
}

func IsWindowsService() bool {
	return false
}
