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
func (l *Logger) log(level Level, tag string, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	levelStr := colorLevel(level)
	msg := fmt.Sprintf(format, args...)

	if tag != "" {
		tagStr := colorTag(tag)
		// 固定宽度：时间8字符，级别5字符（对齐），标签6字符（对齐）
		fmt.Fprintf(l.output, "[%s][%-5s][%-6s] %s\n", Gray(timestamp), levelStr, tagStr, msg)
	} else {
		fmt.Fprintf(l.output, "[%s][%-5s] %s\n", Gray(timestamp), levelStr, msg)
	}
}

// Debug 输出 DEBUG 级别日志
func Debug(format string, args ...interface{}) {
	std.log(DEBUG, "", format, args...)
}

// Info 输出 INFO 级别日志
func Info(format string, args ...interface{}) {
	std.log(INFO, "", format, args...)
}

// Warn 输出 WARN 级别日志
func Warn(format string, args ...interface{}) {
	std.log(WARN, "", format, args...)
}

// Error 输出 ERROR 级别日志
func Error(format string, args ...interface{}) {
	std.log(ERROR, "", format, args...)
}

// 带标签的日志函数

// DebugTag 输出带标签的 DEBUG 日志
func DebugTag(tag string, format string, args ...interface{}) {
	std.log(DEBUG, tag, format, args...)
}

// InfoTag 输出带标签的 INFO 日志
func InfoTag(tag string, format string, args ...interface{}) {
	std.log(INFO, tag, format, args...)
}

// WarnTag 输出带标签的 WARN 日志
func WarnTag(tag string, format string, args ...interface{}) {
	std.log(WARN, tag, format, args...)
}

// ErrorTag 输出带标签的 ERROR 日志
func ErrorTag(tag string, format string, args ...interface{}) {
	std.log(ERROR, tag, format, args...)
}

// TruncateString 截断字符串，显示前后部分
// maxLen: 最大显示长度（rune数量）
// 返回: "前面...后面" 或原字符串
func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	if maxLen <= 6 {
		return string(runes[:maxLen]) + "..."
	}

	// 显示前后各一半
	half := (maxLen - 3) / 2
	return string(runes[:half]) + "..." + string(runes[len(runes)-half:])
}
