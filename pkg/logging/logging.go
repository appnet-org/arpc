package logging

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
	globalConfig *Config // Store the current config
)

// Config holds the logging configuration
type Config struct {
	Level  string `json:"level" yaml:"level"`   // debug, info, warn, error
	Format string `json:"format" yaml:"format"` // json, console
}

// DefaultConfig returns the default logging configuration
func DefaultConfig() *Config {
	return &Config{
		Level:  "info",
		Format: "console",
	}
}

// Init initializes the global logger with the given configuration
func Init(config *Config) error {
	var err error
	once.Do(func() {
		globalConfig = config
		globalLogger, err = newLogger(config)
	})
	return err
}

// GetLogger returns the global logger instance
func GetLogger() *zap.Logger {
	if globalLogger == nil {
		// Initialize with default config if not already initialized
		_ = Init(DefaultConfig())
	}
	return globalLogger
}

// newLogger creates a new zap logger with the given configuration
func newLogger(config *Config) (*zap.Logger, error) {
	// Parse log level
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// Configure encoder
	var encoderConfig zapcore.EncoderConfig
	if config.Format == "json" {
		encoderConfig = zap.NewProductionEncoderConfig()
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Create encoder
	var encoder zapcore.Encoder
	if config.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Create core
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)

	// Create logger with caller skip to skip the wrapper functions
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	return logger, nil
}

// SetLevel dynamically changes the log level
func SetLevel(level string) error {
	if globalLogger == nil {
		return Init(&Config{Level: level, Format: "console"})
	}

	_, err := zapcore.ParseLevel(level)
	if err != nil {
		return err
	}

	// Update the stored config
	if globalConfig == nil {
		globalConfig = DefaultConfig()
	}
	globalConfig.Level = level

	// Recreate the logger with the updated config
	globalLogger, err = newLogger(globalConfig)
	return err
}

// Convenience functions for common log levels
func Debug(msg string, fields ...zap.Field) {
	GetLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetLogger().Fatal(msg, fields...)
}

// Sync flushes any buffered log entries
func Sync() error {
	return GetLogger().Sync()
}
