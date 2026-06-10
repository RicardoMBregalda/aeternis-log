// Package logger provides centralized, structured logging for the API.
//
// It wraps zerolog and exposes a single configured global logger plus a few
// helpers. Structured logging replaces the ad-hoc fmt.Printf("DEBUG ...")
// statements that previously made the service impossible to run in production.
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// Config holds the logging options needed to build the logger. It mirrors the
// relevant fields of config.LoggingConfig so this package stays decoupled from
// the application configuration package.
type Config struct {
	Level        string // debug, info, warn, error
	Format       string // json or console
	Output       string // stdout, stderr or a file path
	EnableCaller bool
	ServiceName  string // value for the "service" field; defaults to "aeternislog-api"
}

// global is the configured logger. It defaults to a sane stderr logger so that
// any log emitted before Init (e.g. during configuration loading) is not lost.
var global = zerolog.New(os.Stderr).With().Timestamp().Logger()

// Init configures the global logger from cfg and returns it. The returned
// io.Closer is non-nil only when logging to a file and must be closed on
// shutdown to flush and release the file handle.
func Init(cfg Config) (zerolog.Logger, io.Closer, error) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.SetGlobalLevel(parseLevel(cfg.Level))

	var (
		writer io.Writer
		closer io.Closer
	)
	switch strings.ToLower(strings.TrimSpace(cfg.Output)) {
	case "", "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		f, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return global, nil, fmt.Errorf("failed to open log file %q: %w", cfg.Output, err)
		}
		writer = f
		closer = f
	}

	// Console format is human-friendly for local development; json is the
	// default and is what log aggregators (Loki, ELK, etc.) expect.
	if strings.EqualFold(cfg.Format, "console") {
		writer = zerolog.ConsoleWriter{Out: writer, TimeFormat: time.RFC3339}
	}

	service := cfg.ServiceName
	if service == "" {
		service = "aeternislog-api"
	}

	builder := zerolog.New(writer).With().Timestamp().Str("service", service)
	if cfg.EnableCaller {
		builder = builder.Caller()
	}

	global = builder.Logger()
	// Mirror into zerolog's package-level logger so internal packages can use
	// github.com/rs/zerolog/log directly without passing the logger around.
	zlog.Logger = global

	return global, closer, nil
}

// L returns the configured global logger.
func L() *zerolog.Logger {
	return &global
}

// WithRequestID returns a child logger tagged with the request ID. When id is
// empty the global logger is returned unchanged.
func WithRequestID(id string) zerolog.Logger {
	if id == "" {
		return global
	}
	return global.With().Str("request_id", id).Logger()
}

// parseLevel converts a textual level into a zerolog level, defaulting to info.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
