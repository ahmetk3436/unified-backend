package logging

import (
	"log/slog"
	"os"
)

// Setup initializes the global slog logger with JSON output to stdout.
func Setup() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}
