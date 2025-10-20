package rlog

import (
	"os"
	"sync"
)

var (
	gDefaultLoggerGuard sync.Mutex
	gDefaultLogger      Logger = NewLogger(NopSink{}, false)
)

// GetDefaultLogger returns the globally configured logger.
func GetDefaultLogger() Logger {
	gDefaultLoggerGuard.Lock()
	l := gDefaultLogger
	gDefaultLoggerGuard.Unlock()
	return l
}

// IsDebugEnabled reports whether the default logger has the Debug level enabled.
func IsDebugEnabled() bool {
	return GetDefaultLogger().IsEnabled(Debug)
}

// Debugf logs using fmt.Sprintf semantics at Debug level.
func Debugf(format string, v ...interface{}) {
	GetDefaultLogger().Debugf(format, v...)
}

// Infof logs using fmt.Sprintf semantics at Info level.
func Infof(format string, v ...interface{}) {
	GetDefaultLogger().Infof(format, v...)
}

// Auditf logs using fmt.Sprintf semantics at Audit level.
func Auditf(format string, v ...interface{}) {
	GetDefaultLogger().Auditf(format, v...)
}

// Warnf logs using fmt.Sprintf semantics at Warn level.
func Warnf(format string, v ...interface{}) {
	GetDefaultLogger().Warnf(format, v...)
}

// Errorf logs using fmt.Sprintf semantics at Error level.
func Errorf(format string, v ...interface{}) {
	GetDefaultLogger().Errorf(format, v...)
}

// Fatalf logs the message at error level and exits the process with status 1.
func Fatalf(format string, v ...interface{}) {
	GetDefaultLogger().Errorf(format, v...)
	Close()
	os.Exit(1)
}

// EnableDefaultLoggerForUtility configures the global logger for short-lived
// command line utilities. It combines console output with a buffered file sink.
func EnableDefaultLoggerForUtility() {
	gDefaultLoggerGuard.Lock()

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = NewLogger(NewMultiWriter(NewConsoleWriter(Audit), NewBufferedSink(100, NewFileWriter("", 1e8))), true)
	gDefaultLoggerGuard.Unlock()
}

// EnableDefaultLoggerForService configures the global logger for services that
// run continuously, defaulting to warning level output on the console.
func EnableDefaultLoggerForService() {
	gDefaultLoggerGuard.Lock()

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = NewLogger(NewMultiWriter(NewConsoleWriter(Warn), NewBufferedSink(100, NewFileWriter("", 1e8))), true)
	gDefaultLoggerGuard.Unlock()
}

// EnableDefaultLoggerForLogServer configures the global logger for log-forwarding
// processes that only need durable file storage.
func EnableDefaultLoggerForLogServer() {
	gDefaultLoggerGuard.Lock()

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = NewLogger(NewBufferedSink(100, NewFileWriter("", 1e9)), true)
	gDefaultLoggerGuard.Unlock()
}

// Close releases resources owned by the global logger and replaces it with a no-op logger.
func Close() {
	gDefaultLoggerGuard.Lock()

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = NewLogger(NopSink{}, false)
	gDefaultLoggerGuard.Unlock()
}
