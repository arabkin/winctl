//go:build !windows

package cmd

import "fmt"

func installService() {
	fmt.Println("Service install is only supported on Windows.")
}

func uninstallService() {
	fmt.Println("Service uninstall is only supported on Windows.")
}

func startService() {
	fmt.Println("Service start is only supported on Windows.")
}

func stopService() {
	fmt.Println("Service stop is only supported on Windows.")
}
