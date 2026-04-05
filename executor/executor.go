package executor

import "log/slog"

func DryRestart() error {
	slog.Info("[DRY RUN] simulating restart", "command", "shutdown /r /t 60")
	return nil
}

func DryLockScreen() error {
	slog.Info("[DRY RUN] simulating screen lock", "command", "LockWorkStation")
	return nil
}
