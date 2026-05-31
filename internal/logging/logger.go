// Package logging provides structured logging helpers for Velkron Pulse.
package logging

import (
	"log/slog"
	"os"
)

// Logger is the default structured logger for the application.
var Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))
