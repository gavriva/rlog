## rlog

`rlog` is a minimalistic, asynchronous logger for Go services that prefer
best-effort throughput over perfect durability. It focuses on simplicity:
format the message once, push it to a buffered channel, and let background
writers take care of I/O.

### Highlights
- Structured around a tiny `LogSink` interface; mix and match sinks as needed.
- Console writer with optional ANSI colouring for warnings and errors.
- Buffered file writer with rotation and configurable size limits.
- Built-in presets surface audit-level events on the console so operators don't miss key milestones.
- Global helpers (`rlog.Infof`, `rlog.Warnf`, …) for applications that want a
  ready-to-use default logger.
- Designed to keep the main code path non-blocking during normal operation.

### Quick Start
```go
package main

import (
	"time"

	"github.com/gavriva/rlog"
)

func main() {
	logger := rlog.NewLogger(
		rlog.NewBufferedSink(128, rlog.NewFileWriter("service", 200*1024*1024)),
		true, // include caller file:line
	)
	defer logger.Close()

	logger.Infof("service started")
	time.Sleep(time.Second)
	logger.Warnf("downstream latency exceeded: %s", time.Second)
}
```

### Log levels
Level | Purpose
----- | -------
`rlog.Debug` | Noisy diagnostics for temporary troubleshooting.
`rlog.Info` | Routine operational updates.
`rlog.Audit` | High-level business events that operators should see on the console by default.
`rlog.Warn` | Recoverable anomalies that merit attention.
`rlog.Error` | Failures that need intervention.

### Built-in sinks
- `ConsoleWriter` — prints to STDOUT, optionally colourised.
- `FileWriter` — rotates log files (`app.log`, `app.1.log`, `app.2.log`) once the
  size limit is reached.
- `BufferedSink` — decouples the caller from I/O by dispatching logs through a
  bounded channel and periodic flushes.
- `MultiWriter` — mirrors logs to two sinks (for example console + file).
Combine sinks to match your deployment scenario:
```go
logger := rlog.NewLogger(
	rlog.NewMultiWriter(
		rlog.NewConsoleWriter(rlog.Audit),
		rlog.NewBufferedSink(256, rlog.NewFileWriter("", 1e8)),
	),
	true,
)
```

### Global logger
For applications that prefer a singleton logger, call one of the presets:
```go
rlog.EnableDefaultLoggerForService()
defer rlog.Close()
rlog.Infof("ready to accept requests")
```
Remember to call `rlog.Close()` during shutdown to flush buffered output.

### Design notes & trade-offs
- `BufferedSink` uses a bounded channel. When the buffer is saturated the call
  to `Log` blocks until space is available. Pick a size that fits your burst rate
  to keep the hot path non-blocking.
- There is no guarantee that every record is written before an unexpected
  process crash. If durable logging is critical, pair `rlog` with an external
  collector or append-only sink.
- File rotation keeps the three most recent log files by default.

### License
MIT — see `LICENSE` for details.
