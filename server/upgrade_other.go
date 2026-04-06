//go:build !windows

package server

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// ServiceName is set by the service package before the server starts.
var ServiceName = "WinCtlSvc"

var upgradeInProgress atomic.Bool

func applyUpgrade(tmpPath string) {
	slog.Warn("in-place upgrade is only supported on Windows", "path", tmpPath)
	_ = os.Remove(tmpPath)
	upgradeInProgress.Store(false)
}
