package executor

import (
	"os/exec"
)

func Restart() error {
	return exec.Command("shutdown", "/r", "/t", "60").Run()
}

func LockScreen() error {
	return exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
}
