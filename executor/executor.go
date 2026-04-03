package executor

import (
	"log/slog"
	"os/exec"
)

func Restart() error {
	return exec.Command("shutdown", "/r", "/t", "60").Run()
}

func LockScreen() error {
	return exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
}

func DryRestart() error {
	slog.Info("[DRY RUN] simulating restart", "command", "shutdown /r /t 60")
	return nil
}

func DryLockScreen() error {
	slog.Info("[DRY RUN] simulating screen lock", "command", "rundll32 LockWorkStation")
	return nil
}
