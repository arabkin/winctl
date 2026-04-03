package logging

import (
	"log/slog"
	"os"
)

// Level constants for configuration.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelError = "error"
)

// Setup initializes the global slog logger with the given level.
// Output format: timestamp + level + message + fields.
func Setup(level string) {
	var lvl slog.Level
	switch level {
	case LevelDebug:
		lvl = slog.LevelDebug
	case LevelInfo:
		lvl = slog.LevelInfo
	case LevelError:
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
