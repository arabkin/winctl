//go:build !windows

package server

import (
	"log/slog"
	"os"
)

func applyUpgrade(tmpPath string) {
	slog.Warn("in-place upgrade is only supported on Windows", "path", tmpPath)
	_ = os.Remove(tmpPath)
}
