package rlog

import "time"

// MultiWriter fans records out to two sinks. It is useful when the same log
// stream needs both a durable and an interactive destination.
type MultiWriter struct {
	first  LogSink
	second LogSink
}

// NewMultiWriter constructs a composite sink that duplicates log records to both
// provided sinks.
func NewMultiWriter(first, second LogSink) MultiWriter {
	return MultiWriter{
		first:  first,
		second: second,
	}
}

func (self MultiWriter) IsEnabled(level int) bool {
	return self.first.IsEnabled(level) || self.second.IsEnabled(level)
}

func (self MultiWriter) Log(when time.Time, level int, message string) {
	self.first.Log(when, level, message)
	self.second.Log(when, level, message)
}

func (self MultiWriter) Close() {
	self.first.Close()
	self.second.Close()
}

func (self MultiWriter) Flush() {
	self.first.Flush()
	self.second.Flush()
}
