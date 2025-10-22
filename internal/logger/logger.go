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

	var logger *zap.Logger
	var err error

	if format == "json" {
		// JSON format for production
		config := zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zapLevel)
		logger, err = config.Build()
	} else {
		// Console format with better readability
		config := zap.Config{
			Level:       zap.NewAtomicLevelAt(zapLevel),
			Development: true,
			Encoding:    "console",
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:        "ts",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				FunctionKey:    zapcore.OmitKey,
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.CapitalColorLevelEncoder,
				EncodeTime:     zapcore.RFC3339TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
		logger, err = config.Build()
	}

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
