package rlog

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/term"
)

var levelNames = []string{
	"DEBUG",
	"INFO ",
	"AUDIT",
	"WARN ",
	"ERROR",
}

// ConsoleWriter renders log lines to the process STDOUT. When STDOUT is detected
// as an interactive terminal it applies ANSI colors to warn and error messages.
type ConsoleWriter struct {
	minLevel   int
	isTerminal bool
}

// NewConsoleWriter creates a console sink that only prints messages at or above
// the provided log level.
func NewConsoleWriter(minLevel int) *ConsoleWriter {
	return &ConsoleWriter{
		minLevel:   minLevel,
		isTerminal: term.IsTerminal(syscall.Stdout),
	}
}

func (self *ConsoleWriter) IsEnabled(level int) bool {
	return level >= self.minLevel
}

func (self *ConsoleWriter) Log(when time.Time, level int, message string) {
	if level < self.minLevel {
		return
	}

	var prefix string
	if level < len(levelNames) {
		prefix = levelNames[level]
	}

	color := 0

	if self.isTerminal {
		if level >= Warn {
			color = 173
		}

		if level >= Error {
			color = 167
		}
	}

	hour, min, sec := when.Clock()
	ms := when.Nanosecond() / 1e6
	if color > 0 {
		fmt.Printf("\033[38;5;%dm%02d:%02d:%02d.%03d %s %s\033[m\n", color, hour, min, sec, ms, prefix, message)
	} else {
		fmt.Printf("%02d:%02d:%02d.%03d %s %s\n", hour, min, sec, ms, prefix, message)
	}
}

func (self *ConsoleWriter) Close() {
}

func (self *ConsoleWriter) Flush() {
}
