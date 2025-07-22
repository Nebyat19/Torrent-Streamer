package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	file       *os.File
	logger     *log.Logger
	level      LogLevel
	maxSize    int64
	maxBackups int
	mu         sync.Mutex
	logPath    string
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Initialize creates and returns a singleton logger instance
func Initialize(logPath string, level LogLevel, maxSizeMB int, maxBackups int) *Logger {
	once.Do(func() {
		defaultLogger = &Logger{
			level:      level,
			maxSize:    int64(maxSizeMB) * 1024 * 1024, // Convert MB to bytes
			maxBackups: maxBackups,
			logPath:    logPath,
		}
		defaultLogger.setupLogger()
	})
	return defaultLogger
}

// GetLogger returns the singleton logger instance
func GetLogger() *Logger {
	if defaultLogger == nil {
		return Initialize("logs/app.log", INFO, 10, 5)
	}
	return defaultLogger
}

func (l *Logger) setupLogger() {
	// Create logs directory if it doesn't exist
	logDir := filepath.Dir(l.logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Failed to create log directory: %v", err)
		return
	}

	// Open log file
	file, err := os.OpenFile(l.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}

	l.file = file

	// Create multi-writer to write to both file and stdout
	multiWriter := io.MultiWriter(os.Stdout, file)
	l.logger = log.New(multiWriter, "", 0)
}

func (l *Logger) rotateLogIfNeeded() {
	if l.file == nil {
		return
	}

	info, err := l.file.Stat()
	if err != nil {
		return
	}

	if info.Size() >= l.maxSize {
		l.rotateLog()
	}
}

func (l *Logger) rotateLog() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close current file
	if l.file != nil {
		l.file.Close()
	}

	// Rotate existing log files
	for i := l.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.logPath, i)
		newPath := fmt.Sprintf("%s.%d", l.logPath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Move current log to .1
	os.Rename(l.logPath, l.logPath+".1")

	// Create new log file
	l.setupLogger()
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if log rotation is needed
	l.rotateLogIfNeeded()

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if ok {
		file = filepath.Base(file)
	} else {
		file = "unknown"
		line = 0
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := levelNames[level]

	message := fmt.Sprintf(format, args...)
	logEntry := fmt.Sprintf("[%s] [%s] [%s:%d] %s", timestamp, levelName, file, line, message)
    fmt.Println(logEntry)
	// if l.logger != nil {
	// 	l.logger.Println(logEntry)
	// } else {
	// 	// Fallback to standard log if logger is not initialized
	// 	log.Println(logEntry)
	// }
}

// Logging methods
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// Global logging functions
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

func Fatal(format string, args ...interface{}) {
	GetLogger().Fatal(format, args...)
}

// Close closes the log file
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
	}
}
