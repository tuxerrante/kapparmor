package main

import (
	"log/slog"
	"os"
)

// newDefaultLogger returns a sane default slog logger used across the app.
func newDefaultLogger() *slog.Logger {
	return slog.New(
		slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: nil, AddSource: false},
		))
}
