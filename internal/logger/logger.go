package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger with additional functionality
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new logger instance
func New(level, format string) (*Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level: %s", level)
	}

	var zapConfig zap.Config
	if format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		// Use production config as base for better console formatting
		zapConfig = zap.NewProductionConfig()
		zapConfig.Encoding = "console"
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapConfig.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
		zapConfig.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
		zapConfig.EncoderConfig.ConsoleSeparator = " "
		zapConfig.Development = false
		zapConfig.DisableStacktrace = true
	}

	zapConfig.Level = zap.NewAtomicLevelAt(zapLevel)
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.OutputPaths = []string{"stdout"}
	zapConfig.ErrorOutputPaths = []string{"stderr"}

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{
		SugaredLogger: logger.Sugar(),
	}, nil
}

// WithFields adds structured fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	var args []interface{}
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{
		SugaredLogger: l.SugaredLogger.With(args...),
	}
}

// Sync flushes any buffered log entries
func (l *Logger) Sync() error {
	return l.SugaredLogger.Sync()
}
