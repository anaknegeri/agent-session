package logger

import (
	"log/slog"
	"os"
)

// LevelEnv overrides the log level for both the CLI and the MCP server.
const LevelEnv = "AGENT_SESSION_LOG_LEVEL"

// New builds a slog.Logger with the given level ("debug", "info", "warn", "error").
func New(level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

// FromEnv builds a logger at the level named by AGENT_SESSION_LOG_LEVEL, falling
// back to fallback. The CLI writes its own human-readable output, so info-level
// records would otherwise show up as a second, noisier copy of it on stderr.
func FromEnv(fallback string) *slog.Logger {
	if level := os.Getenv(LevelEnv); level != "" {
		return New(level)
	}
	return New(fallback)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
