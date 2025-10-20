package rlog

import (
	"sync"
	"time"
)

// BufferedSink decouples producers from downstream sinks by writing log entries
// into a bounded channel. The background goroutine periodically flushes the
// downstream sink to amortize I/O calls.
type BufferedSink struct {
	downstream LogSink
	queue      chan bufEntry
	wg         sync.WaitGroup
}

type bufEntry struct {
	when    time.Time
	level   int
	message string
	ack     chan struct{} // used for flush level
}

// NewBufferedSink constructs a bounded queue backed sink. When the queue is full
// producers block until there is space, trading off log retention for back pressure.
func NewBufferedSink(size int, downstream LogSink) *BufferedSink {
	s := &BufferedSink{
		downstream: downstream,
		queue:      make(chan bufEntry, size),
	}

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(time.Millisecond * 333)

		defer ticker.Stop()

		for {
			select {
			case line := <-s.queue:
				if line.level == closeLevel {
					s.downstream.Close()
					return
				}
				if line.level == flushLevel {
					s.downstream.Flush()
					if line.ack != nil {
						close(line.ack)
					}
					continue
				}
				s.downstream.Log(line.when, line.level, line.message)
			case <-ticker.C:
				s.downstream.Flush()
			}
		}
	}()

	return s
}

func (self *BufferedSink) IsEnabled(level int) bool {
	return self.downstream.IsEnabled(level)
}

func (self *BufferedSink) Log(when time.Time, level int, message string) {
	self.queue <- bufEntry{
		when:    when,
		level:   level,
		message: message,
	}
}

func (self *BufferedSink) Close() {
	self.queue <- bufEntry{level: closeLevel}
	self.wg.Wait()
}

func (self *BufferedSink) Flush() {
	ack := make(chan struct{})
	self.queue <- bufEntry{level: flushLevel, ack: ack}
	<-ack
}
