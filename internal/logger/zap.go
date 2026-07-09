package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger returns a new *zap.Logger
func NewLogger(level, logFilePath string) *zap.Logger {
	lvl := zap.InfoLevel
	parsed, err := zapcore.ParseLevel(level)
	if err == nil {
		lvl = parsed
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	consoleWriter := zapcore.AddSync(os.Stdout)
	consoleCore := zapcore.NewCore(encoder, consoleWriter, lvl)

	if logFilePath == "" {
		return zap.New(consoleCore, zap.AddCaller())
	}

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    100,  // Megabytes before rolling to a new file
		MaxBackups: 5,    // Maximum number of old log files to retain
		MaxAge:     30,   // Maximum days to retain an old log file
		Compress:   true, // Compress rolled files using gzip
	})
	fileCore := zapcore.NewCore(encoder, fileWriter, zap.InfoLevel)

	return zap.New(fileCore, zap.AddCaller())
}
