//go:build !windows

package server

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// ServiceName is set by the service package before the server starts.
// Used on Windows; declared here so non-Windows builds compile.
var ServiceName = "WinCtlSvc"

var upgradeInProgress atomic.Bool

func preflightUpgradeCheck() string {
	_ = ServiceName // used on Windows; set by service package
	return ""       // no preflight on non-Windows; applyUpgrade handles the no-op
}

func applyUpgrade(tmpPath string) {
	slog.Warn("in-place upgrade is only supported on Windows", "path", tmpPath)
	_ = os.Remove(tmpPath)
	upgradeInProgress.Store(false)
}
