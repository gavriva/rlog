package rlog

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Level int

// Log levels.
const (
	_ Level = iota
	DEBUG
	INFO
	AUDIT
	WARNING
	ERROR
	FATAL
	DISABLED
	closeLevel
)

var levelNames = []string{
	"",
	"DEBUG",
	"INFO ",
	"AUDIT",
	"WARN ",
	"ERROR",
	"FATAL",
}

type logLine struct {
	msg   string
	level Level
	tm    time.Time
}

type Logger struct {
	inputQueue      chan logLine
	consoleQueue    chan logLine
	currentFile     *os.File
	currentFileSize int64
	options         Options

	wg sync.WaitGroup
}

// Options holds the optional parameters for a new Logger instance.
type Options struct {
	// LowerLevelToFile defines minimal log level of a message to be written into the log file.
	//
	// The default value is INFO
	LowerLevelToFile Level

	// LowerLevelToFile defines minimal log level of a message to be written to console.
	//
	// The default value is AUDIT
	LowerLevelToConsole Level

	// MaxFileSize is the maximum size in bytes of a log file before rotation.
	//
	// The default value is 100MiB
	MaxFileSize int64

	// MaxLogFiles is the maxumum number of log files.
	//
	// The default value is 3, current file + two rotated ones.
	MaxLogFiles int64

	LogfilePrefix string
}

func New(opt *Options) *Logger {
	l := &Logger{
		inputQueue: make(chan logLine, 2048),
	}

	if opt.LowerLevelToFile < DEBUG {
		opt.LowerLevelToFile = INFO
	}

	if opt.LowerLevelToConsole < DEBUG {
		opt.LowerLevelToConsole = AUDIT
	}

	if opt.MaxFileSize <= 16000 {
		opt.MaxFileSize = 100 * 1024 * 1024
	}

	if opt.MaxLogFiles <= 0 {
		opt.MaxLogFiles = 3
	}

	if opt.MaxLogFiles > 10 {
		opt.MaxLogFiles = 10
	}

	if len(opt.LogfilePrefix) == 0 {
		opt.LogfilePrefix = os.Args[0]
		if strings.HasSuffix(opt.LogfilePrefix, ".bin") || strings.HasPrefix(opt.LogfilePrefix, ".app") {
			opt.LogfilePrefix = opt.LogfilePrefix[:len(opt.LogfilePrefix)-4]
		}
	}

	l.wg.Add(1)

	go func() {
		defer l.wg.Done()
		for line := range l.inputQueue {
			if line.level >= l.options.LowerLevelToConsole {
				sent := false
				for sent == false {
					select {
					case l.consoleQueue <- line:
						sent = true
					default:
						select {
						case <-l.consoleQueue:
						default:
						}
					}
				}
			}
			if line.level >= l.options.LowerLevelToFile {
				l.newFileLine(line)
			}

			if line.level >= FATAL {
				break
			}
		}
	}()

	if opt.LowerLevelToConsole < DISABLED {
		l.consoleQueue = make(chan logLine, 101)
		l.wg.Add(1)

		go func() {
			defer l.wg.Done()
			for line := range l.consoleQueue {
				l.newConsoleLine(line)

				if line.level >= FATAL {
					break
				}
			}
		}()
	}

	return l
}

func (l *Logger) Close() {
	l.inputQueue <- logLine{"", closeLevel, time.Now()}
	l.wg.Wait()
	l.inputQueue = nil
}

func (l *Logger) writeFileLine(line logLine) {

	prefix := levelNames[line.level]

	year, month, day := line.tm.Date()
	hour, min, sec := line.tm.Clock()
	us := line.tm.Nanosecond() / 1e3

	if l.currentFile != nil {
		n, _ := fmt.Fprintf(l.currentFile, "%04d.%02d.%02d %02d:%02d:%02d.%06d %s %s\n", year, month, day, hour, min, sec, us, prefix, line.msg)
		l.currentFileSize += int64(n)
	}
}

func (l *Logger) logFileName(num int) string {
	if num <= 0 {
		return fmt.Sprintf("%s.log", l.options.LogfilePrefix)
	}
	return fmt.Sprintf("%s.%d.log", l.options.LogfilePrefix, num)
}

func (l *Logger) newFileLine(line logLine) {

	if line.level > FATAL {
		return
	}

	if l.currentFile == nil {
		var err error
		l.currentFile, err = os.OpenFile(l.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil || l.currentFile == nil {
			return
		}

		fi, err := l.currentFile.Stat()
		if err == nil {
			l.currentFileSize = fi.Size()
		} else {
			l.currentFileSize = 0
		}
		l.writeFileLine(logLine{
			msg:   "\n\n====================",
			level: AUDIT,
			tm:    time.Now(),
		})
	}

	if l.currentFileSize+int64(len(line.msg))+33 > l.options.MaxFileSize {
		for i := int(l.options.MaxLogFiles) - 1; i > 0; i-- {
			os.Rename(l.logFileName(i-1), l.logFileName(i))
		}
		var err error
		l.currentFileSize = 0
		l.currentFile, err = os.OpenFile(l.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil || l.currentFile == nil {
			return
		}
		l.writeFileLine(logLine{
			msg:   "\n\n====================",
			level: AUDIT,
			tm:    time.Now(),
		})
	}

	l.writeFileLine(line)
}

func (l *Logger) newConsoleLine(line logLine) {

	if line.level > FATAL {
		return
	}

	prefix := levelNames[line.level]
	color := 0
	if line.level >= ERROR {
		color = 167
	} else if line.level == WARNING {
		color = 173
	}
	hour, min, sec := line.tm.Clock()
	ms := line.tm.Nanosecond() / 1e6
	if color > 0 {
		fmt.Printf("\033[38;5;%dm%02d:%02d:%02d.%03d %s %s\033[m\n", color, hour, min, sec, ms, prefix, line.msg)
	} else {
		fmt.Printf("%02d:%02d:%02d.%03d %s %s\n", hour, min, sec, ms, prefix, line.msg)
	}
}

type logWriter struct {
	l     *Logger
	level Level
}

func (dw logWriter) Write(msg []byte) (int, error) {

	s := string(msg)
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}

	dw.l.inputQueue <- logLine{
		msg:   s,
		level: dw.level,
		tm:    time.Now(),
	}

	return len(msg), nil
}

func (l *Logger) NewWriterAsLevel(level Level) io.Writer {
	if level < DEBUG {
		level = DEBUG
	} else if level > FATAL {
		level = FATAL
	}

	return logWriter{
		l:     l,
		level: level,
	}
}

func (l *Logger) addLine(level Level, a []interface{}) {

	if level < l.options.LowerLevelToFile && level < l.options.LowerLevelToConsole {
		return
	}

	now := time.Now()

	var buf bytes.Buffer
	_, file, line, ok := runtime.Caller(2)
	if ok {
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				file = file[i+1:]
				break
			}
		}
		fmt.Fprintf(&buf, "%s:%d: ", file, line)
	}

	fmt.Fprint(&buf, a...)

	l.inputQueue <- logLine{
		msg:   buf.String(),
		level: level,
		tm:    now,
	}
}

func (l *Logger) addLinef(level Level, format string, a []interface{}) {

	if level < l.options.LowerLevelToFile && level < l.options.LowerLevelToConsole {
		return
	}

	now := time.Now()

	var buf bytes.Buffer
	_, file, line, ok := runtime.Caller(2)
	if ok {
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				file = file[i+1:]
				break
			}
		}
		fmt.Fprintf(&buf, "%s:%d: ", file, line)
	}

	fmt.Fprintf(&buf, format, a...)

	l.inputQueue <- logLine{
		msg:   buf.String(),
		level: level,
		tm:    now,
	}
}

func init() {
	SetDefaultLogger(New(&Options{}))
}

var gDefaultLogger *Logger

func SetDefaultLogger(l *Logger) {

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = l
}

func GetDefaultLogger() *Logger {
	return gDefaultLogger
}

func Auditf(format string, v ...interface{}) {
	gDefaultLogger.addLinef(AUDIT, format, v)
}

func Audit(v ...interface{}) {
	gDefaultLogger.addLine(AUDIT, v)
}

func Infof(format string, v ...interface{}) {
	gDefaultLogger.addLinef(INFO, format, v)
}

func Info(v ...interface{}) {
	gDefaultLogger.addLine(INFO, v)
}

func Warnf(format string, v ...interface{}) {
	gDefaultLogger.addLinef(WARNING, format, v)
}

func Warn(v ...interface{}) {
	gDefaultLogger.addLine(WARNING, v)
}

func Errorf(format string, v ...interface{}) {
	gDefaultLogger.addLinef(ERROR, format, v)
}

func Error(v ...interface{}) {
	gDefaultLogger.addLine(ERROR, v)
}

func Fatalf(format string, v ...interface{}) {
	gDefaultLogger.addLinef(FATAL, format, v)
	gDefaultLogger.Close()
	os.Exit(1)
}

func Fatal(v ...interface{}) {
	gDefaultLogger.addLine(FATAL, v)
	gDefaultLogger.Close()
	os.Exit(1)
}

func Close() {
	SetDefaultLogger(nil)
}

func Debugf(format string, v ...interface{}) {
	gDefaultLogger.addLinef(DEBUG, format, v)
}

func Debug(v ...interface{}) {
	gDefaultLogger.addLine(DEBUG, v)
}
