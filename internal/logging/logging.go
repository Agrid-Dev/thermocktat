// Package logging builds a configured *slog.Logger for the app.
//
// The public surface is intentionally tiny: one Config, one constructor. Callers
// receive a *slog.Logger value and inject it where needed; the process-global
// slog.Default() is never modified.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config selects the level and output format. Invalid or empty values fall back
// to info / text so a bad env var cannot prevent the app from starting.
type Config struct {
	Level  string `koanf:"level"  json:"level"  yaml:"level"`
	Format string `koanf:"format" json:"format" yaml:"format"`
}

// New returns a *slog.Logger writing structured records to stderr.
func New(cfg Config) *slog.Logger {
	return newWithWriter(cfg, os.Stderr)
}

func newWithWriter(cfg Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
