// Package logger 提供极简的日志功能
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

// Logger 日志记录器
type Logger struct {
	level  Level
	output io.Writer
	mu     sync.Mutex
}

var std = &Logger{
	level:  INFO,
	output: os.Stdout,
}

// SetLevel 设置全局日志级别
func SetLevel(level Level) {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.level = level
}

// SetOutput 设置输出目标
func SetOutput(w io.Writer) {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.output = w
}

// log 内部日志输出函数
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	levelName := levelNames[level]
	msg := fmt.Sprintf(format, args...)

	fmt.Fprintf(l.output, "[%s] [%s] %s\n", timestamp, levelName, msg)
}

// Debug 输出 DEBUG 级别日志
func Debug(format string, args ...interface{}) {
	std.log(DEBUG, format, args...)
}

// Info 输出 INFO 级别日志
func Info(format string, args ...interface{}) {
	std.log(INFO, format, args...)
}

// Warn 输出 WARN 级别日志
func Warn(format string, args ...interface{}) {
	std.log(WARN, format, args...)
}

// Error 输出 ERROR 级别日志
func Error(format string, args ...interface{}) {
	std.log(ERROR, format, args...)
}
