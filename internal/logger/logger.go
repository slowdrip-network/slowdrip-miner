// internal/logger/logger.go
package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New creates a zerolog Logger with sane defaults:
// - level parsed from string (info/debug/warn/error/trace)
// - RFC3339Nano timestamps
// - JSON output by default
// - pretty console output if LOG_PRETTY=1 (useful in dev)
func New(levelStr string) zerolog.Logger {
	level := parseLevel(levelStr)

	// Global zerolog settings
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"

	var out io.Writer = os.Stdout

	// Pretty console for local dev if requested
	if os.Getenv("LOG_PRETTY") == "1" {
		cw := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05.000",
		}
		// Optional: align level field a bit nicer
		cw.FormatLevel = func(i interface{}) string {
			if ll, ok := i.(string); ok {
				return strings.ToUpper(ll)
			}
			return "?"
		}
		out = cw
	}

	l := zerolog.New(out).Level(level).With().Timestamp().Logger()
	return l
}

// parseLevel maps a string to a zerolog.Level with sensible default.
func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	case "disabled", "off", "none":
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}

// With returns a child logger with additional context fields.
// Usage: log = logger.With(log, "module", "service", "path", p)
func With(l zerolog.Logger, kv ...interface{}) zerolog.Logger {
	return l.With().Fields(kvToMap(kv...)).Logger()
}

func kvToMap(kv ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		m[key] = kv[i+1]
	}
	return m
}
