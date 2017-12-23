package rlog

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
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
	msg   *bytes.Buffer
	level Level
	tm    time.Time
}

type Logger struct {
	inputQueue   chan logLine
	consoleQueue chan logLine
	fp           *os.File
	fileWriter   *bufio.Writer
	fileSize     int64
	options      Options

	wg sync.WaitGroup
}

// Options holds the optional parameters for a new Logger instance.
type Options struct {
	// LowerLevelToFile defines minimal log level of a message to be written into the log file.
	//
	// The default value is INFO.
	LowerLevelToFile Level

	// LowerLevelToFile defines minimal log level of a message to be written to console.
	//
	// The default value is AUDIT.
	LowerLevelToConsole Level

	// MaxFileSize is the maximum size in bytes of a log file before rotation.
	//
	// The default value is 100MiB.
	MaxFileSize int64

	// MaxLogFiles is the maximum number of log files.
	//
	// The default value is 3, current file + two rotated ones.
	MaxLogFiles int64

	// ShowFileLine enables feature to print file name and line number of the caller.
	//
	// The default value is false.
	ShowFileLine bool

	LogfilePrefix string
}

func New(opts Options) *Logger {
	l := &Logger{
		inputQueue: make(chan logLine, 2048),
		options:    opts,
	}

	if l.options.LowerLevelToFile < DEBUG {
		l.options.LowerLevelToFile = INFO
	} else if l.options.LowerLevelToFile > DISABLED {
		l.options.LowerLevelToFile = DISABLED
	}

	if l.options.LowerLevelToConsole < DEBUG {
		l.options.LowerLevelToConsole = AUDIT
	} else if l.options.LowerLevelToConsole > DISABLED {
		l.options.LowerLevelToConsole = DISABLED
	}

	if l.options.MaxFileSize <= 16000 {
		l.options.MaxFileSize = 100 * 1024 * 1024
	}

	if l.options.MaxLogFiles <= 0 {
		l.options.MaxLogFiles = 3
	}

	if l.options.MaxLogFiles > 10 {
		l.options.MaxLogFiles = 10
	}

	if len(l.options.LogfilePrefix) == 0 {
		l.options.LogfilePrefix = path.Base(os.Args[0])
		if strings.HasSuffix(l.options.LogfilePrefix, ".bin") || strings.HasPrefix(l.options.LogfilePrefix, ".app") {
			l.options.LogfilePrefix = l.options.LogfilePrefix[:len(l.options.LogfilePrefix)-4]
		}
	}

	l.wg.Add(1)

	go func() {
		defer l.wg.Done()

		ticker := time.NewTicker(time.Millisecond * 333)

		defer ticker.Stop()

		for {
			select {
			case line := <-l.inputQueue:
				if line.level >= l.options.LowerLevelToConsole && l.consoleQueue != nil {
					sent := false
					for !sent {
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
				if line.level == closeLevel {
					return
				}
				if line.level >= l.options.LowerLevelToFile {
					l.newFileLine(line)
				}
				line.msg.Reset()
				bufPool.Put(line.msg)
			case <-ticker.C:
				if l.fp != nil {
					_ = l.fileWriter.Flush()
				}
			}
		}
	}()

	if l.options.LowerLevelToConsole <= FATAL {
		l.consoleQueue = make(chan logLine, 101)
		l.wg.Add(1)

		go func() {
			defer l.wg.Done()
			for line := range l.consoleQueue {

				if line.level == closeLevel {
					return
				}

				l.newConsoleLine(line)
			}
		}()
	}

	return l
}

func (l *Logger) Close() {
	l.inputQueue <- logLine{&bytes.Buffer{}, closeLevel, time.Time{}}
	l.wg.Wait()

	if l.fp != nil {
		_ = l.fileWriter.Flush()
		_ = l.fp.Close()
		l.fp = nil
	}
}

func (l *Logger) writeFileLine(line logLine) {

	prefix := levelNames[line.level]

	_, month, day := line.tm.Date()
	hour, min, sec := line.tm.Clock()
	us := line.tm.Nanosecond() / 1e3

	if l.fileWriter != nil {
		n, _ := fmt.Fprintf(l.fileWriter, "%02d.%02d %02d:%02d:%02d.%06d %s %s\n", month, day, hour, min, sec, us, prefix, line.msg)
		l.fileSize += int64(n)
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

	if l.fp == nil {
		var err error
		l.fp, err = os.OpenFile(l.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil || l.fp == nil {
			return
		}

		l.fileWriter = bufio.NewWriterSize(l.fp, 128*1024)

		fi, err := l.fp.Stat()
		if err == nil {
			l.fileSize = fi.Size()
		} else {
			l.fileSize = 0
		}
		buf := &bytes.Buffer{}
		buf.WriteString("\n\n====================")
		l.writeFileLine(logLine{
			msg:   buf,
			level: AUDIT,
			tm:    time.Now(),
		})
	}

	if l.fileSize+int64(line.msg.Len())+33 > l.options.MaxFileSize {

		_ = l.fileWriter.Flush()
		_ = l.fp.Close()

		for i := int(l.options.MaxLogFiles) - 1; i > 0; i-- {
			_ = os.Rename(l.logFileName(i-1), l.logFileName(i))
		}
		var err error
		l.fileSize = 0
		l.fp, err = os.OpenFile(l.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0664)
		if err != nil || l.fp == nil {
			l.fp = nil
			l.fileWriter = nil
			return
		}
		l.fileWriter.Reset(l.fp)

		buf := &bytes.Buffer{}
		buf.WriteString("\n\n====================")
		l.writeFileLine(logLine{
			msg:   buf,
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

	l := len(msg)

	if l > 0 && msg[l-1] == 0xA {
		msg = msg[:l-1]
	}

	buf := bufPool.Get().(*bytes.Buffer)
	_, _ = buf.Write(msg)

	dw.l.inputQueue <- logLine{
		msg:   buf,
		level: dw.level,
		tm:    time.Now(),
	}

	return l, nil
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

var bufPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func (l *Logger) addLine(level Level, a []interface{}) {

	if level < l.options.LowerLevelToFile && level < l.options.LowerLevelToConsole {
		return
	}

	now := time.Now()

	buf := bufPool.Get().(*bytes.Buffer)

	if level != FATAL {
		if l.options.ShowFileLine {
			_, file, line, ok := runtime.Caller(2)
			if ok {
				for i := len(file) - 1; i > 0; i-- {
					if file[i] == '/' {
						file = file[i+1:]
						break
					}
				}
				fmt.Fprintf(buf, "%s:%d: ", file, line)
			}
		}
	} else {
		io.WriteString(buf, prettyStack(7)) // nolint: errcheck
		io.WriteString(buf, "\n\n")         // nolint: errcheck
	}

	fmt.Fprint(buf, a...)

	l.inputQueue <- logLine{
		msg:   buf,
		level: level,
		tm:    now,
	}
}

func (l *Logger) addLineF(level Level, format string, a []interface{}) {

	if level < l.options.LowerLevelToFile && level < l.options.LowerLevelToConsole {
		return
	}

	now := time.Now()

	buf := bufPool.Get().(*bytes.Buffer)

	if level != FATAL {
		if l.options.ShowFileLine {
			_, file, line, ok := runtime.Caller(2)
			if ok {
				for i := len(file) - 1; i > 0; i-- {
					if file[i] == '/' {
						file = file[i+1:]
						break
					}
				}
				fmt.Fprintf(buf, "%s:%d: ", file, line)
			}
		}
	} else {
		io.WriteString(buf, "\n")           // nolint: errcheck
		io.WriteString(buf, prettyStack(7)) // nolint: errcheck
		io.WriteString(buf, "\n\n")         // nolint: errcheck
	}
	fmt.Fprintf(buf, format, a...)

	l.inputQueue <- logLine{
		msg:   buf,
		level: level,
		tm:    now,
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (l *Logger) IsDebugEnabled() bool {
	return l.options.LowerLevelToConsole <= DEBUG || l.options.LowerLevelToFile <= DEBUG
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.addLineF(DEBUG, format, v)
}

func (l *Logger) Debug(v ...interface{}) {
	l.addLine(DEBUG, v)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.addLineF(INFO, format, v)
}

func (l *Logger) Info(v ...interface{}) {
	l.addLine(INFO, v)
}

func (l *Logger) Auditf(format string, v ...interface{}) {
	l.addLineF(AUDIT, format, v)
}

func (l *Logger) Audit(v ...interface{}) {
	l.addLine(AUDIT, v)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.addLineF(WARNING, format, v)
}

func (l *Logger) Warn(v ...interface{}) {
	l.addLine(WARNING, v)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.addLineF(ERROR, format, v)
}

func (l *Logger) Error(v ...interface{}) {
	l.addLine(ERROR, v)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func init() {
	var opt Options
	if strings.Contains(os.Getenv("RLOG"), "debug") {
		opt.LowerLevelToFile = DEBUG
	}
	if strings.Contains(os.Getenv("RLOG"), "showfileline") {
		opt.ShowFileLine = true
	}
	SetDefaultLogger(New(opt))
}

var gDefaultLoggerGuard sync.Mutex
var gDefaultLogger *Logger

func SetDefaultLogger(l *Logger) {

	gDefaultLoggerGuard.Lock()

	if gDefaultLogger != nil {
		gDefaultLogger.Close()
	}
	gDefaultLogger = l
	gDefaultLoggerGuard.Unlock()
}

func GetDefaultLogger() *Logger {
	gDefaultLoggerGuard.Lock()
	l := gDefaultLogger
	gDefaultLoggerGuard.Unlock()
	return l
}

func IsDebugEnabled() bool {
	return GetDefaultLogger().IsDebugEnabled()
}

func Debugf(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(DEBUG, format, v)
}

func Debug(v ...interface{}) {
	GetDefaultLogger().addLine(DEBUG, v)
}

func Infof(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(INFO, format, v)
}

func Info(v ...interface{}) {
	GetDefaultLogger().addLine(INFO, v)
}

func Auditf(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(AUDIT, format, v)
}

func Audit(v ...interface{}) {
	GetDefaultLogger().addLine(AUDIT, v)
}

func Warnf(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(WARNING, format, v)
}

func Warn(v ...interface{}) {
	GetDefaultLogger().addLine(WARNING, v)
}

func Errorf(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(ERROR, format, v)
}

func Error(v ...interface{}) {
	GetDefaultLogger().addLine(ERROR, v)
}

func Fatalf(format string, v ...interface{}) {
	GetDefaultLogger().addLineF(FATAL, format, v)
	Close()
	os.Exit(1)
}

func Fatal(v ...interface{}) {
	GetDefaultLogger().addLine(FATAL, v)
	Close()
	os.Exit(1)
}

func NewWriterAsLevel(level Level) io.Writer {
	return GetDefaultLogger().NewWriterAsLevel(level)
}

func Close() {
	SetDefaultLogger(New(Options{LowerLevelToFile: DISABLED, LowerLevelToConsole: DISABLED, LogfilePrefix: "null"}))
}

func prettyStack(skipEntries int) string {
	b := make([]byte, 4000)
	n := runtime.Stack(b, false)
	src := b[:n]
	//fmt.Printf("%s\n", src)
	a := strings.Split(strings.Trim(string(src), " \t\r\n"), "\n")
	ss := a[skipEntries:]

	maxWidth := 0
	for i := 0; i < len(ss)-1; i += 2 {
		method := ss[i]
		file := strings.Split(strings.TrimLeft(ss[i+1], " \t"), " +")[0]
		ss[i] = file
		ss[i+1] = method
		if maxWidth < len(file) {
			maxWidth = len(file)
		}
	}
	for i := 0; i < len(ss); i += 2 {
		ss[i/2] = fmt.Sprintf("%-*s   %s", maxWidth, ss[i], ss[i+1])
	}
	return strings.Join(ss[:len(ss)/2+1], "\n")
}
