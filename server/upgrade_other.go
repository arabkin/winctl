//go:build !windows

package server

import (
	"log/slog"
	"os"
)

// ServiceName is set by the service package before the server starts.
var ServiceName = "WinCtlSvc"

func applyUpgrade(tmpPath string) {
	slog.Warn("in-place upgrade is only supported on Windows", "path", tmpPath)
	_ = os.Remove(tmpPath)
}
