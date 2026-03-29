package executor

import (
	"log"
	"os/exec"
)

func Restart() error {
	return exec.Command("shutdown", "/r", "/t", "60").Run()
}

func LockScreen() error {
	return exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
}

func DryRestart() error {
	log.Println("[DRY RUN] simulating restart (shutdown /r /t 60)")
	return nil
}

func DryLockScreen() error {
	log.Println("[DRY RUN] simulating screen lock (rundll32 LockWorkStation)")
	return nil
}
