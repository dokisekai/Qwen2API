package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelNames = map[Level]string{DEBUG: "DEBUG", INFO: "INFO", WARN: "WARN", ERROR: "ERROR"}

type Logger struct {
	level      Level
	enableFile bool
	logDir     string
	maxSize    int
	maxFiles   int
	mu         sync.Mutex
	file       *os.File
}

var Default *Logger

func Init(level string, enableFile bool, logDir string, maxSize, maxFiles int) {
	lvl := INFO
	switch level {
	case "DEBUG":
		lvl = DEBUG
	case "WARN":
		lvl = WARN
	case "ERROR":
		lvl = ERROR
	}
	Default = &Logger{level: lvl, enableFile: enableFile, logDir: logDir, maxSize: maxSize, maxFiles: maxFiles}
	if enableFile {
		os.MkdirAll(logDir, 0755)
		Default.openFile()
	}
}

func (l *Logger) openFile() {
	p := filepath.Join(l.logDir, "app.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("open log file failed: %v", err)
		return
	}
	l.file = f
}

func (l *Logger) output(lvl Level, module, format string, args ...interface{}) {
	if lvl < l.level {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%s] [%s] %s", ts, levelNames[lvl], module, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Println(line)
	if l.enableFile && l.file != nil {
		l.file.WriteString(line + "\n")
	}
}

func (l *Logger) Debug(mod, f string, a ...interface{}) { l.output(DEBUG, mod, f, a...) }
func (l *Logger) Info(mod, f string, a ...interface{})  { l.output(INFO, mod, f, a...) }
func (l *Logger) Warn(mod, f string, a ...interface{})  { l.output(WARN, mod, f, a...) }
func (l *Logger) Error(mod, f string, a ...interface{}) { l.output(ERROR, mod, f, a...) }

func Debug(mod, f string, a ...interface{}) { Default.Debug(mod, f, a...) }
func Info(mod, f string, a ...interface{})  { Default.Info(mod, f, a...) }
func Warn(mod, f string, a ...interface{})  { Default.Warn(mod, f, a...) }
func Error(mod, f string, a ...interface{}) { Default.Error(mod, f, a...) }
