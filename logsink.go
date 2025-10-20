package rlog

import (
	"time"
)

// LogSink represents a destination for log records. Implementations are invoked
// by the thread-safe LogFormatter, so they do not need their own synchronization
// unless they perform internal background work. Non-blocking behavior is still
// encouraged to keep log calls responsive.
type LogSink interface {
	IsEnabled(level int) bool // reentrant
	Log(when time.Time, level int, message string)
	Flush()
	Close()
}

// NopSink discards every log message and reports all levels as disabled.
// It is primarily used to keep the global logger in a safe, inert state.
type NopSink struct{}

func (self NopSink) IsEnabled(level int) bool {
	return false
}

func (self NopSink) Log(when time.Time, level int, message string) {
}

func (self NopSink) Close() {
}

func (self NopSink) Flush() {
}
