package rlog

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// FileWriter appends log records to rolling log files on disk. Rotation occurs
// once the active file exceeds maxFileSize bytes.
type FileWriter struct {
	maxFileSize int64
	maxLogFiles int
	filename    string

	fp         *os.File
	fileWriter *bufio.Writer
	fileSize   int64
}

// NewFileWriter creates a writer that rotates when the file exceeds maxSize
// bytes. When filename is empty it derives the executable name automatically.
func NewFileWriter(filename string, maxSize int64) *FileWriter {

	if len(filename) == 0 {
		filename = path.Base(os.Args[0])
		filename = strings.TrimSuffix(filename, ".bin")
		filename = strings.TrimSuffix(filename, ".app")
	}

	if maxSize == 0 {
		maxSize = 1e9
	}

	return &FileWriter{
		filename:    filename,
		maxFileSize: maxSize,
		maxLogFiles: 3,
	}
}

func (self *FileWriter) IsEnabled(level int) bool {
	return true
}

func (self *FileWriter) logFileName(num int) string {
	if num <= 0 {
		return fmt.Sprintf("%s.log", self.filename)
	}
	return fmt.Sprintf("%s.%d.log", self.filename, num)
}

// non reentrant
func (self *FileWriter) Log(when time.Time, level int, message string) {
	// Lazily open the file to avoid IO on cold start paths.
	if self.fp == nil {
		var err error
		self.fp, err = os.OpenFile(self.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil || self.fp == nil {
			return
		}

		self.fileWriter = bufio.NewWriterSize(self.fp, 128*1024)

		fi, err := self.fp.Stat()
		if err == nil {
			self.fileSize = fi.Size()
		} else {
			self.fileSize = 0
		}
		self.writeFileLine(time.Now(), Audit, fmt.Sprintf("Start %s pid: %v", self.filename, os.Getpid()))
	}

	if self.fileSize+int64(len(message))+33 > self.maxFileSize {

		_ = self.fileWriter.Flush()
		_ = self.fp.Close()

		for i := int(self.maxLogFiles) - 1; i > 0; i-- {
			_ = os.Rename(self.logFileName(i-1), self.logFileName(i))
		}
		var err error
		self.fileSize = 0
		self.fp, err = os.OpenFile(self.logFileName(0), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0664)
		if err != nil || self.fp == nil {
			self.fp = nil
			self.fileWriter = nil
			return
		}
		self.fileWriter.Reset(self.fp)
	}
	self.writeFileLine(when, level, message)
}

func (self *FileWriter) writeFileLine(when time.Time, level int, message string) {

	prefix := levelNames[level]

	year, month, day := when.Date()
	hour, min, sec := when.Clock()
	us := when.Nanosecond() / 1e3

	if self.fileWriter != nil {
		n, _ := fmt.Fprintf(self.fileWriter, "%04d-%02d-%02d %02d:%02d:%02d.%06d %s %s\n", year, month, day, hour, min, sec, us, prefix, message)
		self.fileSize += int64(n)
	}
}

func (self *FileWriter) Close() {
	self.Flush()
	self.fp.Close()
	self.fp = nil
	self.fileWriter = nil
}

func (self *FileWriter) Flush() {
	if self.fileWriter != nil {
		self.fileWriter.Flush()
	}
}
