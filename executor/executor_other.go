//go:build !windows

package executor

import "fmt"

func Restart() error {
	return fmt.Errorf("restart is only supported on Windows")
}

func LockScreen() error {
	return fmt.Errorf("screen lock is only supported on Windows")
}
