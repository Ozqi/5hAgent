// logger.go - 日志输出
// 功能：带标签的 DEBUG/INFO/WARN/ERROR 日志，支持颜色输出
//
//	启动时写入独立日志文件 ~/.5hAgent/logs/，不污染 TUI
//
// 主要类型：Logger, Level
// 导出函数：SetLevel, InitLog, CloseLog, DebugTag, InfoTag, WarnTag, ErrorTag, TruncateString
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

const (
	maxLogFiles   = 30
	logDirName    = "logs"
	logFilePrefix = "5hagent-"
	oldLogPrefix  = "5hagent-debug-"
)

// Logger 日志记录器
type Logger struct {
	level  Level
	output io.Writer // 实际写入器，已封装 color
	file   *os.File
	mu     sync.Mutex
}

type colorStripWriter struct {
	w io.Writer
}

func (w colorStripWriter) Write(p []byte) (int, error) {
	clean := stripANSIColors(string(p))
	_, err := w.w.Write([]byte(clean))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

var std = &Logger{
	level:  INFO,
	output: io.Discard,
}

// SetLevel 设置全局日志级别
func SetLevel(level Level) {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.level = level
}

// InitLog 初始化日志文件，写入 ~/.5hAgent/logs/
// 每次启动新建一个带时间戳的日志文件，并清理超过 maxLogFiles 个旧文件。
// logger 只写文件，不写 stdout/stderr，避免污染 TUI。
func InitLog() (string, error) {
	std.mu.Lock()
	defer std.mu.Unlock()

	configDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %w", err)
	}
	logDir := filepath.Join(configDir, ".5hAgent", logDirName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create log dir: %w", err)
	}

	// 清理旧文件（只保留最近 maxLogFiles 个）
	cleanOldLogs(logDir)

	// 新建文件
	timestamp := time.Now().Format("20060102-150405")
	logFile := filepath.Join(logDir, logFilePrefix+timestamp+".log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open log file: %w", err)
	}

	std.closeFileLocked()
	// logger 只写文件，避免 stdout/stderr 干扰 Bubble Tea/TUI 渲染。
	std.output = colorStripWriter{w: f}
	std.file = f
	cleanOldLogs(logDir)
	return logFile, nil
}

// CloseLog 关闭日志文件，恢复静默输出。
func CloseLog() {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.closeFileLocked()
	std.output = io.Discard
	std.file = nil
}

func (l *Logger) closeFileLocked() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// cleanOldLogs 删除多余的旧日志文件，只保留最近 maxLogFiles 个
func cleanOldLogs(logDir string) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	var logFiles []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasPrefix(e.Name(), logFilePrefix) || strings.HasPrefix(e.Name(), oldLogPrefix)) {
			logFiles = append(logFiles, e.Name())
		}
	}
	if len(logFiles) <= maxLogFiles {
		return
	}

	sort.Strings(logFiles)
	toDelete := logFiles[:len(logFiles)-maxLogFiles]
	for _, name := range toDelete {
		os.Remove(filepath.Join(logDir, name))
	}
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

// stripANSIColors 移除 ANSI 颜色转义码
func stripANSIColors(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' && i < len(s)-1 && s[i+1] == '[' {
			// 跳过 \033[ 到下一个字母，外层循环会继续处理后续普通字符。
			j := i + 2
			for j < len(s) {
				c := s[j]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
