package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents log severity.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging.
type Logger struct {
	mu       sync.Mutex
	out      io.Writer
	level    Level
	prefix   string
	fields   map[string]interface{}
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Default returns the default logger instance.
func Default() *Logger {
	once.Do(func() {
		defaultLogger = New(os.Stderr, INFO, "")
	})
	return defaultLogger
}

// New creates a new logger.
func New(out io.Writer, level Level, prefix string) *Logger {
	return &Logger{
		out:    out,
		level:  level,
		prefix: prefix,
		fields: make(map[string]interface{}),
	}
}

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the output writer.
func (l *Logger) SetOutput(out io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = out
}

// Writer returns the underlying io.Writer.
func (l *Logger) Writer() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out
}

// WithField returns a new logger with the given field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value

	return &Logger{
		out:    l.out,
		level:  l.level,
		prefix: l.prefix,
		fields: newFields,
	}
}

// WithFields returns a new logger with the given fields.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		out:    l.out,
		level:  l.level,
		prefix: l.prefix,
		fields: newFields,
	}
}

func (l *Logger) log(level Level, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s", timestamp, level.String()))

	if l.prefix != "" {
		sb.WriteString(fmt.Sprintf(" [%s]", l.prefix))
	}

	if len(args) > 0 {
		sb.WriteString(fmt.Sprintf(" "+msg, args...))
	} else {
		sb.WriteString(" " + msg)
	}

	if len(l.fields) > 0 {
		sb.WriteString(" |")
		for k, v := range l.fields {
			sb.WriteString(fmt.Sprintf(" %s=%v", k, v))
		}
	}

	sb.WriteString("\n")

	if l.out != nil {
		l.out.Write([]byte(sb.String()))
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(DEBUG, msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(INFO, msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(WARN, msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(ERROR, msg, args...)
}

// Debugf logs a debug message with formatting.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Infof logs an info message with formatting.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warnf logs a warning message with formatting.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Errorf logs an error message with formatting.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// SetDefaultLevel sets the level on the default logger.
func SetDefaultLevel(level Level) {
	Default().SetLevel(level)
}

// SetDefaultOutput sets the output on the default logger.
func SetDefaultOutput(out io.Writer) {
	Default().SetOutput(out)
}

// ParseLevel parses a level string.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// InitFromEnv initializes logging from environment variables.
func InitFromEnv() *Logger {
	level := ParseLevel(os.Getenv("LOG_LEVEL"))

	// Check for log file
	var out io.Writer = os.Stderr
	if logFile := os.Getenv("LOG_FILE"); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			// Write to both stderr and file
			out = io.MultiWriter(os.Stderr, f)
		}
	}

	logger := New(out, level, "mcp-server")

	// Also set up standard log package
	log.SetOutput(out)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	return logger
}
