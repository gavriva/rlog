// Package rlog provides a minimal asynchronous logger tailored for
// high-throughput, best-effort recording of unstructured log messages.
// The package keeps the API surface tiny and accepts that queued log writes
// may block when downstream sinks stall.
package rlog

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	closeLevel = -2
	flushLevel = -1
	// Debug is the most verbose log level, intended for temporary diagnostic noise.
	Debug = 0
	// Info emits general information about application progress.
	Info = 1
	// Audit is for high-level business events that operators prefer to see by default.
	Audit = 2
	// Warn signals unexpected situations that the application can recover from.
	Warn = 3
	// Error indicates failures that require attention.
	Error = 4
)

// Logger represents the public logging surface. All methods are concurrency-safe
// and forward to the wrapped LogSink implementation.
type Logger interface {
	LogSink
	Debugf(format string, a ...interface{})
	Infof(format string, a ...interface{})
	Auditf(format string, a ...interface{})
	Warnf(format string, a ...interface{})
	Errorf(format string, a ...interface{})
}

// LogFormatter marshals formatted log messages and forwards them to the sink.
// It optionally annotates records with caller file/line metadata.
type LogFormatter struct {
	dest         LogSink
	showFileLine bool
	mut          sync.Mutex
}

// NewLogger constructs a thread-safe Logger around the provided sink.
// When showFileLine is true the resulting logger prepends caller information
// to each message, which is handy for troubleshooting utilities.
func NewLogger(sink LogSink, showFileLine bool) Logger {
	return &LogFormatter{
		dest:         sink,
		showFileLine: showFileLine,
	}
}

func (self *LogFormatter) IsEnabled(level int) bool {
	self.mut.Lock()
	r := self.dest.IsEnabled(level)
	self.mut.Unlock()
	return r
}

func (self *LogFormatter) Log(when time.Time, level int, message string) {
	self.mut.Lock()
	self.dest.Log(when, level, message)
	self.mut.Unlock()
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func (self *LogFormatter) format(level int, format string, a ...interface{}) {
	if !self.IsEnabled(level) {
		return
	}

	now := time.Now()

	buf := bufPool.Get().(*bytes.Buffer)
	if self.showFileLine {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			filename := filepath.Base(file)
			if filename == "global.go" {
				_, file, line, ok = runtime.Caller(3)
				if ok {
					filename = filepath.Base(file)
				}
			}
			_, _ = fmt.Fprintf(buf, "%s:%d: ", filename, line)
		}
	}

	_, _ = fmt.Fprintf(buf, format, a...)

	self.mut.Lock()
	self.dest.Log(now, level, buf.String())
	self.mut.Unlock()

	buf.Reset()
	bufPool.Put(buf)
}

func (self *LogFormatter) Debugf(format string, a ...interface{}) {
	self.format(Debug, format, a...)
}

func (self *LogFormatter) Infof(format string, a ...interface{}) {
	self.format(Info, format, a...)
}

func (self *LogFormatter) Auditf(format string, a ...interface{}) {
	self.format(Audit, format, a...)
}

func (self *LogFormatter) Warnf(format string, a ...interface{}) {
	self.format(Warn, format, a...)
}

func (self *LogFormatter) Errorf(format string, a ...interface{}) {
	self.format(Error, format, a...)
}

func (self *LogFormatter) Close() {
	self.mut.Lock()
	self.dest.Close()
	self.mut.Unlock()
}

func (self *LogFormatter) Flush() {
	self.mut.Lock()
	self.dest.Flush()
	self.mut.Unlock()
}

// NewDefaultLogToConsole returns a convenience logger that writes to STDOUT/STDERR
// using the ConsoleWriter with the supplied minimum log level.
func NewDefaultLogToConsole(minLevel int) Logger {
	return NewLogger(NewConsoleWriter(minLevel), true)
}

// NewDefaultForService combines console and file sinks for long-running daemons.
func NewDefaultForService() Logger {
	return NewLogger(NewMultiWriter(NewConsoleWriter(Audit), NewBufferedSink(100, NewFileWriter("", 1e8))), true)
}
